package scoring

import (
	"encoding/json"
	"strconv"
	"strings"
)

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

type EnumScoreRule struct {
	Mode     string             `json:"mode"`
	Default  *float64           `json:"default"`
	Map      map[string]float64 `json:"map"`
	Critical string             `json:"critical"`
	Warning  string             `json:"warning"`
}

// MetricBounds 单指标评分所需的全部边界（编译策略时从 MetricDefinition 填充）。
type MetricBounds struct {
	ValueType int  // 原始 value_type（含中文），用前缀判定
	IsRate    bool //  累计类是速率型还是存量型（由 rate_unit 有无时间分母决定）

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

	//位掩码与枚举类别
	EnumScore *EnumScoreRule
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
		if b.IsRate {
			return scoreRate(b.NormalRate, b.WarnRate, value)
		}
		return scoreRange(b, value)
	case VTBool:
		return scoreBool(b.BoolNormal, b.BoolAbnormal, value)
	case VTOrdinal:
		return scoreEnum(b.EnumScore, value)
	case VTOther:
		return ScoreHealthy // 版本号/字符串类，不参与数值评分
	default: // VTGauge 及未知
		return scoreRange(b, value)
	}
}

func IsRateUnit(unit string) bool {
	_, _, ok := parseRateUnit(unit)
	return ok
}

// 时间分母 → 秒数。窗口型(≥60s)按滑动窗口累计，瞬时型(<60s)按每秒速率。
var denomSeconds = map[string]float64{
	"秒": 1, "s": 1, "S": 1,
	"分钟": 60, "min": 60,
	"小时": 3600, "h": 3600, "H": 3600,
	"天": 86400, "d": 86400, "D": 86400,
	"周": 604800, "月": 2592000,
}

// parseRateUnit 解析速率单位。
// 返回 (窗口秒数, 值缩放, 是否速率型)
//
//	"次/天"  → (86400, 1,    true)
//	"μs/s"   → (1,     1e-6, true)
//	"次"     → (0,     1,    false)   ← 存量型
//	""       → (0,     1,    false)
func parseRateUnit(unit string) (float64, float64, bool) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return 0, 1, false
	}
	i := strings.Index(unit, "/")
	if i < 0 {
		return 0, 1, false // 没有分母 → 存量型
	}
	numer, denom := unit[:i], strings.TrimSpace(unit[i+1:])
	sec, ok := denomSeconds[denom]
	if !ok {
		return 0, 1, false // 分母不是时间（如 GB/s 的 s 会命中，但 次/包 之类不会）
	}
	scale := 1.0
	if strings.HasPrefix(numer, "μs") || strings.HasPrefix(numer, "us") {
		scale = 1e-6 // μs → s
	}
	return sec, scale, true
}

// ParseRateUnit 导出封装，供外部包调用。
func ParseRateUnit(unit string) (float64, float64, bool) {
	return parseRateUnit(unit)
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
	if normalRate == nil {
		return ScoreHealthy // 未配置速率阈值 → 不扣分（Compile 时已有日志提示）
	}
	if v <= *normalRate {
		return ScoreHealthy
	}
	if warnRate == nil || v > *warnRate {
		return ScoreCritical
	}
	return ScoreWarning
}

// scoreBool 布尔：命中正常值→100，命中异常值→20，无法判定→不扣分。
func scoreBool(normal, abnormal string, v float64) float64 {
	cur := boolToken(v)
	normal, abnormal = strings.TrimSpace(normal), strings.TrimSpace(abnormal)
	if abnormal != "" && cur == abnormal {
		return ScoreCritical
	}
	if normal != "" && cur == normal {
		return ScoreHealthy
	}
	if normal == "" && abnormal == "" {
		return ScoreHealthy // 未配置正常/异常值 → 不扣分
	}
	return ScoreWarning // ★ 配了但落在定义之外 → 状态未知，判警告
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

func ParseEnumScore(raw string) *EnumScoreRule {
	if strings.TrimSpace(raw) == "" || raw == "null" {
		return nil
	}
	var r EnumScoreRule
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil
	}
	return &r
}

func parseMask(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		n, _ := strconv.ParseInt(s[2:], 16, 64)
		return n
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func scoreEnum(r *EnumScoreRule, v float64) float64 {
	if r == nil {
		return ScoreHealthy // 未配置规则：不扣分（启动时会有日志提示）
	}
	switch r.Mode {
	case "bitmask":
		iv := int64(v)
		if m := parseMask(r.Critical); m != 0 && iv&m != 0 {
			return ScoreCritical
		}
		if m := parseMask(r.Warning); m != 0 && iv&m != 0 {
			return ScoreWarning
		}
		return ScoreHealthy
	default: // enum
		if s, ok := r.Map[strconv.FormatInt(int64(v), 10)]; ok {
			return s
		}
		if r.Default != nil {
			return *r.Default
		}
		return ScoreHealthy
	}
}

// IsScorable 判断该指标在当前边界配置下是否可能扣分。
func (b MetricBounds) IsScorable() bool {
	switch b.ValueType {
	case VTOther:
		return false
	case VTOrdinal:
		return b.EnumScore != nil
	case VTGauge, VTGaugeRate:
		return b.UpperBond != nil || b.LowerBound != nil
	case VTCounter, VTDuration, VTLevel:
		if b.IsRate {
			return b.NormalRate != nil
		}
		return b.UpperBond != nil
	case VTBool:
		return b.BoolNormal != "" || b.BoolAbnormal != ""
	}
	return true
}
