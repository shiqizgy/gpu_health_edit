package model

import "time"

// ScoringStrategy 评分策略表
// 对应需求 2.3(2) 健康值：不同任务使用的指标、评估方式可能不同，
// 所以把"维度权重 + 各指标权重 + 各指标曲线"打包成一个可复用的策略。
// 计算某集群/某卡健康度时，选择对应策略即可。
//
// 设计说明：
//   - 一个策略 = 一行 ScoringStrategy + 多行 StrategyMetricRule（一对多）。
//   - dimension_weights 用 JSON 存四个维度的权重，便于整体读写。
//   - is_default 标记默认策略；未指定策略时用它。
//   - 通过策略这层抽象，新增任务类型只需新增一条策略，无需改评分代码。
type ScoringStrategy struct {
	ID               uint64               `gorm:"primaryKey;autoIncrement" json:"id"`
	Code             string               `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"`        // 策略代码
	Name             string               `gorm:"type:varchar(128);not null" json:"name"`                  // 策略名称
	Description      string               `gorm:"type:varchar(255)" json:"description"`                    // 策略说明
	DimensionWeights string               `gorm:"type:json;not null" json:"dimension_weights"`             // 维度权重 JSON: {"hardware":0.45,...}
	IsDefault        bool                 `gorm:"default:false;index" json:"is_default"`                   // 是否默认策略
	Version          int64                `gorm:"not null;default:1" json:"version"`                       // 版本号(改动+1,供评分服务热加载)
	Rules            []StrategyMetricRule `gorm:"foreignKey:StrategyID" json:"rules,omitempty"`            // 该策略下的指标规则
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

func (ScoringStrategy) TableName() string { return "scoring_strategy" }

// StrategyMetricRule 策略-指标规则表
// 描述"某策略下，某个指标的权重 + 用什么曲线打分 + 是否一票否决"。
//
// 设计说明：
//   - 把指标的权重和曲线放在这里(而非 MetricDefinition)，是因为它们随策略变化。
//   - curve_type + curve_params 描述单指标如何映射到 0-100 分：
//       piecewise: curve_params = {"points":[[0,100],[80,100],...]}
//       log:       curve_params = {"threshold":[warn,crit]}
//       xid_table: curve_params 可空(查表逻辑在代码里)
//       veto:      curve_params 可空
//       none:      不扣分(利用率等)
//   - is_veto + veto_threshold 描述一票否决：当指标值 >= 阈值时触发否决。
type StrategyMetricRule struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID    uint64  `gorm:"index:idx_strategy_metric,unique;not null" json:"strategy_id"`
	MetricKey     string  `gorm:"type:varchar(128);index:idx_strategy_metric,unique;not null" json:"metric_key"`
	Weight        float64 `gorm:"type:decimal(6,3);not null;default:1.000" json:"weight"`     // 指标在维度内的权重
	CurveType     string  `gorm:"type:varchar(32);not null;default:none" json:"curve_type"`  // 曲线类型
	CurveParams   string  `gorm:"type:json" json:"curve_params"`                             // 曲线参数 JSON
	IsVeto        bool    `gorm:"default:false" json:"is_veto"`                              // 是否一票否决
	VetoThreshold float64 `gorm:"type:double" json:"veto_threshold"`                         // 否决阈值
}

func (StrategyMetricRule) TableName() string { return "strategy_metric_rule" }
