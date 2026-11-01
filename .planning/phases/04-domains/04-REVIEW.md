---
phase: 04-domains
reviewed: 2026-11-01T14:07:00Z
depth: standard
iteration: 2
files_reviewed: 24
files_reviewed_list:
  - internal/middleware/body_limit.go
  - internal/middleware/body_limit_test.go
  - internal/middleware/op_log.go
  - internal/middleware/op_log_test.go
  - internal/model/dto/shift.go
  - internal/service/config.go
  - internal/service/dict.go
  - internal/handler/dict_config_shift_integration_test.go
  - internal/service/fault_import.go
  - internal/service/file.go
  - internal/service/file_test.go
  - internal/service/shift.go
  - internal/app/router.go
  - internal/handler/user.go
  - internal/handler/shift.go
  - internal/handler/file.go
  - internal/handler/fault.go
  - pkg/types/enum.go
  - internal/repository/dict.go
  - internal/repository/sys_config.go
  - scripts/ddl/sys_dict_type.sql
  - scripts/ddl/sys_config.sql
  - scripts/ddl/sys_dict_data.sql
  - scripts/ddl/sys_file.sql
findings:
  critical: 0
  warning: 1
  info: 1
  total: 2
status: issues_found
---

# Phase 4: Code Review Report（iter-2 复审）

**Reviewed:** 2026-11-01T14:07:00Z
**Depth:** standard
**Files Reviewed:** 24（fix 范围 13 文件全量通读 + 11 支撑文件交叉核验）
**Status:** issues_found

## Summary

对 fix commits `5b3d429..3ca7bd7`（7 个修复）做收敛复审：`git diff f9ef353..3ca7bd7` 全量通读确认无夹带改动、注释与实现同步更新；逐修复核对了与项目既有裁定的一致性（CR-01 保留 D-12 双层封顶、CR-02 遵守「schema 沿用源库」不 ALTER、WR-04 对齐 UsageStatus 全库纪律且 API 契约变化合规）；独立运行验证 `go build` / `go vet` 全绿、middleware+service 单测全过、WR-02/WR-04/WR-03 对应的 9 条集成测试（testcontainers 双容器）独立复核全部 PASS。

**结论：7/7 verified-fixed。** 每个修复均真正消除了 iter-1 报告的问题，修复方式与项目惯例一致，新增测试断言真实有效。两个 fixer 声明的已知残留均判定**可接受**（见「已知残留处置」）。

复审新发现 1 Warning + 1 Info：WR-06 是 WR-01 修复方式引入的次生 CPU 放大面（实测 18.8ms/恶意请求，~2000 倍放大，授权用户可确定性触发）；IN-02 是 CR-01 已知残留（文案数字漂移）的一行修复路径——`http.MaxBytesError.Limit` 字段本可让文案与实际上限永不漂移，比保留残留更优。

## 既有 Finding 复核（iter-1 → iter-2）

### CR-01: BodyLimit multipart 兜底封顶 — **verified-fixed**

**证据：**
- `internal/middleware/body_limit.go:43-54`：multipart 分支由直接放行改为套 `maxMultipartBytes`（20MB）兜底 `MaxBytesReader`——采用 iter-1 建议方案一，无界面（免认证 login 打 multipart 超大 body）被消除；非 multipart 的 1MB 防护面不变。
- D-12 双层封顶设计保留：全局外层 20MB + 路由级内层 20MB（`MaxMultipartMB` 单一事实源），内层生效值不变。`internal/app/router.go:174,204` 两处 `BodyLimitWithMB(20)` 字面量改引 `middleware.MaxMultipartMB`，两层永不漂移。
- 新增 `body_limit_test.go` 三直证断言真实：`TestBodyLimitMultipartBoundedOnNonUploadRoutes`（21MB multipart 打普通路由 → `*http.MaxBytesError`）、`TestBodyLimitMultipartUploadRouteDoubleCap`（15MB 完整可读 / 25MB 封顶——合法大上传不回归）、`TestBodyLimitJSONRoutesUnchanged`（1MB 边界不回归）。独立运行全过。
- 交互面核验：Oplog 对 multipart 跳过缓存（内存防线保持）、对非 multipart 超限 body 经 errReader 回放保持 10401 语义（`op_log.go:236-251` 既有逻辑，未回归）；`BodyLimitWithMB` 语义不变，生效值 = min(全局, 路由级) 在同值叠加下正确。

### CR-02: file_type 列宽钳位 — **verified-fixed**

**证据：**
- `internal/service/file.go:169-177` `normalizeContentType`：先取分号前 media type 本体（去参数段），rune 计数超 50 兜底 `application/octet-stream`；空 Content-Type 原样返回（既有行为不变）。采用建议 (a) service 层钳位，遵守「schema 沿用源库」约束未 ALTER DDL。
- 口径正确性核验：`varchar(50)` utf8mb4 字符语义，rune 计数与列宽语义一致；50 rune CJK（200B）恰在 50×4B 上界内存得下，不误判；兜底值本身 24 字符 ≤ 50。
- `file_test.go` 断言真实：xlsx/docx 65 字符标准 MIME、参数段剥离、50/51 边界全覆盖。
- 联动面：钳位后值同时传 `storage.Put`（S3 对象元数据），xlsx 上传对象 Content-Type 变 octet-stream——下载响应头回读 DB 值自洽，功能无损，属 DDL 保真约束下的必然代价，不另立 finding。

### WR-01: truncateBytes rune 边界回退 — **verified-fixed（含次生面，另报 WR-06）**

**证据：**
- `internal/middleware/op_log.go:321-330`：截断后 `for len(s) > 0 && !utf8.ValidString(s) { s = s[:len(s)-1] }` 回退切点——合法 UTF-8 输入（正常场景）输出恒为合法 UTF-8，utf8mb4 params 列 1366 整批丢日志的确定性触发面消除。
- `op_log_test.go` `TestTruncateBytesUTF8Safe` 直证多字节切点回退（59999A+「错」修复前产出无效尾串、修复后回退），补齐了 iter-1 指出的测试缺口。
- **但回退循环对「输入本身非法」的场景存在 CPU 放大次生面 → 新 finding WR-06。**

### WR-02: Update 撞 uk 翻译对齐 Create — **verified-fixed**

**证据：**
- `internal/service/dict.go:74-82` / `internal/service/config.go:68-76`：Update 的 Patch 错误分支补 `errors.Is(err, gorm.ErrDuplicatedKey)` → 400/10400「字典类型已存在」/「配置键已存在」，if 改 switch 与 Create 形态一致，翻译链（TranslateError 开启 → 1062 → `gorm.ErrDuplicatedKey`）核验成立。
- **覆盖完整性核验**：DDL 实证 Phase 4 六域唯一键仅 `uk_dict_type` / `uk_config_key`，其余四域（dict_data/fault/shift/file）无唯一键——修复范围完整，不存在同形态兄弟缺口。
- 集成断言（TestDictTypeCRUD update rename 撞 uk、TestConfigCRUDAndKey update 撞 uk：400 + 10400 + 源消息原文）独立复核 PASS。

### WR-03: Excel 导入流式迭代器 — **verified-fixed**

**证据：**
- `internal/service/fault_import.go:74-153`：`GetRows` 全量载入改为 `Rows` 流式迭代器（`defer iter.Close()`），行数防线在读取流阶段生效（`rowCount > maxImportRows` 立即 400、不再继续消费迭代器）——20MB zip 炸弹的「解压后全量缓冲」放大面消除，「三维封顶」注释自此与防线位置相符。
- 语义保真逐项核验：空 sheet 仍译「表头不符」（`rowCount == 0` 分支）；行号报错维持 1-based 物理行号（原 `i+1` 与新 `rowCount` 等价）；skip 行计入 rowCount 与原 `len(rows)` 一致；`iter.Error()` 收口迭代错误；`cell()` 边界取值保留（流式 Columns 同样跳过尾部空单元格）；预分配 `make(..., 0, maxImportRows-1)` 约 40KB 指针数组，合理。
- TestFaultImport 六集成用例（Upsert/LocationNullNoDedup/TxRollback/HeaderReject/RowLimit/Yf175aData）独立复核全 PASS——流式重写对导入主链零回归。

### WR-04: shift status 对齐 UsageStatus 枚举纪律 — **verified-fixed**

**证据：**
- `internal/model/dto/shift.go`：`ShiftCreateReq` 删 Status 字段（服务端恒默认 1，`shift.go:38-39`，对齐 dict/fileCategory 域 Create 形态）；`ShiftUpdateReq.Status` 改 `*types.UsageStatus`（`pkg/types/enum.go` UnmarshalJSON 实证：`42` → 报错 → 绑定层 400/10401；`"DISABLED"/"0"`/`"ENABLED"/"1"` 双形态合法）；`ShiftResp.Status` 枚举出网 + `ToShiftResp` 经 `UsageStatusFromInt` 兜底——读侧纪律缺口同根同修，与 ToDictTypeResp 同构。
- 契约变化合规：CLAUDE.md 明确「API 契约允许重新设计」；grep 实证 `ShiftResp` 无其他内部消费方，出网形态 int8→字符串无内部回归面。
- 集成断言 4 条（create 默认 "ENABLED"、`{"status":42}` → 400/10401、`{"status":"DISABLED"}` 合法落库回读、`{"status":1}` 数字双形态回读 "ENABLED"）独立复核 PASS。

### WR-05: 上传链文件名长度钳位 — **verified-fixed**

**证据：**
- `internal/service/file.go:64-66`：入口 `utf8.RuneCountInString(fh.Filename) > 255` → 400「文件名过长」，校验位于所有依赖使用之前（nil 依赖可直测）。rune 口径较 iter-1 建议的字节口径更准——`file_name varchar(255)` 为字符语义，中文名不被字节计数误拒。
- 边界核验：`key = uuid(36) + "_" + filename ≤ 292 < 500`，file_path 恒安全（注释推导正确）。
- `file_test.go` 断言真实：256 ASCII / 256 汉字（768B）超限拒绝、100 汉字（300B > 255B 但 100 rune）不误拒——直接证明了 rune 口径。

### IN-01: InsertBatch 逼近 max_allowed_packet — **deferred（合理）**

fix_scope=critical_warning 不含 Info，deferred 为部署清单项与 iter-1 建议一致，裁定合理。建议部署阶段若确认 MySQL 5.7 则装配处降 `opLogBatchSize` 至 32。

## 已知残留处置（fixer 声明，复审判定）

1. **CR-01 multipart 打 JSON 路由超 20MB 时文案数字漂移（报「超过 1MB 上限」）— 可接受，且存在一行修复路径（IN-02）。** 行为正确（400/10401 而非无界读入），该请求属畸形请求（multipart Content-Type 打 JSON 契约路由），文案漂移不误导合法用户。但 `http.MaxBytesError` 自带 `Limit` 字段，bindJSON 文案本可由截断层实际 limit 生成——见 IN-02。
2. **WR-03 表头不符 + 超限文件现报表头不符 — 可接受。** 表头为首行先校验，防线次序（prohibition：表头与行数校验先于数据解析）不变，内存防线不受影响，且「表头不符」对用户更具指向性。

## 新发现（iter-2）

### WR-06: truncateBytes 回退循环对本身非法的 UTF-8 输入存在 CPU 放大面（实测 ~18.8ms/请求，约 2000-4500 倍）

**File:** `internal/middleware/op_log.go:326-328`（回退循环）；调用点 `internal/middleware/op_log.go:281`（assembleEntry，请求 goroutine 的 defer 中同步执行）

**Issue:**
WR-01 修复用 `for len(s) > 0 && !utf8.ValidString(s) { s = s[:len(s)-1] }` 回退切点。对**合法 UTF-8** 输入（正常场景）切点碎片至多 2 字节，回退 ≤2 次、每次 O(n)，实测 4µs——无回归。但对**输入本身非法**（如含 0xFF 字节的二进制 body）时，回退循环每剥 1 字节都要重新 `ValidString` 全量扫描至最靠前的非法点，最坏 O(n²)：

- 触发路径：授权用户对任意挂 Oplog 的写路由（需 perm）发送 >60KB 且构造为「合法 30000B + 1 个非法字节 + 合法余量」的 body → params 截断 60000 字节 → defer 中 `truncateBytes` 同步执行 ~18.8ms CPU（/tmp 独立基准实测：合法多字节切点 4µs、密集非法点 0.6ms、**单非法字节后置构造 18.8ms**）；
- 正常请求该函数 <10µs，放大 ~2000-4500 倍；每请求 CPU 在业务 goroutine 内同步消耗，工控机小核数下持续并发可侵占服务容量；
- 有界（60000 字节内、无累积无持久化）、需认证，故 Warning 而非 Critical；修复前该函数 O(1) 切断无此面，属修复引入的次生攻击面。

**Fix:**
分两层消除 O(n²)：合法输入用 RuneStart 尾部回退（至多 `utf8.UTFMax-1` 次迭代即达边界）；仅当仍非法（输入本身非法）时做**一次** O(n) 扫描定位首个非法字节处截断：

```go
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	// 合法 UTF-8 的切点碎片至多 utf8.UTFMax-1 字节：按 rune 起始字节回退，
	// 恒 ≤3 次迭代（正常路径 O(1) 次 ValidString）。
	for i := 0; i < utf8.UTFMax && len(s) > 0 && !utf8.RuneStart(s[len(s)-1]); i++ {
		s = s[:len(s)-1]
	}
	if utf8.ValidString(s) {
		return s
	}
	// 输入本身非法（非切点问题）：一次扫描定位首个非法序列起点，O(n) 截断，
	// 不再逐字节回退全量重扫（WR-06 CPU 放大面）。
	for i, r := range s {
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
				return s[:i]
			}
		}
	}
	return s
}
```

并补一条恶意非法输入的直测（如 30000A + `\xff` + 30000B，断言返回 30000 字节合法前缀）。

### IN-02: bindJSON 超限文案可由 MaxBytesError.Limit 生成，一行消除 CR-01 残留的文案漂移

**File:** `internal/handler/user.go:35-40`（bindJSON 的 MaxBytesError 分支）

**Issue:**
CR-01 修复后，multipart Content-Type 打 JSON 路由超 20MB 时 `bindJSON` 的固定文案仍按 `middleware.MaxBodyMB` 生成「请求体超过 1MB 上限」，与实际上限 20MB 漂移（fixer 已声明为无害残留）。`*http.MaxBytesError` 结构体自带 `Limit int64` 字段——即实际触发的截断层的 limit，文案本可精确生成，无需感知「是哪一层截断」。所有 BodyLimit 层上限均 `<<20` 派生整 MB 值（`MaxBodyMB`/`MaxMultipartMB`），整除安全。

**Fix:**
```go
if errors.As(err, &tooLarge) {
	// 文案由实际触发的截断层 limit 生成（MaxBytesError.Limit）——multipart 打
	// JSON 路由超全局 multipart 兜底时文案数字不再漂移（IN-02）。
	resp.Fail(c, errcode.New(errcode.CodeValidation,
		fmt.Sprintf("请求体超过 %dMB 上限", tooLarge.Limit>>20)))
	return false
}
```

改动面与现状相同（只换实参来源），消除后 CR-01 的「已知残留」归零。`internal/middleware/op_log.go` 的 errReader 回放路径原样传递 `MaxBytesError`，Limit 字段在回放后保持不变，无兼容问题。

---

_Reviewed: 2026-11-01T14:07:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Iteration: 2（fix 收敛复审：7/7 verified-fixed，新报 1W+1I）_
