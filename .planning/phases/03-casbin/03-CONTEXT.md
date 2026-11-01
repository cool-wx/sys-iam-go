# Phase 3: Casbin 授权与数据权限 - Context

**Gathered:** 2026-11-01
**Status:** Ready for planning

<domain>
## Phase Boundary

功能权限与数据权限真正生效，且权限变更即时生效（Casbin 策略热重载——用户决策，显式区别于源系统"改权限需重登录"的登录快照语义）。具体交付：

- **Casbin RBAC 落地**：enforcer 集成（casbin/v2 + gorm-adapter）、策略表、requirePerm 路由中间件全量收口
- **即时生效**：权限分配/回收后下一请求即按新策略判定（进程内 enforcer 天然满足）
- **dataScope 五档行过滤**：本人/本部门/及以下/指定部门/全部——列表查询可观察的行差异
- **附带收口**：IN-14（apikey→user 反向索引，过期 key 存量会话撤销）

不含：菜单树/用户管理等 CRUD（Phase 2 已交付）、其他域的 dataScope 应用（Phase 4 复用本阶段通用件）

需求项：PERM-01..06

</domain>

<decisions>
## Implementation Decisions

> 依据 2026-10-31 用户全量自主授权，灰区由 Claude 自主拍板（Phase 1/2 先例一致）；决策含理由留档。

### Casbin 集成架构（Claude 自主决策）

- **D-01:** casbin/v2 + 官方 gorm-adapter（复用既有 GORM 连接与 DSN；版本研究期实证——STACK.md 未沉淀 Casbin 调研，RESEARCH 需补版本/依赖闭包确认）
- **D-02:** enforcer 进程内单例（sync.Once）；PERM-02"即时生效"= 进程内 enforcer API 调用后立即生效——单机单进程**无需 Redis watcher**（watcher 是多实例同步设施，引入属过度设计）
- **D-03:** model.conf 内嵌 const 字符串（单二进制交付原则，禁部署目录文件依赖）
- **D-04:** app.New 装配链初始化 enforcer + 启动全量 LoadPolicy，失败 fail-fast（对齐 Redis Ping 先例）

### 权限模型设计（Claude 自主决策）

- **D-05:** 纯 RBAC 模型：`p, role, perm` + `g, user, role`（无 domain 维度——单机单租户，加维度是无据复杂化）
- **D-06:** perm 串沿源权限串风格 `sys:user:list`（菜单 perms 字段语义——前端按钮联动零成本，源 sys_menu.perms 字段即此格式）
- **D-07:** super_admin 全豁免沿源语义（中间件前置判断或 enforcer 自定义函数，研究定夺；豁免判据=会话 roles 含 super_admin 或 g 策略含，两处需一致）
- **D-08:** requirePerm("perm串") 逐路由显式挂载（替换 Phase 2 的 RequireRoles 过渡 gate；显式优于集中注册表——路由与权限同屏可审计）

### dataScope 行过滤（Claude 自主决策）

- **D-09:** GORM Scope 函数注入 WHERE 子句（repo 层组合、service 传档位）——不侵入既有查询代码，Phase 4 各域可复用
- **D-10:** 五档映射：1 本人=`create_by_id=uid`；2 本部门=`dept_id=user.deptId`；3 及以下=`dept_id IN (dept 子树 ID 集，一次查询)`；4 指定部门=`dept_id IN (自定义部门集合)`；5 全部=无过滤（字段值域沿源 sys_role.data_scope）
- **D-11:** 档位取用户全部角色 data_scope 的最大权限档（多角色取宽——管理惯例；如需收紧是 v2 语义）
- **D-12:** 应用面用户列表先行（PERM-03 验收明文）；Scope 做成通用件（pkg 或 internal/datascope），Phase 4 域复用

### 双轨同步与收口（Claude 自主决策）

- **D-13:** 策略管理面复用既有 PUT /roles/:id/menus（源契约不动）：service 同一写路径内同步 Casbin 策略（RemoveFilteredPolicies + AddPolicies 批量原子），双轨一致性由 service 单点保证
- **D-14:** Casbin 策略表仅作 enforcer 持久化，不暴露独立管理端点（避免 sys_role_menu 与 casbin_rule 两套管理面漂移）
- **D-15:** requirePerm 全量挂管理端点（PERM-06 + WR-05 收口）；kickout 的 RequireRoles(super_admin) 保留叠加（纵深防御，不拆）
- **D-16:** IN-14 顺带收口：apikey→user 反向索引（iam:apikey-user:{key} 或等价），过期 key 登录拒绝时撤销其存量会话——STATE Concerns 挂账项，会话域小改不扩权

### Claude's Discretion

- casbin model 字符串的具体 matcher 表达式写法
- gorm-adapter 表名/列映射细节（默认 casbin_rule 即可）
- dataScope Scope 函数签名与包位置（研究定 pkg/datascope vs internal/…）
- perm 串常量的组织方式（按域文件 or 集中常量）
- 测试组织（miniredis/testcontainers 沿既有基建）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/middleware/auth.go`：RequireRoles 过渡 gate（:64 起注释明确 Phase 3 替换点）——requirePerm 的挂载形态参考
- `internal/auth/session.go`：会话 roles 快照（Identity.Roles）——super_admin 豁免判据现成
- `internal/repository/relation.go` + `internal/service/role.go`：assignMenus 事务写路径（D-13 的同步挂点）
- `internal/model/po/dept.go` + `internal/tree/tree.go`：dept 子树展开（D-10 档位 3 的现成原料）
- `internal/service/user.go`：用户列表（D-12 应用面）+ UserPO 的 create_by_id/dept_id 列

### Established Patterns

- 中间件链挂载（router.go pri 组逐路由声明）；errcode 域码分段（403 语义已有映射）
- newTestApp 契约测试基建（双容器）；per-test 独立容器
- enforcer 单例参考 pkg/ids 的 sync.Once 模式

### Integration Points

- `internal/app/app.go`：装配链加 enforcer 初始化（D-04，Redis client 之后）
- `internal/app/router.go`：管理端点逐路由挂 requirePerm（WR-05 注释收口点 :49-50）
- `internal/service/role.go`：assignMenus 同步 Casbin（D-13）
- DDL：scripts/ddl/ 增 casbin_rule 表（gorm-adapter 自动建 or 显式 DDL——研究定夺，倾向显式 DDL 与既有 15 表管理方式一致）

</code_context>

<specifics>
## Specific Ideas

- 权限变更即时生效是用户对源系统的显式否定（"改权限需重登录"快照语义被 PERM-02 取代）——验收的每一条都要围绕"不重登录、下一请求生效"
- dataScope 五档的行差异必须可观察（PERM-03 验收明文举例：本部门管理员看不到他部门用户，全部档可见）
- 双轨是 PROJECT.md 锁定决策：sys_role_menu/sys_menu 原样不动，Casbin 策略表是增量

</specifics>

<deferred>
## Deferred Ideas

- GET /auth/userInfo、/auth/routes 端点（前端权限菜单联动）——不在 PERM 需求内，若 Phase 4/5 前端联动需要再补
- dataScope 在其他域（字典/配置/日志等）的应用——Phase 4 复用本阶段通用件
- 多角色 data_scope 交集语义（当前取最大档）——v2 如需收紧
- Casbin 多实例 watcher（Redis pub/sub）——单机形态无需求，分布式部署时再议

</deferred>
