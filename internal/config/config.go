// Package config 定义服务配置结构与加载链：YAML 文件 + IAM_ 前缀环境变量覆盖
// （koanf，D-06/D-07）。单词叶键走三步链：TrimPrefix IAM_ → ToLower → 下划线转点
// （IAM_SERVER_PORT → server.port）；多段 snake_case 叶键（max_open_conns 等）
// 三步链会把下划线全部转点导致键永不匹配（WR-01），经 envKeyAliases 显式映射。
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

// envKeyAliases 多段 snake_case 叶键的显式映射表（WR-01）：默认三步链把变量名内
// 全部下划线转为点（IAM_MYSQL_MAX_OPEN_CONNS → mysql.max.open.conns），与 YAML 键
// mysql.max_open_conns 永不匹配，覆盖静默丢失（无任何报错）。koanf env/v2 的
// TransformFunc 收到含前缀的完整变量名，故按变量全名精确映射回真实键。
// 新增多段 snake_case 键时必须在此登记。
var envKeyAliases = map[string]string{
	"IAM_MYSQL_MAX_OPEN_CONNS":    "mysql.max_open_conns",
	"IAM_MYSQL_MAX_IDLE_CONNS":    "mysql.max_idle_conns",
	"IAM_MYSQL_CONN_MAX_LIFETIME": "mysql.conn_max_lifetime",
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
			// 多段 snake_case 叶键经显式别名表映射（WR-01，见 envKeyAliases）
			if mapped, ok := envKeyAliases[s]; ok {
				return mapped, v
			}
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
