---
phase: 01-foundation
plan: 01
subsystem: infra
tags: [gin, gorm, koanf, snowflake, slog, testcontainers, envelope, walking-skeleton]

requires:
  - phase: none
    provides: n/a（Phase 1 首个执行计划；greenfield 仓库仅既有 go.mod）
provides:
  - 可启动/可探活/可优雅关停的单体 Gin 服务骨架（config→slog→GORM→中间件→health→SIGTERM 15s 排空）
  - 全库横切契约包：pkg/errcode（七分段+HTTP 映射）、pkg/resp（{code,message,data} 三键 envelope）、pkg/ids（雪花 ID JSON 字符串化）、pkg/types（时间无时区格式/枚举 name|int 双形态）、internal/auth（ctx identity）
  - sys_user 22 列幂等 DDL（scripts/ddl/sys_user.sql，实测真实 DDL + builtin 补列）
  - POST /api/v1/users create 端到端路径（handler→service→repo→BeforeCreate 钩子→MySQL）
  - testcontainers 集成测试基建（internal/testutil，离线自适应镜像/ryuk）与 6 个横切件单元测试文件
affects: [Phase 2 会话与用户域, 所有后续业务域（契约继承）, Phase 4 文件域]

actuals:
  tokens: 24163        # chars/4 over git diff bc065f6..HEAD（96,655 diff 字节）
  tasks: 2
  commits: 2
  plan_head_before: bc065f6de062b5bc713908b7a2a2128a42e4e5a1
  plan_head_after: 9d3b22a

tech-stack:
  added:
    - gin-gonic/gin v1.12.0
    - gorm.io/gorm v1.31.2 + gorm.io/driver/mysql v1.6.0
    - knadh/koanf/v2 v2.4.1（+providers/file v1.2.1 +providers/env/v2 v2.0.2 +parsers/yaml v1.1.1）
    - bwmarrin/snowflake v0.3.0
    - stretchr/testify v1.12.1
    - testcontainers/testcontainers-go/modules/mysql v0.44.0
  patterns:
    - PO 原生指针类型 / DTO 契约类型分离（GORM 永不见自定义类型，根治 Valuer/Scanner 陷阱）
    - envelope 统一出口（errors.As 收敛未知错误为 10001 固定文案，原文仅 slog）
    - BeforeCreate 钩子从 tx.Statement.Context 读 identity 填审计列（无身份→NULL）
    - koanf env/v2 三步链覆盖（TrimPrefix IAM_ → ToLower → 下划线转点）
    - "testcontainers 铁律：自动化测试只连独立容器，测试镜像本地缓存探测 + ryuk 自适应禁用"

key-files:
  created:
    - scripts/ddl/sys_user.sql
    - configs/config.yaml
    - cmd/server/main.go
    - internal/config/config.go
    - internal/app/app.go
    - internal/app/router.go
    - internal/health/health.go
    - internal/middleware/requestid.go
    - internal/middleware/access_log.go
    - internal/middleware/recovery.go
    - internal/auth/identity.go
    - pkg/errcode/errcode.go
    - pkg/resp/resp.go
    - pkg/ids/id.go
    - pkg/types/datetime.go
    - pkg/types/enum.go
    - internal/model/po/base.go
    - internal/model/po/user.go
    - internal/model/dto/user.go
    - internal/repository/user.go
    - internal/service/user.go
    - internal/handler/user.go
    - internal/testutil/mysql.go
    - internal/handler/user_integration_test.go
    - pkg/ids/id_test.go
    - pkg/types/datetime_test.go
    - pkg/types/enum_test.go
    - pkg/errcode/errcode_test.go
    - pkg/resp/resp_test.go
    - internal/config/config_test.go
    - go.mod（重写为依赖版）/ go.sum（新增）
  modified: []

key-decisions:
  - "受保护分支处置：master 为 protected（is-protected=true 且 config 无 allow_default_branch_commits），按 #2924/#3819 协议 Re-home 到 config 模板分支 gsd/phase-01-foundation 执行，master 未动"
  - "离线自适应测试基建：Docker Hub 不可达（registry EOF），mysql:8.4/ryuk:0.14.0 无法拉取——测试镜像探测本地 mysql:8.4.x 缓存（IAM_TEST_MYSQL_IMAGE 可覆盖），ryuk 默认禁用（显式 cleanup 终止容器；TESTCONTAINERS_RYUK_DISABLED 可覆盖回启用）"
  - "环境重探结论：开发库 mysql-server(8.4.8) 的 sys_user 已被并发会话补上 builtin 列（22 列齐备），计划的开发库 ALTER 条件不成立未执行；自动化测试全程 testcontainers 未受漂移影响"
  - "testcontainers 以 modules/mysql 子模块成为直接依赖（tidy 后主模块转传递依赖）——研究 7 组依赖意图不变，系 Go module 粒度所致"
  - "雪花进程单例采用显式 SetDefault（main 按配置装配）+ Default() 惰性兜底 node=1（测试便利，单机形态安全，A4）"
  - "冒烟端口：计划验收指定的 18080 被本机无关 Java 进程占用，环境变量覆盖机制改在空闲端口 18081 验证（config 日志 + HTTP 200 双证据）"

patterns-established:
  - "Pattern: envelope 出口——handler 一律 resp.OK/resp.Fail，业务错误以 *errcode.Error 传播"
  - "Pattern: DTO 指针字段零值保护——请求 DTO 全指针（nil=未提供），审计字段禁入请求 DTO（ENG-02）"
  - "Pattern: PO BeforeCreate 服务端填充——ID/双时间/四操作人列绝不信客户端"
  - "Pattern: 访问日志五字段白名单 method/path/status/duration_ms/request_id（T-1-02）"
  - "Pattern: /readyz body 仅 {\"status\"} 键（T-1-06）"

requirements-completed: [ENG-01, ENG-02, ENG-03, ENG-04, ENG-07]

coverage:
  - id: D1
    description: 可启动/可探活/可优雅关停的启动链（config/slog/GORM/三中间件/health/SIGTERM）
    requirement: ENG-06 之外的骨架面（ENG-07 配置面在内）
    verification:
      - kind: e2e
        ref: "手工冒烟：./server 启动 → GET /healthz=200；IAM_SERVER_PORT=18081 → 200；SIGTERM → exit 0 + 'shutdown complete' 日志"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go（env 覆盖/默认值/显式缺失/越界拒绝 6 例）"
        status: pass
    human_judgment: false
  - id: D2
    description: sys_user 22 列幂等 DDL（实测 21 列 + builtin 补列）
    requirement: D-03（修订）/D-04
    verification:
      - kind: manual_procedural
        ref: "临时库 gsd_ddl_check 连续执行两次脚本 → 0 错误、22 列、builtin 紧随 first_login（position 14/15）"
        status: pass
      - kind: integration
        ref: "TestCreateUserContract 前置：testcontainer 内执行同一 DDL 后断言通过"
        status: pass
    human_judgment: false
  - id: D3
    description: 契约类型包 errcode/resp/ids/types/auth（分段映射、envelope 三键、ID 字符串化、时间/枚举契约）
    requirement: ENG-01, ENG-04
    verification:
      - kind: unit
        ref: "pkg/errcode/errcode_test.go + pkg/resp/resp_test.go（防泄漏断言）+ pkg/types/datetime_test.go + pkg/types/enum_test.go（三形态+兜底）"
        status: pass
      - kind: integration
        ref: "TestCreateUserContract 断言 envelope/data 键集/时间格式/无 password/extToken"
        status: pass
    human_judgment: false
  - id: D4
    description: 雪花 ID 应用层生成 + 并发唯一性 + JSON 字符串化（>2^53）
    requirement: ENG-03
    verification:
      - kind: unit
        ref: "pkg/ids/id_test.go#TestSnowflakeConcurrentUniqueness（8×2000 零重复）+ 往返无损"
        status: pass
      - kind: integration
        ref: "TestCreateUserContract：data.id 字符串 ParseInt > 9007199254740992，DB 行 id 相等"
        status: pass
    human_judgment: false
  - id: D5
    description: 审计字段服务端填充（BeforeCreate：双时间恒填、四操作人列 ctx identity 有则填无则 NULL）
    requirement: ENG-02
    verification:
      - kind: integration
        ref: "TestCreateUserContract DB 行核对：create_time/update_time 非空、create_by_id IS NULL"
        status: pass
      - kind: unit
        ref: "dto 转换层无审计入键（UserCreateReq 结构断言隐含于集成测试请求体）"
        status: pass
    human_judgment: false
  - id: D6
    description: POST /api/v1/users create 路径端到端（真实 MySQL 8.4 容器，TZ=UTC 无漂移）
    requirement: ENG-01..04 综合载体（D-01）
    verification:
      - kind: integration
        ref: "TZ=UTC go test -tags=integration ./internal/handler/ -run TestCreateUserContract → PASS（13.4s 首跑 + 缓存复验）"
        status: pass
    human_judgment: false
  - id: D7
    description: 人工复核项——枚举写侧 name 大小写约定与 Excel 中文表示面是否与本契约冲突（Pitfall 17）
    requirement: ENG-04
    verification: []
    human_judgment: true
    rationale: "属 Phase 4 Excel 导入时的回归复核点，自动化无法在 Phase 1 证明；计划 Flagged Assumptions #1 原样留任"
  - id: D8
    description: 人工复核项——配置缺失行为裁量定案（默认路径缺失→默认值+warn；-config 显式缺失→致命退出）
    requirement: ENG-07
    verification:
      - kind: unit
        ref: "config_test.go：TestDefaultsWhenFileMissing / TestExplicitMissingFileFails 锁位"
        status: pass
    human_judgment: true
    rationale: "行为由单测锁定，但取舍本身（工控机健壮性 vs 配置指错早失败）系计划明示的裁量定案，需人工确认；不认可时改两点语义即可"

duration: 27min
completed: 2026-10-31
status: complete
---

# Phase 01 Plan 01: Walking Skeleton Tracer Summary

**Gin+GORM 骨架 + 七分段 envelope/雪花 ID/时间枚举契约/审计钩子在真实 MySQL 8.4 上经 POST /api/v1/users 全链证明（26 源文件 + 6 测试文件，testcontainers 端到端全绿）。**

## Performance
- **Duration:** 27min
- **Started:** 2026-10-31T08:06:21Z
- **Completed:** 2026-10-31T08:34:00Z（近似，含总验证）
- **Tasks:** 2/2
- **Files modified:** 32（26 源 + 6 测试；另 go.sum）

## Accomplishments
- Walking skeleton 端到端贯通：config(koanf YAML+IAM_ env 三步链) → slog JSON → GORM(绝无 AutoMigrate) → gin 三中间件 → health(/healthz /readyz) → SIGTERM 15s 优雅关停
- 全库横切契约一次定死并有测试锁定：errcode 七分段+HTTP 如实映射、resp 三键 envelope（未知错误防泄漏）、ids 雪花 JSON 字符串化（8×2000 并发零重复）、types 时间无时区格式/枚举 name|int 双形态未知兜底、auth ctx identity
- POST /api/v1/users 在 testcontainers 独立 MySQL 8.4 容器上通过全部契约断言：id >2^53 字符串化、createTime `yyyy-MM-dd HH:mm:ss` 无漂移、openid null 键保留、password/extToken 永不出网、DB 行审计列为 NULL
- 环境重探消化漂移：自动化测试全程 testcontainers，测试基建离线自适应（本地镜像探测 + ryuk 禁用可覆盖）

## Task Commits
1. **Task 1: Tracer——环境重探 + sys_user DDL + 启动链 + POST create 端到端** - `a0797f1` (feat)
2. **Task 2: 横切件单元测试（D-11 纯逻辑全覆盖）** - `9d3b22a` (test)
**Plan metadata:** （本提交，docs）

## Files Created/Modified
见 frontmatter `key-files.created`（30 个源/测试文件 + go.mod 重写 + go.sum）。

## Decisions Made
见 frontmatter `key-decisions`（分支重置、离线自适应测试基建、惰性雪花兜底、冒烟端口替换等 6 项）。

## Deviations from Plan

**1. [Rule 3 - Blocking] master 为受保护分支，Re-home 到 phase 分支**
- **Found during:** Task 1 首次提交前的 Step 0 断言
- **Issue:** 仓库 HEAD 在 `master`（gsd-tools `git.base-branch --is-protected master` = true），config 无 `allow_default_branch_commits` 覆盖
- **Fix:** 按守卫指示以 config 既有模板 `phase_branch_template` 创建 `gsd/phase-01-foundation` 并切换（非破坏性，master 未动；本计划全部提交落在该分支）
- **Files modified:** 无代码影响（git 元数据操作）
- **Verification:** 提交后 `is-protected(gsd/phase-01-foundation)=false`，HEAD 前进正常
- **Commit:** a0797f1 起

**2. [Rule 3 - Blocking] testcontainers 镜像无法拉取（Docker Hub 不可达），测试基建离线自适应**
- **Found during:** Task 1 第 7 步首跑集成测试
- **Issue:** `mysql:8.4` 与 ryuk reaper（`testcontainers/ryuk:0.14.0`）拉取报 `registry-1.docker.io EOF`；本地已有 `mysql:8.4.8` 与 `ryuk:0.8.1`
- **Fix:** testutil 增加 `testImage()`（`IAM_TEST_MYSQL_IMAGE` 环境变量 > 本地 mysql:8.4.x 缓存探测 > 标准 tag 兜底）与 `maybeDisableRyuk()`（默认禁用 ryuk，已设变量则不覆盖）
- **Files modified:** internal/testutil/mysql.go
- **Verification:** 集成测试以本地 mysql:8.4.8 启动容器全绿（13.4s）
- **Commit:** a0797f1（合入 tracer 提交）

**Total deviations:** 2 auto-fixed + **Impact on plan:** 无范围/契约影响——两项均为环境阻塞性修复，验证语义不变（集成测试仍走 testcontainers 独立容器，D-10 铁律未破）。

## Issues Encountered
- 计划 Task 1 第 2 步"若开发库缺 builtin 则手工 ALTER"：重探发现 builtin 已存在（并发会话已补），条件不成立未执行——重探结论而非问题
- 冒烟端口 18080 被本机无关 Java 进程（pid 242103）占用：服务正确报 `bind: address already in use` 退出 1（端口冲突行为正确），覆盖验证改在空闲 18081 完成
- 首轮 go mod tidy 遭遇 sum.golang.org 网络抖动（EOF），重试成功

## User Setup Required
None - 无外部服务配置要求（开发库凭据经 IAM_MYSQL_DSN 环境变量注入，configs/config.yaml 默认值为部署模板）。

## Flagged Assumptions（计划原文留任，不静默丢弃）
1. **ENG-04 unclassified**：枚举写侧 name 大小写约定与 Excel 中文表示面（Pitfall 17）需 Phase 4 导入时人工回归——见 coverage D7
2. **ENG-07 unclassified**：配置缺失行为裁量定案（warn+默认值 / 显式缺失致命退出）已单测锁位，待人工确认取舍——见 coverage D8
3. **jsonv2 兼容（研究 A5）**：本计划契约测试即防线，非缺口

## Next Phase Readiness
- 骨架即 Phase 2 代码基础（D-01 costly 决策落地）：User 域在既有 handler/service/repo 文件上扩展 update/list/get 与业务规则；认证中间件接入 internal/auth.Identity 注入即可激活审计四操作人列
- 横切契约已定死且有测试锁定，后续域直接继承（envelope/ID/时间/枚举/审计）
- 注意事项：Phase 2 需 go-redis + miniredis（本计划明确未装）；testcontainers 离线自适应设置（ryuk 禁用）在有网 CI 可用 `TESTCONTAINERS_RYUK_DISABLED=false` 恢复默认

## Self-Check: PASSED
- scripts/ddl/sys_user.sql 存在且 22 列幂等（临时库双执行验证）✓
- a0797f1 / 9d3b22a 两提交均为 HEAD 祖先（git merge-base --is-ancestor）✓
- 26 个计划文件 + 6 个测试文件全部落盘 ✓（key-files 与 git show 对账）
- TestCreateUserContract PASS、单元测试 5 包 ok、build+vet 无输出错误 ✓
