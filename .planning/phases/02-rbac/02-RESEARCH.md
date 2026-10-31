---
phase: "02"
status: "complete"
confidence: high
---

# Phase 2: 认证会话与 RBAC 主链路 - Research

**Researched:** 2026-10-31
**Domain:** 源系统六域行为考古 + Redis token-session 设计 + MySQL schema 实证 + 测试基建扩展
**Confidence:** HIGH（全部关键结论均有源码文件:行号、本机 DDL 实证或本地可执行代证）

<user_constraints>
## User Constraints（from CONTEXT.md，planner 必须遵守）

### Locked Decisions（摘要，全文见 02-CONTEXT.md）
- 登录失败统一报"用户名或密码错误"（防枚举）；禁用报"用户已被禁用"；畸形 hash 返回登录失败而非 500
- apikeyLogin 沿源语义：key+ENABLED+未过期 → 关联用户登录；已有有效会话则复用同一 token（Redis 维护 apiKey→token 映射）
- token 传递 `Authorization: Bearer`；X-API-Key header 不实现
- 无账号锁定/密码强度/验证码/刷新令牌双机制（源均无）
- `auth.session.timeout` 缺省 30d（Nacos 值不可得，Sa-Token 官方默认值代理）+ `IAM_AUTH_SESSION_TIMEOUT` env 覆盖
- 活跃续期：认证中间件命中后 EXPIRE 重置 TTL 为完整 timeout；不做 idle+absolute 双超时
- 允许多端多 token 并存；`iam:user-tokens:{userId}` Set 容纳多 token
- 停用用户/重置密码 → 存量会话立即踢下线（DEL session + SREM 索引）——有意安全增强
- 会话 Hash 载荷：userId/userName/realName/roles（角色码列表）；permissions 快照不放（Phase 3 Casbin 取代）
- 路由：`/api/v1/{auth,users,roles,menus,depts,api-keys}`；用户角色分配 `PUT/GET /api/v1/users/:id/roles`（全量替换）
- 树端点：`GET /api/v1/menus/tree`、`GET /api/v1/depts/tree`；孤儿节点提升为根，菜单树复用同一构建器
- builtin 守卫：HTTP 409 + 业务错误码 + 中文消息（USER-07/ROLE-02 统一拦截器）
- 白名单收紧：`/api/v1/auth/login`、`/api/v1/auth/apikey-login`、`/healthz`、`/readyz` 四路径，其余全要求 token
- 导入契约：`POST /api/v1/users/import` → `{total, inserted, updated, failed, errors:[{row,reason}]}`；逐行独立 upsert；BodyLimit 1MB + 10000 行上限 + 表头按列索引严格匹配
- 生产 hash 抽样验证 → 现场部署清单；本阶段三源代证

### Claude's Discretion
- handler/service/repo 三层文件划分与测试组织（沿用 Phase 1 user 域模板）
- 会话管理器内部方法签名、Redis pipeline 使用时机
- Excel 列序常量与错误消息文案细节
- GORM 关联查询（user_role/role_menu）两查询防 N+1 的具体装配写法

### Deferred（OUT OF SCOPE）
- 生产库 20 条 hash 抽样（现场清单）、Nacos timeout 真值（现场校正）
- `/auth/userInfo`、`/auth/routes`、`/auth/refresh` 端点不实现
- 功能权限校验与数据权限（Phase 3）、操作日志（Phase 4）、名称同步（Phase 5）
- arm64 真机首启（现场清单）
</user_constraints>

## Key Findings

### 研究重点 1：源系统六域行为考古（置信度 HIGH，全部一手源码）

#### 1.1 Auth 认证（AuthApplicationService.java，204 行全读）

- **login 顺序与错误消息原文**：查用户（null → `用户名或密码错误`）→ 状态校验（非 ENABLED → `用户已被禁用`）→ BCrypt 校验（失败 → `用户名或密码错误`）[VERIFIED: AuthApplicationService.java:52-60]
- **apikeyLogin 精确分支**：
  1. `apiKeyRepository.getByApiKey(apiKey)` null → `API Key 无效或已过期`
  2. `userId == null` → `API Key 未关联用户`
  3. **复用条件**：`StpUtil.getSessionByLoginId(userId, false) != null && session.get("permissions") != null` → 直接 `StpUtil.login(userId)` 返回 token（跳过重查库装配）；否则查用户（`API Key 关联用户不存在` / `API Key 关联用户已被禁用`）走完整加载 [VERIFIED: AuthApplicationService.java:63-77]
  - CONTEXT.md 拍板 Go 版"复用同一 token + apiKey→token 映射"；源中 token 是否复用取决于 Sa-Token `is-share` 配置（Nacos 不可见 [ASSUMED]），CONTEXT.md 决策为权威
- **loadSessionData 装载语义**：user_role → role 两次查询取 roleCodes；`super_admin` 角色码（`SystemPermissions.SUPER_ADMIN_ROLE = "super_admin"` [VERIFIED: SystemPermissions.java:12]）豁免 → 全部菜单 permission 非空集合；否则按 roleIds 查菜单 permission。会话写入四键：`permissions`/`roles`/`username`/`realName`（realName 为 null 时不写该键）[VERIFIED: AuthApplicationService.java:138-171 + SessionKeys.java:11-14]。**Go 版按 CONTEXT.md：permissions 不放，roles 放**
- **源 refreshToken 细节（仅文档化，不实现）**：源代码是 `timeout = StpUtil.getTokenTimeout(); renewTimeout(timeout)`——**用剩余 TTL 作为续期值**（两次调用间微降，非重置满）[VERIFIED: AuthApplicationService.java:122-126]。PITFALLS.md Pitfall 10 的"重置为 timeout"描述与源码不符。由于 `/auth/refresh` 不实现（CONTEXT.md deferred）且活跃续期按 CONTEXT.md 拍板"重置为完整 timeout"，此差异无落地影响，但 planner 不得引用 Pitfall 10 该句作为 refresh 语义依据
- **用户上下文注入语义**：审计操作人姓名 = realName（非空白时）优先，回退 username [VERIFIED: SaTokenConfig.java:120-137]。Go `auth.Identity{Name}` 的填充须照此语义
- **过滤器错误映射**：NEED_LOGIN→401、ACCESS_DENIED→403、**其他异常兜底 HTTP 200 + 固定文案**（源遗留）；Go 版按 ENG-01 已定"HTTP 状态如实映射"，认证中间件统一真实 401/403 [VERIFIED: SaTokenConfig.java:96-104]

#### 1.2 User 域（UserApplicationService.java + UserController.java 全读）

| 行为 | 源证据 | Go 版要点 |
|---|---|---|
| addUser：deptId 必填 | null → `部门不能为空`；查不到 → `部门不存在: {id}` [VERIFIED: UserApplicationService.java:92-101] | 校验在 service 层，消息原文保留 |
| addUser 默认值 | status=ENABLED、firstLogin=true、builtin 缺省 false [VERIFIED: User.java:71-86] | PO 构造函数等价 |
| assignRoles 校验 | roleId 全量校验，缺失 → `角色不存在: [逗号拼接]`；UserRole 冗余写 roleName + username 列（**username 列实际存 realName 优先回退 username 的值**——源命名与内容不符，原样移植）[VERIFIED: UserApplicationService.java:108,150,182-201] | 逐条 INSERT（源非批量），Go 可批量化（行为增强允许） |
| editUser | `用户不存在: {id}`；不可改 username/password；builtin 仅显式传入才覆写；roleIds 非 null → 全删重建 [VERIFIED: UserApplicationService.java:121-154 + User.java:117-130] | repo.Patch + roleIds 分支 |
| updateStatus | 仅更新 status 列（MP null-skip 等价 = Patch 单列）；**源不踢会话**，Go 版按 CONTEXT.md 增强：停用即踢 [VERIFIED: UserRepositoryImpl.java:87-92] | Patch + SessionManager.Kickout |
| resetPassword | 固定 `BCrypt.hashpw("123456")` + firstLogin=true；无旧密码校验；用户不存在 → `用户不存在: {id}` [VERIFIED: UserApplicationService.java:162-167 + User.java:102-106] | 按 CONTEXT.md 增强踢会话 |
| removeById | builtin=true → `系统内置用户不可删除`（**零删除调用**，前置判断）；先删 user_role 再删用户 [VERIFIED: UserApplicationService.java:170-177] | HTTP 409 + errcode |
| selectPage 过滤 | username/realName/phone LIKE + status/deptId EQ + orderByDesc(createTime) [VERIFIED: UserRepositoryImpl.java:29-38] | Go 加 `, id DESC` tiebreaker（Phase 1 已定） |
| findByUsername | 精确 EQ；MP `getOne` 多行抛异常（uk_username 唯一索引保证不发生）[VERIFIED: UserRepositoryImpl.java:51-53] | First + ErrRecordNotFound |
| UserVo 装配（防 N+1） | `findRolesByUserIds(userIds)` IN 查询 → distinct roleIds → `role IN` 查询 → 内存装配 [VERIFIED: UserApplicationService.java:203-232] | USER-05 验收明文两查询 |
| UserVo 字段集 | User 全字段剔 password/extToken + roles:[{roleId,roleName}] [VERIFIED: UserVo.java:24-48] | dto 扩展 |

#### 1.3 Role 域

- addRole：dataScope 默认 1、builtin 缺省 false、status=ENABLED [VERIFIED: Role.java:37-46]
- editRole：`角色不存在: {id}`；withEdit 逐字段 null-skip + builtin 仅显式覆写 [VERIFIED: RoleApplicationService.java:77-94 + Role.java:52-70]
- removeById：`角色不存在: {id}` → builtin → `系统内置角色不可删除` → 三连删 user_role → role_menu → role（任一失败整体回滚）[VERIFIED: RoleApplicationService.java:100-108]
- assignMenus：`角色不存在: {roleId}` → 全删重建 → menuIds 逐个校验（`菜单不存在: [...]`）→ RoleMenu 冗余 roleName/menuName [VERIFIED: RoleApplicationService.java:111-137]
- getRoleMenuIds：role_menu 按 roleId 查 → menuId 列表 [VERIFIED: RoleApplicationService.java:140-145]
- **selectPage 特例**：roleName LIKE（**先 trim**）+ **orderByAsc(id)**——不是 createTime DESC，与其他域不同 [VERIFIED: RoleRepositoryImpl.java:28-34]。Go 版保持 orderByAsc(id) 语义（id 递增本身即稳定序）
- listAll：orderByAsc(id) [VERIFIED: RoleRepositoryImpl.java:37-39]

#### 1.4 Menu 域

- tree：`list()` orderByAsc(sort) → 服务层 buildTree [VERIFIED: MenuApplicationService.java:33-36 + MenuRepositoryImpl.java:27-29]
- add：parentId 非 null 且非 0 时校验父存在 → `父菜单不存在: {pid}`；冗余写 parentName；parentId=0/null 时 parentName=null [VERIFIED: MenuApplicationService.java:84-95]
- edit：`菜单不存在: {id}`；name 变更发事件（Phase 5）[VERIFIED: MenuApplicationService.java:53-73]
- delete：直接删，无子菜单/角色引用检查（源缺口按源保留）[VERIFIED: MenuApplicationService.java:75-77]
- MenuType 枚举：DIR(0,目录)/MENU(1,菜单)/BUTTON(2,按钮)，fromValue/fromInfo 未知值兜底 **MENU** [VERIFIED: MenuType.java]
- **树构建器死代码警告**：`Menu.java:63-76` 与 `Dept.java:44-57` 实体上的静态 buildTree 是**孤儿丢弃语义且全仓库零调用**（grep 证实 [VERIFIED: 命令输出]）；**实际生效的是三处服务层私有 buildTree（孤儿提升）**。Go 版必须照服务层语义实现，勿照实体死代码

#### 1.5 Dept 域

- 与 Menu 同构：tree/get/add（`上级部门不存在: {pid}` + parentName）/edit（`部门不存在: {id}` + 事件 Phase 5）/delete（无引用检查）[VERIFIED: DeptApplicationService.java 全文]
- list orderByAsc(sort) [VERIFIED: DeptRepositoryImpl.java:16-18]
- **孤儿提升语义实证**：服务层 buildTree `pid==null||0 → roots`；map 未命中 → `roots.add(dept)`——parentId 指向不存在节点时提升为根，与 DEPT-02 验收一致 [VERIFIED: DeptApplicationService.java:89-107]；Menu 服务层同构 [VERIFIED: MenuApplicationService.java:97-115]；Auth 服务层同构 [VERIFIED: AuthApplicationService.java:185-203]。**三处一致，Go 版单泛型构建器即可**

#### 1.6 ApiKey 域

- getByApiKey 登录查询：`api_key = ? AND status = ENABLED AND (expire_time IS NULL OR expire_time > now)` [VERIFIED: ApiKeyRepositoryImpl.java:45-50]
- addApiKey：key = `UUID.randomUUID().toString().replace("-","")`（32 hex）；userId 可选但非空必校验（`用户不存在: {id}`）→ username 冗余；status=ENABLED；**返回完整实体含明文 key**（一次性展示）[VERIFIED: ApiKeyApplicationService.java:30-47 + ApiKeyController.java:68-74]
- editApiKey：仅更新 status 列 [VERIFIED: ApiKeyApplicationService.java:49-54]
- 停用立即生效：登录仓储过滤 status（AKEY-02 验收）[VERIFIED: ApiKeyRepositoryImpl.java:48]

#### 1.7 枚举（三表示面在 Phase 2 的适用范围）

- UsageStatus：DISABLED(0,禁用)/ENABLED(1,正常)，未知兜底 ENABLED [VERIFIED: UsageStatus.java]
- Gender：UNKNOWN(0)/MALE(1)/FEMALE(2)，未知兜底 UNKNOWN [VERIFIED: Gender.java]
- Go 版 pkg/types 已实现（Phase 1），本阶段直接复用；**注意 fromInfo 仅 Excel 中文解析用，本阶段用户导入时启用**

### 研究重点 2：本机 MySQL schema 实证（置信度 HIGH，全部 SHOW CREATE TABLE 实证）

容器确认：`mysql-server (mysql:8.4.8)` Up 5h、`redis-server (redis:8.6.2)` Up 5h、`sysiam-nacos (rnacos)` **空实例**——craft-satoken.yaml 与配置列表均不存在（v1 API 查询 `config data not exist`，列表 totalCount=0）[VERIFIED: docker ps + curl 输出]。**Nacos timeout 缺口终局确认：30d 代理决策维持，无本地路径可解**

15 张表与本阶段相关的 7 张全部实证（注释乱码为 docker exec 字符集显示问题，列定义无碍）：

| 表 | 列数 | 关键列（原文） | 索引 |
|---|---|---|---|
| sys_user | 22 | id bigint PK / username varchar(50) / password varchar(100) / real_name / avatar / gender tinyint / phone / email / openid / ext_token varchar(512) / dept_id / dept_name / status tinyint DEFAULT '1' / first_login tinyint(1) / builtin tinyint(1) DEFAULT '0' / 审计4列 / create_time / update_time / remark varchar(500) | `UNIQUE uk_username`、`KEY idx_dept_id` |
| sys_role | 13 | id PK / role_name / role_code / data_scope tinyint NULL / status DEFAULT '1' / builtin DEFAULT '0' / 审计4列 / 时间2列 / remark | `UNIQUE uk_role_code` |
| sys_menu | 19 | id PK / parent_id / parent_name / name / type tinyint / path varchar(200) / component varchar(200) / permission varchar(100) / icon / sort int / visible tinyint(1) DEFAULT '1' / status DEFAULT '1' / 审计4列 / 时间2列 / remark | `KEY idx_parent_id` |
| sys_dept | 18 | id PK / parent_id / parent_name / dept_name / sort int / status DEFAULT '1' / 审计4列 / 时间2列 / remark | `KEY idx_parent_id` |
| sys_api_key | 13 | id PK / api_key varchar(100) / username / user_id / expire_time datetime / status DEFAULT '1' / 审计4列 / 时间2列 / remark | `UNIQUE uk_api_key`、`KEY idx_user_id` |
| sys_user_role | 8 | id PK / user_id / username / role_id / role_name / create_time / update_time / remark | `KEY idx_user_id`、`KEY idx_role_id` |
| sys_role_menu | 8 | id PK / role_id / role_name / menu_id / menu_name / create_time / update_time / remark | `KEY idx_role_id`、`KEY idx_menu_id` |

**DDL 缺列结论：无缺列。** 源 PO 全部字段在本库均有对应列；关联表（user_role/role_menu）无审计人四列（BaseAggregate 等价，仅 id+时间+remark），Go PO 用独立小基类或直接字段，勿复用 BaseWithOperator

**[VERIFIED: SHOW CREATE TABLE 全部 7 表，2026-10-31 本机容器]**

**存量数据（测试 fixture 直接可用）** [VERIFIED: SELECT 输出]：
- sys_user 2 行：`admin`（id=1，builtin=1，**BCrypt `$2a$10$` 60 字符标准 hash**）、`arm64-uat-1793449338`（**password='Arm64@U' 10 字符畸形值——畸形 hash 路径的天然活 fixture**）
- sys_role 1 行：`超级管理员/super_admin`（builtin=1，**data_scope=NULL**——create 默认 1 只影响新建行）
- sys_menu 41 行、sys_dept 1 行、sys_api_key 0 行

**collation（STATE.md blocker 解答）**：表级与列级均为 `utf8mb4_general_ci` [VERIFIED: SHOW CREATE TABLE]。影响：username/role_code 等唯一索引**大小写不敏感**（`Admin` 与 `admin` 冲突；findByUsername 精确匹配也 ci）。Go 侧无需处理（DB 决定），但集成测试断言不得假设大小写敏感

**STATE.md blocker"Phase 2 规划前向用户索取 Nacos timeout/MySQL collation"**：collation 已实证（上）；timeout 终局确认不可得（rnacos 空）——两项 blocker 均已闭环，无需用户输入

### 研究重点 3：Redis 会话实现细节（置信度 HIGH）

版本验证 [VERIFIED: `go list -m -versions` via proxy.golang.org，2026-10-31]：
`redis/go-redis/v9 v9.23.0`（最新）、`alicedbob/miniredis/v2 v2.40.0`（最新）、`xuri/excelize/v2 v2.11.0`（最新）、`golang.org/x/crypto v0.58.0`（最新；go.mod 现 indirect v0.54.0，Phase 2 `go get` 提升为直接依赖）、`testcontainers-go/modules/redis v0.44.0`（与已用 mysql 模块同版本）

go-redis v9 关键语义 [CITED: context7 /redis/go-redis]：
- `TxPipelined(ctx, func(p redis.Pipeliner) error)`：MULTI/EXEC 单往返原子块——登录写 session+SADD 索引、登出 DEL+SREM 均应用它
- `HGetAll` 对不存在 key 返回**空 map + nil error**（非 redis.Nil）；`Get` 对不存在 key 返回 `redis.Nil` error——两个分支判空方式不同，易踩
- `HExpire`（字段级 TTL）存在但不需要（整 key TTL 即可）
- `Watch` 乐观锁模式存在但本设计不需要（无 read-modify-write 竞态）

#### Redis Key 设计（伪代码级，CONTEXT.md 决策的落地形态）

| key | 类型 | TTL | 写入点 | 删除点 |
|---|---|---|---|---|
| `iam:session:{token}` | Hash：`userId`(string int64)、`userName`、`realName`、`roles`(JSON string) | timeout；**每次中间件命中 EXPIRE 重置为完整 timeout**（AUTH-05） | login（TxPipelined） | logout / kickout / 停用用户 / 重置密码 |
| `iam:user-tokens:{userId}` | Set of token | **不设 TTL**（多 token 各自寿命不同；惰性清理） | login SADD | logout SREM / kickout 全清 |
| `iam:apikey-token:{apiKey}` | String = token | timeout（与 session 同寿命） | apikeyLogin 完整登录后 SET | **不主动删**——自然过期；指向死 token 时由复用校验分支兜底 |

```go
// ---- 登录（密码路径）----
token := randToken() // crypto/rand 32 字节 → hex.EncodeToString（64 hex，256-bit 熵）
_ = rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
    p.HSet(ctx, "iam:session:"+token, map[string]any{
        "userId": strconv.FormatInt(userID, 10),
        "userName": userName, "realName": realName,
        "roles": string(mustJSON(roles)), // ["super_admin","admin"]
    })
    p.Expire(ctx, "iam:session:"+token, timeout)
    p.SAdd(ctx, "iam:user-tokens:"+strconv.FormatInt(userID, 10), token)
    return nil
})

// ---- 中间件校验 + 活跃续期（AUTH-05/06）----
vals, err := rdb.HGetAll(ctx, "iam:session:"+token).Result()
if err != nil { return 500 }        // Redis 故障 → 500（不静默放行）
if len(vals) == 0 { return 401 }    // 会话不存在/已过期
_ = rdb.Expire(ctx, "iam:session:"+token, timeout) // 重置为完整 timeout
identity := auth.Identity{UserID: parse(vals["userId"]), Name: realNameOrFallback(vals)}
c.Set(identity 进 context) → c.Next()

// ---- 登出（AUTH-03：即刻失效）----
rdb.Del(ctx, "iam:session:"+token)
rdb.SRem(ctx, "iam:user-tokens:"+userID, token)

// ---- 踢下线（AUTH-04：userId→token 反查）+ 惰性清理 ----
for _, tk := range rdb.SMembers(ctx, "iam:user-tokens:"+userID).Val() {
    if rdb.Exists(ctx, "iam:session:"+tk).Val() == 1 {
        rdb.Del(ctx, "iam:session:"+tk)          // 真会话删除
    }
    rdb.SRem(ctx, "iam:user-tokens:"+userID, tk) // 死 token 也从索引清掉
}

// ---- apikeyLogin 复用分支（CONTEXT.md 拍板）----
tk, err := rdb.Get(ctx, "iam:apikey-token:"+apiKey).Result()
if err == nil && rdb.Exists(ctx, "iam:session:"+tk).Val() == 1 {
    return tk // 复用同一 token（中间件后续校验自然续期）
}
// 未命中/死 token → 完整登录路径，成功后：
rdb.Set(ctx, "iam:apikey-token:"+apiKey, token, timeout)
```

**语义澄清（对照 CONTEXT.md）**：
- "停用用户 → 立即踢下线"复用 Kickout(userId)；"重置密码 → 立即踢下线"同
- user-tokens Set 不设 TTL 是有意取舍：Sa-Token 同结构也有死键堆积问题，靠惰性清理；Set 本身无敏感数据
- apikey-token 键不主动删：登出/踢下线时无法从 token 反查 apiKey（无反向索引），该键残留至 TTL 到期；期间 GET 到它但 session 已死 → 复用分支 EXISTS 检查兜底走完整登录并覆盖该键——**无正确性风险，仅有 TTL 窗口内的无害残留键**

### 研究重点 4：Excel 导入——高危冲突置顶（置信度 HIGH）

> **DDL 漂移/缺失类高危发现：REQUIREMENTS.md USER-06 与 ROADMAP Phase 2 成功标准、CONTEXT.md 拍板中的"用户 Excel 导入三键 upsert（name+bitIndex+location）"在源系统与本库 schema 上均不成立。**

三方证据：
1. **源系统 User 域无导入端点**：UserController.java 全文 167 行仅 list/get/listByIds/add/edit/delete/resetPwd/status 八个端点，无 import [VERIFIED: UserController.java:86-166]
2. **sys_user 表无 bit_index/location 列**：22 列全量列出，无此两列；schema 沿用禁改（PROJECT.md Out of Scope）[VERIFIED: SHOW CREATE TABLE sys_user]
3. **三键语义属于 FaultInfo 域**：`name+bitIndex+location` 是故障库 PLC 位号三键 [VERIFIED: FaultInfoApplicationService.importExcel:63]；ROADMAP Phase 4 自己写的是"故障信息 CRUD 与 Excel 导入（name+bitIndex+location 三键 upsert）"——Phase 2 的 USER-06 是起草时把 Phase 4 语义复制错域

**与 CONTEXT.md 冲突的处置**（CONTEXT.md 优先原则）：CONTEXT.md 中**可落地的部分全部保留**——导入端点契约（`POST /api/v1/users/import`、`{total, inserted, updated, failed, errors:[{row,reason}]}`）、逐行独立 upsert 不回滚整批、行级错误、BodyLimit 1MB + 10000 行上限 + 表头按列索引严格匹配。**仅"三键"修正为 username 单键**：uk_username 唯一索引是天然 upsert 键 [VERIFIED: sys_user DDL]，全匹配 → UPDATE（by username）否则 INSERT；缺 username 的行跳过并记行级错误。此修正属物理不可实现下的最小变更，planner 在 PLAN 中显式记录该偏差与理由

**Excel 列契约建议**（源 User @ExcelProperty 中文表头子集，按列索引严格匹配）[VERIFIED: User.java:20-64]：
`用户名`(必填,upsert 键) / `真实姓名` / `性别`(男|女|未知 → Gender.fromInfo 未知兜底 UNKNOWN) / `手机号` / `邮箱` / `部门ID`(非空时校验部门存在，复用 addUser 校验链) / `角色`(可选,角色编码逗号分隔)——密码列不进导入模板（导入用户统一走 resetPwd 语义或首登强制改密；避免明文密码进 Excel 文件的安全面）。`状态` 列省略（导入默认 ENABLED，与 User.create 一致）

excelize 注意点 [CITED: context7 /xuri/excelize-doc cell.md]：
- `GetRows(sheet, opts)` 返回 `[][]string`，**尾部连续空 cell 被跳过 → 行长度不一致**，按列索引取值必须做越界保护
- 数字单元格默认返回格式化串（`123456.0`/科学计数法风险）；部门ID 等数值列用 `excelize.Options{RawCellValue: true}` 读原始值后清洗
- 打开文件 `excelize.OpenReader`/`Close` 必须 defer（PITFALLS Pitfall 12/16）

### 研究重点 5：测试基建适配（置信度 HIGH）

**推荐分层**（承接 CLAUDE.md 分层与 Phase 1 先例）：
- **单元测试（miniredis v2.40.0）**：SessionManager 全部逻辑——登录/续期/登出/踢下线/复用分支。miniredis 纯 Go 免 Docker，`RunT(t)` 一行启动；**TTL 不会自动流逝**，用 `m.SetTTL(k, d)` + `m.FastForward(d)` 手动推进时间，可确定性断言"EXPIRE 重置为完整 timeout"与"过期后会话消失" [CITED: context7 /alicebob/miniredis README]
- **集成测试（testcontainers redis 模块 v0.44.0 + mysql 模块）**：HTTP 全链路契约。API 与 mysql 模块同构：`redismodule.Run(ctx, img)` → `ConnectionString(ctx)` [CITED: testcontainers-go docs modules/redis.md]。镜像离线自适应：本地已缓存 `redis:8.6.2` [VERIFIED: docker images]，复刻 Phase 1 `testImage()` 模式（`IAM_TEST_REDIS_IMAGE` env > 本地 `redis:8.x` 探测 > `redis:8` 兜底）；ryuk 禁用逻辑沿用
- MySQL 侧集成测试容器沿用 Phase 1 RunMySQL 不变

**Phase 1 基建缺口（Wave 0 需补）**：
1. `scripts/ddl/` 只有 sys_user.sql——**需补 6 张表 DDL**（sys_role/sys_menu/sys_dept/sys_api_key/sys_user_role/sys_role_menu），可从本机库 `mysqldump --no-data --skip-comments` 提取后手工清理注释 [VERIFIED: scripts/ddl/ 目录列表]
2. `RunMySQL` 只执行单 DDL 文件——需扩展为执行 DDL 目录全部 .sql（或合成单文件）
3. `app.New` 需新增 Redis client 构造与 SessionManager 注入；`config.Config` 需新增 `Redis`/`Auth` 配置段
4. `newTestApp` 需同时启动 MySQL + Redis（集成）或注入 miniredis 地址（契约测试免 Docker 分支可评估，推荐集成测试统一双容器）

### 研究重点 6：Phase 1 模板复用清单（每文件 → 模板源）

| 新文件 | 复制模板 | 适配点 |
|---|---|---|
| `internal/model/po/role.go` | po/user.go | 13 列；role_name/role_code/data_scope/status/builtin |
| `internal/model/po/menu.go` | po/user.go | 19 列；parent_id/parent_name/name/type/path/component/permission/icon/sort/visible/status |
| `internal/model/po/dept.go` | po/user.go | 18 列 |
| `internal/model/po/api_key.go` | po/user.go | 13 列；api_key/username/user_id/expire_time/status |
| `internal/model/po/relation.go` | po/base.go 直加字段 | user_role/role_menu 两 PO：id+业务4列+create_time/update_time/remark，**不含 BaseWithOperator**（无审计人列） |
| `internal/repository/role.go` 等 4 个 | repository/user.go | Patch/List/FindByID 直套；List 增加 Where 过滤闭包（对齐源 LIKE/EQ 条件）；Role 的 Order 改 `id ASC` |
| `internal/repository/relation.go` | （新形态，薄） | SAdd/SRemByUserID/SRemByRoleID/FindByUserIDs/FindByRoleID；批量插入 CreateIn(tx, rows) |
| `internal/session/manager.go` | （新包，无模板） | 研究重点 3 伪代码 |
| `internal/middleware/auth.go` | middleware/access_log.go 形态 + errcode | 白名单路径表 + Bearer 解析 + Resolve + 续期 + Identity 注入 |
| `internal/handler/auth.go` | handler/user.go | login/apikey-login/logout 三端点 |
| `internal/handler/{role,menu,dept,api_key}.go` | handler/user.go | 五域 REST |
| `internal/handler/user.go`（扩展） | 自身 | +roles 子资源 +import +status +resetPwd |
| `internal/service/auth.go` | service/user.go | 登录编排（源六步链） |
| `internal/service/{role,menu,dept,api_key}.go` | service/user.go | 含树构建调用 |
| `internal/tree/tree.go`（或 internal/service 内共享） | （新，泛型 30 行） | 孤儿提升单构建器：`BuildTree[T](items, idOf, parentIDOf, setChildren)` |
| `internal/model/dto/{auth,role,menu,dept,api_key}.go` | dto/user.go | 指针 DTO + ids.ID + types 枚举 |
| `internal/testutil/redis.go` | testutil/mysql.go | testImage 复刻 + RunRedis |
| `configs/config.yaml` + `internal/config/config.go` | 自身 | +`redis.addr`（默认 127.0.0.1:6379）/+`auth.session.timeout`（默认 720h）/+ env alias `IAM_AUTH_SESSION_TIMEOUT`/`IAM_REDIS_ADDR`（多段 snake_case 键须登记 envKeyAliases——config.go:89-98 既有规则） |

既有直接复用（无改动）：pkg/resp、pkg/errcode（新增 409 段错误码与登录特定错误码见 Implementation Guidance）、pkg/paging、pkg/ids、pkg/types、internal/auth/identity.go、internal/testutil/mysql.go

## Implementation Guidance

### 表清单（7 张，全部已实证无缺列）
sys_user（已有 PO）、sys_role、sys_menu、sys_dept、sys_api_key、sys_user_role、sys_role_menu

### 文件清单（见研究重点 6 表格，共约 20 个新文件 + 6 处既有文件扩展）

### REST 端点全集（CONTEXT.md 路由决策 + 源端点映射）

| 方法路径 | 源对应 | 备注 |
|---|---|---|
| POST `/api/v1/auth/login` | /auth/login | body {username,password} → {token,user} |
| POST `/api/v1/auth/apikey-login` | /auth/apikeyLogin | body {apiKey} → {token,user} |
| POST `/api/v1/auth/logout` | /auth/logout | 需登录 |
| GET/PUT `/api/v1/users/:id/roles` | （新端点，CONTEXT.md 拍板） | 全量替换语义；GET 返回角色列表 |
| POST `/api/v1/users/import` | （新端点，CONTEXT.md 拍板） | multipart xlsx；username 单键 upsert（冲突修正） |
| PUT `/api/v1/users/:id/status`、`/api/v1/users/:id/reset-password` | /user/status、/user/resetPwd/{id} | 停用/重置均踢会话（增强） |
| `/api/v1/users` CRUD（已有，扩展角色装配） | /user/* | list 响应升级为 UserVo+roles（两查询防 N+1） |
| `/api/v1/roles` CRUD + GET `/api/v1/roles/:id/menus` + PUT `/api/v1/roles/:id/menus` | /role/* | 分配菜单全删重建 |
| `/api/v1/menus` CRUD + GET `/api/v1/menus/tree` | /menu/* | tree 孤儿提升 |
| `/api/v1/depts` CRUD + GET `/api/v1/depts/tree` | /dept/* | 同构 |
| `/api/v1/api-keys` CRUD（含 status） | /apiKey/* | add 返回含明文 key |

### 错误码增量（pkg/errcode 分段规则内）
- builtin 冲突：HTTP 409 + 新码（如 10600 段，`系统内置用户不可删除`/`系统内置角色不可删除`）
- 登录失败沿用 CodeBadCredentials(10501)；`用户已被禁用` 可复用 10501 语义或独立码——**推荐独立码但 HTTP 同 401**，消息原文区分（源即区分消息）
- 未登录 CodeUnauthenticated(10100)/401、无权限 CodeForbidden(10200)/403（已有）

### 测试策略

单元（无 Docker，秒级）：
- `internal/session/manager_test.go`（miniredis）：TTL 重置断言、登出即失效、kickout 清索引+惰性清理、apikey 复用/死 token 分支、畸形 hash→登录失败
- `internal/tree` 构建器：孤儿提升/parentId=0/null/循环父引用安全（map 不命中即根，天然无死循环）
- pkg/types 枚举兜底回归

集成（testcontainers 双容器，`-tags=integration`）：
- 认证全链路：登录→带 token 访问→登出→401；白名单四路径免鉴权；无 token 全拒
- 存量 hash 兼容：**用本库 admin 真实 hash（`$2a$10$7JB720yubVSZvUI0rEqK/.VqGOZTH.ulu33dHOiBE8ByOhJIrdAu2`，明文 admin123）作 fixture**——已在研究阶段实测 Go bcrypt 验证通过 [VERIFIED: go run 实测，2026-10-31]；**用本库 arm64 用户畸形值 `Arm64@U` 作畸形路径 fixture**（登录失败非 500）
- 五域 CRUD 契约、树形状、builtin 409、分页钳位、两查询装配、Excel 导入（构造 xlsx fixture：正常行/缺键行/未知部门行/坏枚举行）

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.12.1（既有） |
| Config file | 无（go test 原生；integration tag 区分层） |
| Quick run command | `go test ./...`（单元，无 Docker） |
| Full suite command | `go test -tags=integration ./...`（需 Docker 本地镜像） |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command |
|--------|----------|-----------|-------------------|
| AUTH-01 | 登录+存量 hash+畸形 hash | unit + integration | `go test ./internal/service/ -run TestLogin`、`go test -tags=integration ./internal/handler/ -run TestAuth` |
| AUTH-02 | apikey 登录含复用分支 | unit + integration | `go test ./internal/session/ -run TestApiKeyReuse` |
| AUTH-03 | 登出即刻失效 | unit + integration | `go test ./internal/session/ -run TestLogout` |
| AUTH-04 | 踢下线反查索引 | unit | `go test ./internal/session/ -run TestKickout` |
| AUTH-05 | TTL+活跃续期重置满 | unit | `go test ./internal/session/ -run TestRenew`（miniredis FastForward） |
| AUTH-06 | 白名单+其余全拒 | integration | `go test -tags=integration ./internal/middleware/ -run TestAuthWhitelist` |
| USER-01..07 | 五域行为 | integration | `go test -tags=integration ./internal/handler/ -run TestUser` |
| ROLE-01..03 | 角色域 | integration | 同上 `-run TestRole` |
| MENU-01..02 | 菜单域+树 | integration + unit（构建器） | `-run TestMenu`、`go test ./internal/tree/` |
| DEPT-01..02 | 部门域+孤儿提升 | integration + unit | `-run TestDept` |
| AKEY-01..02 | ApiKey 域+停用不可登录 | integration | `-run TestApiKey` |

### Sampling Rate
- Per task commit: `go test ./...`
- Per wave merge: `go test -tags=integration ./...`
- Phase gate: 全套绿 + UAT

### Wave 0 Gaps
- [ ] `scripts/ddl/sys_role.sql` + 5 张表 DDL（从本机库导出）
- [ ] `internal/testutil/redis.go`（镜像离线自适应）
- [ ] `internal/session/` 包骨架 + `config.go` Redis/Auth 段
- [ ] go.mod：go-redis v9.23.0、miniredis v2.40.0、excelize v2.11.0、x/crypto v0.58.0（直接依赖化）

## Security Domain（本阶段为认证核心，必须覆盖）

| 威胁 | 缓解（源语义/已定决策） |
|---|---|
| 用户枚举 | 登录失败统一文案（源行为，CONTEXT.md 锁定） |
| 暴力破解 | 源无锁定机制——不发明（CONTEXT.md）；工控机内网边界 |
| 畸形存量 hash 致 500/DoS | VerifyPassword 封装：任何 bcrypt error → false + 服务端日志（Pitfall 7；本库有真实畸形样本） |
| token 弱熵 | crypto/rand 32 字节（256-bit），禁 uuid v4（122-bit 亦可但标准库 crypto/rand 足够且无依赖） |
| 会话固定/重放 | 登录必发新 token；token 仅经 Bearer header；Redis TTL 滚动 |
| 越权（横向） | 所有非白名单路径要求有效 token（AUTH-06）；资源级属主校验本阶段不引入（管理面语义与源一致） |
| 密码/凭据出网 | password/extToken 永不进响应（Phase 1 契约已有）；apiKey 明文仅 add 响应一次性返回（源语义）；日志不落 token/apiKey 明文（Phase 4 掩码中间件未上，本阶段 slog 避免打印敏感键） |
| SQL 注入 | GORM 占位符（既有约定） |
| 请求体 DoS | BodyLimit 1MB（既有）+ 导入 10000 行上限（CONTEXT.md） |
| ASVS 对应 | V2 密码验证（bcrypt cost 10）、V3 会话管理（服务端会话+TTL+失效）、V4 访问控制（中间件拒未认证）、V5 输入校验（binding+指针 DTO）——L1 目标与 Phase 1 一致 |

## Open Questions

1. **USER-06 三键 upsert 不可实现的最终确认**（高危，见研究重点 4）
   - 已知：源无端点、表无列、三键属 FaultInfo；CONTEXT.md 导入契约其余部分可落地
   - 建议：planner 按 **username 单键 upsert** 规划（uk_username 天然唯一键），PLAN 中显式记录对 CONTEXT.md 的最小偏差与理由；自主授权模式下无需阻塞等用户，但 UAT 时向用户提示此修正
2. **user-tokens Set 惰性清理 vs 全量重建**：研究建议惰性清理（kickout 时顺带 SREM 死 token）；若 planner 希望更简单，可在 Resolve 失败时跳过清理（Set 残留无害），两者均正确——PLAN 定一处即可
3. **`用户已被禁用` 是否复用 CodeBadCredentials(10501)**：源消息可区分；若复用 10501 则响应 code 相同仅 message 不同。建议独立错误码（10502 段）保持 errcode 语义清晰，HTTP 均 401——PLAN 定夺
4. **Excel 导入密码语义**：研究建议导入不设密码列、导入用户走"首次登录强制改密"（firstLogin=true + 统一初始密码 resetPwd 语义）。若用户期望 Excel 带密码列（源 @ExcelProperty 有"密码"列），需在 PLAN 中向用户 checkpoint——低风险，按研究建议先行
5. **并发会话上限**：源/CONTEXT.md 均未设上限；Set 无限增长理论可堆积（工控机单机低风险）。不处理，保持与源一致

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | 全部 | ✓ | 1.27.3（go.mod） | — |
| Docker | 集成测试 | ✓ | 本机 daemon 在跑 | — |
| mysql:8.4.8 本地镜像 | MySQL testcontainer | ✓ | 8.4.8 | IAM_TEST_MYSQL_IMAGE 覆盖 |
| redis:8.6.2 本地镜像 | Redis testcontainer | ✓ | 8.6.2 | IAM_TEST_REDIS_IMAGE 覆盖（Wave 0 建） |
| testcontainers/ryuk:0.8.1 | 容器回收 | 本地有但默认禁用 | 0.8.1 | cleanup 显式终止（Phase 1 先例） |
| 本机 redis-server:6379 | 手工联调（可选） | ✓ | 8.6.2 无密码 | 测试不锚定本机容器（D-10 铁律） |

**Missing dependencies with no fallback:** 无——全部就绪。

## Sources

### Primary（HIGH）
- 源码逐文件阅读（本会话）：`/home/wx/workspace/sys-iam/sys-iam/src/main/java/com/wxinfo/system/iam/application/service/{AuthApplicationService,UserApplicationService,RoleApplicationService,MenuApplicationService,DeptApplicationService,ApiKeyApplicationService}.java`、`interfaces/controller/{Auth,User,Role,Menu,Dept,ApiKey}Controller.java`、`domain/entity/{User,Role,Menu,Dept,ApiKey,UserRole,RoleMenu}.java`、`infrastructure/persistence/{UserRepositoryImpl,RoleRepositoryImpl,MenuRepositoryImpl,DeptRepositoryImpl,ApiKeyRepositoryImpl}.java`、`application/dto/{UserQuery,RoleQuery,UserVo,UserRoleVo,ImportResult}.java`、`sys-kernel/.../config/{SaTokenConfig,SaWhiteListProperties,AutoFillHandler}.java`、`shared/constant/{SystemPermissions,SessionKeys}.java`、`shared/context/UserContext.java`、`infrastructure/persistence/PageHelper.java`、`shared/enums/UsageStatus.java`、`domain/enums/{Gender,MenuType}.java`
- 本机 MySQL 实证（本会话命令输出）：docker exec mysql-server SHOW TABLES / SHOW CREATE TABLE ×7 / 存量 SELECT
- 本机代证（本会话 go run）：Go x/crypto/bcrypt 验证开发库 admin 真实 hash（明文 admin123）成功；畸形值 `Arm64@U`/`$2a$10$short`/空串均返回 error（须译为登录失败）；Go 生成 hash `$2a$` 60 字符与存量同格式
- Go module proxy（本会话 `go list -m -versions`）：go-redis v9.23.0 / miniredis v2.40.0 / excelize v2.11.0 / x/crypto v0.58.0 / testcontainers modules/redis v0.44.0
- 本机 rnacos 实证：craft-satoken.yaml 不存在、配置列表空（timeout 缺口终局闭环）

### Secondary（MEDIUM-HIGH）
- Context7 `/redis/go-redis`（v9.23.0）：TxPipelined/HGetAll 空 map 语义/redis.Nil/HExpire/Watch
- Context7 `/alicebob/miniredis`：RunT/TTL 不自动流逝/FastForward/SetTTL
- Context7 `/xuri/excelize-doc`：GetRows 行长不一/RawCellValue/Close 纪律
- Context7 `/testcontainers/testcontainers-go`：redis 模块 Run/ConnectionString/WithConfigFile

### Prior Research（复用，本会话核对）
- `.planning/research/PITFALLS.md`（Pitfall 3/4/7/8/10/12/14/15/17 全部适用本阶段；Pitfall 10 的 refreshToken 描述与源码有出入已在上文修正）
- `.planning/research/FEATURES.md`（六域行为基准，本会话逐条复核一致）
- `.planning/phases/01-foundation/01-RESEARCH.md`（镜像离线自适应先例）
- `02-CONTEXT.md`（全部决策逐条对照源码核验通过，仅 USER-06 三键一项物理不可实现已置顶）

## Metadata

**Confidence breakdown:**
- 源行为考古：HIGH — 全部一手源码文件:行号引用，无推测
- Schema 实证：HIGH — 本机容器 SHOW CREATE TABLE 直接输出
- Redis 设计：HIGH — CONTEXT.md 决策 + go-redis 官方文档核对 + 伪代码可直接转实现
- Excel 冲突判定：HIGH — 三方证据（源码/DDL/ROADMAP 原文）互相印证
- 测试基建：HIGH — Phase 1 既有先例 + 本地镜像实证

**Research date:** 2026-10-31
**Valid until:** 2026-11-30（稳定；schema 与源码均为静态移植基准）
