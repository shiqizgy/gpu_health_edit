package main

import (
	"flag"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/router"
	"github.com/gpu-health/platform/pkg/logger"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
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

	db, err := repository.NewDB(cfg.MySQL, debug)
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}
	logger.L.Info("MySQL 已连接")

	rc, err := redisclient.New(cfg.Redis)
	if err != nil {
		logger.L.Fatalf("连接 Redis 失败: %v", err)
	}
	logger.L.Info("Redis 已连接")
	defer func() {
		if err := rc.Close(); err != nil {
			logger.L.Errorf("关闭 Redis 失败: %v", err)
		}
	}()

	r := router.Setup(db, rc, cfg.Assistant)
	logger.L.Infof("API 服务启动于 %s", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		logger.L.Fatalf("服务启动失败: %v", err)
	}
}
