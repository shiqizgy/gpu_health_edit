package repository

import (
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// MetricRepo 指标定义仓储
type MetricRepo struct{ db *gorm.DB }

// 详细讲解一下这里，其他的连接数据库的语句类似
// 入参 db *gorm.DB：接收一个指向GORM连接池的指针。注意：这里必须传指针，因为gorm.DB内部包含连接池状态和锁，如果传值（复制），会导致连接池被复制，引发严重的数据竞态和连接泄漏。
// 返回值 *MetricRepo：返回一个指向新创建的 MetricRepo 结构体的指针。通常仓储也是指针传递，因为可能包含缓存或配置状态。
// &MetricRepo{db: db}：这是 Go 的结构体实例化语法。它创建了一个 MetricRepo 对象，并将其内部的 db 字段初始化为传入的 db 指针
// MetricRepo 必须提前定义好才能构造。
// 注意：在 Go 中，凡是持有连接池、锁、或需要变更内部状态的结构体，一律返回指针。
func NewMetricRepo(db *gorm.DB) *MetricRepo { return &MetricRepo{db: db} }

// MetricQuery 指标列表查询条件（分页 + 多条件过滤 + 关键字模糊查找）
type MetricQuery struct {
	Dimension     string // 维度精确匹配
	CardType      string // GPU/NPU
	ValueType     int    // 指标类型精确匹配,0=不过滤
	OwnerSubject  int    // 归属主体,0=不过滤
	HealthPurpose int    // 健康值计算所属,0=不过滤
	Keyword       string // 关键字，模糊匹配  todo 关键字只匹配指标名称
	HealthKeyOnly bool   // 只看参与评分的指标
	Limit         int
	Offset        int
}

// List 按条件分页查询指标，返回当前页数据 + 命中总数。
func (r *MetricRepo) List(q MetricQuery) ([]model.MetricDefinition, int64, error) {
	tx := r.db.Model(&model.MetricDefinition{})
	if q.OwnerSubject != 0 {
		tx = tx.Where("owner_subject_code = ?", q.OwnerSubject)
	}
	if q.HealthPurpose != 0 {
		tx = tx.Where("health_purpose_code = ?", q.HealthPurpose)
	}
	if q.Dimension != "" {
		tx = tx.Where("dimension = ?", q.Dimension)
	}
	if q.CardType != "" {
		tx = tx.Where("card_type = ?", q.CardType)
	}
	if q.ValueType != 0 {
		tx = tx.Where("value_type_code = ?", q.ValueType)
	}
	if q.HealthKeyOnly {
		tx = tx.Where("is_health_key = ?", true)
	}
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("metric_name LIKE ? ", like)
	}

	// 先在过滤条件上数总数（注意要在 Limit/Offset 之前）
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if q.Limit <= 0 {
		q.Limit = 20
	}
	var out []model.MetricDefinition
	err := tx.Order("dimension, id").
		Limit(q.Limit).Offset(q.Offset).
		Find(&out).Error
	return out, total, err
}

func (r *MetricRepo) Get(id uint64) (*model.MetricDefinition, error) {
	var m model.MetricDefinition
	err := r.db.First(&m, id).Error
	return &m, err
}

func (r *MetricRepo) Create(m *model.MetricDefinition) error {
	return r.db.Create(m).Error
}

func (r *MetricRepo) Update(id uint64, m *model.MetricDefinition) error {
	return r.db.Model(&model.MetricDefinition{ID: id}).Updates(map[string]any{
		"seq_no":              m.SeqNo,
		"official_no":         m.OfficialNum,
		"card_type":           m.CardType,
		"metric_name":         m.MetricName,
		"concept":             m.Conception,
		"unit":                m.Unit,
		"dimension":           m.Dimension,
		"owner_subject_code":  m.OwnerSubject,
		"health_purpose_code": m.HealthPurpose,
		"value_type_code":     m.ValueType,
		"work_range":          m.WorkRange,
		"upper_bound":         m.UpperBond,
		"lower_bound":         m.LowerBound,
		"alert_upper":         m.WarnupBound,
		"alert_lower":         m.WarnlowBound,
		"normal_rate":         m.NormalRate,
		"alert_rate":          m.WarnRate,
		"rate_unit":           m.NormalRateUnit,
		"bool_normal":         m.BoolNormal,
		"bool_abnormal":       m.BoolAbnormal,
		"enum_result":         m.EnumResult,
		"enum_score":          m.EnumScore,
		"is_veto":             m.IsVeto,
		"derate_threshold":    m.DerateThreshold,
		"source_ref":          m.SourceRef,
		"vendor":              m.Vendor,
		"remark":              m.Remark,
		"is_health_key":       m.IsHealthKey,
	}).Error
}

func (r *MetricRepo) Delete(id uint64) error {
	return r.db.Delete(&model.MetricDefinition{}, id).Error
}

// ListHealthKeys 返回所有参与评分的指标 key（评分服务用）。
func (r *MetricRepo) ListHealthKeys() ([]model.MetricDefinition, error) {
	var out []model.MetricDefinition
	err := r.db.Where("is_health_key = ?", true).Find(&out).Error
	return out, err
}

// ListAllKeys 返回所有已定义指标的 metric_name 列表
func (r *MetricRepo) ListAllKeys() ([]string, error) {
	var keys []string
	err := r.db.Model(&model.MetricDefinition{}).Pluck("metric_name", &keys).Error
	return keys, err
}

// UpdateHealthKeyByMetricKeys 批量更新某些指标的 is_health_key 状态。
// keys 为空时不执行，避免误操作把全表刷成同一状态。
func (r *MetricRepo) UpdateHealthKeyByMetricKeys(keys []string, enabled bool) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	res := r.db.Model(&model.MetricDefinition{}).
		Where("metric_name IN ? AND is_health_key <> ?", keys, enabled).
		Update("is_health_key", enabled)
	return res.RowsAffected, res.Error
}

//故障检测/异常指标展示使用：
//AllDefsMap 返回 metric_key -> 指标定义 的全量映射（故障检测/异常指标展示用）

func (r *MetricRepo) AllDefsMap() (map[string]model.MetricDefinition, error) {
	var rows []model.MetricDefinition
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]model.MetricDefinition, len(rows))
	for _, d := range rows {
		m[d.MetricName] = d
	}
	return m, nil
}

func (r *MetricRepo) DB() *gorm.DB { return r.db }

// 更新 is_alive 字段
func (r *MetricRepo) UpdateAliveByMetricKeys(keys []string, alive bool) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	res := r.db.Model(&model.MetricDefinition{}).
		Where("metric_name IN ? AND is_alive <> ?", keys, alive).
		Update("is_alive", alive)
	return res.RowsAffected, res.Error
}
