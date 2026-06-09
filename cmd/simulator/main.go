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
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	once := flag.Bool("once", false, "只生成一轮")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}
	logger.Init(cfg.Server.Mode == "debug")
	defer logger.Sync()

	ck, err := ckclient.New(cfg.CK)
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer func() {
		if err := ck.Close(); err != nil {
		}
	}()

	sim := service.NewSimulatorService(cfg.Simulator, cfg.CK.Table, ck)
	ctx := context.Background()

	if *once {
		if err := sim.GenerateAndInsert(ctx); err != nil {
			logger.L.Errorf("生成失败: %v", err)
		}
		return
	}
	c := cron.New()
	c.AddFunc(orDefault(cfg.Simulator.Cron), func() {
		if err := sim.GenerateAndInsert(ctx); err != nil {
			logger.L.Errorf("生成失败: %v", err)
		}
	})
	c.Start()
	go sim.GenerateAndInsert(ctx)
	logger.L.Info("CK 仿真服务启动")
	waitSignal()
	c.Stop()
}

// orDefault 返回非空字符串；若 s 为空则返回 def。
func orDefault(s string) string {
	if s == "" {
		return "空"
	}
	return s
}

// waitSignal 阻塞等待操作系统中断信号（SIGINT / SIGTERM），
// 收到后返回，调用方随后执行清理逻辑。
func waitSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
