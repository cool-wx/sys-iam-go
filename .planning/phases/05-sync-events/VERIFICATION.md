# Phase 5 验收证据链（VERIFICATION）

**Generated:** 2026-11-02（05-03 Task 2 落盘）
**Scope:** v1 全量验收（ROADMAP 成功标准 3）——8 项 "Looks Done But Isn't" 清单逐项映射 + Phase 5 四项裁决/修复记录 + 全量回归证据。
**纪律:** 每一项必须有具体测试用例名（文件在案）或可复核的组合证据链——不接受"应该没问题"（D-05 明文）。

---

## 一、8 项 "Looks Done But Isn't" 清单逐项映射

| # | 清单项 | 测试用例 + 文件 | 状态 |
|---|--------|----------------|------|
| 1 | **存量 hash 登录** | `TestAuthFlow` / `TestAuthLoginFailures`（internal/handler/auth_integration_test.go；播种哈希 `testAdminHash = "$2a$10$7JB7..."` :25——存量 BCrypt `$2a$` 零迁移活体直证） | ✅ 测试直证 |
| 2 | **API Key 双路径** | `TestApiKeyLoginFlow`（密码+apikey 双路径，api_key_integration_test.go）+ `TestBindAPIKeyDualKeys` / `TestConcurrentApiKeyFullLogin`（session 面直证） | ✅ 测试直证 |
| 3 | **内置删除拦截** | `TestDeleteBuiltinUser409`（user 域）+ `TestRoleBuiltinDeleteConflict` + `TestRoleBuiltinDisableConflict`（role 域 WR-06 面）——builtin 生命周期不变量 409/10602/10604 | ✅ 测试直证 |
| 4 | **编辑不传字段不清空** | `TestUpdateUserZeroValueProtection` + `TestUpdateUserExplicitClear`（ENG-05 零值保护 user 域直证；其余域同 `repo.Patch` map 白名单模板，nil=不写库） | ✅ 测试直证 |
| 5 | **分页边界不 500** | `TestListUsersEdgeParams`（page/size 越界/非法值）+ `TestOpLogListPagination`（olog 1/10 特例缺省、翻页倒序不漂移） | ✅ 测试直证 |
| 6 | **枚举未知值兜底** | menu `type=99` 兜底 MENU（menu_dept_integration_test.go:127-129）、gender `=99` 兜底 UNKNOWN（user_integration_test.go:668-669）+ `pkg/types` enum_test.go 全档 | ✅ 测试直证 |
| 7 | **失败请求有日志** | `TestOpLogFailedRequestRecorded`（olog_integration_test.go:174——status=0 + error_msg 非空截 500 rune） | ✅ 测试直证 |
| 8 | **SIGTERM 后日志完整落库** | `TestOlogSurvivesStop`（olog_integration_test.go，**05-03 新增**）——入队 → **不手动 Flush** → 真实 `App.Stop()` → checkDB 断言落库行要素完整（module/operation/method/user_id/real_name/status）。原组合证据（TestOpLogWriteRecorded 的 Flush 同步断言 + TestGracefulShutdownDrains 的真实 Stop）升级为一体化直证 | ✅ 一体化直证（本计划补齐） |

## 二、Phase 5 裁决与修复记录

### Q1：UsernameChangedEvent 无触发路径系源语义（非遗漏）

- 源 `editUser` 签名不含 username（登录锚点创建后不可改），`UsernameChangedEvent` 全源码树仅事件类定义与监听器方法两处、**无任何 publish 调用点**——源内死代码。
- Go 侧一致：`UserService.Update` 白名单刻意不含 username/password（internal/service/user.go 注释在案）；事件类型 + `OnUsernameChanged` handler 照源补齐契约形态（05-01），user.Update 不设发布点。
- 结论：SYNC-01 字面"6 事件"中 username 事件的"无触发路径"是**源语义的原样移植**，非遗漏。

### Q2：sys_role_dept.role_name 纳入 RoleName 同步面为 Go 增量扩展

- sys_role_dept 是 Go 增量表（D-10，源不存在）；其 role_name 冗余由 AssignScopes 写入。源 RoleNameSync 只同步 sys_role_menu + sys_user_role 两表。
- 裁决：纳入同步面（`OnRoleNameChanged` 第三条 UPDATE，同 `<>` 幂等条件，注释「Go 扩展非源行为」）——Go 内一致性强于字面对齐。`TestRoleNameSync` 三表断言直证。

### WR-07/WR-08 修复记录（05-03 Task 1，commit 23303e5）

- **WR-07（三域移根 parent_id 落 NULL 唯一形态）**：file_category/dept/menu 三域 Update 移根分支 `cols["parent_id"]` 由数值 0 改为 nil（GORM Updates(map) 显式 nil 写 NULL，与同分支 parent_name 清 NULL 同款）——根 = parent_id NULL 成为唯一形态，与 Create 根同形态；响应 parentId 恒 null（DTO `if m.ParentID != nil` 零改动自然输出）。对源自身 Create null / Update 0 不对称的 **Go 内一致性修正（显式偏离源）**。存量 parent_id=0 行**不迁移**：`tree.Build`（nil/0 均提升为根）、`ensureNotDescendant`、`resolveParent` 均双形态兼容（dept.go/menu.go/file_category.go 同构），读侧零回归。
- **WR-08（file 移根断言真化）**：internal/handler/file_integration_test.go 移根断言补 `{code,message,data}` envelope Data 层解包（原断言漏 Data 层永不失败）——`assert.Nil(env.Data.ParentID)` 成为真断言，由 WR-07 修复保证通过。两坑同任务落地（Pitfall 4：只修测试不修响应则断言立刻红）。
- 回归证据：`TestFileCategoryTree` / `TestDept*` / `TestMenu*` 全绿（见下方回归门）。

### IN-04 disposition：Stop 内 Flush 已由 TestOlogSurvivesStop 直证，无需另改

- `App.Stop` 对 oplog 连续 Flush 两次（显式 `ologW.Flush` + `ologW.Stop` 内部 `stopOnce` Flush）——`OpLogWriter.Stop` 经 `stopOnce` 幂等（middleware/op_log.go:152 注释明示），双 Flush 属幂等冗余。
- `TestOlogSurvivesStop` 直证"入队→真实 Stop→完整落库"不变量成立；Flush 失败路径已有 Warn 语义（不反压）。**disposition：行为正确，无需另改。**

### D-03 失败语义证据形态 与 D-04 会话快照不刷新说明

- **D-03（同步失败 500 + 重试幂等自愈）**：发布点在 Patch 返回 nil 之后，`Publish` 失败 `return nil, err` → API 500（五域 Update 同构，dept.go/menu.go/role.go/dict.go/user.go 注释在案）；自愈面由 `TestIdempotentSelfHeal` 直证——陈旧行经 `<>` 条件拉齐、已同步行重放不中（幂等 UPDATE 的可重试性）。handler 独立事务（`event.RegisterAll` 持 db，D-02 时序）物理隔离发布者事务。
- **D-04（改名后会话快照照源不刷新）**：Redis 会话 `SessionPayload{UserID,UserName,RealName,Roles}` 不因改名事件刷新（internal/auth 本 phase 零改动）；displayName 短暂陈旧与源系统一致，下次登录生效。改名非安全事件，Phase 2 D-09"停用即踢"不延伸——显式决策非遗漏（negative space，无测试断言面，代码位置即证据）。

## 三、全量回归证据（D-05：既有全部自动化测试全绿即回归证据）

**执行窗口：** 2026-11-02T02:23:47Z — 2026-11-02T03:06:07Z（含分批串行开销）
**资源约束纪律（编排器硬性约束，1.6GB 宿主机）：** `go test -p 1` 包间串行 + 分批 `-run` 正则（14 批）+ 批间容器清理核查；包内零 `t.Parallel()`（grep 证实）→ 任意时刻 ≤1 测试 MySQL 容器；每批结束 `docker ps -a --filter ancestor=mysql:8.4.11` 核查零残留（开发库 `mysql`/`redis` 容器全程未触碰）。

| 层 | 命令形态 | 结果 |
|----|----------|------|
| handler 集成（12 批） | `TZ=UTC go test -tags=integration -p 1 -v -run '^...$' -count=1 ./internal/handler/` | **85 通过 + 1 SKIP**（H1..H12 全绿，零 FAIL） |
| service 集成 | 同形态 `./internal/service/` | 5 通过（TestUpdateAuditWithIdentity + 4 发布条件单测） |
| app 集成 | 同形态 `./internal/app/` | 2 通过（TestReadinessFlipBeforeDrain / TestGracefulShutdownDrains） |
| 单元层 | `go test -p 1 -count=1 ./internal/... ./pkg/...` | 15 包全 ok（auth/authz/config/datascope/event/handler/middleware/service/storage/tree + pkg 5 包） |
| 静态 | `go build ./... && go vet ./...` | 通过 |
| 出厂门 | `make build && make verify-static` | OK: statically linked（CGO_ENABLED=0 单二进制不受影响） |

**SKIP 说明（既有状态，非本计划引入）：** `TestFaultImportYf175aData`（Phase 4 D-14 增强冒烟）因真实数据文件 `/home/wx/workspace/tims/...` 在本开发机不可得按设计 `t.Skip`，fallback 构造数据回测 `TestFaultImportUpsert` 已覆盖同一导入链并 PASS。

**回归结论：** Phase 1-5 全部自动化测试全绿（唯一 SKIP 为文档化的环境条件跳过），14 域接口与自动化测试通过——**项目 Core Value 验收态达成，ROADMAP 成功标准 3 关闭**。

## 四、Scope 边界确认（prohibition 复核）

- 未借收口扩面：无改名即踢线/刷会话快照（D-04 排除）、无 boot 对账/待同步清单（D-03 排除）、internal/auth 零改动——本计划 diff 仅涉 3 个 service 移根分支 + 2 个测试文件 + 本文档。
- Deferred Ideas 边界即验收边界（05-CONTEXT deferred 节逐项显式排除，非 scope creep）。

---

*Phase: 05-sync-events · 05-03 · 2026-11-02*
