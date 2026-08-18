package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// DashboardHandler 健康大盘（需求 2.1）
type DashboardHandler struct{ health *repository.HealthRepo }

func NewDashboardHandler(health *repository.HealthRepo) *DashboardHandler {
	return &DashboardHandler{health: health}
}

// Overview 大盘总览：GPU 总数、平均分、故障数、等级分布、风险最高的几张卡
func (h *DashboardHandler) Overview(c *gin.Context) {
	stats, err := h.health.GlobalStats()
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	riskN, _ := strconv.Atoi(c.DefaultQuery("risk_n", "10"))

	riskiest, err := h.health.ListRiskiest(riskN)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}

	failedCnt := stats.Levels["failed"] + stats.Levels["critical"]

	response.OK(c, gin.H{
		"total_gpu":   stats.Total,
		"avg_score":   stats.AvgScore,
		"fault_count": failedCnt,
		"level_dist":  stats.Levels,
		"riskiest":    riskiest,
	})
}
