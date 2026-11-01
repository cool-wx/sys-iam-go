---
phase: 03-casbin
fixed_at: 2026-11-01T12:30:00Z
review_path: .planning/phases/03-casbin/03-REVIEW.md
iteration: 2
findings_in_scope: 4
fixed: 4
skipped: 0
status: all_fixed
---

# Phase 3: Code Review Fix Report（iteration 2，正文含两轮累计台账）

**Fixed at:** 2026-11-01T12:30:00Z
**Source review:** .planning/phases/03-casbin/03-REVIEW.md（re-review iteration 2/3）
**Iteration:** 2
**分支:** gsd/phase-02-rbac（编排者裁决：直接提交，未建 worktree——与 iteration 1 同模式）

**Summary:**
- Findings in scope: 4（fix_scope=critical_warning 共 3 项 CR-04/WR-06/WR-07 + 编排者指令 IN-07 顺带 1 项）
- Fixed: 4（3 项逻辑类修复标注 requires human verification）
- Skipped: 0

## Verification

| Tier | 检查 | 结果 |
|------|------|------|
| 2 | `go build ./...` | PASS |
| 2 | `go vet ./...` | PASS |
| 2 | `gofmt -l` | 干净（零输出） |
| 2 | `go test ./...`（单元） | PASS 全绿（authz 含新增 TestHasRoleGLayer；errcode 含 10604→409 映射断言） |
| 3 | `TZ=UTC go test -tags=integration -count=1 -timeout 30m ./...` | PASS 全绿（handler 包 739.364s，app 26.797s，service 11.036s；含本轮 4 个新增集成回归测试） |

**运行位置：主 checkout**（`/home/wx/workspace/go/sys-iam-go`，gsd/phase-02-rbac 分支）——编排者裁决直接提交模式，未建隔离 worktree，上述数字可从当前工作树直接复现。每 finding 提交前按 Tier 2 局部验证（build/vet/gofmt），4 个提交全部落盘后一次性跑全量 Tier 3（exit 0）。

## Fixed Issues（iteration 2）

### CR-04: CR-03 守卫判据用会话 roles 快照而非 enforcer g 层——super_admin 回收可被存量会话完全抵消

**Files modified:** `internal/authz/enforcer.go`, `internal/service/user.go`, `internal/authz/enforcer_test.go`, `internal/handler/user_extension_integration_test.go`
**Commit:** 99d00ec
**Status:** fixed: requires human verification（守卫判据源逻辑变更，集成测试端到端直证攻击链）
**Applied fix:** 按编排者裁决（`HasRole(userID int64, role string) (bool, error)` 签名，偏离 REVIEW.md 原建议的 `bool` 吞错签名）：① authz.Enforcer 新增 g 层读原语 `HasRole`——封装 `GetRolesForUser`（sub 十进制串与 SyncUserRoles 口径一致），查询错误原样上抛由调用方 fail-closed 走 500 既有语义，不吞错返回 false；② `assignerIsSuperAdmin` 判据源从 `identity.Roles` 会话快照改为 `s.enf.HasRole` 实时判定——快照是登录时静态副本且滚动续期永不过期，g 层经 SyncUserRoles 即时同步，回收下一请求即失效，与 D-07「唯一判据在 enforcer g 层」纪律全面对齐；identity 缺失仍 fail-closed。回归：enforcer_test `TestHasRoleGLayer`（删 g 行后下一调用即 false 直证）；handler 集成 `TestUserRolesBuiltinAssignGuardRevokedSession`——REVIEW 攻击链逐步直证：受害者持 super_admin+editor（editor 带 user:edit p 行）登录 → 管理员仅回收 super_admin（不踢会话）→ 同一旧 token 重分配 builtin → **403/10603**（快照判据此处会放行 200），含回收前 200 对照组与双轨零写断言。

### WR-06: builtin 角色可经 Update 停用——status 路径直达同等破坏且重启不自愈

**Files modified:** `pkg/errcode/errcode.go`, `pkg/errcode/errcode_test.go`, `internal/service/role.go`, `internal/authz/enforcer.go`, `internal/handler/role_integration_test.go`
**Commit:** fad2be4
**Status:** fixed: requires human verification（新增守卫逻辑，集成测试端到端直证）
**Applied fix:** 按编排者裁决选独立码（「对齐 Delete 409 的 10602 族或独立码」二选一取后者）：errcode 新增 `CodeBuiltinRoleStatus = 10604`「系统内置角色不可停用」（HTTP 409，D-14 builtin 守卫族语义对齐 10602；独立码因消息语义删除/停用有别），HTTPStatus 映射与单测锁定表同步。`RoleService.Update` 前置守卫（前置拦截零写调用，与 Delete 守卫对称）：仅 `req.Status == DISABLED` 时先行查行校验 builtin，命中即 409/10604，Patch/ClearRole 一个都不触发；ErrNotFound 译 404 域消息；仅拦 DISABLED，改名/档位/ENABLED 零误伤。顺带修正 `enforcer.go` ClearRole 失真注释（WR-04 落地起「已被 service 守卫拦截」即失真——Update 路径本可走到 ClearRole；现改为「Delete 不可删除 + Update 不可停用双守卫前置拦截」）。回归：`TestRoleBuiltinDisableConflict`——停用 409/10604、status 保持 ENABLED、p 行未被 ClearRole 双清、admin 豁免 g 行未动（锁死后果链封死直证）、builtin 非 status 更新放行、非 builtin DISABLED/ENABLED 往返照常。

### WR-07: WR-04 停用语义写路径未封口——AssignMenus/AssignRoles 对停用角色照常双写 p/g 行

**Files modified:** `internal/service/role.go`, `internal/service/user.go`, `internal/handler/role_integration_test.go`, `internal/handler/user_extension_integration_test.go`
**Commit:** 19e43ef
**Status:** fixed: requires human verification（新增前置校验逻辑，集成测试端到端直证）
**Applied fix:** 两处写路径前置校验目标角色 status，DISABLED 拒绝（沿域 400/10400 消息风格，REVIEW.md 原建议方向）：① `AssignMenus` 在角色校验后检查 `role.Status`，非 ENABLED → 400「角色已停用，不可分配: {id}」；② `AssignRoles` 在 FindByIDs 同次查询顺带收集 `statusOf`，缺失集合校验后检查——任一角色停用即整体拒绝（混合集合不含糊放行），消息沿域 400 缺失集合风格「角色已停用，不可分配: [逗号拼接]」。NULL status 视作启用（照 UsageStatusFromInt「非 0 一律 ENABLED」读路径兜底；DDL status DEFAULT '1' 实际不可达 NULL）。AssignScopes 未动（REVIEW/编排者范围仅限两处 casbin 双轨写路径；sys_role_dept 非 enforcer 消费数据）。回归：`TestRoleAssignMenusDisabledRejected`（停用→分配 400、p 行零复活、sys_role_menu 不动、恢复后照常且 p 行重建）；`TestAssignRolesDisabledRoleRejected`（单停用与混合集合均 400、g 行与 sys_user_role 零写入、恢复后照常）。

### IN-07: UpdateUserReq.Builtin 用户域白名单保留——裁决记录注释

**Files modified:** `internal/service/user.go`
**Commit:** ffcbdec
**Status:** fixed（注释-only 零行为变更）
**Applied fix:** 按编排者裁决不改行为，仅在 Update 白名单 builtin 分支补裁决记录注释：用户域 builtin 保留可写 = 源语义保真（源 User.java:126-127 withEdit 链接受 builtin 参数），刻意与角色域不对称（WR-01 已收紧角色域）——非遗漏，勿「对齐角色域」误改；残余锁死风险（user:edit+remove 翻转唯一 super_admin 持有者 builtin 后删除）标注记录位置（03-REVIEW iteration 2 IN-07 / 本报告）。

## Skipped Issues

无——本轮 scope 内 4 项全部修复。

---

## 两轮累计台账（iteration 1 + iteration 2）

### Iteration 1（2026-11-01T08:05Z，源初审 11 fixed + 1 skipped；复审验证 8 通过、WR-04 封口不完整、CR-03 判据有洞 → 衍生本轮 CR-04/WR-06/WR-07）

| Finding | Commit | 结果 |
|---------|--------|------|
| CR-01 用户删除预清模式 | a2a7f5e | fixed（复审通过） |
| CR-02 角色删除预清模式 | 06ca7f8 | fixed（复审通过） |
| CR-03 builtin 分配守卫 | a8b1d4a | fixed（复审：守卫已挂但判据有洞 → CR-04） |
| WR-01 builtin 退出可写面 | e514386 | fixed（复审通过） |
| WR-02 请求 ID 去重 | 5c382e9 | fixed（复审通过） |
| WR-03 RequirePerm slog | efa2385 | fixed（复审通过） |
| WR-04 角色停用即刻生效 | 85e25a7 | fixed（复审：主体通过，写路径封口不完整 → WR-06/WR-07） |
| IN-01 go mod tidy | 0b913e1 | fixed（复审通过） |
| IN-02 dataScope 域校验 | c05dcb5 | fixed（复审通过） |
| IN-03 deptName 白名单剔除 | 5f7bcb8 | fixed（复审通过） |
| IN-04 三连删注释族 | f0d6ef1 | fixed（复审通过） |
| WR-05 role:edit 自授权面 | — | skipped: deferred（编排者裁决挂账部署清单；复审维持，CR-04 修复后残余面与裁决一致） |

### Iteration 2（本轮）

| Finding | Commit | 结果 |
|---------|--------|------|
| CR-04 守卫判据改 enforcer g 层 | 99d00ec | fixed: requires human verification |
| WR-06 builtin 不可停用守卫 | fad2be4 | fixed: requires human verification |
| WR-07 停用写路径封口 | 19e43ef | fixed: requires human verification |
| IN-07 builtin 可写裁决注释 | ffcbdec | fixed |

两轮合计：15 fixed（1 项 skipped deferred）。

## 编排者裁决差异记录（iteration 2）

| Finding | REVIEW.md 原建议 | 实际采用（编排者裁决） | 差异理由 |
|---------|-----------------|----------------------|---------|
| CR-04 | `HasRole(userID, roleCode) bool`（err 吞掉返回 false） | `HasRole(...) (bool, error)`，err 上抛 → 500 | 吞错返回 false 会把存储故障译成资格误判且不可诊断；上抛走 fail-closed 既有 500 语义 |
| WR-06 | `errcode.New(CodeInvalidParam, "系统内置角色不可停用")`（400） | 独立码 10604（HTTP 409） | 与 10601/10602 删除守卫同族资源状态冲突语义（D-14 builtin 守卫族统一 409），非参数错误 |
| WR-07 | 循环内逐角色 400 | 同 400 方向，落地为缺失集合风格「角色已停用，不可分配: [逗号拼接]」+ 混合集合整体拒绝 | 沿域既有消息风格（「角色不存在: [...]」同款）；任一停用即拒语义不含糊 |
| IN-07 | 无需代码变更，补裁决注释 | 同（注释-only 零行为变更） | 一致 |

REVIEW-FIX.md 本身未提交（由编排者/workflow 统一提交）。

---

_Fixed: 2026-11-01T12:30:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 2_
