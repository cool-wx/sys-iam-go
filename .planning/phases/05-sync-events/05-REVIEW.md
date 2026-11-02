---
phase: 05-sync-events
reviewed: 2026-11-02T03:31:47Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - internal/app/app.go
  - internal/event/dispatcher.go
  - internal/event/dispatcher_test.go
  - internal/event/event.go
  - internal/event/listeners.go
  - internal/handler/file_integration_test.go
  - internal/handler/name_sync_integration_test.go
  - internal/handler/olog_integration_test.go
  - internal/service/dept.go
  - internal/service/dict.go
  - internal/service/file_category.go
  - internal/service/menu.go
  - internal/service/name_sync_service_integration_test.go
  - internal/service/role.go
  - internal/service/user.go
  - internal/service/user_integration_test.go
findings:
  critical: 0
  warning: 3
  info: 5
  total: 8
status: issues_found
---

# Phase 5: Code Review Report

**Reviewed:** 2026-11-02T03:31:47Z
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

Phase 5 交付的进程内事件机制（internal/event：同步 dispatcher + 6 handler / 35 条幂等 UPDATE）、5 个服务域发布挂点（dept/role/menu/dict/user）、WR-07 三域移根 NULL 语义与 WR-08 断言真化，核心逻辑经逐条核对后**未发现 BLOCKER 级缺陷**。重点核实过的面：

- **dispatcher 并发正确性**：Register 写锁 / Publish 读锁拍快照后调用，监听器执行不持锁（dispatcher.go:41-52）；首错即返语义有测试锁定。
- **35 条 UPDATE 与 PO 映射逐条核对**：6 类事件的表名（sys_user/sys_dept/sys_role_menu/sys_user_role/sys_role_dept/sys_dict_data/sys_menu/sys_role/sys_operation_log/sys_config/sys_api_key/sys_shift/sys_fault_info/sys_file_category/sys_file）与 `TableName()`、列名与 `column` tag 全部一致；`<>` 单列条件 + 占位符参数化，无 SQL 拼接面。
- **无审计副作用**：po 包无任何 `BeforeUpdate` 钩子、无 `autoUpdateTime`/`UpdatedAt` 字段（grep 实证），同步 UPDATE 恒为单列 SET——不会借 GORM 钩子意外改写 update_time/操作人列，与源 MP wrapper-update 语义一致。
- **NULL 三值逻辑纪律**：35 条恒不带「IS NULL 放行」分支，与 SYNC-02 prohibition 一致，且有 `TestNullNameRowsNotSynced` 控制行直证。
- **发布时序（D-02）**：5 个挂点均为「pre-read（显式带名才回读，缺失提前 404）→ Patch 提交 → 新≠旧才 Publish」，代码顺序满足提交后发布；Publish 原始 gorm 错误经 `resp.Fail` 收敛 500/10001 固定文案，不泄漏内部信息（resp.go:32-44）。
- **WR-07 移根 NULL**：三域 `cols["parent_id"] = nil` 经 GORM Updates(map) 写 NULL，与 `tree.Build` 的 nil/0 双形态根判定（tree.go:45）、`ensureNotDescendant`/`resolveParent` 兼容性核实一致；WR-08 断言已解包至 envelope Data 层，可真实失败。
- **装配面**：`event.RegisterAll(dispatcher, db)` 先于 5 个 service 构造（app.go:164-172），handler 经 `db.WithContext(ctx)` 继承请求取消信号。

以下 3 项 WARNING（均为可证实的健壮性/一致性缺口，非崩溃级）与 5 项 INFO 需要处置。

## Warnings

### WR-01: UserService.Update 的 Patch 路径未译 404 域消息，与本 phase 新增的 pre-read 分支消息契约不一致

**File:** `internal/service/user.go:193-195`

**Issue:** 本 phase 给 `Update` 加了「显式带 realName 才回读旧行、行不存在译 404『用户不存在: {id}』」的 pre-read（user.go:146-155），但既有的 `repo.Patch` 错误路径仍为裸 `return nil, err`。当请求**不带** realName（仅改 phone/email/avatar 等）打在不存在的用户上时，`UserRepo.Patch` 返回 `errcode.ErrNotFound` 单例（user.go:66），经 `resp.Fail` 输出 404/10300 但 message 是通用文案「**数据不存在**」。同一函数两条 404 路径消息不一致；且 dept/menu/role/dict/file_category 五个同侪域的 Patch 路径全部做了域消息翻译（如 dept.go:124-126），user 域是唯一漏网。HTTP 语义仍正确（404），属消息契约漂移而非功能错误。

**Fix:**
```go
if err := s.repo.Patch(ctx, id, cols); err != nil {
    if errors.Is(err, errcode.ErrNotFound) {
        return nil, userNotFound(id)
    }
    return nil, err
}
```

### WR-02: RoleService.Update 中改名 Publish 排在 status→casbin 同步之前，安全步骤被非安全步骤遮蔽

**File:** `internal/service/role.go:166-207`

**Issue:** 当单一请求同时改角色名并停用（status=DISABLED）且改名 `Publish` 失败时，函数在 role.go:173-175 提前返回 500——紧随其后的 `enf.ClearRole`（role.go:187，停用即撤权的授权面收口）**不会执行**。结果：角色已停用落库，但 casbin p/g 行原封不动，持该角色的存量 token 在客户端重试之前持续保有完全授权。D-03 的「重试幂等自愈」依赖客户端真的重试；不重试则停用语义（「下一请求即 403」，role.go:91-93 注释自述）无限期不成立。失败方向不安全：一个冗余名同步的失败不应延迟授权撤销。两个 post-commit 步骤无论先后序均满足重试收敛，故安全步骤先行严格更优。

**Fix:** 将 role.go:161-176 的改名 Publish 块整体移到 role.go:180-207 的 status 同步块之后（Patch 与 Publish 的「提交后发布」契约不受影响——两者都在 Patch 成功之后执行，仅互相换序）。

### WR-03: App.Stop 非幂等——二次调用在 ologW.Flush 处静默阻塞满 5s；TestOlogSurvivesStop 每次运行触发该路径且注释断言相反

**File:** `internal/app/app.go:261-266`（缺陷本体）、`internal/handler/olog_integration_test.go:305-340`（触发点）

**Issue:** `App.Stop` 直接调用 `a.ologW.Flush(flushCtx)`，不经 `OpLogWriter.Stop` 的 `stopOnce` 闸。worker 已退出后（首次 Stop 完成），`Flush` 向缓冲容量 1 的 `flushCh` 发送会**立即成功**（无消费者也不再接收），随后等 `done` 关闭只能等满 5s ctx 超时（op_log.go:134-147）。`TestOlogSurvivesStop` 在测试体内显式 `testApplication.Stop()`，而 `newTestApp` 的 `t.Cleanup`（user_integration_test.go:227-232）会再次 `application.Stop()`——第二次 Stop 在 Flush 处停顿整 5s 后才继续，该测试套件每轮净增 5s。测试注释「双 Flush 属幂等冗余，无需另改」（olog_integration_test.go:309-310）与实际行为相反：`OpLogWriter.Stop` 确有 stopOnce 幂等（op_log.go:152-162），但 App 层的 Flush 调用不在其覆盖内。生产 main 只 Stop 一次故非线上故障，但属健壮性缺口——未来任何二次 Stop 调用点（多信号触发、测试扩展）都会静默吃 5s 停顿。

**Fix:** 二选一：① `App` 内加 `stopOnce sync.Once` 把 `Stop` 体收口（与 `OpLogWriter.Stop` 同款约定）；② `OpLogWriter.Flush` 开头加 `select { case <-w.quit: return nil; default: }` 快速路径——worker 已终局 drain 过，Flush 语义上无事可做。

## Info

### IN-01: UpdateUserReq.RealName 无下界校验，空串真名会把展示名列与审计列批量刷空

**File:** `internal/model/dto/user.go:31`、`internal/event/listeners.go:128-223`

**Issue:** `RealName *string` 无 `binding:"omitempty,min=1"` 类下界，`{"realName":""}` 被接受后：新名≠旧行名即发布 RealNameChanged，26 条同步 UPDATE 把 `sys_user_role.username`（列名 username 实存展示名）以及 12 张表的 create_by_name/update_by_name 全部刷为空串——打破 AssignRoles 侧「realName 非空白优先回退 username」的展示名回落语义（user.go:496-500）。源监听器同样直写新值，若此为源行为保真请在 dto 或 handler 注释标注裁决依据；否则建议补下界。

**Fix:** `RealName *string \`json:"realName" binding:"omitempty,min=1"\`` ，或注释留档「空串真名照源放行」的裁决。

### IN-02: 审查范围内 4 个文件 gofmt 不合规

**File:** `internal/handler/file_integration_test.go:269-275`、`internal/handler/name_sync_integration_test.go:225`、`internal/handler/name_sync_service_integration_test.go:183`（service 侧同名文件）、`internal/handler/olog_integration_test.go:80,201`

**Issue:** `gofmt -l` 实测：file_integration_test.go 的 `fileCategoryTreeNode` struct tag 缩进未对齐（`Children` 行多一空格）；两个 name_sync 测试文件在「Task 2 RED 追加」拼接点缺空行；olog_integration_test.go 两处注释行首 `//（` 应为 `// （`（gofmt 对中文括号注释的规范化要求）。

**Fix:** 对 4 个文件执行 `gofmt -w`。（范围外另有 config.go/fault.go/file.go/file.go handler 等既有未格式化文件，非本 phase 引入，未计入。）

### IN-03: file_integration_test.go 下载头断言在头缺失时以 panic 代替干净失败

**File:** `internal/handler/file_integration_test.go:195-196`

**Issue:** `resp.Header.Get("Content-Disposition")[:10]`——若该头缺失，`Get` 返回空串，切片表达式 panic（slice bounds out of range），测试以崩溃而非断言失败呈现，干扰失败定位。

**Fix:**
```go
cd := resp.Header.Get("Content-Disposition")
require.GreaterOrEqual(t, len(cd), 10, "Content-Disposition 头缺失")
assert.Equal(t, "attachment", cd[:10], "下载必须恒 attachment 头（T-4-05 缓解面）")
```

### IN-04: OnUsernameChanged 为无发布点的注册死代码（Q1 裁决保真，防误判提示）

**File:** `internal/event/listeners.go:100-115`

**Issue:** `UsernameChanged` 事件类型 + handler 已注册但全库无任何 `Publish` 调用点——注释已明文记录这是源死代码语义的契约补齐（Q1 裁决，username 为登录锚点不可改）。属有意保留而非缺陷；列出仅供后续维护者免把「缺发布点」误判为遗漏而补挂触发路径。

**Fix:** 无需改动；如希望机器可查证，可在 handler 注释中补一行「grep 断言：全库无 publish(UsernameChanged)」的验收锚点。

### IN-05: 停用判定以字面量 `!= 1` 散落两处，未用 types 域常量

**File:** `internal/service/role.go:302`、`internal/service/user.go:467`

**Issue:** `*role.Status != 1` / `*st != 1` 以魔法数判停用，同文件其它 status 判定用 `types.UsageDisabled`（role.go:114）。常量语义散落会放大未来枚举值调整时的漏改面。

**Fix:** 统一改为 `*role.Status != int8(types.UsageEnabled)` 或引入 `types.IsEnabled(*st)` 谓词收口。

---

_Reviewed: 2026-11-02T03:31:47Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
