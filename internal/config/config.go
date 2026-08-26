package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	App       App              `yaml:"app"`
	Server    ServerConfig     `yaml:"server"`
	MySQL     MySQLConfig      `yaml:"mysql"`
	Redis     RedisConfig      `yaml:"redis"`
	Scorer    ScorerConfig     `yaml:"scorer"`
	Simulator SimulatorConfig  `yaml:"simulator"`
	Assistant AssistantConfig  `yaml:"assistant"`
	CK        CKConfig         `yaml:"ck"`
	Retention RententionConfig `yaml:"retention"` //数据保留时间配置
	Seed      SeedConfig       `yaml:"seed"`      // 新增,灌入种子数据需要
}

type SeedConfig struct {
	Enabled    bool     `yaml:"enabled"`     // 是否启动时灌入种子数据
	Reset      bool     `yaml:"reset"`       // 灌入前清空业务表数据
	DropTables []string `yaml:"drop_tables"` // 旧版遗留表名，启动时直接 DROP
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	Mode string `yaml:"mode"` // debug / release
}

type MySQLConfig struct {
	DSN         string `yaml:"dsn"`
	MaxOpen     int    `yaml:"max_open"`
	MaxIdle     int    `yaml:"max_idle"`
	AutoMigrate bool   `yaml:"auto_migrate"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// ScorerConfig 评分服务配置
type ScorerConfig struct {
	Cron             string            `yaml:"cron"`               // 定时表达式，每分钟评分
	StrategyCode     string            `yaml:"strategy_code"`      // 默认使用的策略代码
	CardTypeStrategy map[string]string `yaml:"card_type_strategy"` // vendor指向评分策略
	Workers          int               `yaml:"workers"`            //评分协程池大小，<=0取CPU核数
}

// SimulatorConfig 仿真服务配置
type SimulatorConfig struct {
	Clusters        []string `yaml:"clusters"` // = CK 的 source
	NodesPerCluster int      `yaml:"nodes_per_cluster"`
	GPUsPerNode     int      `yaml:"gpus_per_node"`
	NodeGroup       string   `yaml:"node_group"` // = CK 的 gpu_node_group
	FaultRate       float64  `yaml:"fault_rate"` // 每卡每轮起新故障概率
	Cron            string   `yaml:"cron"`
}

type AssistantConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	TimeoutSec int    `yaml:"timeout_sec"`
	MaxHistory int    `yaml:"max_history"`
}

type CKConfig struct {
	Addr      string `yaml:"addr"` //host:9000 native 端口
	Database  string `yaml:"database"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Table     string `yaml:"table"`
	Cron      string `yaml:"cron"`
	WindowSec int    `yaml:"window_sec"` // 取最近多长时间的数据，单位秒

	//测试的配置，后续优化调整
	Source   string `yaml:"source"`    // 新增：只采集该 source（集群），空=全部
	GPULimit int    `yaml:"gpu_limit"` // 新增：随机抽样卡数上限，0=不限
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = "configs/config.yaml"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

type App struct {
	Roles []string `yaml:"roles"` // 可选: api / loader / scorer
}

func (a App) Has(role string) bool {
	if len(a.Roles) == 0 {
		return true
	}
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// RetentionConfig数据保留清理配置
type RententionConfig struct {
	Enabled    bool   `yaml:"enabled"`     //是否开启定时清理
	Cron       string `yaml:"cron"`        //执行时机，默认每天凌晨3点
	RetainDays int    `yaml:"retain_days"` //保留天数，默认3
	BatchSize  int    `yaml:"batch_size"`  //单批删除行数，默认1000
}
