package seed

import (
	_ "embed"
	"fmt"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/pkg/logger"
	"gorm.io/gorm"
)

//go:embed sql/accel_metric_scoring.sql
var sqlMetrics string

//go:embed sql/003_score_scope.sql
var sqlScope string

//go:embed sql/seed_strategy.sql
var sqlStrategy string

//go:embed sql/004_fault_rule.sql
var sqlFaultRule string

// Reset 删除全部业务表 + 配置的旧版遗留表。
// 必须在 AutoMigrate 之前调用：旧表结构与新模型冲突时，AutoMigrate 无法在旧表上改索引。
func Reset(db *gorm.DB, cfg config.SeedConfig) error {
	tables := append([]string{
		"ai_message", "ai_conversation",
		"fault_event", "fault_rule", "fault_knowledge",
		"gpu_health_snapshot", "cluster_health_summary",
		"gpu_card", "node", "cluster",
		"scoring_strategy", "strategy_metric_rule",
	}, cfg.DropTables...)

	// 事务保证 SET 和 DROP 跑在同一条连接上（连接池下会话级变量才可靠）
	err := db.Transaction(func(tx *gorm.DB) error {
		tx.Exec("SET FOREIGN_KEY_CHECKS=0")
		for _, t := range tables {
			if err := tx.Exec("DROP TABLE IF EXISTS " + t).Error; err != nil {
				return fmt.Errorf("DROP 旧表 %s 失败: %w", t, err)
			}
		}
		tx.Exec("SET FOREIGN_KEY_CHECKS=1")
		return nil
	})
	if err != nil {
		return err
	}
	logger.L.Infof("已 DROP %d 张旧表，将由 AutoMigrate 重建", len(tables))
	return nil
}

// Run 灌入种子数据（在 AutoMigrate 建表之后调用）。
func Run(db *gorm.DB, cfg config.SeedConfig) error {
	scripts := []struct{ name, body string }{
		{"accel_metric_scoring.sql", sqlMetrics},
		{"003_score_scope.sql", sqlScope},
		{"seed_strategy.sql", sqlStrategy},
		{"004_fault_rule.sql", sqlFaultRule},
	}
	for _, s := range scripts {
		if err := db.Exec(s.body).Error; err != nil {
			return fmt.Errorf("执行 %s 失败: %w", s.name, err)
		}
		logger.L.Infof("种子脚本 %s 执行完成", s.name)
	}
	return nil
}

// NeedInit 判断是否需要初始化：核心表不存在 或 策略表为空 视为首次部署。
// 用于防止 Pod 正常重启时误触发 Reset 清库。
func NeedInit(db *gorm.DB) bool {
	if !db.Migrator().HasTable("scoring_strategy") {
		return true // 核心表都没有，肯定是首次
	}
	var cnt int64
	if err := db.Table("scoring_strategy").Count(&cnt).Error; err != nil {
		return true
	}
	return cnt == 0 // 表在但无策略数据，视为需重灌
}
