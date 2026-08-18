package scoring

import "encoding/json"

// DimensionScore 雷达图用的维度得分（前端详情页展示健康画像）
type DimensionScore struct {
	Dimension string  `json:"dimension"` // 维度名称
	Score     float64 `json:"score"`     // 维度0-100
	Weight    float64 `json:"weight"`    // 权重展示
}

// bdDimFull 复用 breakdown 结构解析维度分（含 score/weight）
type bdDimFull struct {
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
	Weight    float64 `json:"weight"`
}
type bdRootFull struct {
	Dimensions []bdDimFull `json:"dimensions"`
}

// ParseDimensions 从快照 breakdown JSON 解析维度分。
// 健康卡的 breakdown 为 "null"，此时返回空，交由前端按满分渲染。
func ParseDimensions(breakdownJSON string) []DimensionScore {
	if breakdownJSON == "" || breakdownJSON == "null" {
		return nil
	}
	var root bdRootFull
	if err := json.Unmarshal([]byte(breakdownJSON), &root); err != nil {
		return nil
	}
	out := make([]DimensionScore, 0, len(root.Dimensions))
	for _, d := range root.Dimensions {
		out = append(out, DimensionScore{
			Dimension: d.Dimension, Score: d.Score, Weight: d.Weight,
		})
	}
	return out
}
