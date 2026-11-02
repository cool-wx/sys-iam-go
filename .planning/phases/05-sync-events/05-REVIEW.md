---
phase: 05-sync-events
reviewed: 2026-11-02T06:32:38Z
depth: standard
files_reviewed: 3
files_reviewed_list:
  - internal/event/event.go
  - internal/event/listeners.go
  - internal/service/user.go
findings:
  critical: 1
  warning: 2
  info: 5
  total: 8
status: issues_found
---

# Phase 5 (incremental): Code Review Report

**Reviewed:** 2026-11-02T06:32:38Z
**Depth:** standard
**Files Reviewed:** 3（`git diff ee7a95e2..HEAD` 增量 = 16658dc "feat(05): remove UsernameChanged event per UAT Q1 verdict"；对三文件全量现状复审）
**Status:** issues_found

## Summary

本报告**替换**同路径上一轮全量评审（0 Critical / 3 Warning / 5 Info，处置见 05-REVIEW-DISPOSITION.md），覆盖自 ee7a95e2 以来的增量：`UsernameChanged` 事件类型、`OnUsernameChanged` handler 及其注册三处删除，另有多处注释改写。结论先述：

**删除本体干净。** 编译期全绿（`go build ./...`、`go vet ./internal/...` 两包、`gofmt -l` 三文件均零输出）；全仓 `*.go` 无残留的 `event.UsernameChanged` / `OnUsernameChanged` 代码引用（余下 8 处均为注释中对源系统 Q1 裁决的说明性引用，属有意保留）；注册对称性完好：5 常量（event.go:17-23）↔ 5 次 Register（listeners.go:36-43）↔ 5 个 handler ↔ 5 个发布域，无悬空；dispatcher（dispatcher.go）与本轮无关且实现正确（RWMutex 写锁注册 / 快照后调用 / 首错即返，测试锁定）。

**但增量复审发现 1 项 CRITICAL：D-03「客户端重试幂等自愈」这一本 phase 核心韧性契约在全部 5 个发布点被 `新名≠旧名` 发布前置条件证伪**——主行提交成功而 Publish 失败后，重试同一请求时旧行已是新名，守卫恒假、事件永不重发，26 条冗余同步永不补执行。该缺陷非本 delta 引入（增量仅改注释），但属三文件当前状态的既存错误行为，上轮评审漏检，本轮按当前态如实报告。另有 2 项 WARNING（并发改名乱序、空串真名直达 26 列清空）与 5 项 INFO（其中 3 项为**本次删除遗漏的计数悬空引用**——正是增量评审的靶点）。

核实过且无问题的面：listener 34 条 UPDATE 的表名/列名与 PO `TableName()`/`column` tag 逐条一致（含 sys_operation_log 刻意不嵌 BaseWithOperator、user_id/real_name 独立业务列的注释声明与 po/operation_log.go:14-25 实证一致）；全部条件占位符参数化、列名为代码常量，无 SQL 注入面；无硬编码密钥；NULL 三值逻辑「不放行空值列」纪律保持；「14 张表 26 条 UPDATE」的 handler 头注释计数准确；AssignRoles 展示名回退与 realName 同步事件的非空路径语义自洽（真名由空变非空时新值即回退后值）。

## Critical Issues

### CR-01: D-03「客户端重试幂等自愈」契约被发布守卫证伪——Publish 失败后重试永不重发事件，14 表冗余列永久陈旧

**File:** `internal/service/user.go:204-219`（契约声明 `internal/event/event.go:4-6`、注释 `internal/service/user.go:196-203`）

**Issue:** 设计契约（event.go:4-6）与发布点注释（user.go:202-203）均声明：「任一 handler 失败 Publish 返回错误、API 500，**客户端重试触发 `<> newName` 幂等 UPDATE 自愈**」。逐时序推演可证该自愈路径不可达：

1. 请求 `realName: "B"`（当前 "A"）：`Patch` 先提交主行 `real_name="B"`（user.go:193）；
2. `Publish` → `OnUserRealNameChanged` 26 条 UPDATE 单事务执行**失败**（瞬态：死锁/连接中断/ctx 取消），事务回滚，26 处冗余列仍为 "A"；
3. API 500，客户端按契约**重试同一请求**（`realName: "B"`）；
4. user.go:148 回读旧行——`old.RealName` 已是 **"B"**（第 1 步已提交）；
5. user.go:209 守卫 `*req.RealName != oldName` → `"B" != "B"` → **假 → 事件不发布**；
6. 26 条同步 UPDATE 永不补执行。唯一恢复路径是用户将来**再次改名**（届时新事件顺带拉齐），否则 14 张表 26 处冗余列（审计操作人名 + 展示名）**静默永久陈旧**。

该守卫模式在全部 5 个发布点同构存在（role.go:166、dept.go:136、menu.go:170、dict.go:110），即 D-03 契约对 5 类事件全部不成立——与同被引为纪律先例的 casbin 双轨（user.go:527 `SyncUserRoles` 重试时**无条件**重执行，自愈可达）形成直接反差：真名同步路径恰恰多了 `新≠旧` 守卫，把重试自愈的门关死了。上轮评审「发布时序（D-02）」核查仅验证了提交后发布的顺序正确性，未推演失败重试闭环，漏检。

**Fix:** 需设计裁决（改发布条件偏离源语义，须在案）：

```go
// 方案 A（自愈语义最小修复）：主行提交成功且请求显式带名即发布，
// 幂等 UPDATE（AND 冗余列 <> newName）使重发退化为廉价 no-op，
// Publish 失败后重试必重发事件——偏离源 `newName != oldName` 条件，需 Q 裁决在案：
if req.RealName != nil {
    evt := event.NameChangedEvent{Type: event.RealNameChanged, ID: id, OldName: oldName, NewName: *req.RealName}
    if err := s.pub.Publish(ctx, evt); err != nil {
        return nil, err
    }
}
```

方案 B：持久化同步游标（如各行/用户级 last-synced-name 列），发布条件改为「主行名 ≠ 已同步名」，精确恢复重试自愈且保持源首发布语义；方案 C（最低限度）：修正 event.go:4-6 与 5 个服务域注释，如实声明「重试不可自愈、需再次改名或补偿接口」，移除虚假契约。三方案均须同步处置其余 4 个发布点。

## Warnings

### WR-01: 同实体并发改名可乱序应用同步事件——冗余列终值可落后于主行且无自愈

**File:** `internal/service/user.go:146-155,209` + `internal/event/listeners.go:16-23,116-211`

**Issue:** 两个并发请求对同一用户改名 A→B、B→C：主行提交有序（B 先 C 后），但各自 handler 于发布者调用栈内同步执行，执行序不受提交序约束（如 B-sync 因锁等待被 C-sync 超越）——handler UPDATE 仅以 `冗余列 <> newName` 为条件（listeners.go:16-20 形态锁定），无旧名 CAS（`AND 冗余列 = evt.OldName`）或版本单调性守卫，后到的新名 B 会覆盖已写入的更新名 C。终态：主行 C、26 处冗余列 B；且 CR-01 守卫使重发不可能，陈旧持续到下次改名。设计文档「发布时序契约（D-02）」仅覆盖单请求时序，未声明并发乱序语义。

**Fix:** 至少在 event.go 包注释补充并发乱序已知限制（与源 REQUIRES_NEW 形态同险，如实记录）；若需封闭，将 handler 条件升级为旧名 CAS 并对 NULL 旧名列补 `OR (冗余列 IS NULL AND ? = '')` 双形态（须重新过 SYNC-02 prohibition 评审），或对改名请求按实体 ID 加进程内串行化。

### WR-02: 空串真名可直达 Update 并触发 26 列清空事件，与 sys_user_role.username 展示名回退语义冲突

**File:** `internal/service/user.go:157-158`（DTO 根因 `internal/model/dto/user.go:31`；冲突面 `internal/event/listeners.go:128`）

**Issue:** `UpdateUserReq.RealName` 无任何 binding 校验（dto/user.go:31 仅 `json:"realName"`）；Create 侧 `binding:"required"` 经实证（validator v10.30.1 baked_in.go:1816-1820 `hasValue` 对 Ptr 仅查非 nil）**对非 nil 空串同样放行**。故 `PUT /users/:id {"realName":""}` 合法到达：主行 real_name 置空 → `"" != "A"` 发布 NewName="" → listeners.go:128 把 sys_user_role.username 清为空串、24 条审计操作人名列与 operation_log.real_name 全数清空。同列在 AssignRoles（user.go:497-500）的写入语义是「realName 非空优先、回退 username」——真名被清空后，存量关联行 username="" 而重新分配角色行写登录名，同列同状态两种语义并存，审计/展示数据被一次合法请求静默抹除。

**Fix:** Update 入口（或 DTO tag）拒绝空真名：`if req.RealName != nil && strings.TrimSpace(*req.RealName) == "" { return nil, errcode.New(errcode.CodeInvalidParam, "真名不能为空") }`；Create 同步补服务端空串校验（不能依赖 `required` tag 对指针的语义）。若产品语义允许空真名，则发布前按 AssignRoles 同款回退计算 NewName，两路径取齐。

## Info

### IN-01: listeners.go 包注释「35 条 UPDATE」计数悬空——删除事件后实为 34 条

**File:** `internal/event/listeners.go:22-23`

**Issue:** SyncHandlers 包级注释声称「35 条 UPDATE 逐条落齐」。删除 `OnUsernameChanged`（1 条）后当前实数：Dept 2 + DictType 1 + Menu 2 + Role 3 + RealName 26 = **34 条**。删除提交更新了「6 个→5 个」多处计数（event.go:14、listeners.go:33），独漏此处（删除前 35 恰含被删 handler 的 1 条，`git show ee7a95e2` 实证）。审计计数注释与代码不符，正是增量评审靶点的悬空引用。

**Fix:** 改为「34 条 UPDATE 逐条落齐」。

### IN-02: event.go 包注释「6 个同步 handler」计数悬空——实为 5 个

**File:** `internal/event/event.go:2`

**Issue:** 包文档仍写「+ 6 个同步 handler（冗余名称列确定性 UPDATE）」。同文件第 14 行已改「5 个名称变更事件类型常量」，包文档首段漏改——同一提交在同文件内改一半，计数自相矛盾。

**Fix:** 改为「+ 5 个同步 handler」。

### IN-03: app.go 装配注释「全部 6 个同步 handler 注册」计数悬空（文件在三文件清单之外，跨模块核查发现）

**File:** `internal/app/app.go:159-160`

**Issue:** 装配注释「dispatcher 构造 + 全部 6 个同步 handler 注册」——`RegisterAll` 现注册 5 个（listeners.go:36-43）。该文件不在本轮 files 清单内，系悬空引用专项跨模块 grep 命中，与 IN-01/IN-02 同根同批，一并列出供一并清理。

**Fix:** 改为「全部 5 个同步 handler 注册」。

### IN-04: user.go 字段注释「字段暂存不用」已失真——pub 挂点已实装

**File:** `internal/service/user.go:40-43`

**Issue:** `pub` 字段注释仍写「UserRealNameChanged 发布挂点在 05-02 实装——字段暂存不用」，前瞻式计划语态；实际 user.go:216 已使用 `s.pub.Publish`（grep 实证全文件唯一使用点）。删除提交恰好改写了该注释块内 UsernameChanged 句（增量 diff 在案）却未顺手修正同块的过时声明，注释与代码现状矛盾。

**Fix:** 改写为「pub 改真名事件发布依赖（Phase 5 SYNC-01，挂点见 Update）」，删除「暂存不用」句。

### IN-05: Update 的 Patch 缺行路径未译域消息——404 文案「数据不存在」与其余路径「用户不存在: {id}」不一致（上轮 WR-01 遗留，仍在）

**File:** `internal/service/user.go:193-195`（对照 `internal/service/user.go:555-560`）

**Issue:** 上轮全量评审 WR-01 已记录、处置后仍存现状：`Update` 对 `repo.Patch` 的 `errcode.ErrNotFound`（repository/user.go:66）裸返回，经 resp.Fail 落 404/「数据不存在」通用文案；同文件 `UpdateStatus`（555-560）及 Update 自身 pre-read 分支（151）均译域为「用户不存在: {id}」。仅当请求**不带** realName 且行在绑定与 Patch 之间被并发删除时到达（窗口极窄），HTTP 语义正确，纯消息契约漂移，故本轮降记 Info 并保留处置追溯。

**Fix:** `if errors.Is(err, errcode.ErrNotFound) { return nil, userNotFound(id) }`，与 UpdateStatus 对齐。

---

_Reviewed: 2026-11-02T06:32:38Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard（增量，diff_base ee7a95e2）_
