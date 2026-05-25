package handler

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// FaultHandler 故障知识图谱（需求 2.4-1，纯 CRUD）+ 故障注入（演示用）
type FaultHandler struct {
	repo  *repository.FaultRepo
	redis *redisclient.Client
}

func NewFaultHandler(repo *repository.FaultRepo, redis *redisclient.Client) *FaultHandler {
	return &FaultHandler{repo: repo, redis: redis}
}

func (h *FaultHandler) List(c *gin.Context) {
	faultType := c.Query("fault_type")
	keyword := c.Query("keyword")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	list, total, err := h.repo.List(faultType, keyword, limit, offset)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.Page(c, total, list)
}

func (h *FaultHandler) Create(c *gin.Context) {
	var f model.FaultKnowledge
	if err := c.ShouldBindJSON(&f); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if f.FaultType == "" || f.Symptom == "" {
		response.BadRequest(c, "fault_type 和 symptom 必填")
		return
	}
	if err := h.repo.Create(&f); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, f)
}

func (h *FaultHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var f model.FaultKnowledge
	if err := c.ShouldBindJSON(&f); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.repo.Update(id, &f); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

func (h *FaultHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.repo.Delete(id); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// ---- 故障注入（演示用：往 Redis 写故障意图，仿真服务下一轮生成时应用）----

func (h *FaultHandler) InjectFault(c *gin.Context) {
	var req struct {
		UUID string `json:"uuid"`
		Mode string `json:"mode"` // healthy/high_temp/xid/ecc/link_down/remap_fail
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.redis.SetFault(context.Background(), req.UUID, req.Mode); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"uuid": req.UUID, "mode": req.Mode})
}

func (h *FaultHandler) ListFaults(c *gin.Context) {
	faults, err := h.redis.ListFaults(context.Background())
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, faults)
}
