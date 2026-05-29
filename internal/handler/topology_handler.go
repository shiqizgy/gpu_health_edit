package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// TopologyHandler 集群拓扑（需求 2.3-1：集群-节点-GPU 三级树，点击展开子拓扑，支持扩缩容）
type TopologyHandler struct{ topo *repository.TopologyRepo }

func NewTopologyHandler(topo *repository.TopologyRepo) *TopologyHandler {
	return &TopologyHandler{topo: topo}
}

// Clusters 顶层：列出所有集群（树的第一级）
func (h *TopologyHandler) Clusters(c *gin.Context) {
	list, err := h.topo.ListClusters()
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}

// Nodes 点击集群展开：列出该集群的节点（树的第二级）
func (h *TopologyHandler) Nodes(c *gin.Context) {
	clusterID, _ := strconv.ParseUint(c.Param("clusterId"), 10, 64)
	list, err := h.topo.ListNodesByCluster(clusterID)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}

// GPUs 节点展开：列出该节点的 GPU（树的第三级，叶子）
func (h *TopologyHandler) GPUs(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	list, err := h.topo.ListGPUsByNode(nodeID)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}

// AddGPU 动态扩容：新增一张 GPU 卡
func (h *TopologyHandler) AddGPU(c *gin.Context) {
	var g model.GPUCard
	if err := c.ShouldBindJSON(&g); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if g.UUID == "" || g.NodeID == 0 || g.ClusterID == 0 {
		response.BadRequest(c, "uuid / node_id / cluster_id 必填")
		return
	}
	if g.Status == "" {
		g.Status = "online"
	}
	if err := h.topo.UpsertGPU(&g); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, g)
}

// SetGPUStatus 动态缩容：下线/维护某张卡
func (h *TopologyHandler) SetGPUStatus(c *gin.Context) {
	uuid := c.Param("uuid")
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.topo.SetGPUStatus(uuid, req.Status); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}
