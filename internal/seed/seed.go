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
	db.Exec("SET FOREIGN_KEY_CHECKS=0")
	defer db.Exec("SET FOREIGN_KEY_CHECKS=1")

	tables := append([]string{
		"ai_message", "ai_conversation",
		"fault_event", "fault_rule", "fault_knowledge",
		"gpu_health_snapshot", "cluster_health_summary",
		"gpu_card", "node", "cluster",
		"scoring_strategy", "strategy_metric_rule",
	}, cfg.DropTables...)

	for _, t := range tables {
		if err := db.Exec("DROP TABLE IF EXISTS " + t).Error; err != nil {
			return fmt.Errorf("DROP 旧表 %s 失败: %w", t, err)
		}
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
