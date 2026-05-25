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
	initOnly := flag.Bool("init", false, "只初始化拓扑后退出")
	once := flag.Bool("once", false, "初始化后只生成一轮数据")
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

	topoRepo := repository.NewTopologyRepo(db)
	sim := service.NewSimulatorService(cfg.Simulator, rc, topoRepo)

	ctx := context.Background()

	// 初始化机群拓扑（建集群/节点/GPU + 内存仿真状态）
	if err := sim.InitFleet(ctx); err != nil {
		logger.L.Fatalf("初始化机群失败: %v", err)
	}
	if *initOnly {
		logger.L.Info("拓扑初始化完成，退出")
		return
	}

	if *once {
		if err := sim.GenerateOnce(ctx); err != nil {
			logger.L.Errorf("生成数据失败: %v", err)
		}
		return
	}

	// 定时生成
	c := cron.New()
	cronExpr := cfg.Simulator.Cron
	if cronExpr == "" {
		cronExpr = "@every 1m"
	}
	_, err = c.AddFunc(cronExpr, func() {
		if err := sim.GenerateOnce(ctx); err != nil {
			logger.L.Errorf("生成数据失败: %v", err)
		}
	})
	if err != nil {
		logger.L.Fatalf("注册定时任务失败: %v", err)
	}
	c.Start()
	logger.L.Infof("仿真服务启动，调度=%s", cronExpr)

	// 启动即生成一轮
	go sim.GenerateOnce(ctx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	c.Stop()
	logger.L.Info("仿真服务已停止")
}
