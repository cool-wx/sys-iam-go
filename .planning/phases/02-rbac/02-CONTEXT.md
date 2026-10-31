# Phase 2: 认证会话与 RBAC 主链路 - Context

**Gathered:** 2026-10-31
**Status:** Ready for planning

<domain>
## Phase Boundary

用户可登录并获得会话（存量密码零迁移），管理员可管理用户/角色/菜单/部门/API Key 全套 RBAC 主链路数据。具体交付：

- **认证与会话**：自建 token-session 管理器（crypto/rand 32B token + Redis Hash 会话 + user-tokens 反查索引），登录（密码 + API Key 两路径）、登出、踢下线、TTL 活跃续期、认证中间件与白名单
- **五域 CRUD**：User / Role / Menu / Dept / ApiKey 全套（含角色分配/回收、菜单树/部门树查询；用户 Excel 导入已随 USER-06 删除移除——源系统 User 域无导入端点，三键语义属 Phase 4 FAUL-02）
- **不含**：功能权限校验与数据权限（Phase 3 Casbin）、操作日志（Phase 4）、名称同步（Phase 5）

需求项：AUTH-01..06、USER-01..07、ROLE-01..03、MENU-01..02、DEPT-01..02、AKEY-01..02（共 23 项）

</domain>

<decisions>
## Implementation Decisions

> 依据 2026-10-31 用户全量自主授权，本阶段灰区由 Claude 自主拍板（与 Phase 1 先例一致）；所有决策含理由留档，用户可随时复审推翻。

### 认证契约（Claude 自主决策，研究文档支撑）

- **D-01:** `POST /api/v1/auth/login {username,password}` → `{token, user}`；password/extToken 等敏感字段绝不回传
- **D-02:** 登录失败统一报"用户名或密码错误"，不区分用户不存在/密码错误（防用户枚举，源语义 FEATURES.md:22）；禁用用户报"用户已被禁用"；畸形 hash 返回登录失败而非 500（PITFALLS.md Pitfall 7）
- **D-03:** apikeyLogin 沿源语义（FEATURES.md:23）：按 key+ENABLED+未过期查 ApiKey → 关联用户登录；关联用户已有有效会话则复用同一 token（Redis 维护 apiKey→token 映射）
- **D-04:** token 传递：`Authorization: Bearer <token>` 标准方式；X-API-Key header 方式不实现（研究反特性清单 FEATURES.md:219——源仅定义常量无消费方）
- **D-05:** 无账号锁定、无密码强度规则、无验证码、无刷新令牌双机制（源系统均无，不发明）

### 会话参数与生命周期（Claude 自主决策，处置 Nacos 缺口）

- **D-06:** timeout 缺省 30d：源 Nacos craft-satoken.yaml 实际值不可得（STATE 待办），以 Sa-Token 官方默认值（30 天）为最接近代理；落地为 `auth.session.timeout` 配置项 + `IAM_AUTH_SESSION_TIMEOUT` env 覆盖，现场可校正——缺口在 PLAN 中文档化
- **D-07:** 活跃续期：认证中间件每次校验命中后 `EXPIRE` 重置 TTL 为完整 timeout（滚动会话，AUTH-05）；不实现 idle+absolute 双超时（源未启用 active-timeout 窗口语义）
- **D-08:** 并发会话：允许多端多 token 并存（Sa-Token 默认，与源一致）；`iam:user-tokens:{userId}` Set 容纳多 token
- **D-09:** 停用用户/重置密码 → 存量会话立即踢下线（DEL session + SREM 索引）：比源行为更严格——停用用户持旧 token 继续操作是安全洞，且索引天然支持；此为有意的安全增强而非语义漂移
- **D-10:** 会话 Hash 载荷：userId / userName / realName / roles（角色码列表，供 Phase 3 起点与 displayName 用）；permissions 快照不放（Phase 3 Casbin 即时生效语义取代登录快照——PROJECT.md 决策）

### 五域 REST 契约与守卫（Claude 自主决策，延续 Phase 1 模式）

- **D-11:** 路由复数资源：`/api/v1/{auth,users,roles,menus,depts,api-keys}`，延续 Phase 1 `/api/v1/users` 既有形态
- **D-12:** 用户角色分配独立子资源：`PUT /api/v1/users/:id/roles`（全量替换语义）+ `GET /api/v1/users/:id/roles` 查询（USER-04）
- **D-13:** 树接口独立端点：`GET /api/v1/menus/tree`、`GET /api/v1/depts/tree`；孤儿节点提升为根（DEPT-02 源语义，菜单树复用同一构建器——parentId 为 null/0 或指向不存在节点均挂根，FEATURES.md:26）
- **D-14:** builtin 守卫：HTTP 409 + 业务错误码 + 中文消息（errcode 模式延续）；USER-07（builtin 用户删除硬拦截）与 ROLE-02（builtin 角色删除守卫）统一拦截器实现
- **D-15:** 认证中间件落位：Phase 1 的 pub 组白名单直通（D-02 临时态）本阶段收紧——`/api/v1/auth/login`、`/api/v1/auth/apikey-login`、`/healthz`、`/readyz` 白名单，其余路由全部要求有效 token（AUTH-06）

### Excel 导入作废与 hash 抽样代证（Claude 自主决策，处置 STATE 待办）

- **[informational]:** ~~导入契约：`POST /api/v1/users/import`（multipart xlsx）~~ 已作废（2026-10-31 研究实证）：USER-06 系 ROADMAP 起草错域——源系统 User/Role 域无导入端点（仅 FaultInfoController:83 有 importExcel），sys_user 表无 bit_index/location 列，三键语义属 Phase 4 FAUL-02。USER-06 已从 REQUIREMENTS.md/ROADMAP.md 删除，本阶段不做任何 Excel 导入（excelize 依赖随 FAUL-02 在 Phase 4 引入）
- **[informational]:** ~~三键 upsert / 缺键行 / 防线~~ 随导入作废（Phase 4 FAUL-02 实现时以其 CONTEXT 为准，防线设计可参考：BodyLimit 1MB + 行数上限 + 表头按列索引严格匹配）
- **D-16:** 生产 hash 抽样代证（STATE 待办"从生产库取 20 个 hash"生产库不可达）：已由 02-RESEARCH.md 完成端到端活体代证——Go bcrypt 实测验证本库 admin 真实 Hutool `$2a$` hash（明文 admin123）成功，本库 arm64 用户的 10 字符畸形值即"畸形 hash 登录失败非 500"的天然 fixture；生产抽样 20 条仍列现场部署清单（与 arm64 真机首启同栏，随部署执行）

### Claude's Discretion

- handler/service/repo 三层文件划分与测试组织方式（沿用 Phase 1 user 域模板）
- 会话管理器内部方法签名、Redis pipeline 使用时机
- 错误消息文案细节（登录失败/禁用/builtin 拦截等中文文案）
- GORM 关联查询（user_role / role_menu）具体装配写法（两查询防 N+1 已定，实现自由）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `pkg/resp`（R 响应包装）、`pkg/errcode`（业务错误码）、`pkg/paging`（Parse/PageData 钳位模板）、`pkg/ids`（雪花单例）、`pkg/types`（datetime/enum 序列化契约）
- `internal/repository/user.go`：repo.Patch（map 白名单 + RowsAffected 判 404 + 恒带 Where）——全部更新操作模板
- `internal/model/po/base.go`：BaseWithOperator（审计四列 + BeforeCreate/BeforeUpdate 填充）
- `internal/auth/identity.go`：Identity{UserID,Name} context 传播（认证中间件注入点已预留，Phase 1 注释明确）
- `internal/testutil/mysql.go`：per-test 独立容器基建；newTestApp 契约测试模式

### Established Patterns

- 指针列 PO + 显式 TableName + 禁 AutoMigrate；审计在 repo 层显式并入（不依赖 BeforeUpdate 钩子，规避 GORM Updates 钩子坑）
- PageData[T].MarshalJSON 出口保证空列表 `[]` 非 null
- int64 ID JSON 字符串化（types 层契约）
- 中间件链：RequestID→AccessLog→Recovery→BodyLimit（router.go 全局）

### Integration Points

- `internal/app/router.go` newRouter：pub 组收紧点（加 auth 中间件 + 白名单），五域 handler 注册
- `internal/app/app.go`：装配链（Phase 2 需加 Redis client 与 session manager 初始化——go-redis v9 引入）
- `configs/config.yaml` + `internal/config/config.go`：新增 auth.session / redis 配置段（koanf env 覆盖链 WR-01 已修）
- Makefile verify-static 出厂门不含 Redis 依赖变化（纯 Go 无 CGO 不受影响）

</code_context>

<specifics>
## Specific Ideas

- 登录失败防枚举语义与"畸形 hash 非 500"来自源系统直接观察（FEATURES.md），必须保真
- 部门树孤儿节点提升语义"与源一致"是 DEPT-02 验收明文
- 重置密码固定 123456 是源系统行为（USER-03），不做密码强度校验
- 用户分页默认 1/15 钳位、详情两查询装配防 N+1（USER-05）为验收明文

</specifics>

<deferred>
## Deferred Ideas

- 生产库 20 条 BCrypt hash 抽样验证 → 现场部署清单（本阶段三源代证）
- Nacos craft-satoken.yaml 真实 timeout 值 → 现场索取后校正 config 默认值（30d 代理值已配置化）
- `/auth/userInfo`、`/auth/routes`、`/auth/refresh` 源端点：v1 需求 AUTH-01..06 未含，不实现；若 Phase 3 Casbin 需要权限查询端点，届时随 PERM 需求设计
- arm64 真机首启（Phase 1 遗留，现场部署清单）

</deferred>
