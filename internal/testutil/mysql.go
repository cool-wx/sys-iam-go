//go:build integration

// Package testutil 提供集成测试的 MySQL 容器基建（D-10/D-12 铁律：自动化测试一律
// testcontainers 独立容器，不锚定本机具名容器，对环境漂移免疫）。
package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// defaultMySQLImage 标准镜像 tag（有网环境由 testcontainers 拉取）。
const defaultMySQLImage = "mysql:8.4"

// RunMySQL 启动独立 mysql:8.4 容器（库 sys_iam，root/root123），打开 GORM 连接，
// 并在返回前【BLOCKING】执行 scripts/ddl/ 目录全部 .sql 建表（按文件名序，Phase 2
// 起 7 表全量就绪）——断言运行前表必须就绪。
// 返回 go-sql-driver 格式 dsn（parseTime/loc 参数已固化）与 cleanup（关连接+终止容器）。
func RunMySQL(ctx context.Context) (string, func(), error) {
	maybeDisableRyuk()
	ctr, err := tcmysql.Run(ctx, testImage(),
		tcmysql.WithDatabase("sys_iam"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("root123"),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start mysql container: %w", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "charset=utf8mb4", "parseTime=True", "loc=Asia%2FShanghai")
	if err != nil {
		terminate(ctr)
		return "", nil, fmt.Errorf("connection string: %w", err)
	}

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	if err != nil {
		terminate(ctr)
		return "", nil, fmt.Errorf("open gorm: %w", err)
	}

	// 连接就绪保险（模块日志等待策略与真实可连接之间可能存在毫秒级窗口）
	if err := pingUntilReady(ctx, db); err != nil {
		terminate(ctr)
		return "", nil, err
	}

	// BLOCKING：DDL 目录全部 .sql 逐语句执行（先于一切断言）
	if err := execDDLDir(db, filepath.Join(projectRoot(), "scripts", "ddl")); err != nil {
		terminate(ctr)
		return "", nil, err
	}

	cleanup := func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		terminate(ctr)
	}
	return dsn, cleanup, nil
}

// execDDLDir 按 glob 文件名序执行 ddl 目录下全部 .sql（filepath.Glob 返回已排序结果；
// 表间无外键依赖，顺序仅保证确定性）。任一文件失败即整体失败——表不全的容器绝不可用。
func execDDLDir(db *gorm.DB, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("glob ddl dir %s: %w", dir, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no ddl files found in %s", dir)
	}
	for _, f := range files {
		if err := execDDLScript(db, f); err != nil {
			return err
		}
	}
	return nil
}

// execDDLScript 去除注释行后按分号拆分逐语句执行。
func execDDLScript(db *gorm.DB, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read ddl script %s: %w", path, err)
	}
	var sb strings.Builder
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, stmt := range strings.Split(sb.String(), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("exec ddl statement %q: %w", firstLine(stmt), err)
		}
	}
	return nil
}

// pingUntilReady 至多等 30s 直到 MySQL 可执行查询。
func pingUntilReady(ctx context.Context, db *gorm.DB) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		sqlDB, err := db.DB()
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			lastErr = sqlDB.PingContext(pingCtx)
			cancel()
			if lastErr == nil {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("mysql not ready within 30s: %w", lastErr)
}

// terminate 终止容器（独立 ctx：测试 ctx 可能已取消/超时）。
func terminate(ctr *tcmysql.MySQLContainer) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = ctr.Terminate(ctx)
}

// maybeDisableRyuk 离线环境自适应：Docker Hub 不可达时 ryuk reaper 镜像无法拉取
// （v0.44.0 库硬编码 testcontainers/ryuk:0.14.0），导致容器创建直接失败。
// 本测试基建的 cleanup 已显式终止容器（孤儿风险仅限进程被 SIGKILL 的极端场景），
// 故默认禁用 ryuk；用户已显式设置该变量时不覆盖（有网 CI 可设回 false 启用）。
func maybeDisableRyuk() {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

// testImage 决定集成测试 MySQL 镜像：//  1. IAM_TEST_MYSQL_IMAGE 环境变量显式指定；
//  2. 否则探测本地已有的 mysql:8.4.x 镜像（离线环境无 Docker Hub 访问，直接复用本地
//     缓存，避免无效拉取重试拖慢测试）；
//  3. 兜底 mysql:8.4（有网环境由 testcontainers 正常拉取）。
func testImage() string {
	if v := os.Getenv("IAM_TEST_MYSQL_IMAGE"); v != "" {
		return v
	}
	if img := localMySQLImage(); img != "" {
		return img
	}
	return defaultMySQLImage
}

// localMySQLImage 返回本地已缓存的最新 mysql:8.4.x 镜像 tag（无 docker CLI 或无匹配则空串）。
func localMySQLImage() string {
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	out, err := exec.Command(dockerBin, "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		return ""
	}
	best := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "mysql:8.4") || line == "mysql:8.4<none>" {
			continue
		}
		if line > best {
			best = line
		}
	}
	return best
}

// firstLine 取语句首行用于错误信息。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// projectRoot 自测试目录向上找 go.mod 定位仓库根（DDL 路径不锚定 cwd）。
func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
