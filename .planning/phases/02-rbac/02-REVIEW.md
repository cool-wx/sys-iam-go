---
phase: 02-rbac
reviewed: 2026-11-01T00:00:00Z
depth: standard
files_reviewed: 64
files_reviewed_list:
  - internal/auth/session.go
  - internal/middleware/auth.go
  - internal/service/auth.go
  - internal/handler/auth.go
  - internal/model/dto/auth.go
  - internal/model/po/role.go
  - internal/model/po/relation.go
  - internal/model/po/api_key.go
  - internal/repository/relation.go
  - internal/repository/api_key.go
  - internal/testutil/redis.go
  - internal/auth/session_test.go
  - internal/middleware/auth_test.go
  - internal/handler/auth_integration_test.go
  - scripts/ddl/sys_role.sql
  - scripts/ddl/sys_menu.sql
  - scripts/ddl/sys_dept.sql
  - scripts/ddl/sys_api_key.sql
  - scripts/ddl/sys_user_role.sql
  - scripts/ddl/sys_role_menu.sql
  - internal/app/app.go
  - internal/app/router.go
  - internal/config/config.go
  - internal/config/config_test.go
  - configs/config.yaml
  - internal/repository/user.go
  - internal/service/user.go
  - internal/testutil/mysql.go
  - pkg/errcode/errcode.go
  - internal/handler/user_integration_test.go
  - internal/app/shutdown_integration_test.go
  - go.mod
  - go.sum
  - internal/tree/tree.go
  - internal/tree/tree_test.go
  - pkg/types/menu_type.go
  - pkg/types/menu_type_test.go
  - internal/model/po/menu.go
  - internal/model/po/dept.go
  - internal/model/dto/menu.go
  - internal/model/dto/dept.go
  - internal/model/dto/role.go
  - internal/model/dto/api_key.go
  - internal/model/dto/user.go
  - internal/repository/menu.go
  - internal/repository/dept.go
  - internal/repository/role.go
  - internal/service/menu.go
  - internal/service/dept.go
  - internal/service/role.go
  - internal/service/api_key.go
  - internal/service/user_integration_test.go
  - internal/handler/menu.go
  - internal/handler/dept.go
  - internal/handler/role.go
  - internal/handler/api_key.go
  - internal/handler/user.go
  - internal/handler/menu_dept_integration_test.go
  - internal/handler/role_integration_test.go
  - internal/handler/api_key_integration_test.go
  - internal/handler/user_extension_integration_test.go
  - pkg/types/datetime.go
  - pkg/types/datetime_test.go
  - pkg/errcode/errcode_test.go
findings:
  critical: 1
  warning: 5
  info: 8
  total: 14
status: issues_found
---

# Phase 2: Code Review Report

**Reviewed:** 2026-11-01
**Depth:** standard
**Files Reviewed:** 64（另有 9 个 Phase 1 依赖文件交叉验证：identity.go / resp.go / enum.go / paging.go / id.go / body_limit.go / po/base.go / po/user.go / cmd/server/main.go）
**Status:** issues_found

## Summary

审查范围：Phase 2 全量变更——自建 token-session（Redis）、认证中间件、五域 CRUD（User/Role/Menu/Dept/ApiKey）、泛型树构建器及其全部集成/单元测试与 DDL。

**验证结果：** `go build ./...`、`go vet ./...` 全绿；非 integration 单元测试全过（auth/middleware/config/tree/errcode/ids/paging/resp/types）。known-good 的 `Patch` 模板（CR-01 存在性回读 + 审计列显式并入 + 恒带 Where）在 user/menu/dept/role/api_key 五个 repo 复制一致，无 Pitfall 3（`Save` 零值覆写）回归；会话安全基线扎实（crypto/rand 256-bit token、Redis 故障 fail-closed 500、登录 MULTI/EXEC 原子块、dummy-hash 恒时防枚举、畸形 hash 译 401 非 500）。

**核心问题：** 1 个 Critical——`UserService.Delete` 删除用户后不踢存量会话，已删除账号的旧 token 在整个会话 TTL（默认 720h）内继续完全可用，与本 Phase 自己在 `UpdateStatus`/`ResetPassword` 落实的 D-09「停用即踢」安全增强直接矛盾。5 个 Warning 中 3 个（登录状态先验枚举、apikey 复用跳过状态校验、UsageStatus 未知值静默兜底 ENABLED）同属「停用语义可被绕过或枚举」一族，建议一并处置；其余 2 个（树自引用环、管理端点零权限校验）分别为未处理边界与 Phase 3 计划内已知项。

**正面确认（非发现）：** apikeyLogin 双 miss 并发竞态有显式决策与测试直证（`TestConcurrentApiKeyFullLogin`）；`KickoutUser` 与登录的快照竞态属分布式固有语义且影响窗口极小，不列为缺陷；`tree.Build` 环引用不死循环有测试直证；`ids.ID`/`LocalDateTime`/`MenuType`/`UsageStatus` 出网契约与 null 键保留契约在集成测试中逐项锁定。

## Critical Issues

### CR-01: 删除用户不踢存量会话——已删除账号旧 token 在整个 TTL 内继续完全可用

**File:** `internal/service/user.go:361-380`
**Issue:** `UserService.Delete` 在 builtin 守卫 + 事务（删 sys_user_role → 删 sys_user）结束后直接返回，**没有调用 `s.sm.KickoutUser(ctx, id)`**。认证中间件（`internal/middleware/auth.go:38-62`）仅校验 Redis 会话存在性，不回查 DB 用户存在性——因此被删除用户的全部存量 token 在会话自然过期前（默认 `session_timeout=720h`）仍可通过认证，继续调用一切管理端点（创建/删除用户、重置 admin 密码、读取全部明文 API Key 等）。攻击场景：管理员删除一个被入侵的账号，攻击者持原 token 无缝继续操作，删除操作形同未遂。这与本 Phase 自己为 `UpdateStatus`（DISABLED 即踢，`internal/service/user.go:327-329`）和 `ResetPassword`（`internal/service/user.go:355`）落实的 D-09 安全论断（「停用用户持旧 token 继续操作是安全洞」）直接矛盾——删除是比停用更彻底的撤销，反而是唯一不踢的路径。
**Fix:**
```go
// internal/service/user.go Delete 末尾，事务成功后踢存量会话
if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    relationTx := repository.NewRelationRepo(tx)
    if err := relationTx.DeleteUserRolesByUserID(ctx, id); err != nil {
        return err
    }
    userTx := repository.NewUserRepo(tx)
    return userTx.Delete(ctx, id)
}); err != nil {
    return err
}
// D-09 对齐：用户已删除，存量会话必须即刻失效（同 UpdateStatus/ResetPassword 语义）
return s.sm.KickoutUser(ctx, id)
```
配套补集成测试：删除在线用户后其旧 token 访问必须 401。

## Warnings

### WR-01: 登录状态校验先于密码校验——禁用账号可被枚举，削弱 T-2-01 防枚举目标

**File:** `internal/service/auth.go:56-64`
**Issue:** `Login` 六步链中 ②状态校验在 ③bcrypt 校验之前执行。对禁用用户提交**任意密码**（无需正确）即得 10502「用户已被禁用」，而对不存在用户/启用用户错密码得 10501——攻击者可凭响应码区分「不存在 / 启用存在 / 禁用存在」三类用户名，禁用账号名单可被完整枚举。dummy-hash 恒时防枚举（T-2-01）只覆盖了 10501 两路径，对 10502 分支无效；且该分支不做任何 bcrypt 比较，耗时还显著更短，形成第二重时间侧信道。注释声明「顺序与消息照源 AuthApplicationService.java:52-60」属源顺序保真决策，但与 T-2-01 的声明目标冲突，需显式接受或修复。
**Fix:** 将状态校验移到 bcrypt 成功之后（密码错误仍统一 10501，验证通过后才暴露 10502）：
```go
if err := bcrypt.CompareHashAndPassword([]byte(derefStr(u.Password)), []byte(derefStr(req.Password))); err != nil {
    return nil, errcode.ErrBadCredentials
}
if u.Status == nil || *u.Status != 1 {
    return nil, errcode.ErrUserDisabled
}
return s.completeLogin(ctx, u)
```
若坚持源顺序保真，请在 plan/验证文档中明示该残余枚举面并同步修订 T-2-01 结论。

### WR-02: ApikeyLogin 复用分支跳过用户状态校验——禁用用户可经旧映射继续取新有效 token

**File:** `internal/service/auth.go:94-98`
**Issue:** 复用分支（映射命中且会话存活）回查了用户存在性却**不校验 `u.Status`**，注释以「源语义：跳过重查重装配」辩护。常规停用路径靠 `UpdateStatus` 的 kick 使会话死亡、复用双检失败来兜底，但两处漏洞场景可绕过：①绕过 API 的库内直改/外部禁用；②`UpdateStatus` 中 Patch 已提交而 `KickoutUser` 因 Redis 故障失败（返回 500 但禁用已持久，见 IN-06）。此时禁用用户每次 apikey-login 都能经复用分支拿到「会话仍存活」的确认，等于无限续用一个禁用身份的会话（TTL 720h）。已校验存在性却不校验状态，逻辑上自相矛盾——既然查了库，补一个状态判断零成本。
**Fix:**
```go
if reused, ok := s.sm.ResolveAPIKey(ctx, derefStr(req.ApiKey)); ok && key.UserID != nil {
    if u, err := s.userRepo.FindByID(ctx, *key.UserID); err == nil && u != nil &&
        u.Status != nil && *u.Status == 1 { // 禁用用户不得经复用分支续命，顺延完整路径返回 10506
        return &dto.LoginResp{Token: reused, User: dto.ToUserResp(u)}, nil
    }
}
```

### WR-03: UsageStatus 未知值静默兜底 ENABLED——状态变更端点上拼写错误会把「停用」反转成「启用」

**File:** `pkg/types/enum.go:25-34`（消费点 `internal/handler/user.go:159`、`internal/handler/api_key.go:76`）
**Issue:** `UsageStatus.UnmarshalJSON` 对一切未知值（含大小写不符，如 `"disable"`/`"DISABLE "`/`"DISBLED"`）静默兜底为 `UsageEnabled` 且不报错。该兜底用于**读路径/展示兜底**（源 fromValue 语义）合理，但同样作用于**状态变更端点**：管理员 `PUT /api/v1/users/:id/status {"status":"DISABLE"}`（少一个 D）实际执行的是**启用**且返回 200——被停用用户被静默恢复、旧会话不被踢（ENABLED 分支跳过 kick），管理员却以为停用已生效。`PUT /api/v1/api-keys/:id/status` 同型。安全敏感的写端点不应继承读端点的宽容兜底；API 契约本就允许重新设计（CLAUDE.md），无源契约约束。
**Fix:** 状态变更 DTO 用严格解析变体（未知值返回 error → 10401/10400），仅保留读路径（Resp 反序列化）的兜底；或最低限度让 `UnmarshalJSON` 对非空未知字符串值报错、仅 `null`/缺失走兜底。

### WR-04: menu/dept Update 允许 parentId 指向自身或子孙——形成环后节点从 /tree 静默消失

**File:** `internal/service/menu.go:81-97`、`internal/service/dept.go:61-76`（环丢弃语义见 `internal/tree/tree.go:44-55,70-73`）
**Issue:** `Update` 对 `parentId` 仅做 `resolveParent` 存在性 + name 非空校验，不检查 `pid == id` 或 pid 是否为当前节点的子孙。`PUT /api/v1/menus/:id {"parentId": <自身id>}` 通过全部校验落库为自引用；指向子孙同理成环。`tree.Build` 对环的处理是「双方均命中索引 → 均不归根、静默不产出」——即整条子树从 `/api/v1/menus/tree` 与 `/api/v1/depts/tree` 消失（`/list` 仍可见），无任何错误返回，属静默数据不可见 + 持久脏数据。删除带子节点是**文档化的源缺口保留**，但更新路径的注释完全未提及环/self 引用，属未处理边界而非有意保留。
**Fix:** Update 的 parentId 分支加两道拦截：①`pid == id` 直接 400「父菜单不能是自己」；②沿 parent 链向上查库（带 visited 集/深度上限）确认 pid 不是当前节点的子孙。dept 域同构修改。

### WR-05: 全部管理端点仅要求任意有效 token——任何登录用户可删除任意用户/重置 admin 密码/读取全部明文 API Key

**File:** `internal/app/router.go:44-91`
**Issue:** pri 组所有业务路由与 `kickout` 仅挂 `Auth` 中间件（认证），无任何授权校验。当前形态下任意低权用户登录后即可：DELETE 任意用户（含 admin）、重置任意密码（含 admin 的，固定 123456）、停用任意账号、踢任意会话、list/get 全部**明文** API Key、分配任意角色菜单。注释与测试标注「Phase 3 requirePerm 收口，T-2-07 accept」，属计划内已知项，此处记录在案防止两重风险：①Phase 3 未落地就部署；②后续域扩展时照抄「只挂 Auth」样板形成新的裸露端点。尤其 `kickout`（可被任意用户用来踢掉 admin 会话）与 API Key 明文读两处建议提前收口。
**Fix:** Phase 3 requirePerm 落地前的过渡期，最低限度为 `kickout`、`reset-password`、api-keys 全端点加内置角色白名单检查（会话已携带 roles 快照可直接判）；或在本 Phase 验证文档中把「部署期网络隔离/仅内网可达」列为硬性前置条件。

## Info

### IN-01: gofmt 漂移（3 文件）

**File:** `internal/model/po/api_key.go:17-20`、`internal/repository/api_key.go:124`、`internal/handler/auth_integration_test.go:25-28`
**Issue:** `gofmt -l` 报告三处非一致格式：ApiKeyPO 字段列对齐、repo 文件尾空行、测试 const 块对齐。
**Fix:** `gofmt -w` 三文件（已在 CI 前置钩子建议加 `gofmt -l` 检查）。

### IN-02: AssignRoles/AssignMenus 不去重请求 ID——重复 ID 产生重复关联行

**File:** `internal/service/user.go:283-292`、`internal/service/role.go:199-208`
**Issue:** `roleIds:[A,A]`/`menuIds:[M,M]` 前置校验全通过，事务内逐 ID 插行，sys_user_role/sys_role_menu 落重复行；登录 roles 快照与 GetRoles/GetMenus 随之出现重复项。表无 (user_id,role_id) 唯一约束，脏数据无 DB 防线。
**Fix:** 构造 reqIDs 前用 map 去重（保序），或落库前 `slices.Sort` + `slices.Compact`。

### IN-03: 菜单删除不清理 sys_role_menu——GetMenuIDs 返回悬挂 menuId

**File:** `internal/repository/menu.go:62-71`、`internal/repository/relation.go:64-76`
**Issue:** menu Delete 无引用检查是文档化源缺口（子节点孤儿提升已记录），但同一次删除还会留下 sys_role_menu 孤儿行：`GET /api/v1/roles/:id/menus` 此后返回已不存在菜单的 ID，前端回显勾选态指向死节点。影响面超出已记录的「树孤儿」范畴。
**Fix:** 若维持源语义，至少在 `Delete` 的注释中补充 role_menu 悬挂说明；更优：menu 删除时同事务 `DELETE FROM sys_role_menu WHERE menu_id = ?`（关联表冗余列删除无副作用）。

### IN-04: user-tokens 索引死 token 仅靠 kickout 惰性清理——长期不踢场景索引单调增长

**File:** `internal/auth/session.go:148-173`
**Issue:** `iam:user-tokens:{userId}` 无 TTL，登录只 SADD 不清理；死 token 收敛仅发生在 KickoutUser（停用/重置/显式踢）。从未被踢的活跃用户每次登录都永久累积一个索引成员（会话本身已过期）。单机长期运行下无上界增长（注释已认可是 Sa-Token 同构残留）。
**Fix:** Login 的 TxPipelined 内追加 `SRem` 旧值前先 `SCard` 上限检查；或给索引键设 `timeout*2` 的 TTL 并在 Resolve 命中时顺带续期；或定期任务按 SMembers 批量 Exists 清理。

### IN-05: dummyHash 生成错误被忽略——失败时恒时防枚举静默退化为快速路径

**File:** `internal/service/auth.go:31`
**Issue:** `dummyHash, _ := bcrypt.GenerateFromPassword(...)` 丢弃 error。一旦生成失败（概率极低但非零），dummyHash 为 nil，`CompareHashAndPassword(nil, ...)` 立即返回——「用户不存在」路径不再消耗 bcrypt 耗时，T-2-01 恒时语义无声失效且无任何日志。
**Fix:** 显式检查 error，失败时 `panic`/返回构造错误（服务无法安全提供登录语义即不应启动），或至少 slog.Error 留痕。

### IN-06: UpdateStatus/ResetPassword 的 kick 失败返回 500 但 DB 变更已持久

**File:** `internal/service/user.go:327-329,355`
**Issue:** Patch 提交后 `KickoutUser` 失败（Redis 故障）→ 接口 500，但停用/重置已落库。重试幂等安全、且响应正确表达了「未完全成功」，属可接受的最终一致语义；记录在此是因为该窗口正是 WR-02 复用分支绕过状态的成因之一，修复 WR-02 后此残留无害。
**Fix:** 无需必改；可在响应语义文档中注明「踢会话失败需重试」；或失败时 slog Error 留痕便于运维核对。

### IN-07: 默认配置内嵌 root:root123 数据库凭据

**File:** `internal/config/config.go:23`、`configs/config.yaml:9`
**Issue:** `DefaultDSN` 与默认 YAML 均内嵌 `root:root123@tcp(127.0.0.1:3306)/sys_iam`。工控机单机 appliance 部署模型下属已知取舍（env `IAM_MYSQL_DSN` 可覆盖），但二进制/仓库内硬编码超管凭据仍会在配置缺省时静默以 DB 超管连接。
**Fix:** 部署清单强制要求显式 DSN（可将空 DSN 默认值 + 启动期 fail-fast 作为后续 Phase 项），或在部署文档将「修改默认 MySQL 口令」列为安装必做步骤。

### IN-08: ApiKeyPO.Username 非指针列——无主 key 落库 '' 而非 NULL，出网 "" 而非 null

**File:** `internal/model/po/api_key.go:19`（转换 `internal/model/dto/api_key.go:52`）
**Issue:** 同一 PO 内 `UserID *int64` 用指针精确保留 NULL，`Username string` 却是非指针：无 userId 的 key 创建时 username 落库空串（DDL 允许 NULL，源 addApiKey MP null-skip 语义落 NULL），出网契约 `"username":""` 与全库「未赋值字段以 null 键输出」的无 omitempty 契约不一致。
**Fix:** `Username` 改 `*string`（仅当关联用户且 `u.Username != nil` 时赋值），`ToApiKeyResp` 直接透传指针；存量 '' 行可在读取侧归一化为 nil。

---

_Reviewed: 2026-11-01T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
