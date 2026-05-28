package service

import (
	"context"
	"time"

	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/pkg/logger"
)

// ScorerService 评分服务核心逻辑（每分钟执行一次）。
//
// 完整数据流：
//
//	Redis(各卡最新指标) → 按策略评分 → 批量写单卡快照 → 重算集群汇总
type ScorerService struct {
	redis        *redisclient.Client
	health       *repository.HealthRepo
	topo         *repository.TopologyRepo
	strategy     *StrategyService
	strategyCode string
}

func NewScorerService(
	rc *redisclient.Client,
	health *repository.HealthRepo,
	topo *repository.TopologyRepo,
	strategy *StrategyService,
	strategyCode string,
) *ScorerService {
	return &ScorerService{
		redis: rc, health: health, topo: topo,
		strategy: strategy, strategyCode: strategyCode,
	}
}

// RunOnce 执行一轮评分。（多策略，优先顺序：卡>集群>默认 解析）
func (s *ScorerService) RunOnce(ctx context.Context) error {
	start := time.Now()

	// 1. 取默认策略(兜底)
	defaultStrategy, err := s.strategy.GetCompiled(s.strategyCode)
	if err != nil {
		logger.L.Errorf("加载默认策略失败: %v", err)
		return err
	}

	// 2. 从 Redis 读所有卡的最新指标
	frames, err := s.redis.ReadAllFrames(ctx)
	if err != nil {
		logger.L.Errorf("读取 Redis 指标失败: %v", err)
		return err
	}
	if len(frames) == 0 {
		logger.L.Warn("Redis 中无指标数据，跳过本轮评分")
		return nil
	}

	// 3. 查所有在线 GPU,带卡级和集群级 strategy_id
	gpus, err := s.topo.AllOnlineGPUsWithStrategy()
	if err != nil {
		return err
	}
	// 建索引: uuid → 解析出的 strategyID(nil 表示用默认)
	type binding struct {
		clusterID  uint64
		strategyID *uint64
	}
	bindOf := make(map[string]binding, len(gpus))
	for _, g := range gpus {
		// 优先级: 卡级 > 集群级 > 默认(nil)
		var sid *uint64
		if g.CardStrategyID != nil {
			sid = g.CardStrategyID
		} else if g.ClusterStrategyID != nil {
			sid = g.ClusterStrategyID
		}
		bindOf[g.UUID] = binding{clusterID: g.ClusterID, strategyID: sid}
	}

	// 4. 预编译所有用到的非默认策略(按 ID 缓存,避免每卡查库)
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

	// 5. 逐卡评分(各用各的策略)
	now := time.Now()
	snaps := make([]model.GPUHealthSnapshot, 0, len(frames))
	for _, f := range frames {
		b, known := bindOf[f.UUID]
		// 选策略: 绑定的(且编译成功) → 否则默认
		compiled := defaultStrategy
		if known && b.strategyID != nil {
			if cs, ok := compiledCache[*b.strategyID]; ok {
				compiled = cs
			}
		}
		clusterID := uint64(0)
		if known {
			clusterID = b.clusterID
		}

		result := scoring.Score(f.Metrics, compiled)
		snaps = append(snaps, model.GPUHealthSnapshot{
			GPUUUID:    f.UUID,
			ClusterID:  clusterID,
			StrategyID: compiled.StrategyID, // 记录实际用的策略,前端可展示
			Score:      result.Score,
			Level:      result.Level,
			Veto:       result.Veto,
			VetoReason: result.VetoReason,
			Breakdown:  scoring.BreakdownJSON(result),
			ScoredAt:   now,
		})
	}

	// 6. 批量写快照
	if err := s.health.BatchUpsertSnapshots(snaps); err != nil {
		logger.L.Errorf("写快照失败: %v", err)
		return err
	}

	// 7. 重算集群汇总
	if err := s.health.RecomputeClusterSummaries(); err != nil {
		logger.L.Errorf("重算集群汇总失败: %v", err)
		return err
	}

	logger.L.Infof("评分完成：%d 张卡(用到 %d 个非默认策略),耗时 %s",
		len(snaps), len(compiledCache), time.Since(start))
	return nil
}
