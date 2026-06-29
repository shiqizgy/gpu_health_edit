package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/robfig/cron/v3"
)

//  Package main 是 CK 接入服务（ckloader）的入口。
//	每分钟从ClickHouse拉取最近窗口（默认 5 分钟）内的 GPU 采样指标，
//	完成两件事：
//	1) 同步拓扑：根据指标中的 source/sn/ip/tags 字段，将集群（Cluster）、 节点（Node）、GPU 卡（GPUCard）写入 MySQL（幂等，自动扩容）；
//	2) 写入 Redis：把整卡的全量指标打包成 MetricFrame 批量写入 Redis， 供评分服务（scorer）读取算分。
//
// 启动方式：
//	go run ./cmd/ckloader                # 常驻，按 cron 调度定时拉取
//	go run ./cmd/ckloader -once          # 只拉一轮立即退出，方便调试
//	go run ./cmd/ckloader -config xx.yaml  # 指定配置文件
//
//  与 simulator 的关系：
//	simulator用于本地/演示场景，自己造仿真数据写 Redis；
//	ckloader用于生产场景，从真实CK接入实际指标。
//	二者向同一个 Redis key 通道（gpu:metrics:{uuid}）写入，
//	**不能同时启动**，否则评分服务会同时收到两套不相关的 UUID，
//	拓扑混乱、分数不可信。

func main() {
	// -config: 配置文件路径，默认读 configs/config.yaml
	// -once:   只拉取一轮就退出，适合本地调试 / 验证 CK 连通性
	cfgPath := flag.String("config", "configs/local/config.yaml", "配置文件路径")
	once := flag.Bool("once", false, "只拉一轮(调试用)")
	flag.Parse()

	// 失败直接 panic：配置错误属于致命问题，没有兜底意义
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}

	// debug 模式下输出更详细的彩色日志，release 模式只输出 INFO 以上
	// defer Sync 保证退出前把缓冲区日志刷盘，避免日志丢失
	logger.Init(cfg.Server.Mode == "debug")
	defer logger.Sync()

	// MySQL：用于写拓扑（cluster / node / gpu_card），不做 AutoMigrate
	// （第二个参数false表示不自动建表，建表交给 server 的 AutoMigrate 流程）
	db, err := repository.NewDB(cfg.MySQL, false)
	if err != nil {
		logger.L.Fatalf("连接 MySQL 失败: %v", err)
	}

	// Redis：作为指标通道，覆盖写每张卡的最新全量指标
	// defer Close 在 main 退出前归还连接池资源
	rc, err := redisclient.New(cfg.Redis)
	if err != nil {
		logger.L.Fatalf("连接 Redis 失败: %v", err)
	}
	defer rc.Close()

	// ClickHouse：真实指标数据源
	// New 内部会做 5 秒 Ping，提前发现网络/鉴权问题
	ck, err := ckclient.New(cfg.CK)
	if err != nil {
		logger.L.Fatalf("连接 ClickHouse 失败: %v", err)
	}
	defer ck.Close()

	// ---------- 5. 组装业务服务 ----------
	// CKLoaderService 把 ck/redis/topo 三个依赖串成业务流程，
	// 内部维护 cluster/node 的内存缓存，避免每次拉取都重复查 MySQL
	loader := service.NewCKLoaderService(
		cfg.CK, ck, rc,
		repository.NewTopologyRepo(db),
		repository.NewMetricRepo(db),
		repository.NewStrategyRepo(db),
	)

	// 使用 Background 作为根上下文：定时任务无外部超时控制，
	// 每轮内部由 CK / Redis 驱动各自的连接超时
	ctx := context.Background()

	// ---------- 6. 单次模式：拉一轮立即退出 ----------
	// 适用于：CI/CD 检查、首次验证 CK 是否能拉到数据、手动补数据
	if *once {
		if err := loader.LoadOnce(ctx); err != nil {
			logger.L.Errorf("拉取失败: %v", err)
		}
		return
	}

	// ---------- 7. 常驻模式：按 cron 定时拉取 ----------
	c := cron.New()

	// 兜底默认值：配置里没填 cron 表达式时按每分钟一次
	// 注意要和 scorer 的节奏对齐，否则会出现"算分时数据已过期"或"重复算分"
	expr := cfg.CK.Cron
	if expr == "" {
		expr = "@every 1m"
	}

	// 注册定时任务：每次到点就执行 LoadOnce
	// 任意一轮失败仅打日志，不影响后续调度（避免单次故障导致服务停摆）
	// TODO: AddFunc 的返回值（EntryID, error）被忽略，建议捕获错误防止 cron 表达式非法
	c.AddFunc(expr, func() {
		if err := loader.LoadOnce(ctx); err != nil {
			logger.L.Errorf("拉取失败: %v", err)
		}
	})
	c.Start()
	logger.L.Infof("CK 接入服务启动，调度=%s", expr)

	// 启动即跑一次，不用等到下一个 cron tick：
	// 启动后立刻有数据可用，缩短"评分服务空跑"的时间窗口
	// 用 goroutine 是为了不阻塞下面的信号监听
	go loader.LoadOnce(ctx)

	// ---------- 8. 优雅停机 ----------
	// 监听 SIGINT（Ctrl+C）/ SIGTERM（容器 stop），收到后停止 cron
	// defer 链会按 LIFO 顺序关闭 ck → rc → 日志同步
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	c.Stop()
}
