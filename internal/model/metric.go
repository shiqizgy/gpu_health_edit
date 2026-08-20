package model

import "time"

// MetricDefinition 指标定义表
// 存放指标名称、单位、类型、维度、概念说明、
// 所属设备、正常/异常取值范围、备注。前后端支持增删改查。
// 设计说明：
//   - metric_key 是 DCGM 字段名（如 DCGM_FI_DEV_GPU_TEMP），全局唯一， 仿真服务和评分服务都靠它来对齐指标。
//   - dimension 是指标维度之一， 评分时按维度聚合。
//   - 这里只描述"指标是什么"，不存权重和曲线，权重和曲线属于"策略"， 因为同一个指标在不同任务（策略）下权重可能不同。这是本项目可扩展性的关键拆分。
type MetricDefinition struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	SeqNo       *int   `gorm:"type:int" json:"seq_no"`                                    // Excel 内序号
	MetricName  string `gorm:"type:varchar(128);uniqueIndex;not null" json:"metric_name"` // 指标名称(DCGM字段或NPU字段)
	OfficialNum string `gorm:"type:varchar(128);Index;not null" json:"official_num"`      // 官方编号或说明
	CardType    string `gorm:"type:varchar(64);not null" json:"card_type"`                // 卡的类型（GPU/NPU）
	Conception  string `gorm:"column:concept;type:text;not null" json:"concept"`          // 概念说明
	Unit        string `gorm:"type:varchar(32)" json:"unit"`                              // 指标单位
	Dimension   string `gorm:"type:varchar(32);index;not null" json:"dimension"`          // 指标维度
	//DeviceType  string `gorm:"type:varchar(32);not null;default:gpu" json:"device_type"`  所属设备,目前用CardType字段代替

	//归属主体与健康度评分用途
	OwnerSubject  int `gorm:"type:int;index" json:"owner_subject"`  //归属主体：A单卡自身/B链路/C共享/D环境/E跨节点
	HealthPurpose int `gorm:"type:int;index" json:"health_purpose"` //健康度评分用途：核心/归因/配置合规/性能容量
	ValueType     int `gorm:"type:int;index" json:"value_type"`     //指标的数值类型：Gauge连续数值/Gauge_Rate比率/Counter累计计数...

	//三档指标阈值边界
	WorkRange    string   `gorm:"type:varchar(128)" json:"work_range"`                   // 工作范围，如 0~85
	UpperBond    *float64 `gorm:"type:double" json:"upper_bond"`                         //连续数值上界
	LowerBound   *float64 `gorm:"type:double" json:"lower_bound"`                        //连续数值下界
	WarnupBound  *float64 `gorm:"column:warn_upbound;type:double" json:"warn_upbound"`   //todo 警告/故障上界
	WarnlowBound *float64 `gorm:"column:warn_lowbound;type:double" json:"warn_lowbound"` //todo 警告/故障下界

	// 计算速率型指标的边界（counter用）
	NormalRate     *float64 `gorm:"type:double" json:"normal_rate"`    // 正常速率上限
	WarnRate       *float64 `gorm:"type:double" json:"warn_rate"`      // 警告速率上限
	NormalRateUnit string   `gorm:"type:varchar(32)" json:"rate_unit"` // 次/秒 次/分钟 次/小时 次/天 μs/s

	// 布尔/枚举
	BoolNormal   string `gorm:"type:varchar(64)" json:"bool_normal"`   //正常的bool值
	BoolAbnormal string `gorm:"type:varchar(64)" json:"bool_abnormal"` //异常的bool值
	EnumResult   string `gorm:"type:text" json:"enum_result"`          //todo 修改枚举类型
	EnumScore    string `gorm:"type:json" json:"enum_score"`           //todo 枚举/位掩码的结构化评分规则

	//否决
	IsVeto int `gorm:"type:tinyint;index" json:"is_veto"` //todo 是否“一票否决”：1是、0否

	DerateThreshold string `gorm:"type:text" json:"derate_threshold"` //降频/关机阈值说明
	SourceRef       string `gorm:"type:text" json:"source_ref"`       //来源依据
	Vender          string `gorm:"type:varchar(32)" json:"vender"`    //厂商

	Remark      string    `gorm:"type:varchar(255)" json:"remark"`   // 备注
	IsHealthKey bool      `gorm:"default:true" json:"is_health_key"` // 是否参与健康评分
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MetricDefinition) TableName() string { return "metric_definition" }
