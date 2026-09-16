package repository

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HealthRepo 健康快照 + 集群汇总仓储
type HealthRepo struct{ db *gorm.DB }

func NewHealthRepo(db *gorm.DB) *HealthRepo { return &HealthRepo{db: db} }

// UpsertSnapshot 覆盖写单卡最新快照（评分服务每分钟调用）
func (r *HealthRepo) UpsertSnapshot(s *model.GPUHealthSnapshot) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "gpu_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"cluster_id", "strategy_id", "score", "level", "veto", "veto_reason", "breakdown", "scored_at"}),
	}).Create(s).Error
}

// BatchUpsertSnapshots 批量覆盖写（2000 卡用批量，减少往返）
func (r *HealthRepo) BatchUpsertSnapshots(snaps []model.GPUHealthSnapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "gpu_uuid"}},
			DoUpdates: clause.AssignmentColumns([]string{"cluster_id", "strategy_id", "score", "level", "veto", "veto_reason", "breakdown", "scored_at"}),
		}).CreateInBatches(snaps, 500).Error
		if err == nil || !(strings.Contains(err.Error(), "1213") || strings.Contains(err.Error(), "1205")) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
	}
	return err
}

// BatchUpsertSnapshotsConcurrent 把快照切成 shards 份并发 upsert。
// 受 MySQL max_open 限制，shards 建议 <= max_open 的一半（默认 8）。
func (r *HealthRepo) BatchUpsertSnapshotsConcurrent(snaps []model.GPUHealthSnapshot, shards int) error {
	if len(snaps) == 0 {
		return nil
	}
	if shards <= 1 || len(snaps) <= 1000 {
		return r.BatchUpsertSnapshots(snaps) // 量小直接走串行
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].GPUUUID < snaps[j].GPUUUID })
	size := (len(snaps) + shards - 1) / shards
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for start := 0; start < len(snaps); start += size {
		end := start + size
		if end > len(snaps) {
			end = len(snaps)
		}
		chunk := snaps[start:end]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.BatchUpsertSnapshots(chunk); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// GetSnapshot 取单卡快照（详情页）
func (r *HealthRepo) GetSnapshot(uuid string) (*model.GPUHealthSnapshot, error) {
	var s model.GPUHealthSnapshot
	err := r.db.Where("gpu_uuid = ?", uuid).First(&s).Error
	return &s, err
}

type SnapshotWithBinding struct {
	model.GPUHealthSnapshot
	BoundStrategyID *uint64 `gorm:"column:bound_strategy_id" json:"bound_strategy_id"`
}

// ListSnapshotsByCluster 列集群内单卡快照，按分降序（坏卡置顶），分页
func (r *HealthRepo) ListSnapshotsByCluster(clusterID uint64, level string, limit, offset int) ([]SnapshotWithBinding, int64, error) {
	var out []SnapshotWithBinding
	var total int64
	q := r.db.Table("gpu_health_snapshot AS s").
		Joins("JOIN gpu_card g ON g.uuid = s.gpu_uuid AND g.status = 'online'").
		Where("g.cluster_id = ?", clusterID)
	if level != "" {
		q = q.Where("s.level = ?", level)
	}
	base := q.Session(&gorm.Session{})
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := base.Select("s.*, g.strategy_id AS bound_strategy_id").
		Order("s.level = 'unknown' ASC, s.score ASC, s.gpu_uuid ASC"). // "全部"视图下 unknown 排到最后，不再压在故障卡上面
		Limit(limit).Offset(offset).Scan(&out).Error
	return out, total, err
}

// ListRiskiest 全局风险最高的 N 张卡（健康大盘用）
func (r *HealthRepo) ListRiskiest(limit int) ([]model.GPUHealthSnapshot, error) {
	var out []model.GPUHealthSnapshot
	err := r.db.Table("gpu_health_snapshot AS s").
		Select("s.id, s.gpu_uuid, g.cluster_id, s.strategy_id, s.score, s.level, s.veto, s.veto_reason, s.scored_at").
		Joins("JOIN gpu_card g ON g.uuid = s.gpu_uuid AND g.status = 'online'").
		Where("s.level <> ?", "unknown").
		Order("s.score ASC").Limit(limit).Scan(&out).Error
	return out, err
}

type GlobalStats struct {
	Total     int64
	AvgScore  float64
	Levels    map[string]int64
	UpdatedAt *time.Time
}

func (r *HealthRepo) GlobalStats() (*GlobalStats, error) {
	var a struct {
		Total, Healthy, SubHealthy, Warning, Critical, Failed, Unknown int64
		ScoreSum                                                       float64
		UpdatedAt                                                      *time.Time
	}
	err := r.db.Model(&model.ClusterHealthSummary{}).Select(`
        COALESCE(SUM(total_gpu),0)       AS total,
        COALESCE(SUM(healthy_cnt),0)     AS healthy,
        COALESCE(SUM(sub_healthy_cnt),0) AS sub_healthy,
        COALESCE(SUM(warning_cnt),0)     AS warning,
        COALESCE(SUM(critical_cnt),0)    AS critical,
        COALESCE(SUM(failed_cnt),0)      AS failed,
        COALESCE(SUM(unknown_cnt),0)     AS unknown,
        COALESCE(SUM(avg_score*(total_gpu-unknown_cnt)),0) AS score_sum,
        MAX(updated_at)                  AS updated_at`).Scan(&a).Error
	if err != nil {
		return nil, err // 不再吞错误：前端据此保留旧数据，而不是显示 0
	}
	s := &GlobalStats{Total: a.Total, UpdatedAt: a.UpdatedAt, Levels: map[string]int64{
		"healthy": a.Healthy, "sub_healthy": a.SubHealthy, "warning": a.Warning,
		"critical": a.Critical, "failed": a.Failed, "unknown": a.Unknown,
	}}
	if n := a.Total - a.Unknown; n > 0 {
		s.AvgScore = a.ScoreSum / float64(n)
	}
	return s, nil
}

// ---- 集群汇总（预聚合）----

// UpsertClusterSummary 覆盖写集群汇总
func (r *HealthRepo) UpsertClusterSummary(s *model.ClusterHealthSummary) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cluster_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"cluster_code", "cluster_name", "total_gpu", "avg_score", "healthy_cnt", "sub_healthy_cnt", "warning_cnt", "critical_cnt", "failed_cnt", "bound_strategy_id", "updated_at", "unknown_cnt"}),
	}).Create(s).Error
}

// ListClusterSummaries 列出所有集群汇总（集群表格页，毫秒级）
func (r *HealthRepo) ListClusterSummaries() ([]model.ClusterHealthSummary, error) {
	var out []model.ClusterHealthSummary
	err := r.db.Order("avg_score ASC").Find(&out).Error
	return out, err
}

// RecomputeClusterSummaries 从单卡快照重新聚合所有集群汇总（评分服务每轮末尾调用）
//
// 设计说明：用一条 SQL 在数据库侧完成聚合，避免把几千行拉到内存。
// 这是万卡场景下集群表格毫秒响应的关键。
// RecomputeClusterSummaries 从单卡快照重新聚合所有集群汇总（评分服务每轮末尾调用）
func (r *HealthRepo) RecomputeClusterSummaries() error {
	type aggRow struct {
		ClusterID       uint64
		ClusterCode     string
		ClusterName     string
		TotalGPU        int
		AvgScore        float64
		HealthyCnt      int
		SubHealthyCnt   int
		WarningCnt      int
		CriticalCnt     int
		FailedCnt       int
		UnknownCnt      int
		BoundStrategyID *uint64
	}
	var rows []aggRow
	err := r.db.Table("gpu_health_snapshot AS s").
		Select(`g.cluster_id AS cluster_id,
        c.code AS cluster_code, c.name AS cluster_name,
        COUNT(*) AS total_gpu,
        SUM(CASE WHEN s.level='healthy'     THEN 1 ELSE 0 END) AS healthy_cnt,
        SUM(CASE WHEN s.level='sub_healthy' THEN 1 ELSE 0 END) AS sub_healthy_cnt,
        SUM(CASE WHEN s.level='warning'     THEN 1 ELSE 0 END) AS warning_cnt,
        SUM(CASE WHEN s.level='critical'    THEN 1 ELSE 0 END) AS critical_cnt,
        SUM(CASE WHEN s.level='failed'      THEN 1 ELSE 0 END) AS failed_cnt,
        SUM(CASE WHEN s.level='unknown'     THEN 1 ELSE 0 END) AS unknown_cnt,
        COALESCE(AVG(CASE WHEN s.level<>'unknown' THEN s.score END), 0) AS avg_score,
        c.strategy_id AS bound_strategy_id`).
		Joins("JOIN gpu_card g ON g.uuid = s.gpu_uuid AND g.status = 'online'").
		Joins("JOIN cluster c ON c.id = g.cluster_id").
		Group("g.cluster_id, c.code, c.name, c.strategy_id").
		Scan(&rows).Error
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		return nil
	}

	// ===== 改动点：一次批量写入替代逐个循环 =====
	now := time.Now()
	summaries := make([]model.ClusterHealthSummary, 0, len(rows))
	for _, x := range rows {
		summaries = append(summaries, model.ClusterHealthSummary{
			ClusterID:       x.ClusterID,
			ClusterCode:     x.ClusterCode,
			ClusterName:     x.ClusterName,
			TotalGPU:        x.TotalGPU,
			AvgScore:        x.AvgScore,
			HealthyCnt:      x.HealthyCnt,
			SubHealthyCnt:   x.SubHealthyCnt,
			WarningCnt:      x.WarningCnt,
			CriticalCnt:     x.CriticalCnt,
			FailedCnt:       x.FailedCnt,
			UnknownCnt:      x.UnknownCnt,
			BoundStrategyID: x.BoundStrategyID,
			UpdatedAt:       now,
		})
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "cluster_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"cluster_code", "cluster_name", "total_gpu", "avg_score", "healthy_cnt", "sub_healthy_cnt", "warning_cnt", "critical_cnt", "failed_cnt", "bound_strategy_id", "updated_at", "unknown_cnt"}),
		}).CreateInBatches(summaries, 100).Error; err != nil {
			return err
		}
		ids := make([]uint64, 0, len(summaries))
		for _, s := range summaries {
			ids = append(ids, s.ClusterID)
		}
		return tx.Where("cluster_id NOT IN ?", ids).Delete(&model.ClusterHealthSummary{}).Error
	})
}

func (r *HealthRepo) SearchSnapshots(keyword string, limit int) ([]SnapshotWithBinding, error) {
	var out []SnapshotWithBinding
	q := "%" + keyword + "%"
	err := r.db.Table("gpu_health_snapshot AS s").
		Select("s.*, g.strategy_id AS bound_strategy_id").
		Joins("LEFT JOIN gpu_card g ON g.uuid = s.gpu_uuid").
		Where("s.gpu_uuid LIKE ?", q).
		Order("s.score ASC").Limit(limit).Scan(&out).Error
	return out, err
}
