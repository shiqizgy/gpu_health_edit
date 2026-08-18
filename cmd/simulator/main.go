package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/robfig/cron/v3"
)

func main() {
	cfgPath := flag.String("config", "configs/local/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}
	logger.Init(cfg.Server.Mode == "debug")
	defer logger.Sync()

	if len(cfg.Simulator.Clusters) == 0 {
		logger.L.Fatal("simulator.clusters 未配置")
	}

	ck, err := ckclient.New(cfg.CK)
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer ck.Close()

	sim := service.NewSimulatorService(cfg.Simulator, cfg.CK.Table, ck)

	ctx := context.Background()

	// 首轮立即执行一次
	if err := sim.GenerateAndInsert(ctx); err != nil {
		logger.L.Errorf("首轮仿真写入失败: %v", err)
	} else {
		logger.L.Info("首轮仿真数据已写入 CK")
	}

	c := cron.New()
	cronExpr := cfg.Simulator.Cron
	if cronExpr == "" {
		cronExpr = "@every 1m"
	}
	if _, err := c.AddFunc(cronExpr, func() {
		if err := sim.GenerateAndInsert(ctx); err != nil {
			logger.L.Errorf("仿真写入失败: %v", err)
		} else {
			logger.L.Info("仿真数据已写入 CK")
		}
	}); err != nil {
		logger.L.Fatalf("注册仿真定时任务失败: %v", err)
	}
	c.Start()
	logger.L.Infof("仿真器已启动，调度=%s，集群=%v", cronExpr, cfg.Simulator.Clusters)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.L.Info("仿真器已停止")
}
