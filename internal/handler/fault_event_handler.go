package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// FaultEventHandler 故障池（分页 + 多条件搜索 + 统计 + 销项）
type FaultEventHandler struct {
	repo *repository.FaultEventRepo
}

func NewFaultEventHandler(repo *repository.FaultEventRepo) *FaultEventHandler {
	return &FaultEventHandler{repo: repo}
}

// List GET /api/v1/faults/pool
func (h *FaultEventHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	q := repository.FaultEventQuery{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Severity: c.Query("severity"),
		Status:   c.DefaultQuery("status", "open"), // 默认只看进行中
		Since:    c.Query("since"),
		Until:    c.Query("until"),
		Limit:    limit,
		Offset:   offset,
	}
	if cid := c.Query("cluster_id"); cid != "" {
		if v, err := strconv.ParseUint(cid, 10, 64); err == nil {
			q.ClusterID = &v
		}
	}
	if q.Status == "all" {
		q.Status = ""
	}

	list, total, err := h.repo.List(q)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.Page(c, total, list)
}

// Stats GET /api/v1/faults/pool/stats
func (h *FaultEventHandler) Stats(c *gin.Context) {
	stats, err := h.repo.Stats()
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, stats)
}

// Resolve PUT /api/v1/faults/pool/:id/resolve
func (h *FaultEventHandler) Resolve(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.repo.Resolve(id); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}
