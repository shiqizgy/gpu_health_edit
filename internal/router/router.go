package router

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/handler"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/service/assistant"
	"gorm.io/gorm"
)

// Setup 装配所有路由。handler 在这里实例化，依赖通过参数注入。
func Setup(db *gorm.DB, rc *redisclient.Client, assistantCfg config.AssistantConfig, ck *ckclient.Client, table string) *gin.Engine {
	//gin.Default() 自动挂载了两个中间件：Logger（记录请求日志）和 Recovery（捕获 panic 返回 500）
	r := gin.Default()

	// CORS：允许前端开发服务器（Vite 默认 5173 端口）跨域调用后端 API
	// 这个配置目前只允许localhost，如果前端部署在独立域名，需要把该域名加入AllowOrigins，或改为AllowOrigins: []string{"*"}（一般不推荐）。
	// 通常生产环境会用 Nginx 反向代理解决跨域，而不是在代码层开白名单。
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))

	// 实例化仓储
	// 每个 NewXxxRepo 都注入 *gorm.DB，它们各自封装了对应数据表的 CRUD 操作。
	metricRepo := repository.NewMetricRepo(db)
	strategyRepo := repository.NewStrategyRepo(db)
	topoRepo := repository.NewTopologyRepo(db)
	healthRepo := repository.NewHealthRepo(db)
	faultRepo := repository.NewFaultRepo(db)
	faultEventRepo := repository.NewFaultEventRepo(db)
	assistantRepo := repository.NewAssistantRepo(db)

	//实例化 service：把 AI 服务所需的所有依赖一次性注入，生成一个能真正处理业务逻辑的assistantSvc实例
	assistantSvc := assistant.NewService(assistantCfg, topoRepo, healthRepo, metricRepo, faultRepo, ck, table, assistantRepo)

	// 实例化 handler
	// 这一段是HTTP 处理器层（Handler）装配，把之前实例化的各种仓储（Repository）、服务（Service）和基础设施客户端（Redis/ClickHouse），按照“按需分配、最小依赖”的原则，注入到不同的请求处理器中
	// 涉及到：只需1个仓储、需要多个仓储、以及多个仓储的情况
	dashboardH := handler.NewDashboardHandler(healthRepo) //只要健康数据
	metricH := handler.NewMetricHandler(metricRepo, strategyRepo)
	strategyH := handler.NewStrategyHandler(strategyRepo, topoRepo)
	topoH := handler.NewTopologyHandler(topoRepo) //只要拓扑数据
	healthH := handler.NewHealthHandler(healthRepo, metricRepo, faultEventRepo)
	faultH := handler.NewFaultHandler(faultRepo, rc)
	faultEventH := handler.NewFaultEventHandler(faultEventRepo) //只要故障事件
	assistantH := handler.NewAssistantHandler(assistantSvc, assistantRepo)
	metricSeriesH := handler.NewMetricSeriesHandler(ck, table, topoRepo, metricRepo)

	api := r.Group("/api/v1")
	{
		// 健康大盘
		api.GET("/dashboard/overview", dashboardH.Overview)

		// 指标系统
		api.GET("/metrics", metricH.List)
		api.POST("/metrics", metricH.Create)
		api.PUT("/metrics/:id", metricH.Update)
		api.DELETE("/metrics/:id", metricH.Delete)

		// 评分策略
		api.GET("/strategies", strategyH.List)
		api.GET("/strategies/:id", strategyH.Get)
		api.POST("/strategies", strategyH.Create)
		api.PUT("/strategies/:id", strategyH.UpdateMeta)
		api.PUT("/strategies/:id/rules", strategyH.UpdateRules)
		api.DELETE("/strategies/:id", strategyH.Delete)
		api.PUT("/clusters/:id/strategy", strategyH.BindClusterStrategy)
		api.PUT("/gpus/:uuid/strategy", strategyH.BindGPUStrategy)

		// 集群拓扑（三级树）
		api.GET("/topology/clusters", topoH.Clusters)
		api.GET("/topology/clusters/:clusterId/nodes", topoH.Nodes)
		api.GET("/topology/nodes/:nodeId/gpus", topoH.GPUs)
		api.POST("/topology/gpus", topoH.AddGPU)                   // 扩容
		api.PUT("/topology/gpus/:uuid/status", topoH.SetGPUStatus) // 缩容
		api.GET("/topology/search", topoH.Search)                  //拓扑的GPU卡搜索

		// 健康值
		api.GET("/health/clusters", healthH.ClusterSummaries)
		api.GET("/health/clusters/:clusterId/gpus", healthH.ClusterGPUs)
		api.GET("/health/gpus/:uuid", healthH.GPUDetail)
		api.GET("/health/search", healthH.SearchGPUs) //健康值搜索

		// 故障知识图谱
		api.GET("/faults/knowledge", faultH.List)
		api.POST("/faults/knowledge", faultH.Create)
		api.PUT("/faults/knowledge/:id", faultH.Update)
		api.DELETE("/faults/knowledge/:id", faultH.Delete)

		// 故障注入（演示）
		api.POST("/faults/inject", faultH.InjectFault)
		api.GET("/faults/inject", faultH.ListFaults)

		// 故障池（一票否决 / 超阈值 / 命中规则的故障事件）
		api.GET("/faults/pool", faultEventH.List)
		api.GET("/faults/pool/stats", faultEventH.Stats)
		api.PUT("/faults/pool/:id/resolve", faultEventH.Resolve)

		// AI 故障分析助手(SSE 流式)
		api.POST("/assistant/chat", assistantH.Chat)
		api.GET("/assistant/conversations", assistantH.ListConversations)
		api.POST("/assistant/conversations", assistantH.CreateConversation)
		api.GET("/assistant/conversations/:id", assistantH.GetConversation)
		api.PUT("/assistant/conversations/:id", assistantH.UpdateConversation)
		api.DELETE("/assistant/conversations/:id", assistantH.DeleteConversation)

		// 新增：单卡时序曲线
		api.GET("/health/gpus/:uuid/metrics", metricSeriesH.GPUMetrics)
	}

	return r
}
