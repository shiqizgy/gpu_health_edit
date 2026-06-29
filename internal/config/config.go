package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	App       App             `yaml:"app"`
	Server    ServerConfig    `yaml:"server"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	Scorer    ScorerConfig    `yaml:"scorer"`
	Simulator SimulatorConfig `yaml:"simulator"`
	Assistant AssistantConfig `yaml:"assistant"`
	CK        CKConfig        `yaml:"ck"`
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
	Cron           string            `yaml:"cron"`            // 定时表达式，每分钟评分
	StrategyCode   string            `yaml:"strategy_code"`   // 默认使用的策略代码
	VendorStrategy map[string]string `yaml:"vendor_strategy"` // vendor指向评分策略
	Workers        int               `yaml:"workers"`         //评分协程池大小，<=0取CPU核数
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
	MetricTTL int    `yaml:"metric_ttl"` // 写Redis的TTL秒
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
