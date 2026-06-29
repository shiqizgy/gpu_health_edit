package repository

import (
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
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "gpu_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"cluster_id", "strategy_id", "score", "level", "veto", "veto_reason", "breakdown", "scored_at"}),
	}).CreateInBatches(snaps, 200).Error
}

// BatchUpsertSnapshotsConcurrent 把快照切成 shards 份并发 upsert。
// 受 MySQL max_open 限制，shards 建议 <= max_open 的一半（默认 8）。
func (r *HealthRepo) BatchUpsertSnapshotsConcurrent(snaps []model.GPUHealthSnapshot, shards int) error {
	if len(snaps) == 0 {
		return nil
	}
	if shards <= 1 || len(snaps) <= 200 {
		return r.BatchUpsertSnapshots(snaps) // 量小直接走串行
	}
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
func (r *HealthRepo) ListSnapshotsByCluster(clusterID uint64, limit, offset int) ([]SnapshotWithBinding, int64, error) {
	var out []SnapshotWithBinding
	var total int64
	r.db.Model(&model.GPUHealthSnapshot{}).Where("cluster_id = ?", clusterID).Count(&total)
	err := r.db.Table("gpu_health_snapshot AS s").
		Select("s.*, g.strategy_id AS bound_strategy_id").
		Joins("LEFT JOIN gpu_card g ON g.uuid = s.gpu_uuid").
		Where("s.cluster_id = ?", clusterID).
		Order("s.score ASC").Limit(limit).Offset(offset).
		Scan(&out).Error
	return out, total, err
}

// ListRiskiest 全局风险最高的 N 张卡（健康大盘用）
func (r *HealthRepo) ListRiskiest(limit int) ([]model.GPUHealthSnapshot, error) {
	var out []model.GPUHealthSnapshot
	err := r.db.Order("score ASC").Limit(limit).Find(&out).Error
	return out, err
}

// GlobalStats 全局统计（健康大盘：总数/平均分/各等级数）
type GlobalStats struct {
	Total    int64
	AvgScore float64
	Levels   map[string]int64
}

func (r *HealthRepo) GlobalStats() (*GlobalStats, error) {
	stats := &GlobalStats{Levels: map[string]int64{}}
	r.db.Model(&model.GPUHealthSnapshot{}).Count(&stats.Total)

	var avg *float64
	r.db.Model(&model.GPUHealthSnapshot{}).Select("AVG(score)").Scan(&avg)
	if avg != nil {
		stats.AvgScore = *avg
	}

	type row struct {
		Level string
		Cnt   int64
	}
	var rows []row
	r.db.Model(&model.GPUHealthSnapshot{}).
		Select("level, COUNT(*) as cnt").Group("level").Scan(&rows)
	for _, x := range rows {
		stats.Levels[x.Level] = x.Cnt
	}
	return stats, nil
}

// ---- 集群汇总（预聚合）----

// UpsertClusterSummary 覆盖写集群汇总
func (r *HealthRepo) UpsertClusterSummary(s *model.ClusterHealthSummary) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cluster_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"cluster_code", "cluster_name", "total_gpu", "avg_score", "healthy_cnt", "sub_healthy_cnt", "warning_cnt", "critical_cnt", "failed_cnt", "bound_strategy_id", "updated_at"}),
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
		BoundStrategyID *uint64
	}
	var rows []aggRow
	// 修改SQL查询，添加策略ID信息
	err := r.db.Table("gpu_health_snapshot AS s").
		Select(`s.cluster_id,
			c.code AS cluster_code, c.name AS cluster_name,
			COUNT(*) AS total_gpu,
			AVG(s.score) AS avg_score,
			SUM(CASE WHEN s.level='healthy' THEN 1 ELSE 0 END) AS healthy_cnt,
			SUM(CASE WHEN s.level='sub_healthy' THEN 1 ELSE 0 END) AS sub_healthy_cnt,
			SUM(CASE WHEN s.level='warning' THEN 1 ELSE 0 END) AS warning_cnt,
			SUM(CASE WHEN s.level='critical' THEN 1 ELSE 0 END) AS critical_cnt,
			SUM(CASE WHEN s.level='failed' THEN 1 ELSE 0 END) AS failed_cnt,
			c.strategy_id AS bound_strategy_id`).
		Joins("JOIN cluster c ON c.id = s.cluster_id").
		Group("s.cluster_id, c.code, c.name, c.strategy_id").
		Scan(&rows).Error
	if err != nil {
		return err
	}

	now := time.Now()
	for _, x := range rows {
		summary := &model.ClusterHealthSummary{
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
			BoundStrategyID: x.BoundStrategyID,
			UpdatedAt:       now,
		}
		if err := r.UpsertClusterSummary(summary); err != nil {
			return err
		}
	}
	return nil
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
