package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/response"
)

// KGHandler 故障知识图谱 HTTP 层。
//
// 只做三件事：解析参数、调用 service、把哨兵错误翻译成 HTTP 状态码。
// 不包含任何业务判断，保证校验规则只有 service 一处定义。
type KGHandler struct {
	svc *service.KGService
}

func NewKGHandler(svc *service.KGService) *KGHandler { return &KGHandler{svc: svc} }

// fail 把 service 层的哨兵错误映射成合适的状态码。
// 未识别的错误一律 500，并且只在日志里记录细节，不把原始错误回给前端。
func (h *KGHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrKGNotFound):
		response.Fail(c, 404, err.Error())
	case errors.Is(err, service.ErrKGValidation):
		response.BadRequest(c, err.Error())
	case errors.Is(err, service.ErrKGDuplicate):
		response.Fail(c, 409, err.Error())
	case errors.Is(err, service.ErrKGConflict):
		response.Fail(c, 409, err.Error())
	default:
		logger.L.Errorf("知识图谱接口异常 path=%s: %v", c.FullPath(), err)
		response.ServerError(c, "服务内部错误，请稍后重试")
	}
}

// parseID 解析路径参数 :id。返回 0 表示解析失败，调用方需直接返回。
func parseKGID(c *gin.Context) uint64 {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "id 必须是正整数")
		return 0
	}
	return id
}

// ---------------------------------------------------------------------------
// 元数据与查询
// ---------------------------------------------------------------------------

// Meta GET /kg/meta —— 节点类型、关系类型、配色、规模统计。
func (h *KGHandler) Meta(c *gin.Context) {
	meta, err := h.svc.Meta()
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, meta)
}

// Graph GET /kg/graph —— 按筛选条件返回一个自洽的子图。
func (h *KGHandler) Graph(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	g, err := h.svc.Graph(service.GraphOptions{
		NodeType: c.Query("node_type"),
		Severity: c.Query("severity"),
		Keyword:  c.Query("keyword"),
		Limit:    limit,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, g)
}

// Neighbors GET /kg/nodes/:id/neighbors —— 以该节点为中心做有界展开。
func (h *KGHandler) Neighbors(c *gin.Context) {
	id := parseKGID(c)
	if id == 0 {
		return
	}
	depth, _ := strconv.Atoi(c.DefaultQuery("depth", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))

	g, err := h.svc.Neighbors(id, depth, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, g)
}

// ListNodes GET /kg/nodes —— 分页列表，供表格视图。
func (h *KGHandler) ListNodes(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	nodes, total, err := h.svc.ListNodesPaged(
		c.Query("node_type"), c.Query("severity"), c.Query("keyword"), limit, offset)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Page(c, total, nodes)
}

// NodeDetail GET /kg/nodes/:id —— 节点详情 + 全部关系。
func (h *KGHandler) NodeDetail(c *gin.Context) {
	id := parseKGID(c)
	if id == 0 {
		return
	}
	d, err := h.svc.NodeDetail(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, d)
}

// ---------------------------------------------------------------------------
// 节点写操作
// ---------------------------------------------------------------------------

// CreateNode POST /kg/nodes
func (h *KGHandler) CreateNode(c *gin.Context) {
	var in service.NodeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求体不是合法 JSON："+err.Error())
		return
	}
	n, err := h.svc.CreateNode(&in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

// UpdateNode PUT /kg/nodes/:id —— 需带 version 做乐观锁。
func (h *KGHandler) UpdateNode(c *gin.Context) {
	id := parseKGID(c)
	if id == 0 {
		return
	}
	var in service.NodeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求体不是合法 JSON："+err.Error())
		return
	}
	n, err := h.svc.UpdateNode(id, &in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

// DeleteNode DELETE /kg/nodes/:id —— 连带删除全部关联关系。
func (h *KGHandler) DeleteNode(c *gin.Context) {
	id := parseKGID(c)
	if id == 0 {
		return
	}
	edges, err := h.svc.DeleteNode(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted_edges": edges})
}

// ---------------------------------------------------------------------------
// 关系写操作
// ---------------------------------------------------------------------------

// CreateEdge POST /kg/edges
func (h *KGHandler) CreateEdge(c *gin.Context) {
	var in service.EdgeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求体不是合法 JSON："+err.Error())
		return
	}
	e, err := h.svc.CreateEdge(&in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, e)
}

// UpdateEdge PUT /kg/edges/:id —— 只能改说明、强度、扩展属性。
func (h *KGHandler) UpdateEdge(c *gin.Context) {
	id := parseKGID(c)
	if id == 0 {
		return
	}
	var in service.EdgeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请求体不是合法 JSON："+err.Error())
		return
	}
	e, err := h.svc.UpdateEdge(id, &in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, e)
}

// DeleteEdge DELETE /kg/edges/:id
func (h *KGHandler) DeleteEdge(c *gin.Context) {
	id := parseKGID(c)
	if id == 0 {
		return
	}
	if err := h.svc.DeleteEdge(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------------------------------------------------------------------
// 辅助接口
// ---------------------------------------------------------------------------

// MetricOptions GET /kg/metric-options —— 指标名候选，供新建指标节点时选择。
func (h *KGHandler) MetricOptions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	opts, err := h.svc.MetricOptions(c.Query("keyword"), limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, opts)
}

// ImportFaultKnowledge POST /kg/import/fault-knowledge
// 从既有故障知识条目导入，幂等，已存在的节点会被跳过而非覆盖。
func (h *KGHandler) ImportFaultKnowledge(c *gin.Context) {
	res, err := h.svc.ImportFromFaultKnowledge()
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, res)
}
