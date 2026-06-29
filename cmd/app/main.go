package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/router"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/pool"
	"github.com/robfig/cron/v3"
)

func main() {
	cfgPath := flag.String("config", "configs/local/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}
	debug := cfg.Server.Mode == "debug"
	logger.Init(debug)
	defer logger.Sync()
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// ---- 共享客户端（整个进程一套连接池）----
	db, err := repository.NewDB(cfg.MySQL, cfg.MySQL.AutoMigrate)
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}
	rc, err := redisclient.New(cfg.Redis)
	if err != nil {
		logger.L.Fatalf("连接 Redis 失败: %v", err)
	}
	defer rc.Close()
	ck, err := ckclient.New(cfg.CK)
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer ck.Close()

	c := cron.New()
	ctx := context.Background()

	// ---- 构建后台任务所需服务（按角色）----
	var loader *service.CKLoaderService
	if cfg.App.Has("loader") {
		loader = service.NewCKLoaderService(
			cfg.CK, ck, rc,
			repository.NewTopologyRepo(db),
			repository.NewMetricRepo(db),
			repository.NewStrategyRepo(db),
		)
	}

	var scorer *service.ScorerService
	var scorerPool *pool.Pool
	if cfg.App.Has("scorer") {
		scorerPool = pool.New(cfg.Scorer.Workers)
		strategySvc := service.NewStrategyService(repository.NewStrategyRepo(db), repository.NewMetricRepo(db))
		faultDetectSvc := service.NewFaultDetectService(
			repository.NewFaultEventRepo(db), repository.NewFaultRuleRepo(db),
			repository.NewMetricRepo(db), repository.NewTopologyRepo(db),
		)
		scorer = service.NewScorerService(
			rc, repository.NewHealthRepo(db), repository.NewTopologyRepo(db),
			strategySvc, cfg.Scorer.StrategyCode, cfg.Scorer.VendorStrategy, faultDetectSvc, scorerPool,
		)
	}

	// ---- 注册定时任务 ----
	scorerExpr := orDefault(cfg.Scorer.Cron, "@every 1m")
	switch {
	case loader != nil && scorer != nil:
		// 同进程双角色：loader 采集 → 内存直传 scorer，全程不经 Redis
		if _, err := c.AddFunc(scorerExpr, func() {
			frames, err := loader.Collect(ctx)
			if err != nil {
				logger.L.Errorf("CK 采集失败: %v", err)
				return // 采集失败本轮不评分，避免拿空数据覆盖快照
			}
			if err := scorer.RunOnceWith(ctx, frames); err != nil {
				logger.L.Errorf("评分失败: %v", err)
			}
		}); err != nil {
			logger.L.Fatalf("注册定时任务失败: %v", err)
		}
		logger.L.Infof("loader+scorer 内存直传模式（不经 Redis），调度=%s，workers=%d", scorerExpr, scorerPool.Workers())
	case loader != nil:
		loaderExpr := orDefault(cfg.CK.Cron, "@every 1m")
		if _, err := c.AddFunc(loaderExpr, func() {
			if err := loader.LoadOnce(ctx); err != nil {
				logger.L.Errorf("CK 拉取失败: %v", err)
			}
		}); err != nil {
			logger.L.Fatalf("注册 loader 定时任务失败: %v", err)
		}
		logger.L.Infof("仅 loader 角色（经 Redis 发布），调度=%s", loaderExpr)
	case scorer != nil:
		if _, err := c.AddFunc(scorerExpr, func() {
			if err := scorer.RunOnce(ctx); err != nil {
				logger.L.Errorf("评分失败: %v", err)
			}
		}); err != nil {
			logger.L.Fatalf("注册 scorer 定时任务失败: %v", err)
		}
		logger.L.Infof("仅 scorer 角色（从 Redis 读取），调度=%s，workers=%d", scorerExpr, scorerPool.Workers())
	}
	c.Start()

	// ---- API ----
	var srv *http.Server
	if cfg.App.Has("api") {
		srv = &http.Server{
			Addr:    cfg.Server.Addr,
			Handler: router.Setup(db, rc, cfg.Assistant, ck, cfg.CK.Table),
		}
		go func() {
			logger.L.Infof("角色 api 已启用，监听 %s", cfg.Server.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.L.Fatalf("API 启动失败: %v", err)
			}
		}()
	}

	// ---- 优雅退出 ----
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.L.Info("收到退出信号，正在停止...")
	c.Stop()
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}
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
