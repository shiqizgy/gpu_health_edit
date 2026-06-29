package model

import "time"

// Cluster 集群表（三级拓扑的顶层）
// 对应需求 2.3(1) 集群拓扑：集群-节点-GPU 三级树状结构。
type Cluster struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Code       string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"` // 集群编号
	Name       string    `gorm:"type:varchar(128);not null" json:"name"`            // 集群名称
	StrategyID *uint64   `gorm:"index" json:"strategy_id"`                          // 评分策略ID，指针类型,NULL 表示未指定
	Region     string    `gorm:"type:varchar(64)" json:"region"`                    // 所在区域/机房
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Cluster) TableName() string { return "cluster" }

// Node 节点表（三级拓扑的中间层，即物理服务器）
type Node struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterID uint64    `gorm:"index;not null" json:"cluster_id"`                       // 所属集群
	Hostname  string    `gorm:"type:varchar(128);uniqueIndex;not null" json:"hostname"` // 主机名
	IP        string    `gorm:"type:varchar(64)" json:"ip"`                             // 管理 IP
	GPUCount  int       `gorm:"default:8" json:"gpu_count"`                             // 该节点 GPU 数
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Node) TableName() string { return "node" }

// GPUCard GPU 卡基本数据表（三级拓扑的叶子层）
// 对应需求 2.3(1)：存储所监控 GPU 卡的基本数据（卡的唯一编号、属于哪个集群），
// 支持动态扩缩容（新增/下线 GPU 卡，更新拓扑）。
//
// 设计说明：
//   - uuid 是 GPU 全局唯一编号，仿真服务、Redis、评分快照都用它做主键关联。
//   - 冗余 cluster_id 字段（除了 node_id），是为了集群级聚合查询时避免多一次 join，
//     在万卡场景下能显著提速。
//   - status 支持 online/offline/maintenance，配合扩缩容。
type GPUCard struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID       string    `gorm:"type:varchar(128);uniqueIndex;not null" json:"uuid"`            // GPU 唯一编号
	NodeID     uint64    `gorm:"index;not null" json:"node_id"`                                 // 所属节点
	ClusterID  uint64    `gorm:"index;not null" json:"cluster_id"`                              // 所属集群(冗余,加速聚合)
	GPUIndex   int       `gorm:"column:gpu_index;not null" json:"gpu_index"`                    // 卡在节点内的序号 0-7
	Model      string    `gorm:"type:varchar(64)" json:"model"`                                 // 型号 如 H100-SXM5-80GB
	Status     string    `gorm:"type:varchar(32);not null;default:online" json:"status"`        // online/offline/maintenance
	Vendor     string    `gorm:"type:varchar(32);index;not null;default:unknown" json:"vendor"` // nvidia/huawei/unknown
	StrategyID *uint64   `gorm:"index" json:"strategy_id"`                                      // 评分策略ID，指针类型,NULL 表示未指定
	SN         string    `gorm:"type:varchar(128);index" json:"sn"`                             //机器SN（CK时序查询使用）
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (GPUCard) TableName() string { return "gpu_card" }
