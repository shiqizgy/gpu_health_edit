package model

import "time"

// MetricDefinition 指标定义表
// 对应需求 2.2 指标系统页面：存放指标名称、单位、类型、维度、概念说明、
// 所属设备、正常/异常取值范围、备注。前后端支持增删改查。
//
// 设计说明：
//   - metric_key 是 DCGM 字段名（如 DCGM_FI_DEV_GPU_TEMP），全局唯一，
//     仿真服务和评分服务都靠它来对齐指标。
//   - dimension 是四大维度之一（environment/performance/hardware/stability），
//     评分时按维度聚合。
//   - 这里只描述"指标是什么"，不存权重和曲线——权重和曲线属于"策略"，
//     因为同一个指标在不同任务（策略）下权重可能不同。这是本项目可扩展性的关键拆分。
type MetricDefinition struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	MetricKey   string    `gorm:"type:varchar(128);uniqueIndex;not null" json:"metric_key"`   // 指标名称(DCGM字段)
	DisplayName string    `gorm:"type:varchar(128);not null" json:"display_name"`             // 中文显示名
	Unit        string    `gorm:"type:varchar(32)" json:"unit"`                               // 指标单位
	MetricType  string    `gorm:"type:varchar(32);not null" json:"metric_type"`               // 指标类型: gauge/counter
	Dimension   string    `gorm:"type:varchar(32);index;not null" json:"dimension"`           // 指标维度
	Concept     string    `gorm:"type:text" json:"concept"`                                   // 指标概念说明
	DeviceType  string    `gorm:"type:varchar(32);not null;default:gpu" json:"device_type"`   // 所属设备
	NormalRange string    `gorm:"type:varchar(128)" json:"normal_range"`                      // 正常取值范围
	AbnormalRange string  `gorm:"type:varchar(128)" json:"abnormal_range"`                    // 异常取值范围
	Remark      string    `gorm:"type:varchar(255)" json:"remark"`                            // 备注
	IsHealthKey bool      `gorm:"default:true" json:"is_health_key"`                          // 是否参与健康评分
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MetricDefinition) TableName() string { return "metric_definition" }
