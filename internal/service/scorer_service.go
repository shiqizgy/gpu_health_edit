package service

import (
	"context"
	"time"

	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/internal/types"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/pool"
)

type ScorerService struct {
	health         *repository.HealthRepo
	topo           *repository.TopologyRepo
	strategy       *StrategyService
	strategyCode   string
	vendorStrategy map[string]string
	faultDetect    *FaultDetectService
	pool           *pool.Pool
}

func NewScorerService(
	health *repository.HealthRepo,
	topo *repository.TopologyRepo,
	strategy *StrategyService,
	strategyCode string,
	vendorStrategy map[string]string,
	faultDetect *FaultDetectService,
	p *pool.Pool,
) *ScorerService {
	if vendorStrategy == nil {
		vendorStrategy = map[string]string{}
	}
	return &ScorerService{
		health:         health,
		topo:           topo,
		strategy:       strategy,
		strategyCode:   strategyCode,
		vendorStrategy: vendorStrategy,
		faultDetect:    faultDetect,
		pool:           p,
	}
}

// RunOnceWith 执行一轮评分。策略优先级：卡级 > 集群级 > vendor级 > 默认
func (s *ScorerService) RunOnceWith(ctx context.Context, frames []types.MetricFrame) error {
	start := time.Now()

	//先编译默认策略defaultStrategy（GetCompiled，带版本热加载缓存）。frames 为空直接返回
	//主要把分散在多张表里的原始配置，预处理、拼装成评分引擎能直接高效使用的内存结构
	defaultStrategy, err := s.strategy.GetCompiled(s.strategyCode)
	if err != nil {
		logger.L.Errorf("加载默认策略失败: %v", err)
		return err
	}

	if len(frames) == 0 {
		logger.L.Warn("本轮无指标数据，跳过评分")
		return nil
	}

	//查所有在线卡及其策略绑定
	gpus, err := s.topo.AllOnlineGPUsWithStrategy()
	if err != nil {
		return err
	}

	//策略ID的优先级选择：卡级CardStrategyID > 集群级ClusterStrategyID，都没有则 sid=nil（后续走 vendor 或默认）。
	type binding struct {
		clusterID  uint64
		vendor     string
		strategyID *uint64
	}
	bindOf := make(map[string]binding, len(gpus))
	for _, g := range gpus {
		var sid *uint64
		if g.CardStrategyID != nil {
			sid = g.CardStrategyID
		} else if g.ClusterStrategyID != nil {
			sid = g.ClusterStrategyID
		}
		bindOf[g.UUID] = binding{clusterID: g.ClusterID, vendor: g.Vendor, strategyID: sid}
	}

	compiledCache := map[uint64]*scoring.CompiledStrategy{}
	for _, b := range bindOf {
		if b.strategyID == nil {
			continue
		}
		if _, ok := compiledCache[*b.strategyID]; ok {
			continue
		}
		cs, err := s.strategy.GetCompiledByID(*b.strategyID)
		if err != nil {
			logger.L.Warnf("策略 ID=%d 编译失败,该绑定将回退默认: %v", *b.strategyID, err)
			continue
		}
		compiledCache[*b.strategyID] = cs
	}

	vendorCompiled := map[string]*scoring.CompiledStrategy{}
	for vendor, code := range s.vendorStrategy {
		cs, err := s.strategy.GetCompiled(code)
		if err != nil {
			logger.L.Warnf("vendor=%s 策略 code=%s 编译失败: %v", vendor, code, err)
			continue
		}
		vendorCompiled[vendor] = cs
	}

	now := time.Now()
	snaps := make([]model.GPUHealthSnapshot, len(frames))
	entries := make([]scoring.CardScore, len(frames))
	s.pool.Partition(len(frames), func(start, end int) {
		for i := start; i < end; i++ {
			f := frames[i]
			compiled := defaultStrategy
			clusterID := uint64(0)
			if b, known := bindOf[f.UUID]; known {
				clusterID = b.clusterID
				if b.strategyID != nil {
					if cs, ok := compiledCache[*b.strategyID]; ok {
						compiled = cs
					}
				} else if cs, ok := vendorCompiled[b.vendor]; ok {
					compiled = cs
				}
			}

			result := scoring.Score(f.Metrics, compiled)
			entries[i] = scoring.CardScore{Metrics: f.Metrics, Result: result}

			snap := model.GPUHealthSnapshot{
				GPUUUID:    f.UUID,
				ClusterID:  clusterID,
				StrategyID: compiled.StrategyID,
				Score:      result.Score,
				Level:      result.Level,
				Veto:       result.Veto,
				VetoReason: result.VetoReason,
				Breakdown:  scoring.BreakdownJSON(result),
				ScoredAt:   now,
			}
			if result.Level != "healthy" {
				snap.Breakdown = scoring.BreakdownJSON(result)
			}
			snaps[i] = snap
		}
	})

	cardScores := make(map[string]scoring.CardScore, len(frames))
	for i, f := range frames {
		cardScores[f.UUID] = entries[i]
	}

	if err := s.health.BatchUpsertSnapshotsConcurrent(snaps, 8); err != nil {
		logger.L.Errorf("写快照失败: %v", err)
		return err
	}

	if err := s.health.RecomputeClusterSummaries(); err != nil {
		logger.L.Errorf("重算集群汇总失败: %v", err)
		return err
	}

	if s.faultDetect != nil {
		if err := s.faultDetect.Process(ctx, now, cardScores); err != nil {
			logger.L.Errorf("故障检测失败: %v", err)
		}
	}

	logger.L.Infof("评分完成：%d 张卡(用到 %d 个非默认策略, %d 个vendor策略),耗时 %s",
		len(snaps), len(compiledCache), len(vendorCompiled), time.Since(start))
	return nil
}
