package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// HealthHandler 健康值（集群表格 → 点击展开单卡评分）
type HealthHandler struct {
	health *repository.HealthRepo
}

func NewHealthHandler(health *repository.HealthRepo) *HealthHandler {
	return &HealthHandler{health: health}
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

// GPUDetail 单卡评分详情（含维度 breakdown）
func (h *HealthHandler) GPUDetail(c *gin.Context) {
	uuid := c.Param("uuid")
	snap, err := h.health.GetSnapshot(uuid)
	if err != nil {
		response.Fail(c, 404, "该卡暂无评分数据")
		return
	}
	response.OK(c, snap)
}
