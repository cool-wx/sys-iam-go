# Roadmap: sys-iam-go

## Overview

将 115 文件、14 功能域的 Spring Boot IAM 后端整体移植为 Go 1.27 + Gin + GORM 单体服务（工控机单机部署：单二进制 + 本地 MySQL + 本地 Redis）。路线按依赖链推进：先立工程骨架与横切地基（统一响应、雪花 ID、JSON 契约、审计填充、分页等全库共享约定，18 项移植陷阱中 9 项在此一次性预防），再打通认证会话与 RBAC 主链路数据域，随后落地用户决策的 Casbin 授权升级（功能权限即时生效 + dataScope 五档行过滤——显式区别于源系统"登录快照"语义），接着完成其余业务域（字典/配置/文件/排班/故障）与操作日志切面，最后以名称同步事件网收口并做全量接口 + 自动化测试回归。验收标准：全部 14 域接口 + 自动化测试通过。

**本 roadmap 明确不含（v2，见 REQUIREMENTS.md）**：Swagger/OpenAPI 文档（DOC-01）、Prometheus 指标（MON-01）、OAuth2（AUTHX-01）、多租户域模型（AUTHX-02）。

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: 工程骨架与横切地基** - 可启动的单体服务 + 全库共享约定（R 响应/雪花 ID/JSON 契约/零值保护与审计填充/分页/健康检查/本地配置/单二进制交付） (completed 2026-10-31)
- [x] **Phase 2: 认证会话与 RBAC 主链路** - 自建 token-session（登录/登出/踢人/TTL/白名单）+ User/Role/Menu/Dept/ApiKey 五域完整 CRUD (completed 2026-11-01)
- [x] **Phase 3: Casbin 授权与数据权限** - Casbin RBAC + 原生策略表 + 策略热重载（变更即时生效）+ requirePerm 中间件 + dataScope 五档行过滤 (completed 2026-11-01)
- [x] **Phase 4: 业务域与操作日志** - Dict/Config/File/Shift/FaultInfo 六域 + 存储抽象（本地磁盘/RustFS-S3）+ 操作日志中间件 + Excel 导入 (completed 2026-11-02)
- [ ] **Phase 5: 名称同步事件与验收收尾** - 进程内事件 dispatcher + 6 个名称同步 handler + 全量接口与自动化测试回归

## Phase Details

### Phase 1: 工程骨架与横切地基
**Goal**: **As a** 运维工程师与后续各域开发者，**I want to** 一个在工控机环境可构建、部署、启动、优雅关停的单体 Go 服务，且全库共享的横切约定（响应包装、ID 生成、序列化契约、零值保护、审计填充、分页）一次定死，**so that** 后续每个功能域都直接站在稳定的地基上开发（UAT test 1 回写：原非故事格式的 goal 行等语义转写，2026-10-31）
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: ENG-01, ENG-02, ENG-03, ENG-04, ENG-05, ENG-06, ENG-07, ENG-08
**Success Criteria** (what must be TRUE):
  1. `CGO_ENABLED=0 go build` 产出静态单二进制，systemd unit 部署后服务正常启动；SIGTERM 触发优雅关停（15s 排空，进行中请求完整返回）；/healthz /readyz 免鉴权可访问
  2. 任意接口返回统一 R 响应 {code,message,data}，错误码分段且 HTTP 状态如实映射（废弃恒 200）
  3. 新建记录自动获得雪花 ID（DB 无自增）且 JSON 输出为字符串防 JS 精度丢失；时间输出 `yyyy-MM-dd HH:mm:ss` 无时区；null 键照常输出（禁 omitempty）；枚举读侧兼容 name|int 且未知值兜底不 500
  4. 更新接口未提交的字段保持原值（指针 DTO + 显式 Select，禁裸 Save）；create_time/update_time/create_by/update_by 自动填充且无操作人上下文时跳过操作人字段
  5. 配置全部来自本地 YAML + 环境变量（koanf，替代 Nacos）；MySQL DSN 固化 `loc=Asia/Shanghai` + parseTime（TZ=UTC 环境集成测试无 8 小时漂移）；连接池默认值显式且适合工控机
**Plans**: 3/3 plans complete

Plans:
**Wave 1**
- [x] 01-01-PLAN.md — Walking Skeleton：环境重探 + sys_user DDL + 启动链 + POST create 端到端 tracer + 横切件单元测试（Wave 1）

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 01-02-PLAN.md — 四件套补全：update/get/list + 零值保护 + 更新审计 + pkg/paging 分页 + 完整错误面（Wave 2，依赖 01-01）
- [x] 01-03-PLAN.md — 交付物：Makefile 静态交叉编译矩阵 + systemd unit + SIGTERM 优雅关停双重证明（Wave 2，依赖 01-01）

### Phase 2: 认证会话与 RBAC 主链路
**Goal**: **As a** sys-iam 的用户与管理员，**I want to** 用用户名密码或 API Key 登录获得可续期会话、并完整管理用户/角色/菜单/部门/API Key 数据，**so that** 工控机上的 Go 版 IAM 具备与源系统等价的认证与 RBAC 主链路（存量密码零迁移）（2026-10-31 规划期语义等价转写为用户故事格式，原 goal：用户可登录并获得会话（存量密码零迁移），管理员可管理用户/角色/菜单/部门/API Key 全套 RBAC 主链路数据）
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, USER-01, USER-02, USER-03, USER-04, USER-05, USER-07, ROLE-01, ROLE-02, ROLE-03, MENU-01, MENU-02, DEPT-01, DEPT-02, AKEY-01, AKEY-02
**Success Criteria** (what must be TRUE):
  1. 用户用用户名+密码登录获得 token：存量 BCrypt `$2a$` 哈希零迁移直接验证通过（生产 hash 抽样验证），畸形 hash 返回登录失败而非 500；API Key 登录路径可用（含复用已有会话分支）
  2. 登出后会话即刻失效；管理员可踢指定用户下线（userId→token 反查索引）；会话带 TTL 且活跃续期将 TTL 重置为完整 timeout
  3. 白名单路由（登录/健康检查）免鉴权可达，其余无 token 请求一律被拒绝
  4. User/Role/Menu/Dept/ApiKey 五域 CRUD 与查询全可用：用户分页默认 1/15 钳位、详情两查询装配防 N+1、builtin 用户删除硬拦截、停用用户与停用 API Key 不可登录、重置密码固定 123456、用户可分配/回收角色、角色分页条件查询（用户 Excel 导入已随 USER-06 删除移除——源系统 User 域无导入端点，三键语义属 Phase 4 FAUL-02，2026-10-31 研究实证修正）
  5. 菜单树按父子层级返回；部门树保持孤儿节点提升语义（与源一致）
**Plans**: 3/3 plans complete defined

Plans:
**Wave 1**
- [x] 02-01-PLAN.md — 认证会话 tracer 与核心：Redis 基建（DDL×6/testutil/config/依赖）+ login→session→中间件→logout 端到端 + 白名单收紧 + apikeyLogin 复用/竞态 + kickout + TTL 续期（AUTH-01..06）

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 02-02-PLAN.md — 基础数据域：泛型树构建器（孤儿提升）+ Menu/Dept 域 + Role 域（builtin 409 守卫/三连删事务/assignMenus）（MENU-01..02、DEPT-01..02、ROLE-01..03）

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 02-03-PLAN.md — 装配域收口：User 增强（roles 两查询装配/分配回收/停用踢人/重置密码/builtin 拦截）+ ApiKey 域（明文一次性返回/停用过期不可登录）（USER-01..05,07、AKEY-01..02）

### Phase 3: Casbin 授权与数据权限
**Goal**: 功能权限与数据权限真正生效，且权限变更即时生效（Casbin 策略热重载——用户决策，显式区别于源系统"改权限需重登录"的登录快照语义）
**Mode:** mvp
**Depends on**: Phase 2（依赖会话身份中间件与用户/角色数据）
**Requirements**: PERM-01, PERM-02, PERM-03, PERM-04, PERM-05, PERM-06
**Success Criteria** (what must be TRUE):
  1. 访问受保护接口时 requirePerm 中间件按 Casbin RBAC 模型（用户→角色→功能权限串）校验，无权访问返回 403
  2. 管理员通过角色授权接口分配/回收功能权限，策略持久化到新建原生 Casbin 策略表（gorm adapter），现有 sys_role_menu/sys_menu 原样不动（双轨）
  3. 已登录用户在不重新登录的情况下，权限分配/回收后的下一请求即按新策略通过或被拒（即时生效）
  4. dataScope 五档（本人/本部门/本部门及以下/指定部门/全部）在列表查询上产生可观察的行过滤差异（如"本部门"管理员查询用户列表看不到其他部门用户，"全部"则可见）
**Plans**: 3/3 plans complete

Plans:
**Wave 1**
- [x] 03-01-PLAN.md — Casbin enforcer tracer：casbin_rule DDL + authz 包装配（NewSyncedEnforcer fail-fast）+ boot 对账（存量关系表导入策略）+ RequirePerm 中间件 + GET /users 首路由 + 四写路径双轨同步 + 同 token 即时生效断言（PERM-01/02/04/05，PERM-06 中间件本体）

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 03-02-PLAN.md — dataScope 五档：sys_role_dept 存储（D-10 增量表）+ scopes 端点 + 角色四连删 + internal/datascope 通用件（Filter/Widest/子树展开）+ 用户列表挂接 + 五档行差异断言（PERM-03）

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 03-03-PLAN.md — 收口：34+2 路由 requirePerm 全量挂载（RequireRoles 退役、kickout 叠加保留）+ IN-14 apikey→user 反查索引（过期 key 登录拒绝即撤存量会话）（PERM-06 全量/PERM-02 延伸）

### Phase 4: 业务域与操作日志
**Goal**: 其余六个功能域全部可用：字典/系统配置开箱即用，文件存储可在本地磁盘与 RustFS 间配置切换，操作日志自动记录，排班与故障信息含 Excel 导入
**Mode:** mvp
**Depends on**: Phase 3（顺序执行；核心数据依赖为 Phase 1 地基与 Phase 2 认证上下文）
**Requirements**: DICT-01, DICT-02, DICT-03, CONF-01, FILE-01, FILE-02, FILE-03, FILE-04, FILE-05, OLOG-01, OLOG-02, OLOG-03, SHFT-01, SHFT-02, FAUL-01, FAUL-02
**Success Criteria** (what must be TRUE):
  1. 字典类型/字典数据/系统配置 CRUD 可用，且可按类型查询字典数据集
  2. 文件上传 MD5 判重（含 categoryId IS NULL 隐藏分支）、流式下载不整载内存（大文件下载 RSS 平稳）、删除时存储端失败仍删 DB 记录（源行为）、文件分类 CRUD；存储后端经配置在本地磁盘与 RustFS(S3) 间切换
  3. 操作日志中间件自动记录写操作：module/operation 显式元数据、敏感参数 7 字段掩码为 `******`、errorMsg 截 500、失败请求也记录（status=0）且日志落库失败静默不影响业务、body 回放后接口正常
  4. 操作日志分页查询可用（默认 1/10 钳位例外，与源一致）
  5. 排班全量查询（无分页，源行为）与 CRUD；故障信息 CRUD 与 Excel 导入（name+bitIndex+location 三键 upsert、整批事务、返回 ImportResult insertCount/updateCount，用源系统真实导出文件回测）
**Plans**: 4/4 plans complete defined

Plans:
**Wave 1**
- [x] 04-01-PLAN.md — 轻三域 tracer：8 表 DDL 全落 + 六域 perm 常量 + Dict 全链端到端（1062 翻译/类型校验冗余/按类型过滤）+ Config（key 查询）+ Shift（全量无分页）（DICT-01..03、CONF-01、SHFT-01/02）

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 04-02-PLAN.md — FileStorage 抽象（local/S3 配置切换 + rustfs smoke）+ File 域（判重五语义/流式下载/宽容删除）+ FileCategory 树 + BodyLimit 独立上限（FILE-01..05）

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 04-03-PLAN.md — Oplog 中间件（8 字段掩码/body 回放/chan 异步/Stop flush）+ resp 透出 + paging 1/10 特例 + 日志查询域 + 全部写路由声明制挂载（OLOG-01..03）

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 04-04-PLAN.md — Fault 域 CRUD + FaultType 枚举 + Excel 三键 upsert 导入（8 列契约/整批事务/ImportResult/构造等价回测）（FAUL-01/02）

### Phase 5: 名称同步事件与验收收尾
**Goal**: 6 个名称变更事件的冗余字段同步全部生效（横跨 8 个域，须等所有域 CRUD 稳定后统一实现），并完成全量验收回归
**Mode:** mvp
**Depends on**: Phase 4
**Requirements**: SYNC-01, SYNC-02
**Success Criteria** (what must be TRUE):
  1. 修改部门/字典类型/菜单/角色名或用户名/真名后，所有关联表冗余名称字段同步更新（realName 变更波及 14 张表）
  2. 事件在事务提交后发布、handler 同步执行且使用独立事务（非 goroutine 异步，修正源"事务内发布 + REQUIRES_NEW"时序），UPDATE 带 `!= newName` 条件且名称为 NULL 的行不被更新（NULL 语义锁定测试）
  3. 全量验收回归：14 个功能域全部接口 + 自动化测试通过，"Looks Done But Isn't" 清单逐项过（存量 hash 登录、API Key 双路径、内置删除拦截、编辑不传字段不清空、分页边界不 500、枚举未知值兜底、失败请求有日志、SIGTERM 后日志完整落库）
**Plans**: 0/3 plans complete

Plans:
**Wave 1**
- [ ] 05-01-PLAN.md — dispatcher 基建全量（internal/event 4 文件 + 6 handler × 35 条 `<>` UPDATE）+ app.go 装配一次到位（5 service 构造签名改齐）+ dept 域发布挂点 tracer + TestDeptNameSync 端到端（SYNC-01/02 机制与时序）

**Wave 2** *(blocked on Wave 1 completion)*
- [ ] 05-02-PLAN.md — role/menu/dict/user 四域挂点实装 + 发布条件单测（fakePublisher 四负例）+ realName 14 表全量断言 + NULL 语义锁定 + 幂等自愈重放（SYNC-01 主体 + SYNC-02 锁定测试）

**Wave 3** *(blocked on Wave 2 completion)*
- [ ] 05-03-PLAN.md — WR-07/08 修复（三域移根 parent_id NULL 唯一形态 + file 断言真化，同任务落地）+ 清单第 8 项 TestOlogSurvivesStop + 全量回归门 + VERIFICATION.md 八项清单映射（D-05/D-06 收口）

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. 工程骨架与横切地基 | 3/3 | Complete    | 2026-10-31 |
| 2. 认证会话与 RBAC 主链路 | 3/3 | Complete    | 2026-11-01 |
| 3. Casbin 授权与数据权限 | 3/3 | Complete    | 2026-11-01 |
| 4. 业务域与操作日志 | 4/4 | Complete    | 2026-11-02 |
| 5. 名称同步事件与验收收尾 | 0/TBD | Not started | - |
