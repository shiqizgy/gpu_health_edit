package scoring

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/gpu-health/platform/internal/model"
)

// CardScore 评分服务每张卡算完后交给故障检测的输入。
type CardScore struct {
	Metrics map[string]float64
	Result  Result
}

// FaultSignal 一张卡本轮命中的一个故障信号（尚未落库，无拓扑信息）。
type FaultSignal struct {
	Signature   string // 去重用：rule:ID / veto:reason / thr:metricKey
	Name        string
	Severity    string
	MetricKey   string
	MetricDisp  string
	Value       float64
	Threshold   float64
	KnowledgeID *uint64
}

// Detect 对照规则 + 指标门限，返回这张卡本轮命中的所有故障信号。
//   - metrics: 实测值；res: 评分结果(含 veto)；rules: 启用的故障规则；defs: 指标定义 map
func Detect(metrics map[string]float64, res Result,
	rules []model.FaultRule, defs map[string]model.MetricDefinition) []FaultSignal {

	var out []FaultSignal

	// 1) 一票否决：直接由评分结果生成（不依赖规则行），立即上报
	if res.Veto {
		reason := res.VetoReason
		if reason == "" {
			reason = "veto"
		}
		out = append(out, FaultSignal{
			Signature: "veto:" + reason,
			Name:      "一票否决 (" + reason + ")",
			Severity:  "fatal",
		})
	}

	// 2) 阈值类规则
	for _, r := range rules {
		if !r.Enabled || r.TriggerType != "threshold" || r.MetricKey == "" {
			continue
		}
		v, ok := metrics[r.MetricKey]
		if !ok {
			continue
		}
		def := defs[r.MetricKey]

		breach, thr := evalThreshold(def, r, v)
		if !breach {
			continue
		}
		disp := def.DisplayName
		if disp == "" {
			disp = r.MetricKey
		}
		out = append(out, FaultSignal{
			Signature:   "rule:" + strconv.FormatUint(r.ID, 10),
			Name:        r.Name,
			Severity:    r.Severity,
			MetricKey:   r.MetricKey,
			MetricDisp:  disp,
			Value:       v,
			Threshold:   thr,
			KnowledgeID: r.KnowledgeID,
		})
	}
	return out
}

// evalThreshold 判断某指标值是否越界，返回(是否越界, 实际使用的门限)。
func evalThreshold(def model.MetricDefinition, r model.FaultRule, v float64) (bool, float64) {
	// range 型：落在 [normal_min, normal_max] 外即异常
	if def.Direction == "range" && r.Threshold == nil {
		if def.NormalMin != nil && v < *def.NormalMin {
			return true, *def.NormalMin
		}
		if def.NormalMax != nil && v > *def.NormalMax {
			return true, *def.NormalMax
		}
		return false, 0
	}

	// 取门限：规则自带优先，否则用指标的 crit
	var thr float64
	switch {
	case r.Threshold != nil:
		thr = *r.Threshold
	case def.CritThreshold != nil:
		thr = *def.CritThreshold
	default:
		return false, 0 // 没有任何门限可比，不触发
	}

	op := r.Operator
	if op == "" {
		if def.Direction == "low_bad" {
			op = "<="
		} else {
			op = ">=" // high_bad 默认
		}
	}
	switch op {
	case ">=":
		return v >= thr, thr
	case ">":
		return v > thr, thr
	case "<=":
		return v <= thr, thr
	case "<":
		return v < thr, thr
	case "==":
		return v == thr, thr
	}
	return false, thr
}

// ---- 需求(1)：从快照 breakdown 中提取"不正常的指标" ----

// AbnormalMetric 单卡异常指标（前端详情面板用）
type AbnormalMetric struct {
	MetricKey     string   `json:"metric_key"`
	Display       string   `json:"display_name"`
	Dimension     string   `json:"dimension"`
	Value         float64  `json:"value"`
	Unit          string   `json:"unit"`
	Score         float64  `json:"score"`
	Severity      string   `json:"severity"` // warning/critical
	WarnThreshold *float64 `json:"warn_threshold"`
	CritThreshold *float64 `json:"crit_threshold"`
	NormalRange   string   `json:"normal_range"`
}

// breakdown 反序列化结构（对应 BreakdownJSON 的输出）
type bdMetric struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	Score float64 `json:"score"`
}
type bdDim struct {
	Dimension string     `json:"dimension"`
	Metrics   []bdMetric `json:"metrics"`
}
type bdRoot struct {
	Dimensions []bdDim `json:"dimensions"`
}

// PickAbnormal 解析 breakdown，挑出单指标得分 < 60 的指标，并用指标定义补全展示信息。
// 按得分升序（最差的在前）。
func PickAbnormal(breakdownJSON string, defs map[string]model.MetricDefinition) []AbnormalMetric {
	var root bdRoot
	if breakdownJSON == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(breakdownJSON), &root); err != nil {
		return nil
	}
	var out []AbnormalMetric
	for _, d := range root.Dimensions {
		for _, m := range d.Metrics {
			if m.Score >= 60 {
				continue
			}
			sev := "warning"
			if m.Score < 30 {
				sev = "critical"
			}
			def := defs[m.Key]
			disp := def.DisplayName
			if disp == "" {
				disp = m.Key
			}
			out = append(out, AbnormalMetric{
				MetricKey:     m.Key,
				Display:       disp,
				Dimension:     d.Dimension,
				Value:         m.Value,
				Unit:          def.Unit,
				Score:         m.Score,
				Severity:      sev,
				WarnThreshold: def.WarnThreshold,
				CritThreshold: def.CritThreshold,
				NormalRange:   def.NormalRange,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score < out[j].Score })
	return out
}
