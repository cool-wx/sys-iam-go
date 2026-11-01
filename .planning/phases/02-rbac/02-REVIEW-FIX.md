---
phase: 02-rbac
fixed_at: 2026-11-01T02:02:10Z
review_path: .planning/phases/02-rbac/02-REVIEW.md
iteration: 2
findings_in_scope: 6
fixed: 5
skipped: 1
status: partial
---

# Phase 2: Code Review Fix Report

**Fixed at:** 2026-11-01T02:02:10Z
**Source review:** .planning/phases/02-rbac/02-REVIEW.md（iteration 2 复审）
**Iteration:** 2
**Fix scope:** critical_warning + 编排者本轮点名纳入的 4 个 Info（IN-09/IN-10/IN-12/IN-13）；IN-11 编排者裁决不修

**Summary:**
- Findings in scope: 6
- Fixed: 5
- Skipped: 1（IN-11，编排者裁决不修，理由见 Skipped 节）

**验证环境（#2825 记录）：** 两轮均在主 checkout（`gsd/phase-02-rbac` 分支工作树）直接编辑与验证——iteration 1 由编排者指示不建 worktree，iteration 2 沿用同一指示；下述门禁数字均来自主 checkout 当前树，可直接复现。

---

## Iteration 2 修复明细（本轮，cbdfa00..e665a21 复审之后）

### WR-06: API Key 停用/删除不撤销已签发存量会话

**Files modified:** `internal/auth/session.go`, `internal/service/api_key.go`, `internal/app/app.go`, `internal/auth/session_test.go`, `internal/handler/api_key_integration_test.go`
**Commit:** a14c918
**Status:** fixed: requires human verification（新增撤销原语与两处写路径接线；集成直证充分，标注供人工低成本复核）
**Applied fix:** 编排者裁决的精确撤销路径（REVIEW.md Fix ② 的即时实现，利用既有 `iam:apikey-token:{key}` 映射，无需等 Phase 3 反向索引）：

1. `SessionManager.KickoutAPIKey(ctx, apiKey)` 新原语：经映射反查 token → 单 MULTI/EXEC 原子块 **DEL 会话 + SREM user-tokens 索引 + DEL 映射**（Logout+UnbindAPIKey 合体形状）。幂等——映射不存在（key 从未签发过会话）返回 nil；映射指向死 token 的历史残留同样收敛（DEL/SREM 均 no-op）。已知边界如实记录于注释：映射仅指向最新 token，映射 TTL 过期后覆盖不到的旧存活 token 至自然 TTL 到期（既有 02-CONTEXT.md 残留语义）。
2. `ApiKeyService` 注入 `SessionManager`（构造签名扩参，app.go 装配点同步）：
   - `UpdateStatus`：前置 `FindByID` 取 apiKey 明文作映射反查键（Delete 同构取值，404 消息不变）→ Patch 落库 → **DISABLED 时 KickoutAPIKey**；ENABLED 不动作（同用户轴语义）。kick 失败 500 但停用已持久（CR-01/IN-06 同款已裁决语义，残留会话由复用分支 WR-02 状态校验兜底拒绝续命）。
   - `Delete`：物理删成功后 `KickoutAPIKey`（CR-01 用户轴 Delete 同型语义）。
3. 注释声明裁决依据：源系统停用/删除 key 不踢 Sa-Token 会话，本修为 D-09 用户轴同构的 **apikey 轴有意安全增强**（偏离源行为）；加重因素一并记录——WR-05 后 kickout 端点仅 super_admin 可调，普通管理员已无补偿性人工踢会话手段，撤销必须内建。
**回归测试:** 单测 `TestKickoutAPIKeyRevokesSessionAndMapping`（三键终态断言 + 幂等 + 死映射残留场景）；集成 `TestApiKeyRevokeKillsExistingSession`——apikey-login 拿 tokenK → 业务端点 200（存活基线）→ **停用 key → tokenK 即刻 401/10100** → 登录封禁 10503 不变 → 重新启用后完整登录换新 token（死映射已清不复用）→ **DELETE key → 新 token 即刻 401**（两轴同构直证）。

### IN-09: MenuUpdateReq 注释漂移（仍称「未知兜底 ENABLED 落库」）

**Files modified:** `internal/model/dto/menu.go`
**Commit:** e9ced25
**Status:** fixed
**Applied fix:** 注释改为 WR-03 后的实际语义——status 经 UsageStatus 严格解析，非空未知值 400/10401（并点明旧注释描述的兜底行为正是 WR-03 要消除的「拼错停用值静默反转回启用」），null/缺失落 nil 指针由更新分支承接（不写库）。

### IN-10: UsageStatus.UnmarshalJSON 消费方清单不完整

**Files modified:** `pkg/types/enum.go`
**Commit:** 3448073
**Status:** fixed
**Applied fix:** 注释消费方清单更正为全部 5 个状态变更写 DTO：UpdateStatusReq/ApiKeyUpdateStatusReq/MenuUpdateReq/DeptUpdateReq/RoleUpdateReq（后三个为 WR-03 附带收紧路径，IN-10 补记），并保留「无读路径反序列化消费方」结论。

### IN-12: sys_dept.sql 头注释列数错误（18→13）

**Files modified:** `scripts/ddl/sys_dept.sql`
**Commit:** d835e96
**Status:** fixed
**Applied fix:** 头注释「列集 18 列」改为「列集 13 列」（正文 CREATE TABLE 实为 13 列，schema 本身忠实于源库，仅注释复制漂移）。

### IN-13: gofmt 漂移残留两文件（初审 IN-01 最终收口）

**Files modified:** `internal/model/po/api_key.go`, `internal/repository/api_key.go`
**Commit:** 3c5e48d
**Status:** fixed
**Applied fix:** `gofmt -w` 两文件（po/api_key.go 字段列对齐 + repository/api_key.go 文件尾多余空行）；此后 `gofmt -l internal/ pkg/` 全干净——初审 IN-01 三文件至此全部收口。

---

## Skipped Issues

### IN-11: ensureNotDescendant 的 maxDepth=100 截断

**File:** `internal/service/menu.go:212-228`, `internal/service/dept.go:176-192`
**Reason:** 编排者裁决不修：visited 集已保证任意存量环数据不死循环，现实树深（<20 层）范围内算法正确性成立；>100 层菜单/部门树属病态数据且暗示既有脏数据，截断风险极低。REVIEW.md 亦仅要求「记录不要求修改」。本轮未触碰两文件，保持原样（未采纳「可选删 maxDepth / 注释明示」两案，避免无触碰意义的 diff）。
**Original issue:** 父链超 100 层时环检测静默失效；visited 集本身已足以防死循环，maxDepth 并非终止性所需。

---

## Iteration 1 修复台账（合并保留，2026-11-01T01:21:37Z 完成）

首轮 fix_scope=critical_warning（CR-01 + WR-01..05），6/6 修复、0 skip，提交 cbdfa00..e665a21；逐项细节见 iteration 1 报告存档（`02-REVIEW-FIX.iter2.md` 为内容相同备份），此处保留累计台账：

| ID | 标题 | Commit | 状态 |
|----|------|--------|------|
| CR-01 | 删除用户不踢存量会话 | cbdfa00 | fixed（含集成直证 TestDeleteUserKicksSessions） |
| WR-01 | 登录状态校验先于密码校验（禁用名单可枚举） | f1f5aa2 | fixed: requires human verification（顺序重排，集成直证充分） |
| WR-02 | ApikeyLogin 复用分支跳过用户状态校验 | b71f317 | fixed: requires human verification（+UnbindAPIKey，集成直证充分） |
| WR-03 | UsageStatus 未知值静默兜底 ENABLED | 6cee1d0 | fixed（附带收紧 menu/dept/role 写路径 → 遗留 IN-09/IN-10，本轮已修） |
| WR-04 | menu/dept Update parentId 自指/成环 | b3cb473 | fixed: requires human verification（环检测算法，集成直证充分） |
| WR-05 | 管理端点零授权（部分范围） | aeea5cb | fixed（部分范围：kickout 收紧落地；全量 requirePerm 裁决 deferred Phase 3 Casbin，计划内 T-2-07 accept） |
| IN-01（顺带） | gofmt 漂移三文件之一 | e665a21 | fixed（仅触碰的 auth_integration_test.go；余两文件 → IN-13，本轮已修） |

首轮 deferred 子项（非 finding 级跳过）：WR-05 全量授权收口（Phase 3）、IN-01 另两文件（本轮 IN-13 收口，台账闭环）。

---

## 人工复核提示

Iteration 2 新增标注一项：**WR-06（a14c918）** —— 撤销原语与写路径接线的语义正确性已有集成直证（停用→旧 token 401、删除→新 token 401、重启换新 token 三链），人工复核成本为通读 a14c918 diff。Iteration 1 的三项（WR-01/WR-02/WR-04）维持原标注不变。

## 裁决冲突记录

无冲突。WR-06 按编排者裁决采用「映射反查精确撤销」路径（REVIEW.md 推荐 Fix ① 的 KickoutUser 粗粒度方案未采用——编排者明确要求 apikey 轴精确撤销 + 注释声明有意安全增强）；IN-11 按裁决不修。

---

_Fixed: 2026-11-01T02:02:10Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 2_
