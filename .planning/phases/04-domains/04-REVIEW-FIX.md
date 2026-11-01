---
phase: 04-domains
fixed_at: 2026-11-01T17:45:00Z
review_path: .planning/phases/04-domains/04-REVIEW.md
iteration: 2
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 4: Code Review Fix Report

**Fixed at:** 2026-11-01T16:05:00Z
**Source review:** .planning/phases/04-domains/04-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 7（CR-01/02 + WR-01..05，fix_scope=critical_warning）
- Fixed: 7
- Skipped: 0

**验证环境说明（#2825）：** 全部构建/vet/单测/集成测试运行于隔离 worktree
`.claude/worktrees/rf-04-2374224-1793538852`（分支 `gsd-reviewfix/04-2374224`，基于 f9ef353），
清理时 fast-forward 回 `gsd/phase-02-rbac`。集成测试经 testcontainers 自起 MySQL+Redis 容器。

## Fixed Issues

### CR-01: BodyLimit 对 multipart 全局跳过，非 upload 路由可被无界请求体打 OOM

**Files modified:** `internal/middleware/body_limit.go`, `internal/app/router.go`, `internal/middleware/body_limit_test.go`（新建）
**Commit:** 5b3d429
**Applied fix:** 采用 REVIEW 建议方案一（上限豁免而非防护豁免）：multipart 分支不再直接放行，
改套 `maxMultipartBytes`（`MaxMultipartMB=20` 派生）兜底 MaxBytesReader——与 upload/import
路由级 `BodyLimitWithMB` 同值叠加时内层生效值不变（20MB），D-12 双层封顶设计保留。
`MaxMultipartMB` 按 MaxBodyMB 单一事实源先例导出，router.go 两处 `BodyLimitWithMB(20)` 字面量
改引常量，两层永不漂移。新增 `body_limit_test.go` 三直证：multipart 打普通路由（免认证 login
形态）21MB 被封顶且产出 `*http.MaxBytesError`、双层叠加 15MB 完整可读/25MB 封顶、
JSON 1MB 上限不回归。
**已知残留（无害）：** multipart Content-Type 打 JSON 路由且超 20MB 时，`bindJSON` 的固定文案
仍按全局 1MB 生成（「请求体超过 1MB 上限」）——文案数字与实际上限漂移，但该请求本属无效请求，
行为正确（400/10401 而非无界读入），未为此扩大 bindJSON 改动面。

### CR-02: file_type varchar(50) 小于 xlsx/docx 标准 MIME 长度，浏览器正常上传必失败

**Files modified:** `internal/service/file.go`, `internal/service/file_test.go`（新建）
**Commit:** 9f466af
**Applied fix:** 采用建议 (a)（service 层钳位；DDL 保真不 ALTER，对齐项目「schema 沿用源库」
约束）。新增 `normalizeContentType` 收口上传链：先取分号前 media type 本体（去参数段），仍超
50 字符（rune 语义，MIME 恒 ASCII 等价字节计数）兜底 `application/octet-stream`——上游不可信
字段永不触发 strict mode 1406。空 Content-Type（curl 不带 part 头）原样返回空串，既有落库
行为不变。单测覆盖 xlsx/docx 65 字符标准 MIME、参数段剥离、50/51 边界。

### WR-01: truncateBytes 按字节切断可产生无效 UTF-8，整批日志落库失败

**Files modified:** `internal/middleware/op_log.go`, `internal/middleware/op_log_test.go`
**Commit:** ece85f9
**Applied fix:** 照 REVIEW 建议：截断后回退切点至最近完整 rune 边界
（`for len(s) > 0 && !utf8.ValidString(s) { s = s[:len(s)-1] }`），落库恒为合法 UTF-8，
utf8mb4 params 列不再 1366 整批（64 条）丢日志。补多字节切点直测：59999A+「错」切中回退
（修复前产出无效尾串）、全多字节体恰在边界保留完整 rune、不超限原样返回。

### WR-02: Update 路径唯一键冲突未翻译（500 vs Create 的 400）

**Files modified:** `internal/service/dict.go`, `internal/service/config.go`, `internal/handler/dict_config_shift_integration_test.go`
**Commit:** affc74d
**Applied fix:** 照 REVIEW 建议对齐 Create 翻译惯例：`UpdateDictType` Patch 错误分支补
`errors.Is(err, gorm.ErrDuplicatedKey)` → 400/10400「字典类型已存在」（改 if 为 switch 与
Create 形态一致）；`config.go Update` 同构（400/10400「配置键已存在」）。集成测试各补一条
update 撞 uk 断言（TestDictTypeCRUD / TestConfigCRUDAndKey：400 + code 10400 + 源消息原文）。

### WR-03: Excel 导入行数上限在 GetRows 全量载入之后才生效（zip 炸弹放大）

**Files modified:** `internal/service/fault_import.go`
**Commit:** e573a32
**Applied fix:** 采用建议根治方案：`GetRows` 改 excelize `Rows` 流式迭代器（`defer iter.Close()`），
边读边计数边解析——表头校验（首行）→ 逐行物理行计数（含表头与 skip 行，与原 `len(rows)` 语义
一致）→ 超限读到即断、不再消费迭代器，天然消除全量缓冲面，「三维封顶」（T-4-11）注释与防线
位置自此相符。语义保真点：空 sheet 仍译「表头与模板不符」；行号报错改 1-based 物理行号；
`iter.Error()` 收口迭代错误。轻微行为差异：表头不符 + 超限的文件现报「表头不符」（表头为
首行先校验）——对用户更友好且不影响内存防线。TestFaultImport 六集成用例全过（含
TestFaultImportRowLimit 上限、TestFaultImportHeaderReject 表头拒绝）。

### WR-04: shift 域 status 绕过全库 UsageStatus 枚举纪律

**Files modified:** `internal/model/dto/shift.go`, `internal/service/shift.go`, `internal/handler/dict_config_shift_integration_test.go`
**Commit:** 1d62f03
**Applied fix:** 采用建议枚举对齐方向：`ShiftCreateReq` 删除 Status 字段（create 恒服务端默认 1，
对齐 dict/fileCategory 域 Create 形态；API 契约允许重新设计——CLAUDE.md 约束）；
`ShiftUpdateReq.Status` 改 `*types.UsageStatus`（未知值经 UnmarshalJSON 报错 → 绑定层
400/10401）；`ShiftResp.Status` 一并改枚举形态出网（"ENABLED"/"DISABLED"，经
`UsageStatusFromInt` 兜底转换，与 ToDictTypeResp/ToFileCategoryResp 同构）——读侧原样回显
的纪律缺口同根同修。集成测试补 4 断言：create 默认 "ENABLED"、update `{"status":42}` →
400/10401、`{"status":"DISABLED"}` 合法落库回读、`{"status":1}` 数字双形态回读 "ENABLED"。

### WR-05: 上传链对 file_name/file_path 无长度钳位，超长文件名 500 而非 400

**Files modified:** `internal/service/file.go`, `internal/service/file_test.go`
**Commit:** 3ca7bd7
**Applied fix:** 与 CR-02 同一收口（Upload 入口）：`fh.Filename` 按 rune 计数钳位
（`file_name` varchar(255) 为字符语义，中文名不被字节口径误拒——较 REVIEW 建议的 `len`
字节口径更准确），超 255 → 400/10400「文件名过长」；filename ≤255 时 file_path
≤36+1+255=292 < 500 恒安全。校验在所有依赖使用之前，单测以 nil 依赖构造直测：256 ASCII
/256 汉字超限拒绝、100 汉字（300 字节 > 255 字节但 100 rune 合法）不误拒。

## Skipped Issues

None — scope 内全部修复。

## Deferred（scope 外）

### IN-01: InsertBatch 最坏单批 ≈3.8MB 逼近 MySQL 5.7 默认 max_allowed_packet

**处置：** deferred —— 部署清单项（fix_scope=critical_warning 不含 Info）。与 REVIEW 建议
一致：若现场为 MySQL 5.7，装配处将 `opLogBatchSize` 降为 32，或在部署清单注明
`max_allowed_packet` 要求。工控机 MySQL 版本未知，待部署阶段裁定。

## Verification

- `go build ./...` 通过；`go vet ./...` 通过（gopls 误报按纪律忽略，以 go vet 为准）
- 单测：`internal/middleware`（含新增 body_limit_test.go 全套 + op_log_test.go 全套）、
  `internal/service`（新增 file_test.go）全过
- 集成测试（TZ=UTC，testcontainers 自起双容器）：
  - `TestDictTypeCRUD` / `TestConfigCRUDAndKey`（WR-02 新断言）PASS
  - `TestFaultImport*` 六用例（WR-03 流式重写回归）PASS
  - `TestShiftFullListNoPaging`（WR-04 新断言）PASS
  - `TestFile*` 四用例（CR-02/WR-05 上传链回归）PASS
  - `internal/app` 关停集成（CR-01 路由装配回归）PASS
- 全量回归由编排者统一收口

## Commits

| Finding | Commit | 说明 |
|---------|--------|------|
| CR-01 | 5b3d429 | multipart 套 20MB 兜底封顶 |
| CR-02 | 9f466af | contentType 钳位至 file_type 列宽 |
| WR-01 | ece85f9 | truncateBytes rune 边界回退 |
| WR-02 | affc74d | Update 撞 uk 翻译对齐 Create |
| WR-03 | e573a32 | Excel 导入流式迭代器 |
| WR-04 | 1d62f03 | shift status 对齐 UsageStatus |
| WR-05 | 3ca7bd7 | 上传文件名长度钳位 |

---

_Fixed: 2026-11-01T16:05:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_

---

# Iteration 2 增量报告（2026-11-01T17:45:00Z）

**Source review:** .planning/phases/04-domains/04-REVIEW.md（iter-2 复审：7/7 verified-fixed，新报 WR-06 + IN-02）
**Summary:**
- Findings in scope: 2（WR-06 + IN-02，编排者裁定两项都修——IN-02 虽 Info 但一行归零）
- Fixed: 2
- Skipped: 0

**验证环境说明（#2825）：** 全部构建/vet/单测/集成测试运行于隔离 worktree
`.claude/worktrees/rf-04-2475989-1793542182`（分支 `gsd-reviewfix/04-2475989`，基于 02acc9f），
清理时 fast-forward 回 `gsd/phase-02-rbac`。集成测试经 testcontainers 自起 MySQL+Redis 容器。

## Fixed Issues

### WR-06: truncateBytes 回退循环对本身非法的 UTF-8 输入存在 CPU 放大面（~18.8ms/请求）

**Files modified:** `internal/middleware/op_log.go`, `internal/middleware/op_log_test.go`
**Commit:** 76e16a1
**Applied fix:** 与 REVIEW 建议**同目标、修正实现**：消除 O(n²)（每剥 1 字节 ValidString 全量
重扫），全路径至多 2 次 O(n) 扫描。合法输入用尾部解码回退切点；输入本身非法时一次 range
定位首个非法序列起点直接截断（其后合法内容一并丢弃——params 是请求原文日志，语义无损）。
**与 REVIEW 建议的偏差及原因：** 建议的 `RuneStart` 逐字节回退有两侧缺陷——(1) 切点**恰在
rune 边界**时尾字节是完整 rune 的 continuation（如全「错」60000B 的 `0x99`），会被误剥成
不完整序列，既有用例断言的 20000 rune 将回归成 19999；(2) 切点在多字节中间时剥完
continuation 停在孤立起始字节（如 `E9`）上，合法输入也会掉进 O(n) 兜底扫描。故改用
`DecodeLastRuneInString` 尾部完整解码判定：「RuneError 且 size==1」才视为切点碎片剥除
（合法编码 U+FFFD 解码同为 RuneError 但 size==3，不误剥），完整 rune（ASCII / 恰边界多字节
结尾 / 合法 U+FFFD）不剥。18.8ms 恶意构造（30000A+0xFF+30000B）在新实现下走「尾部 O(1)
判定 + ValidString 失败 + 一次 range 定位」，放大面消除。
**新增测试：** 前置 0xFF 构造（iter-2 实测 18.8ms 场景）断言 30000A 正确切点 + 5ms 时间
上界（与 O(n²) 相差近两个数量级，机器抖动不致误报）、切点尾部非法字节、密集非法点
（0.6ms 构造族）、合法 U+FFFD 恰边界不误截；既有截断用例（TestTruncateBytesUTF8Safe /
TestMaskSensitiveOrderWithTruncation）零回归。

### IN-02: bindJSON 超限文案改由 MaxBytesError.Limit 生成（CR-01 已知残留归零）

**Files modified:** `internal/handler/user.go`, `internal/handler/bindjson_message_test.go`（新建）
**Commit:** b47fc19
**Applied fix:** 照 REVIEW 建议形态：文案实参 `middleware.MaxBodyMB` → `tooLarge.Limit>>20`
（实际触发的截断层上限，无需感知「是哪一层截断」）；各 BodyLimit 层上限均整 MB 派生，右移
整除安全。改动面与现状相同（只换实参来源），随之移除不再使用的 `middleware` import。
Oplog errReader 回放路径原样传递 MaxBytesError，Limit 字段回放后不变，无兼容问题。
**新增测试（免容器，同包直调未导出 bindJSON）：** 流式构造「合法 JSON 头 + 超长字符串值」
body 读至超限点（全零/乱码字节会在第一块就 SyntaxError 读不到超限点——构造注意点），
断言 multipart 超 20MB 时文案含「请求体超过 20MB 上限」而非「1MB」、JSON 路由 1MB 既有
文案不回归。**反证已做：** 修复前版本（stash 还原）下两条断言必失败，测试真实抓住漂移。

## Verification（iter-2）

均在上述 worktree 运行：

- `go build ./...` 通过；`go vet ./...` 通过
- 单测：`internal/middleware`（WR-06 新旧用例全套）、`pkg/...` 全过；
  `internal/handler`（IN-02 新用例，免容器）通过
- 集成测试（TZ=UTC，testcontainers 双容器）：
  `go test -tags=integration -run 'TestOpLog|TestFile|TestFaultImport' ./internal/handler/ -count=1`
  PASS（207s）——TestOpLog 系列走真实 Oplog 中间件 + truncateBytes 落库 utf8mb4 链路，
  对 WR-06 同时构成集成验证
- 全量回归由编排者统一收口

## Commits（iter-2）

| Finding | Commit | 说明 |
|---------|--------|------|
| WR-06 | 76e16a1 | truncateBytes 非法输入 O(n²) 回退消除（一次 O(n) 定位首个非法点） |
| IN-02 | b47fc19 | bindJSON 超限文案改由 MaxBytesError.Limit 生成 |

---

_Fixed: 2026-11-01T17:45:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 2_
