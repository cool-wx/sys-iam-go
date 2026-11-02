---
phase: 05-sync-events
fixed_at: 2026-11-02T06:55:02Z
review_path: .planning/phases/05-sync-events/05-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 0
out_of_scope_info: 5
status: all_fixed
fix_scope: critical_warning
worktree: .claude/worktrees/rf-05-2224515-1793601371
verification_env: isolated worktree（gsd-reviewfix/05-2224515，fast-forward 回主分支前）
---

# Phase 5: Code Review Fix Report

**Fixed at:** 2026-11-02T06:55:02Z
**Source review:** .planning/phases/05-sync-events/05-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 3（CR-01、WR-01、WR-02；fix_scope=critical_warning）
- Fixed: 3
- Skipped: 0
- Out of scope (Info): 5（IN-01..IN-05，未修复，见文末记录）

**Commits（均在隔离 worktree 分支 `gsd-reviewfix/05-2224515` 上，清理尾段 fast-forward 回 `gsd/phase-02-rbac`）:**

| Finding | Commit | Status |
|---------|--------|--------|
| CR-01 | df9a63c | fixed: requires human verification |
| WR-01 | bfbc6fd | fixed |
| WR-02 | 411b574 | fixed: requires human verification |

## Fixed Issues

### CR-01: D-03「客户端重试幂等自愈」契约被发布守卫证伪——Publish 失败后重试永不重发事件，14 表冗余列永久陈旧

**Files modified:** `internal/event/event.go`, `internal/service/user.go`, `internal/service/role.go`, `internal/service/dept.go`, `internal/service/menu.go`, `internal/service/dict.go`, `internal/service/name_sync_service_integration_test.go`, `.planning/phases/05-sync-events/05-CONTEXT.md`
**Commit:** df9a63c
**Status:** fixed: requires human verification（逻辑条件变更——重试验证需 DB 集成环境）

**Applied fix:** 采用评审方案 A（自愈语义最小修复），并按编排指令对齐契约而非弱化契约文档：

1. **5 个发布点全部去除「新名≠旧名」发布短路**（user.go RealName / role.go RoleName / dept.go DeptName / menu.go Name / dict.go DictName），统一改为「请求显式带名且回读成功即发布」。
   - 自愈闭环恢复：主行已提交而 Publish 失败后，客户端重试同一请求时旧行已是新名——原守卫恒假、事件永不重发；现在显式带名即重发，handler 幂等 UPDATE（`冗余列 <> newName`）使同名重发退化为廉价 no-op，26 列同步补执行可达。
   - 「防仅改备注触发同步」的原意图保留：显式带名门（`req.X != nil`）不动，不带名的请求仍零发布——非无条件发布。
2. **契约文档随修复改为真**：event.go 包注释 D-03 段、5 个服务文件的发布点注释与函数文档全部改写为新语义（含与源 `!realName.equals(oldRealName)` 等条件的显式偏离声明）；各处「源条件逐字移植」的失真表述一并修正。
3. **裁决在案**：05-CONTEXT.md D-03 追加修订记录（偏离源按值变化检测语义的裁决理由与边界）。
4. **行为锁定测试同步更新**：name_sync_service_integration_test.go 四个发布条件测试的形态 (c) 从「新名==旧行名 → 零发布」改为「仍发布恰 1 条事件（幂等 no-op 同步）」，形态 (d) 计数随之 +1；测试文档注释注明 CR-01 修订。

### WR-01: 同实体并发改名可乱序应用同步事件——冗余列终值可落后于主行且无自愈

**Files modified:** `internal/event/event.go`
**Commit:** bfbc6fd
**Status:** fixed（评审给定的最小修复形态：如实记录已知限制）

**Applied fix:** 在 event.go 包注释追加「已知限制」段：同实体并发改名的多个同步事件无顺序保证（handler 执行序不受主行提交序约束、UPDATE 无旧名 CAS，后到旧名可覆盖先到的新名，冗余列终值可落后至该实体下次改名，或任一显式带当前名的请求经 CR-01 幂等重发拉齐）；封闭方案路径（旧名 CAS + NULL 双形态 / 按实体 ID 进程内串行化）注明须重新过 SYNC-02 prohibition 评审。**竞态本体未封闭**——按评审「至少如实记录」形态处置，与源 REQUIRES_NEW 形态同险，单机低并发场景接受；如需封闭请在后续 phase 立项。

### WR-02: 空串真名可直达 Update 并触发 26 列清空事件，与 sys_user_role.username 展示名回退语义冲突

**Files modified:** `internal/service/user.go`, `internal/model/dto/user.go`
**Commit:** 411b574
**Status:** fixed: requires human verification（新增行为门——原「合法」请求现在 400）

**Applied fix:** 新增 `validateRealName` helper（`realName != nil && strings.TrimSpace(*realName) == ""` → 400/10400「真名不能为空」），在 Create 与 Update 两入口前置拦截（Update 先于回读/Patch/发布）。TrimSpace 判定与 AssignRoles 展示名回退的空白语义同轴；DTO `UpdateUserReq.RealName` 字段注释标明其例外于「空串=显式清空」总则。Create 侧同步补齐（评审明示：不能依赖 `required` tag 对指针只查非 nil 的语义）。

## Verification

**验证环境：隔离 worktree（`.claude/worktrees/rf-05-2224515-1793601371`）**——门禁数字在 worktree 内产出，fast-forward 后可从主 checkout 复现（内容一致）。

- Tier 1（每项修复后）：重读修改段确认修复文本在场、周边代码完好——通过。
- Tier 2（每项修复后）：
  - `go build ./...` — 通过（每次提交前 + 最终全量 3 次）
  - `go vet ./internal/service/ ./internal/event/ ./internal/model/dto/` — 通过
  - `go vet -tags integration ./internal/service/` — 通过（集成测试编译级验证）
  - `gofmt -l`（8 个被改文件）— 零输出
- **未运行集成测试**：name_sync/user 集成测试需 MySQL 容器（约束：任意时刻 ≤1 个容器），本 agent 不启动容器；行为锁定断言已按新契约更新并编译通过，**请 verifier phase 以容器约束运行**（`go test -tags integration ./internal/service/ -run TestNameChangePublish`）。
- 注：`gofmt -l internal/` 下另有 15 个**未被本次修改**的文件存在既有格式问题（handler/middleware/model 等），非本次引入、未顺手改动。

## Skipped Issues

**本节为 out-of-scope 记录**（fix_scope=critical_warning，IN findings 不修复，逐项如实登记）：

### IN-01: listeners.go 包注释「35 条 UPDATE」计数悬空——删除事件后实为 34 条

**File:** `internal/event/listeners.go:22-23`
**Reason:** out of scope（Info，fix_scope=critical_warning）
**Original issue:** SyncHandlers 包级注释「35 条 UPDATE 逐条落齐」在删除 OnUsernameChanged 后应更新为 34。

### IN-02: event.go 包注释「6 个同步 handler」计数悬空——实为 5 个

**File:** `internal/event/event.go:2`
**Reason:** out of scope（Info）；注：CR-01/WR-01 重写该注释块时刻意保留原句未动，未顺手修复。

### IN-03: app.go 装配注释「全部 6 个同步 handler 注册」计数悬空（文件在三文件清单之外，跨模块核查发现）

**File:** `internal/app/app.go:159-160`
**Reason:** out of scope（Info）

### IN-04: user.go 字段注释「字段暂存不用」已失真——pub 挂点已实装

**File:** `internal/service/user.go:40-43`
**Reason:** out of scope（Info）；注：CR-01 触碰同文件其余段落，该注释块保持原样未动。

### IN-05: Update 的 Patch 缺行路径未译域消息——404 文案「数据不存在」与其余路径「用户不存在: {id}」不一致（上轮 WR-01 遗留，仍在）

**File:** `internal/service/user.go:193-195`
**Reason:** out of scope（Info，上轮 WR-01 遗留降记）

---

_Fixed: 2026-11-02T06:55:02Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
