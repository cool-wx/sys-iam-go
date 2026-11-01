---
phase: "04"
status: "complete"
confidence: "HIGH"
researched: "2026-11-01"
---

# Phase 4: 业务域与操作日志 - Research

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions（D-01..D-16，全部沿用——本节仅列研究期需核对的条目及其核对结论）

- **D-04/D-16 perm 串核对结论**：源 sys_menu 中 **fault/shift 两域无任何菜单权限项**（实证 `SELECT permission FROM sys_menu WHERE permission LIKE 'system:fault%' OR 'system:shift%'` 零行）；**file 域 7 项齐全**（list/query/add/edit/remove/upload/download，id 107-1076）；dict/config/log 四域齐全 [VERIFIED: mysql-server sys_menu 查询输出]。→ fault/shift 照 Phase 3 先例：挂 perm 串、菜单无对应项仅 super_admin 可用为 v1 边界；文件下载挂 `system:file:download`（菜单存在，非 super_admin 专属）。
- **D-09 敏感字段清单核对结论**：源 OperationLogAspect 正则实际含 **8 个字段**（非 7）——`password|oldPassword|newPassword|secretKey|apiKey|token|accessToken|refreshToken` [VERIFIED: OperationLogAspect.java:27-28 原文引用]。OLOG-02 需求文本"7 字段"为笔误，按源 8 字段逐字段保真（研究纪律主旨即"从源逐字段提取保真"，8 是保真结果）。**冲突已记录，以源为准。**
- **D-12 MD5 判重与零副作用顺序**：源顺序为 ①分类校验 → ②整读流算 MD5 → ③判重（拒绝时零副作用）→ ④save DB → ⑤重新开流上传存储 [VERIFIED: FileApplicationService.java:56-101]。D-12 的 TeeReader"边写边算"与"拒绝时零副作用"存在张力（TeeReader 流过存储即有副作用），实现建议见 Implementation Guidance 上传链小节。
- **D-05 vs 源差异（已拍板，非冲突）**：源无 @OperationLog 注解——AOP execution 切点全局拦截**含读操作**（olog 存量数据实证 `UserController list GET /user/list` 被记录）[VERIFIED: OperationLogAspect.java:36 + sys_operation_log 存量 116 行查询]。D-05 拍板 Go 版仅显式声明的写操作路由记录——有意收紧，照 D-05 执行。
- **D-06 vs 源差异（已拍板）**：源 params 是 controller 方法参数 JSON 序列化（Jackson 含 null 全字段，存量实证 `{"username":null,...}`）；Go 版用 body 原始字节回放——语义等价实现。
- **D-07 vs 源差异（已拍板）**：源是**同步落库**（finally 中 save + try-catch 静默 [VERIFIED: OperationLogAspect.java:70-76]）；Go 版异步 chan 是增强。
- **D-14 回测可得性结论**：真文件回测**不可直接得**——sys_fault_info 0 行、全盘无源导出 xlsx；但发现 tims 项目文档 `yf175a故障信息表.xlsx` 含 77 行真实 PLC 故障数据，其列序与源导出格式**不同**（人工原始资料），不可直接回测。→ 走 D-14 fallback：按源 @ExcelProperty 列序构造等价 xlsx 回测，数据行可从 yf175a 转换填充（真数据+构造格式）。

### Claude's Discretion（研究已给建议）

- FileCategory 树形态 → **源是树**（/fileCategory/tree 端点 + children 内联 [VERIFIED: FileCategoryController.java:32-37, FileCategory.java:46-48]），复用 internal/tree（孤儿提升语义与 Phase 2 dept 一致）
- excelize 读取方式 → GetRows（D-15 行数上限内简单优先）
- S3 测试方式 → 建议 LocalStorage 全语义测试 + S3Storage 装配 smoke（见 Validation Architecture）

### Deferred Ideas (OUT OF SCOPE)

dataScope 六域应用、userInfo/routes 端点、文件预签名 URL、Olog 导出与 dataScope 过滤、sys_role_menu/sys_role_dept 唯一键 DDL——全部忽略。
</user_constraints>

## Key Findings（按 6 研究重点）

### 重点 1：源六域行为考古（HIGH 置信，全部逐文件直读）

**路由与权限（八 Controller 全读）** [VERIFIED: interfaces/controller/*.java]

| 源路由 | 端点 | perm 串 | 特殊语义 |
|--------|------|---------|----------|
| /dictType | list/get/{id}/add/edit/delete/{id} | system:dict:list/query/add/edit/remove | edit 触发 DictTypeNameChangedEvent（Phase 5 范围，Phase 4 只需 CRUD） |
| /dictData | list?dictTypeId=（可选分支）/get/{id}/add/edit/delete/{id} | 同上共用 dict:* | add/edit 校验类型存在并冗余写 dictTypeName |
| /config | list/key/{key}/get/{id}/add/edit/delete/{id} | system:config:* | getByKey 返回 configValue 字符串 |
| /shift | list（**全量无分页**）/get/{id}/add/edit/delete/{id} | system:shift:*（菜单无此域） | — |
| /file | list（**分页**+categoryId 过滤）/upload（multipart）/download/{id}/delete/{id} | system:file:list/upload/download/remove | list 有分页！非全量 |
| /fileCategory | tree/get/{id}/add/edit/delete/{id} | system:file:* 共用 | tree 树查询 |
| /faultInfo | list（分页）/get/{id}/add/edit/delete/{id}/import/export | system:fault:* + import/export（菜单无此域） | — |
| /log | list（分页）/export | system:log:list | 查询条件 module/operation/realName 全部 **like 模糊** |

**错误消息原文**（逐字保真清单）[VERIFIED: 各 ApplicationService 引用处]：
- `字典类型不存在: {id}`（DictApplicationService:49）、`字典类型不存在: {typeId}`（DictData add/edit:87/102）
- `文件分类不存在: {categoryId}`（FileApplicationService:65）
- `上级分类不存在: {pid}`（FileCategoryApplicationService:54/69）
- `文件已存在：{fileName}（内容 MD5 重复，同一分类下不可重复上传）`（FileApplicationService:79-80）
- `文件不存在: {fileId}`、`文件路径为空，文件ID: {fileId}`（FileApplicationService:105/107）
- 存储删除失败：warn 日志 `S3对象删除失败，继续删除数据库记录: {filePath}` 后继续删 DB（FileApplicationService:112-122——FILE-03 宽容语义源码实证）

**categoryId IS NULL 判重分支（FILE-01 隐藏坑）确切代码** [VERIFIED: FileInfoRepositoryImpl.java:29-37 原文]：
```java
// eq/isNull 双分支缺一不可：唯一索引对 NULL 视为互不相等，未分类上传的判重全靠 isNull 分支
return super.getOne(Wrappers.lambdaQuery(FileInfo.class)
        .eq(categoryId != null, FileInfo::getCategoryId, categoryId)
        .isNull(categoryId == null, FileInfo::getCategoryId)
        .eq(FileInfo::getFileHash, fileHash)
        .last("LIMIT 1"), false);
```
→ Go 版：categoryId 非零值 `WHERE category_id = ?`，nil 时 `WHERE category_id IS NULL`，恒带 `AND file_hash = ? LIMIT 1`。源单测锁定五条语义（同分类同内容拒绝且零副作用/同内容异分类放行/null 走 IS NULL/异内容放行/分类不存在消息保留）[VERIFIED: FileUploadHashDedupTest.java:41-47]，直接作为 Go 集成测试蓝本。

**FileCategory 树**：service.tree() 实现孤儿提升（parent 不在 map → 提升为 root，与 Phase 2 dept 语义同构）[VERIFIED: FileCategoryApplicationService.java:23-42]；注意 entity 上另有一个 buildTree 静态方法孤儿不提升——**以 service 实现为准**（Controller 调用路径）。add/edit 冗余写 parentName；parentId nil 或 0 视为根。

**FaultInfo importExcel** [VERIFIED: FaultInfoApplicationService.java:62-90]：`ExcelHelper.read(file, FaultInfo.class)` 注解驱动读 → 逐行：name==null **skip（continue）**；三键 `findByNameAndBitIndexAndLocation` 全匹配→UPDATE（复用行 id）否则 INSERT；@Transactional 整批单事务；返回 `ImportResult{insertCount int, updateCount int}`（JSON 键 insertCount/updateCount）。
**三键查询 NULL 边界（源行为陷阱）**：`eq(name).eq(bitIndex).eq(location)` [VERIFIED: FaultInfoRepositoryImpl.java:58-63]——location 为 NULL 时生成 `location = NULL` 恒 false → 带 NULL 位置的行**永远判不中重复、反复 INSERT**。Go 版需拍板：照源（NULL 位置不判重）或修正为 IS NULL 分支（同 file 域双分支模式）。建议：**照源 eq 语义**（移植保真优先，Excel 导入的 location 列空 → 存 NULL 时判重失效是源已接受的既有行为），在 plan 标注。

**实体字段全集**（TableName/@ExcelProperty 逐字段）[VERIFIED: 各 entity 文件]：
- `sys_dict_type`: dict_name(100)/dict_type(100)/status(tinyint,1) + BaseUserEntity(id/create_time/update_time/remark(500)/create_by_id/create_by_name/update_by_id/update_by_name)。**DB 层 UNIQUE uk_dict_type(dict_type)** [VERIFIED: SHOW CREATE TABLE]
- `sys_dict_data`: dict_type_id/dict_type_name(100)/label(100)/value(100)/sort(int)。普通 KEY idx_dict_type_id
- `sys_config`: config_key(100)/config_value(500)/config_type(50)。**DB 层 UNIQUE uk_config_key(config_key)**——CONF-01 读写契约的唯一性由 DB 保证，Go 版重复 key 插入将收 MySQL 1062 错误（gorm TranslateError 可译）
- `sys_shift`: shift_code(50)/shift_name(50)/start_time(TIME)/end_time(TIME)/status(tinyint,1)。无唯一索引
- `sys_fault_info`: code(50)/name(100)/**bit_index(int default -1)**/**type(tinyint default 3=UNKNOWN)**/dict_data_id/detail(500)/reason(500)/location(200)/fix(500)。普通 KEY idx_code；三键无唯一索引
- `sys_file`（**表名是 sys_file 非 sys_file_info**）: category_id/category_name(100)/file_name(255)/file_path(500)/file_type(50)/file_size(bigint)/file_hash(varchar(32)，无索引)/description(500)。普通 KEY idx_category_id
- `sys_file_category`: parent_id/parent_name(50)/category_name(100)/**category_type(int)**/sort(int)/icon(100)/status(tinyint,1)。普通 KEY idx_parent_id
- 枚举：UsageStatus DISABLED(0,"禁用")/ENABLED(1,"正常")，fromInfo 兜底 ENABLED [VERIFIED: UsageStatus.java:14-15/33-38]；FaultTypeEnum INFO(0,"消息")/WARNING(1,"警告")/FAULT(2,"故障")/UNKNOWN(3,"未知")，fromValue/fromInfo 兜底 UNKNOWN [VERIFIED: FaultTypeEnum.java:14-17/28-40]（ENG-04 读侧兼容已有先例）

### 重点 2：操作日志切面考古（HIGH 置信）

[VERIFIED: OperationLogAspect.java 全文 90 行逐行]

- **敏感字段 8 个**（正则原文第 27-28 行）：`"(password|oldPassword|newPassword|secretKey|apiKey|token|accessToken|refreshToken)"\s*:\s*"[^"]*"` → 替换为 `"$1":"******"`。只掩码**带引号的字符串值**（数字/布尔/null 值不匹配）。Go 正则直译：`` regexp.MustCompile(`"(password|oldPassword|newPassword|secretKey|apiKey|token|accessToken|refreshToken)"\s*:\s*"[^"]*"`) `` + `ReplaceAllString` → `"${1}":"******"`。
- **module/operation 元数据来源**：module = Controller 类简名（`getDeclaringType().getSimpleName()`，如 "UserController"）；operation = 方法名（如 "create"）。无注解，execution 切点全局。
- **params 构造**：方法参数过滤（跳过 null/ServletRequest/ServletResponse/MultipartFile）→ **limit(3)** 最多 3 个 → JSON 序列化 → 掩码 → `"; "` 连接。无参方法 → 空字符串（存量实证 params 列为空串非 NULL）。
- **同步落库**（finally 中 save，异常 catch 忽略静默）[VERIFIED: 70-76 行]。
- **errorMsg 截断**：`msg.substring(0, Math.min(500, msg.length()))`——500 字符上限确认。
- **其余字段**：status 成功 1/失败 0；duration 毫秒 int；method = `request.getMethod() + " " + request.getRequestURI()`（如 `POST /auth/login`）；ip = `request.getRemoteAddr()`（**无 X-Forwarded-For 处理**——Go 版对齐用 `c.Request.RemoteAddr` 解析还是 `c.ClientIP()`（读 XFF）需拍板，建议对齐源用 RemoteAddr，反代场景再放宽）；userId/realName 从会话取，未登录（登录接口自身）为 null。
- **sys_operation_log 全列**（DDL 实证）[VERIFIED: SHOW CREATE TABLE sys_operation_log]：`id, user_id, real_name(50), module(100), operation(200), method(200), params(TEXT), ip(50), duration(int), status(tinyint), error_msg(TEXT), create_time, update_time, remark(500)`；索引 idx_user_id + idx_create_time；**无 create_by_id/create_by_name/update_by_id/update_by_name 四审计列**（实体 extends BaseAggregate 非 BaseUserEntity，一致）。status 列 DDL default NULL（失败 0/成功 1 由应用写）。
- **分页查询**：LogQuery 字段默认 pageNum=1/pageSize=10（**1/10 特例的真实来源是 DTO 字段默认值**）+ module/operation/realName like 模糊 + orderByDesc createTime [VERIFIED: LogQuery.java:11-25, OperationLogRepositoryImpl.java:36-42]。钳位沿 PageHelper（<1 → 1/15）：源行为 size=0 → 15（非 10）。Go 版 paging.Parse 现无特例参数机制 [VERIFIED: pkg/paging/paging.go:24-34]，需新增（如 `ParseWithDefaults(page, size string, defPage, defSize int)`），Olog 调用传 1/10 默认；<1 钳位目标值照源为 15 还是 10 需 plan 拍板（源=15，语义差异微小）。

### 重点 3：本机 DDL 实证（HIGH 置信）

[VERIFIED: docker exec mysql-server SHOW TABLES / SHOW CREATE TABLE 全输出]

- sys_iam 库 **15 表**，本阶段所需 8 表全部在库：sys_dict_type/sys_dict_data/sys_config/sys_shift/sys_fault_info/sys_file/sys_file_category/sys_operation_log。`sys_import_result` **不是表**（Java DTO，objective 中列表名以 SHOW TABLES 为准——正确排除）。
- 逐列记录见重点 1 实体字段全集（DDL 与 PO 逐列对照一致，无缺列）。
- 唯一索引仅两处：`uk_dict_type(dict_type)`、`uk_config_key(config_key)`——Go 版 add 重复 key 走 1062 错误翻译路径（409 语义沿 Phase 2 惯例）。
- **测试基建现状**：scripts/ddl/ 现 9 文件（8 表 + casbin_rule），**六域 8 表 DDL 不在其中**——需新增。testutil 加载机制已核实：`execDDLDir` glob `scripts/ddl/*.sql` 按文件名序全部执行，任一失败整体失败 [VERIFIED: internal/testutil/mysql.go:78-92] → **新 DDL 文件落目录即自动加载，零代码改动**。

### 重点 4：minio-go 集成与 FileStorage（HIGH 置信）

- **版本实证**：minio-go v7.4.1（2026-10-12）[VERIFIED: proxy.golang.org/github.com/minio/minio-go/v7/@latest]；excelize v2.11.0（2026-07-06）[VERIFIED: proxy.golang.org/github.com/xuri/excelize/v2/@latest]。两者均 STACK 锁定版本，go.mod 需新增这两个直接依赖（当前无）[VERIFIED: go.mod 全文]。
- **API 形态** [CITED: Context7 /minio/minio-go PutObject/GetObject 文档]：
  - `PutObject(ctx, bucket, object, reader, size, opts)`——**size 已知时必须传真实值**；传 -1 会按 multipart 分片且"buffers each part in memory"（文档明示），工控机内存场景禁用 -1。multipart FileHeader.Size 可得真实 size。
  - `GetObject(ctx, bucket, object, opts)` 返回 `*minio.Object`（实现 io.Reader/io.Seeker/io.Closer）——**错误在读流时才浮现**（含对象不存在的 404）；`io.Copy` 到响应即流式下载。
  - 桶：`BucketExists(ctx, bucket)` / `MakeBucket(ctx, bucket, opts)`（启动时 ensure，等价源 @PostConstruct createBucket [VERIFIED: FileApplicationService.java:45-48]）。
- **本机 rustfs 容器实测**：`rustfs/rustfs:1.0.0-alpha.98-glibc` Up 30 hours，端口 9000-9001 映射，S3 API 活跃（返回标准 S3 XML 错误），凭证 env 实证 `RUSTFS_ACCESS_KEY=admin / RUSTFS_SECRET_KEY=admin` [VERIFIED: docker ps -a + docker inspect rustfs Config.Env 输出]。→ S3Storage 可用真容器做集成 smoke。
- **源配置参考**（Go 版 storage.s3 段默认值）：endpoint `http://localhost:9000`、bucket `tims`、connectTimeout 10s [VERIFIED: RustfsProperties.java:14-33]。注意源默认凭证 rustfsadmin/rustfssecret 与本机容器实际 admin/admin 不同——Go 版默认值建议照源（现场可配），集成测试用容器实际值。
- **源存储接口面**（File 域实际用到）：createBucket/upload(key, inputStream, contentLength, contentType)/download（byte[]）/delete(key) [VERIFIED: FileApplicationService 三处调用]——FileStorage 接口按此最小面设计（见 Implementation Guidance）。

### 重点 5：Excel 导入回测可得性（HIGH 置信）

- **不可直接回测**：sys_fault_info **0 行**（无存量数据）[VERIFIED: SELECT COUNT 输出]；全盘 find 无源导出 xlsx [VERIFIED: find /home/wx -name "*.xlsx" 输出]。
- **yf175a 故障信息表.xlsx**（/home/wx/workspace/tims/backend/docs/）77 行真实 PLC 故障数据，列头 `序号|故障变量|触发位|故障代码|故障描述|报警类型|故障原因|解决方法|故障位置|备注` [VERIFIED: xlsx sharedStrings.xml 解析输出]——与源导出格式**列序不同且多列**，是人工原始资料非 sys-iam 导出物。**不可直接回测；可作为数据源转换生成回测文件**。
- **结论（三选一裁决）：构造等价回测，数据行可选从 yf175a 转换**（真数据+构造格式，构造依据 = 源 @ExcelProperty 契约，列序表见 Implementation Guidance——完全满足 D-14"构造依据文档化，不发明格式"）。

### 重点 6：excelize 与 testutil 扩展（HIGH 置信）

- **GetRows 行为** [CITED: Context7 /xuri/excelize-doc GetRows]：返回 [][]string；"continually blank cells in the tail of each row will be skipped, so the length of each row may be inconsistent"——**行长度不一致是常态**，按列索引解析必须做 `len(row) > idx` 边界检查（构造回测文件时也应避免行尾空单元格陷阱，或解析侧统一防御）。
- 从 multipart 流打开：`excelize.OpenReader(io.Reader)` 标准入口 [CITED: 同上 installation.md]。
- testutil 增表方式：已核实 glob 自动加载（见重点 3）——**Phase 4 测试基建改动仅为新增 DDL 文件**。
- File 域测试的存储后端注入：newTestApp 需参数化 FileStorage（现有 handlers struct + newRouter 签名 [VERIFIED: internal/app/router.go:15-34]，装配链在 app.go——FileStorage 作为 handlers 装配参数传入，local 默认）。

### 横切发现（6 重点之外，进 plan 必读）

1. **BodyLimit 1MB 与 upload/import 冲突**：全局 BodyLimit 1MB（http.MaxBytesReader）会拦截大文件上传与 Excel 导入。body_limit.go 注释已预留设计意图："后续大 payload 域如 Excel 导入应另设独立上限，不复用全局值" [VERIFIED: internal/middleware/body_limit.go:9-13 IN-10 注释]。→ plan 必须处理：upload/import 路由挂**独立上限的 BodyLimit 实例**（如 `BodyLimitWithMB(20)`），全局 1MB 不动。
2. **Olog errorMsg 来源**：Go 版 handler 错误经 resp.Fail 写出，中间件 c.Next() 后拿不到 err 对象 [VERIFIED: pkg/resp/resp.go:29-36]。建议：resp.Fail 内加 `c.Error(err)`（gin 原生 errors 收集，不影响响应体），Olog 中间件从 `c.Errors.ByType(gin.ErrorTypePrivate)` 取首条 message 截 500——最小侵入；或 c.Set("olog_error_msg") 显式约定。
3. **Olog 挂载形态**（D-05 路由注册时声明）：`pri.POST("/api/v1/...", middleware.RequirePerm(...), middleware.OpLog(w, "DictType", "create"), hs.X.Create)`——中间件在 c.Next() 前缓存 body 字节（≤1MB 已被 BodyLimit 保证）、后取 status/duration/errorMsg 异步入 chan。
4. **File list 是分页**（CONTEXT.md "轻 CRUD 三域"叙述未涵盖 file 分页细节）——file list 走 paging.Parse 1/15 标准（源 PageHelper.parsePage 同 1/15 [VERIFIED: PageHelper.java:23-33]）+ categoryId 可选过滤 + createTime 倒序 [VERIFIED: FileInfoRepositoryImpl.java:16-21]。
5. **shift/fault/file 域无 builtin 概念**（源无 builtin 字段/守卫）——Phase 2 的 builtin 守卫族不适用此三域。
6. **上传存储顺序**：源为 save DB → 上传存储，且 @Transactional 包含存储调用（S3 失败 DB 回滚）[VERIFIED: FileApplicationService.java:93-100 @Transactional]。Go 版建议同序（service 层事务内调 storage.Put，失败返回错误回滚）。

## Implementation Guidance

### 新增文件清单（按 D-01 四 plan 聚簇）

```
Plan ① Dict+Config+Shift:
  internal/model/{dict_type,dict_data,sys_config,shift}.go          # 4 PO（TableName 显式映射）
  internal/repository/{dict_type,dict_data,sys_config,shift}.go     # 4 repo
  internal/service/{dict,config,shift}.go                           # dict 合并 type+data
  internal/handler/{dict_type,dict_data,config,shift}.go            # 4 handler
  internal/authz/perm.go 增常量                                     # system:dict/config/shift 五档
  scripts/ddl/sys_dict_type.sql, sys_dict_data.sql, sys_config.sql, sys_shift.sql
Plan ② File+FileCategory:
  internal/storage/storage.go                                       # FileStorage 接口
  internal/storage/local.go / s3.go                                 # 双实现
  internal/model/{file_info,file_category}.go + repo + service + handler
  internal/config/config.go 增 storage 段；configs/config.yaml 同步
  scripts/ddl/sys_file.sql, sys_file_category.sql
Plan ③ Olog:
  internal/middleware/op_log.go                                     # 中间件 + chan + worker + 掩码
  internal/model/operation_log.go + repo + handler（查询域）
  scripts/ddl/sys_operation_log.sql
Plan ④ Fault+Excel:
  internal/service/fault_import.go（或并入 fault.go）               # excelize 导入 + 三键 upsert
  internal/model/fault_info.go + repo + handler
  scripts/ddl/sys_fault_info.sql
  tests 构造回测 xlsx
共享改动: internal/app/router.go（六域路由+OpLog 挂载）、app.go（FileStorage 构造+Olog worker+Shutdown flush）、
  pkg/paging/paging.go（ParseWithDefaults 特例）、pkg/resp/resp.go（c.Error 透出，若采纳建议 2）、
  internal/middleware/body_limit.go（BodyLimitWithMB 参数化）、go.mod（excelize+minio-go）
```

### DDL 骨架（8 表，列名以 SHOW CREATE TABLE 实证为准——写入 plan 时逐列照抄本文 Key Findings 重点 1/重点 2 的 DDL 记录）

唯一索引仅 `uk_dict_type`/`uk_config_key` 两处；其余全部普通 KEY；全表主键 `id bigint NOT NULL PRIMARY KEY`（雪花应用层生成，ENG-03 现状）；审计 4 列 + create_time/update_time/remark(500) 全表同构；字符集 utf8mb4_general_ci。

### Olog 中间件伪代码

```go
// D-05/D-06/D-07/D-08/D-09 落地形态
var sensitiveRE = regexp.MustCompile(
    `"(password|oldPassword|newPassword|secretKey|apiKey|token|accessToken|refreshToken)"\s*:\s*"[^"]*"`)

func OpLog(w *OpLogWriter, module, operation string) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        var body []byte
        if c.Request.Body != nil {
            body, _ = io.ReadAll(c.Request.Body) // ≤1MB：BodyLimit 已保证
            c.Request.Body = io.NopCloser(bytes.NewReader(body)) // D-06 缓存重放
        }
        c.Next()
        entry := Entry{
            UserID:   userIDFromCtx(c), RealName: realNameFromCtx(c), // 未登录 nil
            Module:   module, Operation: operation,
            Method:   c.Request.Method + " " + c.Request.URL.Path,
            Params:   sensitiveRE.ReplaceAllString(string(body), `"${1}":"******"`),
            IP:       c.Request.RemoteAddr, // 源 getRemoteAddr 对齐；ClientIP(XFF) 需 plan 拍板
            Duration: int(time.Since(start).Milliseconds()),
            Status:   statusBit(c),                      // 1/0
            ErrorMsg: trunc(errMsg(c), 500),             // 建议 c.Errors 取（见横切发现 2）
        }
        w.Enqueue(entry) // D-07 buffered chan 满则丢弃
    }
}
// worker: 后台 goroutine 批量 INSERT；app.Shutdown 链 drain（D-08，复用 Phase 1 NotifyContext）
```

### FileStorage 接口（D-10）

```go
type FileStorage interface {
    Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
    GetStream(ctx context.Context, key string) (io.ReadCloser, int64, error) // size 从 Stat/HeadObject 取
    Delete(ctx context.Context, key string) error
}
// LocalStorage: filepath 安全 join（防 key 逃逸 root，即使 key 恒为 UUID_原名仍防御）;
//   Put=os.Create+io.Copy; GetStream=os.Open+Stat; Delete=os.Remove（IsNotExist 宽容——对齐 S3 语义）
// S3Storage: minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(ak,sk,""), Secure: bool});
//   EnsureBucket 启动时（BucketExists→MakeBucket）; PutObject 传真实 size（禁 -1）;
//   GetStream=GetObject 返回 *minio.Object; Delete=RemoveObject
// 配置: storage.driver: local|s3（D-10）; local.root 默认 ./data/files（D-11）;
//   s3: {endpoint, access_key, secret_key, bucket, secure}
```

### 上传链（D-12 与零副作用的合并建议）

```go
// 源顺序：分类校验 → 算 MD5 → 判重（拒绝零副作用）→ save DB → 上传存储
// Go 实现（建议，替代 D-12 TeeReader 表述）：
fh, _ := c.FormFile("file")                      // multipart.FileHeader
f, _ := fh.Open()                                 // multipart.File：*os.File 或 bytes.Reader，均支持 Seek
h := md5.New(); io.Copy(h, f)                     // ①流式算 MD5（fh.Size 已知，不整载）
sum := hex.EncodeToString(h.Sum(nil))
// ②findByCategoryAndHash(categoryId, sum) 判重——命中即 4xx 拒绝，存储零副作用
f.(io.Seeker).Seek(0, io.SeekStart)              // ③重放流
// ④storage.Put(ctx, key, f, fh.Size, contentType) + DB save（同事务或先 DB 后 Put 照源）
// key = uuid.NewString() + "_" + originalFilename（源 filePath 形态对齐）
```
TeeReader 单流无法同时满足"先判重再上传"的零副作用语义；multipart.File 的 Seek(0) 重放是标准做法（Go multipart 对小文件内存、大文件临时文件自动管理，RSS 可控）。plan 采纳时将 D-12 实现表述更新为"Seek 重放"，语义不变。

### Excel 列序表（FAUL-02 契约，构造回测文件依据）

源导出 = `FesodSheet.write(head=FaultInfo)` 按字段声明顺序；读取同序匹配（D-15 表头按列索引严格匹配与 EasyExcel/Fesod 默认一致）。Sheet 名 "Sheet1"，下载文件名 "故障库.xlsx" [VERIFIED: ExcelHelper.export 默认 sheetName="Sheet1" + FaultInfoController.java:93]：

| 列序 | 列头（精确文案） | 字段 | 类型/解析 |
|------|------------------|------|-----------|
| 1 | 故障代码 | code | string |
| 2 | 故障变量 | name | string（三键；空 → skip 该行） |
| 3 | 触发位 | bit_index | int；空单元格源保留字段初始值 -1 |
| 4 | 报警类型 | type | 中文：消息/警告/故障/未知 → 0/1/2/3；未知文案 → 3(UNKNOWN) |
| 5 | 故障描述 | detail | string |
| 6 | 故障原因 | reason | string |
| 7 | 故障位置 | location | string（三键；空 → NULL，判重 eq 失效见重点 1 陷阱） |
| 8 | 解决方法 | fix | string |

dictDataId 无 @ExcelProperty 不进 Excel [VERIFIED: FaultInfo.java:39-40]。导入响应 `ImportResult{insertCount, updateCount}`。

### 路由表建议（Go API 重设计，复数资源沿 Phase 2 先例）

```
pri.POST   /api/v1/dict-types            dict:add        # Plan ①
pri.PUT    /api/v1/dict-types/:id        dict:edit
pri.GET    /api/v1/dict-types/:id        dict:query
pri.GET    /api/v1/dict-types            dict:list
pri.DELETE /api/v1/dict-types/:id        dict:remove
pri.       /api/v1/dict-datas 同构 + GET /api/v1/dict-datas?dict_type_id= 分支（DICT-03）
pri.       /api/v1/configs 同构 + GET /api/v1/configs/key/:key（config:query）
pri.       /api/v1/shifts 同构（list 全量无分页——SHFT-01）
pri.       /api/v1/files + /api/v1/files/upload（BodyLimitWithMB(20)+OpLog）+
           /api/v1/files/download/:id（流式）+ delete + list?category_id=（分页）
pri.       /api/v1/file-categories + /api/v1/file-categories/tree
pri.       /api/v1/logs?module=&operation=&real_name=&page=&size=（1/10 特例）
pri.       /api/v1/faults + import（BodyLimitWithMB(20)+OpLog）
```
全部挂 RequirePerm；fault/shift perm 串挂上但菜单不授（super_admin 边界，见 User Constraints）。六域写路由挂 OpLog（module/operation 逐路由声明）。

### 测试策略

- **判重五语义集成测试**（源 FileUploadHashDedupTest 蓝本 → Go httptest 集成形态）：同分类同内容 409 零副作用（断言 DB 0 行 + 存储无对象）/异分类放行/未分类 IS NULL 分支/异内容放行/分类不存在消息。
- **Olog 单测**：掩码正则 8 字段逐字段断言 + 非字符串值不掩码 + 截 500 + chan 满丢弃不阻塞 + flush drain。
- **Fault 导入集成**：构造 xlsx（excelize 写：NewFile→SetCellValue 8 列表头→数据行→WriteToBuffer→multipart 上传）；三键命中 UPDATE/未命中 INSERT/整批事务回滚（一行非法→全批无痕）/ImportResult 计数；可选 yf175a 转换数据行批量导入冒烟。
- **S3Storage**：真容器 smoke（rustfs localhost:9000 admin/admin，连接失败 skip）+ LocalStorage 全语义测试为主——不引入 testcontainers minio 镜像依赖（离线拉取风险，Phase 1 先例）。
- 回归：Phase 1-3 全量套件保持绿（router.go/app.go 改动波及面）。

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | testify v1.12.1 + testcontainers-go v0.44.0（integration tag）+ miniredis v2.40.0 |
| Config file | 无独立配置——build tag `integration` + newTestApp 模式 |
| Quick run command | `go build ./... && go vet ./...` |
| Full suite command | `go test -tags=integration ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DICT-01/02/03 | 类型 CRUD + 数据 CRUD + 按类型查询 | integration | `go test -tags=integration ./internal/handler -run TestDict` | ❌ Wave 0 |
| CONF-01 | config CRUD + key 查询 + 唯一键 1062 | integration | `go test -tags=integration ./internal/handler -run TestConfig` | ❌ Wave 0 |
| SHFT-01/02 | 全量 list（无分页）+ CRUD | integration | `go test -tags=integration ./internal/handler -run TestShift` | ❌ Wave 0 |
| FILE-01..04 | 上传判重五语义/流式下载/宽容删除/分类树 | integration | `go test -tags=integration ./internal/handler -run TestFile` | ❌ Wave 0 |
| FILE-05 | driver 切换 + S3 smoke | integration（skip-if-unreachable） | `go test -tags=integration ./internal/storage` | ❌ Wave 0 |
| OLOG-01/02/03 | 写操作记录/8 字段掩码+截 500+静默/1-10 分页 | unit + integration | `go test ./internal/middleware -run TestOpLog` | ❌ Wave 0 |
| FAUL-01/02 | CRUD + Excel 三键 upsert 整批事务 | integration | `go test -tags=integration ./internal/handler -run TestFault` | ❌ Wave 0 |

### Sampling Rate

- Per task commit: `go build ./... && go test ./internal/...`（单元层）
- Per wave merge: `go test -tags=integration ./...`（全量集成 + Phase 1-3 回归）
- Phase gate: 全量绿 + 判重五语义/掩码 8 字段/三键 upsert 三大验收点专项绿

### Wave 0 Gaps

- [ ] scripts/ddl/ 新增 8 表 SQL（sys_dict_type/sys_dict_data/sys_config/sys_shift/sys_fault_info/sys_file/sys_file_category/sys_operation_log）——落目录自动加载，Plan ① 起逐步补齐（也可 Wave 0 一次性 8 文件全落，推荐后者——DDL 无依赖顺序）
- [ ] go.mod：`go get github.com/xuri/excelize/v2@v2.11.0 github.com/minio/minio-go/v7@v7.4.1`
- [ ] pkg/paging 特例默认参数 / internal/middleware/body_limit.go 参数化（共享件先行）
- [ ] newTestApp FileStorage 参数化 + OpLog writer 装配钩子

## Open Questions

1. **OLOG-02 "7 字段" vs 源 8 字段**——已按研究纪律裁定以源为准（8 字段清单逐字引用 OperationLogAspect.java:27-28），plan 按 8 字段落；执行期如用户问询，引用本节即可。
2. **Fault 三键 location=NULL 判重失效**——照源 eq 语义（建议）或 file 域式 IS NULL 双分支修正，plan 拍板（建议照源，保守移植；本机 fault 0 行无存量影响）。
3. **Olog 钳位细节**——源 size=0 → 15（PageHelper 钳位）而默认 10；Go 特例实现 <1 归 10（直觉）还是归 15（源保真），plan 拍板（差异仅显式传 0 的请求）。
4. **Olog IP 取值**——`c.Request.RemoteAddr`（源保真，反代场景记代理 IP）vs `c.ClientIP()`（读 XFF）；建议源保真，plan 确认。
5. **resp.Fail 透出 errorMsg 的机制**——c.Error(err)（建议）vs c.Set 约定；涉及 pkg/resp 微改，plan 拍板。
6. **S3Storage 真容器 smoke 的 skip 策略**——rustfs 容器当前 Up，但测试纪律"不锚定本机具名容器"（D-10/D-12 铁律）；建议 smoke 测试探测 localhost:9000 可达则跑、不可达 skip（环境自适应，与 Phase 1 镜像探测先例同构），plan 确认。

## Sources

### Primary (HIGH confidence)
- 源码逐文件直读：OperationLogAspect.java / 八 Controller / 七 ApplicationService / FileInfoRepositoryImpl / FaultInfoRepositoryImpl / OperationLogRepositoryImpl / SysConfigRepositoryImpl / 九 entity / ExcelHelper / PageHelper / BaseAggregate / BaseUserEntity / UsageStatus / FaultTypeEnum / 双 Converter / RustfsTemplate / RustfsProperties / FileUploadHashDedupTest（均含行号引用）
- 本机实证：docker exec mysql-server SHOW TABLES + 8×SHOW CREATE TABLE + sys_menu 权限查询 + 六表 COUNT + sys_operation_log 存量样例；docker ps -a + docker inspect rustfs
- proxy.golang.org：excelize v2.11.0 / minio-go v7.4.1 @latest 实测
- Go 仓库现状：router.go / paging.go / body_limit.go / resp.go / access_log.go / testutil/mysql.go / config.yaml / go.mod 直读

### Secondary (MEDIUM confidence)
- Context7 /xuri/excelize-doc GetRows 语义（尾部空单元格跳过）
- Context7 /minio/minio-go PutObject（size=-1 内存警告）/ GetObject（延迟报错）文档

### Tertiary (LOW confidence)
- 无——全部关键结论均有一手证据
