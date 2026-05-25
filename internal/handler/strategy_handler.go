package handler

import (
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// StrategyHandler 评分策略（需求 2.3-2：前端可修改指标权重和维度权重，配置为策略组）
type StrategyHandler struct{ repo *repository.StrategyRepo }

func NewStrategyHandler(repo *repository.StrategyRepo) *StrategyHandler {
	return &StrategyHandler{repo: repo}
}

func (h *StrategyHandler) List(c *gin.Context) {
	list, err := h.repo.List()
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *StrategyHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	s, err := h.repo.GetByID(id)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, s)
}

// createStrategyReq 新建策略请求
type createStrategyReq struct {
	Code             string                     `json:"code"`
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	DimensionWeights string                     `json:"dimension_weights"` // JSON 字符串
	Rules            []model.StrategyMetricRule `json:"rules"`
}

func (h *StrategyHandler) Create(c *gin.Context) {
	var req createStrategyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Code == "" || req.DimensionWeights == "" {
		response.BadRequest(c, "code 和 dimension_weights 必填")
		return
	}
	s := &model.ScoringStrategy{
		Code: req.Code, Name: req.Name, Description: req.Description,
		DimensionWeights: req.DimensionWeights, Version: 1, Rules: req.Rules,
	}
	if err := h.repo.Create(s); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, s)
}

// updateStrategyReq 更新策略（基本信息 + 维度权重）
type updateStrategyReq struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	DimensionWeights string `json:"dimension_weights"`
}

func (h *StrategyHandler) UpdateMeta(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateStrategyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.repo.UpdateMeta(id, req.Name, req.Description, req.DimensionWeights); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// UpdateRules 全量替换某策略的指标权重规则（前端编辑权重后保存）
func (h *StrategyHandler) UpdateRules(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var rules []model.StrategyMetricRule
	if err := c.ShouldBindJSON(&rules); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 添加调试日志
	log.Printf("接收到 %d 条规则", len(rules))
	for i, rule := range rules {
		log.Printf("规则 %d: MetricKey=%s, CurveType=%s, CurveParams=%s",
			i, rule.MetricKey, rule.CurveType, rule.CurveParams)
	}

	if err := h.repo.ReplaceRules(id, rules); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

func (h *StrategyHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.repo.Delete(id); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}
