---
phase: "03"
phase_name: "Casbin 授权与数据权限"
status: "complete"
confidence: "HIGH"
researched: "2026-11-01"
method: "proxy.golang.org 实测 + 本地 go get/go run probe 实测（MySQL 8.4.8 容器）+ 本机容器 DDL 直证 + 源仓库 grep + Context7/官方源码签名"
---

# Phase 3: Casbin 授权与数据权限 - Research

## Summary

本阶段研究以**实测为主**：在 `/tmp/casbin-probe-v2` 建立临时 Go module，对 casbin/v2 v2.135.0 + gorm-adapter v3.35.0 组合做了**端到端可运行验证**（连本机 mysql-server 容器真实建表/读写/回收/重启持久化，全部断言 PASS），而非仅依赖文档。结论：CONTEXT.md D-01 拍板的 casbin/v2 路线**可行且已全链路实证**，但有三个文档查不到、只有实测才能发现的坑（matcher token 名一致性、super_admin 豁免短路顺序、MySQL 8.4 批内重复行 Error 1869），已全部落进实现指引。

两个必须纠正的既有文档表述：其一，CONTEXT.md D-13 与研究任务书中写的 `RemoveFilteredPolicies`（复数）**在 casbin v2 v2.135.0 中不存在**，实际 API 是 `RemoveFilteredPolicy`（单数，management_api.go:304），按文档写会编译失败；其二，D-06 示例 perm 串 `sys:user:list` 与真实数据不符——本机 sys_menu 41 行菜单 39 行有 permission 串，前缀**全部为 `system:`**（如 `system:user:list`），perm 串常量应按真实数据风格 `system:{domain}:{action}`。

一个影响需求的 schema 缺口：**sys_role 真实 DDL（本机容器 SHOW CREATE TABLE 直证）13 列中只有 `data_scope tinyint`，不存在任何"指定部门集合"列**；源项目全仓库 grep（`dataScopeDept|scope_dept|dept_ids`）零命中，源 Role.java 仅 `Integer dataScope` 单字段，RoleController 请求体亦无部门集合入参——即五档中**档位 4（指定部门）的数据来源在源系统与本库均不存在**，需要增量建表/加列（三方案与推荐见 Open Questions Q1）。此外 sys_menu 现有数据没有 apiKey/auth 域的 permission 串，requirePerm 全量收口时这两域 perm 串需要"发明"（只写 casbin p 行，菜单无对应项，见 Open Questions Q2）。

**Primary recommendation:** 锁定 casbin/v2 v2.135.0 + gorm-adapter v3.35.0（最后支持 v2 的 adapter），g/p 层 sub 统一用 role_code、豁免走 matcher 首位短路 `g(r.sub, "super_admin")`，四条写路径（AssignMenus/AssignRoles/角色删除/用户删除）在业务事务提交后同步 enforcer，DDL 显式建 casbin_rule（自带唯一索引，TurnOffAutoMigrate），档位 4 存储推荐新增 sys_role_dept 关联表（Q1 待裁决）。

## Key Findings

### 研究重点 1：Casbin 版本与依赖闭包（HIGH，实测）

**版本矩阵（proxy.golang.org `@latest` 与 `@v/*.mod` 直查，2026-11-01）** [VERIFIED: proxy.golang.org/@latest]：

| 模块 | 最新版 | 发布时间 | 说明 |
|------|--------|----------|------|
| github.com/casbin/casbin/v2 | v2.135.0 | 2025-12-09 | v2 维护线最新 |
| github.com/casbin/gorm-adapter/v3 | v3.41.0 | 2026-01-20 | **依赖 casbin/v3 v3.8.1** |
| github.com/casbin/gorm-adapter/v3 | v3.35.0 | （v3.36.0 前） | **最后依赖 casbin/v2 的版本**（声明 casbin/v2 v2.100.0） |
| github.com/casbin/casbin/v3 | v3.10.0 | 2026-01-25 | 官方主线（v3.36.0+ adapter 配套） |

**关键结论：gorm-adapter v3.36.0 起切换到 casbin/v3 import，与 casbin/v2 不兼容** [VERIFIED: proxy.golang.org/@v/v3.40.0.mod 含 casbin/v3 v3.8.1、v3.35.0.mod 含 casbin/v2 v2.100.0]。两条可行组合：

- **组合 A（推荐，遵 D-01）**：casbin/v2 v2.135.0 + gorm-adapter v3.35.0。本地 `go get` 实测 MVS 解析成功，casbin/v2 被提升到 2.135.0，go-sql-driver/mysql 提升到 1.8.1（与项目 go.mod 现状一致），probe 编译运行全过 [VERIFIED: /tmp/casbin-probe-v2 实测]。
- 组合 B（备选）：casbin/v3 v3.10.0 + gorm-adapter v3.41.0，同样可解析——但 D-01 已拍板 casbin/v2，不推荐；如需跟官方主线须推翻 D-01（用户决策）。

**依赖闭包与 CGO** [VERIFIED: 两组合本地 go.mod 实测]：
- casbin 本体极轻：仅 doublestar（glob）、casbin/govaluate（表达式）、google/uuid。
- gorm-adapter 拖入**全方言**驱动（glebarez/sqlite + driver/postgres + driver/sqlserver）——sqlite 走 glebarez/modernc **纯 Go 实现，无 mattn/go-sqlite3，无 CGO**，`CGO_ENABLED=0` 纯静态单二进制约束不受影响（Phase 1 已证 CGO 铁律）。二进制会有数 MB 增量（全方言编译进二进制，adapter 无 build tag 分方言）[ASSUMED: 具体增量未测，Phase 1 产物 amd64 22.9MB 基数上预计 +3~6MB，验收时实测]。
- gorm-adapter v3.35.0 声明 `go 1.20`，项目 Go 1.27.3 满足 [VERIFIED: @v/v3.35.0.mod]。

**包合法性审计（Package Legitimacy）**：gsd-tools 的 legitimacy check 仅支持 npm/pypi/crates 生态，Go 模块不在覆盖面。按协议精神以**等价权威验证**替代：proxy.golang.org 的 Origin.URL 确认两模块均来自 github.com/casbin 官方组织仓库（casbin/gorm-adapter，Hash 与 tag 对应）[VERIFIED: proxy.golang.org Origin 字段]。无第三方 fork、无 slopsquat 面。

### 研究重点 2：enforcer 集成细节（HIGH，probe 全过实测）

**Boot 链（四步，全部实测通过）** [VERIFIED: /tmp/casbin-probe-v2/main.go 实测 "boot chain ok"]：

```go
import (
    "github.com/casbin/casbin/v2"
    "github.com/casbin/casbin/v2/model"
    gormadapter "github.com/casbin/gorm-adapter/v3"
)

gormadapter.TurnOffAutoMigrate(db)                                       // ① 必须在 NewAdapterByDB* 之前
a, err := gormadapter.NewAdapterByDBUseTableName(db, "", "casbin_rule")  // ② 共享既有 *gorm.DB
m, err := model.NewModelFromString(rbacModel)                            // ③ D-03 内嵌字符串 model 的正解入口
e, err := casbin.NewSyncedEnforcer(m, a)                                 // ④ 并发安全（gin 多 goroutine 必用 Synced 版）
_ = e.LoadPolicy()                                                       // ⑤ 启动全量加载
```

签名与行为实证（源码行号 + probe）：

- `NewAdapterByDBUseTableName(db *gorm.DB, prefix string, tableName string) (*Adapter, error)` — gorm-adapter v3.35.0 adapter.go:198 [VERIFIED: 源码]。
- `TurnOffAutoMigrate(db *gorm.DB)` 返回 void，通过 ctx value（disableMigrateKey）标记；createTable 里 `disableMigrate != nil → return nil`——**连同唯一索引创建一起整体跳过**（adapter.go:410-417）[VERIFIED: 源码]。因此**显式 DDL 必须自带 `UNIQUE KEY (ptype,v0..v5)`**（adapter 关闭迁移后不建任何索引）。
- `model.NewModelFromString(text string) (Model, error)` — casbin/v2 model/model.go:168 [VERIFIED: 源码]。`casbin.NewEnforcer(modelPath, adapter)` 的 string 参数是**文件路径不是内容**（enforcer.go NewEnforcer case 2 走 InitWithFile）——内嵌字符串只能走 Model 构造路径。
- **必须用 `NewSyncedEnforcer`**（enforcer_synced.go:37，签名与 NewEnforcer 同构）：普通 Enforcer 的 Enforce/写 API 无跨 goroutine 锁，gin 每请求一个 goroutine 并发 Enforce + AssignMenus 并发写策略，不加锁有数据竞争；SyncedEnforcer 的 Enforce 用 RLock、写 API 用 Lock [VERIFIED: enforcer_synced.go:146/191/370/465/518 签名齐备]。
- `e.Enforce(rvals ...interface{}) (bool, error)` — Canonical `(sub, obj)` 形态；进程内 matcher 求值，每请求零 SQL（策略全内存，LoadPolicy 一次）[VERIFIED: Context7 /apache/casbin enforcer.md + probe]。

**实测 model 字符串（D-05 纯 RBAC + D-07 super_admin 豁免，probe 全过）**：

```ini
[request_definition]
r = sub, perm

[policy_definition]
p = role, perm

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, "super_admin") || (r.perm == p.perm && g(r.sub, p.role))
```

两个 probe 抓出的 matcher 坑（文档不会告诉你）：

1. **自定义 token 名必须与定义节一致**：`p = role, perm` 时 matcher 必须写 `p.role`，写 `p.sub` 直接报错 `No parameter 'p_sub' found.` [VERIFIED: probe 第一次运行实败后修正]。
2. **super_admin 豁免必须是 matcher 的第一个 `||` 短路分支**：casbin 对每条 p 策略行求值 matcher——若豁免写在 `r.perm == p.perm && (...)` 之后，请求不匹配任何 p 行时 matcher 从未被求值，豁免永不生效（probe 实测 `[FAIL] Enforce(9002 super_admin) got false`，调整顺序后 PASS）[VERIFIED: probe 两次运行对比]。
3. g 层第二参数支持**字符串字面量** `g(r.sub, "super_admin")`——super_admin 豁免判据完全走 enforcer g 层（role_code 常量），撤销 super_admin 角色（g 行删除）后豁免**下一请求立即失效**（probe：RemoveFilteredGroupingPolicy 后 Enforce false）[VERIFIED: probe]。

**sub 采用 role_code 的设计（自洽解）**：g 行 `g, {userID十进制串}, {role_code}`、p 行 `p, {role_code}, {perm串}`。理由：① Identity.Roles 快照存的就是 role_code（session.go:162-171 completeLogin 装配），会话与策略同轴；② role_code 不可改（RoleService.Update 白名单 role_name/data_scope/status/builtin/remark，service/role.go:75-90 实证无 role_code）→ 策略行不会因改名失效 [VERIFIED: service/role.go:76-81]；③ builtin super_admin 角色不可删（守卫已有）→ code 恒定；④ matcher 字面量豁免免查 ID 映射。

**casbin_rule 表结构**（CasbinRule struct，gorm-adapter v3.35.0 adapter.go:48-61）[VERIFIED: 源码 + probe SHOW INDEX]：
- 列：`id`（uint，primaryKey，autoIncrement）、`ptype` varchar(100)、`v0`..`v5` varchar(100)
- 唯一索引：`(ptype, v0, v1, v2, v3, v4, v5)`（probe SHOW INDEX 输出 PRIMARY + idx 复合索引 8 行确认存在）

**403 语义**：直接复用 `errcode.ErrForbidden`（CodeForbidden=10200 → HTTPStatus 403，pkg/errcode/errcode.go:8 与 :65 实证），与 RequireRoles 现行 403 完全同码 [VERIFIED: pkg/errcode/errcode.go:8,58-67]。

### 研究重点 3：策略同步写路径（HIGH，源码 + probe 实证）

**API 实名纠正** [VERIFIED: casbin v2.135.0 management_api.go / rbac_api.go 源码]：

| 用途 | 实际 API（v2.135.0） | 文档常见误写 |
|------|----------------------|--------------|
| 批量加 p 策略 | `AddPolicies(rules [][]string) (bool, error)`（management_api.go:226） | — |
| 按过滤删 p 策略 | **`RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error)`**（management_api.go:304） | ~~RemoveFilteredPolicies~~（复数版不存在，CONTEXT.md D-13 原文即此误写） |
| 批量加 g 策略 | `AddGroupingPolicies(rules [][]string) (bool, error)`（management_api.go:360） | — |
| 按过滤删 g 策略 | `RemoveFilteredGroupingPolicy(fieldIndex int, fieldValues ...string) (bool, error)`（management_api.go:416） | — |
| 删指定角色全部 p 行 | `RemoveFilteredPolicy(0, roleCode)` | — |
| 删指定用户全部 g 行 | `RemoveFilteredGroupingPolicy(0, userID)` | — |
| 删指定角色全部 g 行 | `RemoveFilteredGroupingPolicy(1, roleCode)`（fieldIndex=1 是 role 位） | — |

**事务边界实证（研究任务书点名的坑，确认存在）** [VERIFIED: adapter.go 源码]：
- adapter 的 `AddPolicyCtx` 用 `a.db...OnConflict{DoNothing: true}.Create(&line)`（adapter.go:668-673），批量 `AddPolicies` 同款（adapter.go:686-691）；`a.db` 是 NewAdapterByDBUseTableName 时经 `db.Scopes(...).Session(...)` 派生的**独立 Session，不是业务 tx 对象**（app.go 传入的共享连接池）。结论：**casbin 策略写不在业务 `db.Transaction` 内，两个独立提交**。gorm-adapter 另有 TransactionalAdapter（两阶段提交）可并入事务，但对单机单实例属过度设计（对齐 D-02 watcher 判例），不推荐。
- **推荐顺序（自愈型）**：业务事务先行提交 → 事务成功后同步 enforcer（Remove* + Add*）→ 同步失败上抛 500 管理员重试。重试自愈的根据：`RemoveFilteredPolicy(0, code)` 全删 + `AddPolicies` 全加是幂等全量替换，重试即收敛（probe 跨批重复写入实测：已存在的行返回 false、表内不重复——OnConflict DoNothing 生效）[VERIFIED: probe "AddPolicies(same row again) cross-batch: false" + "admin p-rows after re-add: 2"]。
- **MySQL 8.4 批内重复行 Error 1869**：同一批 `AddPolicies` 含两条完全相同的行时，`ON DUPLICATE KEY UPDATE id=id` 在 MySQL 8.4.8 下报 `Error 1869 (HY000): Auto-increment value in UPDATE conflicts with internally generated values`，整批失败 [VERIFIED: probe 第一次运行实败]。gorm-adapter v3.41.0 写法相同（adapter.go:651/664/673/691/714 均为 OnConflict DoNothing），升级不可解。**业务层必须去重**：assignMenus 同步前对 permission 串 `map[string]struct{}` 去重（sys_menu 允许两菜单同 permission 串，41 行数据已见重复风险），AssignRoles 同理对 roleIDs 去重。

**四条写路径的同步挂点（全部落 service 层、事务成功后）** [VERIFIED: 各 service 源码挂点]：

| 写路径 | 现有事务挂点 | casbin 同步（事务成功后） |
|--------|--------------|---------------------------|
| AssignMenus（PUT /roles/:id/menus） | RoleService.AssignMenus 的 db.Transaction{DeleteRoleMenus + BatchCreateRoleMenus}（service/role.go:194-209） | `RemoveFilteredPolicy(0, roleCode)` + `AddPolicies(code→perm 串集)`；perm 串来自 menuRepo.FindByIDs 查 permission 列非空者，**先去重**；空 menuIds=全回收（只删不加） |
| AssignRoles（PUT /users/:id/roles） | UserService.AssignRoles 的 db.Transaction{DeleteUserRolesByUserID + BatchCreateUserRoles}（service/user.go:278-293） | `RemoveFilteredGroupingPolicy(0, uid)` + `AddGroupingPolicies(uid×roleCodes)`；roleCodes 来自既有 FindByIDs 校验链，**roleIDs 先去重** |
| 角色删除（三连删） | RoleService.Delete 的 db.Transaction{DeleteUserRolesByRoleID + DeleteRoleMenus + Delete}（service/role.go:141-151） | `RemoveFilteredPolicy(0, roleCode)` + `RemoveFilteredGroupingPolicy(1, roleCode)`（builtin 已守卫不会走到这里） |
| 用户删除 | UserService.Delete 的 db.Transaction{DeleteUserRolesByUserID + user Delete}（service/user.go:375-384） | `RemoveFilteredGroupingPolicy(0, uid)`（放事务成功后、KickoutUser 前） |

不需要同步的写路径：角色 Update（白名单不含 role_code，policy sub 不漂移）；菜单 Update/Delete（p 策略存 perm 串快照，菜单删除后策略行仍在——语义影响是"已回收菜单的 perm 串残留"，v1 可接受，记录为已知边界；如需强一致可在 MenuService.Delete/Update 顺带刷新引用该串的角色策略，属增强非必需）。

### 研究重点 4：requirePerm 中间件与测试策略（HIGH）

**中间件形态**（对齐 RequireRoles 现行形态，middleware/auth.go:70-91）：
```go
// RequirePerm("system:user:list") 返回授权 gate：auth.FromContext 取 identity →
// enforcer.Enforce(strconv.FormatInt(identity.UserID, 10), perm) 命中放行；
// identity 缺失 401（防御性分支）；未命中 resp.Fail(ErrForbidden) 403 Abort。
// super_admin 豁免不在中间件判断——model matcher 首位短路已覆盖（D-07 唯一判据）。
func RequirePerm(enf *authz.Enforcer, perm string) gin.HandlerFunc
```
- 挂载位置：pri 组每条管理路由 handler 前（router.go:45-95），与既有 `middleware.RequireRoles("super_admin")` 同屏逐路由声明（D-08 显式挂载）。
- kickout 端点保留 RequireRoles("super_admin") 叠加（D-15 纵深防御不拆），再叠 requirePerm（见挂载清单）。
- 现有 RequireRoles 过渡 gate 在 requirePerm 全量挂载后**移除**（auth.go:64 注释的 Phase 3 替换点收口；kickout 的叠加例外保留）。

**挂载清单（router.go pri 组 25 条路由全量 → perm 串）** [VERIFIED: router.go:45-95 + sys_menu 容器数据]：

| 路由 | perm 串 | 来源 |
|------|---------|------|
| POST /api/v1/auth/kickout（叠 RequireRoles） | `system:auth:kickout` | 发明（Q2） |
| POST /api/v1/users | `system:user:add` | sys_menu id 1012 |
| PUT /api/v1/users/:id | `system:user:edit` | 1013 |
| GET /api/v1/users/:id | `system:user:query` | 1011 |
| GET /api/v1/users | `system:user:list` | 101 |
| DELETE /api/v1/users/:id | `system:user:remove` | 1014 |
| GET /api/v1/users/:id/roles | `system:user:query` | 1011 |
| PUT /api/v1/users/:id/roles | `system:user:edit` | 1013 |
| PUT /api/v1/users/:id/status | `system:user:edit` | 1013 |
| PUT /api/v1/users/:id/reset-password | `system:user:resetPwd` | 1015 |
| POST /api/v1/menus | `system:menu:add` | 1032 |
| PUT /api/v1/menus/:id | `system:menu:edit` | 1033 |
| GET /api/v1/menus/tree | `system:menu:list` | 103 |
| GET /api/v1/menus/:id | `system:menu:query` | 1031 |
| GET /api/v1/menus | `system:menu:list` | 103 |
| DELETE /api/v1/menus/:id | `system:menu:remove` | 1034 |
| POST /api/v1/depts | `system:dept:add` | 1042 |
| PUT /api/v1/depts/:id | `system:dept:edit` | 1043 |
| GET /api/v1/depts/tree | `system:dept:list` | 104 |
| GET /api/v1/depts/:id | `system:dept:query` | 1041 |
| GET /api/v1/depts | `system:dept:list` | 104 |
| DELETE /api/v1/depts/:id | `system:dept:remove` | 1044 |
| POST /api/v1/roles | `system:role:add` | 1022 |
| PUT /api/v1/roles/:id | `system:role:edit` | 1023 |
| GET /api/v1/roles/:id | `system:role:query` | 1021 |
| GET /api/v1/roles | `system:role:list` | 102 |
| DELETE /api/v1/roles/:id | `system:role:remove` | 1024 |
| GET /api/v1/roles/:id/menus | `system:role:query` | 1021 |
| PUT /api/v1/roles/:id/menus | `system:role:edit` | 1023 |
| POST /api/v1/api-keys | `system:apiKey:add` | 发明（Q2） |
| GET /api/v1/api-keys/:id | `system:apiKey:query` | 发明（Q2） |
| GET /api/v1/api-keys | `system:apiKey:list` | 发明（Q2） |
| PUT /api/v1/api-keys/:id/status | `system:apiKey:edit` | 发明（Q2） |
| DELETE /api/v1/api-keys/:id | `system:apiKey:remove` | 发明（Q2） |

perm 串常量组织（CONTEXT discretion）：建议 `internal/authz/perms.go` 集中常量（按域分注释段），或逐域文件；挂载清单表即常量对照表。

**测试策略**：
- `newTestApp`（handler/user_integration_test.go:166）走 `app.New(cfg, logger)` 完整装配——enforcer 进 app.New 装配链后集成测试**自动继承**，零测试代码改动获取 enforcer [VERIFIED: newTestApp 源码]。
- casbin_rule 表就绪：testutil.RunMySQL 的 `execDDLDir` 自动执行 scripts/ddl/*.sql（testutil/mysql.go:62,78-92 实证：按文件名序全量执行、任一失败整体失败）——新增 `scripts/ddl/casbin_rule.sql` 即自动就绪 [VERIFIED: testutil/mysql.go]。
- 即时生效集成断言写法（PERM-02 验收核心）：
  1. 建角色 R + 用户 U（AssignRoles(U,[R])）→ U 登录拿 token → 请求受保护端点 → 403（无 p 策略）；
  2. admin PUT /roles/{R}/menus 分配含 `system:user:list` 权限串的菜单 → **同一 token**（不重登录）再请求 → 200；
  3. admin 置空 menuIds（全回收）→ 同一 token 再请求 → 403；
  4. dataScope 断言：admin 改 U 角色 data_scope 档位 → U GET /users 列表行数差异可观察（本部门管理员看不到他部门用户）。
  - 注意第 2 步角色→菜单同步写的是 p 行，第 1→2 步之间 U 的 g 行已在 AssignRoles 时同步——验证"不重登录"的完整语义。
- enforcer 单元级测试（免容器）：miniredis 思路不适用于 casbin——casbin 可用纯内存 model + NewEnforcer(m, nil)（无 adapter）测 matcher/豁免/Enforce 语义，毫秒级；gorm-adapter 路径由集成测试覆盖。

### 研究重点 5：dataScope 通用件设计（DDL HIGH / 设计 MEDIUM）

**DDL 实证（研究任务书点名必须实证，已完成）** [VERIFIED: 本机 mysql-server 容器 SHOW CREATE TABLE sys_role，2026-11-01]：
- sys_role 真实 13 列：`id, role_name, role_code, data_scope(tinyint NULL), status, builtin, create_by_id, create_by_name, update_by_id, update_by_name, create_time, update_time, remark`——**仅 data_scope 一列，无任何部门集合列**（无 data_scope_dept_ids / dept_ids 类列）。
- 源仓库佐证：`grep -rn "dataScopeDept|scope_dept|dept_ids"` 全仓库零命中；Role.java 仅 `private Integer dataScope`（Role.java:24）；RoleController 请求体仅 dataScope（RoleController.java:50,63）；editRole 无部门集合参数 [VERIFIED: grep + 源码行号]。
- FEATURES.md:218 原文：「源只有存储字段，零过滤实现（grep 全仓库确认仅 CRUD 存取）」[VERIFIED: .planning/research/FEATURES.md:218]。
- **结论：档位 4（指定部门）的数据来源在源系统与本库均不存在**——Go 版要实现五档必须新增存储（Q1 三方案裁决）。

**perm 串真实数据** [VERIFIED: 容器 SELECT]：41 行菜单、39 行有 permission、前缀全部 `system:`、动作集 `{list,query,add,edit,remove,resetPwd,upload,download}`（D-06 示例 `sys:user:list` 应纠正为 `system:user:list` 风格）。

**现有可复用原料** [VERIFIED: 各文件]：
- `tree.Build`（internal/tree/tree.go:32）是内存树构建器（孤儿提升/环免疫）——dept 子树 ID 展开可在其上或以同款 map 索引思路写一个 `CollectSubtreeIDs(depts []po.DeptPO, rootID int64) []int64`（DeptRepo.List 全量返回 sort ASC，工控机 dept 表百行级，一次查询+内存展开优于递归 SQL）。DeptRepo 现有方法仅 Create/Patch/Delete/List/FindByID，**无子树查询**（repository/dept.go 实证）——需新增内存展开辅助或 repo 方法。
- 用户 dept_id：UserPO.DeptID（model/po/user.go:27 `gorm:"column:dept_id"`）；本人档列 create_by_id 在 BaseWithOperator（model/po/base.go:10）。
- 用户角色档位聚合原料：relationRepo.FindRolesByUserIDs（repository/relation.go:32）+ roleRepo（data_scope 列在 RolePO）。

**GORM Scope 通用件伪代码**（internal/datascope 包，签名属 discretion）：
```go
// Filter 是五档解析产物：service 层先解析（档位聚合），repo 层 Apply 注入 WHERE。
type Filter struct {
    Mode    Mode   // 1..5（源 sys_role.data_scope 值域）
    UserID  int64  // 档位 1 用
    DeptIDs []int64 // 档位 2 单元素 / 3,4 多元素
}
func (f Filter) Apply(db *gorm.DB, userTable string) *gorm.DB {
    switch f.Mode {
    case ModeAll:            return db                       // 5 全部：无过滤
    case ModeSelf:           return db.Where(userTable+".create_by_id = ?", f.UserID)         // 1
    case ModeDept:           return db.Where(userTable+".dept_id = ?", f.DeptIDs[0])          // 2
    case ModeDeptSubtree:    return db.Where(userTable+".dept_id IN ?", f.DeptIDs)            // 3（子树一次查询）
    case ModeCustomDepts:    return db.Where(userTable+".dept_id IN ?", f.DeptIDs)            // 4（自定义集合）
    default:                 return db.Where("1 = 0")        // 未知档 fail-closed（安全默认拒绝）
    }
}

// service 层解析（用户列表挂接点 UserService.List，service/user.go:197）：
// ① auth.FromContext(ctx) 取 uid（handler 已注入，无需改 handler 签名）
// ② userRepo.FindByID(uid) 取 DeptID
// ③ relationRepo.FindRolesByUserIDs + roleRepo 取全部角色 data_scope
// ④ 多角色取 max(data_scope)（D-11 取宽惯例）；无角色 → 最窄档（本人）fail-closed
// ⑤ 档位 3 时 deptRepo.List + CollectSubtreeIDs(user.DeptID)；档位 4 时查 Q1 存储取部门集
// ⑥ repo.List 增加 filter 参数（或 db.Scopes）注入
```
- 未登录/无 identity 的列表调用（理论上不可达——pri 组已挂 Auth）按 fail-closed（空结果或最窄档），与 T-2-04 认证 fail-closed 纪律一致。
- Context menu 备注核对：CONTEXT.md D-06 提"源 sys_menu.perms 字段"——实际源与 Go 列名均为 `permission`（sys_menu DDL 第 8 列实证），研究按 `permission` 记录。

### 研究重点 6：IN-14 apikey→user 反向索引（设计 MEDIUM，挂点 VERIFIED）

**缺口实证** [VERIFIED: internal/auth/session.go:19-32,198-235 与 internal/service/auth.go:92-99]：
- 现有键：`iam:apikey-token:{apiKey}` → token（TTL=timeout）。已知边界（session.go:206-210 注释原文）：会话滚动续期可活得比映射 TTL 久，窗口内 KickoutAPIKey 反查不到。管理面停用/删除已有 KickoutAPIKey 链收口；**自然过期**（ApiKeyPO.ExpireTime，api_key.go:22）无任何触发点——key 过期后存量会话活到 TTL。
- 拒绝路径挂点：ApikeyLogin 的 `if key == nil → 10503`（auth.go:97-99）——FindByKey 登录过滤语义把不存在/停用/过期统一收敛为 nil，此刻**无从知道 user**，这正是反向索引的存在理由（研究任务书判断正确）。

**设计（遵 D-16 拍板）**：
- 新键 `iam:apikey-user:{apiKey}` → userID 十进制串，**String 类型、无 TTL**（键生命周期与 key 行同寿命；研究任务书"SET 或 hash"选 String——单值反查零结构开销）。
- 写入点：`SessionManager.BindAPIKey` 改造为 TxPipelined 原子块：`SET iam:apikey-token:{key} token EX timeout` + `SET iam:apikey-user:{key} userID`（完整登录成功路径已有调用点 auth.go:143）。
- 撤销点：ApikeyLogin 的 `key == nil` 分支，10503 返回前：`GET iam:apikey-user:{key}` → 命中则 `KickoutUser(uid)`（幂等，现成方法 session.go:161）+ `DEL iam:apikey-user:{key}`；未命中直接 10503。撤销失败不阻断 10503 响应（尽力而为 + DEL 键防反复触发；用户轴兜底与 STATE Concerns 补偿路径语义一致）。
- 清理点：ApiKeyService.Delete 管理面删除时顺带 `DEL iam:apikey-user:{key}`（与既有 UnbindAPIKey 同块）。
- 语义边界（记录）：撤销粒度为 user 轴（KickoutUser）——该用户密码登录的活跃会话会被连坐撤销。这是 D-16 原文"撤销其存量会话"的拍板语义，安全从严可接受；精确到"仅该 key 签发的 token"需 token 级反向索引，现网单 key 单活 token 模型下 apikey-token 映射即定位——但映射 TTL 过期恰是缺口本身，故反查键落 user 轴最稳（无 TTL 恒可反查）。
- 新前缀 `iam:apikey-user:` 加进 session.go 的 Redis key 前缀约定注释块（全项目契约文档位，session.go:19-27）。

## Implementation Guidance

### 文件清单（预估）

| 操作 | 文件 | 内容 |
|------|------|------|
| 新增 | `internal/authz/enforcer.go` | SyncedEnforcer 单例装配（TurnOffAutoMigrate→NewAdapterByDBUseTableName→NewSyncedEnforcer→LoadPolicy fail-fast）、model 内嵌 const、RequirePerm 中间件、四写路径同步方法（SyncRoleMenus/SyncUserRoles/ClearRole/ClearUser） |
| 新增 | `internal/authz/perms.go` | perm 串常量（挂载清单表全量）+ model 字符串 |
| 新增 | `internal/datascope/datascope.go` | Filter 五档 + Apply + Resolve 辅助（子树展开 CollectSubtreeIDs） |
| 新增 | `scripts/ddl/casbin_rule.sql` | casbin_rule 显式 DDL（见下） |
| 新增 | Q1 拍板后 `scripts/ddl/sys_role_dept.sql`（推荐）或 ALTER | 档位 4 部门集合存储 |
| 修改 | `internal/app/app.go` | 装配链 Redis Ping 之后、services 之前初始化 enforcer（D-04 fail-fast 对齐 Redis 先例），注入 roleSvc/userSvc/authSvc(?) 与 router |
| 修改 | `internal/app/router.go` | pri 组 25 路由逐条挂 RequirePerm（挂载清单）；RequireRoles 移除（kickout 例外保留叠加） |
| 修改 | `internal/service/role.go` | AssignMenus/Delete 事务成功后同步 casbin（含去重） |
| 修改 | `internal/service/user.go` | AssignRoles/Delete 同步 casbin；List 挂 dataScope Resolve |
| 修改 | `internal/service/auth.go` | ApikeyLogin 拒绝路径反查撤销（IN-14） |
| 修改 | `internal/auth/session.go` | BindAPIKey 改 TxPipelined 双 SET；Unbind/新 DEL 反查键；前缀注释块加 iam:apikey-user: |
| 修改 | `internal/repository/user.go`（或 repo 签名） | List 接受 Filter 注入 |
| 新增测试 | `internal/handler/perm_integration_test.go` 等 | 即时生效/403/dataScope 五档契约测试（见 Validation Architecture） |
| 新增测试 | `internal/authz/enforcer_test.go` | 纯内存 matcher/豁免单元测试 |

### casbin_rule 建表 DDL（probe 实测版）

```sql
-- casbin_rule 建表 DDL（Phase 3 PERM-04 原生策略表）
-- 结构对齐 gorm-adapter v3.35.0 CasbinRule（adapter.go:48-61）；唯一索引必须自带——
-- TurnOffAutoMigrate 后 adapter 不建表也不建索引（adapter.go:410-417 实证）。
DROP TABLE IF EXISTS `casbin_rule`;
CREATE TABLE `casbin_rule` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `ptype` varchar(100) DEFAULT NULL,
  `v0` varchar(100) DEFAULT NULL,
  `v1` varchar(100) DEFAULT NULL,
  `v2` varchar(100) DEFAULT NULL,
  `v3` varchar(100) DEFAULT NULL,
  `v4` varchar(100) DEFAULT NULL,
  `v5` varchar(100) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_casbin_rule` (`ptype`,`v0`,`v1`,`v2`,`v3`,`v4`,`v5`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```
注：CasbinRule.ID 是 Go uint——MySQL bigint 自增无碍（uint64 范围内）；项目其余表无自增（雪花），casbin_rule 是例外（adapter 内部管理 ID，业务代码不触碰该表——D-14 禁独立管理端点即为此）。

### 双轨同步伪代码（AssignMenus 例，其余同构）

```go
func (s *RoleService) AssignMenus(ctx, roleID, menuIDs) error {
    // …… 既有校验 + db.Transaction{DeleteRoleMenus + BatchCreateRoleMenus} 原样不动 ……
    err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { /* 既有 */ })
    if err != nil { return err }
    // —— 事务提交成功后，同步 Casbin p 策略（D-13；失败上抛 500 重试自愈）——
    role := /* 已查角色行（含 RoleCode） */
    perms := make([]string, 0, len(menuIDs))
    seen := make(map[string]struct{}, len(menuIDs))          // Error 1869 防御：去重必须
    if len(reqIDs) > 0 {
        menus, err := s.menuRepo.FindByIDs(ctx, reqIDs)       // 已查过可复用
        for i := range menus {
            if p := menus[i].Permission; p != nil && *p != "" {
                if _, dup := seen[*p]; !dup { seen[*p] = struct{}{}; perms = append(perms, *p) }
            }
        }
    }
    if _, err := s.enf.RemoveFilteredPolicy(0, role.RoleCode); err != nil { return err }
    if len(perms) > 0 {
        rules := make([][]string, 0, len(perms))
        for _, p := range perms { rules = append(rules, []string{role.RoleCode, p}) }
        if _, err := s.enf.AddPolicies(rules); err != nil { return err }
    }
    return nil
}
```

### Installation

```bash
go get github.com/casbin/casbin/v2@v2.135.0 github.com/casbin/gorm-adapter/v3@v3.35.0
```
（MVS 会自动带上 casbin/v2 的 doublestar/govaluate/uuid 与 adapter 的全方言驱动；go.mod 的 go-sql-driver/mysql 已在 1.8.1 无变化。）

## Validation Architecture

> config.json `workflow.nyquist_validation: false`——按主流程此节可省；因任务输出结构明确要求，给精简版（Phase 1/2 既有基建延续）。

| Property | Value |
|----------|-------|
| 框架 | testify v1.12.1 + testcontainers-go v0.44.0（双容器）+ httptest（既有） |
| 集成命令 | `go test -tags integration ./internal/handler/ -run TestPerm -count=1` |
| 单元命令 | `go test ./internal/authz/ ./internal/datascope/ -count=1` |
| 全量回归 | `go test ./... && go test -tags integration ./... -count=1` |

| 需求 | 行为 | 测试 | Wave 0 |
|------|------|------|--------|
| PERM-01 | RBAC 三元校验 | 单元（内存 enforcer matcher）+ 集成（分配→200） | 复用 newTestApp |
| PERM-02 | 即时生效 | 集成：分配→同 token 下一请求 200→回收→403（上文 4 步写法） | 新增 perm_integration_test |
| PERM-03 | 五档行差异 | 集成：五档各建数据集断言列表行集（含跨部门不可见） | datascope 单测 + 集成 |
| PERM-04 | 原生策略表 | 集成：assignMenus 后直查 casbin_rule 行存在；sys_role_menu 原样 | DDL 就绪自动 |
| PERM-05 | 授权接口管理 | 复用 AssignMenus 契约 + casbin_rule 落库断言 | 同上 |
| PERM-06 | requirePerm 403 | 集成：无策略角色访问 25 路由采样断言 403/10200 | 同上 |

Wave 0 gaps：无框架缺口（全部复用既有基建）；仅需新测试文件与 enforcer 装配。

## Security Domain（ASVS L1 续期）

| 威胁 | 缓解（本阶段落点） |
|------|---------------------|
| 水平/垂直越权（Broken Access Control） | requirePerm 全量 25 路由收口（D-15/WR-05 挂账清偿）；403 fail-closed；identity 缺失 401 |
| 越权数据读取 | dataScope fail-closed（未知档 `1=0`、无角色最窄档）；Filter 白名单列（dept_id/create_by_id 硬编码，无注入面） |
| 策略注入 | casbin_rule 表业务代码零直写（仅 enforcer API），perm 串来源为 DB permission 列（服务端数据非客户端输入） |
| 过期凭据存续 | IN-14 反查撤销（过期 key 登录拒绝即撤 user 轴会话） |
| matcher 逻辑洞 | super_admin 豁免唯一判据 = enforcer g 层（会话快照判据废弃——撤销角色即时失效，probe 直证） |

## Open Questions

1. **档位 4（指定部门）的部门集合存储——需用户/planner 裁决**
   - 实证：sys_role 无此列（容器 DDL 直证）、源系统无此概念（grep 零命中）。
   - 方案 A（推荐）：新关联表 `sys_role_dept`（id/role_id/dept_id + 时间冗余列，模式同 sys_role_menu 同构）；辩护：与"新增 Casbin 策略表属增量扩展"同类（纯新增不动既有表）；PUT /roles/:id 扩展 scopeDeptIds 字段（API 契约允许重设计，CLAUDE.md 锁定）；data_scope=4 时表驱动。
   - 方案 B：sys_role 加一列（varchar 存逗号分隔 ID 或 JSON）——最小查询成本但破坏"schema 沿用"例外清单的严格性（REQUIREMENTS.md Out of Scope 例外仅点名 Casbin 表）。
   - 方案 C：复用 casbin_rule 存 `d` 行——被 D-14"Casbin 策略表仅作 enforcer 持久化"否决（d 行非 enforcer 消费，LoadPolicy 对未知 ptype 的行为未实证，风险高）。**不推荐**。
   - 建议：planner 按 A 起草，discuss/人工确认时向用户报备一句"档位 4 需要一张新关联表 sys_role_dept（源系统没有这个功能，属增量扩展）"。

2. **api-keys / auth(kickout) 域 perm 串"发明"**
   - 实证：sys_menu 41 行无 apiKey/auth 域 permission 串；assignMenus 只能授菜单存在的串 → 发明的 perm 串仅 super_admin（matcher 豁免）可过，其他角色 v1 无授权入口（D-14 禁独立策略管理面）。
   - 建议：按挂载清单发明（`system:apiKey:*`、`system:auth:kickout`），v1 接受"仅 super_admin 可管 api-keys"现实（Phase 2 前 api-keys 就无菜单联动），前端菜单未来补充时 assignMenus 自然打通。记录为已知边界，不需阻塞。

3. **（已裁决，记录备用）casbin v3 路线**：官方主线已迁 v3（adapter v3.36.0+），若未来多实例/watcher 生态需要 v3 特性，需推翻 D-01 用户决策升级——v1 不动。

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | gorm-adapter 全方言编译使二进制增量约 +3~6MB（未实测具体值） | Key Findings 1 | 低——仅安装包体积，不阻塞功能；验收时 `make verify-static` 顺带实测 |
| A2 | `AddPolicies([])` 空切片行为未单独实测（伪代码用 `len(perms) > 0` 守卫绕开） | Implementation Guidance | 极低——守卫使空批路径不可达 |
| A3 | casbin `RemoveFilteredPolicy` 对 RBAC 模型删 p 行不级联删 g 行（按 API 分工推断，未专项实测） | Key Findings 3 | 低——同步伪代码 p/g 显式分删，不依赖级联；如级联则 RemoveFilteredGroupingPolicy 幂等无害 |
| A4 | LoadPolicy 对 model 未定义的 ptype 行会报错（方案 C 否决依据，未实测） | Open Questions Q1 | 无——方案 C 已不推荐 |
| A5 | 菜单删除后残留策略行的语义影响评估（"已回收 perm 串残留"可接受） | Key Findings 3 | 低——v1 无菜单删除联动场景的强一致需求；Phase 4 前端联调时再评估 |

## Environment Availability

| 依赖 | 需求方 | 可用 | 版本 | 兜底 |
|------|--------|------|------|------|
| mysql-server 容器（DDL 实证源） | 研究/开发调试 | ✓（Up 20h） | mysql:8.4.8 | testcontainers 自动（测试不锚定本机） |
| Docker | testcontainers 集成测试 | ✓ | — | — |
| proxy.golang.org | go get casbin 依赖 | ✓（实测拉取成功） | — | 无需 |
| redis-server 容器 | 会话链测试 | ✓（Up 20h） | redis:8.6.2 | testcontainers 自动 |

无阻塞缺口。

## Metadata

- **研究方法**：三级实证——① proxy.golang.org 版本/依赖闭包直查；② 本地 probe module（/tmp/casbin-probe-v2）对真实 MySQL 8.4.8 容器端到端运行（boot/Enforce/豁免/即时回收/持久化/唯一索引全 PASS）；③ 源码行号级 API 签名确认（GOMODCACHE）+ 本机容器 DDL/数据直证 + 源 Java 仓库 grep。文档类结论仅用于交叉印证（Context7 /apache/casbin、gorm-adapter README）。
- **置信度**：版本矩阵/集成 API/坑清单/DDL = HIGH（实测）；dataScope 设计/IN-14 设计 = MEDIUM（设计类，挂点已实证）；二进制增量 = LOW（A1 假设）。
- **与 CONTEXT.md 的冲突记录**：D-13 的 `RemoveFilteredPolicies` API 名不存在（实为 `RemoveFilteredPolicy`，已按实际 API 设计）；D-06 示例 `sys:user:list` 与真实数据 `system:user:list` 风格不符（按真实数据执行）；D-07 的"会话 roles 或 g 策略"二选一 → 推荐 g 策略唯一判据（即时生效自洽性要求，会话快照判据会使撤销不即时）。三处均为实现细节级纠正，不推翻 D-01..D-16 任何决策本身。
- **缓存**：两条关键 digest 已入 research-store（key 780f22e9… / d97d9abe…，2026-11-01）。
- **Valid until**：版本矩阵 30 天（casbin 发布节奏稳定）；坑清单长期有效（实测结论）。
