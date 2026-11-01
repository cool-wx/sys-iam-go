# Phase 4: 业务域与操作日志 - Context

**Gathered:** 2026-11-01
**Status:** Ready for planning

<domain>
## Phase Boundary

其余六个功能域全部可用：字典/系统配置开箱即用，文件存储可在本地磁盘与 RustFS 间配置切换，操作日志自动记录，排班与故障信息含 Excel 导入。具体交付：

- **轻 CRUD 三域**：DictType/DictData（含按类型查数据集）、SysConfig、Shift（全量查询无分页——源行为）
- **文件域**：FileInfo 上传（MD5 流式判重 + categoryId IS NULL 分支）/流式下载/宽容删除 + FileCategory CRUD + FileStorage 抽象（本地/S3 可配置切换）
- **操作日志**：中间件自动记录写操作（module/operation 元数据 + body 回放 + 敏感掩码 + 异步落库）+ 分页查询（1/10 钳位例外）
- **故障域**：FaultInfo CRUD + Excel 三键 upsert 导入（整批事务 + ImportResult 契约 + 源导出文件回测）

不含：dataScope 在六域的应用（需求未要求，通用件已备）、名称同步事件（Phase 5）、userInfo/routes 端点（deferred）

需求项：DICT-01..03、CONF-01、FILE-01..05、OLOG-01..03、SHFT-01/02、FAUL-01/02（16 项）

</domain>

<decisions>
## Implementation Decisions

> 依据 2026-10-31 用户全量自主授权，灰区由 Claude 自主拍板（Phase 1-3 先例一致）。

### 计划切片（Claude 自主决策）

- **D-01:** 4 plans 按域聚簇——① Dict+Config+Shift（轻 CRUD 三域，Phase 2 模板复用最重）② File+FileCategory（存储抽象重域）③ Olog（中间件+查询）④ Fault+Excel；tracer 为 Plan ①（快速立骨架）
- **D-02:** wave 1→2→3→4 串行链（router.go/app.go 等共享文件跨 plan 必然重叠，串行消除竞态）
- **D-03:** 本阶段零 dataScope 应用（需求未要求；internal/datascope 通用件已就绪，v2 各域按需挂接）
- **D-04:** 六域全部挂 requirePerm（Phase 3 收口纪律延续——新路由漏挂会被 RouteLockdown 式结构断言暴露；perm 串沿 system: 前缀，菜单无对应项的域（如 fault/shift）研究期核对源 sys_menu 后定）

### 操作日志中间件（Claude 自主决策）

- **D-05:** 记录范围路由注册时显式声明——module/operation 元数据随路由挂载（等价源 @OperationLog 注解心智模型；不全局自动记读操作）
- **D-06:** body 回放用缓存字节重放（`c.Request.Body = io.NopCloser(bytes.NewReader(cached))`；1MB BodyLimit 上限内缓存无内存放大）
- **D-07:** 异步落库：buffered chan + 后台 goroutine 批量写；chan 满丢弃不阻塞业务（OLOG-02 "落库失败静默不影响业务"的结构化实现）；失败请求也记（status=0 + errorMsg 截 500）
- **D-08:** 优雅关停 flush：SIGTERM 时 drain chan 残留（app.Shutdown 链复用 Phase 1 NotifyContext）——丢弃窗口最小化
- **D-09:** 敏感参数掩码——**研究实证修正为 8 字段**（源 OperationLogAspect.java:27-28 正则逐字：password/oldPassword/newPassword/secretKey/apiKey/token/accessToken/refreshToken，掩码 `******`；REQUIREMENTS"7 字段"表述系计数笔误）；另两处源差异裁定：源为**同步落库**——D-07 异步为本项目工程化改进（chan 满丢弃优于请求阻塞，留档有意偏离）；源切点**含读操作**——按 D-05 收紧为仅写操作（注解声明制）；分页 1/10 钳位例外（handler 局部实现，不改 pkg/paging 全局默认）

### 文件存储双实现（Claude 自主决策）

- **D-10:** `FileStorage` 接口（Put/GetStream/Delete）双实现：LocalStorage（os 文件流）+ S3Storage（minio-go v7.4.1，STACK 锁定）；`storage.driver: local|s3` 配置切换，local 模式延迟初始化不加载 minio
- **D-11:** 本地默认路径 `storage.local.root` 默认 `./data/files`（相对工作目录，systemd WorkingDirectory 联动；绝对路径可配）——Phase 1 UAT"ProtectHome 与存储路径 Phase 4 复核"结论：v1 unit 维持不加 ProtectHome（现场账号未定先例），路径可配已足够
- **D-12:** MD5 流式判重：io.TeeReader 边写存储边算哈希（不二次读）+ sys_file_info.md5 查询；categoryId IS NULL 判重分支照源保真（FILE-01 验收明文）
- **D-13:** 下载 io.Copy 流式 + Content-Length（FILE-02 工控机增强——RSS 平稳验收）；删除时存储端失败宽容继续删 DB 记录（FILE-03 源行为，日志留痕）

### Excel 导入与杂项（Claude 自主决策）

- **D-14:** 源导出文件回测：研究期探查源库/源项目导出样例；不可得则按源 importExcel 列序逐列提取构造等价 xlsx 回测（构造依据文档化，不发明格式）
- **D-15:** FAUL-02 三键 upsert（name+bitIndex+location 全匹配→UPDATE 否则 INSERT）整批单事务；ImportResult{insertCount,updateCount} 契约（FEATURES.md:205）；表头按列索引严格匹配 + 行数上限（Phase 2 CONTEXT 的防线设计移植——excelize 依赖本阶段引入）
- **D-16:** Shift 全量查询无分页（SHFT-01 源行为直译）；文件下载/上传路由挂认证但 perm 串研究期核对（源菜单若无对应项，仅 super_admin 可用为 v1 边界沿 Phase 3 先例）

### Claude's Discretion

- Dict/Config/Shift 的 DTO 字段与响应形态（研究按源逐列提取）
- FileCategory 树形态（若源是树则复用 internal/tree，平表则平表）
- Olog 表结构列名（沿源 sys_operation_log）
- excelize 读取方式（GetRows vs Rows 流式——行数上限内 GetRows 简单优先）
- 测试组织（newTestApp 沿既有；File 域 S3 用 minio-go 的 testcontainers 或接口 mock——研究定夺）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- Phase 2 五域模板全套（po/dto/repo/service/handler + 集成测试模式）——轻 CRUD 三域直接套
- `internal/middleware/`（RequestID/AccessLog/Recovery/BodyLimit 四件套 + RequirePerm）——Olog 中间件第五件
- `internal/authz`（enforcer/perm 常量）——六域新路由挂载
- Phase 1 `pkg/paging`（1/10 特例参数）、`pkg/resp`、雪花 ID、审计填充
- `internal/app/app.go` 装配链（Olog 后台 goroutine 生命周期挂 Shutdown）

### Established Patterns

- repo.Patch 白名单更新、两查询防 N+1、builtin 守卫族（若源域有 builtin 概念则沿用）
- 指针列 PO + TableName 显式映射 + 禁 AutoMigrate
- newTestApp per-test 容器；集成断言含 Phase 1-3 全量回归

### Integration Points

- `internal/app/router.go`：六域路由注册 + Olog 中间件挂写操作路由
- `configs/config.yaml` + `internal/config/config.go`：storage 段新增（driver/local.root/s3 endpoint 等）
- `scripts/ddl/`：六域表 DDL（本机 15 表已有——研究核对列名；若 testcontainers 已加载则零新增）
- `internal/app/app.go`：FileStorage 构造 + Olog chan/goroutine + Shutdown flush

</code_context>

<specifics>
## Specific Ideas

- FILE-01 的 categoryId IS NULL 判重分支是"隐藏坑"验收明文（FEATURES.md:207 IS NULL 判重分支是隐藏坑）
- OLOG-02 的 7 个敏感字段必须逐字段保真（研究从源 OperationLogAspect 提取，字段名清单进 plan）
- FAUL-02 "用源系统真实导出文件回测"是验收明文——研究必须给出可得性结论
- 文件下载大文件 RSS 平稳是 FILE-02 可观察验收（工控机内存约束）

</specifics>

<deferred>
## Deferred Ideas

- dataScope 在六域的应用（v2 按需）
- userInfo/routes 端点（前端联动需要时）
- 文件预签名 URL（RustFS 场景 v2）
- Olog 查询的 dataScope 过滤与导出（v2）
- sys_role_menu/sys_role_dept 唯一键 DDL（迁移清单挂账项）

</deferred>
