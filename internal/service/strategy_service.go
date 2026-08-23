package service

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/pkg/logger"
)

// StrategyService 负责把数据库里的策略编译成评分引擎用的 CompiledStrategy，
// 并提供版本感知的热加载缓存（前端改了权重，评分服务自动用新策略）。
type StrategyService struct {
	repo  *repository.StrategyRepo
	mRepo *repository.MetricRepo

	// 缓存：code -> 编译后策略 + 版本
	cache sync.Map // map[string]*cachedStrategy
}

type cachedStrategy struct {
	updatedAt time.Time
	compiled  atomic.Pointer[scoring.CompiledStrategy]
}

func NewStrategyService(repo *repository.StrategyRepo, mRepo *repository.MetricRepo) *StrategyService {
	return &StrategyService{repo: repo, mRepo: mRepo}
}

// Compile 把数据库评分策略编译成评分引擎结构。
func (s *StrategyService) Compile(strategy *model.ScoringStrategy) (*scoring.CompiledStrategy, error) {
	// 解析维度权重
	dimWeights := map[string]float64{}
	if err := json.Unmarshal([]byte(strategy.DimensionWeights), &dimWeights); err != nil {
		return nil, err
	} //解析维度权重的json数据

	// 需要知道每个指标属于哪个维度 → 查指标定义
	// 需要每个指标的维度 + 评分边界 → 查指标定义
	metrics, err := s.mRepo.ListHealthKeys()
	if err != nil {
		return nil, err
	}
	defOf := map[string]model.MetricDefinition{}
	for _, m := range metrics {
		defOf[m.MetricName] = m
	}

	rules := map[string]scoring.CompiledRule{}
	var missing []string
	for _, rule := range strategy.Rules {
		def, ok := defOf[rule.MetricKey]
		if !ok {
			//定义表里没有则跳过，不要产生Dimension=“”的规则
			missing = append(missing, rule.MetricKey)
			continue
		}
		if def.Dimension == "" {
			missing = append(missing, rule.MetricKey+"(无维度)")
			continue
		}
		//否决
		isVeto := rule.IsVeto || def.IsVeto == 1
		rules[rule.MetricKey] = scoring.CompiledRule{
			MetricKey: rule.MetricKey,
			Dimension: def.Dimension,
			Weight:    rule.Weight,
			Bounds: scoring.MetricBounds{
				ValueType:    def.ValueType,
				IsRate:       scoring.IsRateUnit(def.NormalRateUnit),
				UpperBond:    def.UpperBond,
				LowerBound:   def.LowerBound,
				WarnupBound:  def.WarnupBound,
				WarnlowBound: def.WarnlowBound,
				NormalRate:   def.NormalRate,
				WarnRate:     def.WarnRate,
				BoolNormal:   def.BoolNormal,
				BoolAbnormal: def.BoolAbnormal,
				EnumScore:    scoring.ParseEnumScore(def.EnumScore),
			},
			IsVeto:        isVeto,
			VetoThreshold: rule.VetoThreshold,
		}
	}

	if len(missing) > 0 {
		logger.L.Warnf("策略%s有%d个规则在metric_definition中缺失或无维度，已跳过：%v", strategy.Code, len(missing), missing)
	}

	//维度权重自检：key 必须能在实际指标维度里找到
	dimsInUse := map[string]bool{}
	for _, r := range rules {
		dimsInUse[r.Dimension] = true
	}
	for dim := range dimWeights {
		if !dimsInUse[dim] {
			logger.L.Warnf("策略 %s 的维度权重含无效 key %q（没有任何指标属于该维度）", strategy.Code, dim)
		}
	}
	for dim := range dimsInUse {
		if _, ok := dimWeights[dim]; !ok {
			logger.L.Warnf("策略 %s 缺少维度 %q 的权重，该维度将不计入总分", strategy.Code, dim)
		}
	}

	var neverPenalized []string
	for k, r := range rules {
		if k == "DCGM_FI_DEV_XID_ERRORS" {
			continue // XID 走专用查表
		}
		if !r.Bounds.IsScorable() {
			neverPenalized = append(neverPenalized, k)
		}
	}
	if len(neverPenalized) > 0 {
		logger.L.Warnf("策略 %s 有 %d 个参与评分的指标缺少边界，将恒定满分: %v",
			strategy.Code, len(neverPenalized), neverPenalized)

	}

	return &scoring.CompiledStrategy{
		StrategyID:       strategy.ID,
		DimensionWeights: dimWeights,
		Rules:            rules,
	}, nil
}

// GetCompiled 获取编译后的策略（带热加载：版本变了自动重编译）。
func (s *StrategyService) GetCompiled(code string) (*scoring.CompiledStrategy, error) {
	strategy, err := s.repo.GetByCode(code) //
	if err != nil {
		return nil, err
	}

	v, ok := s.cache.Load(code)
	if ok {
		cs := v.(*cachedStrategy)
		if cs.updatedAt.Equal(strategy.UpdatedAt) {
			if c := cs.compiled.Load(); c != nil {
				return c, nil // 命中缓存
			}
		}
	}

	// 重新编译并缓存
	compiled, err := s.Compile(strategy)
	if err != nil {
		return nil, err
	}
	cs := &cachedStrategy{updatedAt: strategy.UpdatedAt}
	cs.compiled.Store(compiled)
	s.cache.Store(code, cs)
	logger.L.Infof("策略 %s 已编译/重载，updated_at=%s，规则数=%d", code, strategy.UpdatedAt.Format(time.RFC3339), len(compiled.Rules))
	return compiled, nil
}

// GetCompiledDefault 取默认策略的编译结果
func (s *StrategyService) GetCompiledDefault() (*scoring.CompiledStrategy, error) {
	def, err := s.repo.GetDefault()
	if err != nil {
		return nil, err
	}
	return s.GetCompiled(def.Code)
}

// GetCompiledByID 按策略 ID 取编译结果(带缓存)。scorer 解析绑定关系后用。
func (s *StrategyService) GetCompiledByID(id uint64) (*scoring.CompiledStrategy, error) {
	strategy, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return s.GetCompiled(strategy.Code)
}
