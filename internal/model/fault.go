package model

import "time"

// FaultKnowledge 故障知识图谱表
// 对应需求 2.4(1) 故障知识图谱：单独一张表，前后端支持增删改查。
//
// 设计说明：
//   - 按需求存：故障类型、故障表现形式、可能原因、涉及到的指标变化等。
//   - 本表纯知识展示，不参与任何健康度计算，与评分链路完全解耦。
//   - related_metrics 用 JSON 存涉及的指标 key 列表，便于将来与指标系统关联，
//     但当前阶段仅作展示。
//   - 这里用关系表而非图数据库：当前需求是结构化的知识条目展示+CRUD，
//     关系表足够且部署更轻（无需额外的图数据库），符合"本地负载小"原则。
type FaultKnowledge struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	FaultType      string    `gorm:"type:varchar(128);index;not null" json:"fault_type"`   // 故障类型
	XIDCode        string    `gorm:"type:varchar(32)" json:"xid_code"`                     // 关联 XID 码(可空)
	Symptom        string    `gorm:"type:text;not null" json:"symptom"`                    // 故障表现形式
	PossibleCause  string    `gorm:"type:text" json:"possible_cause"`                      // 可能原因
	MetricChanges  string    `gorm:"type:text" json:"metric_changes"`                      // 涉及的指标变化(文字描述)
	RelatedMetrics string    `gorm:"type:json" json:"related_metrics"`                     // 涉及的指标 key 列表 JSON
	Severity       string    `gorm:"type:varchar(32);default:warning" json:"severity"`     // 严重等级 warning/critical/fatal
	Suggestion     string    `gorm:"type:text" json:"suggestion"`                          // 处置建议
	Reference      string    `gorm:"type:varchar(255)" json:"reference"`                   // 参考资料链接
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (FaultKnowledge) TableName() string { return "fault_knowledge" }
