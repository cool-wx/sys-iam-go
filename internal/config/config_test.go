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
	// P5：IAM_ 前缀覆盖必须真实落到对应配置路径（TrimPrefix→ToLower→下划线转点）
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

func TestInvalidPortRejected(t *testing.T) {
	t.Setenv("IAM_SERVER_PORT", "0")
	_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
	require.Error(t, err, "port 越界必须返回 error")
}
