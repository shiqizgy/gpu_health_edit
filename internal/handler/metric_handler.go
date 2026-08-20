package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/response"
)

// MetricHandler 指标系统（需求 2.2）
type MetricHandler struct {
	repo         *repository.MetricRepo
	strategyRepo *repository.StrategyRepo
}

func NewMetricHandler(repo *repository.MetricRepo, strategyRepo *repository.StrategyRepo) *MetricHandler {
	return &MetricHandler{repo: repo, strategyRepo: strategyRepo}
}

func (h *MetricHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	ownerSubject, _ := strconv.Atoi(c.Query("owner_subject"))
	healthPurpose, _ := strconv.Atoi(c.Query("health_purpose"))
	valueType, _ := strconv.Atoi(c.Query("value_type"))

	q := repository.MetricQuery{
		Dimension:     c.Query("dimension"),
		CardType:      c.Query("card_type"),
		ValueType:     valueType,
		OwnerSubject:  ownerSubject,
		HealthPurpose: healthPurpose,
		Keyword:       strings.TrimSpace(c.Query("keyword")),
		HealthKeyOnly: c.Query("is_health_key") == "true",
		Limit:         limit,
		Offset:        offset,
	}

	list, total, err := h.repo.List(q)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.Page(c, total, list) // 返回 { total, items }
}

func (h *MetricHandler) Create(c *gin.Context) {
	var m model.MetricDefinition
	if err := c.ShouldBindJSON(&m); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if m.MetricName == "" || m.Dimension == "" {
		response.BadRequest(c, "metric_name 和 dimension 必填")
		return
	}
	if err := h.repo.Create(&m); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, m)
}

func (h *MetricHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var m model.MetricDefinition
	if err := c.ShouldBindJSON(&m); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 更新前先查一次旧记录，用来判断维度是否发生了变化
	old, err := h.repo.Get(id)
	if err != nil {
		response.ServerError(c, "指标不存在: "+err.Error())
		return
	}

	if err := h.repo.Update(id, &m); err != nil {
		response.ServerError(c, err.Error())
		return
	}

	// 如果维度发生了变化，把所有包含该指标的策略 version+1
	// 让评分服务在下一轮（≤5秒）自动热加载，用上新维度
	if old.Dimension != m.Dimension {
		if err := h.strategyRepo.TouchByMetricKey(old.MetricName); err != nil {
			// 不阻断主流程，记录日志即可
			// 生产环境建议用 logger，这里用标准库
			logger.L.Warn("刷新策略 updated_at 失败", err)
		}
	}

	response.OK(c, nil)
}

func (h *MetricHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.repo.Delete(id); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}
