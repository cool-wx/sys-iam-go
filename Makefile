# sys-iam-go 构建入口（静态单二进制出厂门 + 关停验证入口）
#
# 核心纪律：
#   - CGO_ENABLED=0 显式进构建命令——任何 CGO 泄漏在工控机上表现为部署期才炸
#     （glibc 不匹配），必须在构建期以 make verify-static 的 file 断言拦截。
#   - -ldflags "-s -w" 剥离符号表（实测：全依赖集产物 17-18MB，未剥离 26MB）。
#   - 构建环境假设：Go 1.27.3、目标机无需 /usr/share/zoneinfo 与 glibc 匹配
#     （time/tzdata 已内嵌进 cmd/server/main.go）。

BIN_DIR := bin
MYSQL_DEV_HOST ?= 127.0.0.1
MYSQL_DEV_PORT ?= 3306
MYSQL_DEV_USER ?= root
MYSQL_DEV_PASSWORD ?= root123
MYSQL_DEV_DATABASE ?= sys_iam

.PHONY: build build-arm64 test test-integration verify-static verify-shutdown ddl-dev clean

# build amd64 静态单二进制（工控机默认架构）
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/sys-iam-go ./cmd/server

# build-arm64 交叉编译矩阵（arm64 工控机，输出独立文件名防覆盖）
build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o $(BIN_DIR)/sys-iam-go-arm64 ./cmd/server

# test 全部单元测试（不含 integration tag，无需 Docker）
test:
	go test ./...

# test-integration testcontainers 集成测试（需 Docker；TZ=UTC 锁定时区无漂移）
test-integration:
	TZ=UTC go test -tags=integration ./...

# verify-static 静态链接断言（出厂门）：file 输出必须含 statically linked，
# 否则以非 0 退出——动态链接产物禁止出厂。
verify-static:
	@out=$$(file $(BIN_DIR)/sys-iam-go); \
	echo "$$out"; \
	case "$$out" in \
	*statically\ linked*) echo "OK: statically linked";; \
	*) echo "FAIL: $(BIN_DIR)/sys-iam-go 缺少 statically linked（CGO 泄漏，禁止出厂）"; exit 1;; \
	esac

# verify-shutdown 真实二进制 SIGTERM 排空验证（脚本 scripts/test-graceful-shutdown.sh）
verify-shutdown:
	bash scripts/test-graceful-shutdown.sh

# ddl-dev 将 sys_user DDL 应用到开发库（凭据经 MYSQL_DEV_* 环境变量覆盖）。
# 无 mysql 客户端时打印手动指引并以非 0 退出（目标未完成）。
ddl-dev:
	@if ! command -v mysql >/dev/null 2>&1; then \
		echo "FAIL: 未找到 mysql 客户端，请手动执行："; \
		echo "  docker exec -i <mysql 容器名> mysql -u$(MYSQL_DEV_USER) -p<密码> $(MYSQL_DEV_DATABASE) < scripts/ddl/sys_user.sql"; \
		echo "  或安装 mysql-client 后：mysql -h$(MYSQL_DEV_HOST) -P$(MYSQL_DEV_PORT) -u$(MYSQL_DEV_USER) -p $(MYSQL_DEV_DATABASE) < scripts/ddl/sys_user.sql"; \
		exit 1; \
	fi; \
	mysql -h $(MYSQL_DEV_HOST) -P $(MYSQL_DEV_PORT) -u $(MYSQL_DEV_USER) -p$(MYSQL_DEV_PASSWORD) $(MYSQL_DEV_DATABASE) < scripts/ddl/sys_user.sql && echo "OK: DDL applied to $(MYSQL_DEV_DATABASE)"

# clean 清理构建产物
clean:
	rm -rf $(BIN_DIR)
