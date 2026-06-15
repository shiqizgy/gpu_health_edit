package repository

import (
	"time"

	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// ---------------- 故障规则仓储 ----------------

type FaultRuleRepo struct{ db *gorm.DB }

func NewFaultRuleRepo(db *gorm.DB) *FaultRuleRepo { return &FaultRuleRepo{db: db} }

// ActiveRules 返回所有启用的故障规则（评分服务每轮加载一次）。
func (r *FaultRuleRepo) ActiveRules() ([]model.FaultRule, error) {
	var out []model.FaultRule
	err := r.db.Where("enabled = ?", true).Find(&out).Error
	return out, err
}

// ---------------- 故障池事件仓储 ----------------

type FaultEventRepo struct{ db *gorm.DB }

func NewFaultEventRepo(db *gorm.DB) *FaultEventRepo { return &FaultEventRepo{db: db} }

// FaultEventQuery 故障池列表查询条件（分页 + 多条件 + 关键字模糊）
type FaultEventQuery struct {
	Keyword   string  // 模糊：fault_name/node_host/gpu_uuid/metric_display
	ClusterID *uint64 // 故障集群
	Severity  string  // 严重度
	Status    string  // open/resolved（空=全部）
	Since     string  // 故障开始时间 >= (RFC3339 或 yyyy-MM-dd HH:mm:ss)
	Until     string  // 故障开始时间 <=
	Limit     int
	Offset    int
}

// List 故障池分页查询，返回当前页 + 命中总数。
func (r *FaultEventRepo) List(q FaultEventQuery) ([]model.FaultEvent, int64, error) {
	tx := r.db.Model(&model.FaultEvent{})
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where(
			"fault_name LIKE ? OR node_host LIKE ? OR gpu_uuid LIKE ? OR metric_display LIKE ?",
			like, like, like, like,
		)
	}
	if q.ClusterID != nil {
		tx = tx.Where("cluster_id = ?", *q.ClusterID)
	}
	if q.Severity != "" {
		tx = tx.Where("severity = ?", q.Severity)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Since != "" {
		tx = tx.Where("started_at >= ?", q.Since)
	}
	if q.Until != "" {
		tx = tx.Where("started_at <= ?", q.Until)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	var out []model.FaultEvent
	// open 在前、再按开始时间倒序（最新故障置顶）
	err := tx.Order("status ASC, started_at DESC").
		Limit(q.Limit).Offset(q.Offset).Find(&out).Error
	return out, total, err
}

// Stats 按严重度统计 open 故障数量（侧边栏角标用）。
func (r *FaultEventRepo) Stats() (map[string]int64, error) {
	type row struct {
		Severity string
		Cnt      int64
	}
	var rows []row
	err := r.db.Model(&model.FaultEvent{}).
		Select("severity, COUNT(*) AS cnt").
		Where("status = ?", "open").
		Group("severity").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]int64{"total": 0}
	for _, x := range rows {
		out[x.Severity] = x.Cnt
		out["total"] += x.Cnt
	}
	return out, nil
}

// ListOpenByGPU 取某张卡当前 open 的故障（单卡详情面板用）。
func (r *FaultEventRepo) ListOpenByGPU(uuid string) ([]model.FaultEvent, error) {
	var out []model.FaultEvent
	err := r.db.Where("gpu_uuid = ? AND status = ?", uuid, "open").
		Order("started_at DESC").Find(&out).Error
	return out, err
}

// Resolve 人工销项（把一条 open 事件置 resolved）。
func (r *FaultEventRepo) Resolve(id uint64) error {
	now := time.Now()
	return r.db.Model(&model.FaultEvent{}).Where("id = ? AND status = ?", id, "open").
		Updates(map[string]any{"status": "resolved", "resolved_at": now}).Error
}

// Reconcile 把"本轮各卡正在发生的故障"与库里 open 事件对账：
//   - 新故障 → 插入(started_at=now)
//   - 持续中 → 更新 last_seen / 最新值
//   - 已消失 → 置 resolved（仅对本轮"评估过"的卡，避免失联卡误销项）
//
// current: gpu_uuid -> 该卡本轮确认的故障事件列表（已带拓扑信息，但 started/last_seen 由本方法填）
func (r *FaultEventRepo) Reconcile(now time.Time, current map[string][]model.FaultEvent) error {
	// 本轮评估过的卡集合（只有这些卡的 open 事件才允许被自动 resolve）
	evaluated := make(map[string]bool, len(current))
	for uuid := range current {
		evaluated[uuid] = true
	}

	// 拉出所有 open 事件，按 dedup_key 索引
	var open []model.FaultEvent
	if err := r.db.Where("status = ?", "open").Find(&open).Error; err != nil {
		return err
	}
	openByKey := make(map[string]model.FaultEvent, len(open))
	for _, e := range open {
		openByKey[e.DedupKey] = e
	}

	seen := map[string]bool{}
	for _, evs := range current {
		for _, e := range evs {
			seen[e.DedupKey] = true
			if old, ok := openByKey[e.DedupKey]; ok {
				// 持续中：只更新 last_seen + 最新触发值
				if err := r.db.Model(&model.FaultEvent{}).Where("id = ?", old.ID).
					Updates(map[string]any{
						"last_seen_at":  now,
						"trigger_value": e.TriggerValue,
					}).Error; err != nil {
					return err
				}
			} else {
				// 新故障
				e.Status = "open"
				e.StartedAt = now
				e.LastSeenAt = now
				if err := r.db.Create(&e).Error; err != nil {
					return err
				}
			}
		}
	}

	// 不再出现且属于本轮评估过的卡 → resolved
	for key, old := range openByKey {
		if seen[key] {
			continue
		}
		if !evaluated[old.GPUUUID] {
			continue // 失联/未评估的卡，保留原状
		}
		if err := r.db.Model(&model.FaultEvent{}).Where("id = ?", old.ID).
			Updates(map[string]any{"status": "resolved", "resolved_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}
