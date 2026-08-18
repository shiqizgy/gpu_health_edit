package assistant

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/repository"
)

// ContextProvider 数据采集层接口。
// 目前只有 GPU 单卡实现;未来会加 ClusterContextProvider 等,接口不变。
// 这些实现未来可直接包装成 function calling 的工具。
type ContextProvider interface {
	// Build 根据目标标识(第一版是 GPU UUID)组装上下文文本
	Build(ctx context.Context, target string) (string, error)
}

// GPUContextProvider GPU 单卡数据采集
type GPUContextProvider struct {
	topo   *repository.TopologyRepo
	health *repository.HealthRepo
	metric *repository.MetricRepo
	fault  *repository.FaultRepo
	ck     *ckclient.Client
	table  string
}

func NewGPUContextProvider(
	topo *repository.TopologyRepo,
	health *repository.HealthRepo,
	metric *repository.MetricRepo,
	fault *repository.FaultRepo,
	ck *ckclient.Client,
	table string,
) *GPUContextProvider {
	return &GPUContextProvider{topo: topo, health: health, metric: metric, fault: fault, ck: ck, table: table}
}

// breakdown JSON 的解析结构(对应 scoring.BreakdownJSON 的输出)
type breakdownData struct {
	Veto       bool   `json:"veto"`
	VetoReason string `json:"veto_reason"`
	Dimensions []struct {
		Dimension string  `json:"dimension"`
		Score     float64 `json:"score"`
		Weight    float64 `json:"weight"`
	} `json:"dimensions"`
}

// Build 组装单卡上下文。查 5 处数据拼成结构化文本。
func (p *GPUContextProvider) Build(ctx context.Context, uuid string) (string, error) {
	var sb strings.Builder

	// ---- 1. 基本信息(从快照拿 cluster_id,卡详情可选) ----
	snap, err := p.health.GetSnapshot(uuid)
	if err != nil {
		return "", fmt.Errorf("未找到该 GPU 的评分数据(UUID 可能不存在或尚未评分): %s", uuid)
	}

	sb.WriteString("【GPU 基本信息】\n")
	sb.WriteString(fmt.Sprintf("UUID: %s | 所属集群ID: %d\n\n", uuid, snap.ClusterID))

	// ---- 2. 健康评分 ----
	sb.WriteString("【健康评分】\n")
	sb.WriteString(fmt.Sprintf("总分: %.1f | 等级: %s | 评分策略ID: %d\n",
		snap.Score, levelCN(snap.Level), snap.StrategyID))
	if snap.Veto {
		sb.WriteString(fmt.Sprintf("⚠️ 触发一票否决! 原因: %s (这是最严重的情况)\n", snap.VetoReason))
	}
	sb.WriteString("\n")

	// ---- 3. 维度明细(从 breakdown JSON 解析) ----
	if snap.Breakdown != "" {
		var bd breakdownData
		if json.Unmarshal([]byte(snap.Breakdown), &bd) == nil && len(bd.Dimensions) > 0 {
			sb.WriteString("【各维度得分】\n")
			for _, d := range bd.Dimensions {
				sb.WriteString(fmt.Sprintf("%s: %.1f (权重 %.2f)\n",
					dimensionCN(d.Dimension), d.Score, d.Weight))
			}
			sb.WriteString("\n")
		}
	}

	// ---- 4. 实时指标(带正常范围标注) ----
	var liveMetrics map[string]float64
	if g, err := p.topo.GetGPUByUUID(uuid); err == nil {
		liveMetrics, _ = p.ck.LatestByGPU(ctx, p.table, g.SN, strconv.Itoa(g.GPUIndex), 5*time.Minute)
	}
	if len(liveMetrics) > 0 {
		// 取指标定义,用于标注正常范围
		defs, _ := p.metric.ListHealthKeys()
		defMap := map[string]string{} // key -> "单位|正常下限|正常上限"
		nameMap := map[string]string{}
		for _, d := range defs {
			lo, hi := "", ""
			if d.LowerBound != nil {
				lo = fmt.Sprintf("%g", *d.LowerBound)
			}
			if d.UpperBond != nil {
				hi = fmt.Sprintf("%g", *d.UpperBond)
			}
			defMap[d.MetricName] = fmt.Sprintf("%s|%s|%s", d.Unit, lo, hi)
			nameMap[d.MetricName] = d.Conception
		}

		sb.WriteString("【当前实时指标】\n")
		var xidValue float64 = -1
		for key, val := range liveMetrics {
			name := nameMap[key]
			if name == "" {
				name = key
			}
			meta := defMap[key]
			rangeInfo := ""
			if meta != "" {
				parts := strings.SplitN(meta, "|", 3)
				if len(parts) == 3 {
					unit, normal, abnormal := parts[0], parts[1], parts[2]
					rangeInfo = fmt.Sprintf(" [%s, 正常:%s 异常:%s]", unit, normal, abnormal)
				}
			}
			sb.WriteString(fmt.Sprintf("%s: %g%s\n", name, val, rangeInfo))
			if key == "DCGM_FI_DEV_XID_ERRORS" {
				xidValue = val
			}
		}
		sb.WriteString("\n")

		// ---- 5. 匹配故障知识(根据 XID 等) ----
		p.appendFaultKnowledge(&sb, xidValue)
	} else {
		sb.WriteString("【当前实时指标】\n暂无实时指标数据(该卡可能刚上线或仿真未覆盖)。\n\n")
	}

	return sb.String(), nil
}

// appendFaultKnowledge 根据 XID 匹配故障知识追加到上下文
func (p *GPUContextProvider) appendFaultKnowledge(sb *strings.Builder, xidValue float64) {
	if xidValue <= 0 {
		return
	}
	xidStr := fmt.Sprintf("%g", xidValue)
	// 按 XID 关键词搜故障知识库
	list, _, err := p.fault.List("", xidStr, 5, 0)
	if err != nil || len(list) == 0 {
		return
	}
	sb.WriteString("【匹配的故障知识(供参考)】\n")
	for _, f := range list {
		sb.WriteString(fmt.Sprintf("- %s (XID %s, %s): %s。处置建议: %s\n",
			f.FaultType, f.XIDCode, severityCN(f.Severity), f.Symptom, f.Suggestion))
	}
	sb.WriteString("\n")
}

// ---- 中文化辅助 ----
func levelCN(l string) string {
	m := map[string]string{"healthy": "健康", "sub_healthy": "亚健康",
		"warning": "警告", "critical": "严重", "failed": "故障"}
	if v, ok := m[l]; ok {
		return v + "(" + l + ")"
	}
	return l
}

func dimensionCN(d string) string {
	m := map[string]string{"hardware": "硬件健康", "stability": "运行稳定性",
		"performance": "性能表现", "environment": "运行环境"}
	if v, ok := m[d]; ok {
		return v
	}
	return d
}

func severityCN(s string) string {
	m := map[string]string{"warning": "警告", "critical": "严重", "fatal": "致命"}
	if v, ok := m[s]; ok {
		return v
	}
	return s
}
