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
//   Redis(各卡最新指标) → 按策略评分 → 批量写单卡快照 → 重算集群汇总
type ScorerService struct {
	redis    *redisclient.Client
	health   *repository.HealthRepo
	topo     *repository.TopologyRepo
	strategy *StrategyService
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

// RunOnce 执行一轮评分。
func (s *ScorerService) RunOnce(ctx context.Context) error {
	start := time.Now()

	// 1. 取当前策略（带热加载）
	compiled, err := s.strategy.GetCompiled(s.strategyCode)
	if err != nil {
		logger.L.Errorf("加载策略失败: %v", err)
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

	// 3. 建立 uuid → cluster_id 映射（评分快照需要 cluster_id 做聚合）
	gpus, err := s.topo.AllOnlineGPUs()
	if err != nil {
		return err
	}
	clusterOf := make(map[string]uint64, len(gpus))
	for _, g := range gpus {
		clusterOf[g.UUID] = g.ClusterID
	}

	// 4. 逐卡评分
	now := time.Now()
	snaps := make([]model.GPUHealthSnapshot, 0, len(frames))
	for _, f := range frames {
		result := scoring.Score(f.Metrics, compiled)
		snaps = append(snaps, model.GPUHealthSnapshot{
			GPUUUID:    f.UUID,
			ClusterID:  clusterOf[f.UUID],
			StrategyID: compiled.StrategyID,
			Score:      result.Score,
			Level:      result.Level,
			Veto:       result.Veto,
			VetoReason: result.VetoReason,
			Breakdown:  scoring.BreakdownJSON(result),
			ScoredAt:   now,
		})
	}

	// 5. 批量写单卡快照
	if err := s.health.BatchUpsertSnapshots(snaps); err != nil {
		logger.L.Errorf("写快照失败: %v", err)
		return err
	}

	// 6. 重算集群汇总（预聚合）
	if err := s.health.RecomputeClusterSummaries(); err != nil {
		logger.L.Errorf("重算集群汇总失败: %v", err)
		return err
	}

	logger.L.Infof("评分完成：%d 张卡，耗时 %s", len(snaps), time.Since(start))
	return nil
}
