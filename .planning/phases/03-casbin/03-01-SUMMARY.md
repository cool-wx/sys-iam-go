---
phase: 03-casbin
plan: 01
subsystem: authz
tags: [casbin, rbac, authorization, gorm-adapter, middleware, policy-sync]

# Dependency graph
requires:
  - phase: 02-rbac
    provides: sys_role_menu/sys_user_role 关系表写路径（AssignMenus/AssignRoles）、会话身份（Identity 注入）、newTestApp 双容器契约测试基建
provides:
  - internal/authz 包：SyncedEnforcer 五步装配链（TurnOffAutoMigrate→adapter→model→NewSyncedEnforcer→LoadPolicy fail-fast）+ Enforce/ReplaceAll/SyncRolePerms/SyncUserRoles/ClearRole/ClearUser 同步 API + Authorizer 接口
  - 全量 perm 串常量（system: 前缀 27 常量，34 路由对照）+ 内嵌纯 RBAC model（super_admin 豁免首位短路）
  - RequirePerm 中间件（401/403/500 fail-closed 三分支）与首路由 GET /api/v1/users 挂载
  - scripts/ddl/casbin_rule.sql 显式 DDL（自带唯一索引，TurnOffAutoMigrate 后 adapter 不建索引）
  - app.New boot 对账：sys_user_role/sys_role_menu 全量导入 g/p 策略（存量 admin 冷启动即可用）
  - 四写路径双轨同步（AssignMenus/AssignRoles/角色删除/用户删除，事务成功后同步 + Error 1869 去重防御）
affects: [03-02-datascope, 03-03-route-lockdown, phase-4-domain-reuse]

# Actuals (#2632) — chars/4 over realized diff（94820 chars），非 harness token 计数
actuals:
  tokens: 23705
  tasks: 2
  commits: 2
  plan_head_before: 983159658d8a30806e3bc939e11ce610be9d49ad
  plan_head_after: 40143b70f07d03a5668a6076aedc8a2631994063

# Tech tracking
tech-stack:
  added:
    - github.com/casbin/casbin/v2 v2.135.0（版本锁死——gorm-adapter v3.36+ 切 casbin/v3 不兼容）
    - github.com/casbin/gorm-adapter/v3 v3.35.0（最后支持 casbin/v2 的 adapter；全方言驱动均纯 Go，CGO_ENABLED=0 不受影响）
  patterns:
    - "enforcer 进程内单例经 app.New 构造注入（D-02/D-04 fail-fast 对齐 Redis Ping 先例）"
    - "策略双轨同步纪律：业务 db.Transaction 提交成功后调 enforcer API（adapter 独立 Session 不在事务内），失败上抛 500 客户端重试自愈（幂等全删全加）"
    - "boot 对账：启动时关系表→casbin_rule 全量导入（ReplaceAll 幂等可重入），失败拒绝启动"
    - "super_admin 豁免唯一判据在 enforcer g 层（matcher 首位短路），撤销角色下一请求即失效"

key-files:
  created:
    - internal/authz/enforcer.go
    - internal/authz/perms.go
    - internal/authz/enforcer_test.go
    - internal/middleware/perm.go
    - internal/middleware/perm_test.go
    - internal/handler/perm_integration_test.go
    - scripts/ddl/casbin_rule.sql
  modified:
    - internal/app/app.go
    - internal/app/router.go
    - internal/repository/relation.go
    - internal/service/role.go
    - internal/service/user.go
    - internal/handler/user_integration_test.go
    - go.mod / go.sum

key-decisions:
  - "sub 采用 role_code：role_code 不可改（Update 白名单无 role_code）→ 策略行不因改名失效；g 行 (uid 串, role_code)、p 行 (role_code, perm)"
  - "super_admin 豁免唯一判据 = enforcer g 层（matcher 首位 g(r.sub, \"super_admin\") 短路），会话 roles 快照判据废弃——撤销即时失效（D-07）"
  - "策略同步放业务事务外两独立提交（adapter 独立 Session 物理上不在事务内），失败 500 重试自愈（D-13）"
  - "boot 对账含内存 map 去重（g/p 双防 Error 1869）+ repo 层 FindRolePerms 去重（第一道防御）"

patterns-established:
  - "RequirePerm(az Authorizer, perm) 逐路由显式挂载形态（03-03 全量收口照此）"
  - "权限类集成测试模板：permFixture 种子三件套 + 同 token 即时生效四步链 + casbin_rule 直查断言"
  - "casbin 纯内存单测：model.NewModelFromString + NewEnforcer(m)（无 adapter）毫秒级锁 matcher 语义"

requirements-completed: [PERM-01, PERM-02, PERM-04, PERM-05, PERM-06]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: enforcer 五步装配链进 app.New（fail-fast）+ boot 对账全量导入 g/p 策略
    requirement: PERM-01
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermEnforcerBootSmoke"
        status: pass
      - kind: integration
        ref: "tests/internal/app（全量回归 internal/app ok）"
        status: pass
    human_judgment: false
  - id: D2
    description: RequirePerm 中间件三分支（identity 缺失 401 / 未命中 403+10200 / enforcer 故障 500 fail-closed）
    requirement: PERM-06
    verification:
      - kind: unit
        ref: "tests/internal/middleware/perm_test.go#TestRequirePermMiddleware"
        status: pass
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermEnforcerBootSmoke（无角色 403/10200 + 无 token 401）"
        status: pass
    human_judgment: false
  - id: D3
    description: tracer 首路由 GET /api/v1/users 挂 RequirePerm（恰 1 处，RequireRoles kickout 叠加保留）
    requirement: PERM-06
    verification:
      - kind: other
        ref: "grep -c \"middleware.RequirePerm(\" internal/app/router.go == 1"
        status: pass
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermHotReload（列表端点行为直证）"
        status: pass
    human_judgment: false
  - id: D4
    description: 四写路径双轨同步（AssignMenus/AssignRoles/角色删除/用户删除，事务后同步 + 去重防御 + 单数 API）
    requirement: PERM-04
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermHotReload（casbin_rule p 行 + sys_role_menu 行同写同删）"
        status: pass
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermDeleteCascades（ClearRole 双清）"
        status: pass
    human_judgment: false
  - id: D5
    description: 即时生效语义——分配→200/回收→403 同一 token 不重登录（p 轴与 g 轴双直证）
    requirement: PERM-02
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermHotReload（(b) 步复用 (a) 步 token，测试内无第二次登录）"
        status: pass
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermRevokeRoleImmediate"
        status: pass
    human_judgment: false
  - id: D6
    description: 授权管理面复用既有 PUT /roles/:id/menus + PUT /users/:id/roles（策略落 casbin_rule，PERM-05）
    requirement: PERM-05
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestPermHotReload（经 API 分配后直查 casbin_rule）"
        status: pass
    human_judgment: false
  - id: D7
    description: matcher 三坑单元锁定（token 名一致 / 豁免首位短路零 p 行直证 / 撤销 g 行即时失效）+ SyncRolePerms 替换语义
    requirement: PERM-01
    verification:
      - kind: unit
        ref: "tests/internal/authz/enforcer_test.go#TestMatcherTokenNames|TestSuperAdminExemptWithoutPolicies|TestRevokeSuperAdminImmediate|TestSyncRolePermsReplacesAll"
        status: pass
    human_judgment: false
  - id: D8
    description: 全量回归——Phase 1/2 既有测试在新挂载下全绿（13 包 ok）
    verification:
      - kind: integration
        ref: "TZ=UTC go test -tags=integration -count=1 ./... 全绿输出"
        status: pass
    human_judgment: false

# Metrics
duration: 50min
completed: 2026-11-01
status: complete
---

# Phase 3 Plan 01: Casbin enforcer 装配 + 双轨写路径 + requirePerm 即时生效 Summary

**Casbin RBAC enforcer 进程内装配（casbin/v2 v2.135.0 + gorm-adapter v3.35.0）+ boot 对账全量导入 + RequirePerm fail-closed 中间件 + 四写路径双轨同步，同 token 分配→200/回收→403 即时生效全链集成锁定**

## Performance

- **Duration:** 50 min
- **Started:** 2026-11-01T04:04:28Z
- **Completed:** 2026-11-01T04:55:20Z
- **Tasks:** 2
- **Files modified:** 19（7 新建 + 12 修改，含 go.mod/go.sum）

## Accomplishments

- enforcer 五步装配链（TurnOffAutoMigrate→NewAdapterByDBUseTableName→NewModelFromString→NewSyncedEnforcer→LoadPolicy）进 app.New fail-fast；boot 对账从 sys_user_role/sys_role_menu 全量导入 g/p 策略（内存 map 去重防 Error 1869），存量 admin 冷启动即可用（T-3-05 缓解落地）
- RequirePerm 中间件三分支（401/403+10200/500 fail-closed 绝不放行）+ tracer 单路由 GET /api/v1/users 挂载（D-08 逐路由显式形态确立，03-03 全量收口的模板）
- 四写路径双轨同步：AssignMenus→SyncRolePerms、AssignRoles→SyncUserRoles、角色删除→ClearRole 双清、用户删除→ClearUser 先于 KickoutUser——全部位于业务事务提交成功后、失败上抛无吞错（D-13 纪律）
- matcher 三坑纯内存单测锁定（token 名一致性/super_admin 豁免首位短路零 p 行直证/撤销 g 行下一调用即失效）+ SyncRolePerms 幂等替换直证（T-3-03 critical 回归锁）
- 即时生效集成断言（PERM-02 核心）：TestPermHotReload 四步链 (b) 步复用 (a) 步同一 token（测试内无第二次 U 登录），分配→200/回收→403；双轨落库断言 PERM-04（casbin_rule p 行与 sys_role_menu 行同写同删）
- 全量集成回归 13 包全绿（Phase 1/2 既有测试在新挂载下零行为回归）

## Task Commits

Each task was committed atomically:

1. **Task 1: enforcer 装配 + boot 对账 + RequirePerm 中间件 + 首路由 tracer** - `d0b3d1f` (feat)
2. **Task 2: 四写路径双轨同步 + 即时生效断言 + matcher 单元锁定** - `40143b7` (feat)

**Plan metadata:** (见最终 docs commit)

_Tracer gate：Task 1 提交后端到端重跑全部三条 verify（build/vet/单测 + BootSmoke 集成 + grep 结构断言）全过，`⚡ Tracer verified end-to-end — expanding` 后进入 Task 2。_

## Files Created/Modified

- `internal/authz/enforcer.go` - SyncedEnforcer 包装 + 六同步 API + Authorizer 接口（全部注释引用决策编号）
- `internal/authz/perms.go` - 内嵌 RBAC model（三坑注释锁定）+ 27 个 perm 串常量（system: 前缀，auth/apiKey 域 6 串发明边界标注）
- `internal/authz/enforcer_test.go` - 纯内存 matcher 单测（免容器毫秒级）
- `internal/middleware/perm.go` - RequirePerm 三分支 fail-closed
- `internal/middleware/perm_test.go` - fake Authorizer 表格驱动四分支
- `internal/handler/perm_integration_test.go` - BootSmoke 冒烟 + HotReload 四步 + RevokeRole + DeleteCascades
- `scripts/ddl/casbin_rule.sql` - 显式 DDL（UNIQUE KEY idx_casbin_rule 自带，id 自增全库唯一例外注明）
- `internal/app/app.go` - enforcer 装配 + boot 对账（Redis Ping 后、service 构造前，双 fail-fast）
- `internal/app/router.go` - az 参数注入 + tracer 首路由
- `internal/repository/relation.go` - FindUserRoleCodes/FindRolePerms 对账查询（JOIN 模板 + 去重）
- `internal/service/role.go` / `internal/service/user.go` - 双轨同步挂点 + 去重防御
- `internal/handler/user_integration_test.go` - newTestApp 种子三件套先于 app.New（生产冷启动形态）

## Decisions Made

- sub 采用 role_code（不可改 + builtin super_admin 恒定 + 会话快照同轴）——策略行不因改名失效
- super_admin 豁免唯一判据在 enforcer g 层（会话 roles 快照判据废弃，D-07；撤销下一请求即失效由 TestRevokeSuperAdminImmediate 回归锁定）
- enforcer 同步放业务事务外（adapter 独立 Session 物理上不在事务内），失败上抛 500 重试自愈——重试=幂等全删全加即收敛
- boot 对账 g/p 双 map 去重 + repo FindRolePerms 去重（Error 1869 两道防御）
- casbin_rule 唯一索引 DDL 自带（TurnOffAutoMigrate 后 adapter 不建任何索引，probe 实证）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] SyncedEnforcer.GetPolicy 实际返回 ([][]string, error)**
- **Found during:** Task 1（enforcer.go ReplaceAll 首次编译）
- **Issue:** plan/RESEARCH 的 ReplaceAll 伪代码按单返回值写 `existingP := e.inner.GetPolicy()`——SyncedEnforcer 的 GetPolicy/GetGroupingPolicy 均带 error 返回（普通 Enforcer 才是单返回值），编译失败 assignment mismatch
- **Fix:** 改为 `existingP, err := e.inner.GetPolicy()` 并对 err 上抛
- **Files modified:** internal/authz/enforcer.go
- **Verification:** go build ./... 通过
- **Committed in:** d0b3d1f

**2. [Rule 1 - Bug] 五个既有集成测试适配新挂载与种子基线**
- **Found during:** Task 2 全量回归（5 个既有测试 FAIL）
- **Issue:** ① TestDeleteUserKicksSessions/TestUserStatusDisableKickout/TestResetPasswordFlow 用无角色用户 token 探活 GET /api/v1/users——该端点本 plan 挂上 requirePerm gate 后无角色用户被授权层 403 污染测试的认证语义；② TestKickoutFlow/TestKickoutRequiresSuperAdmin 自 seed super_admin 角色——newTestApp 现内建同 code 种子，撞 uk_role_code 1062；③ TestRoleListFilter 全量角色计数基线 3→4（种子多一行）
- **Fix:** ① 探活端点改未挂 gate 的详情端点 GET /api/v1/users/:id（认证可用性断言语义不变，均在注释注明 03-01 起原因）；② 删除测试内冗余 seedRole+attachRoleToUser（newTestApp 三件套已提供等价前提）；③ 计数基线 3→4 且排序断言位移（种子 id 恒小于测试 fixture——雪花先创建先生成）
- **Files modified:** internal/handler/user_extension_integration_test.go, auth_integration_test.go, role_integration_test.go
- **Verification:** 六测试单独重跑全过 + 全量集成回归 13 包全绿
- **Committed in:** 40143b7

**3. [Rule 3 - Blocking] service 包既有单测构造签名同步**
- **Found during:** Task 2 全量回归（internal/service build failed）
- **Issue:** NewUserService 签名扩展（+enf 参数）后 internal/service/user_integration_test.go 旧调用编译失败
- **Fix:** 构造调用补传 nil（该测试仅走 Create/Update 路径不触达 sm/enf，注释同步更新）
- **Files modified:** internal/service/user_integration_test.go
- **Verification:** go vet -tags=integration ./internal/service/ 通过 + 全量回归 ok
- **Committed in:** 40143b7

---

**Total deviations:** 3 auto-fixed（2 bug + 1 blocking；全部实现细节/测试适配级，零范围蔓延）
**Impact on plan:** 均为 API 实际签名与既有测试对新挂载语义的必要适配，不改变 plan 任何验收语义。Task 2 执行前 perm_integration_test.go 曾短暂引入 unused helper（dualTrackCount 死 stub）与 `AS rows` 保留字别名，均在提交前自查发现并清除，最终代码无 stub。

## Issues Encountered

- 集成测试 per-test 独立容器使全量回归单次约 10 分钟（581s handler 包）——既有基建成本，非本 plan 引入，未处理
- 无其他阻塞问题

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- enforcer/Authorizer/RequirePerm/perm 常量/双轨同步 API 全部就位，03-02（datascope 五档 + sys_role_dept）可直接开工——与本 plan 无文件耦合（app.go 构造传参点已预留 enf）
- 03-03 全量路由收口照 RequirePerm 逐路由挂载形态 + 03-RESEARCH 挂载清单表（34 路由）执行即可；RequireRoles 过渡 gate 届时移除（kickout 叠加保留）
- casbin_rule 表 DDL 已进 scripts/ddl/，testutil.execDDLDir 自动加载，测试容器零额外配置
- 已知边界留档：auth/apiKey 域 6 串为发明 perm（sys_menu 无对应菜单行，v1 仅 super_admin 可管）；菜单删除后 p 策略 perm 串残留（RESEARCH Assumption A5，v1 可接受）

---
*Phase: 03-casbin*
*Completed: 2026-11-01*

## Self-Check: PASSED

- 7 个 created 文件全部存在（含 SUMMARY 本体）
- 2 个任务提交 d0b3d1f / 40143b7 均为 HEAD 祖先
- 任务的全部 `<verify>` 与 plan 级 `<verification>` 均已执行且通过（单测/集成/grep 结构断言/全量回归 13 包）
