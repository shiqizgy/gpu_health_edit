package scoring

import "encoding/json"

// 一票否决分数：设定为一票否决的指标，所在的维度分和卡的分数都设定为最低分
const VetoFloor = 20.0

// XID指标key（离散错误码，单独走查表评分+致命XID码否决）
const xidMetricKey = "DCGM_FI_DEV_XID_ERRORS"

// 评分聚合器：实现两层加权 + 一票否决"后封顶"逻辑。
// CompiledRule 编译后的策略规则（评分时用，避免反复解析定义）
type CompiledRule struct {
	MetricKey     string
	Dimension     string
	Weight        float64
	Bounds        MetricBounds //MetricDefinition 的评分边界（按类型三区间/布尔/速率
	IsVeto        bool
	VetoThreshold float64
}

// CompiledStrategy 编译后的策略
type CompiledStrategy struct {
	StrategyID       uint64
	DimensionWeights map[string]float64      // 维度权重
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
	Coverage   float64              `json:"coverage"` //本次覆盖的维度权重占比0～1
	Dimensions []DimensionBreakdown `json:"dimensions"`
}

// dimAccum 单个维度的累加器：维度内加权求和 + 明细收集。
type dimAccum struct {
	weightedSum float64
	weightSum   float64
	metrics     []MetricBreakdown
}

// vetoState 一票否决的中间状态。
type vetoState struct {
	hit    bool
	reason string
	dims   map[string]bool // 命中否决的维度集合
}

// Score 核心评分函数。
//   - metrics: 一张卡的指标实测值 map[metricKey]value
//   - strategy: 编译后的策略
//
// 算法：
//  1. 单指标按 value_type 评分 → 0-100 分（XID 走查表）
//  2. 维度内加权平均：维度分 = Σ(单指标分×权重) / Σ(权重)
//  3. 维度间加权平均：总分 = Σ(维度分×维度权重)
//  4. 一票否决"后封顶"：命中否决的维度分与整卡总分都锁到 VetoFloor

const MinCoverage = 0 // 覆盖率低于此值不给出可信分数
func Score(metrics map[string]float64, strategy *CompiledStrategy) Result {
	dims, veto := accumulate(metrics, strategy)

	result := Result{}
	result.Dimensions, result.Score, result.Coverage = aggregateDimensions(dims, strategy, veto)

	// 一票否决把整卡总分锁到最低
	if veto.hit && result.Score > VetoFloor {
		result.Score = VetoFloor
	}
	result.Veto = veto.hit
	result.VetoReason = veto.reason
	// 覆盖率不足：标记为 unknown，不给虚高分数
	if result.Coverage < MinCoverage && !veto.hit {
		result.Level = "unknown"
		return result
	}
	result.Level = LevelFromScore(result.Score, veto.hit)
	return result
}

// accumulate 遍历指标：逐个评分、判否决，并按维度归集。
func accumulate(metrics map[string]float64, strategy *CompiledStrategy) (map[string]*dimAccum, vetoState) {
	dims := map[string]*dimAccum{}
	veto := vetoState{dims: map[string]bool{}}

	for key, value := range metrics {
		rule, ok := strategy.Rules[key]
		if !ok {
			continue // 该指标不在策略中，跳过
		}
		if !validValue(key, value) {
			continue // 无效采样保护
		}

		mScore := scoreOne(key, rule, value)
		if reason, hit := checkVeto(key, rule, value, mScore); hit {
			veto.hit = true
			veto.reason = reason
			veto.dims[rule.Dimension] = true
		}

		addToDim(dims, rule, value, mScore)
	}
	return dims, veto
}

// validValue 无效采样保护：负值一般为无效采样，但 XID 等枚举的 -1 需交由各自逻辑判定。
func validValue(key string, value float64) bool {
	if value < 0 && key != xidMetricKey {
		return false
	}
	return true
}

// scoreOne 单指标评分：XID 走查表，其余按 value_type 分派。
func scoreOne(key string, rule CompiledRule, value float64) float64 {
	if key == xidMetricKey {
		return xidScore(int(value))
	}
	return ScoreByType(rule.Bounds, value)
}

// checkVeto 一票否决判定，返回(否决原因, 是否命中)。
// 优先级：XID 致命码 > 显式阈值(veto_threshold>0) > 该指标本身落入故障档
func checkVeto(key string, rule CompiledRule, value, mScore float64) (string, bool) {
	if key == xidMetricKey {
		// XID：仅命中"致命码"集合才否决
		if XIDIsFatal(int(value)) {
			return "XID_" + itoa(int(value)), true
		}
		return "", false
	}
	if !rule.IsVeto {
		return "", false
	}
	if rule.VetoThreshold > 0 && value >= rule.VetoThreshold {
		return key, true
	}
	// ★ 核心改动：否决指标只要被评为故障档(20分)，即触发否决
	if mScore <= ScoreCritical {
		return key, true
	}
	return "", false
}

// addToDim 把单指标结果累加进所属维度。
func addToDim(dims map[string]*dimAccum, rule CompiledRule, value, mScore float64) {
	d := dims[rule.Dimension]
	if d == nil {
		d = &dimAccum{}
		dims[rule.Dimension] = d
	}
	d.weightedSum += mScore * rule.Weight
	d.weightSum += rule.Weight
	d.metrics = append(d.metrics, MetricBreakdown{
		Key: rule.MetricKey, Value: value, Score: mScore, Weight: rule.Weight,
	})
}

// aggregateDimensions 维度内求分 + 维度间加权，返回维度明细与整卡总分。
func aggregateDimensions(dims map[string]*dimAccum, strategy *CompiledStrategy, veto vetoState) ([]DimensionBreakdown, float64, float64) {
	var total, totalDimWeight float64
	breakdowns := make([]DimensionBreakdown, 0, len(dims))

	for dim, acc := range dims {
		dimScore := dimensionScore(acc, veto.dims[dim])
		dimWeight := strategy.DimensionWeights[dim]
		total += dimScore * dimWeight
		totalDimWeight += dimWeight

		breakdowns = append(breakdowns, DimensionBreakdown{
			Dimension: dim, Score: dimScore, Weight: dimWeight, Metrics: acc.metrics,
		})
	}

	// 分母用策略声明的全部维度权重，算出覆盖率
	var declaredWeight float64
	for _, w := range strategy.DimensionWeights {
		declaredWeight += w
	}
	coverage := 0.0
	if declaredWeight > 0 {
		coverage = totalDimWeight / declaredWeight
	}

	if totalDimWeight <= 0 {
		return breakdowns, 0, 0 // 从 100 改成 0：一个维度都没算出来，不能当健康
	}
	return breakdowns, total / totalDimWeight, coverage
}

// dimensionScore 单维度得分：维度内加权平均；命中否决则锁到 VetoFloor。
func dimensionScore(acc *dimAccum, vetoed bool) float64 {
	dimScore := 100.0
	if acc.weightSum > 0 {
		dimScore = acc.weightedSum / acc.weightSum
	}
	if vetoed && dimScore > VetoFloor {
		dimScore = VetoFloor
	}
	return dimScore
}

// LevelFromScore 等级映射（文档规定：20 分以下 = failed）。
func LevelFromScore(score float64, veto bool) string {
	if veto && score <= VetoFloor {
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
		"coverage":    r.Coverage,
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

// ---- XID 错误码分级（Ordinal 枚举，离散错误码，不走连续区间）----

// 致命 XID 集合：命中即一票否决（ECC/行重映射/掉总线/内部硬件与总线错误等）。
var xidFatalSet = map[int]bool{
	48: true, 61: true, 62: true, 63: true, 64: true, 69: true, 74: true, 79: true,
	94: true, 95: true, 110: true, 119: true, 120: true, 123: true,
	140: true, 143: true, 154: true, 167: true, 169: true, 171: true, 172: true,
}

// 告警 XID 集合：判警告分，不否决。
var xidWarnSet = map[int]bool{
	13: true, 31: true, 32: true, 43: true, 44: true, 45: true,
	61: true, 68: true, 78: true, 80: true, 92: true, 93: true,
	109: true, 137: true,
}

var xidAppSideSet = map[int]bool{}

// xidScore：0→健康；warn 集→警告；fatal 集→故障（并在聚合层触发否决）；其他→警告。
func xidScore(code int) float64 {
	if code == 0 || xidAppSideSet[code] {
		return ScoreHealthy
	}
	if xidFatalSet[code] {
		return ScoreCritical
	}
	if xidWarnSet[code] {
		return ScoreWarning
	}
	return ScoreWarning
}

// XIDIsFatal 供聚合层判断 XID 是否致命（一票否决）。
func XIDIsFatal(code int) bool { return xidFatalSet[code] }
