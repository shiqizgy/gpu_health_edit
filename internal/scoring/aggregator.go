package scoring

import "encoding/json"

// 评分聚合器：实现项目文档的两层加权 + 一票否决"后封顶"逻辑。

// CompiledRule 编译后的策略规则（评分时用，避免反复解析 JSON）
type CompiledRule struct {
	MetricKey     string
	Dimension     string
	Weight        float64
	CurveType     string
	CurveParams   string
	IsVeto        bool
	VetoThreshold float64
}

// CompiledStrategy 编译后的策略
type CompiledStrategy struct {
	StrategyID       uint64
	DimensionWeights map[string]float64    // 维度权重
	Rules            map[string]CompiledRule // metricKey -> 规则
}

// MetricBreakdown 单指标明细
type MetricBreakdown struct {
	Key    string  `json:"key"`
	Value  float64 `json:"value"`
	Score  float64 `json:"score"`
	Weight float64 `json:"weight"`
}

// DimensionBreakdown 维度明细
type DimensionBreakdown struct {
	Dimension string            `json:"dimension"`
	Score     float64           `json:"score"`
	Weight    float64           `json:"weight"`
	Metrics   []MetricBreakdown `json:"metrics"`
}

// Result 评分结果
type Result struct {
	Score      float64              `json:"score"`
	Level      string               `json:"level"`
	Veto       bool                 `json:"veto"`
	VetoReason string               `json:"veto_reason"`
	Dimensions []DimensionBreakdown `json:"dimensions"`
}

// Score 核心评分函数。
//   - metrics: 一张卡的指标实测值 map[metricKey]value
//   - strategy: 编译后的策略
//
// 算法（严格对应文档）：
//  1. 单指标过曲线 → 0-100 分
//  2. 维度内加权平均：维度分 = Σ(单指标分×权重) / Σ(权重)
//  3. 维度间加权平均：总分 = Σ(维度分×维度权重)
//  4. 一票否决"后封顶"：先算完总分，若触发否决则 min(总分, 29)
func Score(metrics map[string]float64, strategy *CompiledStrategy) Result {
	// 按维度归集
	type dimAccum struct {
		weightedSum float64
		weightSum   float64
		metrics     []MetricBreakdown
	}
	dims := map[string]*dimAccum{}

	veto := false
	vetoReason := ""

	for key, value := range metrics {
		rule, ok := strategy.Rules[key]
		if !ok {
			continue // 该指标不在策略中，跳过
		}

		// 1. 单指标得分
		mScore := ScoreMetric(rule.CurveType, rule.CurveParams, value)

		// 一票否决检查（阈值型）
		if rule.IsVeto && rule.VetoThreshold > 0 && value >= rule.VetoThreshold {
			veto = true
			vetoReason = key
		}
		// XID 致命码否决
		if key == "DCGM_FI_DEV_XID_ERRORS" && XIDIsFatal(int(value)) {
			veto = true
			vetoReason = "XID_" + itoa(int(value))
		}

		d := dims[rule.Dimension]
		if d == nil {
			d = &dimAccum{}
			dims[rule.Dimension] = d
		}
		d.weightedSum += mScore * rule.Weight
		d.weightSum += rule.Weight
		d.metrics = append(d.metrics, MetricBreakdown{
			Key: key, Value: value, Score: mScore, Weight: rule.Weight,
		})
	}

	// 2 & 3. 维度内 + 维度间加权
	var total, totalDimWeight float64
	result := Result{}
	for dim, acc := range dims {
		dimScore := 100.0
		if acc.weightSum > 0 {
			dimScore = acc.weightedSum / acc.weightSum
		}
		dimWeight := strategy.DimensionWeights[dim]
		total += dimScore * dimWeight
		totalDimWeight += dimWeight

		result.Dimensions = append(result.Dimensions, DimensionBreakdown{
			Dimension: dim, Score: dimScore, Weight: dimWeight, Metrics: acc.metrics,
		})
	}
	if totalDimWeight > 0 {
		result.Score = total / totalDimWeight
	} else {
		result.Score = 100
	}

	// 4. 一票否决后封顶
	if veto && result.Score > 29 {
		result.Score = 29
	}
	result.Veto = veto
	result.VetoReason = vetoReason
	result.Level = LevelFromScore(result.Score, veto)
	return result
}

// LevelFromScore 等级映射（文档规定：30 分以下 = failed）。
func LevelFromScore(score float64, veto bool) string {
	if veto && score > 29 {
		return "failed"
	}
	switch {
	case score >= 90:
		return "healthy"
	case score >= 75:
		return "sub_healthy"
	case score >= 60:
		return "warning"
	case score >= 30:
		return "critical"
	default:
		return "failed"
	}
}

// BreakdownJSON 序列化维度明细，存入快照表。
func BreakdownJSON(r Result) string {
	b, _ := json.Marshal(map[string]any{
		"veto":        r.Veto,
		"veto_reason": r.VetoReason,
		"dimensions":  r.Dimensions,
	})
	return string(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
