---
phase: "02"
---

# Phase 2: 认证会话与 RBAC 主链路 - Pattern Map

**Mapped:** 2026-10-31
**Files analyzed:** 11 类（约 25 个新/改文件）
**Analogs found:** 8 / 11（3 类为新类别，见 ## New Categories）
**类比文件均经 `git ls-files` 核验为 tracked 源码**

## File Pattern Map

| 新文件 | 角色 | 数据流 | 最近类比 | 关键锚点 |
|--------|------|--------|----------|----------|
| `internal/model/po/role.go` | model | CRUD | `internal/model/po/user.go` | 指针列 + TableName + BeforeCreate（L14-55） |
| `internal/model/po/menu.go` / `dept.go` / `apikey.go` | model | CRUD | 同上 | 同上；树列 parent_id 用 `*int64` |
| `internal/model/po/user_role.go`（关联表） | model | CRUD | `internal/model/po/user.go` | 仅 ID + TableName，无审计基类（按 DDL 裁剪 BaseWithOperator） |
| `internal/model/dto/role.go` 等 5 域 DTO | model/DTO | transform | `internal/model/dto/user.go` | 全指针 Update 请求 + Resp 无 omitempty + ToXxxResp 手写转换（L13-112） |
| `internal/repository/role.go` 等 5 域 repo | repository | CRUD | `internal/repository/user.go` | Patch map 白名单 + RowsAffected==0 回读 Count + List 闭包分页（L44-99） |
| `internal/service/role.go` 等 5 域 service | service | CRUD | `internal/service/user.go` | Update nil→map 翻译 + ErrDuplicatedKey 翻译 + PageData envelope（L31-116） |
| `internal/handler/role.go` 等 5 域 handler | controller | request-response | `internal/handler/user.go` | bindJSON / parseXxxID / paging.Parse / resp.OK-Fail（L33-114） |
| `internal/auth/session.go`（会话管理器） | service（auth） | CRUD（Redis） | `internal/auth/identity.go`（包风格） | 私有 ctx key 风格；Redis 键 `iam:session:{token}` / `iam:user-tokens:{userId}` / `iam:apikey-token:{keyId}` |
| `internal/middleware/auth.go` | middleware | request-response | `internal/middleware/requestid.go` | gin.HandlerFunc 闭包 + ctx 注入 + c.Next()（L19-36） |
| `internal/handler/auth.go`（登录/登出） | controller | request-response | `internal/handler/user.go` | 同 user handler |
| Excel 导入（service 层方法，如 `internal/service/user_import.go`） | service | file-I/O（批量 transform） | 无直接类比 | 错误收集结构对齐 `paging.PageData` 的出口风格；逐行独立 upsert |
| 树构建器（如 `internal/service/tree.go` 或 util） | utility | transform | 无直接类比 | `pkg/paging` 单一职责包风格；孤儿节点提升为根 |
| `internal/app/router.go`（修改：路由收紧 + 5 域注册） | route | — | 本文件 L13-32 | pub 组拆 auth 白名单 + private 组挂 Auth 中间件 |
| `internal/app/app.go`（修改：Redis + session manager 装配） | config/装配 | — | 本文件 L39-63 | gorm.Open 之后加 redis client；newRouter 签名扩参 |
| `internal/config/config.go` + `configs/config.yaml`（修改：auth.session/redis 段） | config | — | `internal/config/config.go` L21-29/L67-71/L101-127/L128-143 | defaults 常量 + envKeyAliases 登记 + Exists 读取 + fail-fast 校验 |
| `internal/testutil/redis.go`（新增） | test | — | `internal/testutil/mysql.go` | testcontainers 独立容器 + cleanup 闭包 + 镜像探测 |
| `internal/*/*_test.go`（新增） | test | — | `internal/handler/user_integration_test.go` | newTestApp + doRequest + envelope 断言模式（L36-60, L156-185） |
| `internal/middleware/access_log.go` / `recovery.go` / `body_limit.go` | middleware | — | 不改动，仅作 auth.go 签名/风格参考 | — |

## Code Excerpts

### 1. PO 模板 — `internal/model/po/user.go` L14-55

```go
type UserPO struct {
	BaseWithOperator

	ID         int64   `gorm:"column:id;primaryKey"`
	Username   *string `gorm:"column:username"`
	Status     *int8   `gorm:"column:status"`
	Builtin    *bool   `gorm:"column:builtin"`
}

// TableName 显式表名（Tabler 接口）。
func (UserPO) TableName() string {
	return "sys_user"
}

func (u *UserPO) BeforeCreate(tx *gorm.DB) error {
	if u.ID == 0 {
		u.ID = int64(ids.NewID())
	}
	now := time.Now()
	u.CreateTime = &now
	u.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		u.CreateByID = &id.UserID
		u.CreateByName = &id.Name
		u.UpdateByID = &id.UserID
		u.UpdateByName = &id.Name
	}
	return nil
}
```

**复制什么/改什么：** 照抄结构（BaseWithOperator 内嵌 + 全指针列 + column tag + TableName + BeforeCreate 雪花/双时间/四操作人）；改为各域表名与列。审计基类见 `internal/model/po/base.go` L9-17。注意：审计更新走 repo 显式并入（见摘录 3），不要给 PO 加 BeforeUpdate。关联表（user_role）按 DDL 若无审计列则不内嵌 BaseWithOperator。

### 2. DTO 契约模板 — `internal/model/dto/user.go` L24-64, L67-71

```go
type UpdateUserReq struct {
	RealName *string       `json:"realName"`   // 全指针，nil=不写库，空串=显式清空
	DeptID   *ids.ID       `json:"deptId"`
	Gender   *types.Gender `json:"gender"`
	// 不定义任何审计字段键（ENG-02）
}

type UserResp struct {
	ID           ids.ID               `json:"id"`      // int64 ID JSON 字符串化
	Username     *string              `json:"username"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	// 全部键无 omitempty——未赋值以 null 输出（JacksonConfig 契约）
	// password/extToken 永不定义（WRITE_ONLY 语义）
}

func ToUserResp(u *po.UserPO) *UserResp {
	r := &UserResp{ID: ids.ID(u.ID), Username: u.Username, ...}
	if u.DeptID != nil { id := ids.ID(*u.DeptID); r.DeptID = &id }
	...
}
```

**复制什么/改什么：** 照抄三件套形态（CreateReq 必填 binding:required / UpdateReq 全指针 / Resp 无 omitempty + 手写 ToXxxResp 转换 int64→ids.ID、int8→types 枚举）。`types.Gender`/`types.UsageStatus`/`types.LocalDateTime` 序列化契约直接复用（`pkg/types`）。

### 3. Repository Patch/分页模板 — `internal/repository/user.go` L44-99

```go
func (r *UserRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil { cols = map[string]any{} }
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.UserPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil { return res.Error }
	if res.RowsAffected == 0 {
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.UserPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 { return errcode.ErrNotFound }
	}
	return nil
}

func (r *UserRepo) List(ctx context.Context, page, size int) ([]po.UserPO, int64, error) {
	q := func() *gorm.DB { return r.db.WithContext(ctx).Model(&po.UserPO{}) }
	var total int64
	if err := q().Count(&total).Error; err != nil { return nil, 0, err }
	offset := (page - 1) * size
	if offset < 0 { offset = 0 }
	err := q().Offset(offset).Limit(size).Order("create_time DESC, id DESC").Find(&users).Error
	...
}

// FindByID: gorm.ErrRecordNotFound → errcode.ErrNotFound（L102-112）
```

**复制什么/改什么：** 五域 repo 逐字照抄 Patch（审计显式并入 + CR-01 回读确认）与 List（闭包条件一致 + offset 防负 + 稳定排序）；换 PO 类型与（需要时）List 的过滤条件——过滤条件必须放进闭包 q()，保证 Count 与 Find 一致防 total 漂移。删除（硬删除）参考 FindByID 的 ErrNotFound 映射；builtin 守卫在 service 层先查后删（409 → `errcode.New(CodeInvalidParam/自定义码, "内置记录不可删除")`，HTTPStatus 映射在 `pkg/errcode` 内按需扩展——如需 409 需给 errcode 增码段与 HTTPStatus case）。

### 4. Service 模板 — `internal/service/user.go` L31-63, L69-116

```go
func (s *UserService) Create(ctx context.Context, req *dto.UserCreateReq) (*dto.UserResp, error) {
	u := &po.UserPO{Username: req.Username, ...}   // DTO→PO，服务端默认值在此层
	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {   // TranslateError 开启，1062→ErrDuplicatedKey
			return nil, errcode.New(errcode.CodeInvalidParam, "用户名已存在")
		}
		return nil, err
	}
	return dto.ToUserResp(u), nil
}

func (s *UserService) Update(...) {           // 全指针字段 nil→map 翻译（白名单）
	cols := map[string]any{}
	if req.RealName != nil { cols["real_name"] = *req.RealName }
	...
	if err := s.repo.Patch(ctx, id, cols); err != nil { return nil, err }
	return s.Get(ctx, id)                     // 落库后回读返回最新契约
}

func (s *UserService) List(...) (*paging.PageData[dto.UserResp], error) {   // 分页 envelope
	items := make([]dto.UserResp, 0, len(users))
	for i := range users { items = append(items, *dto.ToUserResp(&users[i])) }
	return &paging.PageData[dto.UserResp]{Items: items, Total: total, Page: page, Size: size}, nil
}
```

**复制什么/改什么：** 五域 service 照抄（唯一例外：Phase 2 起 Create 的 password 需 `bcrypt.GenerateFromPassword` 哈希化——Phase 1 注释已预留此边界）。用户详情两查询防 N+1（USER-05）：先 FindByID 用户，再按 roleIds 一次查 user_role/role，装配进 DTO——repo 加 `FindByUserID` 类方法，勿在循环内查库。角色分配/回收（`PUT /users/:id/roles` 全量替换）用 service 层 `db.Transaction(func(tx) ...)`：Delete 旧关联 + BatchCreate 新关联，tx 传给 `repository.NewXxxRepo(tx)` 复用（repo 注释 L21"tx 可作 db 注入"已预留）。

### 5. Handler 模板 — `internal/handler/user.go` L33-47, L50-61, L96-114

```go
func bindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			resp.Fail(c, errcode.New(errcode.CodeValidation,
				fmt.Sprintf("请求体超过 %dMB 上限", middleware.MaxBodyMB)))
			return false
		}
		resp.Fail(c, errcode.New(errcode.CodeValidation, "参数校验失败: "+err.Error()))
		return false
	}
	return true
}

func (h *UserHandler) List(c *gin.Context) {
	page, size := paging.Parse(c.Query("page"), c.Query("size"))
	data, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil { resp.Fail(c, err); return }
	resp.OK(c, data)
}

func parseUserID(c *gin.Context) (int64, bool) {   // 路径参数解析样板，可泛化为 parseID(c, "无效的xx ID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "无效的用户 ID"))
		return 0, false
	}
	return id, true
}
```

**复制什么/改什么：** 五域 handler 逐字照抄薄层三段式（parse :id → bindJSON → svc → resp.OK/Fail）。bindJSON/parseUserID 已是包级函数，直接复用勿重复定义。auth handler（登录）无 :id，仅 bindJSON 路径。Excel 导入 handler 用 `c.FormFile("file")` → 打开 multipart 交给 service（multipart 尺寸仍受全局 BodyLimit 1MB 约束）。

### 6. 中间件风格 — `internal/middleware/requestid.go` L19-36

```go
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		...
		c.Set("request_id", rid)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDCtxKey{}, rid))
		c.Next()
	}
}
```

**复制什么/改什么：** auth 中间件照抄此形态（闭包构造器注入 session manager：`func Auth(sm *auth.SessionManager) gin.HandlerFunc`）；从 `Authorization: Bearer <token>` 提取 token → `sm.Validate(ctx, token)` → 未命中/过期 `resp.Fail(c, errcode.ErrUnauthenticated)`（HTTPStatus 已映射 401）并 `c.Abort()` → 命中后 `auth.WithIdentity(...)` 注入 ctx + `sm.Touch` 续期 + `c.Next()`。权限拒绝路径（Phase 3 用）预留 `errcode.ErrForbidden`（403 已映射）。白名单路径（`/api/v1/auth/login`、`/api/v1/auth/apikey-login`、`/healthz`、`/readyz`）在 router 装配处分组解决（白名单组不挂 Auth），中间件自身不判路径。

### 7. 路由收紧点 — `internal/app/router.go` L13-32（现状，待改）

```go
func newRouter(h *health.State, userHandler *handler.UserHandler, logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.RequestID())
	engine.Use(middleware.AccessLog(logger))
	engine.Use(middleware.Recovery(logger))
	engine.Use(middleware.BodyLimit())

	pub := engine.Group("/")
	pub.GET("/healthz", h.Liveness)
	pub.POST("/api/v1/users", userHandler.Create)   // ← D-02 临时态：Phase 2 在此收紧
	...
}
```

**复制什么/改什么：** newRouter 签名扩参（authHandler + 其余 4 域 handler + authMiddleware）；`pub` 组仅留 `/healthz`、`/readyz`、`POST /api/v1/auth/login`、`POST /api/v1/auth/apikey-login`；新增 `pri := engine.Group("/", authMiddleware)` 挂五域全部业务路由 + `POST /api/v1/users/import` + `PUT/GET /api/v1/users/:id/roles` + `GET /menus/tree`、`GET /depts/tree`。中间件注册顺序保持 RequestID→AccessLog→Recovery→BodyLimit→Auth。

### 8. 装配链扩展点 — `internal/app/app.go` L39-63

```go
db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{ ... })
...
userRepo := repository.NewUserRepo(db)
userSvc := service.NewUserService(userRepo)
userHandler := handler.NewUserHandler(userSvc)
engine := newRouter(h, userHandler, logger)
```

**复制什么/改什么：** 在 gorm.Open 与连接池设置之后加 `redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, ...})`（go-redis v9；启动 ping fail-fast 对齐 app.New 的 err 返回风格）→ `sessionMgr := auth.NewSessionManager(rdb, cfg.Auth.SessionTimeout)` → 五域 repo/svc/handler 链式构造 → 扩参 newRouter。Stop() 关停链追加 `rdb.Close()`（对齐 L109-111 sqlDB.Close 模式）。

### 9. 配置段扩展 — `internal/config/config.go` L21-29, L67-71, L101-127, L128-143

```go
const (
	DefaultPort = 8080
	DefaultLogLevel = "info"
	// Phase 2 新增：DefaultSessionTimeout = 30 * 24 * time.Hour（Sa-Token 官方默认 30d 代理值）
)
var envKeyAliases = map[string]string{
	"IAM_MYSQL_MAX_OPEN_CONNS": "mysql.max_open_conns",
	// Phase 2 新增：多段 snake_case 键（如 IAM_AUTH_SESSION_TIMEOUT→auth.session.timeout
	// 恰好三步链可映射；IAM_REDIS_ADDR→redis.addr 同样可映射，仅确需别名时才登记）
}
// Load(): if k.Exists("auth.session.timeout") { ... } + fail-fast 校验
// （timeout 必须 >0；redis.addr 空则报错或按需默认）
```

**复制什么/改什么：** 照抄"默认值常量 → Config struct 字段 → defaults() → k.Exists 读取 → Load 末尾 fail-fast 校验"五步链；`configs/config.yaml` 同步加 `auth:` 与 `redis:` 段（对齐现有 yaml 注释风格，注明 env 覆盖方式）。`internal/config/config_test.go` 已有五类 env 覆盖/校验测试模板（L22-113）可扩展。

### 10. 集成测试模板 — `internal/handler/user_integration_test.go` L36-60, L156-185 + `internal/testutil/mysql.go`

```go
func TestCreateUserContract(t *testing.T) {
	ts, dsn := newTestApp(t)
	reqBody := `{...}`
	statusCode, bodyBytes := doRequest(t, "POST", ts.URL+"/api/v1/users", reqBody)
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	require.Equal(t, 0, envelope.Code)
	...
}

func newTestApp(t *testing.T) (*httptest.Server, string) {
	dsn, stopContainer, err := testutil.RunMySQL(ctx)
	cfg := &config.Config{...}
	application, err := app.New(cfg, logger)
	idsOnce.Do(func() { ids.SetDefault(node) })   // 雪花单例只装配一次
	ts := httptest.NewServer(application.Engine())
	t.Cleanup(func() { ts.Close(); application.Stop(); stopContainer() })
	return ts, dsn
}
```

**复制什么/改什么：** 全部 `//go:build integration` tag + newTestApp 复用（Phase 2 需扩展为注入 Redis：testutil 新增 `RunRedis(ctx)`（testcontainers redis 模块或 miniredis 分层——单元用 miniredis、集成用容器），newTestApp 增设 `cfg.Redis.Addr` 与 `cfg.Auth.SessionTimeout`；auth 中间件生效后既有 user 域测试的 doRequest 需带登录 token——建议 newTestApp 返回 helper `loginAs(t, ts, username)` 或工厂直接建用户+登录拿 token）。testutil/mysql.go 的容器-清理-镜像探测三件套照抄进 redis.go。

### 11. 可直接复用的横切件（不新建，直接 import）

- `pkg/paging`（`paging.Parse` 钳位 + `PageData[T].MarshalJSON` 空列表→[]，`pkg/paging/paging.go` L24-34/L59-70）— 全部五域 List
- `pkg/errcode`（错误码分段/单例/HTTPStatus 映射，`pkg/errcode/errcode.go` L5-59）— 已有 `CodeBadCredentials=10501→401` 与 `ErrUnauthenticated→401`，登录/认证直接用；如需 409（builtin 守卫）需在此文件增码段 + HTTPStatus case
- `pkg/resp`（OK/Fail envelope 统一出口）— 所有 handler
- `pkg/ids`（雪花单例）、`pkg/types`（Gender/UsageStatus/LocalDateTime 契约）
- `internal/auth/identity.go`（`WithIdentity`/`FromContext`）— auth 中间件注入与 repo 审计读取，勿另建 context key

## New Categories（无类比文件的包风格指引）

| 新文件 | 风格参考 | 指引 |
|--------|----------|------|
| `internal/auth/session.go`（会话管理器） | `internal/auth/identity.go` 包风格 + `internal/repository/user.go` 结构体-构造器风格 | 放入既有 `internal/auth` 包（与 identity.go 同包，identity 即其注入物）。`type SessionManager struct { rdb redis.Universer; timeout time.Duration }`；方法面：`Login(ctx, payload) (token, error)` / `Validate(ctx, token) (Identity, error)` / `Touch(ctx, token)` / `Logout(ctx, token)` / `KickoutUser(ctx, userID)` / `BindAPIKey(ctx, keyID, token)`。token 生成用 `crypto/rand` 32B hex（`internal/middleware/requestid.go` L23-29 已有 crypto/rand+hex 样板可仿）。Redis 键常量集中定义；写 session Hash 与 user-tokens Set 用 TxPipeline；一切读后写走 Lua 或 pipeline 保持原子性由 PLAN 决策。单元测试用 miniredis（纯 Go，无 build tag），集成用容器 |
| Excel 导入（`internal/service/user_import.go` 方法 + handler 端点） | service 层方法风格（摘录 4） | 新依赖 excelize；`excelize.OpenReader`→`GetRows` 逐行；表头按列索引严格匹配（不符立即整批拒绝）；逐行三键 upsert（name+bitIndex+location 全匹配→UPDATE 否则 INSERT），单行失败记 `{row, reason}` 继续；行数 >10000 拒绝；返回 `ImportResult{Total, Inserted, Updated, Failed int64; Errors []RowError}` DTO。密码哈希与 Create 同路径 |
| 树构建器（如 `internal/service/tree.go` 或 `internal/model/dto` 内装配） | `pkg/paging` 单一职责小包风格（包注释声明用途 + 泛型） | 建议 `func BuildTree[T](items []T, idOf, parentOf func(T) int64, attach func(T, *[]T)) []T` 泛型一函共用 menu/dept；构建：一次遍历建 map[id]→children，二次遍历挂接；parentID 为 nil/0 或指向不存在节点时提升为根（DEPT-02 验收明文）；输出按 sort 排序保证稳定 |

## Metadata

- **Analog search scope:** `internal/{model,repository,service,handler,middleware,auth,config,app,testutil}`, `pkg/*`, `configs/`, `scripts/ddl/`
- **Files scanned:** 33 tracked Go files（git ls-files 全量核验，无 gitignored 镜像路径）
- **Pattern extraction date:** 2026-10-31
