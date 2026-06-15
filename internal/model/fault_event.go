package model

import "time"

// FaultEvent 故障池事件表（有状态：开始/持续/恢复）
//
// 设计说明：
//   - 同一张卡同一个故障(dedup_key)只保留一条 open 记录，首次检测写 started_at，
//     持续期间只更新 last_seen_at，条件消失才置 resolved。避免每分钟刷一行。
//   - dedup_key = gpu_uuid + "|" + signature，signature 形如 rule:12 / veto:XID_79 / thr:DCGM_FI_DEV_GPU_TEMP。
//   - 字段对齐前端故障池表格：故障名称/涉及设备/故障集群/故障卡号/异常指标/故障开始时间/详情。
type FaultEvent struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	DedupKey  string `gorm:"type:varchar(220);index:idx_fe_dedup;not null" json:"dedup_key"`
	FaultName string `gorm:"type:varchar(128);index;not null" json:"fault_name"` // 故障名称
	Severity  string `gorm:"type:varchar(16);index" json:"severity"`             // warning/critical/fatal

	ClusterID   uint64 `gorm:"index" json:"cluster_id"`
	ClusterName string `gorm:"type:varchar(128)" json:"cluster_name"`            // 故障集群
	NodeHost    string `gorm:"type:varchar(128);index" json:"node_host"`         // 涉及设备(主机名)
	GPUUUID     string `gorm:"type:varchar(128);index;not null" json:"gpu_uuid"` // 故障卡号

	MetricKey     string  `gorm:"type:varchar(128);index" json:"metric_key"` // 异常指标 key
	MetricDisplay string  `gorm:"type:varchar(128)" json:"metric_display"`   // 异常指标中文名
	TriggerValue  float64 `gorm:"type:double" json:"trigger_value"`          // 触发时的值
	Threshold     float64 `gorm:"type:double" json:"threshold"`              // 当时的门限

	Status     string     `gorm:"type:varchar(16);index;not null;default:open" json:"status"` // open/resolved
	StartedAt  time.Time  `gorm:"index;not null" json:"started_at"`                           // 故障开始时间
	LastSeenAt time.Time  `gorm:"not null" json:"last_seen_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Detail     string     `gorm:"type:json" json:"detail"` // 详情 JSON：规则/原因/建议/score 等
}

func (FaultEvent) TableName() string { return "fault_event" }
