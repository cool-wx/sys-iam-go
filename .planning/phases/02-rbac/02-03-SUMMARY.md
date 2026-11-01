---
phase: 02-rbac
plan: 03
subsystem: rbac
tags: [user-assembly, assign-roles, kickout, reset-password, builtin-guard, api-key, plaintext-once, n-plus-one, gin, gorm]

# Dependency graph
requires:
  - phase: 02-rbac-02
    provides: roleRepo.FindByIDs / deptRepo.FindByID（校验链）、errcode 10601 单例（builtin 用户守卫）、db.Transaction + NewXxxRepo(tx) 事务模式、泛型树构建器（本 plan 不触及）
  - phase: 02-rbac-01
    provides: SessionManager.KickoutUser（D-09 踢会话落点）、relationRepo.FindRolesByUserIDs（两查询装配读路径）、po.UserRolePO、po.ApiKeyPO + ApiKeyRepo.FindByKey（10503 登录链）、testutil 双容器 + seedApiKey/apikeyLogin helper
  - phase: 01-foundation
    provides: repo.Patch 模板（CR-01）、UserPO、paging/PageData、ids/types/errcode/resp 契约、user 域 handler/service 模板
provides:
  - User 装配域完整增强：GET/PUT /api/v1/users/:id/roles（全量替换+全回收）、PUT :id/status（停用即踢存量会话）、PUT :id/reset-password（123456+first_login+踢会话）、DELETE :id（builtin 409/10601 前置拦截 + 先删 user_role 再删用户事务）
  - UserResp.Roles 两查询装配（FindRolesByUserIDs 一次性 IN→内存组装——全列表恒定 3 条 SQL 硬上界防 N+1；空角色输出 [] 非 null）
  - Create/Update deptId 必填校验链（「部门不能为空」/「部门不存在: {id}」，deptName 服务端查库冗余不可伪造）
  - ApiKey 域纵切全套：POST /api/v1/api-keys（crypto/rand 32 hex 明文一次性返回）/list 分页/GET :id/PUT :id/status（仅 status 单列）/DELETE :id
  - types.LocalDateTime UnmarshalJSON（与 MarshalJSON 对称——请求侧时间键反序列化能力，主格式 yyyy-MM-dd HH:mm:ss + RFC3339 兜底）
  - 路由：/api/v1/api-keys 五端点挂 pri 组；Phase 2 五域六组路由全量注册完成（白名单仍仅 02-01 四路径）
affects: [Phase 3 Casbin（user-role/role-menu 关联数据面齐备）, Phase 4 文件域（ApiKey 域为管理面参照）, 现场部署（重置密码 123456 语义需运维知悉）]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 22100    # chars/4 over the realized diff (88381 chars, 17 files)
  tasks: 2         # tasks completed
  commits: 3       # MEASURED: git rev-list --count gsd-plan-head-before-02-03..HEAD

tech-stack:
  added: []        # 零新增依赖（T-2-SC 达成：crypto/rand+encoding/hex 标准库生成 key）
  patterns:
    - 两查询装配防 N+1 单收口：assembleRoles（用户集→FindRolesByUserIDs 一次 IN→内存组装），Get/List/详情共用
    - 全删重建单事务：AssignRoles（DeleteUserRolesByUserID + BatchCreateUserRoles）/ Delete（先删关联再删用户）
    - 管理动作即踢存量会话（D-09 安全增强）：UpdateStatus(DISABLED)/ResetPassword → sm.KickoutUser
    - 校验前置优于源"事务内先删后校验"：roleRepo.FindByIDs 缺失集合消息「角色不存在: [逗号拼接]」（与 02-02 assignMenus 同决策）
    - 引用实体 vs 操作主体双轨译码：请求参数引用的实体不存在 → 400 CodeInvalidParam（部门/角色/ApiKey.userId），操作主体不存在 → 404 CodeNotFound（GET/PUT/DELETE :id）
    - 明文凭据一次性语义：key 仅 create 响应返回，写路径零重生成（Patch status 白名单）

key-files:
  created:
    - internal/model/dto/api_key.go
    - internal/service/api_key.go
    - internal/handler/api_key.go
    - internal/handler/api_key_integration_test.go
    - internal/handler/user_extension_integration_test.go
  modified:
    - internal/model/dto/user.go
    - internal/repository/relation.go
    - internal/repository/user.go
    - internal/repository/api_key.go
    - internal/service/user.go
    - internal/service/user_integration_test.go
    - internal/handler/user.go
    - internal/handler/user_integration_test.go
    - internal/app/router.go
    - internal/app/app.go
    - pkg/types/datetime.go
    - pkg/types/datetime_test.go

key-decisions:
  - "ApiKey create userId 引用不存在译 400「用户不存在: {id}」（CodeInvalidParam）——与「部门不存在/角色不存在」同链：请求参数引用实体 400、操作主体 404 双轨惯例成文"
  - "sys_user_role.username 列存 realName（非空白回退 username）而 ApiKey.username 冗余取 user.Username——两处相反语义均照源移植并双向注释防混"
  - "types.LocalDateTime 补 UnmarshalJSON（主格式源 DATE_TIME_FORMAT + RFC3339 兜底 + 空串置零）——MarshalJSON 既有而 Unmarshal 缺失致请求侧 expireTime 键全域不可用，属契约对称性缺口非新功能"
  - "NewUserService 依赖扩为 (db, userRepo, relationRepo, roleRepo, deptRepo, sm)——事务边界与踢会话能力集中一服务；两处既有测试以 nil sm 适配（不触会话路径）"

requirements-completed: [USER-01, USER-02, USER-03, USER-04, USER-05, USER-07, AKEY-01, AKEY-02]

duration: 50 min（恢复执行：含前 executor 中断工作盘点与校验 15 min）
completed: 2026-11-01T00:40:38Z
---

# Phase 2 Plan 3: User 装配域增强与 ApiKey 域纵切 Summary

**One-liner:** 用户域 roles 装配/分配（两查询防 N+1）/停用踢会话/重置密码/builtin 409 与 ApiKey 域全量 CRUD（明文一次性返回 + 停用过期不可登录）——Phase 2 的 21 项需求全齐，五域六组路由全量注册且白名单仅四路径。

## 恢复执行说明

前一 executor 于 2026-10-31 23:02 因 API 限额中断，未提交工作保留在工作区。本次恢复按交接流程：`git status` 盘点 → 读改动核心文件（service/user.go 的 Task 1 全部方法、handler/router/app/DTO/repo、两个测试文件均已成形且编译通过）→ 跑 verify 定位断点（`TestUserRolesAssignReplace` 与 `TestDeleteUserCleansRelations` 两处失败——正是交接描述的 Rule 3 修复中断点，handler.Delete 与路由已补齐但未复验）→ 修复后作为 Task 1 提交边界正常提交（不丢弃、不重做）。ApiKey 域（Task 2）确未动工，本 executor 全量实现。

## Accomplishments

- **USER-01 用户 CRUD 收口**：create 密码 bcrypt + deptId 必填校验链（「部门不能为空」/「部门不存在: {id}」，deptName 服务端查库冗余不可由请求伪造）；edit 白名单不含 username/password；delete 先删 user_role 再删用户（事务）；username 重复 400
- **USER-02 停用即踢**：PUT :id/status 停用后该用户全部存量会话立即失效（TestUserStatusDisableKickout 三段直证：旧 token 401 → 停用登录拒 10502 → 启用恢复登录），D-09 安全增强落地
- **USER-03 重置密码**：PUT :id/reset-password 重置 123456（bcrypt 入库 + first_login=true）+ 存量会话即踢；TestResetPasswordFlow 直证 tokenA 401 → 123456 登录成功且 firstLogin=true → 旧密码失效 → DB 行 60 字符 $2a$ 哈希非明文
- **USER-04 角色子资源**：PUT :id/roles 全量替换（角色不存在「角色不存在: [逗号拼接]」精确消息；空数组=全回收）；GET :id/roles 返回当前角色；sys_user_role.username 列存 realName（源命名错位原样移植，DB 行级断言）
- **USER-05 两查询装配**：UserResp.Roles 经 assembleRoles 单收口（FindRolesByUserIDs 一次性 IN→内存组装，代码结构无 per-user 查询调用）；TestUserListDetailRolesAssembly 断言 3 用户 roles 互不串扰、无角色用户 [] 非 null；分页默认 1/15 钳位保持
- **USER-07 builtin 删除拦截**：DELETE builtin=true 用户 → HTTP 409 + code=10601 零删除调用前置拦截（TestDeleteBuiltinUser409 直证用户仍在）
- **AKEY-01 ApiKey CRUD**：create 生成 crypto/rand 32 hex 明文一次性随响应返回（正则直证）；userId 可选但非空必校验（400「用户不存在: {id}」）；username 冗余取 user.Username（源语义）；status 默认 ENABLED；edit 仅 status 列（PUT 后 GET 断言 expire_time/username/user_id/remark 全部不动）；list 分页 envelope；delete 后 404 域消息
- **AKEY-02 停用/过期不可登录**：TestApiKeyDisabledCannotLogin（管理面停用 → apikey-login 200→401/10503 → 启用恢复）；TestApiKeyExpiredCannotLogin（过期 seed → 10503）——FindByKey 登录查询恒带过滤直证（T-2-16）
- **Phase 2 完成标志**：六组路由 GET 无 token 全 401 遍历断言（TestApiKeyCRUD 内 12 路径 + PUT/DELETE 抽查）；全量集成 wave 合并门全绿

## Coverage

coverage:
  - deliverable: USER-04 roles 子资源全量替换/全回收 + username 列 realName 错位移植
    verification: [{kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestUserRolesAssignReplace", status: pass}, {kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestUserRolesInvalidRole", status: pass}]
    human_judgment: false
  - deliverable: USER-02/T-2-13 停用即踢存量会话（D-09）
    verification: [{kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestUserStatusDisableKickout", status: pass}]
    human_judgment: false
  - deliverable: USER-03/T-2-14 重置密码 123456 + 踢会话
    verification: [{kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestResetPasswordFlow", status: pass}]
    human_judgment: false
  - deliverable: USER-07/D-14 builtin 用户删除 409/10601 前置拦截
    verification: [{kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestDeleteBuiltinUser409", status: pass}]
    human_judgment: false
  - deliverable: USER-07 删除清理关联（先删 user_role 再删用户事务）
    verification: [{kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestDeleteUserCleansRelations", status: pass}]
    human_judgment: false
  - deliverable: USER-05 两查询装配防 N+1（list/detail 互不串扰 + [] 非 null）
    verification: [{kind: tests, ref: "tests/internal/handler/user_extension_integration_test.go#TestUserListDetailRolesAssembly", status: pass}, {kind: command, ref: "grep assembleRoles internal/service/user.go（FindRolesByUserIDs 用户集后单次调用，无 per-user 查询）", status: pass}]
    human_judgment: false
  - deliverable: AKEY-01 ApiKey CRUD（32 hex 明文/username 冗余/仅 status 单列/分页/404）
    verification: [{kind: tests, ref: "tests/internal/handler/api_key_integration_test.go#TestApiKeyCRUD", status: pass}]
    human_judgment: false
  - deliverable: AKEY-02/T-2-16 停用/过期 key 不可登录（10503）
    verification: [{kind: tests, ref: "tests/internal/handler/api_key_integration_test.go#TestApiKeyDisabledCannotLogin", status: pass}, {kind: tests, ref: "tests/internal/handler/api_key_integration_test.go#TestApiKeyExpiredCannotLogin", status: pass}]
    human_judgment: false
  - deliverable: AUTH-06 白名单外无遗漏（六组路由 401 遍历）与 Phase 1 user/auth 回归
    verification: [{kind: tests, ref: "tests/internal/handler/api_key_integration_test.go#TestApiKeyCRUD（六组 GET 401 遍历段）", status: pass}, {kind: command, ref: "TZ=UTC go test -tags=integration ./... 全绿（handler 446s + service + pkg）", status: pass}]
    human_judgment: false

## Deviations from Plan

### 恢复期修复（前 executor 中断遗留）

**1. [Rule 1 - Bug] TestUserRolesAssignReplace 断言对象错位**
- **Found during:** Task 1 恢复复验
- **Issue:** 空数组全回收后对 PUT 响应断言 `"data":[]`——PUT `resp.OK(c,nil)` 输出 `data:null`；测试注释与 plan 意图均为「GET 返回 [] 非 null」
- **Fix:** getUserRoles helper 返回 body，断言改指 GET 响应
- **Files modified:** internal/handler/user_extension_integration_test.go
- **Verification:** TestUserRolesAssignReplace PASS
- **Commit:** 07dfbed

**2. [Rule 1 - Bug] service.Get 404 未译域（域内不一致）**
- **Found during:** Task 1 恢复复验（TestDeleteUserCleansRelations 断言「用户不存在: {id}」）
- **Issue:** 其余单行方法（AssignRoles/GetRoles/UpdateStatus/ResetPassword/Delete）均译域，唯 Get 透传哨兵「数据不存在」
- **Fix:** Get 补 ErrNotFound → userNotFound(id)（既有回归仅断言 code 不锁文案，无破坏）
- **Files modified:** internal/service/user.go
- **Verification:** TestDeleteUserCleansRelations PASS
- **Commit:** 07dfbed

**3. [Rule 1 - Bug] service 层 TestUpdateAuditWithIdentity 未适配部门校验链**
- **Found during:** plan 级全量集成验证
- **Issue:** 前 executor 适配了 NewUserService 签名但 Create 请求缺 DeptID（02-03 起必填）→ 全量 wave 合并门 FAIL
- **Fix:** seed 部门 100 + Create 请求补 DeptID
- **Files modified:** internal/service/user_integration_test.go
- **Verification:** 全量集成（`TZ=UTC go test -tags=integration ./...`）全绿
- **Commit:** c8bebc5

### Task 2 执行期修复

**4. [Rule 2 - Missing Critical] types.LocalDateTime 缺 UnmarshalJSON**
- **Found during:** Task 2（实现 ApiKeyCreateReq.ExpireTime 时实验验证）
- **Issue:** 仅 MarshalJSON 存在；encoding/json 对命名 time.Time 别名不走 time.Time 特判——请求侧 `expireTime` 键任何格式均反序列化失败（plan 定义的 ExpireTime 可选参数全域不可用）
- **Fix:** 补 UnmarshalJSON（主格式 yyyy-MM-dd HH:mm:ss 源 DATE_TIME_FORMAT 语义 + RFC3339 兜底 + 空串置零 + 非法格式报错不静默）；含引号原始字节先 Trim（同 UsageStatus 先例）
- **Files modified:** pkg/types/datetime.go、pkg/types/datetime_test.go（4 断言单测）
- **Verification:** go test ./pkg/types 全绿；TestApiKeyCRUD expireTime 落库断言 PASS
- **Commit:** 574f44f

**5. [Rule 1 - Bug] ApiKey create userId 引用不存在初实现译 404，按 plan/域内惯例改 400**
- **Found during:** Task 2 集成测试
- **Issue:** 初版复用 userNotFound helper（404 CodeNotFound）；plan action 明确 400，且域内惯例已成文——请求参数引用的实体不存在（部门/角色）→ 400 CodeInvalidParam
- **Fix:** userId 分支改译 CodeInvalidParam「用户不存在: {id}」（消息原文保留）；操作主体 404 双轨惯例在 SUMMARY key-decisions 成文
- **Files modified:** internal/service/api_key.go
- **Verification:** TestApiKeyCRUD 400+消息精确断言 PASS
- **Commit:** 574f44f

**Total deviations:** 5 auto-fixed（Rule 1 x4、Rule 2 x1）。**Impact:** 全部为恢复复验/测试驱动暴露的域内一致性与契约对称性缺口，不改变 plan 交付范围；types 层补全惠及后续任何请求侧时间字段。

## Threat Mitigations Verified

| Threat | Mitigation | Evidence |
|--------|-----------|----------|
| T-2-13 停用用户持旧 token | UpdateStatus(DISABLED) → sm.KickoutUser | TestUserStatusDisableKickout 三段 PASS |
| T-2-14 重置后旧会话存活 | ResetPassword → sm.KickoutUser | TestResetPasswordFlow tokenA 401 PASS |
| T-2-15 明文凭据/敏感键泄漏 | UserResp 无 password/extToken；ApiKey 明文仅 create+管理面读；写路径 Patch status 白名单不可覆写 key | TestApiKeyCRUD 仅 status 变化断言 PASS；grep 无明文落日志行 |
| T-2-16 停用/过期 key 可登录 | FindByKey 恒带 status=ENABLED + expire_time 过滤（唯一登录取 key 路径） | TestApiKeyDisabledCannotLogin / TestApiKeyExpiredCannotLogin 10503 PASS |
| T-2-SC 供应链 | 零新增依赖（crypto/rand 标准库） | go.mod/go.sum 未触碰 |

## Known Stubs

None——本 plan 无 stub、无 skipped test、无未跑 verify（全部 `<verify>` 与 acceptance criteria 均已执行并通过）。

## Authentication Gates

None——执行全程无认证门。

## Issues Encountered

- testcontainers 双容器 per-test 启动使单测试 ~13s、handler 包全量 ~7.5min：为控制耗时，修复迭代期用 `-run` 精确过滤单测复验，最终以 plan 级全量集成一遍收口（避免反复全量）
- 前一 executor 中断点即两处测试失败（其 Rule 3 修复已成形未复验）——恢复后先复验定位、修复、再提交，与交接描述完全吻合，无工作丢失

## Next Phase Readiness

Phase 2 全部 3 个 plan 完成，21 项需求全齐（AUTH 6 + MENU/DEPT/ROLE 7 + USER/AKEY 8）。五域纵切全通、六组路由全量注册且经认证中间件保护、白名单仅 /healthz /readyz /auth/login /auth/apikey-login 四路径。Phase 3（功能权限 Casbin）的 user-role/role-menu 关联数据面与 roles 会话快照已就绪。

## Self-Check: PASSED

- 17 个 key-files（created 5 + modified 12）全部存在于磁盘（`[ -f ]` 逐一核验）
- 3 个 commit 全部为 HEAD 祖先：07dfbed / 574f44f / c8bebc5（`git merge-base --is-ancestor` 核验）
- `gsd_run check evaluation-scope` 同源核验：commits=3 与 frontmatter 一致
- plan 级 `<verification>` 四项全部重跑通过（build/vet/单元/全量集成）
