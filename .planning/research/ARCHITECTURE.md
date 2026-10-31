# Architecture Research

**Domain:** Java Spring Boot DDD 四层 IAM 后端 → Go 单体 Gin 服务移植（工控机单机部署）
**Researched:** 2026-10-31
**Confidence:** HIGH（源系统全部关键机制逐一读取源码确认；Go 侧模式经 context7 + web 检索交叉验证，均为 MEDIUM，与源码事实叠加后整体 HIGH）

> 源码事实标注 `[源码]`（直接阅读 `/home/wx/workspace/sys-iam` 得到，置信 HIGH）；
> Go 生态模式标注 `[Go]`（context7/官方文档 + 社区共识，置信 MEDIUM）。

## Standard Architecture

### System Overview

```
                    systemd (工控机，单机单进程)
                            │ SIGTERM
                            ▼
┌───────────────────────────────────────────────────────────────────┐
│  cmd/server/main.go ──► internal/app（组装：config→DB→Redis→路由）  │
├───────────────────────────────────────────────────────────────────┤
│  Gin 中间件链（从外到内）                                            │
│  RequestID → AccessLog → Recovery(统一envelope)                    │
│    ├─ public 路由组: /healthz /readyz /auth/login /auth/logout     │
│    └─ private 路由组: Auth(identity入ctx) → OperationLog → handler │
├───────────────────────────────────────────────────────────────────┤
│  handler 层（14 域 × REST 端点：绑定/校验/调 service/写 envelope）    │
├───────────────────────────────────────────────────────────────────┤
│  service 层（业务逻辑 + 事务边界 db.Transaction + 发事件）            │
│        │ ctx(identity) + ctx(timeout)                             │
│        ├──────────────► internal/event（进程内同步 dispatcher）      │
│        │                    └─► 6 个 name-sync handler（新事务）     │
├───────────────────────────────────────────────────────────────────┤
│  repository 层（GORM：按域分文件，接收 tx 或 db）                     │
├──────────────┬─────────────────────┬──────────────────────────────┤
│  MySQL       │  Redis              │  Storage 抽象                │
│  (现有schema) │  会话/权限/缓存       │  local disk ⇄ S3(RustFS)    │
└──────────────┴─────────────────────┴──────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| `cmd/server` | 进程入口：加载配置、装配依赖、启动/优雅关停 | `package main`，仅 main.go，无业务逻辑 |
| `internal/app` | 路由注册、中间件链编排、依赖图手工装配 | 不用 wire/fx——单服务 40+ 端点手工 new 足够清晰 |
| `internal/config` | YAML 配置结构 + 加载（文件→环境变量覆盖） | struct + yaml.Unmarshal + os.Getenv |
| `internal/handler` | HTTP 协议层：参数绑定校验、调 service、写响应 | 按域一文件（user.go/role.go/...），薄 |
| `internal/service` | 业务规则、事务边界、发布领域事件 | 按域一文件，方法首参 `context.Context` |
| `internal/repository` | GORM 数据访问，映射现有 schema | 按域一文件，构造器收 `*gorm.DB`（可为 tx） |
| `internal/model` | GORM PO（对齐现有表结构）+ API DTO | PO 与 DTO 分离，不混用 |
| `internal/middleware` | requestid/accesslog/recovery/auth/oplog | 每个中间件一文件，纯函数返回 `gin.HandlerFunc` |
| `internal/auth` | Sa-Token 等价：token 签发、会话存取、权限校验 | SessionStore 接口 + Redis 实现 |
| `internal/event` | 领域事件定义 + 进程内同步 dispatcher | 类型化 Publish/Subscribe，无第三方总线 |
| `internal/storage` | 文件存储抽象 | 接口 + local / s3 两个实现 |
| `internal/health` | liveness/readiness 端点 + 关停时翻转 | 独立小包，readiness 做 MySQL/Redis ping |
| `pkg/resp` | 统一响应 envelope {code,message,data} | 跨项目复用（后续工控机后端共用） |
| `pkg/errcode` | 业务错误码定义 + 类型化错误 | `*errcode.Error{Code,Message}` 实现 error |
| `pkg/paging` | 分页参数解析与响应结构 | 等价源 PageHelper（null→1/15 钳位） |

### Recommended Project Structure

```
sys-iam-go/
├── cmd/
│   └── server/
│       └── main.go            # 入口：config → 依赖装配 → ListenAndServe + 优雅关停
├── internal/
│   ├── app/
│   │   ├── app.go             # App 结构体：聚合全部依赖，Run()/Shutdown()
│   │   └── router.go          # 路由注册：public/private 两组，各域 RegisterRoutes
│   ├── config/
│   │   └── config.go          # Config 结构体 + Load(path) + 环境变量覆盖
│   ├── handler/               # = 源 interfaces/controller（14 域）
│   │   ├── auth.go
│   │   ├── user.go
│   │   ├── role.go
│   │   ├── menu.go
│   │   ├── dept.go
│   │   ├── dict_type.go
│   │   ├── dict_data.go
│   │   ├── config.go
│   │   ├── operation_log.go
│   │   ├── file.go
│   │   ├── file_category.go
│   │   ├── shift.go
│   │   ├── fault_info.go
│   │   └── api_key.go
│   ├── service/               # = 源 application/service（14 个）
│   │   ├── auth.go            # 登录/登出/续期/当前会话
│   │   ├── user.go …          # 其余域一一对应
│   │   └── event_publishers.go# 各域事件构造（等价源 domain/event/*.java）
│   ├── repository/            # = 源 infrastructure/persistence/mapper + repository impl
│   ├── model/
│   │   ├── po/                # GORM PO，列 tag 对齐现有 MySQL schema
│   │   └── dto/               # 请求/响应 DTO（源 AddBody/EditBody 内嵌类的归宿）
│   ├── middleware/
│   │   ├── requestid.go
│   │   ├── access_log.go      # slog 结构化访问日志
│   │   ├── recovery.go        # panic → envelope 500 固定文案（防信息泄漏，等价源兜底分支）
│   │   ├── auth.go            # 白名单 + token 校验 + identity 注入 ctx
│   │   └── operation_log.go   # 等价源 OperationLogAspect
│   ├── auth/
│   │   ├── session.go         # Session 结构 + SessionStore 接口
│   │   ├── redis_store.go     # go-redis 实现（TTL/滑动续期）
│   │   └── identity.go        # Identity 类型 + ctx key + FromContext()
│   ├── event/
│   │   ├── event.go           # Event 基础字段（聚合ID/类型/时间/事件ID）+ Dispatcher
│   │   └── name_sync.go       # 6 个名称冗余同步 handler（dept/dict_type/menu/role/username/realname）
│   ├── storage/
│   │   ├── storage.go         # Storage 接口（Put/Get/Delete/Presign）
│   │   ├── local.go           # 本地磁盘实现（工控机默认）
│   │   └── s3.go              # RustFS/S3 实现
│   └── health/
│       └── health.go          # /healthz /readyz
├── pkg/
│   ├── resp/resp.go           # envelope 写出 helper：OK(c,data) / Fail(c,err) / PageOK(c,items,total)
│   ├── errcode/errcode.go     # 错误码常量 + *errcode.Error
│   └── paging/paging.go       # Parse(page,size) 等价源 PageHelper
├── configs/
│   └── config.yaml            # 默认配置（部署时旁路放置）
├── docs/
│   └── swagger/               # swag 生成产物，go:embed 进二进制
├── deploy/
│   └── sys-iam-go.service     # systemd unit
├── scripts/
├── go.mod                     # module github.com/cool-wx/sys-iam-go
└── Makefile
```

### Structure Rationale

- **分层映射表**（源 DDD → Go，用户已定的三映射再细分）：

| 源（DDD 四层） | Go 去向 | 说明 |
|----------------|---------|------|
| `interfaces/controller` | `internal/handler` | 只做 HTTP 协议转换；源 controller 里的 `@Valid` 内嵌 Body 类 → `internal/model/dto` |
| `application/service` | `internal/service` | 业务逻辑 + 事务边界所在；事件从这里发布 |
| `domain/entity` | `internal/model/po` | 不再承载领域行为（事件收集改由 service 显式构造事件）|
| `domain/event` | `internal/event` | 事件类型 + dispatcher |
| `infrastructure/persistence/mapper` + `persistence/*RepositoryImpl` | `internal/repository` | MyBatis mapper 与 repository impl 在 Go 中合并为一层——GORM 本身就是 repository |
| `infrastructure/config`（GlobalExceptionHandler/SaTokenConfig/AutoFillHandler/枚举模块） | `internal/middleware` + `internal/auth` + `pkg/resp` + GORM hooks | 逐一打散归位，见下文各 Pattern |
| `sys-kernel shared/context.UserContext` | `internal/auth/identity.go` | ThreadLocal → context.Context |
| `result-r`（R/RCode） | `pkg/resp` + `pkg/errcode` | 放 pkg/ 因后续工控机后端将复用（PROJECT.md Context 明示） |
| `kernel/shared/constant`（SystemPermissions 等） | `internal/auth/permissions.go` | 权限码常量 |
| 4 个 RPC 接口 | 删除 | Out of Scope（单机无服务间调用） |
| starter-redis / starter-rustfs | `internal/auth/redis_store.go` / `internal/storage` | Go 无 starter 概念，直接落库为包 |

- **handler/service/repository 均按域分文件**而非按技术分层再分目录：14 个域改一个域时只碰三个文件，垂直切片与阶段化移植节奏一致。
- **DTO 与 PO 分离**：源系统直接把 entity 序列化返回，password 等字段靠 `@JsonIgnore` 挡；Go 版 API 本就允许重设计，handler 收发独立 DTO，杜绝 GORM struct 直接出网。
- **pkg/ 只放真正跨项目复用的三个小包**（resp/errcode/paging）：PROJECT.md 指明"后续所有工控机后端均用 Go"，这三个包是每服务都要的横切件；其余全进 internal/ 由编译器强制私有。Go 官方 guidance 与社区共识都支持"pkg/ 宁缺毋滥" `[Go]`。

## Architectural Patterns

### Pattern 1: 统一响应 envelope + 错误码（等价 result-r + GlobalExceptionHandler）

**What:** 所有端点输出 `{code, message, data}`；错误经 `*errcode.Error` 类型在调用链上传播，最后由统一出口写出。**何时用:** 每个 handler 出口。
**Trade-offs:** envelope 形状比纯 REST 语义多一层包装，但保持源系统前端心智模型，且 code 段可承载业务语义。

**envelope 重设计（API 允许重设计，这是明确建议而非照搬）：**

```jsonc
// 成功（含分页——废弃源把 total 放 envelope 顶层的怪癖，收进 data）
{ "code": 0, "message": "ok", "data": { "items": [...], "total": 42, "page": 1, "size": 15 } }

// 失败
{ "code": 10103, "message": "无效参数: roleName 不能为空", "data": null }
```

- `code=0` 表成功（Go 零值惯例），废弃源的"200 且靠 message 区分 create/delete/update 文案"——HTTP method 本身已表达。
- 错误码分段：`1xxxx` 通用（10001 内部错误、10100 未登录、10200 无权限、10300 数据不存在、10400 参数无效、10401 参数校验失败、10501 密码错误…），语义对齐源 RCode 但重新编号留出扩展空间。**HTTP 状态码正确映射**：未登录 401、无权限 403、参数 400、不存在 404、内部 500——源 SaToken filter 里"除 401/403 外恒 200"的做法不延续 `[源码]`。
- 兜底分支沿用源的安全语义 `[源码]`：未知错误只回固定文案，完整错误进服务端日志，不回显 SQL/内部类名。

```go
// pkg/errcode
type Error struct{ Code int; Msg string }
func (e *Error) Error() string { return e.Msg }
func New(code int, msg string) *Error { return &Error{code, msg} }
var ErrNotFound = New(10300, "数据不存在")

// pkg/resp
func Fail(c *gin.Context, err error) {
    var ec *errcode.Error
    if !errors.As(err, &ec) { ec = errcode.ErrInternal } // 未知错误收敛 + 日志记录原始 err
    c.JSON(ec.HTTPStatus(), gin.H{"code": ec.Code, "message": ec.Msg, "data": nil})
}
func OK(c *gin.Context, data any)     { c.JSON(200, gin.H{"code": 0, "message": "ok", "data": data}) }
```

- `errors.As` 链路 = 源 `@RestControllerAdvice` 按异常类型分发的等价物；panic 由 recovery 中间件兜底（同 envelope），两出口合流后 GlobalExceptionHandler 无单独对应物——这是 Go 的优势：错误不是控制流异常，是普通返回值。

### Pattern 2: context.Context 用户传播（等价 UserContext ThreadLocal）

**What:** auth 中间件校验 token 后把不可变 `Identity` 注入 `c.Request.Context()`，service/repository 方法首参收 `ctx`，审计填充从 GORM hook 内取。
**When to use:** 所有经过 private 路由组的请求。
**Trade-offs:** 相比 ThreadLocal，Go 方式显式传参、无 goroutine 泄漏残留风险；代价是每个 service 方法签名多一个 ctx 参数（本就是 Go 惯例）。context 用于请求作用域值（身份、trace id）是官方认可用法，前提是类型化 key、不塞必需依赖 `[Go]`。

```go
// internal/auth/identity.go —— 等价源 UserContext.set/get/clear
type ctxKey struct{}                        // 私有类型 key，绝不用 string key
type Identity struct{ UserID int64; Name string }

func WithIdentity(ctx context.Context, id Identity) context.Context {
    return context.WithValue(ctx, ctxKey{}, id)
}
func FromContext(ctx context.Context) (Identity, bool) { // 与源 getUserId() 可空语义一致
    id, ok := ctx.Value(ctxKey{}).(Identity)
    return id, ok
}

// middleware/auth.go 内：
c.Request = c.Request.WithContext(auth.WithIdentity(c.Request.Context(), ident))
c.Next()
// 无需源 UserContextCleanupFilter 等价物：ctx 随请求生命周期消亡，天然无线程池残留 [源码→Go 语义升级]
```

**后台任务语义保留** `[源码]`：源 AutoFillHandler 在无登录上下文线程跳过审计填充（"系统行为无操作人"）。Go 版定时任务/RPC-like 入口直接传 `context.Background()`，`FromContext` 返回 `ok=false` → hook 跳过填充，语义完全一致。

### Pattern 3: GORM 事务边界 + 审计字段自动填充（等价 AutoFillHandler + Base 实体）

**What:** service 方法是事务边界，用 `db.Transaction` 闭包；审计 6 字段（create_time/update_time/create_by_id/create_by_name/update_by_id/update_by_name）由 GORM hook 从 `tx.Statement.Context` 自动填。
**When to use:** 所有跨多表写的 service 方法（如角色分配菜单、用户导入）。
**Trade-offs:** 闭包事务胜在"忘 return err 就回滚"不可能发生；缺点是闭包内不能开新 goroutine 用 tx（连接绑定）。嵌套事务 GORM 用 SavePoint 支持，覆盖源 REQUIRES_NEW 场景 `[Go]`。

```go
// internal/service/role.go —— 事务边界模式
func (s *RoleService) AssignMenus(ctx context.Context, roleID int64, menuIDs []int64) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        roleRepo := repository.NewRoleRepo(tx)      // tx-scoped repo，事务内重建
        if err := roleRepo.ReplaceMenus(ctx, roleID, menuIDs); err != nil {
            return err                              // return err ⇒ 自动 rollback [Go: gorm.io/docs/transactions]
        }
        return repository.NewMenuRepo(tx).Touch(ctx, menuIDs)
    })                                              // return nil ⇒ commit
}

// internal/model/po/base.go —— 等价 BaseAggregate + BaseUserEntity（字段名对齐现有 schema 列）
type Base struct {
    ID         int64      `gorm:"primaryKey" json:"id"`
    CreateTime *time.Time `gorm:"column:create_time" json:"createTime"`
    UpdateTime *time.Time `gorm:"column:update_time" json:"updateTime"`
    Remark     *string    `gorm:"column:remark" json:"remark"`
}
type BaseWithOperator struct {
    Base
    CreateByID   *int64  `gorm:"column:create_by_id"   json:"createById"`
    CreateByName *string `gorm:"column:create_by_name" json:"createByName"`
    UpdateByID   *int64  `gorm:"column:update_by_id"   json:"updateById"`
    UpdateByName *string `gorm:"column:update_by_name" json:"updateByName"`
}

// BeforeCreate/BeforeUpdate hook —— 等价 AutoFillHandler.insertFill/updateFill
func (b *BaseWithOperator) BeforeCreate(tx *gorm.DB) error {
    now := time.Now()
    b.CreateTime, b.UpdateTime = &now, &now
    if id, ok := auth.FromContext(tx.Statement.Context); ok {  // [Go] hooks 可读 Statement.Context
        b.CreateByID, b.CreateByName = &id.UserID, &id.Name
        b.UpdateByID, b.UpdateByName = &id.UserID, &id.Name
    } // 无 identity（后台任务）→ 保持 null，与源"系统行为无操作人"语义一致 [源码]
    return nil
}
```

**列名注意**：源 MyBatis-Plus 默认驼峰转下划线（`createById → create_by_id`），仓库内无 SQL 文件可直接确认，PO 的 `gorm:"column:..."` tag 须在 Phase 骨架期对实际 schema 逐列核对一次（标记为 phase research flag，见文末）。

### Pattern 4: 领域事件 → 进程内同步 dispatcher（等价 SpringDomainEventPublisher + 6 @EventListener）

**What:** 小型类型化事件总线：service 在**事务提交成功后**同步调用 `dispatcher.Publish(ctx, event)`，注册的 handler（6 个名称冗余同步）各自开独立事务执行。
**When to use:** 仅 6 个 name-sync 场景 + 未来同类"派生数据同步"副作用。
**Trade-offs:** 直接 service 互调更简单，但会让 5 个业务 service 交叉依赖且散落副作用；进程内总线保留源解耦语义且无第三方依赖 `[Go]`。**不引入**跨进程 MQ——单机部署，源的 RabbitMQ 依赖不在移植闭包内。

```go
// internal/event/event.go
type Event interface{ EventType() string }
type Dispatcher struct{ mu sync.RWMutex; handlers map[reflect.Type][]func(ctx context.Context, e Event) error }
func (d *Dispatcher) Subscribe(e Event, h func(context.Context, Event) error)
func (d *Dispatcher) Publish(ctx context.Context, e Event)          // 同步顺序执行

// service 用法 —— 事务提交后发布（对源语义的修正，见下）
if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    return repository.NewMenuRepo(tx).Update(ctx, menu)
}); err != nil { return err }
s.events.Publish(ctx, &event.MenuNameChanged{MenuID: id, OldName: old, NewName: nw})

// internal/event/name_sync.go —— 等价 MenuNameSyncListener 等 6 个 @EventListener
d.Subscribe(&event.MenuNameChanged{}, func(ctx context.Context, e event.Event) error {
    ev := e.(*event.MenuNameChanged)
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {   // 独立事务 ≈ REQUIRES_NEW
        if err := s.roleRepo(tx).SyncMenuName(ctx, ev.MenuID, ev.NewName); err != nil { return err }
        return s.menuRepo(tx).SyncParentName(ctx, ev.MenuID, ev.NewName)
    })
})
```

**与源的两处刻意偏差**（都要写进 roadmap 说明）：
1. **发布时机移到提交后**。源在事务内 `publishEvent`，监听器 REQUIRES_NEW 立即独立提交——若主事务随后回滚，冗余同步已落库且无人回补（源的行为瑕疵）。Go 版"提交后才发布"根除此类不一致。
2. **同步执行（不派 goroutine）**。Publish 阻塞至 handler 完成，失败仅记日志不影响主请求（与源一致：监听器异常不回滚已提交主事务）。进程崩溃丢一次同步的风险由"名称冗余本质是可重建派生数据"兜底——可加启动期 reconcile（roadmap 可选项，非必需）。

### Pattern 5: 中间件链编排（等价 SaTokenConfig filter + GlobalExceptionHandler + AccessLog）

**What:** 两条路由组。全局链三条（RequestID→AccessLog→Recovery），private 组追加 Auth→OperationLog；白名单路由（登录、登出、健康检查）挂 public 组。
**When to use:** app/router.go 组装一次。
**Trade-offs:** auth 用 Gin 原生中间件而非全局 filter，白名单变成"路由组归属"问题——比源 SaWhiteListProperties 路径字符串匹配更结构化（源仍保留配置化白名单做补充，覆盖静态资源等特例）。

```
engine := gin.New()                              // 弃用默认 Logger/Recovery，换 slog + envelope 版
engine.Use(middleware.RequestID())               // X-Request-ID（入站有则复用）
engine.Use(middleware.AccessLog(logger))         // method path status dur reqID userID（slog JSON）
engine.Use(middleware.Recovery(logger))          // panic → {code:10001,"服务器内部错误"} 固定文案 [源码防泄漏语义]

pub := engine.Group("/")
priv := engine.Group("/", middleware.Auth(authSvc))            // 等价 SaServletFilter.checkLogin
priv.Use(middleware.OperationLog(oplogSvc))                    // 等价 OperationLogAspect

// 权限校验：源 @SaCheckPermission 注解 → Go 无注解，路由注册处声明
priv.GET("/roles", requirePerm(perm.RoleList), h.ListRoles)    // requirePerm 是返回 gin.HandlerFunc 的高阶函数
```

```go
// middleware/auth.go —— 等价 SaTokenConfig：校验 + 注入；失败 AbortWithStatusJSON(401, envelope)
// middleware/recovery.go —— gin.CustomRecovery 出统一 envelope [Go: gin CustomRecovery 支持自定义 JSON]
```

### Pattern 6: 优雅关停 + systemd 健康端点（等价 actuator health + k3s 探针）

**What:** `ListenAndServe` 进 goroutine；`signal.NotifyContext` 捕 SIGINT/SIGTERM；先翻转 readiness 再 `srv.Shutdown(带超时 ctx)`；最后逆序关 Redis/MySQL。`/healthz`（liveness=进程在）、`/readyz`（readiness=MySQL+Redis ping 通）挂 public 组。
**When to use:** main.go 固定骨架。
**Trade-offs:** context 限"何时开始关"，Shutdown timeout 限"排空多久"，两个时间约束分开 `[Go]`。systemd 用 `Type=simple` + 默认 SIGTERM 即可；不引 sd_notify（`Type=notify`）避免 cgo/systemd 库依赖，保持单二进制纯净。

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
go func() { if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) { log.Fatal(err) } }()
<-ctx.Done()
health.SetNotReady()                                  // 先摘流（等价 k3s readiness 翻转）
shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
_ = srv.Shutdown(shutdownCtx)                         // 排空在途请求
_ = redisCli.Close(); _ = sqlDB.Close()               // 逆序关资源
```

### Pattern 7: OperationLog 中间件（等价 AOP 切面）

**What:** private 组中间件，`c.Next()` 前采样（method/path/clientIP/userID/realName）、后落库（status/duration/errorMsg），请求参数脱敏截断。
**When to use:** 全部管理端点（源切面拦全部 controller，无注解豁免 `[源码]`）。
**Trade-offs:** Go 中间件拿不到"handler 名"，模块/操作描述改为路由注册处声明元数据。落库建议同步 insert + 失败吞错（源 finally 里 save 失败静默 `[源码]`）；单机低并发下同步写足够，批量异步 writer 留作优化项。

```go
priv.GET("/roles", oplog.WithMeta("角色管理", "分页查询角色"), h.ListRoles)
// middleware/operation_log.go：c.Next() 后写 OperationLog
// params = body(≤3KB, 正则脱敏 password/apiKey/token/secretKey → ******) + query [源码等价]
// errorMsg 截断 500 字符 [源码等价]
```

## Data Flow

### Request Flow（读路径）

```
客户端 → RequestID → AccessLog → Recovery → [public: 直接 handler]
                                        └→ [private: Auth(token→Redis会话→identity入ctx)
                                                → OperationLog
                                                → handler(绑定DTO+校验)
                                                → service(ctx, …业务校验)
                                                → repository(ctx, GORM→MySQL)
                                           ← PO→DTO 转换 ← resp.OK(c, data)]
```

### Login Flow（写路径 + 会话建立）

```
POST /auth/login（public，白名单）
  → handler 绑定 {username, password}
  → service: 查 user（repo）→ bcrypt/源同款哈希校验 → 加载 roles+permissions（role/menu repo 联查）
  → auth: 签发 token（随机 256bit hex）→ 会话 {userID, name, roles, permissions, expireAt} 写 Redis（TTL）
  → resp.OK(token) ；后续请求 Auth 中间件以 token 换会话 + identity 注入 ctx
```

### Event Flow（名称冗余同步）

```
service.Role.Update 成功提交事务
  → event.Publish(RoleNameChanged{roleID, old, new})
  → name_sync handler：独立事务 UPDATE role_menu SET role_name=? WHERE role_id=?
  →                UPDATE user SET role_name=? WHERE role_id=?（等源 6 监听器的各目标表）
  → 失败仅 slog.Error 记录（派生数据可由 reconcile 重建）
```

### Key Data Flows

1. **权限校验流**：登录时权限列表冻结进 Redis 会话（与源 StpInterface 从 SaSession 读 PERMISSIONS/ROLES 完全同构 `[源码]`）；每次请求 Auth 中间件从会话取列表，`requirePerm` 做包含判断。改名/授权变更后权限陈旧问题与源一致，由源既有的"变更时刷新会话"机制对等解决。
2. **审计字段流**：identity 只活在 ctx → GORM hook 读 `Statement.Context` → 6 审计列自动填充；无 identity 上下文保持 NULL。
3. **文件流**：`Storage` 接口双实现，配置 `storage.driver` 决定注入 local 或 s3；File/FileInfo 域只依赖接口。

## Build Order（roadmap 阶段依赖）

组件依赖图决定了建设顺序（下游消费：阶段划分建议）：

```
config ─→ app 组装 ─→ 中间件链(RequestID/Log/Recovery) ─→ envelope+errcode ─→ health+优雅关停
                                   │
                                   ▼
                 auth（会话存储→登录→Auth中间件→identity）
                                   │
                                   ▼
                 纵向切片：User 域（handler+service+repo+审计hook 全链路打通）
                                   │
        ┌──────────────────────────┼───────────────────────┐
        ▼                          ▼                       ▼
  横切设施第二波              其余 CRUD 域               文件域
  （oplog 中间件、            （Dept→Menu→Role→          （storage 抽象→
   event dispatcher+          RoleMenu→Dict*→Config）     local/s3→File 域）
   6 个 name-sync）
                                   │
                                   ▼
                     Swagger 生成 + 各域测试伴随收尾
```

**排序理由**：
- 骨架（config→envelope→middleware→health）先行：一切 handler 都站在它上面，且是 systemd 部署验收的最小单元。
- auth 早于业务域：identity 是审计 hook、oplog、权限校验的共同前置。
- User 域做第一个纵向切片：它同时压到 auth（登录）、审计 hook（BaseWithOperator）、分页、CRUD 四件横切设施，是打通全链路的最佳试验田。
- name-sync 事件依赖 Menu/Role/Dept/Dict/User 域全部存在，放横切第二波（此时 dispatcher 的消费方才齐备）。
- Import（Excel 导入）最后：依赖 User/Role 域成熟。

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 当前目标（工控机，几十~几百用户） | 单进程单机足够；MySQL 连接池 ≤20；每请求 1 次 Redis 读（会话）无压力 |
| 权限校验成为热点 | 权限列表随会话已整块存 Redis（一次 GET），无需再优化；可加进程内短 TTL 缓存 |
| oplog 写入放大 | 切异步批量 writer（channel + 定时 flush + 关停 flush），接口不变 |
| 多工控机共享中心库（远期） | 本架构不改：Redis/MySQL 连接指向中心地址即可；这才需要重新评估文件存储切 S3 |

### Scaling Priorities

1. **First bottleneck:** oplog 同步 insert 拖慢所有写请求 → 切异步 writer（预留了中间件边界，改造成本低）。
2. **Second bottleneck:** Redis 单点（会话丢失=全员重登）→ 工控机场景接受（源单机形态同样接受），不做哨兵。

## Anti-Patterns

### Anti-Pattern 1: 把 *gin.Context 传进 service/repository
**What people do:** 沿源代码调用链把 `c` 一路传下去。
**Why it's wrong:** service 被 web 框架绑架，无法单测、无法复用于定时任务；gin.Context 在 handler 返回后失效，被后台 goroutine 持有会数据竞争。
**Do this instead:** handler 体内取 `c.Request.Context()`，向下只传 `context.Context`。

### Anti-Pattern 2: 复刻 DDD 目录名（domain/infrastructure/interfaces）
**What people do:** 换语言不换骨架，四个空壳层照搬。
**Why it's wrong:** 用户已明确否决；Go 中 GORM struct 即 repository，多一层接口只是搬运。
**Do this instead:** 按上文映射表打散归位，service 直呼 repository。

### Anti-Pattern 3: 事件 handler 派 goroutine / 在事务闭包内 Publish
**What people do:** `go dispatcher.Publish(...)` 想"不阻塞请求"。
**Why it's wrong:** goroutine 里的 ctx 已取消（请求结束）、panic 无人 recover、失败无痕迹；事务内同步 Publish 则重现源"主事务回滚但同步已提交"的瑕疵。
**Do this instead:** 提交后同步 Publish（Pattern 4），确需异步时用带缓冲 channel + 专属 worker + 关停 flush（明确列为后续优化，MVP 不做）。

### Anti-Pattern 4: string context key / ctx 存结构体字段
**What people do:** `context.WithValue(ctx, "userId", ...)` 或把 ctx 塞进 App 结构体。
**Why it's wrong:** string key 跨包冲突；存进 struct 断开取消链 `[Go]`。
**Do this instead:** 私有类型 key + `FromContext` 访问器（Pattern 2）；ctx 只作方法首参流动。

### Anti-Pattern 5: envelope 顶层 total + "恒 200" 状态码照搬
**What people do:** 为省事把 `{...,total}` 和"除 401/403 外 HTTP 全 200"原样复刻。
**Why it's wrong:** API 本就允许重设计（PROJECT.md）；恒 200 让监控/网关按状态码统计全失效。
**Do this instead:** data 内含 total；HTTP 状态码如实映射（Pattern 1）。

### Anti-Pattern 6: GORM PO 直接做 API 出入参
**What people do:** handler 绑定 `model/po.User`。
**Why it's wrong:** password_hash 等敏感列靠 tag 挡不如 DTO 隔离；schema 与 API 契约耦合，重设计无从谈起。
**Do this instead:** `model/po` 与 `model/dto` 分离，handler 层手写转换（14 域体量下不值得引 copier 类反射库）。

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| MySQL（现有 schema） | GORM `gorm.Open(mysql.Open(dsn))`，**AutoMigrate 禁用**（schema 沿用） | PO 列 tag 必须逐列核对实际 schema；下划线命名是推测（MyBatis-Plus 默认）而非实证 |
| Redis | go-redis v9；会话 key `iam:session:{token}`，TTL 对齐源 Sa-Token timeout | 滑动续期等价 `renewTimeout`；token 名/超时进 config |
| RustFS（S3 协议） | `internal/storage` 接口 + minio-go 或 aws-sdk-go-v2 二选一（stack 研究员定） | local 实现为默认（单机最简），driver 配置切换 |
| systemd | `Type=simple` + KillMode=mixed；SIGTERM 驱动优雅关停 | ExecStart 绝对路径 + `--config /etc/sys-iam-go/config.yaml` |
| Swagger UI | swag 注解生成 OpenAPI（swag/v2 出 3.x），`go:embed docs/swagger` 进二进制，单路由挂载 | 工控机现场无外网 → embed 是硬需求；UI 开关进 config，仅内网开 `[Go]` |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| handler ↔ service | 函数调用 + ctx | service 不感知 HTTP；错误以 `*errcode.Error` 返回 |
| service ↔ service | 尽量避免；确需经 event 或上提合并 | 防止源 service 交叉依赖复现 |
| service ↔ event | 提交后 Publish（同步） | 仅派生数据同步类副作用；主流程查询一律直调 |
| service ↔ repository | 函数调用，事务闭包内以 tx 重建 repo | repo 是 GORM 薄封装，不设接口（单实现不抽象） |
| 域与域数据 | 各域 repo 独立，跨域读经 service 传参 | RoleMenu 这类联结表归 Role 域 repo |

## Sources

**源码实证（HIGH）——`/home/wx/workspace/sys-iam` 直接阅读：**
- `sys-kernel/.../shared/context/UserContext.java` — ThreadLocal 结构与后台任务语义
- `sys-kernel/.../infrastructure/config/SaTokenConfig.java` — filter 链、白名单、会话读权限、状态码映射
- `sys-kernel/.../infrastructure/config/GlobalExceptionHandler.java` — 异常→R 映射与防泄漏兜底
- `sys-kernel/.../infrastructure/config/AutoFillHandler.java`、`domain/entity/BaseAggregate.java`、`BaseUserEntity.java` — 审计填充与实体基类
- `sys-kernel/.../infrastructure/persistence/PageHelper.java` — 分页钳位
- `sys-kernel/.../domain/event/DomainEvent.java`、`infrastructure/event/SpringDomainEventPublisher.java`
- `sys-iam/.../application/listener/MenuNameSyncListener.java`（等 6 个）— REQUIRES_NEW 同步事务
- `sys-iam/.../application/listener/OperationLogAspect.java` — 切面字段集与脱敏规则
- `result-r/.../R.java`、`RCode.java` — envelope 与错误码全集
- `sys-iam/src/main/resources/application.yml` — Nacos 配置结构

**Go 生态（MEDIUM，context7 官方文档 + web 共识）：**
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout)、[go.dev/doc/modules/layout](https://go.dev/doc/modules/layout) — 布局共识
- [Gin Context API](https://github.com/gin-gonic/gin)（context7 /gin-gonic/gin）— Next/AbortWithStatusJSON/CustomRecovery
- [GORM Transactions](https://gorm.io/docs/transactions.html)、[GORM Context in Hooks](https://gorm.io/docs/context.html)（context7 /websites/gorm_io）— 事务闭包与 hook 读 ctx
- [VictoriaMetrics: Graceful Shutdown in Go](https://victoriametrics.com/blog/go-graceful-shutdown)（2025-05）— 关停双时间约束
- [swaggo/swag](https://github.com/swaggo/swag)（v2 = OpenAPI 3）— 注解式文档现状
- r/softwarearchitecture [Method Calls vs Event-Driven in Modular Monolith](https://www.reddit.com/r/softwarearchitecture/comments/1ckjtdx/method_calls_vs_eventdriven_architecture_in_a)、[Go Is the Language Built for Modular Monoliths](https://medium.com/@saatvikawasthi1998/go-is-the-language-built-for-modular-monoliths-1cb1df7950cc) — 进程内事件取舍

---
*Architecture research for: sys-iam-go（Java→Go IAM 移植）*
*Researched: 2026-10-31*
