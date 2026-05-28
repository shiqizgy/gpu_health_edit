package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	Scorer    ScorerConfig    `yaml:"scorer"`
	Simulator SimulatorConfig `yaml:"simulator"`
	Assistant AssistantConfig `yaml:"assistant"`
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
	Cron         string `yaml:"cron"`          // 定时表达式，每分钟评分
	StrategyCode string `yaml:"strategy_code"` // 默认使用的策略代码
}

// SimulatorConfig 仿真服务配置
type SimulatorConfig struct {
	Cron        string  `yaml:"cron"`         // 定时表达式，每分钟生成
	GPUCount    int     `yaml:"gpu_count"`    // 仿真 GPU 数量
	GPUPerNode  int     `yaml:"gpu_per_node"` // 每节点 GPU 数
	AnomalyRate float64 `yaml:"anomaly_rate"` // 异常卡比例
	MetricTTL   int     `yaml:"metric_ttl"`   // Redis 指标过期秒数
}

type AssistantConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	TimeoutSec int    `yaml:"timeout_sec"`
	MaxHistory int    `yaml:"max_history"`
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
