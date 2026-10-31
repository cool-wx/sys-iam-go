// Package config 定义服务配置结构与加载链：YAML 文件 + IAM_ 前缀环境变量覆盖
// （koanf，D-06/D-07）。TransformFunc 三步链：TrimPrefix IAM_ → ToLower → 下划线转点。
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// 默认值（配置文件缺失/键缺省时使用，工控机取值见研究 P14）。
const (
	DefaultPort            = 8080
	DefaultDSN             = "root:root123@tcp(127.0.0.1:3306)/sys_iam?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
	DefaultMaxOpenConns    = 20
	DefaultMaxIdleConns    = 5
	DefaultConnMaxLifetime = 30 * time.Minute
	DefaultSnowflakeNode   = 1
	DefaultLogLevel        = "info"
)

// Config 服务配置聚合。
type Config struct {
	Server    ServerConfig    `json:"server"`
	MySQL     MySQLConfig     `json:"mysql"`
	Snowflake SnowflakeConfig `json:"snowflake"`
	Log       LogConfig       `json:"log"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Port int `json:"port"`
}

// MySQLConfig MySQL 连接配置。
type MySQLConfig struct {
	DSN             string        `json:"dsn"`
	MaxOpenConns    int           `json:"maxOpenConns"`
	MaxIdleConns    int           `json:"maxIdleConns"`
	ConnMaxLifetime time.Duration `json:"connMaxLifetime"`
}

// SnowflakeConfig 雪花 ID 节点配置。
type SnowflakeConfig struct {
	Node int `json:"node"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level string `json:"level"`
}

// Load 加载配置：path 为 YAML 路径，explicit 表示路径是否由 -config 显式指定。
// 默认路径文件缺失 → warn 日志 + 内置默认值；explicit=true 且文件缺失 → 返回 error。
// IAM_ 前缀环境变量覆盖文件值（后加载者胜）。
func Load(path string, explicit bool) (*Config, error) {
	k := koanf.New(".")
	if _, statErr := os.Stat(path); statErr != nil {
		if explicit {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		slog.Warn("config file not found, using built-in defaults", "path", path)
	} else if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}

	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: "IAM_",
		TransformFunc: func(s, v string) (string, any) {
			// IAM_MYSQL_DSN → mysql.dsn（三步链：TrimPrefix → ToLower → _ → .）
			return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "IAM_")), "_", "."), v
		},
	}), nil); err != nil {
		return nil, fmt.Errorf("load env overrides: %w", err)
	}

	cfg := defaults()
	if k.Exists("server.port") {
		cfg.Server.Port = k.Int("server.port")
	}
	if k.Exists("mysql.dsn") {
		cfg.MySQL.DSN = k.String("mysql.dsn")
	}
	if k.Exists("mysql.max_open_conns") {
		cfg.MySQL.MaxOpenConns = k.Int("mysql.max_open_conns")
	}
	if k.Exists("mysql.max_idle_conns") {
		cfg.MySQL.MaxIdleConns = k.Int("mysql.max_idle_conns")
	}
	if k.Exists("mysql.conn_max_lifetime") {
		d, err := time.ParseDuration(k.String("mysql.conn_max_lifetime"))
		if err != nil {
			return nil, fmt.Errorf("invalid mysql.conn_max_lifetime %q: %w", k.String("mysql.conn_max_lifetime"), err)
		}
		cfg.MySQL.ConnMaxLifetime = d
	}
	if k.Exists("snowflake.node") {
		cfg.Snowflake.Node = k.Int("snowflake.node")
	}
	if k.Exists("log.level") {
		cfg.Log.Level = k.String("log.level")
	}

	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return nil, fmt.Errorf("invalid server.port %d (must be 1..65535)", cfg.Server.Port)
	}
	if cfg.MySQL.DSN == "" {
		return nil, fmt.Errorf("mysql.dsn must not be empty")
	}
	return cfg, nil
}

// defaults 内置默认值。
func defaults() *Config {
	return &Config{
		Server: ServerConfig{Port: DefaultPort},
		MySQL: MySQLConfig{
			DSN:             DefaultDSN,
			MaxOpenConns:    DefaultMaxOpenConns,
			MaxIdleConns:    DefaultMaxIdleConns,
			ConnMaxLifetime: DefaultConnMaxLifetime,
		},
		Snowflake: SnowflakeConfig{Node: DefaultSnowflakeNode},
		Log:       LogConfig{Level: DefaultLogLevel},
	}
}
