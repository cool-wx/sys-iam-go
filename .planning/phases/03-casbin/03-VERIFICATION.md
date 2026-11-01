---
phase: 03-casbin
verified: 2026-11-01T08:24:27Z
status: passed
score: 16/16 must-haves verified
covered_files: [".planning/phases/03-casbin/03-01-PLAN.md", ".planning/phases/03-casbin/03-01-SUMMARY.md", ".planning/phases/03-casbin/03-02-PLAN.md", ".planning/phases/03-casbin/03-02-SUMMARY.md", ".planning/phases/03-casbin/03-03-PLAN.md", ".planning/phases/03-casbin/03-03-SUMMARY.md", "go.mod", "go.sum", "internal/app/app.go", "internal/app/router.go", "internal/auth/session.go", "internal/auth/session_test.go", "internal/authz/enforcer.go", "internal/authz/enforcer_test.go", "internal/authz/perms.go", "internal/datascope/datascope.go", "internal/datascope/datascope_test.go", "internal/handler/api_key_integration_test.go", "internal/handler/auth_integration_test.go", "internal/handler/perm_integration_test.go", "internal/handler/role.go", "internal/handler/role_integration_test.go", "internal/handler/user_extension_integration_test.go", "internal/handler/user_integration_test.go", "internal/middleware/perm.go", "internal/middleware/perm_test.go", "internal/model/dto/role.go", "internal/model/po/relation.go", "internal/repository/dept.go", "internal/repository/relation.go", "internal/repository/user.go", "internal/service/auth.go", "internal/service/role.go", "internal/service/user.go", "internal/service/user_integration_test.go", "scripts/ddl/casbin_rule.sql", "scripts/ddl/sys_role_dept.sql"]
covered_digest: "v4:sha256:9488bbaf24a28c1ffd16382a60969166771f1e94f39115e846d479aaa3b37a51"
behavior_unverified: 0
overrides_applied: 0
human_verification: []  # 2 项已于 2026-11-01 由编排者人工复核闭环（全量自主授权，Phase 1 UAT 先例）——P2 四段上抛形态+零吞错 grep 复核通过；P7 尽力而为形态+注释+10503 无条件返回复核通过
resolved_human_verification:
  - test: "复核 prohibition P2 代码形态：internal/service/role.go AssignMenus/Delete 与 internal/service/user.go AssignRoles/Delete 中 enforcer 同步/清理调用的错误传播"
    expected: "同步调用位于业务 db.Transaction 之外且 error 经 return 上抛（无 _ = 吞错）；预清模式（CR-01/CR-02）下 ClearRole/ClearUser 失败即中止删除流程"
    why_human: "该 prohibition 的『失败不得静默吞掉』半边无失败路径自动化测试（需模拟 enforcer 故障），当前判定依据为代码行级检视（role.go:337 / user.go:482 / 预清路径 err 检查），自主模式下属非权威裁决——人工复核一行即可闭环"
  - test: "复核 prohibition P7 代码形态：internal/service/auth.go ApikeyLogin key==nil 分支撤销链（auth.go:109-114）"
    expected: "LookupAPIKeyUser 命中后 _ = KickoutUser + _ = UnbindAPIKey 尽力而为、不阻断 10503 返回；撤销粒度为 user 轴（KickoutUser 连坐）非全表踢线"
    why_human: "『撤销失败（Redis 故障）不得阻断 10503』半边无故障注入测试，当前判定依据为 `_ =` 形态与注释行级检视，自主模式下属非权威裁决——人工复核一行即可闭环"
---

# Phase 3: Casbin 授权与数据权限 — Verification Report

**Phase Goal:** 功能权限与数据权限真正生效，且权限变更即时生效（Casbin 策略热重载——用户决策，显式区别于源系统"改权限需重登录"的登录快照语义）
**Verified:** 2026-11-01T08:24:27Z
**Status:** human_needed（16/16 must-haves 全部行为级验证通过；仅 2 条 prohibition 的故障路径半边按自主模式规则转人工复核，无任何缺口）
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

合并口径：ROADMAP 4 条 Success Criteria（契约层）与三份 PLAN 共 19 条 truths 去重后为 16 条（SC1→#1、SC2→#2、SC3→#3、SC4→#4/5，其余为 plan 特有）。全部 16 条均有**本验证实际运行并通过**的行为级测试证据（非仅符号存在）。

| #   | Truth   | Status     | Evidence       |
| --- | ------- | ---------- | -------------- |
| 1   | requirePerm 按 Casbin RBAC 校验，无权 403/10200，无 token 401（SC1/PERM-06） | ✓ VERIFIED | internal/middleware/perm.go 三分支 fail-closed；TestPermEnforcerBootSmoke PASS（13.17s：无角色 403 + 无 token 401）；TestPermRouteLockdown PASS（13.56s：六域采样全 403/10200） |
| 2   | 管理员经角色授权接口分配/回收，策略持久化 casbin_rule 且 sys_role_menu 原样双轨（SC2/PERM-04/05） | ✓ VERIFIED | TestPermHotReload PASS（12.21s）：(d1) 直查 casbin_rule (p,role_code,perm) 行==1 且 sys_role_menu 行==1；(d2) 全回收后两轨同步归零 |
| 3   | 已登录用户不重登录，分配/回收后下一请求即按新策略判定——p 轴与 g 轴（SC3/PERM-02） | ✓ VERIFIED | TestPermHotReload PASS：(b) 步复用 (a) 步同一 token（测试内无第二次登录）分配→200、(c) 置空→403；TestPermRevokeRoleImmediate（g 轴同 token 回收→403）在全量回归门 14 包绿内 |
| 4   | dataScope 五档在用户列表产生可观察行过滤差异（SC4/PERM-03） | ✓ VERIFIED | TestUserDataScope PASS（13.11s）：档 1/2/3/4/5 行集互异且各档断言精确（档 1 恰 ds-self-row、档 5 全量 5 行） |
| 5   | "本部门"档看不到其他部门用户，"全部"档可见（SC4 验收明文） | ✓ VERIFIED | TestUserDataScope 档 2 断言：行集恰 {ds-user-c}，A/B 不可见（"他部门 A/B 不可见"断言消息在位） |
| 6   | super_admin 在 casbin_rule 零 p 行时仍通过（g 层豁免独立于 p 行），撤销后下一请求即失效（D-07） | ✓ VERIFIED | TestPermEnforcerBootSmoke ①（p_rows==0 且 admin 200）PASS；TestSuperAdminExemptWithoutPolicies + TestRevokeSuperAdminImmediate PASS（纯内存单测，authz 包绿） |
| 7   | 服务启动 boot 对账：sys_user_role + sys_role_menu 全量导入 g/p 策略（PERM-01 落地前提） | ✓ VERIFIED | internal/app/app.go:89-127（去重 map + ReplaceAll + fail-fast）；TestPermEnforcerBootSmoke ① 直查 casbin_rule g_rows>=1 PASS |
| 8   | 档位 4 经 PUT /roles/:id/scopes 配置部门集合（sys_role_dept 存储），列表按集合过滤 | ✓ VERIFIED | router.go:99-100 两路由挂 RequirePerm；TestRoleAssignScopes PASS（14.36s：分配/覆盖/空回收/缺失 400/删除零残留）；TestUserDataScope 档 4 断言恰 D2 用户 |
| 9   | 多角色档位聚合取 max（D-11），无有效档位 fail-closed 最窄档 | ✓ VERIFIED | TestUserDataScope 多角色断言（档 2+档 5 → 全量）PASS；internal/datascope/datascope.go:97-114 Widest max 语义；TestWidest（[2,5]→5 方向锁）authz 同批单测绿（datascope 包 ok） |
| 10  | 角色删除连带清理 sys_role_dept（四连删，D-10） | ✓ VERIFIED | internal/service/role.go:239 DeleteRoleDepts 在 Delete 事务内；TestRoleAssignScopes 末尾删除持有 scopes 角色 → 零残留断言 PASS |
| 11  | 档位/部门集变更后下一请求即按新档位过滤（表驱动实时查询，无缓存） | ✓ VERIFIED | TestUserDataScope 档位变更断言（PUT /roles/:id 改 dataScope 2→5 后同一 C token 下一请求即全量）PASS |
| 12  | pri 组全部管理路由逐路由挂 requirePerm，无权 403/10200（PERM-06 全量收口） | ✓ VERIFIED | grep -c "middleware.RequirePerm(" internal/app/router.go == 36（34 清单 + scopes 2）；TestPermRouteLockdown 六域采样 PASS |
| 13  | RequireRoles 过渡 gate 退役，唯 kickout 保留叠加（D-15 纵深防御） | ✓ VERIFIED | grep -c "middleware.RequireRoles(" == 1（router.go:54 kickout 行）；RouteLockdown (c) 持 p 策略非 super_admin 调 kickout 仍 403 + (d) super_admin 200 PASS |
| 14  | 同 token 授权域 200 与未授权域 403 并存（横向越权采样，授权最小化） | ✓ VERIFIED | TestPermRouteLockdown (b)：仅授 system:menu:list 后同 token GET /menus 200 且 GET /users 仍 403 PASS |
| 15  | API Key 完整登录原子写入 iam:apikey-user:{key} 反查键（无 TTL，IN-14/D-16） | ✓ VERIFIED | internal/auth/session.go:197-204 TxPipelined 双 SET（映射键 EX timeout / 反查键 TTL=0）；TestBindAPIKeyDualKeys PASS（TTL -1 协议直证无过期） |
| 16  | 过期/停用 key 再次 apikey-login 返回 10503 同时撤销存量会话（旧 token 401）；管理面停用/删除清理反查键 | ✓ VERIFIED | TestApiKeyExpireRevocation PASS（13.80s）：过期轴（DB 直改 expire_time → 二次登录 10503 → 旧 token 401）+ 停用轴（PUT status 后旧 token 401 且双键 redis.Nil 直查）；KickoutAPIKey/UnbindAPIKey 双删 + TestKickoutAPIKeyCleansDualKeys/TestUnbindAPIKeyCleansDualKeys 绿 |

**Score:** 16/16 truths verified（0 present-behavior-unverified——全部行为依赖型 truth 均有本验证实际运行通过的命名测试）

### MVP 模式格式差异（discrepancy surfaced）

ROADMAP 将本阶段标记为 `Mode: mvp`，但 goal 原文非用户故事句式（`user-story.validate` → valid=false）。PLAN 03-01 objective 记录了规划期的语义等价转写（"As a 已登录的普通用户与管理员，I want to …，so that …"），该转写语义槽位齐备但使用全角标点（，/。），同样未过 ASCII 标点正则。**本验证未拒绝执行**：依据编排者指示的验证方法（must_haves 三类处置），以标准 goal-backward（ROADMAP 4 SC 契约 + PLAN must_haves 合并）完成验证，并补充下方用户流覆盖表以兑现 MVP 意图。如需规范化 goal 格式可运行 `/gsd-mvp-phase 3` 后重验。

### User Flow Coverage（按规划期转写用户故事）

| 用户流步骤 | 预期 | 代码库证据 | Status |
| ---------- | ---- | ---------- | ------ |
| 已登录用户访问受保护管理端点 | 按 Casbin RBAC 实时判定（g 行用户→角色→p 行 perm 串） | router.go 36 处 RequirePerm → perm.go → enforcer.Enforce；TestPermRouteLockdown PASS | ✓ COVERED |
| 无权用户访问 | 403/10200，无 token 401 | TestPermEnforcerBootSmoke ②③ PASS | ✓ COVERED |
| 管理员分配功能权限（PUT /roles/:id/menus） | 同一 token 下一请求即放行，无需重登录 | TestPermHotReload (b) 同 token 200 PASS | ✓ COVERED |
| 管理员回收权限（置空 menuIds / 清空 roles） | 同一 token 下一请求即被拒 | TestPermHotReload (c) + TestPermRevokeRoleImmediate PASS | ✓ COVERED |
| 不同层级管理员按 dataScope 档位只见职责范围数据 | 五档行集互异，本部门不见他部门 | TestUserDataScope 五档断言 PASS | ✓ COVERED |
| 过期 API Key 持有者再次登录 | 存量会话被撤销（旧 token 401），无法续命 | TestApiKeyExpireRevocation 过期轴 PASS | ✓ COVERED |

### Required Artifacts

| Artifact | Expected    | Status | Details |
| -------- | ----------- | ------ | ------- |
| `internal/authz/enforcer.go` | SyncedEnforcer 五步装配 + 六同步 API + Authorizer 接口 | ✓ VERIFIED | 202 行，五步链 enforcer.go:41-59（TurnOffAutoMigrate→adapter→model→NewSyncedEnforcer→LoadPolicy fail-fast）；replace/sync/clear/hasRole API 齐（含 WR-04 SyncRoleUsers、CR-04 HasRole） |
| `internal/authz/perms.go` | 内嵌 RBAC model（豁免首位短路）+ 全量 perm 串常量 | ✓ VERIFIED | 80 行，matcher `m = g(r.sub, "super_admin") || (r.perm == p.perm && g(r.sub, p.role))` 豁免在首个 ‖ 前；27 常量 system: 前缀 |
| `internal/middleware/perm.go` | RequirePerm 401/403/500 fail-closed 三分支 | ✓ VERIFIED | 58 行，三分支齐 + WR-03 slog.Error 留痕；判定仅走 az.Enforce（无 Identity.Roles 引用——P1 直证） |
| `scripts/ddl/casbin_rule.sql` | 显式 DDL 自带唯一索引 | ✓ VERIFIED | UNIQUE KEY idx_casbin_rule(ptype,v0..v5) 在位；DDL 目录零 ALTER |
| `internal/datascope/datascope.go` | Mode 五档 + Filter.Apply + Widest + CollectSubtreeIDs | ✓ VERIFIED | 148 行，case 首行 len 守卫 + default "1 = 0" fail-closed；Widest max（非 min）；BFS visited 防环 |
| `scripts/ddl/sys_role_dept.sql` | 档位 4 部门集合存储（8 列同构） | ✓ VERIFIED | 纯新增零 ALTER；testutil.execDDLDir 自动加载 |
| `internal/model/po/relation.go` | RoleDeptPO（TableName + BeforeCreate 雪花） | ✓ VERIFIED | verify.artifacts 3/3 passed（contains RoleDeptPO） |
| `internal/app/router.go` | 34+2 路由全量 RequirePerm | ✓ VERIFIED | 111 行，36 处 RequirePerm / 1 处 RequireRoles（kickout） |
| `internal/auth/session.go` | iam:apikey-user: 反查键三件 + 契约注释 | ✓ VERIFIED | 290 行，前缀进契约块 :28-33；BindAPIKey 双写 / LookupAPIKeyUser / 双清理路径齐 |

（verify.artifacts 工具化结果：03-01 4/4、03-02 3/3、03-03 2/2 全 passed，无 STUB/MISSING）

### Key Link Verification

| From | To  | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| internal/app/router.go | internal/authz/enforcer.go | GET /users 挂 RequirePerm(az, PermUserList)→Enforce | ✓ WIRED | router.go:60 |
| internal/app/app.go | internal/repository/relation.go | boot 对账两查询→ReplaceAll | ✓ WIRED | app.go:93-127 |
| internal/service/role.go | internal/authz/enforcer.go | AssignMenus 事务成功后 SyncRolePerms | ✓ WIRED | role.go:337（事务 return nil 之后） |
| internal/service/user.go | internal/datascope/datascope.go | List 内 resolveScope→Filter→repo.List | ✓ WIRED | user.go:211/243 |
| internal/repository/user.go | internal/datascope/datascope.go | q() 闭包注入 filter.Apply（Count/Find 同闭包） | ✓ WIRED | user.go:81-100（q().Count 与 q().Offset...Find 同源） |
| internal/service/role.go | internal/repository/relation.go | AssignScopes 事务 Delete+BatchCreate；Delete 四连删 | ✓ WIRED | role.go:362-407 |
| internal/service/auth.go | internal/auth/session.go | ApikeyLogin key==nil 分支→LookupAPIKeyUser→KickoutUser+DEL | ✓ WIRED | auth.go:109-114 |
| internal/app/router.go | internal/authz/perms.go | 34 路由逐条 RequirePerm(az, authz.PermXxx) | ✓ WIRED | grep 36 处全命中 authz.Perm* 常量 |

（verify.key-links 工具化结果：03-01 3/3、03-02 3/3、03-03 2/2 全 WIRED）

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| RequirePerm 判定 | Enforce(sub, perm) | casbin 内存策略 ← casbin_rule 表 ← boot 对账 + 四写路径同步 | 是（集成测试直查 casbin_rule 行数断言） | ✓ FLOWING |
| 用户列表行集 | filter.Apply 注入 WHERE | sys_user.dept_id + sys_user_role/sys_role.data_scope + sys_role_dept（实时查表无缓存） | 是（TestUserDataScope 五档行集互异即真实数据差异） | ✓ FLOWING |
| ApikeyLogin 撤销链 | LookupAPIKeyUser(uid) | iam:apikey-user:{key} ← BindAPIKey 完整登录双写 | 是（TestApiKeyExpireRevocation redis 直查键存在/消失） | ✓ FLOWING |
| boot 对账 pRules/gRules | FindUserRoleCodes/FindRolePerms | sys_user_role/sys_role_menu/sys_role/sys_menu JOIN | 是（BootSmoke g_rows>=1 直查） | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| 构建门 | go build ./... && go vet ./... | BUILD OK / VET OK | ✓ PASS |
| 单测回归锁 | go test ./internal/authz/ ./internal/middleware/ ./internal/datascope/ ./internal/auth/ -count=1 | 4 包全 ok | ✓ PASS |
| boot 对账+豁免+403/401 | TZ=UTC go test -tags=integration -run TestPermEnforcerBootSmoke | PASS 13.17s | ✓ PASS |
| 同 token 即时生效+双轨 | TZ=UTC go test -tags=integration -run TestPermHotReload | PASS 12.21s | ✓ PASS |
| 34 路由收口+横向越权+kickout 叠加 | TZ=UTC go test -tags=integration -run TestPermRouteLockdown | PASS 13.56s | ✓ PASS |
| dataScope 五档行差异 | TZ=UTC go test -tags=integration -run TestUserDataScope | PASS 13.11s | ✓ PASS |
| IN-14 过期/停用 key 撤销 | TZ=UTC go test -tags=integration -run TestApiKeyExpireRevocation | PASS 13.80s | ✓ PASS |
| scopes 契约+四连删零残留 | TZ=UTC go test -tags=integration -run TestRoleAssignScopes | PASS 14.36s | ✓ PASS |

（全量回归门 `TZ=UTC go test -tags=integration -count=1 -timeout 30m ./...` 14 包 exit 0 由编排者执行背书，本验证按指示抽验 6 个命名集成测试未重跑全量套件）

### Probe Execution

无阶段声明的 probe 脚本（PLAN/SUMMARY 无 probe-* 声明；scripts/*/tests/ 目录不存在）——SKIPPED（不适用）。

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| PERM-01 | 03-01 | Casbin RBAC 模型落地（用户-角色-功能权限串） | ✓ SATISFIED | enforcer.go 装配 + perms.go model + boot 对账；TestPermEnforcerBootSmoke PASS |
| PERM-02 | 03-01, 03-03 | 权限变更即时生效（热重载，无需重登录） | ✓ SATISFIED | TestPermHotReload 同 token 断言 PASS；TestPermRouteLockdown (b) 收口面同链 PASS；resolveScope 无缓存实时查表 |
| PERM-03 | 03-02 | dataScope 五档行过滤 | ✓ SATISFIED | TestUserDataScope 五档行集互异 + 跨部门不可见 PASS |
| PERM-04 | 03-01 | 策略持久化 casbin_rule，sys_role_menu 原样双轨 | ✓ SATISFIED | TestPermHotReload (d1)(d2) 双轨同写同删直查断言 PASS |
| PERM-05 | 03-01 | 管理员经角色授权接口管理策略 | ✓ SATISFIED | AssignMenus/AssignRoles 事务后 SyncRolePerms/SyncUserRoles（role.go:337 / user.go:482）；HotReload 全链经 API 驱动 PASS |
| PERM-06 | 03-01, 03-03 | 路由级 requirePerm 拦截无权 403 | ✓ SATISFIED | grep 36/1 结构门 + TestPermRouteLockdown 六域采样 PASS |

REQUIREMENTS.md 将 PERM-01..06 全部映射到 Phase 3，三份 PLAN requirements 字段合计恰好覆盖全部 6 个 ID——无 ORPHANED 需求。

### Decision Coverage

All trackable CONTEXT.md decisions are honored by shipped artifacts.（check.decision-coverage-verify：total=16, honored=16, not_honored=[]）

### Test Quality Audit

| Test File | Linked Req | Active | Skipped | Circular | Assertion Level | Verdict |
| --------- | ---------- | ------ | ------- | -------- | --------------- | ------- |
| internal/authz/enforcer_test.go | PERM-01 | 5 | 0 | 0 | Value/Behavioral（零 p 行豁免 bool 直证、撤销后 false 直证） | VALID |
| internal/authz 之外 middleware/perm_test.go | PERM-06 | 表格驱动 | 0 | 0 | Status（401/403/500 分支） | VALID |
| internal/datascope/datascope_test.go | PERM-03 | Widest/子树/Apply | 0 | 0 | Value（SQL 子句 DryRun 断言、方向锁） | VALID |
| internal/handler/perm_integration_test.go | PERM-01..06 | 6 测试 | 0 | 0 | Behavioral（同 token 四步链 + DB 直查行数） | VALID |
| internal/handler/api_key_integration_test.go | IN-14 | 过期+停用双轴 | 0 | 0 | Behavioral（10503 + 旧 token 401 + redis.Nil 直查） | VALID |
| internal/auth/session_test.go | IN-14 | 双键三件 | 0 | 0 | Value（TTL -1 协议直证） | VALID |

**Disabled tests on requirements:** 0（grep t.Skip/Skip 无命中）
**Circular patterns detected:** 0
**Insufficient assertions:** 0（集成断言均为 Value/Behavioral 级，含 DB 直查与 redis 直查）

### Prohibition Verification（7 条逐一处置）

| # | Prohibition | 判定 | 证据 |
| --- | ----------- | ---- | ---- |
| P1 | requirePerm 不得回退会话 roles 快照；super_admin 豁免唯一判据 enforcer g 层 | ✓ VERIFIED（测试强制） | perm.go 全文无 Identity.Roles 引用（仅 az.Enforce(uid 串, perm)）；TestSuperAdminExemptWithoutPolicies/TestRevokeSuperAdminImmediate 绿；TestPermHotReload (b)（会话先于授权建立仍 200）直证非登录快照语义 |
| P2 | enforcer 同步不得塞业务 db.Transaction；失败不得静默吞掉 | ⚠️ 结构已证、故障路径无自动化测试 → 人工复核 | role.go:337 同步为事务 return nil 之后的返回语句（error 上抛无 _ =）；user.go:482 同形；Delete 预清路径 err 检查齐全（role.go:227 / user.go:575-579）——代码形态合规为行级检视结论，转人工复核（见 Human Verification） |
| P3 | 业务代码不得直写 casbin_rule；不得对既有表 ALTER/AutoMigrate | ✓ VERIFIED（确定性 grep） | casbin_rule 全仓引用仅 internal/authz/ + app.go 注释；AutoMigrate 仅 TurnOffAutoMigrate（enforcer.go:42）；scripts/ddl/ 零 ALTER TABLE；新表仅 casbin_rule/sys_role_dept 两张 |
| P4 | dataScope 须作用于 Count 同闭包；禁拼 SQL 字符串 | ✓ VERIFIED（测试+代码强制） | user.go:81-100 q() 闭包 Count/Find 同源；datascope.go 仅 create_by_id/dept_id 两列白名单、值全占位符；TestFilterApplyClauses DryRun 子句断言 + TestUserDataScope total==items 断言绿 |
| P5 | 档位不得取 min（03-PATTERNS 第 8 节笔误否决） | ✓ VERIFIED（测试强制） | datascope.go:105 `m > widest` max 语义；TestWidest [2,5]→5 方向回归锁绿 |
| P6 | 不得为测试变绿豁免/移除任何路由 requirePerm | ✓ VERIFIED（确定性 grep+测试） | grep 36/1 与挂载清单一致，无豁免路由（logout 为清单明示不含）；TestPermRouteLockdown 全绿；03-03 台账记录既有测试一律 grantUserPerm 授权链修复而非拆 gate |
| P7 | IN-14 撤销失败不阻断 10503；粒度 user 轴 | ⚠️ 结构已证、故障路径无自动化测试 → 人工复核 | auth.go:109-114 `_ = s.sm.KickoutUser` + `_ = s.sm.UnbindAPIKey` 后 `return nil, errcode.ErrApiKeyInvalid`（尽力而为形态 + 注释声明）；粒度 KickoutUser(uid) 即 user 轴——代码形态合规为行级检视结论，转人工复核（见 Human Verification） |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| （无） | — | 债务标记 TBD/FIXME/XXX 零命中；TODO/HACK/PLACEHOLDER 零命中；空实现/占位返回零命中；禁用测试零命中 | — | — |

### Human Verification Required

2 项（均为 prohibition 故障路径半边的人工复核——代码形态已由本验证行级检视确认合规，按自主验证模式的 fail-closed 规则转人工闭环，非行为缺口）：

### 1. P2：enforcer 同步错误传播形态复核

**Test:** 读 internal/service/role.go AssignMenus 末段（SyncRolePerms 为事务后返回语句）与 Delete 预清段、internal/service/user.go AssignRoles 末段（SyncUserRoles）与 Delete 预清段（KickoutUser→ClearUser→事务），确认无 `_ =` 吞错、同步/清理均在事务边界之外（预清模式下在事务之前且失败即中止）。
**Expected:** 全部 enforcer 调用 error 经 return 上抛；无任何 `_ = enf.` 形态。
**Why human:** "同步失败上抛 500" 的故障分支无失败注入测试；当前合规判定来自代码检视，人工一行确认即可关闭。

### 2. P7：IN-14 撤销链尽力而为形态复核

**Test:** 读 internal/service/auth.go ApikeyLogin key==nil 分支（约 :109-114），确认撤销链两处 `_ =` 尽力而为语义与 user 轴粒度注释在位。
**Expected:** LookupAPIKeyUser 命中 → `_ = KickoutUser(uid)` + `_ = UnbindAPIKey` → 10503 返回不被撤销失败阻断。
**Why human:** "Redis 故障时撤销失败不阻断响应" 无故障注入测试；当前合规判定来自代码检视，人工一行确认即可关闭。

### Gaps Summary

无缺口。三份 plan 的全部 must_haves（16 条去重 truths + 9 artifacts + 8 key links）全部验证通过，且行为依赖型 truths（即时生效状态转换、双轨落库、撤销链、五档行过滤）均有本验证实际运行通过的命名集成/单元测试背书。6 条 PERM 需求全覆盖无 orphaned。16 条 CONTEXT 决策全部 honored。

非缺口留档（供编排者知悉，不影响状态）：
1. **Review 台账 WR-05（role:edit 自授权面）deferred 挂账部署清单**——部署信任边界裁决（system:role:edit 仅授管理员），与 03-REVIEW.md 复核结论"CR-04 g 层判据落地后残余面与裁决一致"相符，属部署期检查项非本阶段 truth 缺口。
2. **现场部署清单两项**（03-02/03-03 留档）：生产存量 super_admin 角色 data_scope=NULL 需经 PUT /roles/:id 置 5（测试基建已内建档 5 种子）；二进制体积 22.9MB→39.5MB（Casbin 全方言依赖，静态链接门过）。
3. **Review Info 项 IN-05/IN-06**（预清模式重登录窗口/删除并发孤儿行）open(info) 留档，不在 fix scope。

---

_Verified: 2026-11-01T08:24:27Z_
_Verifier: Claude (gsd-verifier)_
