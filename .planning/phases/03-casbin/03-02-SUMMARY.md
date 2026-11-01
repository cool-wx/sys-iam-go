---
phase: 03-casbin
plan: 02
subsystem: authz
tags: [datascope, row-filtering, sys_role_dept, data-permission, gorm-scope, perm-03]

# Dependency graph
requires:
  - phase: 03-casbin
    plan: 01
    provides: RequirePerm 中间件（scopes 两路由挂载形态）、perm 常量（PermRoleEdit/PermRoleQuery）、boot 对账与 newTestApp 种子三件套基建
  - phase: 02-rbac
    provides: sys_role_menu 关联表同构模式（AssignMenus 全删重建/冗余列写法）、会话身份（auth.FromContext）、dept 树语义（tree.go 孤儿/环锁定）
provides:
  - internal/datascope 通用件（D-12 Phase 4 各域复用）：Mode 五档常量 + Filter.Apply（table 参数化 GORM WHERE 注入）+ Widest 档位聚合 + CollectSubtreeIDs 内存子树展开
  - sys_role_dept 全链（DDL/RoleDeptPO/RelationRepo 四方法/DeptRepo.FindByIDs）+ PUT/GET /api/v1/roles/:id/scopes 端点（RequirePerm 挂载）
  - 角色删除四连删（user_role→role_menu→role_dept→role，D-10 第三删）
  - UserRepo.List filter 注入（Count/Find 同闭包，P15）+ UserService.resolveScope 全链 fail-closed Resolve 编排
affects: [03-03-route-lockdown, phase-4-domain-reuse（各域列表接 Filter.Apply 仅需传表名常量）]

# Actuals (#2632) — chars/4 over realized diff（65277 chars），非 harness token 计数
actuals:
  tokens: 16319
  tasks: 2
  commits: 2
  plan_head_before: 82f3b64eb0e45a463bc40bcd1933f9efa6c49563
  plan_head_after: 7ccbbf8b6b73a87f815a3f11acff7243ec7b0bfb

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "datascope 通用件形态：service 传档位（resolveScope 编排）→ repo 闭包内 filter.Apply（Count/Find 同闭包防 total 漂移，P15）——Phase 4 各域照此接入，表名传代码常量"
    - "fail-closed 三兜底：未知档/空部门集 → '1 = 0' 空结果（case 首行 len 守卫是正确性要求非风格）；无角色/全 NULL → ModeSelf 最窄档"
    - "GORM DryRun 免容器 SQL 断言基建：mysql dialector + SkipInitializeWithVersion + DisableAutomaticPing（gorm.Open 默认 Ping 会触碰连接）"
    - "档位/部门集变更即时生效走表驱动实时查询（无缓存）——与 PERM-02 即时语义同轴"

key-files:
  created:
    - internal/datascope/datascope.go
    - internal/datascope/datascope_test.go
    - scripts/ddl/sys_role_dept.sql
  modified:
    - internal/model/po/relation.go（RoleDeptPO）
    - internal/model/dto/role.go（AssignScopesReq）
    - internal/repository/relation.go（四方法 + RoleScope + FindRoleScopesByUserIDs）
    - internal/repository/dept.go（FindByIDs）
    - internal/repository/user.go（List 签名扩 filter）
    - internal/service/role.go（deptRepo 依赖 + AssignScopes/GetScopeDeptIDs + 四连删）
    - internal/service/user.go（resolveScope + List 挂接）
    - internal/handler/role.go（AssignScopes/GetScopes）
    - internal/app/router.go（scopes 两路由）
    - internal/app/app.go（NewRoleService 装配传参）
    - internal/handler/role_integration_test.go（TestRoleAssignScopes + TestRoleCRUD 四连删）
    - internal/handler/user_integration_test.go（super_admin data_scope=5 + seedDeptChild）
    - internal/handler/auth_integration_test.go（seedRoleWithScope/seedUserWithDeptCreator）
    - internal/handler/perm_integration_test.go（TestUserDataScope）

key-decisions:
  - "super_admin 不做档位特判——严格表驱动（03-02 裁决记录 1）：newTestApp 种子 super_admin data_scope=5 保 Phase 2 全量列表行为；生产存量 NULL 行修正走既有 PUT /roles/:id（现场部署清单项）"
  - "档位 4 管理面取独立子资源端点 PUT/GET /roles/:id/scopes（sys_role_menu 同构先例，全删重建语义自洽）而非扩 PATCH 白名单——存储裁决遵 D-10 不变"
  - "'无角色 fail-closed' 经授权面的可观察形态 = 仅 NULL 档角色持 list perm → 本人档（纯无角色被 RequirePerm 403 挡在授权层，无法经 API 观察档位）"
  - "Widest 对域外档位值按 max 语义原样参与——经 Apply default 分支落 '1 = 0'，与档位枚举双保险兜底（T-3-07/T-3-09）"

patterns-established:
  - "dataScope 域接入模板：resolveScope 编排（identity→查询者行→FindRoleScopesByUserIDs→Widest→部门集装配）+ repo 闭包 filter.Apply——Phase 4 各域复制此骨架"
  - " scopes 子资源端点模板：GET/PUT /:res/:id/scopes 全删重建（照 roles/:id/menus）+ RequirePerm(query/edit) 逐路由挂载"

requirements-completed: [PERM-03]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: sys_role_dept 存储域（DDL 同构 8 列 + RoleDeptPO + repo 四方法）+ scopes 端点全删重建契约 + 角色删除四连删
    requirement: PERM-03
    verification:
      - kind: integration
        ref: "tests/internal/handler/role_integration_test.go#TestRoleAssignScopes（分配/覆盖/缺失 400/空回收零行/冗余列/403/删除零残留）"
        status: pass
      - kind: integration
        ref: "tests/internal/handler/role_integration_test.go#TestRoleCRUD（四连删 sys_role_dept 零残留断言）"
        status: pass
      - kind: other
        ref: "grep -c \"middleware.RequirePerm(\" internal/app/router.go == 3"
        status: pass
    human_judgment: false
  - id: D2
    description: datascope 五档通用件（Apply case 守卫 + Widest max 聚合 + CollectSubtreeIDs 防环 BFS）
    requirement: PERM-03
    verification:
      - kind: unit
        ref: "tests/internal/datascope/datascope_test.go#TestWidest（[nil,nil]→ModeSelf、[2,5]→5 非 min 方向回归锁）"
        status: pass
      - kind: unit
        ref: "tests/internal/datascope/datascope_test.go#TestFilterApplyClauses（未知档 0/99 与三空集 1=0、占位符、IN、table 参数化、Mode2 空集不 panic）"
        status: pass
      - kind: unit
        ref: "tests/internal/datascope/datascope_test.go#TestCollectSubtreeIDsThreeLayers|OrphanAbsent|CycleFinite（2s 超时纪律）"
        status: pass
    human_judgment: false
  - id: D3
    description: 用户列表 Resolve 挂接（Filter 注入 Count/Find 同闭包，total 与行集同过滤 P15）
    requirement: PERM-03
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestUserDataScope（每档断言内嵌 total==len(items) 同闭包断言）"
        status: pass
      - kind: other
        ref: "code 结构断言：UserRepo.List 的 q() 闭包内统一 filter.Apply（Count 与 Find 同一闭包构造）"
        status: pass
    human_judgment: false
  - id: D4
    description: 五档行差异集成锁定（跨部门不可见/多角色 max/NULL 档 fail-closed/档位变更即时同 token）
    requirement: PERM-03
    verification:
      - kind: integration
        ref: "tests/internal/handler/perm_integration_test.go#TestUserDataScope（档 1..5 行集互异 + 档 2 不含 A/B + 档2+档5→全量 + NULL 档→本人 + 2→5 下一请求全量）"
        status: pass
    human_judgment: false
  - id: D5
    description: 全量回归——既有测试在新过滤下全绿（super_admin 种子 data_scope=5 生效）
    verification:
      - kind: integration
        ref: "TZ=UTC go test -tags=integration -count=1 -timeout 30m ./... 全绿输出（14 包 ok，handler 601s）"
        status: pass
    human_judgment: false

# Metrics
duration: 36min
completed: 2026-11-01
status: complete
---

# Phase 3 Plan 02: dataScope 五档行过滤 + sys_role_dept 存储 + 用户列表挂接 Summary

**dataScope 五档通用件（internal/datascope：Apply fail-closed 注入 + Widest max 聚合 + 子树 BFS）+ sys_role_dept 存储全链（scopes 端点/四连删）+ 用户列表 Resolve 挂接，五档行差异/跨部门不可见/多角色取宽/变更即时全链集成锁定（PERM-03 交付）**

## Performance

- **Duration:** 36 min
- **Started:** 2026-11-01T04:59:58Z
- **Completed:** 2026-11-01T05:36:11Z
- **Tasks:** 2
- **Files modified:** 17（3 新建 + 14 修改）

## Accomplishments

- sys_role_dept 存储域全链：DDL（sys_role_menu 同构 8 列，纯新增零 ALTER）+ RoleDeptPO（雪花+双时间）+ RelationRepo 四方法（Delete/BatchCreate/FindByID/FindByIDs 映射）+ DeptRepo.FindByIDs；PUT/GET /api/v1/roles/:id/scopes 两路由挂 RequirePerm（edit/query）
- AssignScopes 全删重建事务（角色 404→dept 缺失 400「部门不存在: [...]」→事务，dept_name 查库值防伪造 T-2-15）；角色删除三连删扩四连删（第三删 DeleteRoleDepts，D-10）
- internal/datascope 通用件（D-12）：Filter.Apply 五档分派 + 未知档/空部门集 "1 = 0" fail-closed（case 首行 len 守卫防 Mode2 空集 panic）；Widest 多角色 max 聚合（[2,5]→5，否决 03-PATTERNS 第 8 节 min 笔误并单测方向回归锁）；CollectSubtreeIDs 内存 BFS 子树展开（含根、孤儿缺席、visited 防环 2s 超时直证）
- 免容器单测基建：GORM DryRun（mysql dialector + SkipInitializeWithVersion + DisableAutomaticPing）断言五档 SQL 子句——占位符形态、IN 逐元素不内联、"1 = 0"、table 参数化（Phase 4 复用契约直证）
- 用户列表 Resolve 挂接：UserService.resolveScope（identity→查询者行→FindRoleScopesByUserIDs→Widest→档 3 子树展开/档 4 多角色集并集去重）→ UserRepo.List 签名扩 filter，Count/Find 同 q() 闭包 Apply（P15 防 total 漂移）
- TestUserDataScope（PERM-03 验收核心）：dept 三层树 + D2 数据集上五档行集互异断言（档 2 跨部门不可见验收明文）、多角色取宽（档 2+5→全量）、NULL 档 fail-closed（本人档）、total==items、档位 2→5 变更同 token 下一请求即全量（无缓存直证）
- 全仓集成回归 14 包 ok（TestListUsersPaging/EdgeParams 等既有列表断言在新过滤下零回归）

## Task Commits

Each task was committed atomically:

1. **Task 1: sys_role_dept 存储域 + scopes 端点 + 角色删除四连删** - `9f91940` (feat)
2. **Task 2: datascope 五档通用件 + 用户列表 Resolve 挂接 + 五档行差异断言** - `7ccbbf8` (feat)

## Files Created/Modified

- `internal/datascope/datascope.go` - Mode 五档常量 + Filter.Apply + Widest + CollectSubtreeIDs（全注释引用 D-09..D-12/T-3-06..T-3-09）
- `internal/datascope/datascope_test.go` - Widest 表格驱动 + 子树三案例 + Apply DryRun SQL 断言（免容器毫秒级）
- `scripts/ddl/sys_role_dept.sql` - 档位 4 部门集合存储 DDL（D-10 裁决方案 A，增量扩展例外族注明；testutil 自动加载零改动）
- `internal/model/po/relation.go` / `internal/model/dto/role.go` - RoleDeptPO + AssignScopesReq
- `internal/repository/relation.go` - sys_role_dept 四方法 + RoleScope 投影 + FindRoleScopesByUserIDs（NULL 档位指针保留）
- `internal/repository/dept.go` / `internal/repository/user.go` - FindByIDs + List filter 注入
- `internal/service/role.go` / `internal/service/user.go` - AssignScopes/GetScopeDeptIDs/四连删 + resolveScope 编排
- `internal/handler/role.go` / `internal/app/router.go` / `internal/app/app.go` - 两 handler + 两路由 + 装配传参
- `internal/handler/{role,user,auth,perm}_integration_test.go` - TestRoleAssignScopes/TestUserDataScope + 种子 helper 四件（seedRoleWithScope/seedUserWithDeptCreator/seedDeptChild/super_admin data_scope=5）

## Decisions Made

- super_admin 不做档位特判——严格表驱动；测试种子 super_admin data_scope=5（03-02 裁决记录 3），生产存量 NULL 行修正留现场部署清单（既有 PUT /roles/:id 即可）
- 档位 4 管理面为独立子资源端点（D-10 同构先例），全删重建语义与 AssignMenus 自洽
- "无角色 fail-closed" 的可观察形态 = 仅 NULL 档角色持 list perm → 本人档（纯无角色在 RequirePerm 层即 403，档位语义不可经 API 观察）
- Widest 域外值原样参与 max——Apply default 分支兜底 "1 = 0"（枚举+过滤双保险）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] go test 默认 10m 包超时杀掉全量回归**
- **Found during:** Task 2 全量回归（首次后台跑 handler 包 FAIL 600.049s）
- **Issue:** 03-01 时 handler 包已 581s 贴近默认 -timeout 10m；本 plan 新增两集成测试（各 ~13s）后超限，go test 包级 10m 默认超时整体判 FAIL
- **Fix:** 全量回归显式 `-timeout 30m` 重跑——14 包全 ok（handler 601.189s）
- **Files modified:** 无代码改动（测试命令参数）
- **Verification:** 全仓回归 EXIT=0
- **Committed in:** 7ccbbf8（提交信息注明）

**2. [Rule 1 - Bug] gorm.Open 默认自动 Ping 触碰连接，DryRun 免容器断言必挂**
- **Found during:** Task 2 datascope 单测首跑（dial tcp 127.0.0.1:1: connection refused）
- **Issue:** gorm.Open 默认 DisableAutomaticPing=false——即便 SkipInitializeWithVersion 免版本查询，Open 仍对不可达 DSN 发起 Ping；DryRun 只保证 Find 不执行，不保证 Open 不连接
- **Fix:** 测试 gorm.Config 加 `DisableAutomaticPing: true`（SkipDefaultTransaction 顺带对齐生产配置）
- **Files modified:** internal/datascope/datascope_test.go
- **Verification:** go test ./internal/datascope/ ok
- **Committed in:** 7ccbbf8

**3. [Rule 1 - Bug] Widest 入参类型接驳：repository.RoleScope ≠ []*int8**
- **Found during:** Task 2 service 编译（cannot use scopes[...] as []*int8）
- **Issue:** plan 伪代码 Widest(scopes []*int8) 与 FindRoleScopesByUserIDs 返回 map[int64][]RoleScope 两契约未显式接驳——service 需从投影提取档位指针切片
- **Fix:** resolveScope 内 roleScopes→dsPtrs 提取后传 Widest（投影结构保留 RoleID 供档 4 角色集查询，两用一查）
- **Files modified:** internal/service/user.go
- **Verification:** go build/vet 通过
- **Committed in:** 7ccbbf8

**4. [Rule 3 - Blocking] 集成测试 const 整数不可取址（&d1 编译失败）**
- **Found during:** Task 2 TestUserDataScope 集成 vet（invalid operation: cannot take address of d1）
- **Issue:** 种子部门 ID 声明为 const 后 &d1 取址非法（seedUserWithDeptCreator 需要 *int64）
- **Fix:** const 改 var 声明
- **Files modified:** internal/handler/perm_integration_test.go
- **Verification:** 集成 vet 通过 + 测试 PASS
- **Committed in:** 7ccbbf8

### 计划文本指向澄清（非代码偏差）

- plan verify 模式中的 `TestRoleDelete`/`TestUserList` 测试名在现有代码中不存在：角色删除断言实际位于 **TestRoleCRUD**（已扩展 sys_role_dept 四连删零残留直证，并在 TestRoleAssignScopes 末尾补删除持有 scopes 角色的零残留直证）；既有列表全量断言实际为 **TestListUsersPaging/TestListUsersEdgeParams**（全量回归 14 包 ok 覆盖）——验收语义全部落地，仅测试名指向修正。

---

**Total deviations:** 4 auto-fixed（2 bug + 2 blocking；全部实现细节/测试基建级，零范围蔓延）
**Impact on plan:** 不改变任何验收语义。Task 2 执行前 datascope_test.go 曾短暂存在 buildSQL 错误断言写法（require.NoError 包裹布尔表达式）与 const 取址编译错误，均在提交前自查/首轮验证发现并清除，最终代码无 stub、无残留。

## Issues Encountered

- 集成测试 per-test 独立容器使全量回归单次约 10 分钟（handler 包 601s）——既有基建成本，非本 plan 引入；go test 默认包超时 10m 与该成本冲突，后续 plan 全量回归需显式 `-timeout 30m`（已在本 plan 验证命令中生效）
- 无其他阻塞问题

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- internal/datascope 通用件就位（table 参数化无 user 域耦合），03-03 全量路由收口与 Phase 4 各域列表接入仅按"resolveScope 编排 + repo 闭包 Apply"模板复制即可
- scopes 两路由已挂 RequirePerm（grep 基线 3），03-03 全量收口时按挂载清单表照常推进
- 现场部署清单新增一项（03-02 裁决记录 3）：生产存量 super_admin 角色行 data_scope=NULL 需经 PUT /roles/:id 置 5（否则管理员列表回退本人档）；测试基建已内建档 5 种子
- go test 集成回归命令需带 `-timeout 30m`（handler 包 ~600s 超默认值）

---

*Phase: 03-casbin*
*Completed: 2026-11-01*

## Self-Check: PASSED

- 3 个 created 文件全部存在（datascope.go/datascope_test.go/sys_role_dept.sql）
- 2 个任务提交 9f91940 / 7ccbbf8 均为 HEAD 祖先
- 任务的全部 `<verify>` 与 plan 级 `<verification>` 均已执行且通过（单测/scopes 集成/五档集成/全仓回归 14 包/grep 结构断言 3）
