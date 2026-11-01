---
phase: "04"
---

# Phase 4: 业务域与操作日志 - Pattern Map

**Mapped:** 2026-11-01
**Files analyzed:** 10 类（约 30+ 文件）
**Analogs found:** 8 / 10 类有精确类比；2 类（FileStorage 抽象、Olog 中间件）为新类别，给包组织指引

## File Pattern Map

| 新文件类别 | Role | Data Flow | Closest Analog | Match Quality |
|-----------|------|-----------|----------------|---------------|
| dict/dictData/config/shift + file/fileCategory + fault + olog 的 po/dto/repo/service/handler 全套（~26 文件） | po/dto/repo/service/handler 四层 | CRUD / request-response | `internal/model/po/menu.go` → `internal/handler/menu.go` 全链 | exact |
| olog 查询（分页列表） | repo+handler | request-response（分页） | `internal/repository/user.go` List + `internal/handler/user.go` List + `pkg/paging` | exact |
| `internal/storage/storage.go` + `local.go` + `s3.go` | service（存储抽象） | file-I/O | 无精确类比；包组织参照 `internal/datascope/datascope.go`（internal 通用件单文件+文档头） | new category |
| `internal/handler/file.go` 上传 | handler | multipart/file-I/O | handler 薄层结构照 menu.go；multipart 读法参照 `internal/middleware/body_limit.go` 的 MaxBytesReader 语义 | role-match |
| `internal/handler/file.go` 下载 | handler | streaming | 无类比（io.Copy 流式响应新模式） | no analog |
| `internal/middleware/oplog.go` | middleware | event-driven（异步落库） | 结构照 `internal/middleware/access_log.go`；chan/goroutine 生命周期照 `internal/app/app.go` Stop 链 | role-match |
| fault Excel 导入方法 | service | batch/transform | 无类比（excelize 新依赖，按 RESEARCH.md 集成） | no analog |
| `internal/app/router.go` 六域注册 + Olog 挂载 | route | — | 现文件修改（摘录见下） | exact |
| `internal/app/app.go` FileStorage/chan/goroutine/Shutdown | provider | — | 现文件修改（摘录见下） | exact |
| `internal/config/config.go` storage 段 + `configs/config.yaml` | config | — | 现有 redis 段模式（摘录见下） | exact |
| `scripts/ddl/` 六域表（sys_dict_type/sys_dict_data/sys_config/sys_shift/sys_file_info/sys_file_category/sys_operation_log/sys_fault_info） | migration | — | `scripts/ddl/sys_api_key.sql` | exact |
| 各域集成测试 + newTestApp 扩展 | test | — | `internal/handler/menu_dept_integration_test.go` | exact |

## Pattern Assignments

### 1. 轻 CRUD 三域（Dict/DictData/Config/Shift）+ Olog/FileCategory/Fault 常规部分

**Analog:** `internal/model/po/menu.go` → `internal/model/dto/menu.go` → `internal/repository/menu.go` → `internal/service/menu.go` → `internal/handler/menu.go`

**复制什么：**
- **PO**（menu.go:15-53）：`BaseWithOperator` 内嵌 + 指针原生类型列 + 显式 column tag + `TableName()` 显式表名 + `BeforeCreate` 雪花 ID/双时间/ctx identity 四操作人填充。逐字复用 BeforeCreate，只换列与表名。
- **Repo**（menu.go:28-71）：`Create`（WithContext 透传 ctx）+ `Patch`（白名单 map 更新、显式并入 update_time/操作人列、恒带 `Where("id = ?")`、RowsAffected==0 时 Count 二次确认 CR-01）+ `Delete`（物理删，不校验存在性）+ `FindByID`（gorm.ErrRecordNotFound → errcode.ErrNotFound）。
- **Service**（menu.go:31-66, 73-137）：Create 中 DTO→PO + 服务端默认值；Update 的 nil-skip 白名单 cols map 构建模式；NotFound 错误译为 `errcode.New(errcode.CodeNotFound, "XX不存在: %d")`；落库后回读返回（`return s.Get(ctx, id)`）。
- **Handler**（menu.go:28-35, 38-49）：`parseID(c, label)` 通用路径参数解析（新建域 handler 统一走此函数，注释明说）；薄层三段式 bindJSON→svc→resp.OK/resp.Fail。
- **测试**：`internal/handler/menu_dept_integration_test.go` 的 `newTestApp` + `doRequest/doAuthRequest` 样板。

**改什么：** Shift 全量查询无分页 → repo 照 `MenuRepo.List`（menu.go:76-85，`Order("sort ASC, id ASC")` 换成源列序）；DictData 按类型查数据集 → 照 FindByIDs 的 `Where("dict_type_id = ?")` 简单条件变体。

### 2. Olog 分页查询域

**Analog:** `internal/repository/user.go:84-106` + `internal/handler/user.go:95-103` + `pkg/paging/paging.go:22-48`

**Repo 分页核心（user.go:96-106）——Count 与查询同一闭包，total 不漂移：**
```go
func (r *UserRepo) List(ctx context.Context, page, size int, filter datascope.Filter) ([]po.UserPO, int64, error) {
	q := func() *gorm.DB {
		return filter.Apply(r.db.WithContext(ctx).Model(&po.UserPO{}), "sys_user")
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&users).Error
```
Olog 域无 dataScope（D-03），闭包内去掉 `filter.Apply` 只留 `r.db.WithContext(ctx).Model(&po.OperationLogPO{})` + 条件查询即可，闭包结构保留。

**Handler 分页入口（user.go:96-97）：**
```go
page, size := paging.Parse(c.Query("page"), c.Query("size"))
data, err := h.svc.List(c.Request.Context(), page, size)
```

**分页 envelope（paging.go:50-54）：** `paging.PageData[T]{Items, Total, Page, Size}` 泛型结构，nil items 自动 marshal 为 `[]`。service 层构造 `paging.PageData[*dto.OlogResp]{...}` 返回。

**1/10 钳位例外（D-09）：** paging.Parse 全库缺省 1/15——Olog 的 1/10 特例**不要改 pkg/paging**，在 olog handler 内自行钳位（缺省 page=1、size=10，同样的 clamp 逻辑局部实现），注释标注"OLOG-02 源缺省 1/10 例外"。

### 3. Olog 中间件（新类别）

**结构 Analog:** `internal/middleware/access_log.go:14-26` —— 函数式 gin.HandlerFunc + `c.Next()` 前后分段 + slog 结构化字段：
```go
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http", ...)
	}
}
```

**body 缓存重放（D-06）：** 在 `c.Next()` 前读 body（BodyLimit 已限 1MB，见 `internal/middleware/body_limit.go:25-31`——`http.MaxBytesReader` 已包裹 body，oplog 读取超限同样得 MaxBytesError）：
```go
var cached []byte
if c.Request.Body != nil {
	cached, _ = io.ReadAll(c.Request.Body)
	c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(cached))
}
```

**D-05 路由显式声明：** 中间件参数化 module/operation 元数据，形如 `middleware.Oplog(logger, sink, "用户管理", "新增用户")`，路由注册处逐条包在 RequirePerm 之内/之外（plan 定夺次序），而非全局 engine.Use。

**异步落库 + 优雅关停（D-07/D-08）生命周期 Analog:** `internal/app/app.go:195-210` Stop 链——oplog sink 应有 `Close()/Flush()` 方法挂在 `App.Stop` 内 `srv.Shutdown` 之后（drain chan 残留后关 DB）。chan 满丢弃：
```go
select {
case ch <- entry:
default: // chan 满丢弃，不阻塞业务（OLOG-02）
}
```
失败请求也记：`c.Next()` 之后读 `c.Writer.Status()`，非 2xx 时填 errorMsg（截 500 字节）。

### 4. FileStorage 抽象（新类别，包组织指引）

- 新包 `internal/storage/`，单文件接口 `storage.go` 定义：
```go
type FileStorage interface {
	Put(ctx context.Context, key string, r io.Reader) (int64, error) // 返回写入字节数
	GetStream(ctx context.Context, key string) (io.ReadCloser, int64, error) // reader + Content-Length
	Delete(ctx context.Context, key string) error
}
```
- `local.go`：`os.CreateTemp/os.OpenFile` + `io.Copy`；`s3.go`：minio-go v7.4.1 `PutObject/GetObject/RemoveObject`（RESEARCH.md 给集成细节）。
- 包文档头参照 `internal/datascope/datascope.go:1-21` 风格：多段注释说明设计决策（driver 切换、local 延迟初始化不加载 minio——D-10）。
- 延迟初始化：local 模式下 `storage.New` 只返回 LocalStorage，不 import 触发 minio 连接；S3Storage 构造仅在 `cfg.Storage.Driver == "s3"` 时调用。

### 5. File 域上传/下载

- **上传 handler**：薄层结构照 menu.go；multipart `c.FormFile` + 打开文件流交给 service；MD5 判重用 `io.TeeReader`（D-12）边写边算——service 层模式：
```go
h := md5.New()
n, err := stor.Put(ctx, key, io.TeeReader(file, h))
sum := hex.EncodeToString(h.Sum(nil))
```
之后查 `sys_file_info.md5`（categoryId IS NULL 分支照源保真，FILE-01）。
- **下载（无类比，新模式）**：handler 直接流式——`stor.GetStream` 取 reader 后：
```go
c.DataFromReader(http.StatusOK, reader, contentType, filename,
	map[string]string{"Content-Length": strconv.FormatInt(size, 10)})
```
或 `io.Copy(c.Writer, reader)` + 手写 header；禁止先 `io.ReadAll` 全量入内存（FILE-02 RSS 验收）。删除宽容（D-13）：存储端 Delete 失败仅 slog.Warn 留痕，继续删 DB 行。
- **BodyLimit 独立上限：** body_limit.go:12-13 注释明示"大 payload 域如 Excel 导入应另设独立上限，不复用全局值"——文件上传/Excel 导入路由需独立更大的 MaxBytesReader（plan 定数值）。

### 6. Fault Excel 导入（无类比，按 RESEARCH.md）

service 方法形态：`Import(ctx, reader io.Reader) (*dto.ImportResult, error)`；三键 upsert 整批 `tx := s.db.WithContext(ctx).Transaction(...)`（app.go:49 已 `SkipDefaultTransaction: true`，service 必须显式 Transaction——参照 `service/role.go` 的 RoleService 持有 db 字段做事务的构造形态 `service.NewRoleService(db, ...)`，app.go:140）；ImportResult{insertCount, updateCount} 契约。表头按列索引严格匹配 + 行数上限（D-15）。

### 7. router.go 六域注册 + Olog 挂载（现文件修改）

**Analog:** `internal/app/router.go:15-22`（handlers 聚合结构加字段）+ 72-77（域注册模板）：
```go
pri.POST("/api/v1/menus", middleware.RequirePerm(az, authz.PermMenuAdd), hs.Menus.Create)
pri.GET("/api/v1/menus/:id", middleware.RequirePerm(az, authz.PermMenuQuery), hs.Menus.Get)
```
六域照此逐条 `RequirePerm(az, authz.PermXxxYyy)`——**perm 常量新增进 `internal/authz/perms.go`**（照既有 `PermMenuAdd` 等命名，D-04 system: 前缀沿既有串）。Olog 挂载：写操作路由逐条包 `middleware.Oplog(...)` 元数据（D-05），不全局 Use。下载/上传路由挂认证但 perm 研究期核对（D-16）。

### 8. app.go 装配（现文件修改）

**Analog:** `internal/app/app.go:131-156`——repo→svc→handler→handlers 结构逐域追加（照 menuRepo 三行模式）；FileStorage 构造插在 service 构造之前（照 enforcer 先例 app.go:84-87 的 fail-fast `return nil, fmt.Errorf(...)` 风格）；oplog chan+goroutine 启动照 enforcer 段位置，`Stop()`（app.go:196-210）内追加 oplog flush（Shutdown 之后、DB close 之前）。

### 9. config storage 段（现文件修改）

**Analog:** `internal/config/config.go:65-70`（RedisConfig 结构）+ 87-94（envKeyAliases 多段 snake_case 登记——`storage.local_root`、`storage.s3_*` 若为多段下划线叶键**必须登记**否则覆盖静默丢失 WR-01）+ 144-146（`k.Exists` 单键读取）+ 185-187（空值 fail-fast 校验）：
```go
if cfg.Redis.Addr == "" {
	return nil, fmt.Errorf("redis.addr must not be empty")
}
```
`configs/config.yaml` 照 redis 段（16-19 行）风格加 storage 段：`driver: local`、`local.root: ./data/files`（D-11）。

### 10. DDL 文件

**Analog:** `scripts/ddl/sys_api_key.sql`（全文 25 行）——头部三行注释（来源声明：本机容器实测 SHOW CREATE TABLE）、`DROP TABLE IF EXISTS` + `CREATE TABLE`、审计 6 列 + remark 固定形态、无外键（T-1-07）、utf8mb4_general_ci。列名以本机 mysql-server 容器实测为准（MEMORY：15 表已导入，含缺列清单——research 期核对六域表是否已在其中）。

### 11. 集成测试

**Analog:** `internal/handler/user_integration_test.go:174-222` newTestApp。扩展点：File 域需给 `cfg` 加 `Storage` 段（local 模式 + `t.TempDir()` 作 root——**这是 newTestApp 签名/内部最小改动点**）；S3 用 testcontainers minio 或接口 mock 由 research 定夺（CONTEXT Discretion）。测试断言样板：`doAuthRequest` + require/assert 分层。Olog 测试需断言异步落库——注意 goroutine 时序，测试内等待/轮询或提供同步 flush 钩子。

## Shared Patterns

### 审计填充（全部六域 PO）
**Source:** `internal/model/po/menu.go:39-53` + `internal/repository/menu.go:37-46`（Patch 路径显式并入）
照抄 BeforeCreate + Patch 审计段，零改动换列名。

### 错误处理（全部 service/repo）
**Source:** `internal/repository/menu.go:88-98` + `internal/service/menu.go:141-150`
repo 层 ErrNotFound 哨兵 → service 层译为 `errcode.New(errcode.CodeNotFound, "XX不存在: %d")` → handler 层 `resp.Fail(c, err)` 统一 envelope。

### 权限挂载（全部新路由）
**Source:** `internal/app/router.go:72-77` + `internal/authz/perms.go`
新域 perm 常量 + 逐路由 RequirePerm；漏挂会被 Phase 3 结构断言暴露。

### 分页 envelope
**Source:** `pkg/paging/paging.go`
Olog 专用（其余域无分页）；1/10 钳位在 handler 局部实现，不改公共件。

## No Analog Found

| 文件/能力 | Role | Data Flow | Reason | 指引 |
|------|------|-----------|--------|------|
| 下载流式响应 | handler | streaming | 代码库首个大 payload 流式出站 | io.Copy / DataFromReader，禁全量读内存（FILE-02 RSS 验收） |
| Excel 导入 | service | batch | excelize 新依赖，首例 | RESEARCH.md 集成；显式 Transaction（SkipDefaultTransaction=true）；独立 BodyLimit 上限（body_limit.go:12 注释预留） |
| 后台 goroutine 生命周期 | middleware | event-driven | 首例常驻 goroutine | chan buffered + select-default 丢弃 + App.Stop drain（照 app.go Stop 链次序） |

## Metadata

**Analog search scope:** internal/{model,repository,service,handler,middleware,app,config,testutil,datascope}, pkg/paging, scripts/ddl, configs/
**Files read in full/part:** 14
**Pattern extraction date:** 2026-11-01
