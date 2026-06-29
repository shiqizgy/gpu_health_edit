package service

import (
	"context"
	"time"

	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/pool"
)

type ScorerService struct {
	redis          *redisclient.Client
	health         *repository.HealthRepo
	topo           *repository.TopologyRepo
	strategy       *StrategyService
	strategyCode   string
	vendorStrategy map[string]string // vendor -> strategy_code
	faultDetect    *FaultDetectService
	pool           *pool.Pool
}

func NewScorerService(
	rc *redisclient.Client,
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
		redis:          rc,
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
func (s *ScorerService) RunOnceWith(ctx context.Context, frames []redisclient.MetricFrame) error {
	start := time.Now()

	defaultStrategy, err := s.strategy.GetCompiled(s.strategyCode)
	if err != nil {
		logger.L.Errorf("加载默认策略失败: %v", err)
		return err
	}

	if len(frames) == 0 {
		logger.L.Warn("本轮无指标数据，跳过评分")
		return nil
	}

	gpus, err := s.topo.AllOnlineGPUsWithStrategy()
	if err != nil {
		return err
	}

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

	// 预编译所有用到的非默认策略（卡级/集群级绑定）
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

	// 预编译 vendor 级策略
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
					// vendor 级策略（卡级和集群级都没绑定时生效）
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
				Breakdown:  "null",
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

func (s *ScorerService) RunOnce(ctx context.Context) error {
	frames, err := s.redis.ReadAllFrames(ctx)
	if err != nil {
		logger.L.Errorf("读取 Redis 指标失败: %v", err)
		return err
	}
	return s.RunOnceWith(ctx, frames)
}
