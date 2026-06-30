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
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/router"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/pool"
	"github.com/robfig/cron/v3"
)

func main() {
	cfgPath := flag.String("config", "configs/pre/config.yaml", "配置文件路径")
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

	db, err := repository.NewDB(cfg.MySQL, cfg.MySQL.AutoMigrate)
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}
	ck, err := ckclient.New(cfg.CK)
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer ck.Close()

	c := cron.New()
	ctx := context.Background()

	var loader *service.CKLoaderService
	if cfg.App.Has("loader") {
		loader = service.NewCKLoaderService(
			cfg.CK, ck,
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
			repository.NewHealthRepo(db), repository.NewTopologyRepo(db),
			strategySvc, cfg.Scorer.StrategyCode, cfg.Scorer.VendorStrategy, faultDetectSvc, scorerPool,
		)
	}

	scorerExpr := orDefault(cfg.Scorer.Cron, "@every 1m")
	if loader != nil && scorer != nil {
		if _, err := c.AddFunc(scorerExpr, func() {
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
	} else if loader != nil {
		loaderExpr := orDefault(cfg.CK.Cron, "@every 1m")
		if _, err := c.AddFunc(loaderExpr, func() {
			if _, err := loader.Collect(ctx); err != nil {
				logger.L.Errorf("CK 采集失败: %v", err)
			}
		}); err != nil {
			logger.L.Fatalf("注册 loader 定时任务失败: %v", err)
		}
		logger.L.Infof("仅 loader 角色，调度=%s", loaderExpr)
	}
	c.Start()

	var srv *http.Server
	if cfg.App.Has("api") {
		srv = &http.Server{
			Addr:    cfg.Server.Addr,
			Handler: router.Setup(db, cfg.Assistant, ck, cfg.CK.Table),
		}
		go func() {
			logger.L.Infof("角色 api 已启用，监听 %s", cfg.Server.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.L.Fatalf("API 启动失败: %v", err)
			}
		}()
	}

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
