# Phase 5: 名称同步事件与验收收尾 - Research

**Researched:** 2026-11-02
**Domain:** 进程内事件分发（Spring DomainEvent → Go 轻量 dispatcher）+ 冗余名称字段同步 + 全量验收回归
**Confidence:** HIGH（源监听器/发布时序全部逐文件直读；Go 挂点/测试基建全部逐文件直读）

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** internal/event 轻量 dispatcher——Publisher 接口 + 事件类型定义 + 监听器注册（slice of listeners，同步调用，约 50-80 行自建）。零框架依赖（CLAUDE.md 已排除 watermill 等）；对齐源 DomainEvent 心智模型（6 个监听器按事件类型注册）；可 mock 测试。— **Reversibility:** reversible — 新增独立包，重构不影响业务层
- **D-02:** 发布时序照 SYNC-02 锁定语义执行——service 层 `db.Transaction` 提交成功后显式 `publish`；handler 同步执行（非 goroutine 异步）且各自使用独立事务（新开 `db.Transaction`）。这是对源"事务内发布 + REQUIRES_NEW"时序的显式修正，验收明文。
- **D-03:** handler 独立事务失败上抛——API 返回 500，客户端重试触发 `!= newName` 幂等 UPDATE 自愈。沿 Phase 3 D-13 casbin 双轨纪律（业务事务提交后外部设施失败 500 + 幂等重试自愈），失败可观察、与既有错误处理链一致。不引入待同步清单/boot 对账（单机场景重试即自愈，无跨实例竞争）。
- **D-04:** 改名/改真名后在线会话快照照源不刷新——Redis 会话 `SessionPayload{UserID,UserName,RealName,Roles}`（internal/auth/session.go:49）中的 userName/realName 下次登录生效；displayName 短暂陈旧与源系统一致。Phase 2 D-09"停用即踢"安全增强**不延伸**到改名（改名非安全事件，踢线过度）。
- **D-05:** 全量回归证据 = 既有全部自动化测试全绿（回归即证）+ 8 项 "Looks Done But Isn't" 清单逐项映射（每项指向具体测试用例或 VERIFICATION.md 文档证据）+ VERIFICATION.md 逐项记录。清单项（ROADMAP 成功标准 3）：存量 hash 登录、API Key 双路径、内置删除拦截、编辑不传字段不清空、分页边界不 500、枚举未知值兜底、失败请求有日志、SIGTERM 后日志完整落库。不新建独立 regression 包（与既有分域集成测试重复）。
- **D-06:** WR-07/WR-08 顺带修复——永真断言（文件分类"移至根"集成断言恒真）修为真断言；parentId="0"（Update 移根响应）与 null（Create 根）双形态统一。统一方向研究期对齐源行为后定（倾向响应恒一形态），修复随本阶段测试收口落地。

### Claude's Discretion

- dispatcher 内部 API 形态（Publish 签名、事件结构体字段、监听器注册方式）
- 6 事件 × 关联表映射清单（researcher 从源 6 个监听器逐表提取，含 realName 波及 14 张表的精确列清单——sys_user_role.username 列存 realName 的相反语义需双向注释防混）
- NULL 语义锁定测试的具体断言形态（UPDATE 带 `!= newName` 对 NULL 行恒不中）
- 测试组织（沿 newTestApp 基建；事件同步集成测试的 fixture 设计）
- 计划切片与 wave 划分（Phase 4 先例：共享文件跨 plan 串行链）

### Deferred Ideas (OUT OF SCOPE)

- 改名即刷在线会话快照（经 iam:user-tokens 反查 HSET）——v2 增强
- 改名即踢下线——安全语义过度，不采纳（讨论中显式排除）
- 同步失败 boot 对账自愈（待同步清单 + 启动补齐）——现场重试自愈不足再引入
- 事件机制向其他域扩展（新事件类型随需注册，dispatcher 天然支持）
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SYNC-01 | 6 个名称变更事件（dept/dictType/menu/role/username/realName）同步关联表（realName 波及 14 张表） | 重点 1 的逐条映射表：事件→表→列→WHERE 全部从源 5 个监听器逐行提取（realName = 14 表 26 条 UPDATE，逐表验证）；重点 3 的 UsernameChangedEvent 死代码发现 |
| SYNC-02 | 事件事务提交后发布、handler 独立事务执行（防死锁时序）、UPDATE 带 `!= newName` 条件（NULL 语义锁定测试） | 重点 2 的源时序证据（文件:行号 + 2026-09-29 死锁实测注释）；Go 挂点为单语句 Patch 自动提交，Patch 返回后发布即天然满足"提交后发布"；NULL 锁定测试断言形态见 Implementation Guidance |
</phase_requirements>

## Key Findings（按研究重点）

### 重点 1：源 6 监听器逐条映射表（HIGH 置信，全部逐文件直读）

源监听器共 **5 个类承载 6 个事件**（RealNameSyncListener 一个类同时处理 UserRealNameChangedEvent 与 UsernameChangedEvent）。所有 UPDATE 均为三段式：`SET 冗余列 = newName WHERE 关联列 = 事件ID AND 冗余列 != newName`。

**DeptNameChangedEvent**（`DeptNameSyncListener.java:30-50`，事件类型 `DEPT_NAME_CHANGED`）:

| # | 表 | SET 列 | WHERE |
|---|----|--------|-------|
| 1 | sys_user | dept_name | `dept_id = {deptId} AND dept_name != {newName}` |
| 2 | sys_dept | parent_name | `parent_id = {deptId} AND parent_name != {newName}` |

**DictTypeNameChangedEvent**（`DictTypeNameSyncListener.java:26-38`，`DICT_TYPE_NAME_CHANGED`）:

| # | 表 | SET 列 | WHERE |
|---|----|--------|-------|
| 1 | sys_dict_data | dict_type_name | `dict_type_id = {typeId} AND dict_type_name != {newName}` |

**MenuNameChangedEvent**（`MenuNameSyncListener.java:30-50`，`MENU_NAME_CHANGED`）:

| # | 表 | SET 列 | WHERE |
|---|----|--------|-------|
| 1 | sys_role_menu | menu_name | `menu_id = {menuId} AND menu_name != {newName}` |
| 2 | sys_menu | parent_name | `parent_id = {menuId} AND parent_name != {newName}` |

**RoleNameChangedEvent**（`RoleNameSyncListener.java:30-50`，`ROLE_NAME_CHANGED`）:

| # | 表 | SET 列 | WHERE |
|---|----|--------|-------|
| 1 | sys_role_menu | role_name | `role_id = {roleId} AND role_name != {newName}` |
| 2 | sys_user_role | role_name | `role_id = {roleId} AND role_name != {newName}` |

**UsernameChangedEvent**（`RealNameSyncListener.java:114-125` 的 `onUsernameChanged`，`USERNAME_CHANGED`）:

| # | 表 | SET 列 | WHERE |
|---|----|--------|-------|
| 1 | sys_user_role | username | `user_id = {userId} AND username != {newName}` |

**UserRealNameChangedEvent**（`RealNameSyncListener.java:88-193`，`USER_REAL_NAME_CHANGED`）——**14 张表 26 条 UPDATE**：

| # | 表 | SET 列 × WHERE | 条数 |
|---|----|----------------|------|
| 1 | sys_user | create_by_name ← create_by_id={id}；update_by_name ← update_by_id={id} | 2 |
| 2 | sys_user_role | username ← user_id={id}（**列名 username 实存展示名 realName**，与上一事件同列不同值——源命名错位原样移植） | 1 |
| 3 | sys_dept | create_by_name/update_by_name ← create_by_id/update_by_id={id} | 2 |
| 4 | sys_role | 同上 | 2 |
| 5 | sys_menu | 同上 | 2 |
| 6 | sys_dict_type | 同上 | 2 |
| 7 | sys_dict_data | 同上 | 2 |
| 8 | sys_config | 同上 | 2 |
| 9 | sys_api_key | 同上 | 2 |
| 10 | sys_shift | 同上 | 2 |
| 11 | sys_fault_info | 同上 | 2 |
| 12 | sys_file_category | 同上 | 2 |
| 13 | sys_file | 同上（源 FileInfoMapper 实体名 FileInfo，Go 表名实证 `sys_file`，见 po/file.go:1-4） | 2 |
| 14 | sys_operation_log | real_name ← user_id={id} | 1 |

"realName 波及 14 张表"逐表清点完毕，与 REQUIREMENTS SYNC-01 的数字吻合。

**Go 侧表名/列名核对**（全部 `TableName()` 本会话直读 [VERIFIED: internal/model/po/]）：sys_user、sys_user_role、sys_role_menu、sys_role_dept、sys_dept、sys_role、sys_menu、sys_dict_type、sys_dict_data、sys_config、sys_api_key、sys_shift、sys_fault_info、sys_file、sys_file_category、sys_operation_log——与映射表逐一对应。审计列由 `BaseWithOperator`（po/base.go:9-17：create_by_id/create_by_name/update_by_id/update_by_name）承载；sys_operation_log 刻意不内嵌（po/operation_log.go:14-17），有独立 user_id/real_name 业务列（po/operation_log.go:24-25）；sys_user_role 无审计四列，仅 username/role_name 冗余（po/relation.go:14-24）。sys_dict_data.dict_type_name 列实证（po/dict.go:57）。

### 重点 2：源发布时序证据（HIGH 置信，文件:行号直读）

**两种时序在源中并存**：

| 服务方法 | 事务注解 | 发布点 | 监听器时序 |
|----------|----------|--------|-----------|
| DeptApplicationService.editDept | `@Transactional`（:52-53） | :75（事务内） | `@EventListener` + `REQUIRES_NEW`（DeptNameSyncListener.java:30-31） |
| RoleApplicationService.editRole | `@Transactional`（:76-77） | :86（事务内） | `@EventListener` + `REQUIRES_NEW`（RoleNameSyncListener.java:30-31） |
| MenuApplicationService.editMenu | `@Transactional`（:52-53） | :65（事务内） | `@EventListener` + `REQUIRES_NEW`（MenuNameSyncListener.java:30-31） |
| DictApplicationService.editDictType | **无 @Transactional**（:45） | :54（单语句自动提交后） | `@EventListener` + `REQUIRES_NEW`（DictTypeNameSyncListener.java:26-27） |
| UserApplicationService.editUser | `@Transactional` | :140（事务内） | `@TransactionalEventListener(AFTER_COMMIT, fallbackExecution=true)` + `REQUIRES_NEW`（RealNameSyncListener.java:88-89） |

关键证据——**源自己已经修过一次死锁**。RealNameSyncListener.java:85-87 原文注释：

> AFTER_COMMIT：主事务提交（行锁释放）后再同步——修复同线程自死锁
> （事务 A updateById 持行锁未提交时，REQUIRES_NEW 的全表冗余同步 UPDATE 等同一行锁 50s 超时；
> 2026-09-29 交付机实测锁定：innodb_trx 双事务 LOCK WAIT + 事务空闲持锁对）

即：源 realName 轴已改 AFTER_COMMIT，而 4 个名称监听器仍停留在"事务内发布 + `@EventListener` + REQUIRES_NEW"（监听器新事务在主事务持锁未提交时即启动，正是该注释描述的死锁形态——4 监听器的 UPDATE 目标是关联行非本行，未爆过不等于无风险）。**SYNC-02 的"修正源时序"是把 realName 轴已验证的 AFTER_COMMIT 语义统一到全部 6 个事件**，有源内实证背书，非发明。

发布机制：`SpringDomainEventPublisher.publish` → `ApplicationEventPublisher.publishEvent`（SpringDomainEventPublisher.java:24-26）；全源码树未检出 `@EnableAsync`/异步执行器配置——**事件同步分发**，监听器在发布者线程执行。事件基类 `DomainEvent`（aggregateRootId/aggregateRootType/eventType/occurredAt/eventId）；6 个事件类均携带 `oldName`/`newName` 两字段（发布侧 oldName 空值兜底为 `""`，如 DeptApplicationService.java:77）。

**发布条件**（全部为"请求带新名 且 新名≠旧名"检测，逐点直读）：
- dept：`dept.getDeptName() != null && !dept.getDeptName().equals(oldName)`
- role：`roleName != null && !roleName.equals(oldName)`
- menu：`menu.getName() != null && !menu.getName().equals(oldName)`
- dictType：`updated && !Objects.equals(oldName, newName) && newName != null`（多一个"更新成功"前置）
- user realName：`realName != null && !realName.equals(oldRealName)`

### 重点 3：UsernameChangedEvent 是源内死代码——Go 同样无触发路径（HIGH 置信）

- 源 `editUser`（UserApplicationService.java:115-155）签名**不含 username 参数**——源系统 username 创建后不可改（登录锚点）。全源码树 grep：`UsernameChangedEvent` 仅出现在事件类定义与监听器方法两处，**无任何 publish 调用点**；`editUsername/changeUsername/updateUsername/renameUser` 零命中。
- Go 侧一致：`UserService.Update` 白名单刻意不含 username/password（internal/service/user.go:130-133 注释明文"源 editUser 不可改语义：用户名为登录锚点"）。
- 结论：SYNC-01 字面的"6 个事件"中，username 事件**在源与 Go 都没有触发路径**。处置建议见 Open Questions Q1（倾向：handler 与事件类型照源补齐（约 10 行，契约形态与 SYNC-01 文字对齐），user.Update 不设发布点，VERIFICATION.md 注明"无触发路径系源语义"）。

### 重点 4：Go 侧挂点清单与波及面（HIGH 置信）

6 个发布挂点全部落在既有 `Update` 方法，但**均需在 Patch 前回读旧行**（当前无一回读旧名——源发布条件需要 oldName）：

| 挂点文件:行 | 方法 | 事件 | 需要的前置改动 |
|-------------|------|------|----------------|
| internal/service/dept.go:56-101 | Update | DeptNameChanged | `req.DeptName != nil` 时 Patch 前 `FindByID` 取旧名；Patch 成功后发布 |
| internal/service/role.go:106-172 | Update | RoleNameChanged | 同上（status 守卫已有条件 FindByID，改名检测需独立判断 `req.RoleName != nil`） |
| internal/service/menu.go:73-137 | Update | MenuNameChanged | `req.Name != nil` 时同上 |
| internal/service/dict.go:60-84 | UpdateDictType | DictTypeNameChanged | `req.DictName != nil` 时同上；源条件含 updated（Patch 成功） |
| internal/service/user.go:135-177 | Update | UserRealNameChanged | `req.RealName != nil` 时同上 |
| （无） | — | UsernameChanged | 无触发路径（重点 3） |

时序语义说明：`SkipDefaultTransaction: true` 下各 Update 的 `repo.Patch` 是**单语句自动提交**（app.go:51），Patch 返回 nil 即已提交——"提交后发布"由调用顺序天然满足，无需包显式事务。handler 侧再开 `db.Transaction`（独立事务，D-02），物理上与发布者事务先后分离，源注释描述的行锁死锁面不存在。

**波及面**：
- 5 个 service 构造函数需增 publisher 依赖：`NewDeptService(repo)`（dept.go:23）、`NewMenuService(menuRepo)`、`NewDictService(typeRepo, dataRepo)`、`NewRoleService(...)`（role.go:40）、`NewUserService(...)`（user.go:42）。
- 装配点唯一：internal/app/app.go:160-172 构造链（CONTEXT 指定 dispatcher 构造在 Redis client 之后、router 之前——即 :131 sessionMgr 与 :188 newRouter 之间任意点，建议 repo 构造后 service 构造前，约 :155 处）。
- 测试直接构造仅 1 处：internal/service/user_integration_test.go:49 `NewUserService(...)`——需传 fake/nil publisher。
- 集成测试经 `newTestApp`（user_integration_test.go:178）走真实 `app.New` 装配，dispatcher 注入自动生效，无需逐测试改。
- repository 层现状：internal/repository/relation.go **无任何带 `.ne` 条件的 UPDATE 方法**——同步 UPDATE 的落点需要决策（建议见 Implementation Guidance）。

### 重点 5：WR-07/WR-08 修复方向（HIGH 置信）

**机理（均已实测实证，04-REVIEW-DISPOSITION 原始记录）**：
- **WR-07**：`ids.ID` 按 ENG-03 序列化为 JSON 字符串。Update 移根写 `parent_id = 0`（dept.go:66-67、menu.go:87-88、file_category.go 同款）→ 响应 `parentId: "0"`；Create 根 parent_id 保持 NULL → 响应 `parentId: null`。同域两路径对"根"输出两种形态。
- **WR-08**：TestFileCategoryTree 移根断言（file_integration_test.go:348-353）`json.Unmarshal(body, &cleared)` **漏了 `{code,message,data}` envelope 的 `Data` 层**——字段永不填充、断言永不失败。修 envelope 后断言即真，且会立刻暴露 WR-07（实际响应是 `"0"` 非_nil_）→ **WR-07/WR-08 必须同任务落地，不可拆分**。

**源行为考古（FileCategoryApplicationService.editCategory，全文直读）**：源 Update 移根（pid=0）时 parentName 置 null 后 `updateById`——MP null-skip 使 **parent_name 残留旧值**（Go 已有意修正为清 NULL，menu.go:84-86 注释在案），parent_id=0 正常写入；源 Create 根 parent_id 为 NULL。**源自身即不对称**（Create null / Update 0）——D-06"倾向响应恒一形态"即接受这一处对源的显式偏离。

**统一方向建议（二选一，推荐 A）**：
- **方向 A（DB+响应双恒一，推荐）**：Update 移根改写 `cols["parent_id"] = nil`（替代 0），三域（file_category/dept/menu）同改。根 = parent_id NULL 成为唯一形态，响应经既有 `if m.ParentID != nil`（dto/dept.go:66、dto/menu.go:91、dto/file.go:77）自然输出 null，DTO 零改动。消费者安全性已核：`tree.Build` 对 nil/0 均提升为根、`ensureNotDescendant` 与 `resolveParent` 均双形态兼容（dept.go:182、186）；既有测试不锁定响应形态（dept 移根仅断 200，menu_dept_integration_test.go:411-413；file 移根断言真空待修）。
- **方向 B（仅响应恒一）**：DB 保留 0，三个 `To*Resp` 加 `*m.ParentID != 0` 判断。改动面更大（3 个 DTO + 注释），DB 仍双形态。
- 范围决策：WR-07 挂账只在 file_category，但 dept/menu 同款双形态（menu.go:84-88 注释自称"与 Create 语义对齐"实未对齐 parent_id 列）。建议三域同改（每域 1 行 + 注释更新），VERIFICATION 记录为 Go 内一致性修正。

### 重点 6：8 项 "Looks Done But Isn't" 清单测试映射现状（HIGH 置信）

| # | 清单项 | 现有测试/证据 | 状态 |
|---|--------|---------------|------|
| 1 | 存量 hash 登录 | auth_integration_test.go:25 `testAdminHash = "$2a$10$7JB7..."`（newTestApp 播种）；TestAuthFlow / TestAuthLoginFailures | ✅ 已覆盖 |
| 2 | API Key 双路径 | TestApiKeyLoginFlow（密码+apikey 双路径）+ TestBindAPIKeyDualKeys / TestConcurrentApiKeyFullLogin（session_test.go） | ✅ 已覆盖 |
| 3 | 内置删除拦截 | TestDeleteBuiltinUser409 + TestRoleBuiltinDeleteConflict（+ TestRoleBuiltinDisableConflict WR-06 面） | ✅ 已覆盖 |
| 4 | 编辑不传字段不清空 | TestUpdateUserZeroValueProtection + TestUpdateUserExplicitClear（ENG-05 零值保护） | ✅ 已覆盖（user 域直证；其他域同 Patch 模板） |
| 5 | 分页边界不 500 | TestListUsersEdgeParams + TestOpLogListPagination（olog 1/10 钳位例外） | ✅ 已覆盖 |
| 6 | 枚举未知值兜底 | menu type=99 兜底 MENU（menu_dept_integration_test.go:127-129）、gender=99 兜底 UNKNOWN（user_integration_test.go:668-669）+ pkg/types enum_test.go | ✅ 已覆盖 |
| 7 | 失败请求有日志 | TestOpLogFailedRequestRecorded（olog_integration_test.go:174，status=0 落库） | ✅ 已覆盖 |
| 8 | SIGTERM 后日志完整落库 | **组合证据，无单一端到端测试**：TestOpLogWriteRecorded 经 `FlushOpLogs` 同步断言落库（olog_integration_test.go:75）+ TestGracefulShutdownDrains 走真实 `Stop()`（shutdown_integration_test.go:168，Stop 内含 flush，app.go:255-259），但无"入队→真实 Stop→断言 DB 行"一体的测试；IN-04（Stop 双 Flush）同面 | ⚠️ 缺口：补一个小测试或接受组合证据并在 VERIFICATION.md 记录链路 |

D-05 明文"既有全部测试全绿即回归证据"——本阶段改动（5 个 service Update + app 装配 + 3 处移根语义）触及的既有测试面：menu/dept/file/dict/role/user 各域集成测试 + `internal/service/user_integration_test.go` 直接构造点。全量 `go test ./...` 即回归门。

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| 事件分发（dispatcher/注册/发布） | API/Backend（internal/event） | — | 进程内同步机制，单机无外部中间件；service 层发布、handler 消费 |
| 名称变更检测与发布 | API/Backend（internal/service） | — | 业务语义（新名≠旧名）只有 service 层知道；事务边界也在 service |
| 冗余字段同步 UPDATE | Database/Storage（GORM） | API/Backend（internal/event handler） | handler 开独立事务执行确定性 UPDATE；非业务查询不进 repository（建议，见 Implementation Guidance） |
| 会话快照陈旧 | Storage（Redis） | — | D-04 照源不刷新——本阶段显式不动 internal/auth |
| WR-07/08 响应形态 | API/Backend（internal/service + dto） | — | 移根落库语义在 service，序列化形态在 dto |
| 回归证据 | Test（integration） | 文档（VERIFICATION.md） | D-05：全绿 + 8 项映射 + 逐项记录 |

## Standard Stack

**零新外部依赖**——本阶段纯自建（D-01 明文禁事件总线框架）+ 既有库（gorm/redis/testify/testcontainers 已在 go.mod）。无安装步骤、无版本核对项。

## Package Legitimacy Audit

本阶段**不安装任何外部包**（dispatcher 约 50-80 行标准库自建，D-01）。审计表为空：无新增依赖、无 `go get` 步骤、无 slopsquat 暴露面。

## Architecture Patterns

### System Architecture Diagram

```
PUT /api/v1/{depts|roles|menus|dict-types|users}/:id
        │
        ▼
┌───────────────────────── internal/handler ─────────────────────────┐
│  DTO 绑定 → service.Update(ctx, id, req)                           │
└──────────────────────────────┬─────────────────────────────────────┘
                               ▼
┌───────────────────────── internal/service ─────────────────────────┐
│ 1. req.{Name} != nil ? FindByID 回读旧行                            │
│ 2. repo.Patch（单语句自动提交，SkipDefaultTransaction）              │
│        │ 提交成功（err == nil）                                     │
│ 3. 新名 != 旧名 ? ────否────► 返回 200（不发布）                     │
│        │是                                                          │
│ 4. publisher.Publish(NameChangedEvent{ID, Old, New})                │
│        │ 失败 → 500（D-03；客户端重试 → 幂等 UPDATE 自愈）           │
└────────┬───────────────────────────────────────────────────────────┘
         ▼ 同步调用（非 goroutine；主事务已提交，无行锁交叉）
┌───────────────────────── internal/event ───────────────────────────┐
│  Dispatcher：按事件类型查 listeners slice → 逐个同步调用             │
│        │ 任一 listener 出错 → Publish 返回 error → service → 500    │
└────────┬───────────────────────────────────────────────────────────┘
         ▼
┌──────────────────── 6 个同步 handler（独立事务）────────────────────┐
│ db.Transaction{                                                     │
│   UPDATE <表> SET 冗余列 = newName                                  │
│   WHERE 关联列 = eventID AND 冗余列 <> newName  ← NULL 行恒不中      │
│ } （realName handler：14 表 26 条 UPDATE 同一事务，照源 REQUIRES_NEW）│
└─────────────────────────────────────────────────────────────────────┘
```

主用例追踪：改名请求 → handler 绑定 → service 回读旧名 → Patch 提交 → 检测到变化发布 → dispatcher 同步分发 → 对应 handler 独立事务内 UPDATE 冗余列 → 全部成功返回 200；任一同步 UPDATE 失败 → 500 → 重试幂等自愈。

### Recommended Project Structure

```
internal/
├── event/                    # 新增（D-01）
│   ├── event.go              # 事件类型常量 + NameChangedEvent 结构 + Publisher 接口
│   ├── dispatcher.go         # 注册（map[type][]listener）+ 同步 Publish
│   ├── listeners.go          # 6 个同步 handler（持 *gorm.DB，独立事务 UPDATE）
│   └── *_test.go             # dispatcher 单测 + handler 集成测试（或放 handler 层）
├── service/
│   ├── dept.go               # Update：+旧名回读 +发布（其余 4 域同构）
│   └── ...
└── app/
    └── app.go                # dispatcher 构造 + 6 listener 注册 + 5 service 注入
```

### Pattern 1: dispatcher（D-01 建议形态，约 60 行）

```go
// internal/event/event.go
type Type string

const (
    DeptNameChanged  Type = "DEPT_NAME_CHANGED"      // 与源 eventType 字符串一致
    DictTypeNameChanged Type = "DICT_TYPE_NAME_CHANGED"
    MenuNameChanged  Type = "MENU_NAME_CHANGED"
    RoleNameChanged  Type = "ROLE_NAME_CHANGED"
    UsernameChanged  Type = "USERNAME_CHANGED"
    RealNameChanged  Type = "USER_REAL_NAME_CHANGED"
)

// NameChangedEvent 6 事件共用载荷（源事件类均为 aggregateRootId+oldName+newName 三元组）。
type NameChangedEvent struct {
    Type    Type
    ID      int64  // 源 aggregateRootId
    OldName string // 发布侧空值兜底 ""（源 oldName != null ? oldName : "" 同构）
    NewName string
}

// Publisher service 层依赖的最小接口（mock 点）。
type Publisher interface {
    Publish(evt NameChangedEvent) error
}

// internal/event/dispatcher.go
type Listener func(ctx context.Context, evt NameChangedEvent) error

type Dispatcher struct {
    mu        sync.Mutex
    listeners map[Type][]Listener
}

// Publish 同步逐个调用；首个错误即返回（→ API 500，D-03）。
// handler 运行于发布者 goroutine，主事务已提交——无 REQUIRES_NEW 行锁交叉面。
```

### Pattern 2: handler（独立事务 + `<>` 幂等，dept 例）

```go
// internal/event/listeners.go —— UPDATE 用 gorm Model+占位符（列名为代码常量，
// 值全参数化，T-1-03 同纪律）。
func (h *SyncHandlers) OnDeptNameChanged(ctx context.Context, evt NameChangedEvent) error {
    return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Model(&po.UserPO{}).
            Where("dept_id = ? AND dept_name <> ?", evt.ID, evt.NewName).
            Update("dept_name", evt.NewName).Error; err != nil {
            return err
        }
        return tx.Model(&po.DeptPO{}).
            Where("parent_id = ? AND parent_name <> ?", evt.ID, evt.NewName).
            Update("parent_name", evt.NewName).Error
    })
}
```

### Pattern 3: 发布挂点（service 层，dept 例）

```go
// Update 内，Patch 之前：
var old *po.DeptPO
if req.DeptName != nil {
    var err error
    old, err = s.repo.FindByID(ctx, id)
    if err != nil { /* 404 译域照 Get 同款，提前拦 */ }
}
// ... Patch 成功（err == nil 即已提交）之后：
if old != nil && req.DeptName != nil {
    oldName := ""
    if old.DeptName != nil { oldName = *old.DeptName }  // 源 oldName 兜底 ""
    if *req.DeptName != oldName {
        if err := s.pub.Publish(NameChangedEvent{...}); err != nil { return nil, err }  // 500
    }
}
```

### Anti-Patterns to Avoid

- **goroutine 异步分发**：D-02 明文禁止——异步使 handler 失败脱离请求错误链，D-03 的"500 + 重试自愈"语义崩塌。
- **发布点放在 Patch 之前或事务内**：复刻源死锁形态（重点 2 证据）；单语句 Patch 返回即提交，发布必须在其后。
- **发布条件做成"无条件发布"**：源按值变化检测发布；无条件发布会让"仅改备注"也触发 26 条 UPDATE（realName 轴），违背 `!=` 幂等的设计初衷且浪费。
- **hand-rolled 异步队列/事件总线框架**：CLAUDE.md What NOT to Use 明文（watermill 等）；单机同步场景 50-80 行 slice of listeners 即够。
- **对 sys_user_role.username 列"顺手修正"语义**：realName handler 写展示名、username handler 写登录名，同列两值是源命名错位的原样移植（02 阶段已在 AssignRoles 处注释过一次）——handler 内必须再注释防混（CONTEXT Discretion 明文）。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 事务边界 | 手动 BEGIN/COMMIT/回滚 | GORM `db.Transaction(func(tx) error)` | 自动回滚语义既有（Phase 2 四连删先例） |
| 事件分发 | channel/队列/反射注册 | slice of listeners + map[type]（D-01） | 6 个静态事件类型，注册表直查最简且可 mock |
| NULL 比较语义 | Go 侧先查后比再绕过 NULL 行 | SQL `<> `三值逻辑（NULL 恒不中） | 数据库原生语义即验收要求，Go 侧模拟反而破坏原子性 |
| 变化检测 | GORM Hooks（BeforeUpdate 比对） | service 层显式 FindByID + 值比较 | hooks 拿不到请求语义（"是否显式传名"），且 Phase 1 已裁决绕开 Updates(map) 钩子 |

**Key insight:** 全部机制是"确定性 UPDATE × 6"，复杂度在映射清单的准确性（重点 1）与时序正确性（重点 2），不在基础设施——基础设施越薄越好。

## Common Pitfalls

### Pitfall 1: 发布点放错位置复刻源死锁
**What goes wrong:** 在 `db.Transaction` 闭包内 publish，handler 独立事务 UPDATE 与主事务争同一行锁（源 2026-09-29 交付机实测 50s 锁超时，RealNameSyncListener.java:85-87）。
**Why it happens:** 源 4 个名称监听器就是这个形态（事务内 publish + REQUIRES_NEW），照抄即踩。
**How to avoid:** 发布语句物理上放在 `repo.Patch`/`db.Transaction` 返回 nil 之后（代码顺序即提交顺序）；handler 内再开新事务。
**Warning signs:** 测试中出现 `Error 1205: Lock wait timeout`；或 handler UPDATE 卡住直至超时。

### Pitfall 2: 未回读旧行导致发布条件失真
**What goes wrong:** Go Update 方法当前不读旧行；若用"请求带了名字"当发布条件，仅改备注/状态的请求也会触发全表冗余同步；若漏判 `req.X != nil`，nil 解引用 panic。
**How to avoid:** 严格照源条件五处逐一移植（重点 2 发布条件清单）；oldName NULL 兜底 `""`（源同款）。
**Warning signs:** 改 remark 请求后 DB 冗余列 update_time 被意外批量刷新。

### Pitfall 3: `<>` 条件写成漏 NULL 判断的形态
**What goes wrong:** 若有人"优化"成 `WHERE (col <> ? OR col IS NULL)`，NULL 行会被更新，破坏源的 NULL 语义（NULL 冗余列代表"从未冗余过"，更新它会让无值行突然带值）——SYNC-02 的 NULL 锁定测试正是防此。
**How to avoid:** handler UPDATE 恒为 `col = id AND col <> newName` 三段式，无 IS NULL 分支；集成测试固化 NULL 行不被更新。
**Warning signs:** NULL 锁定测试失败；存量 NULL 冗余列行 count 异常。

### Pitfall 4: WR-08 修 envelope 不修 WR-07 导致测试红
**What goes wrong:** 只补 `Data` 层使断言真化，`assert.Nil(cleared.ParentID)` 立即失败（实际 `"0"`）。
**How to avoid:** WR-07（响应统一）与 WR-08（断言修真）同任务落地（重点 5）。
**Warning signs:** file 集成测试在"只修测试"提交上红。

### Pitfall 5: 构造签名变更漏改测试直构造点
**What goes wrong:** `NewUserService` 加 publisher 参数后，internal/service/user_integration_test.go:49 编译失败。
**How to avoid:** 计划任务明列该改动点；传 fake publisher（顺带成为 service 层发布条件的单测 mock 点）。
**Warning signs:** `go build ./...` 失败。

## Implementation Guidance

### 新增/修改文件清单

**新增：**
- `internal/event/event.go`——Type 常量（6 个，取源 eventType 字符串）+ `NameChangedEvent` + `Publisher` 接口
- `internal/event/dispatcher.go`——Register/Publish（同步、首错即返）
- `internal/event/listeners.go`——6 个 handler + 注册函数 `RegisterAll(d *Dispatcher, db *gorm.DB)`；realName handler 26 条 UPDATE 单事务；username handler 1 条（无发布点，见 Q1）
- `internal/event/listeners_test.go` 或 `internal/handler/name_sync_integration_test.go`——集成测试（沿 newTestApp + openCheckDB 先例）

**修改：**
- `internal/service/{dept,role,menu,dict,user}.go`——5 个 Update 加旧名回读 + 发布（Pattern 3）；5 个构造函数加 `pub event.Publisher`
- `internal/app/app.go`——dispatcher 构造 + RegisterAll + 5 个 service 构造注入（:155-172 区间）
- `internal/service/user_integration_test.go:49`——直构造点补参数
- `internal/service/{file_category,dept,menu}.go`——WR-07 方向 A：移根 `cols["parent_id"] = nil`（3 行 + 注释更新）
- `internal/handler/file_integration_test.go:348-353`——WR-08：断言补 `Data` envelope

### 同步 UPDATE 的落点（研究建议，Discretion 范围）

handler 直接持 `*gorm.DB` 用 `tx.Model(&po.XxxPO{}).Where(...).Update(col, val)`（Pattern 2），**不扩 repository**：26 个专用 repo 方法会膨胀 relation.go 且这些 UPDATE 是"冗余同步"非实体仓储语义；列名经代码常量、值经占位符，T-1-03 纪律不破。若 planner 偏好 repository 收口，可加一个通用 `SyncNameColumn(model, setCol, whereCol, id, newName)` 单方法（列名白名单由调用方常量收敛），二选一均可。

### 测试策略（NULL 锁定 + 幂等 + 发布条件三层）

1. **service 层单测**（fake Publisher）：改名请求→发布一次且载荷正确；仅改备注→零发布；新名==旧名→零发布；Patch 404→零发布。（沿 service/user_integration_test 直构造形态，无需容器）
2. **handler 集成测试**（testcontainer MySQL，沿 newTestApp/openCheckDB）：
   - 6 事件逐个：PUT 改名 → checkDB 断言波及表冗余列=新名
   - realName 全量面：用户改真名 → 14 表抽样断言（至少 user 双列、user_role.username（展示名语义！）、operation_log.real_name、一个 BaseWithOperator 域）
   - **NULL 语义锁定**：SQL 直插冗余列为 NULL 的行 → 改名 → 断言该行仍 NULL（`<>` 三值逻辑）
   - **幂等自愈**：手工把某行冗余列改陈旧 → 重放改名请求 → 断言被纠正（`<>` 命中）；再重放 → 无变化（`<>` 不中）
   - role 改名若采纳 sys_role_dept 扩展（Q2）→ 第 4 表断言
3. **回归门**：`go test ./...` 全绿（D-05）；8 项清单逐项在 VERIFICATION.md 记测试名/证据链。

### WR-07/08 具体修法

见重点 5。推荐方向 A：三域移根写 `parent_id = NULL`；file_integration_test 断言补 envelope 后 `assert.Nil(ParentID)` 真化；menu/dept 既有移根断言（仅 200）不受影响。

### Env/验收提醒

- `make verify-static` 等既有出厂门不受影响（无新依赖、无 CGO 面）。
- dispatcher 无状态、不持连接——SIGTERM 路径零新增清理责任（同步分发无后台 worker），App.Stop 不动。

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | 编译/测试 | ✓ | go1.27.3 | — |
| Docker | 集成测试 testcontainers（既有基建） | ✓ | 29.7.2 | — |
| MySQL 镜像（本地缓存） | RunMySQL 测试容器 | ✓（Phase 1 起在用） | 8.4.x | IAM_TEST_MYSQL_IMAGE 覆盖 |

**Missing dependencies with no fallback:** 无。

## Security Domain

`security_enforcement: true`（asvs=1）。本阶段无新认证/会话/权限面（D-04 明确不改会话；无新路由）。

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | —（本阶段不动认证链） |
| V3 Session Management | no | D-04：会话快照不刷新为显式决策，非遗漏 |
| V4 Access Control | no | 既有 requirePerm 链原样覆盖 5 个 Update 路由 |
| V5 Input Validation | yes | DTO binding 既有白名单；同步 UPDATE 值全参数化、列名为代码常量（T-1-03 纪律延伸） |
| V6 Cryptography | no | — |

| Threat Pattern | STRIDE | Mitigation |
|----------------|--------|------------|
| handler UPDATE 列名/值拼接注入 | Tampering | gorm 占位符 + 常量列名（Pattern 2 形态，禁 fmt.Sprintf 拼 SQL） |
| 事件载荷伪造（旧名注入） | Tampering | 载荷仅服务端构造（查库旧值+请求新值），无客户端直传字段；UPDATE WHERE 以 ID 为锚不依赖 OldName |

## Open Questions

1. **UsernameChangedEvent 处置（源死代码，SYNC-01 字面计 6 事件）**
   - What we know: 源从未发布该事件（重点 3）；Go username 无编辑路径；事件类+监听器在源齐备。
   - What's unclear: "6 个事件"验收按字面要 handler 形态齐备，还是按触发路径实现 5 个即可。
   - Recommendation: 实现 handler + 事件类型（~10 行，契约齐），user.Update 不设发布点；VERIFICATION.md 注明"无触发路径系源语义（username 为登录锚点不可改）"。规划时作为 checkpoint 向用户口头确认一次即可（不阻塞）。
2. **sys_role_dept.role_name 是否纳入 RoleNameChanged 同步面**
   - What we know: 该表是 Go 增量扩展（D-10，源不存在），role_name 冗余由 AssignScopes 写入（role.go:414）；源 RoleNameSync 只同步 role_menu+user_role 两表。
   - What's unclear: 不同步则该表冗余列在改名后陈旧（读侧谁消费它尚未逐一排查——当前仅存取 API 回显）。
   - Recommendation: 纳入（handler 第三条 UPDATE，同 `<>` 条件）——Go 内一致性强于"源没有此表"的字面对齐；plan 明文记录为 Go 扩展非源行为。
3. **清单第 8 项（SIGTERM 后日志完整落库）证据形态**
   - What we know: 组合证据已有（重点 6 #8）；缺"入队→真实 Stop→断言 DB 行"单测；IN-04（Stop 双 Flush）同面待 disposition。
   - Recommendation: 本阶段顺带补一个小集成测试（复用 newShutdownApp + olog 入队 + Stop + checkDB 断言），同时给 IN-04 一个 disposition——比纯文档证据链更硬，成本 ~40 行。
4. **WR-07 统一范围（仅 file_category vs 三域同改）**
   - What we know: 三域同款双形态（重点 5）；方向 A 每域 1 行。
   - Recommendation: 三域同改（方向 A），避免"同仓库两种根语义"残留；既有测试无形态锁定，零回归风险。

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | 源 MP updateById null-skip 使移根后 parent_name 残留、parent_id=0 正常写入（MyBatis-Plus 默认字段策略细节） | 重点 5 | 低——Go 侧行为已与此结论一致且为既有注释在案的有意修正；不影响本阶段修法 |
| A2 | 源全树无 @EnableAsync（grep 无命中即"同步分发"的判定） | 重点 2 | 低——即使存在异步配置，SYNC-02 已锁定 Go 侧同步语义，不受源影响 |
| A3 | 源 Create 根时 parent_id 列落 NULL（依据 MP insert 非 null 策略推断，未直读源 DDL 默认值） | 重点 5 | 低——统一方向 A 下 Go 行为不再依赖源此项 |
| A4 | sys_role_dept.role_name 当前无读侧消费者依赖实时性（仅 AssignScopes 写入与回显） | Q2 | 中——若前端列表页消费该冗余名做展示，陈旧窗口可见；但本就是冗余列，与源其他冗余列陈旧容忍度一致 |

## Sources

### Primary (HIGH confidence)
- `/home/ecs-user/workspace/sys-iam/sys-iam/src/main/java/com/wxinfo/system/iam/application/listener/{Dept,DictType,Menu,Role,RealName}SyncListener.java` — 全文直读（逐条 UPDATE 映射、注解时序、死锁注释）
- `/home/ecs-user/workspace/sys-iam/sys-iam/src/main/java/com/wxinfo/system/iam/domain/event/*.java` — 6 事件类全文
- `/home/ecs-user/workspace/sys-iam/sys-iam/src/main/java/com/wxinfo/system/iam/application/service/{Dept,Role,Menu,Dict,User,FileCategory}ApplicationService.java` — 发布点/事务注解/发布条件/移根行为直读
- `/home/ecs-user/workspace/sys-iam/sys-kernel/.../event/SpringDomainEventPublisher.java`、`domain/event/DomainEvent.java` — 发布机制
- 本仓库 `internal/service/{dept,role,menu,dict,user,file_category}.go`、`internal/repository/{relation,dept}.go`、`internal/model/po/*.go`、`internal/auth/session.go`、`internal/app/app.go` — 挂点/表名列名/装配/会话载荷全文直读
- 本仓库 `internal/handler/*_integration_test.go`、`internal/app/shutdown_integration_test.go`、`pkg/ids/id.go` — 测试映射直读

### Secondary (MEDIUM confidence)
- `.planning/phases/04-domains/04-REVIEW-DISPOSITION.md` — WR-07/08 原始记录
- `.planning/phases/05-sync-events/05-CONTEXT.md`、`.planning/REQUIREMENTS.md`、`.planning/STATE.md` — 决策/验收/挂账

### Tertiary (LOW confidence)
- MyBatis-Plus null-skip 字段策略细节（A1/A3 推断依据，社区通识未在本会话查证文档）

## Metadata

**Confidence breakdown:**
- 源映射与时序：HIGH —— 5 监听器 + 5 服务 + 发布器全部逐行直读，无推测成分
- Go 挂点与测试映射：HIGH —— 涉及文件全部本会话直读，行号在案
- WR-07/08 修复方向：HIGH —— 机理实测实证（REVIEW-DISPOSITION）+ 修复面代码直读
- 唯一 LOW 面：源 MP 策略细节推断（A1/A3），不影响修法

**Research date:** 2026-11-02
**Valid until:** 2026-12-02（稳定——源系统为冻结存量，Go 侧为本仓库自身）

## RESEARCH COMPLETE

**Phase:** 05 - 名称同步事件与验收收尾
**Confidence:** HIGH
**核心交付:** 源 5 监听器 → 6 事件 → 逐表逐列 WHERE 映射（realName=14 表 26 UPDATE 实证）；源两种发布时序并存 + 2026-09-29 死锁实测注释（SYNC-02 修正的源内背书）；UsernameChangedEvent 源内死代码发现（Go 同样无触发路径）；Go 5 挂点 + 2 波及面（构造签名/测试直构造）；WR-07/08 同任务修法与方向 A 建议；8 项清单 7 项已有测试、1 项组合证据缺口。
