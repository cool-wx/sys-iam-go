---
phase: 03-casbin
reviewed: 2026-11-01T07:56:20Z
depth: standard
files_reviewed: 19
files_reviewed_list:
  - internal/authz/enforcer.go
  - internal/authz/perms.go
  - internal/middleware/perm.go
  - internal/app/app.go
  - internal/app/router.go
  - internal/repository/relation.go
  - internal/service/role.go
  - internal/service/user.go
  - internal/datascope/datascope.go
  - internal/auth/session.go
  - internal/handler/role.go
  - internal/model/dto/role.go
  - internal/model/po/relation.go
  - internal/repository/dept.go
  - internal/repository/user.go
  - internal/service/auth.go
  - scripts/ddl/casbin_rule.sql
  - scripts/ddl/sys_role_dept.sql
  - go.mod
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
status: clean
---

# Phase 3: Code Review Report（Re-review，iteration 3/3 最终收敛判定）

**Reviewed:** 2026-11-01T07:56:20Z
**Depth:** standard
**Files Reviewed:** 19（编排者指定清单全量；本轮变更面重读 + 未变文件与 iter2 验证态一致性核实）
**Status:** clean

## Summary

对 iteration 2 的 4 项修复（99d00ec / fad2be4 / 19e43ef / ffcbdec）逐项验证：**4 项全部通过**，无新缺陷。三项 C/W 修复的编排者裁决差异（HasRole 上抛签名、10604 独立码 409、混合集合整体拒绝）落地均正确且语义优于原建议。除 4 个修复提交外，自 iter2 复审树（f0d6ef1）起源码零变更（`git diff --stat f0d6ef1..HEAD` 仅 8 文件=4 生产 + 4 测试），iter2 已验证的其余 15 个 phase 文件逐位一致，前两轮结论直接继承。

最终判定：**三轮收敛达成**——无 Critical、无 Warning，Info 亦无新增。`go build`/`go vet`/`gofmt -l`/6 包单元测试全绿，集成测试编译通过（`go vet -tags=integration`）。

---

## 修复验证结论（iteration 2 的 4 项）

| Finding | 修复提交 | 验证结论 | 关键证据 |
|---------|---------|---------|---------|
| CR-04 守卫判据改 enforcer g 层 | 99d00ec | **通过** | 判据源、错误传播、并发面、回归测试四层全过，详下 |
| WR-06 builtin 不可停用守卫 | fad2be4 | **通过** | 前置位置零写调用、写路径唯一性、锁死链封死，详下 |
| WR-07 停用写路径封口 | 19e43ef | **通过** | 两轴守卫前置、混合集合整体拒绝、AssignScopes 豁免理由成立，详下 |
| IN-07 builtin 可写裁决注释 | ffcbdec | **通过** | user.go:160-166 注释与裁决文本一致（源语义保真 + 残余风险记录位置），零行为变更 |

### CR-04（99d00ec）——判据源、错误传播、并发面三层验证

1. **判据源正确**：`assignerIsSuperAdmin`（user.go:347-353）改读 `enf.HasRole`，identity 缺失 `(false, nil)` fail-closed。`HasRole`（enforcer.go:172-183）sub 十进制串与 SyncUserRoles/ClearUser g 行口径一致，与 D-07「唯一判据在 enforcer g 层」纪律全面对齐。全库唯一调用点（user.go:439），无旁路。
2. **错误传播正确（fail-closed）**：`isSuper, err := s.assignerIsSuperAdmin(ctx); if err != nil { return err }`（user.go:439-441）——查询错误在事务/双轨零写前原样上抛走 500 既有语义，不吞错不误放。已实证 casbin v2.135.0 源码：`RoleManagerImpl.GetRoles` 对无 g 行用户返回空切片 + `nil` error（role_manager.go:391-397），**不存在无角色操作者被伪 500 拒绝的回归路径**；且 RequirePerm 门先于守卫（能到守卫者必持 ≥1 角色），空集场景仅剩微秒级并发窗口且方向 fail-closed。
3. **并发面安全**：`SyncedEnforcer` 未覆写 `GetRolesForUser`（继承自内嵌 Enforcer、无外层锁），但 casbin 默认角色管理器为 `sync.Map` + `SyncLRUCache` 内建并发实现（RoleManagerImpl 结构实证）——与管理端写（Add/RemoveGroupingPolicies）并发无数据竞争。
4. **direct-only 语义与豁免判据等价（不变量核实）**：matcher 豁免 `g(r.sub,"super_admin")` 是传递闭包而 HasRole 只查直接 g 行——两者等价依赖「g 行恒为 user→role 无 role→role 链」不变量。核实全部 g 行写入路径（SyncUserRoles / SyncRoleUsers / boot 对账 ReplaceAll 导入）均只写 (userID, roleCode)，role→role 链不可达，等价成立。
5. **回归测试端到端直证攻击链**：TestUserRolesBuiltinAssignGuardRevokedSession 逐复现 iter2 攻击链——受害者持 super_admin+editor 登录（快照=真相）→ 管理员仅回收 super_admin（不踢会话，DB 断言 g 行已删）→ 同一旧 token 重分配 builtin → **403/10603**；含回收前 200 对照组与双轨零写断言（g 行保持删除、sys_user_role 无残留）。TestHasRoleGLayer 单测直证删行后下一调用即 false 与无 g 行用户 (false, nil)。

### WR-06（fad2be4）——前置位置、完备性、可恢复性三层验证

1. **前置拦截零写调用**：守卫（role.go:109-120）先于 cols 构造/Patch/ClearRole——仅 `req.Status==DISABLED` 时先行 FindByID 校验 builtin，命中即 409/10604，Patch/ClearRole 一个不触发。ErrNotFound 译 404 域消息，非 NotFound 错误原样上抛。
2. **写路径唯一性核实**：角色 status 全库唯一写路径 = `PUT /roles/:id` → RoleService.Update → 唯一 `roleRepo.Patch` 调用点（role.go:134；router.go:89-100 无其他 status 端点，Create 恒落 status=1）——守卫覆盖完备，无旁路端点。
3. **TOCTOU 不可利用**：守卫 FindByID 与 Patch 间的 builtin 翻转竞态无 API 面——builtin 不在 RoleUpdateReq 白名单（WR-01 保持，dto/role.go:30-35 复核），仅 seed 可设。
4. **锁死后果链封死直证**：TestRoleBuiltinDisableConflict 断言 409/10604、status 保持 ENABLED、p 行未被双清、admin 豁免 g 行未动；builtin 非 status 更新（改名）放行、非 builtin DISABLED/ENABLED 往返照常。
5. **遗留态可恢复（附加正向性质）**：守卫仅拦 DISABLED 不拦 ENABLED——若存量 DB 存在 status=0 的 builtin 角色（守卫落地前的历史态），`{"status":"ENABLED"}` 可走重建路径（SyncRolePerms+SyncRoleUsers，role.go:153-168）恢复 g/p 行，WR-06 未引入新的不可自愈面。

### WR-07（19e43ef）——两轴封口、拒绝语义、豁免边界三层验证

1. **AssignMenus（role.go:265-267）**：守卫位于 FindByID 之后、去重/菜单校验/事务/SyncRolePerms 之前——前置拦截零写调用。NULL status 视作启用（照 UsageStatusFromInt 读路径兜底；DDL DEFAULT '1' 实际不可达 NULL）。
2. **AssignRoles（user.go:420-428）**：statusOf 同次 FindByIDs 顺带收集（零额外查库），任一停用即整体拒绝，消息列出全部停用 id（混合集合不含糊放行）——与「角色不存在: [...]」缺失集合风格同款。守卫位于事务与 SyncUserRoles 之前。
3. **数据源无静默失效**：roleRepo.FindByID/FindByIDs 为全模型 SELECT（repository/role.go:103-127），status/builtin 指针必然带出——守卫判据不依赖限列查询，无「指针恒 nil 致守卫静默失效」面（专项排查项）。
4. **AssignScopes 豁免边界成立**：sys_role_dept 非 enforcer 消费数据（无 p 行原料），dataScope 聚合已滤停用角色（relation.go:102 `JOIN sys_role r ... AND r.status = 1`）——对停用角色 AssignScopes 无任何运行期授权效果，停用期间写入的部门集在恢复后按业务表事实源生效，语义正确非遗漏。
5. **回归测试双轴直证**：TestRoleAssignMenusDisabledRejected（停用→400、p 行零复活、sys_role_menu 不动、恢复后重建）；TestAssignRolesDisabledRoleRejected（单停用与混合集合均 400、g 行与 sys_user_role 零写入、恢复后照常）。

### 交叉排查（修复无新缺陷专项）

| 排查项 | 结论 |
|--------|------|
| HasRole 与管理写并发的数据竞争 | 无——casbin 角色管理器 sync.Map 内建并发安全（上述 CR-04 §3） |
| 无 g 行用户触发伪 500 | 无——GetRoles 对未知用户返回空+nil（casbin 源码实证） |
| role→role g 链导致 direct/transitive 判据分歧 | 不可达——g 行写入三路径均 user→role（CR-04 §4） |
| RequireRoles 快照层残余是否构成新旁路 | 否——仅挂 kickout 单路由（router.go:54），双层 gate 纵深防御：回收者快照层放行但 enforcer 层（PermAuthKickout 仅豁免可达）必拒，fail-closed |
| 角色停用写路径旁路 | 无——全库唯一 Patch 调用点（WR-06 §2） |
| builtin 停用竞态（TOCTOU） | 不可利用——builtin 无 API 可写面（WR-06 §3） |
| WR-06/WR-07 守卫间组合态 | 无冲突——builtin 不可停用故「停用 builtin」组合不可达；守卫均在零写前置位 |
| boot 对账口径与运行期守卫一致性 | 一致——FindUserRoleCodes/FindRolePerms `r.status=1` 不还魂（WR-04 语义），WR-06/WR-07 封死后运行期亦无行可留 |
| Debug 痕迹/空捕获/硬编码密钥全库扫描 | 零命中 |

---

## 已裁决不再重复计（三轮维持）

| 项 | 裁决 | 本轮复核 |
|----|------|---------|
| WR-05 role:edit 自授权面 | deferred 挂账部署清单 | 维持；CR-04 g 层判据落地后残余面与裁决一致 |
| IN-05 预清模式重登录窗口 / IN-06 删除并发孤儿行 | Info 不在 fix scope，记录在案 | 维持；本轮变更未触碰该两处时序结构 |
| IN-07 用户域 builtin 可写 | 源语义保真裁决，注释已落（ffcbdec） | 已闭环 |
| sys_role_menu / sys_role_dept 唯一键 | DDL 变更挂迁移清单 | 维持 |
| WR-07 守卫字面量 `1`（*int8 PO 层）vs 枚举常量 | 不立项——两侧语义注释锚定 + 类型安全比较 + 双调用点风格一致，PO 层比较本需 int8 转换，属风格权衡非实质缺陷 | 本轮记录后关闭 |

---

## 验证记录

- `go build ./...` PASS
- `go vet ./...` PASS；`go vet -tags=integration ./internal/handler/... ./internal/service/... ./internal/app/...` PASS（集成测试编译面）
- `gofmt -l .` 零输出
- 单元测试：authz / errcode / types / middleware / auth / datascope 六包 PASS 全绿（含 TestHasRoleGLayer、10604 锁定表）
- casbin v2.135.0 源码级验证：GetRolesForUser / RoleManagerImpl.GetRoles 语义（未知用户空+nil、sync.Map 并发实现）
- 变更面核实：`git diff --stat f0d6ef1..HEAD` = 4 修复提交、8 文件（4 生产 + 4 测试），iter2 其余 15 个 phase 文件零变更

**收敛判定：clean**——三轮评审循环（11 项初审 → 6 项 iter2 → 4 项 iter2 修复 → 本轮 0 新发现）闭环，无遗留 Critical/Warning。

---

_Reviewed: 2026-11-01T07:56:20Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard（re-review iteration 3/3，最终收敛判定）_
