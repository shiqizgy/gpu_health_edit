package types

// MetricFrame 一张卡某时刻的全量指标
type MetricFrame struct {
	UUID    string             `json:"uuid"`
	TS      int64              `json:"ts"`
	Metrics map[string]float64 `json:"metrics"`
}
