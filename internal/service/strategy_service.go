package service

import (
	"encoding/json"
	"sync"
	"sync/atomic"

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
	cache   sync.Map // map[string]*cachedStrategy
}

type cachedStrategy struct {
	version  int64
	compiled atomic.Pointer[scoring.CompiledStrategy]
}

func NewStrategyService(repo *repository.StrategyRepo, mRepo *repository.MetricRepo) *StrategyService {
	return &StrategyService{repo: repo, mRepo: mRepo}
}

// Compile 把数据库策略编译成评分引擎结构。
func (s *StrategyService) Compile(strategy *model.ScoringStrategy) (*scoring.CompiledStrategy, error) {
	// 解析维度权重
	dimWeights := map[string]float64{}
	if err := json.Unmarshal([]byte(strategy.DimensionWeights), &dimWeights); err != nil {
		return nil, err
	}

	// 需要知道每个指标属于哪个维度 → 查指标定义
	metrics, err := s.mRepo.ListHealthKeys()
	if err != nil {
		return nil, err
	}
	dimOf := map[string]string{}
	for _, m := range metrics {
		dimOf[m.MetricKey] = m.Dimension
	}

	rules := map[string]scoring.CompiledRule{}
	for _, rule := range strategy.Rules {
		rules[rule.MetricKey] = scoring.CompiledRule{
			MetricKey:     rule.MetricKey,
			Dimension:     dimOf[rule.MetricKey],
			Weight:        rule.Weight,
			CurveType:     rule.CurveType,
			CurveParams:   rule.CurveParams,
			IsVeto:        rule.IsVeto,
			VetoThreshold: rule.VetoThreshold,
		}
	}

	return &scoring.CompiledStrategy{
		StrategyID:       strategy.ID,
		DimensionWeights: dimWeights,
		Rules:            rules,
	}, nil
}

// GetCompiled 获取编译后的策略（带热加载：版本变了自动重编译）。
func (s *StrategyService) GetCompiled(code string) (*scoring.CompiledStrategy, error) {
	strategy, err := s.repo.GetByCode(code)
	if err != nil {
		return nil, err
	}

	v, ok := s.cache.Load(code)
	if ok {
		cs := v.(*cachedStrategy)
		if cs.version == strategy.Version {
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
	cs := &cachedStrategy{version: strategy.Version}
	cs.compiled.Store(compiled)
	s.cache.Store(code, cs)
	logger.L.Infof("策略 %s 已编译/重载，版本=%d，规则数=%d", code, strategy.Version, len(compiled.Rules))
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
