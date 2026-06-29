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
	"github.com/gpu-health/platform/pkg/pool"
	"github.com/robfig/cron/v3"
)

func main() {
	cfgPath := flag.String("config", "configs/local/config.yaml", "配置文件路径") //flag包，定义一个名为 "config" 的命令行标志（flag），并绑定一个默认值和帮助信息。部署时需要调整
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
	scorerPool := pool.New(cfg.Scorer.Workers)
	defer scorerPool.Close()
	scorer := service.NewScorerService(rc, healthRepo, topoRepo, strategySvc, cfg.Scorer.StrategyCode, cfg.Scorer.VendorStrategy, faultDetectSvc, scorerPool)

	if *once {
		if err := scorer.RunOnce(context.Background()); err != nil {
			logger.L.Errorf("评分失败: %v", err)
		}
		return
	}

	// 定时评分
	// cron 库是一个功能强大的定时任务调度库
	c := cron.New() //实例化一个新的调度器对象c。这个c对象是管理所有定时任务的核心。
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
