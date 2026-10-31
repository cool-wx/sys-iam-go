---
phase: 01-foundation
plan: 03
subsystem: build-deploy-shutdown
tags: [makefile, systemd, graceful-shutdown, sigterm, static-binary, cross-compile, journald, verify-script]

requires:
  - phase: 01-foundation (plans 01/02 — startup chain, four-endpoint slice, contract packages)
    provides: [App.Run/Stop 摘流排空链, health.SetNotReady, main 信号骨架, testutil.RunMySQL testcontainers 基建, scripts/ddl/sys_user.sql, configs/config.yaml]
provides:
  - Makefile 构建矩阵（build/build-arm64/test/test-integration/verify-static/verify-shutdown/ddl-dev/clean）——一条命令双架构静态单二进制 + file 静态断言出厂门（T-1-09）
  - deploy/sys-iam-go.service systemd unit（Type=simple/绝对路径 ExecStart/TimeoutStopSec=30/journald/NoNewPrivileges+ProtectSystem）——工控机交付物
  - SIGTERM 优雅关停双重证明：进程内集成测试（摘流 503 直证 + 排空窗口永不 200 强断言）与真实二进制脚本（exit 0 + shutdown complete 日志）
  - scripts/test-graceful-shutdown.sh 一键关停验证入口（离线镜像自适应）
affects: [Phase 2+ 全部域（构建/验证入口沿用）, 工控机交付流程（三件套）, Phase 2 会话与用户域]

actuals:
  tokens: 5074          # chars/4 over git diff 4ff6321..d2549e5（20,298 diff 字节）
  tasks: 2
  commits: 2
  plan_head_before: 4ff632132fc33e4c3c1c47a4f19a6ee1b40a7482
  plan_head_after: d2549e5430d42fa1823ce3b633cbc0998364453d

tech-stack:
  added: []             # 无新外部依赖——Makefile/file/systemd-analyze 均为既有系统工具
  patterns:
    - "make 出厂门：build 与 verify-static 分离，CGO_ENABLED=0 显式进每条构建命令，file 输出缺 statically linked 即非 0 退出（负路径已验证）"
    - "SIGTERM 排空双重证明：进程内集成测试（摘流直证+排空窗口观察）与真实二进制脚本（退出码+日志行）互为独立证据"
    - "Go Shutdown 并发观察纪律：Shutdown 并发关 idle 连接并停止 accept，关停窗口探针行为以'永不 200'强断言表达（503/拒绝均为不 ready 的合法表现）"
    - "验证脚本镜像离线自适应：IAM_TEST_MYSQL_IMAGE > 本地 mysql:8.4.x 缓存探测 > mysql:8.4 兜底（与 testutil 同模式）"

key-files:
  created:
    - Makefile
    - .gitignore
    - deploy/sys-iam-go.service
    - scripts/test-graceful-shutdown.sh
    - internal/app/shutdown_integration_test.go
  modified: []

key-decisions:
  - "摘流断言实现方式重设计：Go http.Server.Shutdown 并发时立即关 idle 连接并停止 accept，'排空窗口内外部观察到 readyz 503'的窗口仅 SetNotReady→closeIdleConns 微秒级竞态（首轮集成测试实测 FAIL 确认）——改为摘流翻面单步直证（真实 handler 200→SetNotReady→503，100% 确定）+ 排空窗口内'永不 200'强断言（503 与连接拒绝均为不 ready 表现，严格强于仅断言 503）"
  - "systemd unit 不配 ExecStop：Type=simple 默认 KillSignal=SIGTERM，进程内 signal.NotifyContext→SetNotReady→Shutdown(15s) 完成，与 D-09 原文'ExecStop 依赖 SIGTERM 触发 15s 优雅排空'意图语义等价（研究 Pattern 5 定案沿用，unit 注释与 SUMMARY 双记录）"
  - "systemd-analyze verify 依赖 ExecStart 文件存在性（本机无 /opt/sys-iam-go 首跑报 not executable）——按部署三步第一步临时放置交付物（二进制+config）后校验通过 exit 0，校验后清理开发机残留；verify 的环境前置即部署布局本身"
  - "verify-shutdown 脚本镜像选择离线自适应（Docker Hub 不可达、本地仅 mysql:8.4.8 缓存，无 mysql:8.4 标签直接 docker run 会拉取失败）——沿用 01-01 testutil 三级探测模式"
  - "Stop() 耗时断言替代'返回 nil'断言：实际 API func (a *App) Stop() 无返回值，排空预算以耗时 <15s 表达（Shutdown 错误经 slog Error 可见）"

patterns-established:
  - "Pattern: make 出厂门——build 必经 verify-static 静态断言，动态链接产物禁止出厂（T-1-09）"
  - "Pattern: 关停验证三证据——摘流翻面（503）、排空窗口无 200、真实进程 exit 0+完成日志"
  - "Pattern: 集成测试临时慢路由——测试进程内 engine.GET 注册，生产路由零污染"

requirements-completed: [ENG-06, ENG-08]

coverage:
  - id: D1
    description: Makefile 构建矩阵——双架构（amd64/arm64）CGO_ENABLED=0 静态单二进制 + verify-static file 断言出厂门（负路径非恒真已验证）+ test/test-integration/ddl-dev/clean 标准入口
    requirement: ENG-08
    verification:
      - kind: other
        ref: "make build && make build-arm64 && make verify-static → exit 0；file 两产物均 statically linked（amd64 x86-64 / arm64 aarch64）"
        status: pass
      - kind: other
        ref: "负路径：动态链接文件顶替 bin/sys-iam-go → verify-static exit 2 + FAIL 消息（脚本语义非恒真）"
        status: pass
      - kind: unit
        ref: "make test → 全部单元测试包 ok（无 integration tag，无 Docker 依赖）"
        status: pass
    human_judgment: false
  - id: D2
    description: systemd unit 交付物——Type=simple、WorkingDirectory+绝对路径 ExecStart（D-07）、TZ=Asia/Shanghai（P2 双保险）、journald（D-08）、TimeoutStopSec=30（P6）、NoNewPrivileges+ProtectSystem=full（T-1-05）
    requirement: ENG-08
    verification:
      - kind: other
        ref: "systemd-analyze verify deploy/sys-iam-go.service → exit 0、无 error 行（交付物按部署第一步放置 /opt/sys-iam-go 后校验；唯一输出为本机 teamviewerd 既有 warning，非本 unit）"
        status: pass
    human_judgment: false
  - id: D3
    description: 摘流可观察性直证——真实 health.Readiness handler + 真实 MySQL ping：SetNotReady 前 200、后立即 503、body 仅 status 键（T-1-06）
    requirement: ENG-06
    verification:
      - kind: integration
        ref: "internal/app/shutdown_integration_test.go#TestReadinessFlipBeforeDrain → PASS（10.6s，真容器）"
        status: pass
    human_judgment: false
  - id: D4
    description: App.Stop() 全路径排空——慢请求（1200ms 在途）完整 200、Stop <15s、排空窗口内 readyz 永不 200（观察洪流覆盖 Stop 前后）、Stop 后新连接拒绝
    requirement: ENG-06
    verification:
      - kind: integration
        ref: "internal/app/shutdown_integration_test.go#TestGracefulShutdownDrains → PASS（11.1s，真容器真监听）"
        status: pass
    human_judgment: false
  - id: D5
    description: 真实二进制 SIGTERM 证明——bin/sys-iam-go 独立进程：/healthz 200 → kill -TERM → exit 0（实测 0s，≤20s 预算）→ stdout 含 shutdown complete 日志行
    requirement: ENG-06
    verification:
      - kind: e2e
        ref: "make verify-shutdown → GRACEFUL SHUTDOWN VERIFIED (exit=0 elapsed=0s log=shutdown-complete)"
        status: pass
    human_judgment: false
  - id: D6
    description: D-09 语义等价确认——unit 不配 ExecStop，systemd 默认 KillSignal=SIGTERM 由进程内 NotifyContext→SetNotReady→Shutdown(15s) 完成优雅退出；exit 0 + 完成日志实证该链路
    requirement: ENG-06
    verification:
      - kind: e2e
        ref: "scripts/test-graceful-shutdown.sh 端到端（SIGTERM→exit 0→shutdown complete 日志）+ unit 注释记录等价性"
        status: pass
    human_judgment: false
  - id: D7
    description: 人工复核项——unit 是否追加 User=sysiam 与更严 ProtectHome/PrivateTmp（研究 unit 骨架未含 User=，工控机常以 root 或专用账号运行属现场裁量）
    requirement: ENG-08
    verification: []
    human_judgment: true
    rationale: "计划 Flagged Assumptions #1 原样留任：硬化项（NoNewPrivileges/ProtectSystem=full/绝对路径）已落地，专用账号与文件系统隔离强度取决于现场账号规划，需人工在部署时确认"
  - id: D8
    description: 人工复核项——arm64 产物仅在交叉编译矩阵证明（statically linked），未在真实 arm64 工控机首启验证
    requirement: ENG-08
    verification: []
    human_judgment: true
    rationale: "计划 Flagged Assumptions #2（研究 A3）原样留任：目标机现场首启验证属交付流程，开发机无法替代"

duration: 17min
completed: 2026-10-31
status: complete
---

# Phase 01 Plan 03: 构建矩阵 + systemd unit + SIGTERM 排空双重证明 Summary

**一条命令产出双架构静态单二进制（file 断言出厂门），systemd unit 静态校验零 error，SIGTERM 优雅关停经进程内集成测试与真实二进制脚本双重证明（exit 0 + shutdown complete 日志），ENG-06/ENG-08 全部可观察落地。**

## Performance
- **Duration:** 17min
- **Started:** 2026-10-31T09:02:20Z
- **Completed:** 2026-10-31T09:18:56Z
- **Tasks:** 2/2
- **Files created:** 5（Makefile/.gitignore/deploy unit/验证脚本/集成测试）

## Accomplishments
- Task 1（Makefile）：build/build-arm64 双架构 `CGO_ENABLED=0 -ldflags "-s -w"` 静态产物（实测 amd64 22.9MB / arm64 21.1MB，均 statically linked）；verify-static file 断言出厂门（负路径用动态链接文件顶替验证 exit 2 非恒真）；test/test-integration（TZ=UTC）/ddl-dev（无 mysql 客户端时打印手动指引）/verify-shutdown/clean 全部 .PHONY 入口；bin/ 入 .gitignore
- Task 2（unit + 双证明）：systemd unit 静态校验通过（systemd-analyze verify exit 0 无 error；TimeoutStopSec=30、NoNewPrivileges=true、ProtectSystem=full、绝对路径 ExecStart 全部在位）；进程内集成测试两件（摘流 503 直证 + Stop 全路径排空：慢请求完整 200、<15s、排空窗口永不 200、新连接拒绝）；真实二进制脚本一键验证（SIGTERM→exit 0 实测 0s→shutdown complete 日志）
- **D-09 语义等价注记：** D-09 原文含 "ExecStop" 字样，其意图是"SIGTERM 触发 15s 优雅排空"——Type=simple 省略 ExecStop 时 systemd 默认即向主进程发 SIGTERM（KillSignal 默认值），由进程内 signal.NotifyContext → health.SetNotReady()（摘流）→ http.Server.Shutdown(15s)（排空）→ 关 MySQL 连接完成；脚本实证该链路 exit 0。unit 文件注释与本次记录双写
- 计划级 verification 四条全部通过（build 矩阵/systemd verify/集成测试/真实脚本）

## Task Commits
1. **Task 1: Makefile 构建矩阵 + 静态链接断言** - `9372d7b` (feat)：Makefile 8 目标 + .gitignore
2. **Task 2: systemd unit + SIGTERM 排空双重证明** - `d2549e5` (feat)：unit + 集成测试 + 验证脚本（440 行）

## Files Created/Modified
见 frontmatter `key-files.created`（5 个新文件，无修改既有文件）。

## Decisions Made
见 frontmatter `key-decisions`（摘流断言重设计、ExecStop 等价、verify 的 /opt 前置、脚本镜像离线自适应、Stop 耗时断言 5 项）。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 摘流 503 并发观察在 Go Shutdown 语义下不可稳定实现，断言实现方式重设计**
- **Found during:** Task 2 集成测试首跑（FAIL：Eventually 3s 未观察到 503）
- **Issue:** 计划断言 (c)"摘流期间（慢请求在途时）另一次 GET /readyz 返回 503"——Go `http.Server.Shutdown` 并发执行时立即关闭 idle 连接（closeIdleConns）并停止 accept，keep-alive 连接在轮询间隔内处于 idle 即被服务端关闭，503 的外部可观察窗口仅 SetNotReady→closeIdleConns 首轮的微秒级竞态
- **Fix:** 拆为两个等价更强的确定性证明：(a) TestReadinessFlipBeforeDrain——真实 handler + 真实 MySQL ping 上摘流单步直证（200 基线→SetNotReady→503，100% 确定）；(b) TestGracefulShutdownDrains——排空窗口内 readyz **永不返回 200** 的观察洪流强断言（503 与连接拒绝均为"不 ready"的合法表现，强断言覆盖其全部表现，严格强于仅断言 503）
- **Files modified:** internal/app/shutdown_integration_test.go（重写）
- **Verification:** 两个测试 PASS（10.6s/11.1s，真容器）
- **Commit:** d2549e5

**2. [Rule 3 - Blocking] systemd-analyze verify 检查 ExecStart 文件存在性**
- **Found during:** Task 2 verify 首跑（exit 1："Command /opt/sys-iam-go/sys-iam-go is not executable"）
- **Issue:** 本机无 /opt/sys-iam-go 部署布局，verify 对 unit 引用的绝对路径做存在性检查
- **Fix:** 按部署三步第一步临时放置交付物（sudo mkdir /opt/sys-iam-go + cp 二进制与 config.yaml）后 verify → exit 0 无 error；校验后清理开发机残留；verify 的环境前置即部署布局本身（记录为验证流程的一部分）
- **Files modified:** 无代码影响（环境操作）
- **Commit:** d2549e5（验证闭环）

**3. [Rule 1 - Bug] 计划断言 (b)"Stop() 返回 nil"与实际 API 形态不符**
- **Found during:** Task 2 测试编写
- **Issue:** 实际 API `func (a *App) Stop()` 无返回值（Shutdown 错误经 slog Error 记录），计划设想的返回值断言无法编译
- **Fix:** 按实际语义断言耗时 <15s（排空预算的表达）；Stop 错误路径由日志可观察
- **Files modified:** internal/app/shutdown_integration_test.go
- **Commit:** d2549e5

**4. [Rule 3 - Blocking] verify-shutdown 脚本镜像选择需离线自适应**
- **Found during:** Task 2 脚本设计（环境事实：Docker Hub 不可达、本地仅 mysql:8.4.8 缓存）
- **Issue:** 计划写"docker run mysql:8.4"——本地无 mysql:8.4 标签，直接 run 会触发拉取并失败
- **Fix:** 脚本沿用 01-01 testutil 三级探测（IAM_TEST_MYSQL_IMAGE env > 本地 mysql:8.4.x 缓存 > mysql:8.4 兜底）
- **Files modified:** scripts/test-graceful-shutdown.sh
- **Commit:** d2549e5

**Total deviations:** 4 auto-fixed + **Impact on plan:** 无范围/契约影响——ENG-06/ENG-08 全部验收语义保留且断言强度不减（503 单步直证 + 永不 200 强断言 > 计划的单点并发 503 观察）。

## Issues Encountered
- 集成测试首版文件含两处写作错误（引用未定义的辅助函数、err 未定义）——均在首次编译检查中发现并重写/修复，坏状态从未进入 git
- systemd-analyze verify 输出本机 teamviewerd.service 的既有 PIDFile warning——与本 unit 无关、非 error 字样，不属本任务范围（scope boundary 未处理）
- 产物体积 22.9MB 略高于研究 scratch 的 17-18MB 估算——scratch 为最小示例，本产物含完整 internal 树（gin+gorm+koanf+snowflake 全依赖集），量级一致非异常

## User Setup Required
None - 工控机现场部署三步已记录于 unit 文件头注释（拷贝二进制+config 到 /opt/sys-iam-go、cp unit 到 /etc/systemd/system、`systemctl daemon-reload && systemctl enable --now sys-iam-go`）；真实 systemctl install 属现场步骤，开发机不执行。

## Known Stubs
None - 全部交付物为完整实现（grep TODO/FIXME/placeholder 零命中；ddl-dev 无 mysql 客户端时打印手动指引为计划明示行为非 stub）。

## Flagged Assumptions（计划原文留任，不静默丢弃）
1. **ENG-08 unclassified（交付形态人工复核）**：unit 未设专用 User=（研究骨架未含，工控机常以 root 或专用账号运行需现场定）；是否追加 ProtectHome/PrivateTmp 属现场部署裁量——见 coverage D7
2. **arm64 产物未在真实 arm64 工控机运行验证**（A3：仅交叉编译矩阵，目标机现场首启验证属交付流程）——见 coverage D8

## Next Phase Readiness
- Phase 01 三计划全部完成：骨架（01-01）→ 四件套（01-02）→ 构建部署关停交付物（01-03）——Phase 1 成功标准载体齐备
- `make build/verify-static/verify-shutdown` 为后续所有 phase 的构建与验证标准入口；`deploy/sys-iam-go.service` 为交付模板（后续 phase 若新增依赖服务如 Redis 需同步 After= 行）
- Phase 2 注意事项：go-redis + miniredis 待装；unit 的 TZ=Asia/Shanghai 与 main.go time/tzdata 内嵌已构成时区双保险（P2），Phase 2 会话 TTL 计算直接受益
- 构建环境假设已显式写进 Makefile 头注释（Go 1.27.3、CGO_ENABLED=0、tzdata 内嵌、目标机无 zoneinfo/glibc 依赖）

## Self-Check: PASSED
- Makefile、.gitignore、deploy/sys-iam-go.service、scripts/test-graceful-shutdown.sh、internal/app/shutdown_integration_test.go 均存在（git show d2549e5 对账，脚本 mode 100755）✓
- 9372d7b / d2549e5 两提交均为 HEAD=d2549e5 祖先（git rev-list 确认）✓
- 计划级 verification 四条全部实跑通过（build 矩阵 exit 0 / systemd verify exit 0 无 error / 集成 ok 21.5s / verify-shutdown GRACEFUL VERIFIED）✓
- must_haves.truths 七条逐条对账为真（见 coverage D1-D6）✓
