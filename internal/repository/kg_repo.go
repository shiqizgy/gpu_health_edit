package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// KGRepo 故障知识图谱仓储。
//
// 只负责数据访问，所有业务校验（类型白名单、端点约束、JSON 合法性）
// 都在 service 层完成，避免校验逻辑散落在两处对不上。
type KGRepo struct{ db *gorm.DB }

func NewKGRepo(db *gorm.DB) *KGRepo { return &KGRepo{db: db} }

// DB 暴露底层句柄，供 service 层开启事务。
func (r *KGRepo) DB() *gorm.DB { return r.db }

// ---------------------------------------------------------------------------
// 查询条件
// ---------------------------------------------------------------------------

// NodeQuery 节点列表查询条件。
type NodeQuery struct {
	NodeType string // 按类型过滤，空=不限
	Severity string // 按严重等级过滤，空=不限
	Keyword  string // 在 name / summary / node_key 中模糊匹配
	Limit    int
	Offset   int
}

// ---------------------------------------------------------------------------
// 节点
// ---------------------------------------------------------------------------

// ListNodes 分页查询节点。
func (r *KGRepo) ListNodes(q NodeQuery) ([]model.KGNode, int64, error) {
	tx := r.db.Model(&model.KGNode{})
	if q.NodeType != "" {
		tx = tx.Where("node_type = ?", q.NodeType)
	}
	if q.Severity != "" {
		tx = tx.Where("severity = ?", q.Severity)
	}
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("name LIKE ? OR summary LIKE ? OR node_key LIKE ?", like, like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var out []model.KGNode
	err := tx.Order("node_type ASC, id DESC").
		Limit(q.Limit).Offset(q.Offset).Find(&out).Error
	return out, total, err
}

// GetNode 按主键取单个节点。不存在时返回 gorm.ErrRecordNotFound。
func (r *KGRepo) GetNode(id uint64) (*model.KGNode, error) {
	var n model.KGNode
	if err := r.db.First(&n, id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// GetNodesByIDs 批量取节点。用于组装子图，避免逐个查询造成 N+1。
func (r *KGRepo) GetNodesByIDs(ids []uint64) ([]model.KGNode, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []model.KGNode
	err := r.db.Where("id IN ?", ids).Find(&out).Error
	return out, err
}

// GetNodeByKey 按业务唯一键取节点，供幂等导入使用。
func (r *KGRepo) GetNodeByKey(key string) (*model.KGNode, error) {
	var n model.KGNode
	if err := r.db.Where("node_key = ?", key).First(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// CreateNode 新建节点。node_key 冲突时由唯一索引拦截，错误由 service 层翻译。
func (r *KGRepo) CreateNode(n *model.KGNode) error {
	n.Version = 1
	return r.db.Create(n).Error
}

// UpdateNode 带乐观锁的更新。
// 返回 (影响行数, error)。影响行数为 0 表示版本号不匹配或记录已被删除，
// 由 service 层翻译成 409 Conflict。
func (r *KGRepo) UpdateNode(id uint64, expectVersion int, fields map[string]any) (int64, error) {
	fields["version"] = gorm.Expr("version + 1")
	res := r.db.Model(&model.KGNode{}).
		Where("id = ? AND version = ?", id, expectVersion).
		Updates(fields)
	return res.RowsAffected, res.Error
}

// DeleteNodeCascade 在单个事务里删除节点及其所有关联边。
//
// 这里必须用事务：如果先删节点再删边，中途失败会留下悬空边，
// 后续组装子图时会引用到不存在的节点，前端渲染直接报错。
// 返回被连带删除的边数，供接口提示用户。
func (r *KGRepo) DeleteNodeCascade(id uint64) (int64, error) {
	var edgeCount int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 先锁住节点行，防止并发的建边操作在删除窗口内插入新边
		var n model.KGNode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&n, id).Error; err != nil {
			return err
		}
		res := tx.Where("from_id = ? OR to_id = ?", id, id).Delete(&model.KGEdge{})
		if res.Error != nil {
			return res.Error
		}
		edgeCount = res.RowsAffected
		return tx.Delete(&model.KGNode{}, id).Error
	})
	return edgeCount, err
}

// CountNodesByType 各类型节点数量统计，供页面顶部概览。
func (r *KGRepo) CountNodesByType() (map[string]int64, error) {
	type row struct {
		NodeType string
		Cnt      int64
	}
	var rows []row
	if err := r.db.Model(&model.KGNode{}).
		Select("node_type, COUNT(*) AS cnt").
		Group("node_type").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, x := range rows {
		out[x.NodeType] = x.Cnt
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 边
// ---------------------------------------------------------------------------

// ListEdgesWithin 查询「两端都在 ids 集合内」的边。
// 组装子图时用这个：只有两端都在画布上的边才画得出来。
func (r *KGRepo) ListEdgesWithin(ids []uint64) ([]model.KGEdge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []model.KGEdge
	err := r.db.Where("from_id IN ? AND to_id IN ?", ids, ids).Find(&out).Error
	return out, err
}

// ListEdgesTouching 查询「任意一端在 ids 集合内」的边。
// 邻域展开时用这个：需要通过边找到集合外的新节点。
func (r *KGRepo) ListEdgesTouching(ids []uint64) ([]model.KGEdge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []model.KGEdge
	err := r.db.Where("from_id IN ? OR to_id IN ?", ids, ids).Find(&out).Error
	return out, err
}

// ListEdgesOfNode 查询与单个节点相连的全部边，供详情面板展示。
func (r *KGRepo) ListEdgesOfNode(id uint64) ([]model.KGEdge, error) {
	var out []model.KGEdge
	err := r.db.Where("from_id = ? OR to_id = ?", id, id).
		Order("rel_type ASC, id ASC").Find(&out).Error
	return out, err
}

// GetEdge 按主键取边。
func (r *KGRepo) GetEdge(id uint64) (*model.KGEdge, error) {
	var e model.KGEdge
	if err := r.db.First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateEdgeChecked 在事务内校验并创建边。
//
// 关键点：用 SELECT ... FOR UPDATE 锁住两个端点节点，确保在建边的瞬间
// 它们不会被并发的 DeleteNodeCascade 删掉。没有这道锁，
// "A 事务在删节点 / B 事务在建指向它的边" 会产生悬空边。
//
// validate 回调在拿到两个端点节点之后调用，由 service 层注入类型约束校验，
// 这样 repo 不需要知道业务规则，业务规则也不需要重复查一次节点。
func (r *KGRepo) CreateEdgeChecked(
	e *model.KGEdge,
	validate func(from, to *model.KGNode) error,
) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var nodes []model.KGNode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", []uint64{e.FromID, e.ToID}).
			Find(&nodes).Error; err != nil {
			return err
		}

		byID := make(map[uint64]*model.KGNode, 2)
		for i := range nodes {
			byID[nodes[i].ID] = &nodes[i]
		}
		from, okF := byID[e.FromID]
		to, okT := byID[e.ToID]
		if !okF {
			return fmt.Errorf("起点节点 id=%d 不存在", e.FromID)
		}
		if !okT {
			return fmt.Errorf("终点节点 id=%d 不存在", e.ToID)
		}
		if validate != nil {
			if err := validate(from, to); err != nil {
				return err
			}
		}

		e.Version = 1
		return tx.Create(e).Error
	})
}

// UpdateEdge 带乐观锁的边更新。只允许改 label / weight / props，
// 端点和关系类型不可变——要改就删了重连，否则唯一索引的语义会被绕过。
func (r *KGRepo) UpdateEdge(id uint64, expectVersion int, fields map[string]any) (int64, error) {
	fields["version"] = gorm.Expr("version + 1")
	res := r.db.Model(&model.KGEdge{}).
		Where("id = ? AND version = ?", id, expectVersion).
		Updates(fields)
	return res.RowsAffected, res.Error
}

// DeleteEdge 删除单条边。
func (r *KGRepo) DeleteEdge(id uint64) (int64, error) {
	res := r.db.Delete(&model.KGEdge{}, id)
	return res.RowsAffected, res.Error
}

// CountEdges 边总数，供概览统计。
func (r *KGRepo) CountEdges() (int64, error) {
	var n int64
	err := r.db.Model(&model.KGEdge{}).Count(&n).Error
	return n, err
}

// ---------------------------------------------------------------------------
// 工具
// ---------------------------------------------------------------------------

// IsNotFound 判断是否为"记录不存在"错误。
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsDuplicate 判断是否为唯一索引冲突。
// GORM v2 + MySQL 驱动会返回 gorm.ErrDuplicatedKey（需开启 TranslateError），
// 未开启时退化为字符串匹配，两条路都覆盖以免依赖具体配置。
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") || strings.Contains(msg, "1062")
}
