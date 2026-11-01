# Phase 5: 名称同步事件与验收收尾 - Context

**Gathered:** 2026-11-02
**Status:** Ready for planning

<domain>
## Phase Boundary

6 个名称变更事件的冗余字段同步全部生效（横跨 8 个域，此时所有域 CRUD 已稳定），并完成全量验收回归。具体交付：

- **名称同步事件**：dept/dictType/menu/role 名变更 + 用户 username/realName 变更，共 6 个事件；关联表冗余名称字段同步更新（realName 变更波及 14 张表——精确映射清单由研究期从源监听器逐表提取）
- **事件机制**：进程内轻量 dispatcher + 6 个同步 handler；事务提交后发布、handler 独立事务执行（修正源"事务内发布 + REQUIRES_NEW"时序）
- **全量验收回归**：14 个功能域全部接口 + 自动化测试通过；"Looks Done But Isn't" 8 项清单逐项过；WR-07/WR-08（Phase 4 advisory 挂账）顺带修复

不含：改名即刷会话快照（v1 照源不刷新，见 D-04）、boot 对账自愈（D-03 重试自愈已足）、事件机制向其他域扩展（随需注册属后续）。

需求项：SYNC-01、SYNC-02

</domain>

<decisions>
## Implementation Decisions

> 2026-11-02 用户逐区确认（4 灰区全选讨论、4 决策均采纳推荐方案）；Phase 2-4「用户全量自主授权」先例继续适用于未列出细节。

### 事件分发机制

- **D-01:** internal/event 轻量 dispatcher——Publisher 接口 + 事件类型定义 + 监听器注册（slice of listeners，同步调用，约 50-80 行自建）。零框架依赖（CLAUDE.md 已排除 watermill 等）；对齐源 DomainEvent 心智模型（6 个监听器按事件类型注册）；可 mock 测试。— **Reversibility:** reversible — 新增独立包，重构不影响业务层
- **D-02:** 发布时序照 SYNC-02 锁定语义执行——service 层 `db.Transaction` 提交成功后显式 `publish`；handler 同步执行（非 goroutine 异步）且各自使用独立事务（新开 `db.Transaction`）。这是对源"事务内发布 + REQUIRES_NEW"时序的显式修正，验收明文。

### 同步失败语义

- **D-03:** handler 独立事务失败上抛——API 返回 500，客户端重试触发 `!= newName` 幂等 UPDATE 自愈。沿 Phase 3 D-13 casbin 双轨纪律（业务事务提交后外部设施失败 500 + 幂等重试自愈），失败可观察、与既有错误处理链一致。不引入待同步清单/boot 对账（单机场景重试即自愈，无跨实例竞争）。

### 会话快照

- **D-04:** 改名/改真名后在线会话快照照源不刷新——Redis 会话 `SessionPayload{UserID,UserName,RealName,Roles}`（internal/auth/session.go:49）中的 userName/realName 下次登录生效；displayName 短暂陈旧与源系统一致。Phase 2 D-09"停用即踢"安全增强**不延伸**到改名（改名非安全事件，踢线过度）。

### 回归证据形态

- **D-05:** 全量回归证据 = 既有全部自动化测试全绿（回归即证）+ 8 项 "Looks Done But Isn't" 清单逐项映射（每项指向具体测试用例或 VERIFICATION.md 文档证据）+ VERIFICATION.md 逐项记录。清单项（ROADMAP 成功标准 3）：存量 hash 登录、API Key 双路径、内置删除拦截、编辑不传字段不清空、分页边界不 500、枚举未知值兜底、失败请求有日志、SIGTERM 后日志完整落库。不新建独立 regression 包（与既有分域集成测试重复）。
- **D-06:** WR-07/WR-08 顺带修复——永真断言（文件分类"移至根"集成断言恒真）修为真断言；parentId="0"（Update 移根响应）与 null（Create 根）双形态统一。统一方向研究期对齐源行为后定（倾向响应恒一形态），修复随本阶段测试收口落地。

### Claude's Discretion

- dispatcher 内部 API 形态（Publish 签名、事件结构体字段、监听器注册方式）
- 6 事件 × 关联表映射清单（researcher 从源 6 个监听器逐表提取，含 realName 波及 14 张表的精确列清单——sys_user_role.username 列存 realName 的相反语义需双向注释防混）
- NULL 语义锁定测试的具体断言形态（UPDATE 带 `!= newName` 对 NULL 行恒不中）
- 测试组织（沿 newTestApp 基建；事件同步集成测试的 fixture 设计）
- 计划切片与 wave 划分（Phase 4 先例：共享文件跨 plan 串行链）

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求与约束
- `.planning/REQUIREMENTS.md` §名称同步事件（SYNC） — SYNC-01/02 验收定义（6 事件、14 张表、事务时序、`!= newName`、NULL 语义）
- `.claude/CLAUDE.md` §关键移植映射表 + §What NOT to Use — 事件机制约束：进程内 pub/sub 约 50 行或 service 直接调用，禁事件总线框架；Spring 事件 + 6 监听器的等价映射
- `.planning/PROJECT.md` §Key Decisions — 双轨同步纪律（Phase 3 D-13）与"源快照语义被即时生效取代"的决策族

### 挂账与先例
- `.planning/phases/04-domains/04-REVIEW-DISPOSITION.md` — WR-07/WR-08 原始记录（parentId 双形态 + 永真断言），D-06 修复依据
- `.planning/phases/03-casbin/03-CONTEXT.md` — D-13 双轨同步纪律（提交后同步、失败 500、幂等重试自愈）——D-03 直接引用的先例
- `.planning/phases/02-rbac/02-CONTEXT.md` — D-09 停用踢人先例（本阶段明确不延伸，D-04）；D-10 会话 Hash 载荷定义

### 源系统（研究期提取目标）
- `/home/wx/workspace/sys-iam` 下 6 个名称同步监听器（DeptNameChanged/DictTypeChanged/MenuNameChanged/RoleNameChanged/UserNameChanged/RealNameChanged 及同族命名）— 事件→表→列精确映射的提取来源；源 DomainEvent 发布时序（事务内发布 + REQUIRES_NEW）为已锁定的修正对象

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/service/{dept,role,menu,user,dict}.go` 的 Update 方法 — 6 个名称变更事件的发布挂点（dept/role/menu/dictType 四处 + user 一处双事件 username/realName）
- `internal/auth/session.go:49` SessionPayload — D-04 会话快照现状（userName/realName 在内，本阶段不动）
- `internal/repository/relation.go` + `internal/model/po/relation.go` — sys_user_role/sys_role_menu 等关联表冗余列所在（同步 handler 的 UPDATE 目标）
- Phase 3 `internal/service/role.go` assignMenus — "事务提交后调外部设施 + 失败上抛"的双轨纪律现成参照（D-03 同构）

### Established Patterns
- service 层 `db.Transaction` + `repository.NewXxxRepo(tx)` 多步关联写模板（file/fault_import/role/user 四处先例）
- repo.Patch（map 白名单 + RowsAffected 判 404 + 恒带 Where）——handler 的 `!= newName` 条件 UPDATE 可仿其确定性风格
- newTestApp per-test 容器基建；集成断言含 Phase 1-4 全量回归
- pkg/ids sync.Once 单例模式——dispatcher 装配可参照

### Integration Points
- `internal/app/app.go` 装配链 — dispatcher 构造与监听器注册（Redis client 之后、router 之前）
- `internal/service/*.go` Update 路径 — 事务提交后 publish 调用点（6 处）
- 修复面：Phase 4 文件分类测试的永真断言（WR-08）与 parentId 响应形态（WR-07）

</code_context>

<specifics>
## Specific Ideas

- SYNC-02"修正源时序"是验收明文：源"事务内发布 + REQUIRES_NEW"→ Go 版"提交后发布 + handler 独立事务"，不是可选优化
- UPDATE 带 `!= newName` 条件且名称为 NULL 的行不被更新——SQL `!=` 对 NULL 比较恒为 NULL（不中），NULL 语义锁定测试是验收明文
- "Looks Done But Isn't" 8 项清单逐项过是成功标准 3 的明文——每项必须有测试或文档证据指向，不接受"应该没问题"
- realName 波及 14 张表的精确映射以源监听器为准提取，不凭推测发明

</specifics>

<deferred>
## Deferred Ideas

- 改名即刷在线会话快照（经 iam:user-tokens 反查 HSET）——若现场要求 displayName 即时一致，v2 增强
- 改名即踢下线——安全语义过度，不采纳（讨论中显式排除）
- 同步失败 boot 对账自愈（待同步清单 + 启动补齐）——若现场重试自愈不足再引入
- 事件机制向其他域扩展（新事件类型随需注册，dispatcher 天然支持）

None — discussion stayed within phase scope (deferred items above were explicitly weighed and excluded, not scope creep)

</deferred>

---

*Phase: 5-名称同步事件与验收收尾*
*Context gathered: 2026-11-02*
