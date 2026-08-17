package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Name    string   `mapstructure:"name"`
	Mode    string   `mapstructure:"mode"`
	Port    int      `mapstructure:"port"`
	APIKey  string   `mapstructure:"api_key"`
	APIKeys []string `mapstructure:"api_keys"`
}

type DatabaseConfig struct {
	Driver       string `mapstructure:"driver"`
	DSN          string `mapstructure:"dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type MaxMindConfig struct {
	Country string `mapstructure:"country"`
	City    string `mapstructure:"city"`
	ASN     string `mapstructure:"asn"`
}

type IPDBConfig struct {
	IP2Location     string        `mapstructure:"ip2location"`
	IP2Proxy        string        `mapstructure:"ip2proxy"`
	IP2ProxyIPv6CSV string        `mapstructure:"ip2proxy_ipv6_csv"`
	MaxMind         MaxMindConfig `mapstructure:"maxmind"`
}

type RateLimitConfig struct {
	Window      int `mapstructure:"window"`
	MaxRequests int `mapstructure:"max_requests"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
	Path  string `mapstructure:"path"`
}

type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	IPDB      IPDBConfig      `mapstructure:"ipdb"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Log       LogConfig       `mapstructure:"log"`
}

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	v.SetEnvPrefix("RISK")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config failed: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}

	if cfg.App.Port == 0 {
		cfg.App.Port = 8080
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.RateLimit.Window == 0 {
		cfg.RateLimit.Window = 60
	}
	if cfg.RateLimit.MaxRequests == 0 {
		cfg.RateLimit.MaxRequests = 100
	}

	return &cfg, nil
}
