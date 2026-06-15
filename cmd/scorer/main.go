package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/robfig/cron/v3"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	once := flag.Bool("once", false, "只执行一次(调试用)")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}
	logger.Init(cfg.Server.Mode == "debug")
	defer logger.Sync()

	db, err := repository.NewDB(cfg.MySQL, false)
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}
	rc, err := redisclient.New(cfg.Redis)
	if err != nil {
		logger.L.Fatalf("连接 Redis 失败: %v", err)
	}
	defer rc.Close()

	strategyRepo := repository.NewStrategyRepo(db)
	metricRepo := repository.NewMetricRepo(db)
	healthRepo := repository.NewHealthRepo(db)
	topoRepo := repository.NewTopologyRepo(db)
	faultEventRepo := repository.NewFaultEventRepo(db)
	faultRuleRepo := repository.NewFaultRuleRepo(db)

	strategySvc := service.NewStrategyService(strategyRepo, metricRepo)
	faultDetectSvc := service.NewFaultDetectService(faultEventRepo, faultRuleRepo, metricRepo, topoRepo)
	scorer := service.NewScorerService(rc, healthRepo, topoRepo, strategySvc, cfg.Scorer.StrategyCode, faultDetectSvc)

	if *once {
		if err := scorer.RunOnce(context.Background()); err != nil {
			logger.L.Errorf("评分失败: %v", err)
		}
		return
	}

	// 定时评分
	c := cron.New()
	cronExpr := cfg.Scorer.Cron
	if cronExpr == "" {
		cronExpr = "@every 1m"
	}
	_, err = c.AddFunc(cronExpr, func() {
		if err := scorer.RunOnce(context.Background()); err != nil {
			logger.L.Errorf("评分失败: %v", err)
		}
	})
	if err != nil {
		logger.L.Fatalf("注册定时任务失败: %v", err)
	}
	c.Start()
	logger.L.Infof("评分服务启动，调度=%s，策略=%s", cronExpr, cfg.Scorer.StrategyCode)

	// 启动即跑一次
	go scorer.RunOnce(context.Background())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	c.Stop()
	logger.L.Info("评分服务已停止")
}
