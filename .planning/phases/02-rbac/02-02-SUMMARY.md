---
phase: 02-rbac
plan: 02
subsystem: rbac
tags: [menu, dept, role, tree-builder, orphan-promotion, builtin-guard, transaction, gin, gorm, assign-menus]

# Dependency graph
requires:
  - phase: 02-rbac-01
    provides: 认证中间件 pri 组（新路由挂入即受保护）、handlers 聚合 struct 扩展位、RelationRepo（FindRolesByUserIDs）、po/role.go + po/relation.go 关联表 PO、testutil 双容器 + seedUser/loginAs helper、errcode 分段模式
  - phase: 01-foundation
    provides: repo.Patch 模板（map 白名单+CR-01）、UserPO/BaseWithOperator 模板、paging.Parse/PageData、ids/types 契约、resp envelope
provides:
  - internal/tree 泛型构建器 Build[T]（孤儿提升为根单点实现——menu/dept 两域共用；环引用天然免疫）
  - Menu/Dept 域纵切全套（PO/DTO/repo/service/handler）：CRUD + GET tree + 父校验（父菜单不存在/上级部门不存在）+ parentName 服务端查库冗余 + 物理删不加引用检查（源缺口按源保留）
  - pkg/types MenuType 三值枚举（DIR/MENU/BUTTON，name|int 双形态，未知兜底 MENU）
  - Role 域纵切全套：CRUD（默认 dataScope=1/status=ENABLED/builtin=false）+ builtin 守卫 409/10602 + 三连删事务 + assignMenus 全量替换 + roleName trim LIKE 分页 id ASC（源特例）
  - errcode 409 段：10601 系统内置用户不可删除（02-03 User 域复用）/ 10602 系统内置角色不可删除
  - RelationRepo 写路径扩展：FindMenuIDsByRoleID / DeleteRoleMenus / BatchCreateRoleMenus / DeleteUserRolesByRoleID（tx 可注入复用）
  - 路由：/api/v1/{menus,depts}（CRUD+tree）与 /api/v1/roles（CRUD+GET/PUT :id/menus）全部挂 pri 组；gin 静态段 tree 与 :id 参数段共存实证
affects: [02-03 User 装配域（menuRepo.FindByIDs/roleRepo.FindByIDs/10601 复用）, Phase 3 Casbin（角色-菜单关联数据面）]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 28500    # chars/4 over the realized diff (113959 chars, 25 files)
  tasks: 2         # tasks completed
  commits: 3       # MEASURED: git rev-list --count gsd-plan-head-before-02-02..HEAD

tech-stack:
  added: []        # 零新增依赖（T-2-SC 达成：go.mod/go.sum 本 plan 未触碰）
  patterns:
    - 泛型树构建器（一次遍历建索引+children 分组 → 仅从根后序装配；attach 叶子空挂接归一化 children [] 非 null）
    - 多步关联写单事务：service 层 db.Transaction + repository.NewXxxRepo(tx) 复用（三连删/assignMenus 全删重建）
    - builtin 守卫前置拦截（零删除调用）+ errcode 单例 409 语义
    - repo 行不存在 → service 译域消息（「菜单/部门/角色不存在: {id}」，404/10300 code/HTTP 契约不变）

key-files:
  created:
    - internal/tree/tree.go
    - internal/tree/tree_test.go
    - pkg/types/menu_type.go
    - pkg/types/menu_type_test.go
    - internal/model/po/menu.go
    - internal/model/po/dept.go
    - internal/model/dto/menu.go
    - internal/model/dto/dept.go
    - internal/model/dto/role.go
    - internal/repository/menu.go
    - internal/repository/dept.go
    - internal/repository/role.go
    - internal/service/menu.go
    - internal/service/dept.go
    - internal/service/role.go
    - internal/handler/menu.go
    - internal/handler/dept.go
    - internal/handler/role.go
    - internal/handler/menu_dept_integration_test.go
    - internal/handler/role_integration_test.go
  modified:
    - internal/repository/relation.go（+4 关联表写方法）
    - pkg/errcode/errcode.go（10601/10602 + 409 映射）
    - pkg/errcode/errcode_test.go（409 case）
    - internal/app/router.go（handlers 聚合 +3 域 / pri 组 +19 路由）
    - internal/app/app.go（三域装配链）

key-decisions:
  - "环引用语义按源保真：plan 行为文本『两节点均落根』与源服务层 buildTree 及 plan 自身实现规格（map 未命中即根——环双方 parent 均命中索引）矛盾，判行为文本笔误——环节点不可达静默不产出，tree_test 直证 2s 内返回（不死循环）+ 根层排除"
  - "assignMenus 缺失消息按验收明文带方括号『菜单不存在: [999]』（源为『菜单不存在: 999, 888』无括号）——硬验收门优先，join ', ' 保持源分隔风格"
  - "assignMenus 校验前置（plan T-2-11）：先 menuRepo.FindByIDs 全量校验再开事务，优于源顺序（源在事务内先删后校验靠回滚兜底）——网络语义等价且免依赖回滚"
  - "menu/dept 行不存在消息对齐 plan truth：Get/Update 译『菜单不存在: {id}』/『部门不存在: {id}』（源消息原文），code 恒 10300/HTTP 404 与 Phase 1 契约一致——truth 文本按字面语义落地"
  - "menu/dept create 默认 visible=true/status=ENABLED（plan 未列，对齐 DDL default 与源 MP null-skip 语义——null 字段跳过 INSERT 由 DB default 生效）"
  - "update 移至根（parentId 显式 0）：parent_name 清 NULL——与 create 的 pid=0 语义对齐；源此处因 MP null-skip 残留旧值属脏数据态，有意修正"
  - "GetMenuIDs 不校验角色存在性（源 getRoleMenuIds 语义保真：未分配/不存在角色均返回空列表）"
  - "TDD RED 证据：Go 新包需编译性 stub（Build 恒 nil / MenuType 恒 DIR）使 11 个目标测试在计划断言上失败，分类器 RED_EVIDENCE_OK；targetTest 须用报告内裸名（包限定名不匹配）"

patterns-established:
  - "Pattern: 树查询域组装——repo.List(sort ASC,id ASC) → dto.ToXxxTreeResp 预置空 Children → tree.Build(三回调) → []*TreeResp；孤儿提升语义禁止在域内复刻"
  - "Pattern: 子资源全量替换（PUT /roles/:id/menus）——前置校验 + db.Transaction{Delete 全量 + BatchCreate 全量}，空数组=全回收"
  - "Pattern: 域消息翻译——repo 层 ErrNotFound 哨兵不变，service 层 errors.Is 判定后译域消息（code/HTTP 恒定）"

requirements-completed: [MENU-01, MENU-02, DEPT-01, DEPT-02, ROLE-01, ROLE-02, ROLE-03]

coverage:
  - id: D1
    description: 泛型树构建器——空列表非 nil、两代挂接、nil/0/孤儿提升根、环引用 2s 内返回且环节点不出根层、子节点序=输入序（T-2-09 直证）
    requirement: MENU-02
    verification:
      - kind: unit
        ref: "internal/tree/tree_test.go#TestBuildOrphanParentPromotedToRoot"
        status: pass
      - kind: unit
        ref: "internal/tree/tree_test.go#TestBuildCycleDoesNotHang"
        status: pass
    human_judgment: false
  - id: D2
    description: MenuType 枚举契约——name|int 双形态读侧、未知 int 兜底 MENU 不报错不 500、Marshal name 输出
    verification:
      - kind: unit
        ref: "pkg/types/menu_type_test.go#TestMenuTypeUnmarshalUnknownFallsBackToMenu"
        status: pass
      - kind: unit
        ref: "pkg/types/menu_type_test.go#TestMenuTypeFromIntFallback"
        status: pass
    human_judgment: false
  - id: D3
    description: 菜单 CRUD 全链（MENU-01）——create 父校验 400「父菜单不存在: 999」、parent_name 服务端查库冗余（伪造键被忽略）、patch 白名单、删除带子节点仍成功（源缺口保留）、不存在 404 域消息、无 token 401
    requirement: MENU-01
    verification:
      - kind: integration
        ref: "internal/handler/menu_dept_integration_test.go#TestMenuCRUD"
        status: pass
    human_judgment: false
  - id: D4
    description: 菜单树（MENU-02）——根→子→孙嵌套、parentId=999 孤儿与 parentId=NULL 根均在根层、叶子 children [] 非 null
    requirement: MENU-02
    verification:
      - kind: integration
        ref: "internal/handler/menu_dept_integration_test.go#TestMenuTree"
        status: pass
    human_judgment: false
  - id: D5
    description: 部门 CRUD 全链（DEPT-01）——与菜单同构，父校验「上级部门不存在: 888」、parent_name 冗余、物理删
    requirement: DEPT-01
    verification:
      - kind: integration
        ref: "internal/handler/menu_dept_integration_test.go#TestDeptCRUD"
        status: pass
    human_judgment: false
  - id: D6
    description: 部门树孤儿提升（DEPT-02 验收明文）——parentId=777 孤儿提升为根层，子代嵌套正确
    requirement: DEPT-02
    verification:
      - kind: integration
        ref: "internal/handler/menu_dept_integration_test.go#TestDeptTreeOrphanPromotion"
        status: pass
      - kind: unit
        ref: "internal/tree/tree_test.go#TestBuildOrphanParentPromotedToRoot"
        status: pass
    human_judgment: false
  - id: D7
    description: 角色 CRUD + 三连删事务（ROLE-01）——默认值 dataScope=1/ENABLED/builtin=false、编码重复 400、删除后 sys_user_role/sys_role_menu 零残留、404 域消息
    requirement: ROLE-01
    verification:
      - kind: integration
        ref: "internal/handler/role_integration_test.go#TestRoleCRUD"
        status: pass
    human_judgment: false
  - id: D8
    description: builtin 守卫（ROLE-02/D-14）——DELETE builtin 角色 HTTP 409 + code 10602 + 零删除证明（其后仍可 GET）
    requirement: ROLE-02
    verification:
      - kind: integration
        ref: "internal/handler/role_integration_test.go#TestRoleBuiltinDeleteConflict"
        status: pass
    human_judgment: false
  - id: D9
    description: 角色分页条件查询（ROLE-03 源特例）——roleName trim 后 LIKE（空格包裹仍命中）、items id 升序、无参数钳位 1/15、未命中 total 0 items []
    requirement: ROLE-03
    verification:
      - kind: integration
        ref: "internal/handler/role_integration_test.go#TestRoleListFilter"
        status: pass
    human_judgment: false
  - id: D10
    description: 角色菜单分配——全量替换（第二次分配后 GET 精确等于第二次提交集）、缺失 400「菜单不存在: [999]」且不动存量、空数组全回收、role_name/menu_name 冗余落库、GET 返回 menuId 列表（T-2-11）
    requirement: ROLE-03
    verification:
      - kind: integration
        ref: "internal/handler/role_integration_test.go#TestRoleAssignMenus"
        status: pass
    human_judgment: false

# Metrics
duration: 35min
completed: 2026-10-31
status: complete
plan_head_before: 77bb509052f1c2c9e2636d55a31bd14a7c2698d8
plan_head_after: 1b10844985eb845b7fc6b27e0ff11f6a52be5346
commits: 3
---

# Phase 02 Plan 02: 树构建器与 Menu/Dept/Role 基础数据域 Summary

**泛型树构建器（孤儿提升单点）+ Menu/Dept/Role 三域纵切全套：CRUD、树端点、builtin 守卫 409/10602、三连删与 assignMenus 单事务、roleName trim LIKE 分页源特例——零新增依赖，TDD RED→GREEN 双提交**

## Performance

- **Duration:** 35 min
- **Started:** 2026-10-31T14:04:53Z
- **Completed:** 2026-10-31T14:39:32Z
- **Tasks:** 2
- **Files modified:** 25（20 新建 + 5 修改）

## Accomplishments
- internal/tree.Build 单泛型函数：一次遍历建节点索引 + children 分组（nil/0/索引未命中 → 归根），仅从根后序装配；环引用天然免疫（不可达即终止，tree_test 2s 超时直证）；attach 叶子空挂接把 children 归一化 [] 非 null——MENU-02/DEPT-02 唯一实现点，两域共用
- Menu/Dept 域纵切：CRUD + GET tree + 父校验（「父菜单不存在: {pid}」/「上级部门不存在: {pid}」，父 name 为 NULL 同样拒绝——源 resolveParentName 逐字语义）+ parentName 服务端查库冗余 + 物理删不加引用检查（源缺口按源保留）
- pkg/types.MenuType：DIR/MENU/BUTTON name|int 双形态，未知值兜底 MENU（源 fromValue 契约形态）
- Role 域纵切：默认值三件（dataScope=1/ENABLED/builtin=false）、builtin 守卫前置拦截（409/10602 零删除）、db.Transaction 三连删（user_role→role_menu→role 源顺序）、assignMenus 前置校验+同事务全删重建+双名冗余、roleName trim LIKE + id ASC 分页（源 RoleRepositoryImpl 特例）
- errcode 409 段：10601（02-03 复用）/10602 + HTTPStatus 映射；RelationRepo 写路径四方法（tx 可注入）
- 路由装配：三域 19 条路由全部挂 pri 组（认证保护）；gin 静态段 tree 与 :id 参数段共存实证（TestMenuTree 即验证）
- 测试：单元 15 用例（tree 7 + menuType 4 + errcode 4）+ 集成 8 用例（menu/dept 4 + role 4）全绿；handler 全族集成 317s 全绿（Phase 1 回归含 auth/user）

## Task Commits

Each task was committed atomically:

1. **Task 1 (TDD RED): 泛型树构建器 + MenuType 失败测试（附编译性 stub）** - `81b64ba` (test)
2. **Task 1 (TDD GREEN): tree.Build 实现 + Menu/Dept 域纵切全套** - `6ca1be8` (feat)
3. **Task 2: Role 域纵切——CRUD + builtin 守卫 + 三连删事务 + assignMenus** - `1b10844` (feat)

_Note: Task 1 为 tdd="true"——RED 证据经 `gsd_run check tdd-red-evidence` 分类器（RED_EVIDENCE_OK：11 个 --- FAIL 全落目标断言，go-test 文本适配器）；RED→GREEN 顺序合规（feat 提交自 test 提交下降）。REFACTOR 无独立提交（无 cleanup 变更）。Task 2 非 TDD 单 feat 提交。_

## Files Created/Modified
- `internal/tree/tree.go` - 泛型树构建器（孤儿提升单点，包注释声明源语义与死代码警告）
- `pkg/types/menu_type.go` - MenuType 三值枚举契约
- `internal/model/po/{menu,dept}.go` - MenuPO 19 列 / DeptPO 13 列（对齐 scripts/ddl 实测）
- `internal/model/dto/{menu,dept,role}.go` - 三域 DTO 三件套 + TreeResp 递归嵌套 + ToXxxResp 转换
- `internal/repository/{menu,dept,role}.go` - Patch/List/FindByID 模板套用 + menu.FindByIDs + role List 特例
- `internal/repository/relation.go` - +FindMenuIDsByRoleID/DeleteRoleMenus/BatchCreateRoleMenus/DeleteUserRolesByRoleID
- `internal/service/{menu,dept,role}.go` - 父校验/树组装/builtin 守卫/事务编排/域消息翻译
- `internal/handler/{menu,dept,role}.go` - 薄层三段式 + parseID 泛化（按域文案）
- `internal/app/{router,app}.go` - handlers 聚合 +3 域、pri 组 19 路由、装配链
- `pkg/errcode/errcode.go` - 10601/10602 + 409 映射
- `internal/handler/{menu_dept,role}_integration_test.go` - 集成 8 用例（双容器）

## Decisions Made
- 环引用语义判 plan 行为文本笔误、按源保真（详见 Deviations #1）——环节点不归根不产出，不死循环直证
- 「菜单不存在: [逗号拼接]」按验收明文带方括号（源无括号）——硬验收门优先
- assignMenus 前置校验优于源"事务内先删后校验"顺序（T-2-11 计划要求，语义等价免回滚依赖）
- 行不存在 404 消息按 plan truth 落域消息原文，code/HTTP 契约与 Phase 1 一致（10300/404）
- menu/dept create 默认 visible/status 对齐 DDL default（源 MP null-skip 等价语义）
- update 显式 parentId=0 清 parent_name NULL（修正源 MP null-skip 残留旧值的脏数据态）
- GetMenuIDs 无角色存在性校验（源 getRoleMenuIds 语义保真）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] 环引用落根语义——plan 行为文本与源语义/自身实现规格矛盾**
- **Found during:** Task 1（tree_test 用例设计时源码核对）
- **Issue:** plan 行为写「环引用 A.parent=B、B.parent=A 两节点均落根」，但源 DeptApplicationService.java:89-107（push 进父对象单遍算法）与 plan 自身实现规格（"map 未命中 id → 归入 roots"）下环引用双方 parent 均命中索引，均不归根（不可达静默不产出）且无死循环；验收明文（硬门）亦只要求"环引用不死循环两断言"
- **Fix:** 按源语义实现；环测试断言 2s 内返回（goroutine+select 超时兜底）+ 环节点不出现在根层；行为文本冲突记入本 SUMMARY 供 UAT 复核
- **Files modified:** internal/tree/tree.go, internal/tree/tree_test.go
- **Verification:** TestBuildCycleDoesNotHang PASS（单元）
- **Committed in:** 81b64ba + 6ca1be8

**2. [Rule 2 - Missing Critical] menu/dept 行不存在消息缺域语义**
- **Found during:** Task 2（role service 实现「角色不存在: {id}」时回查 plan truth）
- **Issue:** Task 1 的 menu/dept Get/Update 对不存在行返回统一 ErrNotFound 文案「数据不存在」，plan truth 明文为『菜单不存在: {id}』/『部门不存在: {id}』（源消息原文）
- **Fix:** service 层 errors.Is(ErrNotFound) 判定后译域消息（code 恒 10300、HTTP 恒 404 与 Phase 1 契约一致）；集成测试补消息断言
- **Files modified:** internal/service/menu.go, internal/service/dept.go, internal/handler/menu_dept_integration_test.go
- **Verification:** TestMenuCRUD/TestDeptCRUD 消息断言 PASS
- **Committed in:** 1b10844

**3. [Rule 3 - Blocking] dept 树集成测试解包键错位**
- **Found during:** Task 1（TestDeptTreeOrphanPromotion 首跑失败）
- **Issue:** 测试解包结构复用菜单域 treeNode（name 键），部门域契约键为 deptName → Name 断言空串
- **Fix:** 新增 deptTreeNode 结构（deptName 键）
- **Files modified:** internal/handler/menu_dept_integration_test.go
- **Verification:** TestDeptTreeOrphanPromotion PASS
- **Committed in:** 6ca1be8

---

**Total deviations:** 3 auto-fixed（1 Rule 1 语义裁决 + 1 Rule 2 契约补齐 + 1 Rule 3 测试阻塞）
**Impact on plan:** 三处均为 plan 文本内部矛盾或测试实现细节的收口，无范围蔓延；环引用语义与源/威胁模型 T-2-09 完全一致。

## Issues Encountered
- TDD RED 分类器首跑 INVALID_RED（no_target_test_failure）：targetTest 用包限定名 `internal/tree.TestXxx` 无法匹配 go-test 报告内裸名——改用报告精确裸名后 RED_EVIDENCE_OK（分类器行为留档，后续 TDD plan 直接用裸名）
- TestRoleAssignMenus 首跑断言笔误：误断言消息含非缺失 id（990302）——源 missingIds 语义只报缺失集合，改为 NotContains 锁定语义
- gofmt 发现 02-01 遗留的 auth_integration_test.go / po/api_key.go 未格式化——非本 plan 触碰文件，按范围边界不处理（留待后续 plan 顺手收口）

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 02-03（User 装配域）依赖就绪：menuRepo.FindByIDs / roleRepo.FindByIDs（用户角色装配）、deptRepo.FindByID（user→dept 存在校验）、errcode 10601 单例（USER-07 builtin 用户删除守卫）、relation repo 写方法（PUT /users/:id/roles 可直接套 assignMenus 模式）
- 三域数据面就绪：Phase 3 Casbin 可基于 sys_role_menu 关联行（role_name/menu_name 冗余齐备）构建策略
- 会话 roles 载荷（02-01）+ 三域 CRUD 主链路合流——RBAC 主链路仅剩 User 装配域（02-03）与 ApiKey 域
- 无阻塞项

---
*Phase: 02-rbac*
*Completed: 2026-10-31*

## Self-Check: PASSED

- 20/20 关键新建文件存在于磁盘（`[ -f ]` 全过）
- 3/3 任务提交存在于 HEAD 祖先链（81b64ba RED / 6ca1be8 GREEN / 1b10844 feat）
- 实测提交数：`git rev-list --count gsd-plan-head-before-02-02..HEAD` = 3（与 frontmatter `commits: 3` 一致）
- 全部 `<verify>` 复跑通过：build/vet 绿、errcode/tree/types 单元绿、handler 全族集成 317s 全绿（auth/menu/dept/role + Phase 1 回归）
- requirements.ready-ids 门：7 项需求全部 ready（02-03 无共享 ID），已 mark-complete
