# sys-iam-go

## What This Is

将现有 Spring Boot / Java 实现的 sys-iam 身份与访问管理（IAM）后端**整体移植为 Go 语言**。源系统位于 `/home/wx/workspace/sys-iam`（115 个 Java 文件的 DDD 四层架构），Go 版将其全部功能域统一为单个 Gin 服务，部署目标从中心服务器转向**工控机单机环境**，追求跨平台、高性能、轻量化、安装包小、易维护。

## Core Value

在工控机上以单个轻量 Go 服务完整复刻 sys-iam 的全部 IAM 能力，且接口实现与自动化测试全部通过。

## Requirements

### Validated

<!-- 源系统已运行验证的能力，作为移植基准（brownfield：能力存在于源项目，非本仓库） -->

- ✓ 认证与会话管理（Sa-Token + OAuth2 依赖 + Redis 会话）— 源系统既有
- ✓ 认证会话 Go 化（自建 token-session：登录/登出/踢下线/TTL 滚动续期/API Key 登录复用，存量 BCrypt `$2a$` 零迁移活体直证）— Phase 2
- ✓ 用户/角色/菜单/部门/API Key 五域 RBAC 主链路（CRUD/树查询/角色分配/停用即踢双轴/builtin 409 守卫/重置 123456）— Phase 2
- ✓ 用户/角色/菜单/部门 RBAC 权限体系（User/Role/Menu/RoleMenu/Dept）— 源系统既有
- ✓ 字典管理（DictType/DictData 双表）— 源系统既有
- ✓ 系统配置管理（SysConfig）— 源系统既有
- ✓ 操作日志（AOP 切面自动记录 + 查询）— 源系统既有
- ✓ 文件管理（FileCategory/FileInfo + RustFS 对象存储）— 源系统既有
- ✓ 排班管理（Shift）— 源系统既有
- ✓ 故障信息管理（FaultInfo）— 源系统既有
- ✓ API Key 管理（ApiKey）— 源系统既有
- ✓ 名称冗余同步（6 个领域事件监听器：部门/字典/菜单/角色/用户名/真名变更同步）— 源系统既有
- ✓ 数据导入（ImportResult，故障信息 Excel 导入）— 源系统既有（仅 FaultInfo 域，2026-10-31 Phase 2 研究实证修正原"用户/角色"表述）
- ✓ Go 工程骨架与横切地基（Gin+GORM+cmd/internal/pkg 结构、R 响应/雪花 ID/JSON 契约/审计填充/分页、静态单二进制+systemd+优雅关停）— Phase 1

### Active

- [ ] 14 个功能域 REST API 全量重新设计并实现（允许 REST 最佳实践重设计）（Auth/User/Role/Menu/Dept/ApiKey 六域已完成——Phase 2）
- [ ] 权限体系升级为 Casbin：RBAC 功能权限 + 数据权限（dataScope 部门级行过滤）+ 变更即时生效（不复刻源系统"登录快照"语义）
- [ ] MySQL 沿用现有 schema（GORM 接入，不重设计表结构）
- [ ] 文件存储抽象：本地磁盘 / RustFS(S3) 可配置切换
- [ ] 配置管理去 Nacos 化：本地 YAML + 环境变量
- [ ] 操作日志 AOP 等价实现（Gin 中间件方案）
- [ ] 名称同步监听器等价实现（Go 事件机制替代 Spring 事件）
- [ ] OpenAPI/Swagger 文档自动生成
- [ ] 每个功能域配套自动化测试

### Out of Scope

- Dubbo RPC 服务（User/DictData/Shift/FaultInfo 四个 RPC 接口）— 单机部署无服务间调用，后续需要时再考虑协议选型
- Nacos 注册/配置中心 — 单机形态无需注册发现；源项目注释称"nacos 为长期形态"，但那是中心化部署的前提，本项目明确去除
- 源系统"登录快照"权限语义 — 用户决策升级为 Casbin 即时生效；快照行为不复刻
- 前端改造 — 本项目只做后端；API 允许重新设计意味着前端后续自行适配
- MQTT 采集（starter-mqtt）— 属于 craft-wcs 设备采集场景，不在 sys-iam 依赖闭包内
- 存量会话/token 迁移 — 停机切换，Go 版上线即全新会话
- 表结构重设计 — MySQL schema 原样沿用（例外：新增 Casbin 策略表属增量扩展）
- 用户 Excel 导入（USER-06）— 2026-10-31 Phase 2 研究实证删除：源系统仅 FaultInfo 域有导入端点（FaultInfoController:83），sys_user 无 bit_index/location 列，三键语义属 FAUL-02（Phase 4 实现故障导入）

## Context

- **源项目**：`/home/wx/workspace/sys-iam`，Gradle 多模块仓库，本身就是渐进重写产物（tims → craft-twin，sys-iam 于 Phase 4 加入）
- **依赖闭包**：sys-iam → sys-kernel + starter-rustfs；sys-kernel → starter-redis + common-utils；响应封装为 result-r 模块。"整个后端"= 该闭包内全部能力统一移植进 Go 仓库
- **sys-kernel 提供的公共设施**（Go 版需等价实现）：Sa-Token 配置与白名单、UserContext 用户上下文、全局异常处理、统一响应包装（result-r）、分页助手（PageHelper）、Base 实体（BaseAggregate/BaseUserEntity + MyBatis 自动填充）、DomainEvent 事件发布、Enum 值序列化约定
- **源系统功能域清单**（14 Controller / 14 ApplicationService / 13 实体）：Auth、User、Role、Menu、Dept、DictType、DictData、Config、OperationLog、File、FileCategory、Shift、FaultInfo、ApiKey
- **部署环境**：工控机（工业控制计算机），单机跑全套（Go 服务 + 本地 MySQL + 本地 Redis），后续所有工控机后端均用 Go
- **OAuth2 疑点**：源 build 引入 sa-token-oauth2 但 sys-iam 源码内无直接引用，研究阶段确认是否实际启用 OAuth2 授权码流程
- 源项目有 k3s 探针（actuator health）与 Prometheus 指标暴露，Go 版等价提供健康检查端点

## Constraints

- **Tech stack**: Go 1.27 + Gin（Web）+ GORM（ORM）— 用户明确指定；module 名 `github.com/cool-wx/sys-iam-go`
- **架构**: Go 标准项目结构（cmd/internal/pkg 布局），**不用 DDD 四层** — 用户明确指定
- **数据库**: MySQL，schema 沿用源项目，不重设计
- **基础设施**: 保留 Redis（会话/缓存）；去除 Nacos；文件存储可配置（本地磁盘/RustFS S3）
- **API 契约**: 允许重新设计（不受源 REST 契约约束）
- **部署**: 工控机单机零外部服务依赖（MySQL/Redis 在本机），单二进制交付

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Gin 作为 Web 框架 | 用户指定；轻量、生态成熟，适合工控场景 | ✓ Phase 1 骨架落地 |
| Go 标准项目结构，放弃 DDD 四层 | Go 社区惯例；降低移植复杂度，用户指定 | ✓ Phase 1 骨架落地 |
| API 允许重新设计 | 无存量前端兼容包袱，按 REST 最佳实践来 | ✓ Phase 1 四端点先行落地 |
| 去除 Dubbo RPC | 单机部署无服务间调用；后续再考虑 | — Pending |
| 去除 Nacos | 单机无注册发现需求；配置走本地 YAML + 环境变量 | ✓ Phase 1 koanf 落地 |
| MySQL schema 沿用 | 数据层零迁移成本，工控机本地 MySQL | ✓ Phase 1 sys_user 22 列实证 |
| 保留 Redis | 会话集中管理延续源系统模式 | — Pending |
| 文件存储可配置切换（本地磁盘/RustFS） | 单机最简用磁盘，需要分布式时切 S3 | — Pending |
| GORM 作为 ORM | Go 主流 ORM，支持 MySQL 方言与现有 schema 映射 | ✓ Phase 1 落地（显式列映射+禁 AutoMigrate） |
| Casbin 作为授权框架（用户决策，覆盖研究建议的自建方案） | 源权限体系不完善：快照需重登录、无数据权限；升级为即时生效 RBAC + dataScope | — Pending |
| 数据权限 dataScope 真正实现 | 部门级行过滤（本人/本部门/及以下/指定部门/全部），激活源系统死字段 | — Pending |
| Casbin 策略存原生新表 | 现有 sys_role_menu/sys_menu 不动；新建 Casbin 策略表，双轨清晰 | — Pending |
| 权限变更即时生效 | Casbin 策略重载，废弃源"改权限需重登录"行为 | — Pending |
| MVP 契约载体：ROADMAP goal 回写用户故事格式 | UAT test 1 定案——回写优于 override，契约真正成立而非豁免 | ✓ Phase 1 UAT 2026-10-31 |
| ENG-07 配置缺失取舍定案 | 默认路径缺失→内置默认值+warn 启动（工控机无人值守可用性）；-config 显式缺失→退出码 1（指错早失败） | ✓ Phase 1 UAT 认可 |
| systemd unit 不追加 User=/ProtectHome/PrivateTmp | 现场账号未定不硬编码（防现场启动失败）；ProtectHome 与 Phase 4 存储路径强相关，届时随路径一并定 | ✓ Phase 1 UAT 定案，Phase 4 复核 |
| arm64 验证策略：qemu-aarch64 binfmt 全链路 | 开发机无真机；qemu 下 boot/healthz/readyz/POST 创建/SIGTERM 全链路实证，真机首启留现场部署清单 | ✓ Phase 1 UAT 2026-10-31 |
| USER-06 删除（用户 Excel 导入） | 源系统仅 FaultInfo 域有导入端点；三键 upsert 系 ROADMAP 起草错域复制——v1 不发明源系统没有的功能 | ✓ Phase 2 研究修正 2026-10-31 |
| 失效写路径全链踢会话（D-09 双轴） | 停用/重置/删除（用户轴）+ 停用/删除 API Key（apikey 轴 KickoutAPIKey）即时撤销存量会话——有意的源行为安全增强；CR-01/WR-06 review 补全 | ✓ Phase 2 |
| 登录状态校验后置（WR-01） | 防枚举优先于源顺序保真：错密码恒 10501（含禁用用户），密码正确才 10502——T-2-01 对 D-02 的显式覆盖 | ✓ Phase 2 review 修正 |
| kickout 端点收紧 RequireRoles(super_admin) | WR-05 部分修复过渡态（会话 roles 快照判定）；全量 requirePerm 属 Phase 3 Casbin | ✓ Phase 2 过渡 |
| 会话 timeout 30d 缺省 + env 覆盖 | Nacos 实际值永不可得（rnacos 空实例确认），Sa-Token 官方默认为最接近代理；现场可校正 | ✓ Phase 2（D-06） |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-11-01 after Phase 2*
