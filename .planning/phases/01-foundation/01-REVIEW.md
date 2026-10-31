---
phase: 01-foundation
reviewed: 2026-10-31T11:40:41Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - internal/app/app.go
  - internal/middleware/body_limit.go
  - internal/handler/user.go
  - internal/handler/user_integration_test.go
  - internal/middleware/requestid.go
  - internal/middleware/recovery.go
  - internal/app/router.go
  - internal/app/shutdown_integration_test.go
  - internal/service/user.go
  - internal/testutil/mysql.go
  - scripts/test-graceful-shutdown.sh
  - Makefile
  - deploy/sys-iam-go.service
findings:
  critical: 0
  warning: 0
  info: 8
  total: 8
status: clean
---

# Phase 1: Code Review Report（第 3 轮，--auto iteration 3 终轮收敛复审）

**Reviewed:** 2026-10-31T11:40:41Z
**Depth:** standard
**Files Reviewed:** 13
**Status:** clean

## Summary

终轮收敛结论：**iteration 3 的三个修复（WR-05、IN-09、IN-10）经源码逐行 + 外部事实核验全部正确，零回归、零新增 Critical/Warning**。判定依据：

- **WR-05（body 读取无时间上界）— `bb649ba` 修复正确**：`internal/app/app.go:86` 补 `ReadTimeout: 30 * time.Second`。注释（app.go:74-84）已同步改写为准确的三段式描述（ReadHeaderTimeout 封慢头 / ReadTimeout 封头+体读取总时长 / IdleTimeout 回收空闲），并删去了原来以"体积由 BodyLimit 约束"论证不需要 ReadTimeout 的体积/时间混淆表述。交互核验：ReadTimeout(30s) > 排空预算 shutdownTimeout(15s)，`Stop()` 的 `Shutdown(ctx)` 只等 15s 后记 error 返回——该排空预算行为是 P16 既有设计且上一轮已被关停集成测试覆盖，ReadTimeout 不会延长 Shutdown 等待、不与排空冲突；对 keep-alive 连接 net/http 按请求重置读 deadline，`ReadHeaderTimeout=10s` 保留为更早触发的首道闸，层叠语义正确。
- **IN-09（不实注释）— `15aae78` 修复正确，注释新声称经外部事实核验属实**：两条关键断言独立验证——(1) `grep -r requestTooLarge` 扫描 gin v1.12.0 模块源码**零匹配**，即 gin `responseWriter` 确未实现该接口，net/http `MaxBytesReader` 内的接口断言必然静默失败，标记路径为死路径；(2) Go 1.27.3 GOROOT `net/http/server.go:1164` 确有 `maxPostHandlerReadBytes = 256 << 10`、`:1465` 以该阈值判定响应后关连，注释"残量 >256KB 时响应写完即关连接"描述准确（实际判定为 `>=`，差异无实义）。`TestCreateUserBodyTooLarge` 的同断言注释（user_integration_test.go:524-527）已同步修正。纯注释变更，行为零改动。
- **IN-10（1MB 字面量双源）— `8dfca69` 修复正确，无导入环**：`internal/middleware/body_limit.go:13` 导出 `MaxBodyMB int64 = 1` 作单一事实源，`:16` `maxBodyBytes = MaxBodyMB << 20` 编译期常量派生（无运行时开销、无溢出）；`internal/handler/user.go:39-40` 文案改为 `fmt.Sprintf("请求体超过 %dMB 上限", middleware.MaxBodyMB)`，生成结果与原字面量逐字符相同。导入环排除：grep 证实 `internal/middleware` 包**零内部依赖**（仅 net/http + gin），handler→middleware 单向。`TestCreateUserBodyTooLarge` 的 `Contains "1MB"` 断言与派生文案吻合。

**实证记录**：`go build ./...`、`go vet ./...` 通过；`go test ./...`（单元）通过；`TZ=UTC go test -tags=integration -count=1 ./...`（testcontainers 真实 MySQL）**全部通过**——含 app 包 22.4s 的 SIGTERM 关停套件（覆盖 ReadTimeout 新配置下的排空语义）与 handler 包 117.6s 全套（含派生文案下的 `TestCreateUserBodyTooLarge`）。工作区对源码树零未提交改动（仅 .planning/）。

**历史处置**：CR-01、WR-01..04（iteration 1 修复）与 WR-05、IN-09、IN-10（iteration 3 修复）均已修复并经源码验证，本轮不再列为 finding。下列 8 条 INFO 为 standing 项，逐条文本复核**全部仍成立**（IN-01 行号自 38 漂移至 43，其余定位不变），按 fix_scope 设计**不修复、结转**——均不构成收敛阻塞。

无 `<structural_findings>` 输入，无外部审查证据输入。

## Info

> 以下 8 条为 standing findings（自 iteration 1 起在案），本轮逐条复核仍然成立，按 fix_scope 设计结转、不在本轮修复。Info-only 不影响 `status: clean` 收敛判定。

### IN-01: 绑定失败回显 validator 原始错误，与 T-1-01 防泄漏纪律不一致（结转，仍成立）

**File:** `internal/handler/user.go:43`

**Issue:** 非超限分支仍为 `"参数校验失败: "+err.Error()`，validator 原文（含 Go 结构体名如 `UserCreateReq.Username`）出网，与 Recovery / `resp.Fail` 的固定文案纪律不一致。同函数的超限分支已做固定文案，说明团队知晓该纪律，此分支属遗留。

**Fix:** 出网仅保留字段路径/tag 摘要或固定文案，`err.Error()` 原文进 slog。

### IN-02: 入站 X-Request-ID 无长度/字符集约束即回显并入库日志（结转，仍成立）

**File:** `internal/middleware/requestid.go:21-32`

**Issue:** 任意超长/畸形请求头被原样写回响应头、ctx 与访问日志（net/http 会消毒换行注入，但日志体积可被放大）。

**Fix:** 长度上限（如 64）+ 字符白名单（`[0-9a-fA-F-]` 等），不合规则丢弃重生成。

### IN-03: Recovery 中 `c.Abort()` 冗余（结转，仍成立）

**File:** `internal/middleware/recovery.go:25`

**Issue:** `gin.CustomRecovery` 在自定义 handler 返回后自身已执行 Abort，此处再 Abort 属死代码。

**Fix:** 删除该行。

### IN-04: testutil 镜像探测用字典序选"最新" tag，会误选旧版本并误匹配变体镜像（结转，仍成立）

**File:** `internal/testutil/mysql.go:154-174`（同模式 `scripts/test-graceful-shutdown.sh:53-54`）

**Issue:** `line > best` 字典序比较下 `mysql:8.4.9` 会被选中而非 `mysql:8.4.30`；`HasPrefix("mysql:8.4")` / `grep -E '^mysql:8\.4'` 会匹配 `mysql:8.4-oracle` 等变体（脚本侧 `sort | tail -1` 同为字典序）。仅影响测试基建稳定性。

**Fix:** 提取 `.Tag` 数值段按 semver 排序，或正则精确匹配 `mysql:8\.4\.\d+$`。

### IN-05: 关停集成测试断言 (c) 以响应完成时刻归属观察值，存在微秒级 flake 窗口（结转，仍成立）

**File:** `internal/app/shutdown_integration_test.go:202,227-243`

**Issue:** 洪流循环记录的是完成时刻 `o.at`：在 `stopStart` 前发出、排空窗口内完成的 200（合法被排空的请求）会被误计入 `postStart200`，理论上可致强断言偶发失败。

**Fix:** 观察结构体增加 `issuedAt`（发出时刻），按发出时刻归属断言。

### IN-06: ddl-dev 明文密码进进程列表（结转，仍成立）

**File:** `Makefile:58`

**Issue:** `mysql -p$(MYSQL_DEV_PASSWORD)` 使密码出现在 `ps` 输出与 shell history。仅本地开发工具，风险低。

**Fix:** 用 `MYSQL_PWD` 环境变量传递或 `--defaults-extra-file`。

### IN-07: systemd unit 以 root 运行，加固项偏少（结转，仍成立）

**File:** `deploy/sys-iam-go.service:18-30`

**Issue:** 已有 `NoNewPrivileges`/`ProtectSystem=full`，但未设 `User=`/`DynamicUser`、`PrivateTmp=`、`ProtectHome=`——进程仍以 root 全能力运行。

**Fix:** 增加 `DynamicUser=yes`（或专用低权用户）、`PrivateTmp=yes`、`ProtectHome=read-only`；部署文档同步说明数据目录权限。

### IN-08: builtin 管理字段可经公开 PUT 任意翻转（依赖 Phase 2 收紧，需跨阶段追踪）（结转，仍成立）

**File:** `internal/service/user.go:92-94`（路由白名单 `internal/app/router.go:22-29`）

**Issue:** `builtin`（内置标记）在无认证的 Phase 1 仍可被任何调用方置 true/false，D-02 临时态未变。Phase 2 加认证时必须显式收敛到管理员权限，需在 Phase 2 计划中登记。

**Fix:** Phase 2 权限模型中把 `builtin`（及后续 status/重置密码类字段）划入管理员专属可更新集合。

---

_Reviewed: 2026-10-31T11:40:41Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
