<!-- GSD:project-start source:PROJECT.md -->

## Project

**sys-iam-go**

将现有 Spring Boot / Java 实现的 sys-iam 身份与访问管理（IAM）后端**整体移植为 Go 语言**。源系统位于 `/home/wx/workspace/sys-iam`（115 个 Java 文件的 DDD 四层架构），Go 版将其全部功能域统一为单个 Gin 服务，部署目标从中心服务器转向**工控机单机环境**，追求跨平台、高性能、轻量化、安装包小、易维护。

**Core Value:** 在工控机上以单个轻量 Go 服务完整复刻 sys-iam 的全部 IAM 能力，且接口实现与自动化测试全部通过。

### Constraints

- **Tech stack**: Go 1.27 + Gin（Web）+ GORM（ORM）— 用户明确指定；module 名 `github.com/cool-wx/sys-iam-go`
- **架构**: Go 标准项目结构（cmd/internal/pkg 布局），**不用 DDD 四层** — 用户明确指定
- **数据库**: MySQL，schema 沿用源项目，不重设计
- **基础设施**: 保留 Redis（会话/缓存）；去除 Nacos；文件存储可配置（本地磁盘/RustFS S3）
- **API 契约**: 允许重新设计（不受源 REST 契约约束）
- **部署**: 工控机单机零外部服务依赖（MySQL/Redis 在本机），单二进制交付

<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->

## Technology Stack

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended | Confidence |
|------------|---------|---------|-----------------|------------|
| Go | 1.27.3（go.dev 当前稳定版） | 语言运行时 | 用户指定 Go 1.27 已于 2026-08 发布，当前补丁版 1.27.3；`CGO_ENABLED=0` 纯静态交叉编译契合工控机单二进制交付 | HIGH |
| Gin | v1.12.0（2026-02-28） | HTTP 框架 | 用户指定；Go 生态最广泛使用的 HTTP 框架，gin-contrib 中间件生态齐全，零依赖路由性能足够；与 swaggo/slog-gin 集成成熟 | HIGH |
| GORM | v1.31.2（2026-06-22） | ORM | 用户指定；`Tabler` 接口（`TableName() string`）精确映射现有 MySQL 表名，`gorm.Config` 可关闭 AutoMigration 纯读既有 schema；`NamingStrategy` 可配置 snake_case 列名映射 | HIGH |
| gorm.io/driver/mysql | v1.6.0 | GORM MySQL 方言 | 官方方言驱动，底层 go-sql-driver/mysql v1.10.1（纯 Go，无 CGO）；支持 DSN 参数化连接池配置 | HIGH |
| go-redis | v9.23.0（2026-10-05） | Redis 客户端 | Redis 官方维护的 Go 客户端；typed commands、连接池（默认 10×GOMAXPROCS）、TTL 原语齐全——会话存储和缓存共用一个 client | HIGH |
| golang.org/x/crypto/bcrypt | v0.58.0（随 x/crypto 模块） | 密码哈希 | **已验证与源系统兼容**：源码用 Hutool `BCrypt.hashpw/checkpw`（标准 `$2a$` 格式，见 `AuthApplicationService.java:56`）；Go 的 `CompareHashAndPassword` 直接验证 Java 侧生成的哈希，接受 `$2a$/$2b$/$2y$`，cost 存于哈希串内，**存量用户密码零迁移** | HIGH |

### 认证与会话（Sa-Token 等价方案）

- Sa-Token 是**服务端会话**模式：随机 token ↔ Redis 中的 SaSession（存 userId、realName、roles、permissions 列表），`StpInterface` 每次从 session 读权限 —— **不是 JWT**。
- 认证语义：登录发 token → 请求带 token → filter 校验 `checkLogin` → 注入 UserContext（userId/realName）→ 注解/路径校验权限角色 → 白名单路径放行 → 401/403 JSON 响应。

| 实现要点 | Go 等价 | 说明 |
|----------|---------|------|
| token 生成 | `crypto/rand` 32 字节 hex（256-bit 熵） | 标准库即可，比 Sa-Token 默认 UUID 更安全；不引依赖 |
| 会话存储 | Redis Hash：`iam:session:{token}` → `{userId, realName, userName, roles[], permissions[]}`，TTL = 登录超时（对齐源 Sa-Token 配置） | 登录时把 roles/permissions 冗余进会话（与源 `StpInterface` 从 session 读的行为一致，权限变更后需重登录生效——语义等价） |
| 用户→token 索引 | Redis Set：`iam:user-tokens:{userId}` | 支持"改密/禁用踢下线"（Sa-Token kickout 等价）——**这是通用 session 库没有的能力** |
| 校验中间件 | Gin middleware：读 `Authorization: Bearer <token>`（或自定义 header）→ 查 Redis → 未命中 401；权限不匹配 403 | 对齐源 `SaServletFilter` 的错误码语义（NEED_LOGIN→401，ACCESS_DENIED→403） |
| 用户上下文 | `context.Context` 携带 userId/realName（自定义 context key），handler 内取用 | ThreadLocal 的 Go 正解，禁止全局变量 |
| 续期 | 每次校验命中后 `EXPIRE` 刷新 TTL | Sa-Token 活跃检测等价 |

### Supporting Libraries

| Library | Version | Purpose | When to Use | Confidence |
|---------|---------|---------|-------------|------------|
| go-playground/validator | v10.30.5（2026-09-20） | 请求参数校验 | 全部 14 个域的请求 DTO 校验；Gin 的 `ShouldBindJSON` 已内置集成（binding tag），无需额外接线。等价源 spring-boot-starter-validation | HIGH |
| excelize | v2.11.0（2026-07-06） | Excel 导入导出 | 用户/角色 Excel 导入（等价 Hutool/POI）；`GetRows` 读二维表，`Rows` 流式迭代大表，`StreamWriter` 写导出。纯 Go 无 CGO | HIGH |
| minio-go | v7.4.1（2026-10-12） | S3 兼容对象存储客户端 | File 域接 RustFS：`minio.New(endpoint, opts)` 直连任意 S3 兼容端点（RustFS 即此类），PutObject/GetObject/PresignedGetObject 齐。单依赖，远轻于 aws-sdk-go-v2 | HIGH |
| koanf | v2.4.1（2026-10-18） | 配置加载 | YAML 文件 + 环境变量覆盖（去 Nacos 化）；模块化 provider 依赖树小，契合"安装包小"。等价源 nacos-config + application.yml | MEDIUM |
| log/slog（标准库） | Go 1.21+ | 结构化日志 | 2025/2026 社区共识默认选择：零依赖、结构化、handler 可插拔；工控机单机场景吞吐非瓶颈，zap/zerolog 无必要。Gin 访问日志配 samber/slog-gin v1.21.1 | MEDIUM |
| samber/slog-gin | v1.21.1（2026-04-29） | Gin 请求日志中间件 | 把每个 HTTP 请求以 slog 结构化记录（method/path/status/耗时）；与 slog 生态无缝 | MEDIUM |
| swaggo/swag + gin-swagger | v1.16.6 / v1.6.1 | OpenAPI/Swagger 文档 | **code-first**：注释注解 → OpenAPI 2.0 + 内置 Swagger UI，与源 springdoc-openapi 的心智模型一致；API 边实现边演化时同步成本最低。等价源 springdoc | MEDIUM |
| prometheus/client_golang | v1.25.0（2026-10-08） | Prometheus 指标 | 等价源 `/actuator/prometheus`（micrometer）；健康检查端点（等价 actuator health）用 Gin 直接写 `/healthz` `/readyz` 即可，零依赖 | HIGH |
| google/uuid | v1.6.0 | UUID（如需） | 业务 ID/请求 ID 场景；token 生成本身用 crypto/rand 不用它 | HIGH |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| stretchr/testify | v1.12.1（2026-08-17） | 断言 + suite；`require` 用于前置失败、`assert` 用于聚合断言 |
| testcontainers-go | v0.44.0（2026-08-07） | MySQL/Redis 集成测试容器 | **需要 Docker**——只在开发机/CI 跑，工控机上不跑测试 |
| miniredis | v2.40.0（2026-10-10） | 纯 Go 内存 Redis mock | 单元测试会话管理器时免 Docker，快；与 testcontainers 分层：单元用 miniredis，集成用 testcontainers |
| pressly/goose | v3.29.0（2026-10-11） | SQL 迁移（暂缓引入） | schema 沿用源库首期**不需要**迁移工具；未来演进时 goose 可 `embed` SQL 进单二进制（工控机友好）。golang-migrate v4.20.1 的乱序版本处理有已知坑 |
| swag CLI | `go install github.com/swaggo/swag/cmd/swag@v1.16.6` | 生成 docs.go | 构建流程：`swag init` → go build；注解写在 handler 上 |

## Installation

# 初始化

# Core

# Supporting

# Dev/test

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| 自建 token-session 管理器 | JWT（golang-jwt/jwt/v5） | 多服务无状态校验、且接受"登出靠黑名单"时；本项目单机单服务，无此需求 |
| 自建 token-session 管理器 | alexedwards/scs v2.9.0 + redisstore | 若接受 cookie 传 token（浏览器 session 语义）且不需要踢人下线 |
| swaggo（code-first，OpenAPI 2.0） | oapi-codegen（spec-first，OpenAPI 3.x） | 若采用严格 design-first 流程（先评审 YAML spec 再写实现）；本项目"边移植边定 API"，code-first 更顺 |
| koanf | spf13/viper | 团队已熟 viper 且不在乎依赖体积；viper 更主流但重（传递依赖多，二进制更大） |
| slog | uber-go/zap v1.28.0 / rs/zerolog v1.35.1 | profiling 证明日志是瓶颈的超高吞吐服务；工控机单机不适用 |
| minio-go | aws-sdk-go-v2（config v1.33.13） | 需要全套 AWS 生态（IAM role 链、其他 AWS 服务）时；仅 S3 读写场景 minio-go 依赖小一个量级 |
| goose（未来演进时） | golang-migrate v4.20.1 / Atlas | golang-migrate：纯 up/down SQL 且线性历史可接受；Atlas：想要声明式 schema-as-code 纪律 |
| GORM 原生 SQL/查询构建器 | sqlc（类型安全 codegen） | 团队偏好 SQL-first 且能接受额外 codegen 步骤；本项目 schema 已固定，GORM 贴近 MyBatis-Plus 的移植心智 |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| JWT 作为会话机制 | 与源 Sa-Token 服务端会话语义不等价：无法登出即失效、无法踢人、权限变更不能即时生效；黑名单方案又把状态拉回 Redis | Redis 服务端 token-session（见上文方案） |
| gorilla/sessions | 默认 cookie 存储（会话数据出服务端，安全性弱）、曾整库存档一年、Redis 需第三方 redistore 适配 | 自建或 scs |
| Hutool 等价的"大而全"工具库 | Go 惯例是小依赖组合；每个工具标准库已有（strings/slices/maps/crypto） | 标准库 + 按需小库 |
| DDD 四层目录（interfaces/application/domain/infrastructure） | 用户已明确不用；Go 惯例是 cmd/internal/pkg 标准布局 | `cmd/server` + `internal/{handler,service,repo,model,middleware,config}` |
| 任何 CGO 依赖（如 mattn/go-sqlite3、cgo 加密库） | 工控机交付要求 `CGO_ENABLED=0` 纯静态单二进制；CGO 会锁死交叉编译 | 纯 Go 驱动（go-sql-driver/mysql、miniredis、excelize 均纯 Go） |
| GORM AutoMigration 常开 | schema 沿用源库，AutoMigrate 会试图改表（加外键/改列），污染既有 schema | 显式映射 + 仅在独立迁移流程（goose）中改表 |
| Dubbo/Nacos 的任何 Go 等价引入 | 单机部署无服务间调用；PROJECT.md 已明确 Out of Scope | 进程内函数调用 + koanf 本地配置 |
| spring 事件机制的重量级事件总线（watermill 等） | 6 个名称同步监听器是进程内同步场景，引事件总线框架过度设计 | 进程内 pub/sub（channel + slice of listeners，约 50 行）或 service 层直接调用 |

## Stack Patterns by Variant

- 存储抽象接口 `FileStorage`（Put/Get/Presign/Delete）两个实现：`LocalStorage`（os.WriteFile + 静态路由 `/files/`）与 `S3Storage`（minio-go）
- 配置 `storage.driver: local | s3` 切换；本地模式不加载 minio-go 连接（延迟初始化）
- 会话已集中 Redis，天然支持多实例；只需在前面加负载均衡，代码不变——这是选 Redis session 而非 JWT 的额外收益
- swag v1.16.6 仍是 2.0 输出；需要 3.0 时在 CI 加 `swagger2openapi` 转换步骤，或届时再评估迁移 oapi-codegen

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.27.3 | 全部上述版本 | excelize v2.11、GORM v1.31 均要求 Go ≥1.23，满足 |
| gorm.io/gorm v1.31.2 | gorm.io/driver/mysql v1.6.0 | 官方配对；driver 依赖 go-sql-driver/mysql v1.10.1 |
| go-redis v9.23.0 | Redis Server ≥6.2 | 工控机本地 Redis 建议 7.x；会话用 Hash+TTL 无高级特性依赖 |
| Gin v1.12.0 | validator v10.x | Gin 内置 validator 集成版本兼容 v10.30.5 |
| swaggo/gin-swagger v1.6.1 | swaggo/files v1.0.1 + swag CLI v1.16.6 | 三件套版本需配套（gin-swagger v1.6.x 对应 swag v1.16.x） |
| x/crypto/bcrypt v0.58.0 | Hutool BCrypt（源系统） | 已验证：同为标准 `$2a$` modular crypt format；**迁移前先用一条源库真实哈希写兼容性测试**（Phase 验收项） |
| testcontainers-go v0.44.0 | Docker（仅开发/CI 环境） | 工控机不跑集成测试 |

## 关键移植映射表（Java → Go）

| 源组件 | Go 等价 | 备注 |
|--------|---------|------|
| Sa-Token（sa-token-redis-jackson） | 自建 token-session 管理器 + go-redis | 见"认证与会话"节 |
| Hutool BCrypt | golang.org/x/crypto/bcrypt | 存量哈希直接可验，HIGH 置信 |
| Hutool/POI Excel | excelize v2.11.0 | `GetRows` 对齐 POI 逐行读 |
| MyBatis-Plus | GORM v1.31 | TableName 显式映射；自动填充（createdBy 等审计字段）用 GORM Hooks（BeforeCreate）对齐 AutoFillHandler |
| springdoc OpenAPI | swaggo | 注释驱动 |
| spring-boot-starter-validation | validator（Gin binding） | tag 驱动 |
| Spring AOP 操作日志 | Gin middleware + slog/DB | 源系统操作日志落库（OperationLog 域），中间件异步写 |
| Spring 事件 + 6 个监听器 | 进程内事件分发（轻量自建） | 名称冗余同步 |
| actuator health/prometheus | `/healthz` `/readyz` + client_golang | k3s 探针等价 |
| DigestUtils.md5DigestAsHex（文件去重） | crypto/md5 + encoding/hex | File 域哈希去重 |
| PageHelper 分页 | GORM `Offset/Limit` + 自建分页结构 | 对齐 result-r 响应包装 |
| ThreadLocal UserContext | context.Context value | 中间件注入 |

## Sources

- Go module proxy（proxy.golang.org/@latest）— 全部版本号实时验证，2026-10-31（**HIGH**）
- go.dev/dl — Go 1.27.3 为当前稳定版（**HIGH**）
- Context7 `/gin-gonic/gin` — v1.12.0 版本常量、中间件生态（**HIGH**，与 Go proxy 交叉验证）
- Context7 `/go-gorm/gorm` — Tabler/TableName、gorm.Config 全量配置项（**HIGH**，交叉验证）
- Context7 `/redis/go-redis` — v9.23.0、连接池与 TTL 原语（**HIGH**，交叉验证）
- Context7 `/minio/minio-go` — S3 兼容端点接入、PutObject/Presigned（**HIGH**，交叉验证）
- Context7 `/xuri/excelize-doc` — GetRows/Rows 流式读写（**HIGH**，交叉验证）
- pkg.go.dev/golang.org/x/crypto/bcrypt + Wikipedia bcrypt + Stack Overflow — `$2a$` 跨语言兼容性多源交叉（**HIGH**）
- 源码直接证据：`/home/wx/workspace/sys-iam/sys-iam/.../AuthApplicationService.java:56`（BCrypt.checkpw）、`sys-kernel/.../SaTokenConfig.java`（session 模式认证链）、`UserContext.java`（ThreadLocal）（**HIGH**）
- Web 搜索多源一致：koanf vs viper、slog vs zap/zerolog、swaggo vs oapi-codegen、goose vs golang-migrate、scs vs gorilla（**MEDIUM** — 社区共识，非官方规范）

<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->

## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->

## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->

## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->

## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-fast` for a trivial task inline, with no subagents and no PLAN.md
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->

## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
