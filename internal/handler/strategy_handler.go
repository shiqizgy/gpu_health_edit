package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

// StrategyHandler 评分策略（需求 2.3-2：前端可修改指标权重和维度权重，配置为策略组）
type StrategyHandler struct {
	repo *repository.StrategyRepo
	topo *repository.TopologyRepo
}

func NewStrategyHandler(repo *repository.StrategyRepo, topo *repository.TopologyRepo) *StrategyHandler {
	return &StrategyHandler{repo: repo, topo: topo}
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
	dw, err := normalizeDimWeights(req.DimensionWeights)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	req.DimensionWeights = dw
	s := &model.ScoringStrategy{
		Code: req.Code, Name: req.Name, Description: req.Description,
		DimensionWeights: req.DimensionWeights, Rules: req.Rules,
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
	dw, err := normalizeDimWeights(req.DimensionWeights)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.repo.UpdateMeta(id, req.Name, req.Description, dw); err != nil {
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
	//log.Printf("接收到 %d 条规则", len(rules))
	//for i, rule := range rules {
	//	log.Printf("规则 %d: MetricKey=%s, Weight=%.3f, IsVeto=%v",
	//		i, rule.MetricKey, rule.Weight, rule.IsVeto)
	//}

	if err := h.repo.ReplaceRules(id, rules); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

func (h *StrategyHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	// 保护 default 策略
	s, err := h.repo.GetByID(id)
	if err != nil {
		response.Fail(c, 404, "策略不存在")
		return
	}
	if s.IsDefault {
		response.BadRequest(c, "默认策略不可删除")
		return
	}
	// 检查是否还有集群/卡在用它
	cc, gc := h.topo.CountStrategyUsage(id)
	if cc > 0 || gc > 0 {
		response.BadRequest(c, "该策略仍被 "+strconv.FormatInt(cc, 10)+" 个集群、"+strconv.FormatInt(gc, 10)+" 张卡使用,请先解绑")
		return
	}
	if err := h.repo.Delete(id); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// BindClusterStrategy 给集群绑定/解绑策略。body: {"strategy_id": 2} 绑定;{"strategy_id": null} 解绑
func (h *StrategyHandler) BindClusterStrategy(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		StrategyID *uint64 `json:"strategy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.topo.BindClusterStrategy(id, req.StrategyID); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// BindGPUStrategy 给单卡绑定/解绑策略
func (h *StrategyHandler) BindGPUStrategy(c *gin.Context) {
	uuid := c.Param("uuid")
	var req struct {
		StrategyID *uint64 `json:"strategy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.topo.BindGPUStrategy(uuid, req.StrategyID); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// normalizeDimWeights 校验并规范化维度权重：
// 必须是 {"维度名": 数字} 形式的 JSON 对象，权重非负且总和为 1。
// 返回重新序列化后的 JSON，保证库里永远是"对象 + 数字"，前端和评分引擎都能正确解析。
func normalizeDimWeights(raw string) (string, error) {
	var m map[string]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", fmt.Errorf("dimension_weights 必须是 {\"维度\": 数字} 形式的 JSON 对象: %v", err)
	}
	if len(m) == 0 {
		return "", errors.New("dimension_weights 不能为空")
	}
	sum := 0.0
	for k, v := range m {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", fmt.Errorf("维度 %s 的权重非法: %v", k, v)
		}
		sum += v
	}
	if math.Abs(sum-1) > 0.001 {
		return "", fmt.Errorf("维度权重之和必须为 1，当前为 %.3f", sum)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
