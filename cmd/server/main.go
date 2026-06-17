package main

import (
	"flag"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/router"
	"github.com/gpu-health/platform/pkg/logger"
)

func main() {
	cfgPath := flag.String("config", "configs/local/config.yaml", "配置文件路径") //flag包，定义一个名为 "config" 的命令行标志（flag），并绑定一个默认值和帮助信息。部署时需要调整
	flag.Parse()                                                            //解析命令行参数。这一步必须在定义完所有标志之后、使用标志值之前调用。

	cfg, err := config.Load(*cfgPath) //调用load方法，传入之前解析出的配置文件路径
	if err != nil {
		panic(err)
	} //有错误就出发panic
	debug := cfg.Server.Mode == "debug" //从配置对象中读取 Server 字段下的 Mode 属性，将其与字符串 "debug" 进行比较，将比较结果（布尔值）赋值给变量 debug
	logger.Init(debug)                  //调用自定义的 logger 包的 Init 函数，并将布尔值 debug 传入
	defer logger.Sync()                 //确保程序退出前，把内存缓冲区里还没来得及写入磁盘的日志，全部强制刷（Flush）到磁盘文件中，防止日志丢失。

	if !debug {
		gin.SetMode(gin.ReleaseMode) //如果当前的 debug 布尔值为 false（即非调试模式），就将 Gin 框架（一个流行的 Go Web 框架）的运行模式强制设置为“发布模式”（Release Mode）。
	}

	//下面是初始化所有外部基础设施依赖
	db, err := repository.NewDB(cfg.MySQL, debug) //NewDB方法
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}
	logger.L.Info("MySQL 已连接")

	rc, err := redisclient.New(cfg.Redis) //redisclient.New方法
	if err != nil {
		logger.L.Fatalf("连接 Redis 失败: %v", err)
	}
	logger.L.Info("Redis 已连接")
	defer func() {
		if err := rc.Close(); err != nil {
			logger.L.Errorf("关闭 Redis 失败: %v", err)
		}
	}()

	ck, err := ckclient.New(cfg.CK) //ckclient.New方法
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer ck.Close()

	//调用Setup方法，启动所有服务；如果要上生产，需要补上“优雅退出”
	r := router.Setup(db, rc, cfg.Assistant, ck, cfg.CK.Table)
	logger.L.Infof("API 服务启动于 %s", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		logger.L.Fatalf("服务启动失败: %v", err)
	}
}
