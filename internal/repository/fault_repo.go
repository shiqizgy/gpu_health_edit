package repository

import (
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// FaultRepo 故障知识图谱仓储（纯 CRUD，不参与计算）
type FaultRepo struct{ db *gorm.DB }

func NewFaultRepo(db *gorm.DB) *FaultRepo { return &FaultRepo{db: db} }

func (r *FaultRepo) List(faultType, keyword string, limit, offset int) ([]model.FaultKnowledge, int64, error) {
	q := r.db.Model(&model.FaultKnowledge{})
	if faultType != "" {
		q = q.Where("fault_type = ?", faultType)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("fault_type LIKE ? OR symptom LIKE ? OR possible_cause LIKE ?", like, like, like)
	}
	var total int64
	q.Count(&total)
	var out []model.FaultKnowledge
	err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, total, err
}

func (r *FaultRepo) Get(id uint64) (*model.FaultKnowledge, error) {
	var f model.FaultKnowledge
	err := r.db.First(&f, id).Error
	return &f, err
}

func (r *FaultRepo) Create(f *model.FaultKnowledge) error { return r.db.Create(f).Error }

func (r *FaultRepo) Update(id uint64, f *model.FaultKnowledge) error {
	return r.db.Model(&model.FaultKnowledge{ID: id}).Updates(map[string]any{
		"fault_type":      f.FaultType,
		"xid_code":        f.XIDCode,
		"symptom":         f.Symptom,
		"possible_cause":  f.PossibleCause,
		"metric_changes":  f.MetricChanges,
		"related_metrics": f.RelatedMetrics,
		"severity":        f.Severity,
		"suggestion":      f.Suggestion,
		"reference":       f.Reference,
	}).Error
}

func (r *FaultRepo) Delete(id uint64) error {
	return r.db.Delete(&model.FaultKnowledge{}, id).Error
}
