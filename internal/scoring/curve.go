package scoring

import "strings"

// 单指标评分：按 value_type 分派到不同评分方法，统一输出 0-100。
// 三档固定分：健康 100 / 警告 60 / 故障 20。

// 指标类型（对应metric_definition.value_type，含中文后缀，用前缀匹配）：
//   Gauge连续数值、Gauge_Rate比率                 → 连续数值，四边界三区间
//   Counter累计计数、Counter_Duration累计时长、Level_Count水位计数      → 增长速率（数据层转增量），定义正常/告警速率，分三个区间
//   Bool布尔             → 命中正常值→100，命中异常值→20
//   Ordinal枚举          → XID/降频掩码等，查表（默认走连续数值兜底或满分）
//   Other其他            → 版本/字符串类，不参与数值评分

const (
	ScoreHealthy  = 100.0 // 健康区
	ScoreWarning  = 60.0  // 警告区
	ScoreCritical = 20.0  // 故障区
)

// MetricBounds 单指标评分所需的全部边界（编译策略时从 MetricDefinition 填充）。
type MetricBounds struct {
	ValueType int // 原始 value_type（含中文），用前缀判定

	// 连续数值 / 比率：四边界
	UpperBond    *float64 // 正常上限
	LowerBound   *float64 // 正常下限
	WarnupBound  *float64 // 警告上界（超此进故障区）
	WarnlowBound *float64 // 警告下界（低于此进故障区）

	// 速率型：正常速率上限 / 告警速率上限
	NormalRate *float64
	WarnRate   *float64

	// 布尔：正常值 / 异常值（字符串，兼容 "0"/"1"/"OK" 等）
	BoolNormal   string
	BoolAbnormal string
}

// 指标数值类型码（对应 metric_ordinal.md）
const (
	VTGauge     = 1 // Gauge连续数值
	VTGaugeRate = 2 // Gauge_Rate比率
	VTCounter   = 3 // Counter累计计数
	VTDuration  = 4 // Counter_Duration累计时长
	VTLevel     = 5 // Level_Count水位计数
	VTBool      = 6 // Bool布尔
	VTOrdinal   = 7 // Ordinal枚举
	VTOther     = 8 // Other其他
)

// ScoreByType 单指标评分统一入口（按 value_type 数值码做计算分支）。
func ScoreByType(b MetricBounds, value float64) float64 {
	switch b.ValueType {
	case VTGauge, VTGaugeRate:
		return scoreRange(b, value) //todo 调整算法
	case VTCounter, VTDuration, VTLevel:
		return scoreRate(b.NormalRate, b.WarnRate, value)
	case VTBool:
		return scoreBool(b.BoolNormal, b.BoolAbnormal, value)
	case VTOrdinal, VTOther:
		return ScoreHealthy // 枚举/其他：交由专用逻辑(XID查表)或不扣分
	default: // VTGauge 及未知
		return scoreRange(b, value)
	}
}

// scoreRange 连续数值 / 比率类：四个边界三个区间。
//
//	正常：[LowerBound, UpperBond]                                  → 100
//	警告：[WarnlowBound, LowerBound) 和 (UpperBond, WarnupBound]    → 60
//	故障：[下限, WarnlowBound) 和 (WarnupBound, 上限]               → 20
func scoreRange(b MetricBounds, v float64) float64 {
	if b.UpperBond != nil && v > *b.UpperBond {
		if b.WarnupBound != nil && v > *b.WarnupBound {
			return ScoreCritical
		}
		return ScoreWarning
	}
	if b.LowerBound != nil && v < *b.LowerBound {
		if b.WarnlowBound != nil && v < *b.WarnlowBound {
			return ScoreCritical
		}
		return ScoreWarning
	}
	return ScoreHealthy
}

// scoreRate 累计类增长速率(数据层已转采样窗口增量)：三档。
func scoreRate(normalRate, warnRate *float64, v float64) float64 {
	if normalRate != nil && v > *normalRate {
		if warnRate != nil && v > *warnRate {
			return ScoreCritical
		}
		return ScoreWarning
	}
	return ScoreHealthy
}

// scoreBool 布尔：命中正常值→100，命中异常值→20，无法判定→不扣分。
func scoreBool(normal, abnormal string, v float64) float64 {
	cur := boolToken(v)
	if abnormal != "" && cur == strings.TrimSpace(abnormal) {
		return ScoreCritical
	}
	if normal != "" && cur == strings.TrimSpace(normal) {
		return ScoreHealthy
	}
	return ScoreHealthy
}

func boolToken(v float64) string {
	if v == 0 {
		return "0"
	}
	if v == 1 {
		return "1"
	}
	return strings.TrimSpace(itoa(int(v)))
}
