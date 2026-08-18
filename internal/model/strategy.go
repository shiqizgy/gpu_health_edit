package model

import "time"

// ScoringStrategy 评分策略表
// 不同任务使用的指标、评估方式可能不同， 所以把"维度权重 + 各指标权重 + 各指标曲线"打包成一个可复用的策略。
// 计算某集群/某卡健康度时，选择对应策略即可。
// 设计说明：
//   - 一个策略 = 一行 ScoringStrategy + 多行 StrategyMetricRule（一对多）。
//   - dimension_weights 用JSON存维度的权重，便于整体读写。
//   - is_default 标记默认策略；未指定策略时用它。
//   - 通过策略这层抽象，新增任务类型只需新增一条策略，无需改评分代码。
type ScoringStrategy struct {
	ID               uint64               `gorm:"primaryKey;autoIncrement" json:"id"`
	Code             string               `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"` // 策略代码
	Name             string               `gorm:"type:varchar(128);not null" json:"name"`            // 策略名称
	Description      string               `gorm:"type:varchar(255)" json:"description"`              // 策略说明
	DimensionWeights string               `gorm:"type:json;not null" json:"dimension_weights"`       // 维度权重
	IsDefault        bool                 `gorm:"default:false;index" json:"is_default"`             // 是否默认策略
	Rules            []StrategyMetricRule `gorm:"foreignKey:StrategyID" json:"rules,omitempty"`      // 该策略下的指标规则
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

func (ScoringStrategy) TableName() string { return "scoring_strategy" }

// StrategyMetricRule 策略-指标规则表
// 描述"某策略下，某个指标的权重 + 是否一票否决"。
//
// 设计说明：
//   - 把指标的权重放在这里，因为权重可以随策略变化。
//   - 单指标如何映射到 0-100 分：由 MetricDefinition 的 value_type + 阈值边界，在评分引擎中按类型直接判档(健康100/警告60/故障20)。
//   - is_veto + veto_threshold 描述一票否决：当指标值 >= 阈值时触发否决。
type StrategyMetricRule struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID    uint64  `gorm:"index:idx_strategy_metric,unique;not null" json:"strategy_id"`
	MetricKey     string  `gorm:"type:varchar(128);index:idx_strategy_metric,unique;not null" json:"metric_key"`
	Weight        float64 `gorm:"type:decimal(6,3);not null;default:1.000" json:"weight"` // 指标在维度内的权重
	IsVeto        bool    `gorm:"default:false" json:"is_veto"`                           // 是否一票否决
	VetoThreshold float64 `gorm:"type:double" json:"veto_threshold"`                      // 否决阈值
}

func (StrategyMetricRule) TableName() string { return "strategy_metric_rule" }
