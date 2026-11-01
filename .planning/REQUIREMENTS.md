# Requirements: sys-iam-go

**Defined:** 2026-10-31
**Core Value:** 在工控机上以单个轻量 Go 服务完整复刻 sys-iam 的全部 IAM 能力，且接口实现与自动化测试全部通过。

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### 认证与会话（AUTH）

- [x] **AUTH-01**: 用户可用用户名+密码登录获得 token（存量 BCrypt 哈希零迁移直接验证，畸形 hash 返回登录失败而非 500）
- [x] **AUTH-02**: 用户可用 API Key 登录（apikeyLogin 路径）
- [x] **AUTH-03**: 用户登出后会话即刻失效
- [x] **AUTH-04**: 管理员可将指定用户踢下线（userId→token 反查索引）
- [x] **AUTH-05**: 会话带 TTL 与活跃续期
- [x] **AUTH-06**: 白名单路由（登录/健康检查）免鉴权可访问

### 权限体系 Casbin（PERM）

- [x] **PERM-01**: Casbin RBAC 模型落地：用户-角色-功能权限（菜单/按钮级权限串）
- [ ] **PERM-02**: 权限变更即时生效（策略热重载，无需重新登录）
- [x] **PERM-03**: 数据权限 dataScope 行过滤：本人/本部门/本部门及以下/指定部门/全部，五档
- [x] **PERM-04**: Casbin 策略持久化到新建原生策略表（现有 sys_role_menu/sys_menu 不动，双轨）
- [x] **PERM-05**: 管理员可通过角色授权接口管理策略（分配/回收功能权限，写 Casbin 策略）
- [ ] **PERM-06**: 路由级权限声明中间件（requirePerm）拦截无权访问并返回 403

### 用户管理（USER）

- [x] **USER-01**: 用户 CRUD（物理删除）
- [x] **USER-02**: 用户状态启停（停用后不可登录）
- [x] **USER-03**: 重置密码（重置为固定 123456，源系统行为）
- [x] **USER-04**: 用户分配/回收角色
- [x] **USER-05**: 用户分页查询（默认 1/15 钳位）+ 详情防 N+1 两查询装配
<!-- USER-06 已删除（2026-10-31 Phase 2 研究实证）：源系统 User/Role 域无导入端点（仅 FaultInfoController:83 importExcel），三键语义属 FAUL-02；系 ROADMAP 起草错域复制，v1 不发明源系统没有的功能 -->
- [x] **USER-07**: builtin 内置用户删除硬拦截

### 角色管理（ROLE）

- [x] **ROLE-01**: 角色 CRUD（物理删除）
- [x] **ROLE-02**: builtin 内置角色删除守卫
- [x] **ROLE-03**: 角色分页/条件查询

### 菜单管理（MENU）

- [x] **MENU-01**: 菜单 CRUD（物理删除）
- [x] **MENU-02**: 菜单树查询（父子层级返回）

### 部门管理（DEPT）

- [x] **DEPT-01**: 部门 CRUD
- [x] **DEPT-02**: 部门树查询（孤儿节点提升语义，与源一致）

### 字典管理（DICT）

- [ ] **DICT-01**: 字典类型 CRUD
- [ ] **DICT-02**: 字典数据 CRUD
- [ ] **DICT-03**: 按类型查询字典数据集

### 系统配置（CONF）

- [ ] **CONF-01**: 系统参数读写（SysConfig CRUD）

### API Key（AKEY）

- [x] **AKEY-01**: ApiKey CRUD
- [x] **AKEY-02**: ApiKey 状态启停（停用后不可用于登录）

### 文件管理（FILE）

- [ ] **FILE-01**: 文件上传（MD5 判重，含 categoryId IS NULL 隐藏分支）
- [ ] **FILE-02**: 文件流式下载（不整载内存，工控机友好增强）
- [ ] **FILE-03**: 文件删除（存储端删除失败宽容继续删 DB 记录，源行为）
- [ ] **FILE-04**: 文件分类 CRUD（FileCategory）
- [ ] **FILE-05**: 存储后端可配置切换（本地磁盘默认 / RustFS-S3）

### 操作日志（OLOG）

- [ ] **OLOG-01**: 操作日志中间件自动记录（module/operation 显式元数据 + body 回放）
- [ ] **OLOG-02**: 敏感参数掩码（7 字段）+ errorMsg 截 500 + 记录失败静默不影响业务
- [ ] **OLOG-03**: 操作日志分页查询（默认 1/10 钳位例外，与源一致）

### 排班（SHFT）

- [ ] **SHFT-01**: 排班全量查询（无分页，源行为）
- [ ] **SHFT-02**: 排班 CRUD

### 故障信息（FAUL）

- [ ] **FAUL-01**: 故障信息 CRUD
- [ ] **FAUL-02**: 故障 Excel 导入（三键 upsert，整批事务）

### 名称同步事件（SYNC）

- [ ] **SYNC-01**: 6 个名称变更事件（dept/dictType/menu/role/username/realName）同步关联表（realName 波及 14 张表）
- [ ] **SYNC-02**: 事件事务提交后发布、handler 独立事务执行（防死锁时序）、UPDATE 带 `!= newName` 条件（NULL 语义锁定测试）

### 工程化与横切（ENG）

- [x] **ENG-01**: 统一 R 响应 {code,message,data} + 错误码分段 + HTTP 状态如实映射（废弃恒 200）
- [x] **ENG-02**: 审计字段自动填充（create_time/update_time/create_by/update_by，无操作人跳过）
- [x] **ENG-03**: 雪花 ID 应用层生成（DB 无自增）+ int64 JSON 字符串化防精度丢失
- [x] **ENG-04**: JSON 契约：时间 `yyyy-MM-dd HH:mm:ss` 无时区、null 键照常输出（禁 omitempty）、枚举读侧兼容 name|int 且未知值兜底
- [x] **ENG-05**: GORM 写路径零值保护（指针 DTO + 显式 Select 更新，禁裸 Save，防静默清空数据）
- [x] **ENG-06**: /healthz /readyz 健康检查 + 优雅关停（SIGTERM 15s 排空）
- [x] **ENG-07**: 配置本地 YAML + 环境变量（koanf，替代 Nacos）
- [x] **ENG-08**: 单二进制交叉编译（CGO_ENABLED=0 纯静态）+ systemd unit 文件

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### 文档与监控

- **DOC-01**: Swagger/OpenAPI 文档自动生成（swaggo 注解 + go:embed 内嵌 UI）
- **MON-01**: Prometheus 指标暴露（/metrics 端点）

### 认证扩展

- **AUTHX-01**: OAuth2 授权码流程（源系统死依赖，从未启用；如未来需要第三方授权再启用）
- **AUTHX-02**: 多租户域模型（Casbin RBAC with domains）

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Dubbo RPC 四接口等价物 | 单机部署无服务间调用；后续需要时再考虑协议选型 |
| Nacos 注册/配置中心 | 单机形态无需注册发现；本地 YAML + 环境变量替代 |
| 源系统"登录快照"权限语义 | 用户决策升级为 Casbin 即时生效（PERM-02） |
| 软删除 / 乐观锁 | 源系统即无（全库无 @TableLogic/@Version），GORM 不引入 DeletedAt |
| dataScope 过滤逻辑缺失的延续 | 与用户决策相反——本版真正实现（PERM-03） |
| 前端改造 | 本项目只做后端，API 允许重新设计 |
| MQTT 采集 | craft-wcs 设备采集场景，不在 sys-iam 依赖闭包 |
| 存量会话/token 迁移 | 停机切换，Go 版上线即全新会话 |
| 表结构重设计 | schema 原样沿用（例外：新增 Casbin 策略表属增量扩展） |
| 动态策略在线编辑器 | v1 通过角色授权接口管理策略；策略编辑 UI 属前端范畴 |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| ENG-01 | Phase 1 | Complete |
| ENG-02 | Phase 1 | Complete |
| ENG-03 | Phase 1 | Complete |
| ENG-04 | Phase 1 | Complete |
| ENG-05 | Phase 1 | Complete |
| ENG-06 | Phase 1 | Complete |
| ENG-07 | Phase 1 | Complete |
| ENG-08 | Phase 1 | Complete |
| AUTH-01 | Phase 2 | Complete |
| AUTH-02 | Phase 2 | Complete |
| AUTH-03 | Phase 2 | Complete |
| AUTH-04 | Phase 2 | Complete |
| AUTH-05 | Phase 2 | Complete |
| AUTH-06 | Phase 2 | Complete |
| USER-01 | Phase 2 | Complete |
| USER-02 | Phase 2 | Complete |
| USER-03 | Phase 2 | Complete |
| USER-04 | Phase 2 | Complete |
| USER-05 | Phase 2 | Complete |
| USER-07 | Phase 2 | Complete |
| ROLE-01 | Phase 2 | Complete |
| ROLE-02 | Phase 2 | Complete |
| ROLE-03 | Phase 2 | Complete |
| MENU-01 | Phase 2 | Complete |
| MENU-02 | Phase 2 | Complete |
| DEPT-01 | Phase 2 | Complete |
| DEPT-02 | Phase 2 | Complete |
| AKEY-01 | Phase 2 | Complete |
| AKEY-02 | Phase 2 | Complete |
| PERM-01 | Phase 3 | Complete |
| PERM-02 | Phase 3 | Pending |
| PERM-03 | Phase 3 | Complete |
| PERM-04 | Phase 3 | Complete |
| PERM-05 | Phase 3 | Complete |
| PERM-06 | Phase 3 | Pending |
| DICT-01 | Phase 4 | Pending |
| DICT-02 | Phase 4 | Pending |
| DICT-03 | Phase 4 | Pending |
| CONF-01 | Phase 4 | Pending |
| FILE-01 | Phase 4 | Pending |
| FILE-02 | Phase 4 | Pending |
| FILE-03 | Phase 4 | Pending |
| FILE-04 | Phase 4 | Pending |
| FILE-05 | Phase 4 | Pending |
| OLOG-01 | Phase 4 | Pending |
| OLOG-02 | Phase 4 | Pending |
| OLOG-03 | Phase 4 | Pending |
| SHFT-01 | Phase 4 | Pending |
| SHFT-02 | Phase 4 | Pending |
| FAUL-01 | Phase 4 | Pending |
| FAUL-02 | Phase 4 | Pending |
| SYNC-01 | Phase 5 | Pending |
| SYNC-02 | Phase 5 | Pending |

**Coverage:**
- v1 requirements: 54 total
- Mapped to phases: 54
- Unmapped: 0 ✓

---
*Requirements defined: 2026-10-31*
*Last updated: 2026-10-31 after roadmap creation (traceability mapped to 5 phases)*
