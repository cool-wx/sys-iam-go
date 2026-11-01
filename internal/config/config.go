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
	// DefaultRedisAddr 本机 Redis 默认端点（工控机单机部署：MySQL/Redis 同机）。
	DefaultRedisAddr = "127.0.0.1:6379"
	// DefaultSessionTimeout 会话 TTL 默认 720h = Sa-Token 官方默认 30d 代理值。
	// 源 Nacos craft-satoken.yaml 实际值不可得（rnacos 空实例实证，02-RESEARCH
	// 研究重点 2），现场可经 IAM_AUTH_SESSION_TIMEOUT env 校正（D-06 Nacos 缺口文档化）。
	DefaultSessionTimeout = 720 * time.Hour

	// StorageDriverLocal / StorageDriverS3 storage.driver 合法值域（D-10）。
	StorageDriverLocal = "local"
	StorageDriverS3    = "s3"
	// DefaultStorageLocalRoot 本地存储根目录默认值（D-11）：相对工作目录，
	// systemd WorkingDirectory 联动；绝对路径可配。
	DefaultStorageLocalRoot = "./data/files"
	// DefaultStorageS3* S3 段默认值（照源 RustfsProperties.java:14-33 默认形态：
	// endpoint http://localhost:9000、bucket tims；凭证 rustfsadmin/rustfssecret
	// 为源默认值——现场部署必须替换，与本机 rustfs 容器实际 admin/admin 无关；
	// 集成测试用容器实际值经 env 覆盖注入）。
	DefaultStorageS3Endpoint  = "http://localhost:9000"
	DefaultStorageS3AccessKey = "rustfsadmin"
	DefaultStorageS3SecretKey = "rustfssecret"
	DefaultStorageS3Bucket    = "tims"
)

// Config 服务配置聚合。
type Config struct {
	Server    ServerConfig    `json:"server"`
	MySQL     MySQLConfig     `json:"mysql"`
	Redis     RedisConfig     `json:"redis"`
	Auth      AuthConfig      `json:"auth"`
	Snowflake SnowflakeConfig `json:"snowflake"`
	Log       LogConfig       `json:"log"`
	Storage   StorageConfig   `json:"storage"`
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

// RedisConfig Redis 连接配置（会话/缓存共用一个 client）。
type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// AuthConfig 认证与会话配置。YAML 叶键为 snake_case（auth.session_timeout）。
type AuthConfig struct {
	SessionTimeout time.Duration `json:"sessionTimeout"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level string `json:"level"`
}

// StorageConfig 文件存储配置（FILE-05，D-10/D-11）：driver 切换 local|s3，
// 两段互斥生效——local 模式 S3 段仅占位（构造链不触碰 minio），s3 模式
// Local 段仅占位。YAML 叶键 snake_case（storage.local_root 多段、storage.s3_*）。
type StorageConfig struct {
	Driver string             `json:"driver"`
	Local  LocalStorageConfig `json:"local"`
	S3     S3Config           `json:"s3"`
}

// LocalStorageConfig 本地磁盘存储段。
type LocalStorageConfig struct {
	Root string `json:"root"`
}

// S3Config S3 兼容对象存储段（工控机现场为 RustFS）。SecretKey 全程不进日志/
// 错误消息（T-4-04）。
type S3Config struct {
	Endpoint  string `json:"endpoint"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Bucket    string `json:"bucket"`
	Secure    bool   `json:"secure"`
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
	// Phase 2：auth.session_timeout 多段 snake_case 叶键（不登记则覆盖静默丢失，WR-01）。
	// redis.addr/redis.password/redis.db 为单词叶键，三步链直映（IAM_REDIS_ADDR→redis.addr）无需别名。
	"IAM_AUTH_SESSION_TIMEOUT": "auth.session_timeout",
	// Phase 4：storage 段（FILE-05，D-10/D-11）。三段嵌套键内带下划线的叶键
	// （s3.access_key/s3.secret_key）三步链必然错映（→storage.s3.access.key 永不
	// 匹配），必须显式别名（WR-01）；其余键（driver/local.root/s3.endpoint/
	// s3.bucket/s3.secure）三步链可直映，但按"三段以上全部显式登记最稳"一并
	// 登记，消除键名演化时的静默丢失面。
	"IAM_STORAGE_DRIVER":        "storage.driver",
	"IAM_STORAGE_LOCAL_ROOT":    "storage.local.root",
	"IAM_STORAGE_S3_ENDPOINT":   "storage.s3.endpoint",
	"IAM_STORAGE_S3_ACCESS_KEY": "storage.s3.access_key",
	"IAM_STORAGE_S3_SECRET_KEY": "storage.s3.secret_key",
	"IAM_STORAGE_S3_BUCKET":     "storage.s3.bucket",
	"IAM_STORAGE_S3_SECURE":     "storage.s3.secure",
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
	if k.Exists("redis.addr") {
		cfg.Redis.Addr = k.String("redis.addr")
	}
	if k.Exists("redis.password") {
		cfg.Redis.Password = k.String("redis.password")
	}
	if k.Exists("redis.db") {
		cfg.Redis.DB = k.Int("redis.db")
	}
	if k.Exists("auth.session_timeout") {
		d, err := time.ParseDuration(k.String("auth.session_timeout"))
		if err != nil {
			return nil, fmt.Errorf("invalid auth.session_timeout %q: %w", k.String("auth.session_timeout"), err)
		}
		cfg.Auth.SessionTimeout = d
	}
	if k.Exists("snowflake.node") {
		cfg.Snowflake.Node = k.Int("snowflake.node")
	}
	if k.Exists("log.level") {
		cfg.Log.Level = k.String("log.level")
	}
	if k.Exists("storage.driver") {
		cfg.Storage.Driver = k.String("storage.driver")
	}
	if k.Exists("storage.local.root") {
		cfg.Storage.Local.Root = k.String("storage.local.root")
	}
	if k.Exists("storage.s3.endpoint") {
		cfg.Storage.S3.Endpoint = k.String("storage.s3.endpoint")
	}
	if k.Exists("storage.s3.access_key") {
		cfg.Storage.S3.AccessKey = k.String("storage.s3.access_key")
	}
	if k.Exists("storage.s3.secret_key") {
		cfg.Storage.S3.SecretKey = k.String("storage.s3.secret_key")
	}
	if k.Exists("storage.s3.bucket") {
		cfg.Storage.S3.Bucket = k.String("storage.s3.bucket")
	}
	if k.Exists("storage.s3.secure") {
		cfg.Storage.S3.Secure = k.Bool("storage.s3.secure")
	}

	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return nil, fmt.Errorf("invalid server.port %d (must be 1..65535)", cfg.Server.Port)
	}
	if cfg.MySQL.DSN == "" {
		return nil, fmt.Errorf("mysql.dsn must not be empty")
	}
	// 连接池参数校验（WR-04/T-1-04）：database/sql 对 n<=0 的语义是"无限制连接数 /
	// 不保留空闲连接 / 永不回收"，配置写 0、键拼错致 koanf Int() 得 0、或经失效
	// 覆盖链传入 0，都会静默解除分页钳位注释（paging.go）声明的连接池 DoS 兜底
	// ——启动即 fail fast。MaxIdleConns=0 合法（仅表示不缓存空闲连接）。
	if cfg.MySQL.MaxOpenConns <= 0 || cfg.MySQL.MaxIdleConns < 0 || cfg.MySQL.MaxIdleConns > cfg.MySQL.MaxOpenConns {
		return nil, fmt.Errorf("invalid mysql pool config (max_open_conns=%d max_idle_conns=%d)", cfg.MySQL.MaxOpenConns, cfg.MySQL.MaxIdleConns)
	}
	if cfg.MySQL.ConnMaxLifetime <= 0 {
		return nil, fmt.Errorf("invalid mysql.conn_max_lifetime %s (must be > 0)", cfg.MySQL.ConnMaxLifetime)
	}
	// Redis/会话参数 fail-fast：空 addr 拼错键会在启动期以连接失败暴露；timeout<=0
	// 会让登录会话写入即过期（EXPIRE 0/负值），auth 中间件全量 401——均启动即拦截。
	if cfg.Redis.Addr == "" {
		return nil, fmt.Errorf("redis.addr must not be empty")
	}
	if cfg.Auth.SessionTimeout <= 0 {
		return nil, fmt.Errorf("invalid auth.session_timeout %s (must be > 0)", cfg.Auth.SessionTimeout)
	}
	// Storage 段 fail-fast（FILE-05，D-10/D-11）：非法 driver / s3 模式缺参启动
	// 即拦截——拼错键会静默落默认值（现场表现为"配了 s3 却 local 落盘"的静默
	// 错配，远比启动失败难排查）。错误消息不含任何凭证值（T-4-04）。
	switch cfg.Storage.Driver {
	case StorageDriverLocal:
		// D-11：local.root 空取默认值不报错（相对工作目录，systemd
		// WorkingDirectory 联动；env/文件覆盖空串与缺省同义）。
		if cfg.Storage.Local.Root == "" {
			cfg.Storage.Local.Root = DefaultStorageLocalRoot
		}
	case StorageDriverS3:
		if cfg.Storage.S3.Endpoint == "" || cfg.Storage.S3.Bucket == "" ||
			cfg.Storage.S3.AccessKey == "" || cfg.Storage.S3.SecretKey == "" {
			return nil, fmt.Errorf("storage.s3 requires endpoint, bucket, access_key and secret_key to be set when storage.driver is s3")
		}
	default:
		return nil, fmt.Errorf("invalid storage.driver %q (must be %q or %q)", cfg.Storage.Driver, StorageDriverLocal, StorageDriverS3)
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
		Redis:     RedisConfig{Addr: DefaultRedisAddr},
		Auth:      AuthConfig{SessionTimeout: DefaultSessionTimeout},
		Log:       LogConfig{Level: DefaultLogLevel},
		Storage: StorageConfig{
			Driver: StorageDriverLocal,
			Local:  LocalStorageConfig{Root: DefaultStorageLocalRoot},
			S3: S3Config{
				Endpoint:  DefaultStorageS3Endpoint,
				AccessKey: DefaultStorageS3AccessKey,
				SecretKey: DefaultStorageS3SecretKey,
				Bucket:    DefaultStorageS3Bucket,
			},
		},
	}
}
