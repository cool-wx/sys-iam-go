---
phase: 03-casbin
reviewed: 2026-11-01T06:31:59Z
depth: standard
files_reviewed: 30
files_reviewed_list:
  - internal/authz/enforcer.go
  - internal/authz/perms.go
  - internal/authz/enforcer_test.go
  - internal/middleware/perm.go
  - internal/middleware/perm_test.go
  - internal/handler/perm_integration_test.go
  - scripts/ddl/casbin_rule.sql
  - internal/app/app.go
  - internal/app/router.go
  - internal/repository/relation.go
  - internal/service/role.go
  - internal/service/user.go
  - internal/handler/user_integration_test.go
  - internal/datascope/datascope.go
  - internal/datascope/datascope_test.go
  - scripts/ddl/sys_role_dept.sql
  - go.mod
  - internal/auth/session.go
  - internal/auth/session_test.go
  - internal/handler/api_key_integration_test.go
  - internal/handler/auth_integration_test.go
  - internal/handler/role.go
  - internal/handler/role_integration_test.go
  - internal/handler/user_extension_integration_test.go
  - internal/model/dto/role.go
  - internal/model/po/relation.go
  - internal/repository/dept.go
  - internal/repository/user.go
  - internal/service/auth.go
  - internal/service/user_integration_test.go
findings:
  critical: 3
  warning: 5
  info: 4
  total: 12
status: issues_found
---

# Phase 3: Code Review Report

**Reviewed:** 2026-11-01T06:31:59Z
**Depth:** standard
**Files Reviewed:** 30（清单另含 `go.sum`，属锁文件按 scope 规则不作审查对象，仅验证 casbin 依赖条目存在）
**Status:** issues_found

## Summary

对 Phase 3（Casbin 授权与数据权限）31 个变更文件做了全量阅读与交叉验证：matcher 首位短路（测试锁定）、RequirePerm 三分支 fail-closed（identity 缺 401 / enforcer 故障 500 / 未命中 403）、boot 对账幂等（先枚举全删再全加 + 双 map 去重防 Error 1869）、业务事务提交后双轨同步的幂等可重试路径、dataScope 五档 SQL 构造（列名硬编码白名单、值全占位符、case 内空集 `1 = 0` 守卫、BFS visited 防环）、`sys_role_dept` 四连删事务边界、IN-14 反查索引原子双写/双删，均验证为正确实现。`go build`、`go vet ./...`、`gofmt -l` 全部通过。

但发现 3 个 Critical 问题，集中在审查要点指定的两个面上：**双轨同步失败窗口**——`UserService.Delete` 与 `RoleService.Delete` 的 post-commit 级联（ClearUser/ClearRole → 踢会话）一旦中途失败，客户端重试命 404 永远无法自愈，前者留下"已删账号旧 token 整个 TTL 内完全可用"的认证逃逸窗口，后者留下"已删角色权限保留至下次重启"的提权残留窗口；**策略提权**——`PUT /users/:id/roles` 对 builtin 角色零守卫，仅持 `system:user:edit` 的操作者可将 super_admin 分配给自己且下一请求即生效，Phase 3 建立的 least-privilege 体系被一个合法端点整体绕行。另有 5 个 Warning（builtin 可覆写使删除守卫非不变量、AssignMenus/AssignScopes 去重纪律不一致、RequirePerm 吞错误零日志、角色 status 停用对授权零效果、role:edit 自授权面未裁决）与 4 个 Info。

## Critical Issues

### CR-01: 用户删除的 post-commit 失败链不可自愈——被删用户存量会话存活至 TTL（认证逃逸窗口）

**File:** `internal/service/user.go:506-513`
**Issue:** `Delete` 在业务事务提交成功后依次执行 `enf.ClearUser(id)` 与 `sm.KickoutUser(ctx, id)`，两步任一失败都直接 `return err`（500），**短路跳过后续步骤**。此时用户行已删除，客户端重试 DELETE 会命中 `FindByID → ErrNotFound → 404`（`user.go:486-492`），重试永远无法触达踢会话步骤。而认证中间件仅校验 Redis 会话存在性、不回查 DB（`middleware/auth.go:45-53`；这正是 `TestDeleteUserKicksSessions` 注释自述的威胁模型），于是：ClearUser 失败 → KickoutUser 未执行；或 ClearUser 成功但 KickoutUser 失败——两种情况下**已删除账号的存量 token 在整个滚动 TTL 内继续完全可用**，且其 casbin g 行（ClearUser 失败时）原样保留旧权限。这正是删除路径独有的缺陷：`UpdateStatus`/`ResetPassword` 的同类 post-commit 失败可经重试自愈（用户行仍在，幂等重跑），唯独 Delete 的重试被 404 挡死。失败面是两段连续网络调用（casbin_rule SQL 写 + Redis 循环），窗口触发概率不可忽略。
**Fix:**
```go
// 聚合错误而非短路：策略清理失败也绝不能跳过踢会话（踢会话是安全关键步骤）
clearErr := s.enf.ClearUser(id)
kickErr := s.sm.KickoutUser(ctx, id)
if clearErr != nil || kickErr != nil {
    // 结构化日志供运维排查；策略残留由 boot 对账兜底，会话残留必须当场踢掉
    return fmt.Errorf("delete user %d post-commit: clearPolicy=%v, kickSessions=%v", id, clearErr, kickErr)
}
return nil
```
（可选加固：404 分支前若 Redis 索引仍含该 uid 的 token，补一次尽力而为的 KickoutUser。）

### CR-02: 角色删除的 ClearRole 失败同样不可自愈——已删角色权限保留至下次重启

**File:** `internal/service/role.go:148-169`
**Issue:** 与 CR-01 同构的四连删事务提交后 `return s.enf.ClearRole(role.RoleCode)`（`role.go:168`）。ClearRole 失败 → 500；重试 DELETE 命中 `FindByID → 404`（`role.go:138-144`），永不可自愈。后果：该 roleCode 的 casbin p 行（角色→perm）与 g 行（用户→角色）**原样残留**，尽管 `sys_user_role` 关联已随四连删事务清空——即已被 admin 显式收回（删除）的角色权限，对原持有用户**继续生效直到服务下次重启**（boot 对账 `ReplaceAll` 才会按业务表重建清掉）。工控机单机场景重启周期长，这是一个以周计的提权残留窗口。且代码无任何日志记录该失败，残留静默存在。
**Fix:**
```go
if err := s.enf.ClearRole(role.RoleCode); err != nil {
    // 残留 p/g 行在下次重启前仍授予权限——必须 ERROR 级显式留痕，不可静默
    return fmt.Errorf("role %d(%s) deleted but casbin ClearRole failed: %w", id, role.RoleCode, err)
}
return nil
```
并建议：提供按 roleCode 的补偿清理入口（或复用既有重试语义文档化"重启自愈"边界），确保失败可观察、可人工收敛。

### CR-03: PUT /users/:id/roles 对 builtin 角色零守卫——system:user:edit 持有者可自助提权为 super_admin

**File:** `internal/service/user.go:340-415`（AssignRoles）；挂载点 `internal/app/router.go:66`
**Issue:** `AssignRoles` 对 roleIDs 仅做存在性校验（`FindByIDs` + missing 集合 400），**不检查 builtin**。任何持 `system:user:edit` 权限的操作者可调用 `PUT /api/v1/users/{任意用户或自己}/roles` 传入 super_admin 的 roleID：业务表与 casbin g 行经 `SyncUserRoles` 即时双写，豁免判据在 enforcer g 层（D-07），**下一请求即具备 super_admin 全权**。规划文档（03-03-PLAN/03-RESEARCH）检索确认无任何裁决接受此路径——D-14 只封了"独立策略管理面"（直写 casbin_rule 的端点），未封经合法角色分配端点的提权链。这与本阶段自己修复 WR-05（"任何低权用户可踢 admin 会话"收口为 super_admin）的安全标准直接矛盾：踢会话尚且收紧，变身为 super_admin 却不设防。发现 super_admin roleID 需 `system:role:list/query` 之一，但雪花 ID 可枚举面低不等于无——`system:user:edit` + 任意相邻角色管理权限的组合即完整攻击链。
**Fix:**
```go
// AssignRoles 前置守卫（照 RoleService.Delete 的 builtin 守卫哲学：前置拦截零写调用）
for i := range roles {
    if roles[i].Builtin != nil && *roles[i].Builtin {
        return errcode.New(errcode.CodeForbidden, "系统内置角色不可分配")
    }
}
```
集成测试补断言：持 user:edit 的角色分配 super_admin → 403/409 且 casbin g 行零变化。

## Warnings

### WR-01: builtin 标志可经 Create/Update 任意写——角色删除守卫不是不变量

**File:** `internal/service/role.go:60-63`（Create）、`internal/service/role.go:91-93`（Update）
**Issue:** `RoleUpdateReq` 白名单包含 `builtin`，`RoleCreateReq` 亦接受显式 `builtin:true`。两个后果：① `system:role:edit` 持有者可对 super_admin 角色先 `PUT /roles/:id {"builtin":false}` 再 DELETE——CR 的 builtin 删除守卫被整体绕行，四连删清掉 `sys_user_role` 后 boot 对账再也无法重建 admin 的 g 行，**全系统管理员豁免被永久摧毁**（权限锁死型破坏）；② 反向地，`role:add` 可制造 `builtin:true` 的"永生角色"（守卫使其永不可删）。守卫在 `Delete` 里检查的值恰好是可被同权限角色自由改写的字段，防线形同虚设。
**Fix:** 将 `builtin` 移出 Create/Update 可写白名单（builtin 只能由 seed/迁移流程设定）；或 Update 中禁止对已存在行翻转 builtin（读取原行，新旧不一致即 400）。

### WR-02: AssignMenus / AssignScopes 不去重请求 ID——与 AssignRoles 纪律不一致，产生重复脏行

**File:** `internal/service/role.go:184-187`（AssignMenus reqIDs）、`internal/service/role.go:277-280`（AssignScopes reqIDs）；对照 `internal/service/user.go:350-359`（AssignRoles 的 reqSeen 去重）
**Issue:** `AssignRoles` 对重复 roleID 做了 map 去重（注释明言 Error 1869 防御），但同文件的 `AssignMenus`/`AssignScopes` 只对 **permission 串**去重（`permSeen`），对 reqIDs 本身不去重。`{"menuIds":["1","1"]}` 会在 `sys_role_menu` 插入两行完全重复记录（两表均无 (role_id, menu_id)/(role_id, dept_id) 唯一键，仅普通索引，`scripts/ddl/sys_role_dept.sql:19-20`）；`GET /roles/:id/menus`（`Pluck`，`relation.go:165-177`）会把重复 menuId 原样回显，响应契约失真。casbin 侧因 FindRolePerms/boot 对账去重而幸免，但业务表脏数据持续累积且无法解释。
**Fix:** 照 AssignRoles 模板给两处 reqIDs 加 `reqSeen` 去重；中期给 `sys_role_menu(role_id, menu_id)`、`sys_role_dept(role_id, dept_id)` 补唯一键。

### WR-03: RequirePerm 吞掉 enforcer 错误且零日志——授权基建故障不可诊断

**File:** `internal/middleware/perm.go:35-40`
**Issue:** `az.Enforce` 出错时收敛为 500 fail-closed（方向正确），但 error 对象被完全丢弃，没有任何 slog 记录。当 casbin/gorm-adapter/DB 出现故障时，表现是全站无差别 500（每条请求都过 RequirePerm），而日志里只有 AccessLog 的 status 行，无任何根因线索。对一个 IAM 服务，授权链故障是 P0 事件，必须有可归因的日志。
**Fix:**
```go
if err != nil {
    slog.Error("enforcer enforce failed", "sub", identity.UserID, "perm", perm, "error", err)
    resp.Fail(c, errcode.ErrInternal)
    c.Abort()
    return
}
```

### WR-04: 角色 status=DISABLED 对全部授权路径零效果——状态字段是死配置

**File:** `internal/repository/relation.go:62-67`（FindRolesByUserIDs）、`relation.go:96-101`（FindRoleScopesByUserIDs）、`relation.go:140-146`（FindRolePerms）；消费方 `internal/service/auth.go:174-181`（completeLogin）、`internal/app/app.go:95-98`（boot 对账）
**Issue:** 三条角色相关查询的 `JOIN sys_role` 均无 `r.status = 1` 过滤。后果链：停用某角色后，① casbin p 行继续为该角色授权（运行期无任何失效动作）；② boot 对账照常导入停用角色的 p 行；③ 用户登录时角色装配照常把停用角色写进会话快照、dataScope 档位照常参与聚合。即 PUT /roles/:id `{"status":"DISABLED"` 在授权语义上完全无操作，仅是展示字段。用户轴的停用（UpdateStatus→Kickout）已闭环，角色轴没有对应物，二者不对称。
**Fix:** 明确裁决二选一并落地：① 角色查询链补 `r.status = 1`（停用角色的 p/g 行在 boot 对账不导入 + AssignMenus 同步时拒绝对停用角色授权 + 登录装配滤停用）；② 或在文档中声明角色 status 仅展示、停用请走"清空 menuIds"路径——但后者应同步在 API 层拒绝或标注，避免运维误操作。

### WR-05: system:role:edit 即无限制自授权面——D-14"禁独立策略管理面"可经合法端点整体绕行

**File:** `internal/app/router.go:95`（PUT /roles/:id/menus 挂 PermRoleEdit）；`internal/service/role.go:176-245`（AssignMenus 无角色归属/权限parity约束）
**Issue:** `AssignMenus` 允许对**任意角色**（含调用者自己所属角色）授予 sys_menu 中存在的**任意** permission 串。持 `system:role:edit`（而无其他管理权限）的操作者给自己的角色追加 `system:user:edit` 菜单 → 同一 token 下一请求即持 user:edit → 再经 CR-03 链条变身 super_admin；即便封掉 CR-03，role:edit 单独就等价于"任意权限授予权"。这是 03-RESEARCH"assignMenus 只能授菜单存在的串 → 发明串仅 super_admin 可过"分析的反面：菜单**存在**的串全部可自授。规划文档未见对此信任边界（role:edit ⊇ super_admin 能力）的显式裁决。
**Fix:** 至少择一：① 文档显式裁决"role:edit = 全权授予信任级"并写入部署清单（只授予 super_admin）；② 代码约束：AssignMenus 只允许操作调用者已持有其全部 perm 的角色、或禁止对自身所属角色操作（需 identity 参与，属 API 契约变更需评审）。

## Info

### IN-01: go.mod/go.sum 未 tidy——casbin 两依赖被错标 indirect，go.sum 有哈希漂移

**File:** `go.mod:31-32`
**Issue:** `internal/authz` 直接 import `github.com/casbin/casbin/v2` 与 `github.com/casbin/gorm-adapter/v3`，但两者被列在 indirect 段；`go mod tidy -diff` 另显示 go.sum 存在应补哈希条目。不影响构建（go vet/build 均过），属依赖元数据失真，误导后续维护者对直接依赖的判断。
**Fix:** 运行 `go mod tidy` 并提交。

### IN-02: dataScope 无 1..5 域校验——域外值静默接受，可被用于定向"清空视图"

**File:** `internal/model/dto/role.go:16,27`
**Issue:** `RoleCreateReq.DataScope`/`RoleUpdateReq.DataScope` 为裸 `*int8`，无 `min=1,max=5` binding；传入 99/-1 不报 400，落库后经 `Widest` max 语义参与聚合、`Apply` default 分支 fail-closed（行为安全，测试已锁定）。但反向后果是：role:edit 持有者可将他人角色 dataScope 置 99，使该角色全部用户的列表视图静默清空（fail-closed 变成定向 DoS 面），且 API 层对非法输入零反馈。
**Fix:** DTO 增加 binding 校验（validator 对 `*int8` 支持 deref）：`binding:"omitempty,min=1,max=5"`，域外值 400/10401。

### IN-03: UserService.Update 在 deptId 缺席时接受客户端 deptName 直写——T-2-15 纪律半覆盖

**File:** `internal/service/user.go:146-148`
**Issue:** `Create` 与 `Update` 的 deptId 存在分支均以查库值覆写 dept_name（服务端冗余不可伪造），但 `Update` 的这段独立分支在请求只带 `deptName` 不带 `deptId` 时把客户端值直写 `dept_name` 列——与全文件声明的"dept_name 以查库值覆写（请求体 deptName 键被忽略）"纪律自相矛盾，展示列可漂移/伪造为任意串（无权限后果，纯数据完整性）。
**Fix:** 删除该分支（deptName 不再是 Update 白名单键），或仅当其与 dept 行当前名一致时才写。

### IN-04: 过期注释——handler Delete 仍写"三连删事务"（现为四连删）

**File:** `internal/handler/role.go:81-82`
**Issue:** `RoleHandler.Delete` 注释 "service（builtin 守卫 + 三连删 事务）"未随 D-10 第四删（sys_role_dept）更新，与 `service/role.go` 的"四连删"注释矛盾，误导读者低估删除面。
**Fix:** 注释改为"四连删事务（user_role→role_menu→role_dept→role）"。

---

_Reviewed: 2026-11-01T06:31:59Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
