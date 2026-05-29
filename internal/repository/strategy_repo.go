package repository

import (
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

// StrategyRepo 评分策略仓储
type StrategyRepo struct{ db *gorm.DB }

func NewStrategyRepo(db *gorm.DB) *StrategyRepo { return &StrategyRepo{db: db} }

// List 列出所有策略（含规则）
func (r *StrategyRepo) List() ([]model.ScoringStrategy, error) {
	var out []model.ScoringStrategy
	err := r.db.Preload("Rules").Order("id").Find(&out).Error
	return out, err
}

// GetByCode 按代码取策略（含规则），评分服务用
func (r *StrategyRepo) GetByCode(code string) (*model.ScoringStrategy, error) {
	var s model.ScoringStrategy
	err := r.db.Preload("Rules").Where("code = ?", code).First(&s).Error
	return &s, err
}

// GetByID 按 ID 取策略
func (r *StrategyRepo) GetByID(id uint64) (*model.ScoringStrategy, error) {
	var s model.ScoringStrategy
	err := r.db.Preload("Rules").First(&s, id).Error
	return &s, err
}

// GetDefault 取默认策略
func (r *StrategyRepo) GetDefault() (*model.ScoringStrategy, error) {
	var s model.ScoringStrategy
	err := r.db.Preload("Rules").Where("is_default = ?", true).First(&s).Error
	return &s, err
}

// Create 新建策略（含规则），用事务保证一致性
func (r *StrategyRepo) Create(s *model.ScoringStrategy) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 若 rules 为空,自动复制默认策略的规则作为起点
		if len(s.Rules) == 0 {
			var defaultStrategy model.ScoringStrategy
			if err := tx.Preload("Rules").Where("is_default = ?", true).First(&defaultStrategy).Error; err == nil {
				for _, r := range defaultStrategy.Rules {
					//确保curve_params不为空字符串
					if r.CurveParams == "" {
						r.CurveParams = "null" // 或者使用sql.NullString
					}
					s.Rules = append(s.Rules, model.StrategyMetricRule{
						MetricKey:     r.MetricKey,
						Weight:        r.Weight,
						CurveType:     r.CurveType,
						CurveParams:   r.CurveParams,
						IsVeto:        r.IsVeto,
						VetoThreshold: r.VetoThreshold,
					})
				}
			}
		} else {
			//处理传入的规则
			for i := range s.Rules {
				if s.Rules[i].CurveParams == "" {
					s.Rules[i].CurveParams = "null" // 或者使用sql.NullString
				}
			}
		}
		return tx.Create(s).Error
	})
}

// UpdateMeta 更新策略基本信息 + 维度权重，并把 version+1 触发评分服务热加载
func (r *StrategyRepo) UpdateMeta(id uint64, name, desc, dimWeights string) error {
	return r.db.Model(&model.ScoringStrategy{ID: id}).Updates(map[string]any{
		"name":              name,
		"description":       desc,
		"dimension_weights": dimWeights,
		"version":           gorm.Expr("version + 1"),
	}).Error
}

// ReplaceRules 全量替换某策略的指标规则（前端编辑权重后保存），事务处理
func (r *StrategyRepo) ReplaceRules(strategyID uint64, rules []model.StrategyMetricRule) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("strategy_id = ?", strategyID).Delete(&model.StrategyMetricRule{}).Error; err != nil {
			return err
		}
		for i := range rules {
			rules[i].StrategyID = strategyID
			// 修复：确保curve_params不为空字符串
			if rules[i].CurveParams == "" {
				rules[i].CurveParams = "null" // 或者使用sql.NullString
			}
		}
		if len(rules) > 0 {
			if err := tx.Create(&rules).Error; err != nil {
				return err
			}
		}
		// 规则变了也要 version+1
		return tx.Model(&model.ScoringStrategy{ID: strategyID}).
			Update("version", gorm.Expr("version + 1")).Error
	})
}

func (r *StrategyRepo) Delete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("strategy_id = ?", id).Delete(&model.StrategyMetricRule{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.ScoringStrategy{}, id).Error
	})
}
