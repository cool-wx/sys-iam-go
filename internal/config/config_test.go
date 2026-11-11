package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestEnvOverrideThreeStepChain(t *testing.T) {
	// IAM_ 前缀覆盖必须真实落到对应配置路径（TrimPrefix→ToLower→下划线转点）
	path := writeConfig(t, "server:\n  port: 8080\n")
	t.Setenv("IAM_SERVER_PORT", "18080")

	cfg, err := config.Load(path, false)
	require.NoError(t, err)
	assert.Equal(t, 18080, cfg.Server.Port, "IAM_SERVER_PORT 必须覆盖 server.port")
}

func TestEnvOverrideDSNPath(t *testing.T) {
	path := writeConfig(t, "server:\n  port: 8080\n")
	t.Setenv("IAM_MYSQL_DSN", "x")

	cfg, err := config.Load(path, false)
	require.NoError(t, err)
	assert.Equal(t, "x", cfg.MySQL.DSN, "IAM_MYSQL_DSN 必须落到 mysql.dsn（下划线转点三步链）")
}

func TestEnvOverrideSnakeCaseKeys(t *testing.T) {
	// 回归防护：多段 snake_case 键必须可被 IAM_ 环境变量覆盖——默认三步链会把
	// 变量名内全部下划线转点（IAM_MYSQL_MAX_OPEN_CONNS → mysql.max.open.conns），
	// 与 YAML 键 mysql.max_open_conns 永不匹配，覆盖静默丢失。经别名表映射后必须真实生效。
	path := writeConfig(t, "server:\n  port: 8080\n")
	t.Setenv("IAM_MYSQL_MAX_OPEN_CONNS", "50")
	t.Setenv("IAM_MYSQL_MAX_IDLE_CONNS", "10")
	t.Setenv("IAM_MYSQL_CONN_MAX_LIFETIME", "1h")

	cfg, err := config.Load(path, false)
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.MySQL.MaxOpenConns, "IAM_MYSQL_MAX_OPEN_CONNS 必须覆盖 mysql.max_open_conns")
	assert.Equal(t, 10, cfg.MySQL.MaxIdleConns, "IAM_MYSQL_MAX_IDLE_CONNS 必须覆盖 mysql.max_idle_conns")
	assert.Equal(t, time.Hour, cfg.MySQL.ConnMaxLifetime, "IAM_MYSQL_CONN_MAX_LIFETIME 必须覆盖 mysql.conn_max_lifetime")
}

func TestDefaultsWhenFileMissing(t *testing.T) {
	// 默认路径缺失：内置默认值 + warn（不报错）
	cfg, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.NotEmpty(t, cfg.MySQL.DSN, "默认 DSN 必须内置（parseTime/loc 三元组固化）")
	assert.Contains(t, cfg.MySQL.DSN, "parseTime=True")
	assert.Contains(t, cfg.MySQL.DSN, "loc=Asia%2FShanghai")
	assert.Equal(t, 20, cfg.MySQL.MaxOpenConns)
	assert.Equal(t, 5, cfg.MySQL.MaxIdleConns)
	assert.Equal(t, 30*time.Minute, cfg.MySQL.ConnMaxLifetime)
	assert.Equal(t, 1, cfg.Snowflake.Node)
	assert.Equal(t, "info", cfg.Log.Level)
}

func TestExplicitMissingFileFails(t *testing.T) {
	// -config 显式指定的文件缺失：返回 error（main 以非 0 退出）
	_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), true)
	require.Error(t, err, "显式路径缺失必须早失败")
}

func TestFileValuesLoad(t *testing.T) {
	path := writeConfig(t, `server:
  port: 9090
mysql:
  dsn: "u:p@tcp(h:1)/db"
  max_open_conns: 7
  max_idle_conns: 3
  conn_max_lifetime: 5m
snowflake:
  node: 2
log:
  level: debug
`)
	cfg, err := config.Load(path, true)
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "u:p@tcp(h:1)/db", cfg.MySQL.DSN)
	assert.Equal(t, 7, cfg.MySQL.MaxOpenConns)
	assert.Equal(t, 3, cfg.MySQL.MaxIdleConns)
	assert.Equal(t, 5*time.Minute, cfg.MySQL.ConnMaxLifetime, "duration 字符串解析")
	assert.Equal(t, 2, cfg.Snowflake.Node)
	assert.Equal(t, "debug", cfg.Log.Level)
}

func TestEnvOverrideRedisAndSessionTimeout(t *testing.T) {
	// redis 段三步链直映（单词叶键 addr/password/db）+ auth.session_timeout
	// 经别名表映射（多段 snake_case 键同型）必须真实生效。
	path := writeConfig(t, "server:\n  port: 8080\n")
	t.Setenv("IAM_REDIS_ADDR", "10.0.0.9:6380")
	t.Setenv("IAM_REDIS_PASSWORD", "secret")
	t.Setenv("IAM_REDIS_DB", "2")
	t.Setenv("IAM_AUTH_SESSION_TIMEOUT", "48h")

	cfg, err := config.Load(path, false)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.9:6380", cfg.Redis.Addr, "IAM_REDIS_ADDR 必须覆盖 redis.addr（三步链直映）")
	assert.Equal(t, "secret", cfg.Redis.Password, "IAM_REDIS_PASSWORD 必须覆盖 redis.password")
	assert.Equal(t, 2, cfg.Redis.DB, "IAM_REDIS_DB 必须覆盖 redis.db")
	assert.Equal(t, 48*time.Hour, cfg.Auth.SessionTimeout, "IAM_AUTH_SESSION_TIMEOUT 必须经别名表覆盖 auth.session_timeout")
}

func TestDefaultsRedisAndSessionTimeout(t *testing.T) {
	// 默认值：redis.addr=127.0.0.1:6379、timeout=720h（30 天）
	cfg, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:6379", cfg.Redis.Addr)
	assert.Equal(t, "", cfg.Redis.Password)
	assert.Equal(t, 0, cfg.Redis.DB)
	assert.Equal(t, 720*time.Hour, cfg.Auth.SessionTimeout)
}

func TestInvalidRedisSessionConfigRejected(t *testing.T) {
	// fail-fast 回归：redis.addr 空 / timeout 非法值必须启动即失败——
	// 空 addr 拼错键会在启动期以连接失败暴露；timeout<=0 让登录会话写入即过期
	// （EXPIRE 0/负值），auth 中间件全量 401。
	cases := []struct {
		name string
		env  string
		val  string
	}{
		{"redis.addr 空", "IAM_REDIS_ADDR", ""},
		{"session_timeout=0（写入即过期）", "IAM_AUTH_SESSION_TIMEOUT", "0s"},
		{"session_timeout 负值", "IAM_AUTH_SESSION_TIMEOUT", "-1h"},
		{"session_timeout 非法格式", "IAM_AUTH_SESSION_TIMEOUT", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, tc.val)
			_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
			require.Error(t, err, "非法配置（%s=%s）必须返回 error", tc.env, tc.val)
		})
	}
}

func TestInvalidPortRejected(t *testing.T) {
	t.Setenv("IAM_SERVER_PORT", "0")
	_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
	require.Error(t, err, "port 越界必须返回 error")
}

func TestInvalidPoolConfigRejected(t *testing.T) {
	// 回归防护：连接池参数非法必须启动即失败——database/sql 对 n<=0 的语义是
	// "无限制/不保留/永不回收"，0 值会静默解除连接池 DoS 兜底。
	cases := []struct {
		name string
		env  string
		val  string
	}{
		{"max_open_conns=0（无限制语义）", "IAM_MYSQL_MAX_OPEN_CONNS", "0"},
		{"max_open_conns 负值", "IAM_MYSQL_MAX_OPEN_CONNS", "-3"},
		{"idle>open 不一致", "IAM_MYSQL_MAX_IDLE_CONNS", "99"},
		{"idle 负值", "IAM_MYSQL_MAX_IDLE_CONNS", "-1"},
		{"conn_max_lifetime=0（永不回收语义）", "IAM_MYSQL_CONN_MAX_LIFETIME", "0s"},
		{"conn_max_lifetime 负值", "IAM_MYSQL_CONN_MAX_LIFETIME", "-5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, tc.val)
			_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
			require.Error(t, err, "非法连接池配置（%s=%s）必须返回 error", tc.env, tc.val)
		})
	}
}

func TestDefaultsStorage(t *testing.T) {
	// storage 段默认值：driver=local、local.root=./data/files
	// （相对工作目录，systemd WorkingDirectory 联动）、s3 四参数默认形态
	// （endpoint 带 scheme http://localhost:9000、bucket tims——现场可配）。
	cfg, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
	require.NoError(t, err)
	assert.Equal(t, "local", cfg.Storage.Driver)
	assert.Equal(t, "./data/files", cfg.Storage.Local.Root)
	assert.Equal(t, "http://localhost:9000", cfg.Storage.S3.Endpoint)
	assert.Equal(t, "tims", cfg.Storage.S3.Bucket)
	assert.False(t, cfg.Storage.S3.Secure)
}

func TestEnvOverrideStorage(t *testing.T) {
	// 回归防护（storage 段）：三段嵌套键必须经别名表真实生效——s3.access_key/
	// secret_key 带下划线叶键三步链必然错映（→storage.s3.access.key 永不匹配），
	// 显式别名是唯一覆盖通道；driver/local.root/s3 三键一并登记（三段以上全部
	// 显式登记最稳）。
	path := writeConfig(t, "server:\n  port: 8080\n")
	t.Setenv("IAM_STORAGE_DRIVER", "s3")
	t.Setenv("IAM_STORAGE_LOCAL_ROOT", "/var/lib/iam/files")
	t.Setenv("IAM_STORAGE_S3_ENDPOINT", "http://10.0.0.9:9000")
	t.Setenv("IAM_STORAGE_S3_ACCESS_KEY", "ak-x")
	t.Setenv("IAM_STORAGE_S3_SECRET_KEY", "sk-x")
	t.Setenv("IAM_STORAGE_S3_BUCKET", "iam-bucket")
	t.Setenv("IAM_STORAGE_S3_SECURE", "true")

	cfg, err := config.Load(path, false)
	require.NoError(t, err)
	assert.Equal(t, "s3", cfg.Storage.Driver, "IAM_STORAGE_DRIVER 必须经别名表覆盖 storage.driver")
	assert.Equal(t, "/var/lib/iam/files", cfg.Storage.Local.Root, "IAM_STORAGE_LOCAL_ROOT 必须覆盖 storage.local.root")
	assert.Equal(t, "http://10.0.0.9:9000", cfg.Storage.S3.Endpoint, "IAM_STORAGE_S3_ENDPOINT 必须覆盖 storage.s3.endpoint")
	assert.Equal(t, "ak-x", cfg.Storage.S3.AccessKey, "IAM_STORAGE_S3_ACCESS_KEY 必须覆盖 storage.s3.access_key（带下划线叶键唯一通道）")
	assert.Equal(t, "sk-x", cfg.Storage.S3.SecretKey, "IAM_STORAGE_S3_SECRET_KEY 必须覆盖 storage.s3.secret_key（带下划线叶键唯一通道）")
	assert.Equal(t, "iam-bucket", cfg.Storage.S3.Bucket, "IAM_STORAGE_S3_BUCKET 必须覆盖 storage.s3.bucket")
	assert.True(t, cfg.Storage.S3.Secure, "IAM_STORAGE_S3_SECURE 必须覆盖 storage.s3.secure")
}

func TestStorageFailFast(t *testing.T) {
	// fail-fast 三分支：非法 driver / s3 模式缺参启动即拦截——拼错
	// 键静默落默认值会表现为"配了 s3 却 local 落盘"的静默错配；local 模式
	// Root 空取默认值不报错。错误消息不含凭证值。
	t.Run("非法 driver 拒绝", func(t *testing.T) {
		t.Setenv("IAM_STORAGE_DRIVER", "minio")
		_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
		require.Error(t, err, "driver 非 local/s3 必须启动即失败")
		assert.Contains(t, err.Error(), "storage.driver")
	})

	t.Run("s3 缺 endpoint 拒绝", func(t *testing.T) {
		t.Setenv("IAM_STORAGE_DRIVER", "s3")
		t.Setenv("IAM_STORAGE_S3_ENDPOINT", "")
		_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
		require.Error(t, err, "driver=s3 且 endpoint 空必须启动即失败")
	})

	t.Run("s3 缺 secret_key 拒绝且消息不含凭证值", func(t *testing.T) {
		path := writeConfig(t, `storage:
  driver: s3
  s3:
    endpoint: http://10.0.0.9:9000
    access_key: real-ak
    secret_key: ""
    bucket: iam
`)
		_, err := config.Load(path, true)
		require.Error(t, err, "driver=s3 且 secret_key 空必须启动即失败")
		assert.NotContains(t, err.Error(), "real-ak", "错误消息绝不能回显凭证值")
	})

	t.Run("local Root 空取默认值不报错", func(t *testing.T) {
		path := writeConfig(t, `storage:
  driver: local
  local:
    root: ""
`)
		cfg, err := config.Load(path, true)
		require.NoError(t, err, "driver=local 且 root 空必须取默认值而非报错")
		assert.Equal(t, "./data/files", cfg.Storage.Local.Root)
	})

	t.Run("文件加载 storage 段", func(t *testing.T) {
		path := writeConfig(t, `storage:
  driver: s3
  s3:
    endpoint: http://10.0.0.9:9000
    access_key: ak
    secret_key: sk
    bucket: iam
    secure: true
`)
		cfg, err := config.Load(path, true)
		require.NoError(t, err)
		assert.Equal(t, "s3", cfg.Storage.Driver)
		assert.Equal(t, "http://10.0.0.9:9000", cfg.Storage.S3.Endpoint)
		assert.Equal(t, "ak", cfg.Storage.S3.AccessKey)
		assert.Equal(t, "sk", cfg.Storage.S3.SecretKey)
		assert.Equal(t, "iam", cfg.Storage.S3.Bucket)
		assert.True(t, cfg.Storage.S3.Secure)
	})
}
