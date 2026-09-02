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
// 方法在输出的过程中，已经把连接池大小、日志策略和表结构都安排好了。
func NewDB(cfg config.MySQLConfig, debug bool) (*gorm.DB, error) {
	//开发环境主要看日志，生产环境看报出的异常
	gormCfg := &gorm.Config{}
	if debug {
		gormCfg.Logger = logger.Default.LogMode(logger.Info)
	} else {
		gormCfg.Logger = logger.Default.LogMode(logger.Warn)
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN), gormCfg) //建立 GORM 的 ORM 会话层
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB() //从 GORM 中取出底层的 *sql.DB 标准库连接池对象
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpen)         //最大打开连接数（默认无限）。必须设置，否则突发流量会无限创建连接，打爆 MySQL 的 max_connections 限制。
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)         //最大空闲连接数。如果设置得太小，高并发下会频繁创建和销毁连接，增加延迟；如果设置得太大，会占用 MySQL 资源。
	sqlDB.SetConnMaxLifetime(30 * time.Minute) //硬编码 30 分钟：连接的最大存活时间。这是为了防止 MySQL 服务端主动断开空闲连接导致客户端报错 bad connection。建议此值略小于 MySQL 服务端的超时时间。

	//如果配置开关打开，程序启动时会自动根据项目的 Model 结构体去数据库增删改表结构（如新增列、修改字段类型）
	//开发环境开启，方便本地调试；测试/预发布环境手动执行 SQL；生产环境绝对关闭，由 DBA 通过专业的 pt-online-schema-change 等工具进行变更
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
		&model.KGNode{}, // 知识图谱节点
		&model.KGEdge{}, // 知识图谱边
	)
}
