package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/pkg/logger"
)

// confirmCycles 阈值类故障需连续命中这么多轮才真正入池（去抖，避免单次尖刺刷故障）。
// 一票否决(fatal)不去抖，立即入池。
const confirmCycles = 2

// FaultDetectService 评分链路里的故障检测层：
// 每轮拿到各卡的评分结果，对照故障规则/指标门限产生故障信号，
// 经去抖后写入故障池(fault_event)，并对账开始/持续/恢复。
type FaultDetectService struct {
	events     *repository.FaultEventRepo
	rules      *repository.FaultRuleRepo
	metricRepo *repository.MetricRepo
	topo       *repository.TopologyRepo

	// 去抖状态：dedupKey -> 连续命中轮数（仅阈值类用）
	breach map[string]int
}

func NewFaultDetectService(
	events *repository.FaultEventRepo,
	rules *repository.FaultRuleRepo,
	metricRepo *repository.MetricRepo,
	topo *repository.TopologyRepo,
) *FaultDetectService {
	return &FaultDetectService{
		events: events, rules: rules, metricRepo: metricRepo, topo: topo,
		breach: map[string]int{},
	}
}

// Process 处理一轮：cards 为本轮评估过的每张卡的评分结果。
func (s *FaultDetectService) Process(_ context.Context, now time.Time, cards map[string]scoring.CardScore) error {
	if len(cards) == 0 {
		return nil
	}

	// 一次性加载规则、指标定义、拓扑名称，避免逐卡查库
	rules, err := s.rules.ActiveRules()
	if err != nil {
		return err
	}
	defs, err := s.metricRepo.AllDefsMap()
	if err != nil {
		return err
	}
	meta, err := s.topo.GPUMetaMap()
	if err != nil {
		return err
	}

	current := make(map[string][]model.FaultEvent, len(cards))
	// 本轮命中的 dedupKey 集合（用于清理去抖计数）
	hitNow := map[string]bool{}

	for uuid, cs := range cards {
		signals := scoring.Detect(cs.Metrics, cs.Result, rules, defs)
		m := meta[uuid] // 可能为空(理论上在线卡都有)

		for _, sig := range signals {
			dedup := uuid + "|" + sig.Signature
			hitNow[dedup] = true

			// 去抖：仅对非 fatal 的阈值类做"连续 N 轮"确认
			if sig.Severity != "fatal" {
				s.breach[dedup]++
				if s.breach[dedup] < confirmCycles {
					continue // 还没达到确认轮数，本轮先不入池
				}
			}

			current[uuid] = append(current[uuid], model.FaultEvent{
				DedupKey:      dedup,
				FaultName:     sig.Name,
				Severity:      sig.Severity,
				ClusterID:     m.ClusterID,
				ClusterName:   m.ClusterName,
				NodeHost:      m.NodeHost,
				GPUUUID:       uuid,
				MetricKey:     sig.MetricKey,
				MetricDisplay: sig.MetricDisp,
				TriggerValue:  sig.Value,
				Threshold:     sig.Threshold,
				Detail:        buildDetail(sig, cs.Result.Score),
			})
		}
	}

	// 清理：本轮没命中的去抖计数清零，避免"间歇命中"累积成误报
	for k := range s.breach {
		if !hitNow[k] {
			delete(s.breach, k)
		}
	}

	if err := s.events.Reconcile(now, current); err != nil {
		return err
	}
	logger.L.Infof("故障检测完成：本轮 %d 张卡，确认故障事件 %d 类",
		len(cards), countEvents(current))
	return nil
}

func buildDetail(sig scoring.FaultSignal, score float64) string {
	b, _ := json.Marshal(map[string]any{
		"signature":    sig.Signature,
		"metric_key":   sig.MetricKey,
		"value":        sig.Value,
		"threshold":    sig.Threshold,
		"severity":     sig.Severity,
		"card_score":   score,
		"knowledge_id": sig.KnowledgeID,
	})
	return string(b)
}

func countEvents(m map[string][]model.FaultEvent) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}
