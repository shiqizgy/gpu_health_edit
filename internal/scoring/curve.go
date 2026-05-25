package scoring

import (
	"encoding/json"
	"math"
)

// 曲线引擎：把单个指标的原始测量值映射为 0-100 分。
// 严格对应项目文档"单指标分数映射"的四种曲线 + none。

// CurveParams 曲线参数（从策略规则的 curve_params JSON 解析）
type CurveParams struct {
	Points    [][]float64 `json:"points"`    // piecewise: [[x,score],...]
	Threshold []float64   `json:"threshold"` // log: [warn, crit]
}

// ScoreMetric 根据曲线类型和参数计算单指标得分。
//   - curveType: piecewise / log / xid_table / veto / none
//   - rawParams: 策略规则里的 curve_params JSON 字符串
//   - value: 指标实测值
func ScoreMetric(curveType, rawParams string, value float64) float64 {
	switch curveType {
	case "none":
		// 利用率类指标：不扣分，恒满分（其健康意义由是否在跑体现，不影响健康度扣分）
		return 100
	case "piecewise":
		p := parseParams(rawParams)
		return piecewise(p.Points, value)
	case "log":
		p := parseParams(rawParams)
		return logScore(p.Threshold, value)
	case "xid_table":
		return xidScore(int(value))
	case "veto":
		// veto 曲线本身：触发即 0 分（否决逻辑在聚合层另行处理封顶）
		if value >= 1 {
			return 0
		}
		return 100
	default:
		return 100
	}
}

// piecewise 分段线性插值。points 按 x 升序，形如 [[0,100],[80,100],[85,90],...]。
func piecewise(points [][]float64, x float64) float64 {
	if len(points) == 0 {
		return 100
	}
	if x <= points[0][0] {
		return points[0][1]
	}
	if x >= points[len(points)-1][0] {
		return points[len(points)-1][1]
	}
	for i := 0; i < len(points)-1; i++ {
		x0, y0 := points[i][0], points[i][1]
		x1, y1 := points[i+1][0], points[i+1][1]
		if x >= x0 && x <= x1 {
			if x1 == x0 {
				return y0
			}
			t := (x - x0) / (x1 - x0)
			return y0 + (y1-y0)*t
		}
	}
	return 100
}

// logScore 对数插值。<=warn 给 100，>=crit 给 0，中间按对数刻度。
// 适合累计错误计数器：偶尔几次没事，几百次才严重。
func logScore(threshold []float64, value float64) float64 {
	if len(threshold) < 2 {
		return 100
	}
	warn, crit := threshold[0], threshold[1]
	if value <= warn {
		return 100
	}
	if value >= crit {
		return 0
	}
	if warn <= 0 {
		warn = 1
	}
	if value <= 0 {
		value = 1
	}
	ratio := math.Log(value/warn) / math.Log(crit/warn)
	return 100 - 100*ratio
}

// XID 错误码分级（与项目文档一致）。
var xidFatalSet = map[int]bool{
	48: true, 62: true, 64: true, 69: true, 74: true, 79: true, 95: true,
	110: true, 119: true, 120: true, 123: true, 140: true, 143: true,
	154: true, 167: true, 169: true, 171: true, 172: true,
}

var xidWarnSet = map[int]bool{
	13: true, 31: true, 32: true, 43: true, 44: true, 45: true,
	61: true, 63: true, 68: true, 78: true, 80: true, 92: true,
	93: true, 94: true, 109: true, 137: true,
}

// xidScore：0→100；warn 集→50；fatal 集→0（并在聚合层触发否决）；其他→80。
func xidScore(code int) float64 {
	if code == 0 {
		return 100
	}
	if xidFatalSet[code] {
		return 0
	}
	if xidWarnSet[code] {
		return 50
	}
	return 80
}

// XIDIsFatal 供聚合层判断 XID 是否致命（一票否决）。
func XIDIsFatal(code int) bool { return xidFatalSet[code] }

func parseParams(raw string) CurveParams {
	var p CurveParams
	if raw == "" {
		return p
	}
	_ = json.Unmarshal([]byte(raw), &p)
	return p
}
