# Walking Skeleton — sys-iam-go

**Phase:** 1
**Generated:** 2026-10-31

## Capability Proven End-to-End

进程可启动、/healthz /readyz 可探活，POST /api/v1/users 经真实 MySQL（sys_user 表，22 列）落库并返回契约化 R envelope（雪花 ID 字符串化、时间无时区格式、null 键保留）——全部横切约定经一条真实请求路径证明。

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Framework | Gin v1.12.0 | 用户指定；Go 生态最广泛 HTTP 框架，gin-contrib 生态齐全 |
| Data layer | MySQL 8.4 + GORM v1.31.2（AutoMigrate 禁用） | schema 沿用源项目；显式列 tag 映射真实 DDL |
| Auth | Phase 1 无鉴权全白名单（D-02）；Phase 2 自建 token-session + Redis；Phase 3 Casbin | 会话/授权分层落地；骨架先立横切契约 |
| 序列化契约 | PO 原生类型 + DTO 层契约类型（ids.ID / types.LocalDateTime / 枚举） | GORM 永远看不到自定义类型，根治 Valuer/Scanner 陷阱（研究 Pattern 1） |
| ID 生成 | bwmarrin/snowflake v0.3.0，应用层 BeforeCreate 生成 | DB 无自增（真实 DDL 证实）；scratch 实证 16k 并发零重复 |
| 配置 | koanf v2.4.1：单一 config.yaml + IAM_ 前缀环境变量覆盖 | 去 Nacos 化（D-06/D-07）；依赖树小 |
| 日志 | log/slog JSON handler → stdout，journald 接管 | D-08；不引文件滚动 |
| Deployment target | CGO_ENABLED=0 静态单二进制 + systemd unit（TimeoutStopSec=30） | 工控机单机交付（D-09）；time/tzdata 内嵌防时区漂移 |
| Directory layout | cmd/server + internal/{app,config,handler,service,repository,model,middleware,health,auth} + pkg/{resp,errcode,ids,types,paging} | Go 标准布局，用户明确否决 DDD 四层；pkg/ 只放跨项目复用件 |
| 错误出口 | pkg/errcode 分段（10001/10100/10200/10300/10400/10401/10501）+ HTTP 如实映射 | 废弃源"恒 200"怪癖；errors.As 统一出口 |

## Stack Touched in Phase 1

- [x] Project scaffold（go.mod 既有；目录结构、依赖安装、Makefile）
- [x] Routing — /healthz /readyz + /api/v1/users 四件套（POST create 为 tracer，其余 01-02 补全）
- [x] Database — sys_user 真实 DDL + builtin 补列；create/update/list/get 读写
- [x] 验证载体 — 四件套接口即"UI"（后端项目，无前端；契约经接口可观察）
- [x] Deployment — make build 静态单二进制 + deploy/sys-iam-go.service + SIGTERM 15s 排空证明

## Out of Scope (Deferred to Later Slices)

- 认证会话（登录/token/踢人/TTL）— Phase 2
- Casbin 授权与 dataScope 行过滤 — Phase 3
- 其余 12 实体的 DDL 与域 CRUD — 各域 Phase
- 操作日志中间件、Excel 导入、文件存储抽象 — Phase 4/5
- Swagger/Prometheus — v2（DOC-01/MON-01）
- 生产库存量 BCrypt hash 抽样验证 — Phase 2 验收开放项（D-05）
- password 的 BCrypt 哈希化 — Phase 2 User 域业务规则（D-01 边界：Phase 1 四件套只证明横切契约）

## Subsequent Slice Plan

- Phase 2: 用户可登录获得会话；User/Role/Menu/Dept/ApiKey 五域完整 CRUD（在本骨架四件套之上补业务规则）
- Phase 3: Casbin RBAC + 策略热重载即时生效 + dataScope 五档行过滤
- Phase 4: Dict/Config/File/Shift/FaultInfo 六域 + 存储抽象 + 操作日志
- Phase 5: 名称同步事件网 + 全量接口回归
