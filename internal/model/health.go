package model

import "time"

// GPUHealthSnapshot GPU 健康快照表（每分钟每卡一条最新评分）
// 实时评分，每分钟更新。
//
// 设计说明：
//   - 这里采用"最新态覆盖写"：每张卡只保留最新一条（uuid 唯一），
//     评分服务用 upsert 更新。这样表大小恒定为卡数（2000 行），
//     避免历史快照无限膨胀。若将来要历史趋势，再单独建时序表。
//   - breakdown 存维度明细 JSON，供单卡详情页展示各维度得分。
//   - 冗余 cluster_id 用于集群级聚合。
type GPUHealthSnapshot struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	GPUUUID    string    `gorm:"type:varchar(128);uniqueIndex;not null" json:"gpu_uuid"`
	ClusterID  uint64    `gorm:"index;not null" json:"cluster_id"`
	StrategyID uint64    `gorm:"not null" json:"strategy_id"`                  // 用哪个策略算的
	Score      float64   `gorm:"type:decimal(6,3);not null" json:"score"`      // 总分 0-100
	Level      string    `gorm:"type:varchar(32);index;not null" json:"level"` // 健康等级
	Veto       bool      `gorm:"default:false" json:"veto"`                    // 是否触发一票否决
	VetoReason string    `gorm:"type:varchar(128)" json:"veto_reason"`         // 否决原因
	Breakdown  string    `gorm:"type:json" json:"breakdown"`                   // 维度明细 JSON
	ScoredAt   time.Time `gorm:"index;not null" json:"scored_at"`              // 评分时间
}

func (GPUHealthSnapshot) TableName() string { return "gpu_health_snapshot" }

// ClusterHealthSummary 集群健康汇总表（预聚合）
// 展示每个集群的 GPU 平均分。
//
// 设计说明（万卡场景关键优化）：
//   - 评分服务每轮算完单卡分后，顺手聚合每个集群的统计写入这里。
//   - 概览页/集群表格只查这张表（行数=集群数，几十行）
//     避免每次请求去扫描几千上万行单卡快照。
//     数据量很大的情况下，用预聚合把概览层的数据量自己控制住。
type ClusterHealthSummary struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterID     uint64    `gorm:"uniqueIndex;not null" json:"cluster_id"`
	ClusterCode   string    `gorm:"type:varchar(64);not null" json:"cluster_code"`
	ClusterName   string    `gorm:"type:varchar(128);not null" json:"cluster_name"`
	TotalGPU      int       `json:"total_gpu"`                          // GPU 总数
	AvgScore      float64   `gorm:"type:decimal(6,3)" json:"avg_score"` // 平均健康分
	HealthyCnt    int       `json:"healthy_cnt"`                        // 各等级数量
	SubHealthyCnt int       `json:"sub_healthy_cnt"`
	WarningCnt    int       `json:"warning_cnt"`
	CriticalCnt   int       `json:"critical_cnt"`
	FailedCnt     int       `json:"failed_cnt"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (ClusterHealthSummary) TableName() string { return "cluster_health_summary" }
