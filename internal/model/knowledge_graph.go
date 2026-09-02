package model

import "time"

// ============================================================================
// 故障知识图谱：属性图模型（节点 kg_node + 有向边 kg_edge）
//
// 设计边界（重要）：
//   1. 本模块与健康度评分链路完全解耦。评分只读 accel_metric_scoring /
//      strategy_metric_rule / gpu_health_snapshot，不读本模块任何表。
//   2. 指标节点通过 props.metric_name 弱引用 accel_metric_scoring.metric_name，
//      不建外键、不做级联。指标定义被删除时图谱节点保留（成为悬空引用，
//      由前端标记为"未匹配"），不会影响评分。
//   3. 本模块不参与 seed.Reset 的清表列表——图谱内容是人工沉淀的知识资产，
//      不能被"重灌种子数据"这个操作清空。
//
// 为什么用关系表而不是图数据库：
//   当前规模是"数百节点 + 数千边"的领域知识，查询模式是「筛选子图」和
//   「N 跳邻域展开」，MySQL 加索引完全够用，且不需要为预发环境额外部署
//   一套 Neo4j。BFS 在服务层做，深度和节点数都有硬上限，不会出现图数据库
//   那种无界遍历打爆内存的问题。
// ============================================================================

// ---------------------------------------------------------------------------
// 节点类型
// ---------------------------------------------------------------------------

const (
	NodeTypeFault   = "fault"   // GPU 故障
	NodeTypeConcept = "concept" // 故障概念（术语、机理、分类）
	NodeTypeScope   = "scope"   // 影响范围（单卡 / 整机 / 通信域 / 任务）
	NodeTypeMetric  = "metric"  // 影响指标（弱引用 accel_metric_scoring）
)

// NodeTypeMeta 节点类型的展示元数据，由 /kg/meta 下发给前端，
// 保证前后端对类型的中文名和配色只有一份定义。
type NodeTypeMeta struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Color string `json:"color"`
	Desc  string `json:"desc"`
}

// NodeTypes 全部合法节点类型。新增类型只需在此追加一行，
// 校验、前端图例、配色会自动生效。
var NodeTypes = []NodeTypeMeta{
	{NodeTypeFault, "故障", "#ef4444", "一类可观测的 GPU/NPU 故障"},
	{NodeTypeConcept, "概念", "#38bdf8", "故障机理、术语或分类概念"},
	{NodeTypeScope, "影响范围", "#f59e0b", "故障波及的硬件层级或业务范围"},
	{NodeTypeMetric, "影响指标", "#22c55e", "能反映该故障的监控指标"},
}

// IsValidNodeType 校验节点类型是否在白名单内。
func IsValidNodeType(t string) bool {
	for _, m := range NodeTypes {
		if m.Type == t {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 关系类型 + 端点类型约束
// ---------------------------------------------------------------------------

const (
	RelBelongsTo    = "BELONGS_TO"     // 故障 → 概念：该故障属于某个机理/分类
	RelAffects      = "AFFECTS"        // 故障 → 影响范围
	RelIndicatedBy  = "INDICATED_BY"   // 故障 → 指标：该故障可由此指标反映
	RelCauses       = "CAUSES"         // 故障 → 故障：级联引发
	RelSubConceptOf = "SUB_CONCEPT_OF" // 概念 → 概念：概念层级
	RelRelatedTo    = "RELATED_TO"     // 任意 → 任意：泛化关联（兜底）
)

// RelTypeMeta 关系类型元数据 + 端点约束。
// Pairs 为空表示不限制端点类型（仅 RELATED_TO 如此）。
type RelTypeMeta struct {
	Type  string      `json:"type"`
	Label string      `json:"label"`
	Desc  string      `json:"desc"`
	Pairs [][2]string `json:"pairs"` // 允许的 (起点类型, 终点类型) 组合
}

// RelTypes 全部合法关系类型。
// Pairs 是这套模型的"schema 约束"——它保证图谱不会被连成语义上讲不通的样子，
// 比如把"影响范围"连到"指标"上。
var RelTypes = []RelTypeMeta{
	{
		Type: RelBelongsTo, Label: "属于概念", Desc: "该故障属于某个机理或分类",
		Pairs: [][2]string{{NodeTypeFault, NodeTypeConcept}},
	},
	{
		Type: RelAffects, Label: "影响", Desc: "该故障波及的范围",
		Pairs: [][2]string{{NodeTypeFault, NodeTypeScope}},
	},
	{
		Type: RelIndicatedBy, Label: "指标反映", Desc: "该故障可由此指标反映",
		Pairs: [][2]string{{NodeTypeFault, NodeTypeMetric}},
	},
	{
		Type: RelCauses, Label: "引发", Desc: "上游故障引发下游故障",
		Pairs: [][2]string{{NodeTypeFault, NodeTypeFault}},
	},
	{
		Type: RelSubConceptOf, Label: "子概念", Desc: "概念之间的层级关系",
		Pairs: [][2]string{{NodeTypeConcept, NodeTypeConcept}},
	},
	{
		Type: RelRelatedTo, Label: "相关", Desc: "语义相关，不限端点类型",
		Pairs: nil,
	},
}

// IsValidRelType 校验关系类型是否在白名单内。
func IsValidRelType(t string) bool {
	for _, m := range RelTypes {
		if m.Type == t {
			return true
		}
	}
	return false
}

// AllowRelPair 校验 (关系类型, 起点类型, 终点类型) 组合是否合法。
// 未知关系类型一律拒绝；Pairs 为 nil 表示该关系不限端点。
func AllowRelPair(relType, fromType, toType string) bool {
	for _, m := range RelTypes {
		if m.Type != relType {
			continue
		}
		if len(m.Pairs) == 0 {
			return true
		}
		for _, p := range m.Pairs {
			if p[0] == fromType && p[1] == toType {
				return true
			}
		}
		return false
	}
	return false
}

// ---------------------------------------------------------------------------
// 表结构
// ---------------------------------------------------------------------------

// KGNode 图谱节点。
//
// NodeKey 是业务唯一键（如 "fault:XID_48"、"metric:DCGM_FI_DEV_GPU_TEMP"），
// 由调用方指定或由服务层按 类型:名称 自动生成。它的作用是让"导入"操作可以
// 幂等重跑，不会因为重复执行而产生一堆同名节点。
type KGNode struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"                json:"id"`
	NodeKey  string `gorm:"type:varchar(191);uniqueIndex;not null"  json:"node_key"`
	NodeType string `gorm:"type:varchar(32);index;not null"         json:"node_type"`
	Name     string `gorm:"type:varchar(191);index;not null"        json:"name"`

	Summary     string `gorm:"type:varchar(512)" json:"summary"`     // 一句话摘要，图上 tooltip 用
	Description string `gorm:"type:text"         json:"description"` // 详细说明

	// Severity 仅对 fault 类型有意义：warning / critical / fatal。
	// 提成独立列而不是塞进 props，是因为前端要按它决定节点配色和大小，
	// 每次渲染都解析 JSON 不划算。
	Severity string `gorm:"type:varchar(16);index" json:"severity"`

	// Props 扩展属性 JSON。约定用法：
	//   metric 节点：{"metric_name":"DCGM_FI_DEV_GPU_TEMP","card_type":"GPU"}
	//   fault  节点：{"xid_code":"48","vendor":"NVIDIA"}
	// 写入前由服务层校验必须是合法 JSON 对象。
	Props string `gorm:"type:json" json:"props"`

	// Version 乐观锁版本号。多人同时编辑同一节点时，后提交的一方会收到 409，
	// 而不是静默覆盖前一个人的修改。
	Version int `gorm:"not null;default:1" json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (KGNode) TableName() string { return "kg_node" }

// KGEdge 图谱有向边。
//
// (from_id, to_id, rel_type) 建唯一索引，从数据库层杜绝重复连线；
// from_id / to_id 各建普通索引，支撑邻域展开的双向查询。
type KGEdge struct {
	ID uint64 `gorm:"primaryKey;autoIncrement" json:"id"`

	FromID  uint64 `gorm:"not null;uniqueIndex:uk_kg_edge,priority:1;index:idx_kg_edge_from" json:"from_id"`
	ToID    uint64 `gorm:"not null;uniqueIndex:uk_kg_edge,priority:2;index:idx_kg_edge_to"   json:"to_id"`
	RelType string `gorm:"type:varchar(32);not null;uniqueIndex:uk_kg_edge,priority:3"      json:"rel_type"`

	Label  string  `gorm:"type:varchar(128)"  json:"label"`  // 边上的自定义说明，空则用 RelTypes 的中文名
	Weight float64 `gorm:"not null;default:1" json:"weight"` // 关联强度 0~1，影响线宽
	Props  string  `gorm:"type:json"          json:"props"`

	Version int `gorm:"not null;default:1" json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (KGEdge) TableName() string { return "kg_edge" }
