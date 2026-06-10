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

// Setup 装配所有路由。handler 在这里实例化，依赖通过参数注入，结构清晰。
func Setup(db *gorm.DB, rc *redisclient.Client, assistantCfg config.AssistantConfig, ck *ckclient.Client, ckTable string) *gin.Engine {
	r := gin.Default()

	// CORS：允许前端 dev server
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))

	// 实例化仓储
	metricRepo := repository.NewMetricRepo(db)
	strategyRepo := repository.NewStrategyRepo(db)
	topoRepo := repository.NewTopologyRepo(db)
	healthRepo := repository.NewHealthRepo(db)
	faultRepo := repository.NewFaultRepo(db)
	assistantRepo := repository.NewAssistantRepo(db)

	//实例化 service
	assistantSvc := assistant.NewService(assistantCfg, topoRepo, healthRepo, metricRepo, faultRepo, rc, assistantRepo)

	// 实例化 handler
	dashboardH := handler.NewDashboardHandler(healthRepo)
	metricH := handler.NewMetricHandler(metricRepo, strategyRepo)
	strategyH := handler.NewStrategyHandler(strategyRepo, topoRepo)
	topoH := handler.NewTopologyHandler(topoRepo)
	healthH := handler.NewHealthHandler(healthRepo)
	faultH := handler.NewFaultHandler(faultRepo, rc)
	assistantH := handler.NewAssistantHandler(assistantSvc, assistantRepo)

	seriesH := handler.NewMetricSeriesHandler(ck, ckTable, topoRepo, metricRepo)

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
		api.GET("/health/gpus/:uuid", healthH.GPUDetail)
		api.GET("/health/gpus/:uuid/metrics", seriesH.GPUMetrics) // ← 新增：下钻曲线

		// 健康值
		api.GET("/health/clusters", healthH.ClusterSummaries)
		api.GET("/health/clusters/:clusterId/gpus", healthH.ClusterGPUs)

		// 故障知识图谱
		api.GET("/faults/knowledge", faultH.List)
		api.POST("/faults/knowledge", faultH.Create)
		api.PUT("/faults/knowledge/:id", faultH.Update)
		api.DELETE("/faults/knowledge/:id", faultH.Delete)

		// 故障注入（演示）
		api.POST("/faults/inject", faultH.InjectFault)
		api.GET("/faults/inject", faultH.ListFaults)

		// AI 故障分析助手(SSE 流式)
		api.POST("/assistant/chat", assistantH.Chat)
		api.GET("/assistant/conversations", assistantH.ListConversations)
		api.POST("/assistant/conversations", assistantH.CreateConversation)
		api.GET("/assistant/conversations/:id", assistantH.GetConversation)
		api.PUT("/assistant/conversations/:id", assistantH.UpdateConversation)
		api.DELETE("/assistant/conversations/:id", assistantH.DeleteConversation)
	}

	return r
}
