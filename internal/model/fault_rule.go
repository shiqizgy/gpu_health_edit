package model

import "time"

// FaultRule 故障规则表
// 定义"什么情况算一个故障、叫什么名字、多严重"，是故障池里"故障名称"的来源。
//
// 三种触发源(trigger_type)：
//   - threshold：某指标越过门限。门限优先用规则自带 threshold，
//     为空则回退到该指标 metric_definition 上的 crit_threshold / 正常区间。
//   - veto：一票否决（直接由评分结果的 veto 标志生成，可不建规则行）。
//   - xid：致命 XID（保留，当前由 veto 覆盖）。
type FaultRule struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(128);not null" json:"name"` // 故障名称，如"显存温度过高"
	TriggerType string    `gorm:"type:varchar(16);not null;default:threshold" json:"trigger_type"`
	MetricKey   string    `gorm:"type:varchar(128);index" json:"metric_key"`                  // threshold 类型关联的指标 key
	Operator    string    `gorm:"type:varchar(8)" json:"operator"`                            // >=,<=,>,<,==（空则按指标 direction 推断）
	Threshold   *float64  `gorm:"type:double" json:"threshold"`                               // 规则自定义门限，空则用指标 crit/区间
	Severity    string    `gorm:"type:varchar(16);not null;default:critical" json:"severity"` // warning/critical/fatal
	KnowledgeID *uint64   `gorm:"index" json:"knowledge_id"`                                  // 关联故障知识图谱(可空)，详情里给处置建议
	Enabled     bool      `gorm:"default:true;index" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (FaultRule) TableName() string { return "fault_rule" }
