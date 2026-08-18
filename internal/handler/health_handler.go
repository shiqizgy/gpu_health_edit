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
	topo       *repository.TopologyRepo
}

func NewHealthHandler(
	health *repository.HealthRepo,
	metricRepo *repository.MetricRepo,
	faultEvent *repository.FaultEventRepo,
	topo *repository.TopologyRepo,
) *HealthHandler {
	return &HealthHandler{health: health, metricRepo: metricRepo, faultEvent: faultEvent, topo: topo}
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

// dimensionBreakdown 详情页雷达图用（只暴露维度分与权重，不含单指标明细）
//type dimensionBreakdown struct {
//	Dimension string  `json:"dimension"`
//	Score     float64 `json:"score"`
//	Weight    float64 `json:"weight"`
//}

// gpuMeta 详情页头部信息
type gpuMeta struct {
	ClusterName string `json:"cluster_name"`
	NodeIP      string `json:"node_ip"`
	GPUIndex    int    `json:"gpu_index"`
	Model       string `json:"model"`
	SN          string `json:"sn"`
}

// GPUDetail 单卡评分详情：snapshot + dimensions(雷达) + abnormal + faults + meta
func (h *HealthHandler) GPUDetail(c *gin.Context) {
	uuid := c.Param("uuid")
	snap, err := h.health.GetSnapshot(uuid)
	if err != nil {
		response.Fail(c, 404, "该卡暂无评分数据")
		return
	}

	// 维度明细（雷达图）：从 breakdown 解析；健康卡 breakdown 为 "null" → 返回 null
	var dims []scoring.AbnormalMetricDim
	if snap.Breakdown != "" && snap.Breakdown != "null" {
		dims = scoring.PickDimensions(snap.Breakdown)
	}

	// 异常指标：仅非健康卡解析
	abnormal := []scoring.AbnormalMetric{}
	if snap.Level != "healthy" {
		if defs, derr := h.metricRepo.AllDefsMap(); derr == nil {
			abnormal = scoring.PickAbnormal(snap.Breakdown, defs)
		}
	}

	faults, _ := h.faultEvent.ListOpenByGPU(uuid)

	// meta：从拓扑关联查（gpu_card + node + cluster）
	var meta *gpuMeta
	if h.topo != nil {
		if m, merr := h.topo.GetGPUDetailMeta(uuid); merr == nil && m != nil {
			meta = &gpuMeta{
				ClusterName: m.ClusterName,
				NodeIP:      m.NodeIP,
				GPUIndex:    m.GPUIndex,
				Model:       m.Model,
				SN:          m.SN,
			}
		}
	}

	response.OK(c, gin.H{
		"snapshot":   snap,
		"dimensions": dims, // 健康卡为 null
		"abnormal":   abnormal,
		"faults":     faults,
		"meta":       meta,
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
