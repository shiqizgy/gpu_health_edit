package repository

import (
	"time"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB 创建 GORM 数据库连接，按需自动迁移所有表结构。
func NewDB(cfg config.MySQLConfig, debug bool) (*gorm.DB, error) {
	gormCfg := &gorm.Config{}
	if debug {
		gormCfg.Logger = logger.Default.LogMode(logger.Info)
	} else {
		gormCfg.Logger = logger.Default.LogMode(logger.Warn)
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN), gormCfg)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if cfg.AutoMigrate {
		if err := AutoMigrate(db); err != nil {
			return nil, err
		}
	}
	return db, nil
}

// AutoMigrate 迁移全部模型。新增表只需在此追加。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.MetricDefinition{},
		&model.ScoringStrategy{},
		&model.StrategyMetricRule{},
		&model.Cluster{},
		&model.Node{},
		&model.GPUCard{},
		&model.GPUHealthSnapshot{},
		&model.ClusterHealthSummary{},
		&model.FaultKnowledge{},
		&model.FaultRule{},
		&model.FaultEvent{},
		&model.AIConversation{},
		&model.AIMessage{},
	)
}
