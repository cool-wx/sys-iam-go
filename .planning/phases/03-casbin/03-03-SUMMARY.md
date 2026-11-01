---
phase: 03-casbin
plan: 03
subsystem: authz
tags: [casbin, rbac, require-perm, route-lockdown, apikey-reverse-index, session-revocation, perm-06, in-14]

# Dependency graph
requires:
  - phase: 03-casbin
    plan: 01
    provides: RequirePerm 中间件与 Authorizer、perm 串常量（27 常量）、四写路径双轨同步、boot 对账、newTestApp 种子三件套
  - phase: 03-casbin
    plan: 02
    provides: scopes 两路由 RequirePerm 挂载先例（grep 基线 3）、newTestApp 的 super_admin data_scope=5 种子
  - phase: 02-rbac
    provides: SessionManager 三键契约（iam:session:/iam:user-tokens:/iam:apikey-token:）、KickoutAPIKey 撤销链、ApiKeyService UpdateStatus/Delete 调用点
provides:
  - router.go 全量收口：34+2 路由逐路由 RequirePerm（grep 断言 36/1——RequireRoles 仅 kickout 叠加保留，D-15 纵深防御不拆，WR-05/T-2-07 清偿）
  - iam:apikey-user: 反查键三件（IN-14/D-16）：BindAPIKey TxPipelined 原子双写（+userID 签名）/LookupAPIKeyUser 反查（损坏视同无键）/KickoutAPIKey+UnbindAPIKey 双键同删
  - ApikeyLogin key==nil 分支撤销链：10503 前反查 → KickoutUser 用户轴连坐 + UnbindAPIKey 双删（尽力而为不阻断，T-3-11 accept）
  - grantUserPerm 集成测试 helper（API 授权链运行时同步 enforcer——全量收口后无角色用户探活的标准修法）
  - testRedisAddr 包级暴露 + openCheckRedis helper（集成测试 Redis 键直查基建）
affects: [phase-4-domain-reuse（各域路由 RequirePerm 挂载即收口态）、phase-5-security-review]

# Actuals (#2632) — chars/4 over realized diff（45585 chars），非 harness token 计数
actuals:
  tokens: 11396
  tasks: 2
  commits: 2
  plan_head_before: bf156ac537411d62dbf570bc7c2539cc70d62a63
  plan_head_after: b8c9c8de80789e479c0168293b929e2596b4eb8f

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "路由收口纪律：grep 结构断言（RequirePerm==36/RequireRoles==1）+ 挂载清单表逐行对照 + TestPermRouteLockdown 每域采样 403——新域路由漏挂在三道门任一暴露"
    - "测试种子授权而非拆 gate（prohibition 落地形态）：grantUserPerm 经 PUT /roles/:id/menus + PUT /users/:id/roles API 链同步 enforcer，运行时用户当前会话下一请求即持 perm"
    - "反查键同寿命契约：无 TTL String 与 apikey-token 映射同一 MULTI/EXEC 双写、全部清理路径（KickoutAPIKey/UnbindAPIKey）双删——键寿命与 sys_api_key 行同寿命"
    - "尽力而为撤销语义：撤销失败不阻断登录拒绝响应（_ = 丢弃），DEL 反查键防同一 key 反复触发撤销链"

key-files:
  created: []
  modified:
    - internal/app/router.go（34 路由 RequirePerm + RequireRoles 退役 + WR-05 注释收口）
    - internal/auth/session.go（第四前缀 iam:apikey-user: 契约 + 反查键三件）
    - internal/auth/session_test.go（双键契约单测三件 + 2 处调用点适配）
    - internal/service/auth.go（ApikeyLogin 拒绝分支撤销链 + BindAPIKey 调用点）
    - internal/handler/perm_integration_test.go（TestPermRouteLockdown）
    - internal/handler/api_key_integration_test.go（TestApiKeyExpireRevocation + openCheckRedis/ctx0）
    - internal/handler/user_extension_integration_test.go（grantUserPerm helper + 三测试种子授权修复）
    - internal/handler/user_integration_test.go（testRedisAddr 包级变量）

key-decisions:
  - "撤销粒度 user 轴（KickoutUser 连坐密码登录活跃会话）为 D-16 拍板语义：apikey-token 映射 TTL 过期恰是缺口本身，无 TTL 反查键落 user 轴最稳——精确 token 轴需 token 级反向索引，v1 不做"
  - "既有测试适配走授权链不拆 gate：03-01 曾以『改探未挂 gate 端点』绕行，全量收口后无未挂 gate 管理端点，探活用户一律 grantUserPerm 持 system:user:query（prohibition 纪律落地）"
  - "kickout 双层 gate 顺序照现行形态 RequireRoles 前置 + RequirePerm 后置——两层任一拒绝即 403/10200，顺序无语义差异仅纵深可读性"
  - "logout 不挂 perm（挂载清单不含）——会话撤销自身不设权限门槛，已认证 token 撤自己会话无越权面"

patterns-established:
  - "全量收口后新域路由接入模板：perms.go 加常量 + router.go 逐路由 RequirePerm + TestPermRouteLockdown 采样清单加一行——三处同步即完成域收口"
  - "集成测试 Redis 键直查模板：newTestApp 暴露 testRedisAddr → openCheckRedis(t) → GET/redis.Nil 断言（键级契约测试免 DB）"

requirements-completed: [PERM-06, PERM-02]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: 34+2 路由全量 requirePerm 收口（挂载清单表逐行对照）+ RequireRoles 过渡 gate 退役（kickout 叠加保留）
    requirement: PERM-06
    verification:
      - kind: other
        ref: "grep -c \"middleware.RequirePerm(\" internal/app/router.go == 36 且 RequireRoles == 1"
        status: pass
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermRouteLockdown（六域采样 403/10200）"
        status: pass
    human_judgment: false
  - id: D2
    description: 横向越权采样直证——同 token 授权域 200 与未授权域 403 并存（授权最小化；仅授 system:menu:list 后 GET /menus 200 且 GET /users 仍 403）
    requirement: PERM-06
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermRouteLockdown 步 (a)(b)（同一 uToken 不重登录）"
        status: pass
    human_judgment: false
  - id: D3
    description: kickout 叠加语义（D-15 纵深防御直证）——持 system:auth:kickout p 策略的非 super_admin 仍 403（casbin_rule p+g 行直查佐证 403 来自 RequireRoles 第二层）；super_admin 200
    requirement: PERM-06
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermRouteLockdown 步 (c)(d)"
        status: pass
    human_judgment: false
  - id: D4
    description: PERM-02 即时生效语义在全量收口面保持——分配菜单后同一 token 下一请求 200（D2 步 (b) 同链直证）+ 既有 TestPermHotReload/RevokeRole 回归全绿
    requirement: PERM-02
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermRouteLockdown 步 (b) + TestPermHotReload（全量回归 PASS）"
        status: pass
    human_judgment: false
  - id: D5
    description: iam:apikey-user: 反查键三件（原子双写/反查/双键清理）——TTL 契约（映射键 EX timeout、反查键 -1 无过期）+ FastForward 对照直证映射过期后反查键恒可反查
    verification:
      - kind: unit
        ref: "tests/internal/auth/session_test.go#TestBindAPIKeyDualKeys|TestKickoutAPIKeyCleansDualKeys|TestUnbindAPIKeyCleansDualKeys"
        status: pass
    human_judgment: false
  - id: D6
    description: IN-14 核心断言——过期 key 二次登录 10503 的同时存量会话已被反查撤销（旧 token pri 端点 401/10100）+ 第二次拒绝后双键 DEL + 从未登录 key 无副作用幂等路径
    verification:
      - kind: integration
        ref: "tests/internal/handler/api_key_integration_test.go#TestApiKeyExpireRevocation 过期轴"
        status: pass
    human_judgment: false
  - id: D7
    description: 停用分支管理面撤销链端到端——PUT /api-keys/:id/status → KickoutAPIKey → 旧 token 401 且双键 redis.Nil 直查（UpdateStatus/Delete 零改动得清理的集成级证明）
    verification:
      - kind: integration
        ref: "tests/internal/handler/api_key_integration_test.go#TestApiKeyExpireRevocation 停用轴"
        status: pass
    human_judgment: false
  - id: D8
    description: 全量回归——既有测试在新挂载/新签名下全绿零 gate 拆除（既有 33 文件测试仅种子授权适配与 BindAPIKey 调用点适配）
    verification:
      - kind: integration
        ref: "TZ=UTC go test -tags=integration -count=1 -timeout 30m ./... 全绿输出（14 包 ok，handler 654.8s）+ make test EXIT=0"
        status: pass
    human_judgment: false

# Metrics
duration: 40min
completed: 2026-11-01
status: complete
---

# Phase 3 Plan 03: 34 路由 requirePerm 全量收口 + IN-14 apikey 反查索引 Summary

**router.go 全量挂载 34+2 路由 RequirePerm（grep 门 36/1，kickout RequireRoles 叠加保留）+ iam:apikey-user: 反查键三件使过期 key 二次登录即撤销存量会话（旧 token 401）——PERM-06/WR-05 清偿与 STATE Concerns IN-14 挂账收口**

## Performance

- **Duration:** 40 min
- **Started:** 2026-11-01T05:38:56Z
- **Completed:** 2026-11-01T06:18:42Z
- **Tasks:** 2
- **Files modified:** 8（0 新建 + 8 修改）

## Accomplishments

- 34 路由 requirePerm 全量收口（03-RESEARCH 挂载清单表逐行对照）：users 九/menus 六/depts 六/roles 七/api-keys 五/kickout 一；叠加 03-02 scopes 两路由后 RequirePerm 恰 36 处、RequireRoles 恰 1 处（kickout 双层 gate——RequireRoles 前置 + PermAuthKickout 授权层，D-15 纵深防御不拆）；logout 不挂（挂载清单不含，会话撤销不设权限门槛）；WR-05 过渡注释块替换为收口完成说明
- TestPermRouteLockdown 四步链：无 p 策略六域采样全 403/10200（users/menus/depts.tree/roles/api-keys/kickout）→ 仅授 system:menu:list 后同 token GET /menus 200 且 GET /users 仍 403（授权最小化与横向隔离直证）→ R 追加 kickout 菜单后 casbin_rule p+g 行直查佐证、kickout 仍 403（RequireRoles 第二层拦截直证）→ super_admin kickout 200
- iam:apikey-user: 反查键三件（IN-14/D-16）：BindAPIKey 改 TxPipelined 原子双写（签名 +userID——映射键 EX timeout、反查键无 TTL 同寿命，照 Login MULTI/EXEC 先例防半态）；LookupAPIKeyUser（键缺失与载荷损坏统一 ok=false）；KickoutAPIKey 块内追加 DEL + UnbindAPIKey 双删（UpdateStatus/Delete 两管理面调用点零改动即得清理）
- ApikeyLogin key==nil 分支撤销链：10503 前反查命中 → KickoutUser（用户轴连坐，D-16 拍板语义注释留档）+ UnbindAPIKey 双删——`_ =` 尽力而为不阻断 10503（T-3-11 accept），DEL 防反复触发
- 单测三件：TestBindAPIKeyDualKeys（redis TTL 协议 -1 直证反查键无过期 + FastForward 对照直证映射过期后恒可反查——IN-14 存在理由）、Kickout/Unbind 双键清理 + 幂等
- TestApiKeyExpireRevocation：过期轴（DB 直改 expire_time 模拟自然过期 → 二次登录 10503 → 旧 token pri 端点 401/10100 → 双键 DEL → 从未登录 key 幂等无副作用）+ 停用分支（管理面撤销链端到端 + openCheckRedis 直查 redis.Nil）
- 全量集成回归 14 包全绿（Task 1 态 handler 616.4s / Task 2 态 654.8s 两次 exit 0）+ make test EXIT=0 + make verify-static statically linked OK（amd64 39.5MB——Casbin 全方言依赖累积增量较 RESEARCH A1 预估 +3~6MB 实际更大，03-01 引入时已发生，工控机单包交付可接受）

## Task Commits

Each task was committed atomically:

1. **Task 1: 34 路由 requirePerm 全量挂载 + RequireRoles 退役 + 横向越权采样断言** - `49bfecc` (feat)
2. **Task 2: IN-14 apikey→user 反向索引（双写/反查撤销/清理三件）** - `b8c9c8d` (feat)

## Files Created/Modified

- `internal/app/router.go` - 34 路由逐条 RequirePerm（perm 串与挂载清单表逐行一致）+ RequireRoles 退役（kickout 叠加保留）+ 顶部注释收口说明
- `internal/auth/session.go` - 第四 Redis key 前缀契约登记（iam:apikey-user: 全项目契约文档位）+ BindAPIKey 双写/LookupAPIKeyUser/双清理
- `internal/service/auth.go` - ApikeyLogin 拒绝分支撤销链（尽力而为 + D-16 语义注释）+ BindAPIKey 调用点传 userID
- `internal/auth/session_test.go` - 双键契约单测三件 + TestConcurrentApiKeyFullLogin/KickoutAPIKey/BindingBranches 三处调用点适配
- `internal/handler/perm_integration_test.go` - TestPermRouteLockdown（横向越权采样 + 叠加语义直证）
- `internal/handler/api_key_integration_test.go` - TestApiKeyExpireRevocation + openCheckRedis/ctx0 helper
- `internal/handler/user_extension_integration_test.go` - grantUserPerm helper + TestUserStatusDisableKickout/TestResetPasswordFlow/TestDeleteUserKicksSessions 种子授权修复
- `internal/handler/user_integration_test.go` - testRedisAddr 包级变量（newTestApp 内赋值，零调用点改动）

## Decisions Made

- 既有测试适配走授权链不拆 gate：03-01 曾把探活从列表端点改到详情端点绕 gate，本次全量收口后详情端点同样挂 PermUserQuery——三个探活测试改 grantUserPerm（DB 直插菜单+角色 → API 授权链同步 enforcer），prohibition「收口后回归失败应修测试种子授权而非拆 gate」的落地形态
- 撤销粒度 user 轴照 D-16 拍板（KickoutUser 连坐密码登录活跃会话，安全从严）：精确 token 轴需 token 级反向索引，v1 不做；语义边界记录于 service/auth.go 与 session.go 注释
- 撤销链复用 UnbindAPIKey（双删）而非裸 DEL 反查键——key 已无效时 apikey-token 映射同无存在意义，同寿命语义一次清理
- kickout 双层 gate 顺序照现行 RequireRoles 前置形态（两层任一拒绝即 403，顺序仅纵深可读性差异）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] miniredis TTL 协议语义：无过期键返回 -1 而非 error**
- **Found during:** Task 2（TestBindAPIKeyDualKeys 首跑）
- **Issue:** 反查键无 TTL 断言误写为 `assert.Error(ttlErr)`——redis TTL 协议对存在但无过期的键返回 -1（go-redis 译为 time.Duration(-1)），不报错，断言必挂
- **Fix:** 改为 `assert.Equal(t, time.Duration(-1), ttlUser)`（协议语义直证反查键永不过期）
- **Files modified:** internal/auth/session_test.go
- **Verification:** go test ./internal/auth/ 10 测试全 PASS
- **Committed in:** b8c9c8d

**2. [Rule 3 - Blocking] BindAPIKey 签名变更的调用点涟漪**
- **Found during:** Task 2 中间态 make test（internal/auth、internal/service、internal/handler build failed）
- **Issue:** BindAPIKey 增第四参后 session_test.go 三处既有调用与 service/auth.go 一处未同步——纯编译中间态（Task 1 提交时点 make test 因此失败，属执行顺序现象非 Task 1 缺陷：Task 1 态的全量集成回归已独立 exit 0 验证）
- **Fix:** 4 处调用点同步传 userID（字面量/变量）；Task 2 完成态 make test EXIT=0
- **Files modified:** internal/auth/session_test.go, internal/service/auth.go
- **Verification:** make test EXIT=0 + 全量集成回归 14 包 ok
- **Committed in:** b8c9c8d

---

**Total deviations:** 2 auto-fixed（1 bug + 1 blocking；全部实现细节/编译同步级，零范围蔓延）
**Impact on plan:** 不改变任何验收语义。Task 1 的 perm_integration_test.go 编辑曾短暂误删 TestUserDataScope 注释首行，提交前自查发现并即时恢复，最终代码无残留。

## Issues Encountered

- make test（无 tag 单元回归）在 Task 1 提交时点因工作区已含 Task 2 中间态而 build failed——非代码缺陷；Task 1 验证由同刻启动的全量集成回归（exit 0，14 包）独立背书，Task 2 完成态 make test EXIT=0 补全
- 集成测试 per-test 独立容器成本延续（handler 包 654.8s）——既有基建成本，全量回归须显式 `-timeout 30m`（03-02 留档纪律已生效）

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 3 全部 3 plan 完成：PERM-01..06 全交付（本 plan 清偿 PERM-06 全量收口 + PERM-02 收口面回归保持）；STATE Blockers 两条 Phase 3 挂账（IN-14 过期 key 撤销触发点、管理端点全量授权）随本 plan 双双收口
- Phase 4 各域列表接 dataScope 仅需复制「resolveScope 编排 + repo 闭包 Apply」模板；新域路由收口照「perms.go 常量 + router.go RequirePerm + TestPermRouteLockdown 采样加行」三处同步模板
- 现场部署清单累计两项（03-02 留档）：生产存量 super_admin data_scope=NULL 经 PUT /roles/:id 置 5；本 plan 无新增部署项（iam:apikey-user: 键由完整登录自然写入，无需数据迁移）
- 二进制体积留档：Casbin 全方言依赖使 amd64 产物 22.9MB → 39.5MB（03-01 引入，本 plan 实测确认；CGO 仍为零，statically linked 出厂门过）

---
*Phase: 03-casbin*
*Completed: 2026-11-01*

## Self-Check: PASSED

- 8 个 modified 文件全部在 49bfecc/b8c9c8d 两提交中落库（git show 确认，无文件删除）
- 2 个任务提交均为 HEAD 祖先（plan_head_before=bf156ac → plan_head_after=b8c9c8d，rev-list count=2）
- 任务的全部 `<verify>` 与 plan 级 `<verification>` 均已执行且通过（结构门 36/1、auth 单测 10、TestPermRouteLockdown、TestApiKeyExpireRevocation + apikey/auth 集成族 7、make test、全量集成回归 14 包 ×2、verify-static）
