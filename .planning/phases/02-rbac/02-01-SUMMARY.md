---
phase: 02-rbac
plan: 01
subsystem: auth
tags: [redis, token-session, gin-middleware, bcrypt, go-redis, miniredis, testcontainers]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: Gin 装配链（app.New/newRouter）、UserRepo/Handler/Service 模板、errcode/resp/paging/ids 契约、testutil MySQL testcontainers 基建
provides:
  - SessionManager（Redis token-session：登录/校验续期/登出/踢下线/apiKey 绑定）
  - 认证中间件（Bearer 解析 + fail-closed）与 pub/pri 路由分组（白名单四路径）
  - auth 三端点 login/apikey-login/logout + kickout
  - Redis key 全项目契约：iam:session:{token} / iam:user-tokens:{userId} / iam:apikey-token:{apiKey}
  - 6 张表幂等 DDL + RunRedis testcontainers 基建 + seedUser/loginAs/seedApiKey 测试 helper
  - config redis/auth 段（session_timeout=720h 代理值，env 可校正）+ errcode 10502..10506
  - relation repo（FindRolesByUserIDs IN 查询，Phase 2 五域与 Phase 3 共用）
affects: [02-02 五域 CRUD（user/role 扩展）、02-03（menu/dept/apikey 域）、Phase 3 Casbin（会话 roles 载荷起点）]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 31500    # chars/4 over the realized diff (125966 chars)
  tasks: 2         # tasks completed
  commits: 3       # MEASURED: git rev-list --count gsd-plan-head-before-02-01..HEAD

tech-stack:
  added:
    - github.com/redis/go-redis/v9 v9.23.0
    - github.com/alicebob/miniredis/v2 v2.40.0
    - github.com/testcontainers/testcontainers-go/modules/redis v0.44.0
    - golang.org/x/crypto v0.58.0（indirect→直接依赖，bcrypt）
  patterns:
    - Redis token-session 管理器（Sa-Token 等价；TxPipelined 原子写、EXPIRE 滚动续期、Set 反查索引惰性清理）
    - pub/pri 路由分组 + handlers 聚合 struct（扩域不改 newRouter 签名）
    - 认证中间件 fail-closed（ErrSessionNotFound→401、其余错误→500 绝不放行）
    - 恒时防枚举登录（dummyHash 常量 + 统一 10501 文案）

key-files:
  created:
    - internal/auth/session.go
    - internal/middleware/auth.go
    - internal/service/auth.go
    - internal/handler/auth.go
    - internal/model/dto/auth.go
    - internal/model/po/role.go
    - internal/model/po/relation.go
    - internal/model/po/api_key.go
    - internal/repository/relation.go
    - internal/repository/api_key.go
    - internal/testutil/redis.go
    - internal/auth/session_test.go
    - internal/middleware/auth_test.go
    - internal/handler/auth_integration_test.go
    - scripts/ddl/sys_role.sql
    - scripts/ddl/sys_menu.sql
    - scripts/ddl/sys_dept.sql
    - scripts/ddl/sys_api_key.sql
    - scripts/ddl/sys_user_role.sql
    - scripts/ddl/sys_role_menu.sql
  modified:
    - internal/app/app.go
    - internal/app/router.go
    - internal/config/config.go
    - internal/config/config_test.go
    - configs/config.yaml
    - internal/repository/user.go
    - internal/service/user.go
    - internal/testutil/mysql.go
    - pkg/errcode/errcode.go
    - internal/handler/user_integration_test.go
    - internal/app/shutdown_integration_test.go
    - go.mod
    - go.sum

key-decisions:
  - "SessionManager 六方法面照 02-RESEARCH 伪代码逐段转译；ErrSessionNotFound 包级哨兵归一 HGetAll 空 map（go-redis 对不存在 key 返回空 map+nil 而非 redis.Nil）"
  - "apikeyLogin 复用分支命中但用户已删时顺延完整路径（由 10504/10505 给精确错误）——避免复用路径裸返 10300 语义错位"
  - "并发 apikeyLogin 竞态显式决策：双 token 各自有效、映射 SET 最后写者赢、无反向索引（TTL 窗口无害残留）——TestConcurrentApiKeyFullLogin 直证"
  - "miniredis 直接 API 对缺失键返回错误（非空值）——测试断言一律经 go-redis 客户端（缺失=空集语义）"
  - "status 列 NULL fail-closed 视为禁用（非 ENABLED 分支含 NULL，登录拦截从紧）"

patterns-established:
  - "Pattern: 认证中间件三分支——无 token/会话缺失 401、Redis 故障 500 fail-closed、命中注入 Identity 续期"
  - "Pattern: 登录编排六步链在 service 层，completeLogin 私有装配供 password/apiKey 两路径共用"
  - "Pattern: 集成测试种子直插 SQL 自举（create 端点已要求认证）——seedUser/seedApiKey helper"
  - "Pattern: 单元 miniredis（FastForward 推进 TTL）+ 集成双容器 testcontainers 分层"

requirements-completed: [AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06]

coverage:
  - id: D1
    description: 用户名密码登录端到端——存量 Hutool $2a$ 哈希零迁移登录成功；用户不存在与密码错误 body 逐字节相同（恒时防枚举）；畸形 hash 返回 10501/401 非 500；禁用用户 10502
    requirement: AUTH-01
    verification:
      - kind: integration
        ref: "internal/handler/auth_integration_test.go#TestAuthFlow"
        status: pass
      - kind: integration
        ref: "internal/handler/auth_integration_test.go#TestAuthLoginFailures"
        status: pass
    human_judgment: false
  - id: D2
    description: API Key 登录——有效/ENABLED/未过期 key 换取 token；同 key 复用同一 token；停用/过期 10503、未关联用户 10504；并发双登录竞态双 token 各自有效最后写者赢
    requirement: AUTH-02
    verification:
      - kind: integration
        ref: "internal/handler/auth_integration_test.go#TestApiKeyLoginFlow"
        status: pass
      - kind: unit
        ref: "internal/auth/session_test.go#TestConcurrentApiKeyFullLogin"
        status: pass
    human_judgment: false
  - id: D3
    description: 登出即刻失效——登出后原 token 访问受保护路由 401（code=10100），重复登出幂等
    requirement: AUTH-03
    verification:
      - kind: integration
        ref: "internal/handler/auth_integration_test.go#TestAuthFlow"
        status: pass
      - kind: unit
        ref: "internal/auth/session_test.go#TestLogoutInvalidatesSession"
        status: pass
    human_judgment: false
  - id: D4
    description: 踢下线——kickout 后目标用户全部 token 立即 401、iam:user-tokens 索引清空（含死 token 惰性清理）、被踢用户可重新登录
    requirement: AUTH-04
    verification:
      - kind: integration
        ref: "internal/handler/auth_integration_test.go#TestKickoutFlow"
        status: pass
      - kind: unit
        ref: "internal/auth/session_test.go#TestKickoutCleansIndexAndDeadTokens"
        status: pass
    human_judgment: false
  - id: D5
    description: 会话 TTL 活跃续期——TTL=auth.session_timeout（默认 720h），每次校验命中 EXPIRE 重置为完整 timeout（miniredis FastForward 双段推进证明）；未续期对照组证过期
    requirement: AUTH-05
    verification:
      - kind: unit
        ref: "internal/auth/session_test.go#TestSessionTTLRenewReset"
        status: pass
    human_judgment: false
  - id: D6
    description: 白名单收紧——login/apikey-login/healthz/readyz 四路径免 token 可达，其余全部路由无 token 401；Redis 故障时中间件 fail-closed 500 绝不放行
    requirement: AUTH-06
    verification:
      - kind: integration
        ref: "internal/handler/auth_integration_test.go#TestAuthWhitelist"
        status: pass
      - kind: unit
        ref: "internal/middleware/auth_test.go#TestAuthMiddlewareFailsClosedOnRedisError"
        status: pass
    human_judgment: false
  - id: D7
    description: 测试与 schema 基建——6 张表幂等 DDL（容器实证提取）+ RunRedis 离线自适应 + seedUser/loginAs/seedApiKey helper；Phase 1 既有测试带 token 全绿
    verification:
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestCreateUserContract"
        status: pass
      - kind: integration
        ref: "TZ=UTC go test -tags=integration ./internal/handler/ ./internal/service/ ./internal/app/"
        status: pass
    human_judgment: false
  - id: D8
    description: 配置面——redis/auth 配置段五步链（默认 127.0.0.1:6379 / 720h Sa-Token 30d 代理值）+ IAM_AUTH_SESSION_TIMEOUT 别名覆盖 + fail-fast 非法值启动拦截
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestEnvOverrideRedisAndSessionTimeout"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestInvalidRedisSessionConfigRejected"
        status: pass
    human_judgment: false

# Metrics
duration: 43min
completed: 2026-10-31
status: complete
plan_head_before: 01c6afc1071e504f2a6cf2b72efacd8fea6d0167
plan_head_after: ca0b550510f0496218e4835181bd6603a48b24b8
commits: 3
---

# Phase 02 Plan 01: 认证会话 tracer 与核心 Summary

**Redis token-session 全链路：存量 `$2a$` BCrypt 密码零迁移登录、API Key 复用/竞态处置、TTL 滚动续期、kickout 惰性清理、白名单四路径收紧，双容器集成测试全绿**

## Performance

- **Duration:** 43 min
- **Started:** 2026-10-31T13:15:42Z
- **Completed:** 2026-10-31T13:59:38Z
- **Tasks:** 2
- **Files modified:** 33（20 新建 + 13 修改）

## Accomplishments
- SessionManager（internal/auth/session.go）：crypto/rand 32B token、TxPipelined 原子登录写、Resolve 命中即 EXPIRE 重置满、Logout 幂等、KickoutUser 死 token 惰性清理、apiKey→token 绑定双检——Redis key 三前缀即全项目契约
- 认证中间件 fail-closed 三分支（401/500/注入 Identity 续期）+ 路由收紧：pub 白名单四路径（login/apikey-login/healthz/readyz），pri 组挂 logout/kickout + 既有 users 四路由
- 登录编排：六步链照源（恒时防枚举 dummyHash、畸形 hash 译 10501 非 500、roles 装配 permissions 不放）；apikeyLogin 六分支照源 + 复用分支 + 并发竞态显式决策注释
- 测试基建：6 张表幂等 DDL（mysqldump 实证提取）、RunRedis 离线自适应（IAM_TEST_REDIS_IMAGE>本地 redis:8.x>redis:8）、miniredis 单元 6 用例 + 双容器集成 5 用例
- 存量 BCrypt 零迁移活体直证：本库 admin 真实哈希（明文 admin123）作为集成 fixture 登录成功（D-16 代证链闭环）
- config redis/auth 段五步链 + IAM_AUTH_SESSION_TIMEOUT 别名 + fail-fast；720h=Sa-Token 30d 代理值缘由文档化（Nacos 缺口闭环）
- user Create 密码 bcrypt 哈希化收口（Phase 1 预留边界）

## Task Commits

Each task was committed atomically:

1. **Task 1: Tracer——Redis 基建 + login→session→中间件→logout 端到端 + 白名单收紧** - `6c1a258` (feat)
2. **Task 2 (TDD RED): apikey-login/kickout lifecycle 失败测试** - `8c7c667` (test)
3. **Task 2 (TDD GREEN): apikeyLogin 复用/竞态、kickout、白名单第四路径** - `ca0b550` (feat)

_Note: Task 2 为 tdd="true"——RED 证据经 `gsd-run check tdd-red-evidence` 分类器（RED_EVIDENCE_OK，目标测试 404≠200 有意失败）；REFACTOR 无变更未提交（completeLogin 已共用）。_

## Files Created/Modified
- `internal/auth/session.go` - SessionManager 六方法 + Redis key 三前缀契约 + ErrSessionNotFound 哨兵
- `internal/middleware/auth.go` - 认证中间件（fail-closed）+ BearerToken 提取
- `internal/service/auth.go` - Login 六步链 / ApikeyLogin 六分支 / Logout / Kickout
- `internal/handler/auth.go` + `internal/model/dto/auth.go` - 四端点与请求 DTO
- `internal/app/app.go` + `router.go` - Redis 装配链（Ping fail-fast）+ pub/pri 分组 + handlers 聚合
- `internal/config/config.go` + `configs/config.yaml` - redis/auth 段与 env 覆盖链
- `scripts/ddl/sys_{role,menu,dept,api_key,user_role,role_menu}.sql` - 6 张表幂等 DDL
- `internal/testutil/{redis.go,mysql.go}` - Redis testcontainers 基建 + DDL 目录遍历
- `pkg/errcode/errcode.go` - 10502..10506 五码全 401
- `internal/repository/{relation,api_key,user}.go` - FindRolesByUserIDs IN 查询 / FindByKey 登录过滤 / FindByUsername
- `internal/model/po/{role,relation,api_key}.go` - 四 PO（关联表不内嵌 BaseWithOperator）
- `internal/handler/{auth,user}_integration_test.go` - 认证链路集成测试 + 既有测试带 token 适配
- `internal/service/user.go` - Create 密码哈希化

## Decisions Made
- ErrSessionNotFound 包级哨兵归一（HGetAll 空 map 非 redis.Nil，易踩点显式收口）
- apikeyLogin 复用分支用户已删时顺延完整路径（精确 10504/10505，不裸返 10300）
- 并发 apikeyLogin：双 token 各自有效 + 映射 SET 最后写者赢 + 无反向索引（CONTEXT 认可的无害残留）
- status NULL 在登录路径 fail-closed 视为禁用
- 测试断言 miniredis 经 go-redis 客户端（其直接 API 对缺失键报错而非空值）
- 既有测试审计断言随收紧更新：带 token 后 create/update 的操作人列必填 admin identity、列表 total 含 admin 种子行

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] shutdown 集成测试双容器适配**
- **Found during:** Task 1（app.go 装配改造后）
- **Issue:** app.New 新增 Redis Ping fail-fast 后，shutdown_integration_test.go 的字面量 cfg（无 Redis 段）装配必败——Phase 1 测试回归将被破坏
- **Fix:** newShutdownApp 扩为 MySQL+Redis 双容器（testutil.RunRedis），cfg 补 Redis/Auth 段
- **Files modified:** internal/app/shutdown_integration_test.go
- **Verification:** TZ=UTC go test -tags=integration ./internal/app/ 全绿（27.9s）
- **Committed in:** 6c1a258（Task 1 commit）

**2. [Rule 3 - Blocking] 既有测试断言随认证收紧语义更新**
- **Found during:** Task 1（集成测试适配）
- **Issue:** 带 token 后 identity 进入 ctx——create_by/update_by 审计列由 NULL 变为 admin identity；newTestApp 种子 admin 使列表 total +1；原 `assert.Nil` 断言必失败
- **Fix:** TestCreateUserContract/TestUpdateUserZeroValueProtection 审计断言改为等值 admin（createById/updateById/updateByName）；TestListUsersPaging total 3→4、TestListUsersEdgeParams total 1→2
- **Files modified:** internal/handler/user_integration_test.go
- **Verification:** handler 包全量集成 ok（215.8s，九测全绿）
- **Committed in:** 6c1a258

---

**Total deviations:** 2 auto-fixed（均 Rule 3 阻塞类——app.New 签名语义收紧的直接后果，无范围蔓延）
**Impact on plan:** 两处适配均为既有测试在收紧后的必然同步，plan 步骤 15"既有测试适配"的应尽范围；无 scope creep。

**Task 内小偏差说明：** session_test.go 6 用例中 4 个（TTL 续期/kickout 清理/apiKey 绑定/登出）随 Task 1 的 session.go 提前落盘（plan 两 task 文件清单重叠；Task 1 产物必须自证 fail-closed 等验收项）；Task 2 以 TestConcurrentApiKeyFullLogin 补齐 behavior 清单第 4 用例。终态与 plan 完全一致。

## Issues Encountered
- proxy.golang.org 首次下载 go-redis 时 unexpected EOF——重试成功（网络抖动，非镜像问题）
- miniredis 直接 API 语义差异：SMembers 对缺失键返回 `ERR no such key`（非空切片）——kickout 全清后断言改经 go-redis 客户端（缺失=空集）
- go.mod 依赖在 `go get` 后仍标 indirect——`go mod tidy` 提升为直接依赖（验收要求直接依赖）

## User Setup Required

None - no external service configuration required.（Nacos timeout 缺口已文档化：`auth.session_timeout: 720h` + `IAM_AUTH_SESSION_TIMEOUT` env 可现场校正）

## Next Phase Readiness
- 认证链已验证：02-02/02-03 的五域业务路由全部挂 pri 组即可（handlers 聚合 struct 已留扩展位，newRouter 签名无需再改）
- seedUser/seedApiKey/loginAs helper 与双容器 newTestApp 契约可被五域集成测试直接复用
- relation repo（FindRolesByUserIDs）就绪——用户详情两查询装配（USER-05）与角色分配可直接站上
- 会话 roles 载荷已写入（Phase 3 Casbin 的起点）；permissions 快照有意不放
- 无阻塞项；phases.list 无遗留 blocker

---
*Phase: 02-rbac*
*Completed: 2026-10-31*

## Self-Check: PASSED

- 20/20 关键新建文件存在于磁盘（`[ -f ]` 全过）
- 3/3 任务提交存在于 HEAD 祖先链（6c1a258 tracer / 8c7c667 TDD RED / ca0b550 TDD GREEN）
- 实测提交数：`git rev-list --count gsd-plan-head-before-02-01..HEAD` = 3（与 frontmatter `commits: 3` 一致）
- 全部 `<verify>` 复跑通过：build/vet 绿、8 包单元绿、handler/service/app 集成全绿、`make verify-static` 静态门过
