package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

type FaultHandler struct {
	repo *repository.FaultRepo
}

func NewFaultHandler(repo *repository.FaultRepo) *FaultHandler {
	return &FaultHandler{repo: repo}
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
