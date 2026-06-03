package repository

import (
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// MetricRepo 指标定义仓储
type MetricRepo struct{ db *gorm.DB }

func NewMetricRepo(db *gorm.DB) *MetricRepo { return &MetricRepo{db: db} }

func (r *MetricRepo) List(dimension, deviceType string, healthKeyOnly bool) ([]model.MetricDefinition, error) {
	q := r.db.Model(&model.MetricDefinition{})
	if dimension != "" {
		q = q.Where("dimension = ?", dimension)
	}
	if deviceType != "" {
		q = q.Where("device_type = ?", deviceType)
	}
	if healthKeyOnly {
		q = q.Where("is_health_key = ?", true)
	}
	var out []model.MetricDefinition
	err := q.Order("dimension, id").Find(&out).Error
	return out, err
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
	// 用 map 显式指定可更新字段，避免 GORM 对零值字段跳过更新
	return r.db.Model(&model.MetricDefinition{ID: id}).Updates(map[string]any{
		"display_name":   m.DisplayName,
		"unit":           m.Unit,
		"metric_type":    m.MetricType,
		"dimension":      m.Dimension,
		"concept":        m.Concept,
		"device_type":    m.DeviceType,
		"normal_range":   m.NormalRange,
		"abnormal_range": m.AbnormalRange,
		"remark":         m.Remark,
		"is_health_key":  m.IsHealthKey,
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
