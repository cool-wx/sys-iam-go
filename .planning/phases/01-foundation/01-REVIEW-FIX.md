---
phase: 01-foundation
fixed_at: 2026-10-31T19:33:24+08:00
review_path: /home/wx/workspace/go/sys-iam-go/.planning/phases/01-foundation/01-REVIEW.md
iteration: 3
findings_in_scope: 3
fixed: 3
skipped: 0
status: all_fixed
---

# Phase 1: Code Review Fix Report（第 3 轮 / final iteration）

**Fixed at:** 2026-10-31T19:33:24+08:00
**Source review:** /home/wx/workspace/go/sys-iam-go/.planning/phases/01-foundation/01-REVIEW.md
**Iteration:** 3

**Summary:**
- Findings in scope: 3（WR-05 + 经授权的 IN-09、IN-10——均为 WR-02 修复引入/暴露的残留面；IN-01..IN-08 为长期 Info 项，本轮不动）
- Fixed: 3
- Skipped: 0

## Fixed Issues

### WR-05: 请求体读取时间无上界——慢速滴漏 body 可无限期占用连接与 goroutine

**Files modified:** `internal/app/app.go`
**Commit:** bb649ba
**Applied fix:** `http.Server` 补 `ReadTimeout: 30 * time.Second`（覆盖头+体的完整读取总时长），保留 `ReadHeaderTimeout=10s` 为更早触发的首道闸。慢速滴漏路径（10s 内发完合法头 → 每秒 1 字节滴漏 1MB body）现被 30s 总读取预算切断，fd 缓慢耗尽路径闭合。同时修正原注释以"请求体尺寸由 BodyLimit 约束"论证不需要 ReadTimeout 的**体积界/时间界混淆**表述，新注释逐项说明三个超时各封哪段、为何 30s 对工控机局域网宽裕（1MB JSON 百兆内网 <1s 传完）、以及 ReadTimeout 不影响响应写出（WriteTimeout 仍刻意不设，保留长响应兼容）。

**注：** 30s 为编排器认可的合理值；若后续出现超慢合法客户端场景可再调。未新增慢滴漏回归测试——行为级测试需真实 30s 等待（超 CI 预算），配置级断言需导出 `srv` 访问器且 `app.New` 依赖真实 MySQL 容器（`package app_test` 无法触及未导出字段），按编排器"非必需不强造"授权跳过；已改以**现有集成套件全绿**证明 ReadTimeout 未破坏正常请求与优雅关停（见 Verification）。

### IN-09: BodyLimit 注释声称的 requestTooLarge 机制在 gin 下不生效——不实注释误导读者

**Files modified:** `internal/middleware/body_limit.go`, `internal/handler/user_integration_test.go`
**Commit:** 15aae78
**Applied fix:** 按审查核实的事实（Go 1.27.3 `net/http/request.go` 中 MaxBytesReader 经 `requestTooLarger` 接口断言触发连接标记，gin v1.12.0 的 `responseWriter` 未实现该接口，断言静默失败）修正两处注释：改为"超限错误由 `handler.bindJSON` 收敛为固定文案 10401 envelope；连接复用交由 net/http 对未排空 body 的内置策略（残量 >256KB 时响应写完即关连接）"，并显式标注 requestTooLarge 路径在 gin 下不生效（IN-09），防止后续读者据旧注释做错误推断或"改回"。`TestCreateUserBodyTooLarge` 的同断言注释同步修正。行为零变更，纯注释修正。

### IN-10: 1MB 上限值在 handler 侧以字符串字面量重复，与 middleware.maxBodyBytes 解耦

**Files modified:** `internal/middleware/body_limit.go`, `internal/handler/user.go`
**Commit:** 8dfca69
**Applied fix:** 导出 `middleware.MaxBodyMB`（=1）作为单一事实源，字节上限 `maxBodyBytes = MaxBodyMB << 20` 改为编译期常量派生；`bindJSON` 超限文案改为 `fmt.Sprintf("请求体超过 %dMB 上限", middleware.MaxBodyMB)` 生成。调整上限只改一处，用户可见文案随真实上限自动更新，不再过期失真（编译期无提示的问题随之消除）。已核实 handler→middleware 无导入环（middleware 仅依赖 net/http/gin/pkg 包）。`TestCreateUserBodyTooLarge` 对"1MB"文案的 `Contains` 断言不受影响（生成结果仍含 1MB），集成实测通过。

## Skipped Issues

无。

## Verification

**运行位置：主检出** `/home/wx/workspace/go/sys-iam-go`（branch `gsd/phase-01-foundation`，每笔提交前以 `git rev-parse --show-toplevel` 根钉校验）——本轮按编排器指示未建 worktree。数字可从当前工作树直接复现。

| 层级 | 检查 | 结果 |
|------|------|------|
| Tier 2 | `go build ./...` | PASS |
| Tier 2 | `go vet ./...` | PASS |
| Tier 2 | `go vet -tags=integration ./internal/handler/`（覆盖被 tag 排除的测试文件） | PASS |
| Tier 2 | `gofmt -l internal/` | 无输出（全部已格式化） |
| 单元 | `go test ./pkg/... ./internal/config/` | 全 PASS |
| 集成 | `TZ=UTC go test -tags=integration -run TestCreateUserBodyTooLarge ./internal/handler/`（真实 MySQL 容器，WR-02 回归 + IN-10 文案断言） | PASS（10.43s） |
| 集成 | `TZ=UTC go test -tags=integration -run 'TestGracefulShutdownDrains|TestReadinessFlipBeforeDrain' ./internal/app/`（真实 http.Server 生命周期，证明 ReadTimeout=30s 不影响 1.2s 慢请求排空与摘流） | 双 PASS |

每笔提交均为原子单 finding 提交，仅 staging 触及文件；`.planning/` 未入任何 fix 提交。

---

_Fixed: 2026-10-31T19:33:24+08:00_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 3（final）_
