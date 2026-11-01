---
phase: 04-domains
reviewed: 2026-11-01T13:10:36Z
depth: standard
files_reviewed: 70
files_reviewed_list:
  - configs/config.yaml
  - go.mod
  - internal/app/app.go
  - internal/app/router.go
  - internal/app/shutdown_integration_test.go
  - internal/authz/perms.go
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/handler/config.go
  - internal/handler/dict_config_shift_integration_test.go
  - internal/handler/dict_data.go
  - internal/handler/dict_type.go
  - internal/handler/fault.go
  - internal/handler/fault_integration_test.go
  - internal/handler/file_category.go
  - internal/handler/file.go
  - internal/handler/file_integration_test.go
  - internal/handler/olog_integration_test.go
  - internal/handler/operation_log.go
  - internal/handler/shift.go
  - internal/handler/user_integration_test.go
  - internal/middleware/body_limit.go
  - internal/middleware/op_log.go
  - internal/middleware/op_log_test.go
  - internal/model/dto/config.go
  - internal/model/dto/dict.go
  - internal/model/dto/fault_info.go
  - internal/model/dto/file.go
  - internal/model/dto/operation_log.go
  - internal/model/dto/shift.go
  - internal/model/po/dict.go
  - internal/model/po/fault_info.go
  - internal/model/po/file.go
  - internal/model/po/operation_log.go
  - internal/model/po/shift.go
  - internal/model/po/sys_config.go
  - internal/repository/dict.go
  - internal/repository/fault_info.go
  - internal/repository/file_category.go
  - internal/repository/file_info.go
  - internal/repository/operation_log.go
  - internal/repository/shift.go
  - internal/repository/sys_config.go
  - internal/service/config.go
  - internal/service/dict.go
  - internal/service/fault.go
  - internal/service/fault_import.go
  - internal/service/file_category.go
  - internal/service/file.go
  - internal/service/operation_log.go
  - internal/service/shift.go
  - internal/storage/local.go
  - internal/storage/local_test.go
  - internal/storage/s3.go
  - internal/storage/s3_test.go
  - internal/storage/storage.go
  - pkg/paging/paging.go
  - pkg/paging/paging_test.go
  - pkg/resp/resp.go
  - pkg/resp/resp_test.go
  - pkg/types/fault_type.go
  - pkg/types/fault_type_test.go
  - scripts/ddl/sys_config.sql
  - scripts/ddl/sys_dict_data.sql
  - scripts/ddl/sys_dict_type.sql
  - scripts/ddl/sys_fault_info.sql
  - scripts/ddl/sys_file_category.sql
  - scripts/ddl/sys_file.sql
  - scripts/ddl/sys_operation_log.sql
  - scripts/ddl/sys_shift.sql
findings:
  critical: 2
  warning: 5
  info: 1
  total: 8
status: issues_found
---

# Phase 4: Code Review Report

**Reviewed:** 2026-11-01T13:10:36Z
**Depth:** standard
**Files Reviewed:** 70
**Status:** issues_found

## Summary

Phase 4 六域（config/dict/fault/file/olog/shift）整体质量高：分层纪律一致（handler 薄层三段式 → service 白名单 Patch → repo 恒带 `Where("id = ?")`）、审计钩子/雪花 ID/1062 翻译等 Phase 2 惯例得到忠实延续、测试覆盖扎实（掩码顺序、回滚、NULL 判重锁定、优雅关停 drain 均有直证测试），`go vet` / `go build` 全绿。

但仍审出 2 个 Critical：

1. **BodyLimit 对 multipart 的全局跳过打开了无界请求体读取面**——免认证的 `POST /api/v1/auth/login` 等路由以 `Content-Type: multipart/form-data` 发送任意大 JSON 时完全绕过 1MB 上限，`ShouldBindJSON` 全量读入内存（已读 gin v1.12 源码实证 `decodeJSON` 无任何限制），30s ReadTimeout 只限时不限量，内网可 OOM 工控机。
2. **file_type 列 varchar(50) 而上传链直接回存客户端 Content-Type**——xlsx/docx 的标准 MIME（65 字符）超列长，浏览器正常上传 Office 文档在 strict mode 下 INSERT 必失败（500），文件域核心场景对合法输入确定性损坏。

5 个 Warning 集中在：字节截断可切出无效 UTF-8 导致整批日志落库失败、Update 路径唯一键冲突未翻译（500 vs Create 的 400）、Excel 导入行数防线在 GetRows 全量载入之后才生效、shift 域 status 绕过全库 UsageStatus 枚举纪律、上传文件名/Content-Type 无长度钳位。

## Critical Issues

### CR-01: BodyLimit 对 multipart 全局跳过，非 upload 路由可被无界请求体打 OOM（免认证 login 即可触发）

**File:** `internal/middleware/body_limit.go:32-43`（跳过逻辑）；`internal/app/router.go:65-66`（pub 路由无路由级上限）；`internal/handler/user.go:33-47`（bindJSON 无二次防护）

**Issue:**
全局 `BodyLimit()` 对 `Content-Type: multipart/` 前缀请求直接 `c.Next(); return`——**不包 MaxBytesReader**。该上限原意是给 `files/upload`、`faults/import` 两个挂了 `BodyLimitWithMB(20)` 的路由让路，但跳过是按 Content-Type 判定而非按路由判定，于是**其余全部路由（无路由级上限）对 multipart Content-Type 的请求完全无体积封顶**：

- 客户端只需对任意 JSON 路由（如免认证的 `POST /api/v1/auth/login`）声明 `Content-Type: multipart/form-data; boundary=x`，同时发送任意大 JSON body；
- `c.ContentType()` 返回去参 media type（gin context.go:1036-1038，已读 v1.12 源码实证），前缀命中 → 跳过；
- handler `bindJSON` → `c.ShouldBindJSON` → gin `decodeJSON` = `json.NewDecoder(r).Decode(obj)`（gin binding/json.go:44-56，已实证无任何大小限制）——`Decode` 解析单个 JSON value 时必须把整个 value 缓冲进内存；
- `ReadTimeout=30s` 只封读取时长不封字节数：内网百兆 30s 可注入数百 MB～GB 级 body → 工控机（内存有限，注释自述"无上限 body 经 ShouldBindJSON 全量读入可致 OOM"）被单请求 OOM。

免认证 login 路由意味着该 DoS 无需任何凭据；已认证用户则可打全部写路由。这与本中间件注释自述的防护目标（WR-02/T-1-04 DoS 防护）直接矛盾——multipart 豁免把防护面豁免掉了。

**Fix:**
最小改动（保持 upload/import 20MB 生效不变）：multipart 跳过时仍包一层全局 multipart 上限的 MaxBytesReader，与路由级 20MB 叠加时内层生效值不变：

```go
// 全局 multipart 上限：与 upload/import 路由级 BodyLimitWithMB(20) 对齐。
const maxMultipartBytes int64 = 20 << 20

func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			if strings.HasPrefix(c.ContentType(), "multipart/") {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMultipartBytes)
			} else {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
			}
		}
		c.Next()
	}
}
```

更严格的替代：非 `files/upload`、`faults/import` 路由对 multipart Content-Type 直接 415（这些路由契约本就是 JSON body）——但同时消除"合法 multipart 打到错误路由"的静默语义。两案取一即可，推荐前者（改动面小、`BodyLimitWithMB` 语义不变）。

### CR-02: file_type 列 varchar(50) 小于 xlsx/docx 标准 MIME 长度，浏览器正常上传 Office 文档必失败（500）

**File:** `internal/service/file.go:115,117-125`（contentType 直存）；`scripts/ddl/sys_file.sql`（`file_type` varchar(50)）

**Issue:**
上传链把客户端 multipart part 的 Content-Type 原样落库：`contentType := fh.Header.Get("Content-Type")` → `FileInfoPO.FileType` → INSERT。而 `sys_file.file_type` 为 `varchar(50)`。浏览器对 Office 文档 part 发送的标准 MIME 超长：

- `.xlsx` → `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`（65 字符）
- `.docx` → `application/vnd.openxmlformats-officedocument.wordprocessingml.document`（65 字符）

MySQL strict mode（5.7+/8.0 默认）下 INSERT 报 1406 Data too long → 上传事务回滚 → 用户收到 500/10001 固定文案。即：**文件域对最常见的办公文档类型上传，标准浏览器/HTTP 客户端行为下确定性失败**；只有 curl 等不带 part Content-Type 的客户端才侥幸成功（空串落库）。这不是边缘输入，是文件管理域的核心合法输入。

**Fix:**
两处择一（推荐 a+b 双管齐下）：

```go
// (a) service 层超长防御：超列长收敛为合法兜底值
contentType := fh.Header.Get("Content-Type")
if len(contentType) > 50 { // 与 DDL file_type varchar(50) 对齐
    contentType = contentType[:strings.IndexByte(contentType+";", ';')] // 取 media type 本体（去参数）
}
if len(contentType) > 50 {
    contentType = "application/octet-stream"
}
```

```sql
-- (b) 若现场允许改列（源库列长本就与真实 MIME 不兼容，属 DDL 保真缺陷而非契约）：
ALTER TABLE sys_file MODIFY file_type varchar(100) DEFAULT NULL;
```

至少要做 (a)，使上游来源不受信任的该字段永不触发 500。

## Warnings

### WR-01: truncateBytes 按字节切断可产生无效 UTF-8，MySQL strict mode 下整批日志落库失败（可被确定性触发）

**File:** `internal/middleware/op_log.go:316-321`（truncateBytes）；`internal/middleware/op_log.go:280`（调用点）

**Issue:**
`truncateBytes(s, 60000)` 在字节 60000 处硬切。`params` 列为 utf8mb4，请求体含中文（3 字节/rune）时切点大概率落在多字节字符中间，产生非法 UTF-8 尾串。go-sql-driver 以该字节串绑定参数，MySQL strict mode 报 1366 Incorrect string value → `CreateInBatches` 整条多行 INSERT 失败 → `flushBatch` 整批（最多 64 条）日志丢弃。攻击者只需发送 >60KB 且含多字节字符的请求体（对有 perm 的任意写路由），即可让**与其请求无关的同批其他日志一并丢失**——日志可审计性受损。测试只覆盖了 ASCII（`TestMaskSensitiveOrderWithTruncation` 用 "A" 重复），未覆盖多字节切点。

**Fix:**
截断后回退到最近完整 rune 边界：

```go
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	// 回退切点，保证不切断多字节 rune（utf8mb4 列 INSERT 才不会报 1366）
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
```

并补一条多字节切点直测（如 59999 字节 ASCII + "错" 开头的 body）。

### WR-02: Update 路径唯一键冲突未翻译——撞 uk 返回 500/10001，Create 路径同冲突返回 400/10400

**File:** `internal/service/dict.go:58-79`（UpdateDictType）；`internal/service/config.go:52-73`（Update）

**Issue:**
`CreateDictType`/`Create` 均把 `gorm.ErrDuplicatedKey` 翻译为 400/10400「已存在」，但两个 Update 路径都允许修改唯一键列（`dict_type`、`config_key`），Patch 撞 `uk_dict_type`/`uk_config_key` 产生的 `gorm.ErrDuplicatedKey` 未被翻译：service 只判 `errcode.ErrNotFound`，原始错误透传到 `resp.Fail` → `errors.As` 失败 → 500/10001 + slog.Error。用户重命名字典类型/配置键为已存在值（常见操作）收到"内部错误"而非业务 400，且每次触发都污染 error 日志。

**Fix:**
两个 Update 的错误分支与 Create 对齐：

```go
if err := s.typeRepo.Patch(ctx, id, cols); err != nil {
	switch {
	case errors.Is(err, errcode.ErrNotFound):
		return nil, errcode.New(errcode.CodeNotFound, fmt.Sprintf("字典类型不存在: %d", id))
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return nil, errcode.New(errcode.CodeInvalidParam, "字典类型已存在")
	}
	return nil, err
}
```

（config.go Update 同构：「配置键已存在」。）

### WR-03: Excel 导入行数上限在 GetRows 全量载入之后才生效，20MB xlsx zip 炸弹可放大数百 MB 内存

**File:** `internal/service/fault_import.go:64-79`（(a)(b) 两步顺序）

**Issue:**
`maxImportRows=5000` 检查位于 `excelize.OpenReader` + `f.GetRows("Sheet1")` **之后**——而 `GetRows` 本身就是把整表全量载入 `[][]string` 的调用（注释自己也承认这一点，但防线位置在它后面）。xlsx 是 zip 容器：20MB 体积上限内可构造高压缩比工作表（zip 炸弹），解压后 XML 与 GetRows 结果可达数百 MB～GB，行数防线对这一步不起作用。缓解面：`PermFaultImport` 为发明串 v1 仅 super_admin 可用 + `BodyLimitWithMB(20)` 封顶体积——故定 Warning 而非 Critical，但"三维封顶"的注释声明（T-4-11）与实际防线位置不符。

**Fix:**
改用 excelize 流式迭代器，边读边计数边解析、超限即中断（同时天然消除 GetRows 的全量缓冲）：

```go
iter, err := f.Rows("Sheet1")
if err != nil { /* 缺 Sheet1 */ }
// 表头：iter.Next() 取首行校验；随后循环 iter.Next()/iter.Columns()
// 每行先 count++，count > maxImportRows → 立即 400 返回，不再继续读
```

若需保留 GetRows 形态，至少先用 `f.GetSheetDimension("Sheet1")` 预检声明维度做前置拒绝（可被伪造，属降险非根治）。

### WR-04: shift 域 status 绕过全库 UsageStatus 枚举纪律，任意 int8 静默落库

**File:** `internal/model/dto/shift.go:17,28`（`Status *int8`）；`internal/service/shift.go:37-42,65-67`（直存）

**Issue:**
同为 `tinyint DEFAULT '1'` 的 status 列，dict 域 Update 用 `*types.UsageStatus` 严格解析（未知值 400/10401，Phase 2 WR-03 先例）、fileCategory 域同样用 `UsageStatus`（dto/file.go:34），且这两域 Create 均不收 status（服务端默认 1）。shift 域却在 Create/Update 都以裸 `*int8` 接受任意 -128..127（如 42）静默落库、读侧原样回显——输入验证与域内纪律双重不一致。源是否同样宽松可辩，但同仓库同形态字段两套处理至少应显式裁定。

**Fix:**
`ShiftCreateReq.Status`/`ShiftUpdateReq.Status` 改为 `*types.UsageStatus`（create 时忽略客户端值保持服务端默认 1，与其他域对齐；update 时严格解析未知值 400）。若裁定 shift 域 status 无枚举语义（源直译），则在 DTO 注释与 04-CONTEXT 留档该偏离，消除"遗漏 vs 裁定"的歧义。

### WR-05: 上传链对 file_name/file_path 无长度钳位，超长文件名 500 而非 400

**File:** `internal/service/file.go:114,117-125`；`scripts/ddl/sys_file.sql`（`file_name` varchar(255)、`file_path` varchar(500)）

**Issue:**
`key = uuid.NewString() + "_" + fh.Filename`（uuid 恒 36 字符 + 下划线 → filename > 463 即超 `file_path` varchar(500)；filename > 255 超 `file_name`）后 INSERT 无任何长度校验，超长即 strict mode 1406 → 上传事务回滚 → 500/10001。浏览器通常发 basename 不易触发，但任何允许自定义 filename 的客户端（curl `-F 'file=@x;filename=<500字>'`）或未来前端透传路径的场景都会稳定触发。与 CR-02 同根：**落库字符串字段全部信任客户端长度**。

**Fix:**
上传链入口对 `fh.Filename` 做钳位/校验（超 255 → 400「文件名过长」或截断到安全长度后再拼 key），与 CR-02 的 contentType 钳位同一处收口。

## Info

### IN-01: InsertBatch 最坏单批 ≈3.8MB，逼近 MySQL 5.7 默认 max_allowed_packet=4MB

**File:** `internal/middleware/op_log.go:40-42`（批 64 × params 60KB）；`internal/repository/operation_log.go:32-37`

**Issue:**
批满 64 条 × params 截断上限 60KB ≈ 3.75MB，叠加 SQL 文本与其他列、转义膨胀后可能超过 MySQL 5.7 默认 `max_allowed_packet=4MB`（8.0 默认 64MB 无虞）——整批 INSERT 失败、64 条日志丢弃。触发条件苛刻（64 条并发接近满 60KB params），工控机 MySQL 版本未知，故仅记录。

**Fix:** 若现场为 MySQL 5.7，可在 `NewOpLogWriterWithSink` 装配处将 `opLogBatchSize` 降为 32，或在部署清单注明 `max_allowed_packet` 要求。

---

_Reviewed: 2026-11-01T13:10:36Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
