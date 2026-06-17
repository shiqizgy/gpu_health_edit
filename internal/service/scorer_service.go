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
	faultDetect  *FaultDetectService // 可为 nil（不启用故障池时）
	pool         *pool.Pool
}

func NewScorerService(
	rc *redisclient.Client,
	health *repository.HealthRepo,
	topo *repository.TopologyRepo,
	strategy *StrategyService,
	strategyCode string,
	faultDetect *FaultDetectService,
	p *pool.Pool,
) *ScorerService {
	return &ScorerService{
		redis:        rc,
		health:       health,
		topo:         topo,
		strategy:     strategy,
		strategyCode: strategyCode,
		faultDetect:  faultDetect,
		pool:         p,
	}
}

// RunOnce 执行一轮评分。（多策略，优先顺序：卡>集群>默认 解析）
func (s *ScorerService) RunOnceWith(ctx context.Context, frames []redisclient.MetricFrame) error {
	start := time.Now()

	// 1. 取默认策略(兜底)
	defaultStrategy, err := s.strategy.GetCompiled(s.strategyCode)
	if err != nil {
		logger.L.Errorf("加载默认策略失败: %v", err)
		return err
	}

	// 2. 入参即本轮各卡最新指标（由调用方提供：内存直传或 Redis）
	if len(frames) == 0 {
		logger.L.Warn("本轮无指标数据，跳过评分")
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

	// 5. 逐卡评分（并行：bindOf/compiledCache/defaultStrategy 均为只读，按下标写互不重叠，安全）
	now := time.Now()
	snaps := make([]model.GPUHealthSnapshot, len(frames)) //预分配，按下标写
	entries := make([]scoring.CardScore, len(frames))     //故障检测用，先按下标存切片
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

	// 并行段结束后单线程合成 map（给故障检测用），O(n) 可忽略
	cardScores := make(map[string]scoring.CardScore, len(frames))
	for i, f := range frames {
		cardScores[f.UUID] = entries[i]
	}

	// 6. 批量写快照
	if err := s.health.BatchUpsertSnapshotsConcurrent(snaps, 8); err != nil {
		logger.L.Errorf("写快照失败: %v", err)
		return err
	}

	// 7. 重算集群汇总
	if err := s.health.RecomputeClusterSummaries(); err != nil {
		logger.L.Errorf("重算集群汇总失败: %v", err)
		return err
	}

	// 8. 故障检测 + 故障池对账（启用时）
	if s.faultDetect != nil {
		if err := s.faultDetect.Process(ctx, now, cardScores); err != nil {
			logger.L.Errorf("故障检测失败: %v", err)
			// 不阻断主流程：评分已落库，故障池失败仅记日志
		}
	}

	logger.L.Infof("评分完成：%d 张卡(用到 %d 个非默认策略),耗时 %s",
		len(snaps), len(compiledCache), time.Since(start))
	return nil
}

// RunOnce 从 Redis 读取指标后评分（供独立 cmd/scorer、拆分部署使用）。
func (s *ScorerService) RunOnce(ctx context.Context) error {
	frames, err := s.redis.ReadAllFrames(ctx)
	if err != nil {
		logger.L.Errorf("读取 Redis 指标失败: %v", err)
		return err
	}
	return s.RunOnceWith(ctx, frames)
}
