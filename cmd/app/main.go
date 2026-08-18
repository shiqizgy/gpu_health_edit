package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/router"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/pool"
	"github.com/robfig/cron/v3"
)

func main() {
	// 从命令行读取配置文件路径,默认 configs/local/config.yaml
	cfgPath := flag.String("config", "configs/local/config.yaml", "配置文件路径")
	flag.Parse()

	//加载配置,失败直接 panic(启动阶段致命错误)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}

	// 根据配置的运行模式决定是否 debug
	debug := cfg.Server.Mode == "debug"
	logger.Init(debug)  // 初始化全局日志器
	defer logger.Sync() // 程序退出前刷盘日志缓冲
	if !debug {
		gin.SetMode(gin.ReleaseMode) // 生产模式关闭 gin 的调试输出
	}

	// 连接 MySQL(业务数据库),AutoMigrate 控制是否自动建表
	db, err := repository.NewDB(cfg.MySQL, cfg.MySQL.AutoMigrate)
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}
	// 连接 ClickHouse(指标时序数据源)
	ck, err := ckclient.New(cfg.CK)
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer ck.Close()

	c := cron.New()             // cron 定时调度器
	ctx := context.Background() // 传递给采集/评分任务的根上下文

	var loader *service.CKLoaderService
	// 仅当配置的角色包含 "loader" 时才创建采集服务
	if cfg.App.Has("loader") {
		loader = service.NewCKLoaderService(
			cfg.CK, ck, // CK 配置 + 客户端
			repository.NewTopologyRepo(db), // 拓扑仓储(集群/节点/卡)
			repository.NewMetricRepo(db),   // 指标定义仓储
			repository.NewStrategyRepo(db), // 策略仓储
		)
	}

	var scorer *service.ScorerService
	var scorerPool *pool.Pool
	// 仅当角色包含 "scorer" 时创建评分服务
	if cfg.App.Has("scorer") {
		scorerPool = pool.New(cfg.Scorer.Workers) // 并行打分的协程池
		// 策略服务:加载/编译评分策略
		strategySvc := service.NewStrategyService(repository.NewStrategyRepo(db), repository.NewMetricRepo(db))
		// 故障检测服务:评分后基于规则识别故障事件
		faultDetectSvc := service.NewFaultDetectService(
			repository.NewFaultEventRepo(db), repository.NewFaultRuleRepo(db),
			repository.NewMetricRepo(db), repository.NewTopologyRepo(db),
		)
		// 评分服务:健康仓储 + 拓扑 + 策略 + 默认策略码 + vendor策略映射 + 故障检测 + 协程池
		scorer = service.NewScorerService(
			repository.NewHealthRepo(db), repository.NewTopologyRepo(db),
			strategySvc, cfg.Scorer.StrategyCode, cfg.Scorer.VendorStrategy, faultDetectSvc, scorerPool,
		)
	}

	var runMu sync.Mutex                                  // 互斥锁:保证同一时刻只有一轮任务在跑,防止重叠
	scorerExpr := orDefault(cfg.Scorer.Cron, "@every 1m") // 调度表达式,默认每分钟
	// 情况 A:同时是 loader + scorer → "内存直传模式"(采集完直接在内存里传给评分)
	if loader != nil && scorer != nil {
		if _, err := c.AddFunc(scorerExpr, func() {
			if !runMu.TryLock() {
				logger.L.Warn("上一轮采集+评分未完成，跳过本轮")
				return
			}
			defer runMu.Unlock()
			frames, err := loader.Collect(ctx)
			if err != nil {
				logger.L.Errorf("CK 采集失败: %v", err)
				return
			}
			if err := scorer.RunOnceWith(ctx, frames); err != nil {
				logger.L.Errorf("评分失败: %v", err)
			}
		}); err != nil {
			logger.L.Fatalf("注册定时任务失败: %v", err)
		}
		logger.L.Infof("loader+scorer 内存直传模式，调度=%s，workers=%d", scorerExpr, scorerPool.Workers())
		// 情况 B:仅 loader(采集器独立部署,评分在别的实例)→ 只采集
	} else if loader != nil {
		loaderExpr := orDefault(cfg.CK.Cron, "@every 1m")
		if _, err := c.AddFunc(loaderExpr, func() {
			if !runMu.TryLock() {
				logger.L.Warn("上一轮采集未完成，跳过本轮")
				return
			}
			defer runMu.Unlock()
			if _, err := loader.Collect(ctx); err != nil {
				logger.L.Errorf("CK 采集失败: %v", err)
			}
		}); err != nil {
			logger.L.Fatalf("注册 loader 定时任务失败: %v", err)
		}
		logger.L.Infof("仅 loader 角色，调度=%s", loaderExpr)
	}

	// 数据保留清理（仅在 loader 或 scorer 角色实例上跑，避免多实例重复删）
	if cfg.Retention.Enabled && (cfg.App.Has("scorer") || cfg.App.Has("loader")) {
		retention := service.NewRetentionService(
			cfg.Retention,
			repository.NewFaultEventRepo(db),
			repository.NewAssistantRepo(db),
		)
		retExpr := orDefault(cfg.Retention.Cron, "0 0 3 * * *")
		if _, err := c.AddFunc(retExpr, retention.RunOnce); err != nil {
			logger.L.Fatalf("注册数据清理任务失败: %v", err)
		}
		logger.L.Infof("数据保留清理已启用，调度=%s，保留=%d天", retExpr, cfg.Retention.RetainDays)
	}

	c.Start() // 启动所有已注册的 cron 任务(非阻塞,后台运行)

	var srv *http.Server
	// 仅当角色包含 "api" 时启动 HTTP 服务
	if cfg.App.Has("api") {
		srv = &http.Server{
			Addr: cfg.Server.Addr,
			// router.Setup 注册所有路由,注入 db/助手配置/CK 客户端/表名
			Handler: router.Setup(db, cfg.Assistant, ck, cfg.CK.Table),
		}
		// 在独立 goroutine 里监听,避免阻塞主线程(主线程要去等退出信号)
		go func() {
			logger.L.Infof("角色 api 已启用，监听 %s", cfg.Server.Addr)
			// ErrServerClosed 是正常关闭,不算错误
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.L.Fatalf("API 启动失败: %v", err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	// 监听中断/终止信号(Ctrl+C 或 kill)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig // 阻塞主线程,直到收到信号
	logger.L.Info("收到退出信号，正在停止...")
	c.Stop() //停止 cron(不再触发新任务)

	//优雅关闭 HTTP 服务,给 10 秒处理存量请求
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}

	// 关闭协程池,等待在跑的打分任务结束
	if scorerPool != nil {
		scorerPool.Close()
	}
	logger.L.Info("app 已停止")
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
