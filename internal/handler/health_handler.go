package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/pkg/response"
)

// HealthHandler 健康值（集群表格 → 点击展开单卡评分 → 单卡异常指标）
type HealthHandler struct {
	health     *repository.HealthRepo
	metricRepo *repository.MetricRepo
	faultEvent *repository.FaultEventRepo
}

func NewHealthHandler(
	health *repository.HealthRepo,
	metricRepo *repository.MetricRepo,
	faultEvent *repository.FaultEventRepo,
) *HealthHandler {
	return &HealthHandler{health: health, metricRepo: metricRepo, faultEvent: faultEvent}
}

// ClusterSummaries 集群健康汇总表格（查预聚合表）
func (h *HealthHandler) ClusterSummaries(c *gin.Context) {
	list, err := h.health.ListClusterSummaries()
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ClusterGPUs 点击某个集群：分页展示该集群内每张 GPU 的评分（坏卡置顶）
func (h *HealthHandler) ClusterGPUs(c *gin.Context) {
	clusterID, _ := strconv.ParseUint(c.Param("clusterId"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	list, total, err := h.health.ListSnapshotsByCluster(clusterID, limit, offset)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.Page(c, total, list)
}

// GPUDetail 单卡评分详情：快照 + 异常指标(需求1) + 当前 open 故障
func (h *HealthHandler) GPUDetail(c *gin.Context) {
	uuid := c.Param("uuid")
	snap, err := h.health.GetSnapshot(uuid)
	if err != nil {
		response.Fail(c, 404, "该卡暂无评分数据")
		return
	}

	// 仅当不健康时解析异常指标；健康卡返回空列表
	abnormal := []scoring.AbnormalMetric{}
	if snap.Level != "healthy" {
		defs, derr := h.metricRepo.AllDefsMap()
		if derr == nil {
			abnormal = scoring.PickAbnormal(snap.Breakdown, defs)
		}
	}

	faults, _ := h.faultEvent.ListOpenByGPU(uuid)

	response.OK(c, gin.H{
		"snapshot": snap,
		"abnormal": abnormal,
		"faults":   faults,
	})
}

func (h *HealthHandler) SearchGPUs(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		response.BadRequest(c, "搜索关键词不能为空")
		return
	}
	list, err := h.health.SearchSnapshots(keyword, 50)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}
