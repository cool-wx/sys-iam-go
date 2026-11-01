---
phase: 04-domains
reviewed: 2026-11-01T15:59:30Z
depth: standard
iteration: 3
files_reviewed: 36
files_reviewed_list:
  - configs/config.yaml
  - internal/app/app.go
  - internal/app/router.go
  - internal/app/shutdown_integration_test.go
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/handler/bindjson_message_test.go
  - internal/handler/file_category.go
  - internal/handler/file.go
  - internal/handler/file_integration_test.go
  - internal/handler/olog_integration_test.go
  - internal/handler/operation_log.go
  - internal/handler/user.go
  - internal/handler/user_integration_test.go
  - internal/middleware/body_limit.go
  - internal/middleware/op_log.go
  - internal/middleware/op_log_test.go
  - internal/model/dto/file.go
  - internal/model/dto/operation_log.go
  - internal/model/po/file.go
  - internal/model/po/operation_log.go
  - internal/repository/file_category.go
  - internal/repository/file_info.go
  - internal/repository/operation_log.go
  - internal/service/file_category.go
  - internal/service/file.go
  - internal/service/operation_log.go
  - internal/storage/local.go
  - internal/storage/local_test.go
  - internal/storage/s3.go
  - internal/storage/s3_test.go
  - internal/storage/storage.go
  - pkg/paging/paging.go
  - pkg/paging/paging_test.go
  - pkg/resp/resp.go
  - pkg/resp/resp_test.go
findings:
  critical: 0
  warning: 2
  info: 3
  total: 5
status: issues_found
---

# Phase 4: Code Review Report（iter-3 全新复审）

**Reviewed:** 2026-11-01T15:59:30Z
**Depth:** standard
**Files Reviewed:** 36（`files` 配置清单全量通读；4 个真增量文件重点核查，其余安全网复审）
**Status:** issues_found

## Summary

对 `02acc9f..HEAD` 现状做全新对抗复审。真增量代码为 4 文件（`bindjson_message_test.go`、`user.go`、`op_log.go`、`op_log_test.go`，commits `76e16a1`/`b47fc19`），逐行通读 + `git diff 02acc9f..HEAD` 确认无夹带改动；其余 32 文件按 standard 深度全文重读并交叉核验（路由挂载、GORM 钩子、存储路径防御、分页钳位、响应出口契约）。

**独立验证（本机实测）：** `go build ./...`、`go vet ./...` 全绿；middleware/storage/paging/resp/config/handler/service 七包单测 `-count=1` 全过；集成测试 `TestFileCategoryTree`（MySQL 8.4 + Redis 8.8 双容器）实测 PASS；`TestGracefulShutdownDrains` 连跑 6 次全 PASS（针对排空窗口洪流断言做 flaky 探测，未观察到竞态失败）。集成测试依赖在 `GOPROXY=goproxy.cn` 下可复现（默认 proxy.golang.org 当前网络不可达）。

**既有 Finding 复核：iter-1/iter-2 全部 10 项（CR-01/02、WR-01..06、IN-01/02）在当前代码中逐项核实——9 项 verified-fixed，IN-01 维持 deferred（合理），无一回归。** 本轮新发现 **0 Critical + 2 Warning + 3 Info**：两个 Warning 来自同一实测暴露的问题链——`PUT /file-categories/:id {"parentId":0}` 移至根后响应 `parentId` 为 `"0"` 而非 `null`（WR-07），而本应锁住该契约的集成测试断言因 envelope 解包形态错误恒真空通过（WR-08，插桩复现实证）。

## 既有 Finding 复核（iter-1/iter-2 → iter-3）

| Finding | 状态 | 当前代码证据 |
|---------|------|--------------|
| CR-01 multipart 兜底封顶 | **verified-fixed** | `body_limit.go:43-54` multipart 分支套 `maxMultipartBytes`（20MB）`MaxBytesReader`；`router.go:174,204` 路由级 `BodyLimitWithMB(middleware.MaxMultipartMB)` 同值单一事实源 |
| CR-02 file_type 列宽钳位 | **verified-fixed** | `service/file.go:130,165-177` `normalizeContentType` 分号前取 media type 本体、超 50 rune 兜底 octet-stream；service 单测全过 |
| WR-01 truncateBytes rune 边界 | **verified-fixed** | `op_log.go:340-345` `DecodeLastRuneInString` 尾部解码回退（`RuneError && size==1` 判定，合法 U+FFFD size==3 不误剥）；`TestTruncateBytesUTF8Safe` 实跑 PASS |
| WR-02 Update 撞 uk 翻译 | **verified-fixed**（iter-2 已核，文件未再变动） | dict/config service 不在本轮清单，`02acc9f..HEAD` 无改动 |
| WR-03 Excel 导入流式 | **verified-fixed**（iter-2 已核，文件未再变动） | fault_import 不在本轮清单，`02acc9f..HEAD` 无改动 |
| WR-04 shift status 枚举纪律 | **verified-fixed**（iter-2 已核，文件未再变动） | dto/shift、pkg/types/enum 不在本轮清单，`02acc9f..HEAD` 无改动 |
| WR-05 文件名长度钳位 | **verified-fixed** | `service/file.go:64-66` 入口 `RuneCountInString(fh.Filename) > 255` → 400「文件名过长」，先于一切依赖使用 |
| WR-06 truncateBytes O(n²) | **verified-fixed** | `op_log.go:333-358`：尾部解码回退为 O(UTFMax) 常数窗口（连续 continuation 字节最坏也仅每迭代 O(4) 剥 1 字节，整体 O(n)）；非法输入一次 `range` 定位首个非法点截断。`TestTruncateBytesInvalidInputLinear`（含 18.8ms 构造族 + 5ms 阈值）实跑 PASS |
| IN-01 InsertBatch max_allowed_packet | **deferred（维持）** | 部署清单项；64×60KB≈3.84MB 在 MySQL 8 默认 64MB（乃至 5.7 默认 4MB）下均安全，维持不另立 finding |
| IN-02 bindJSON 文案漂移 | **verified-fixed** | `user.go:40` `tooLarge.Limit>>20` 文案由实际截断层生成；`TestBindJSONMaxBytesMessageFromLimit`（20MB multipart 打 JSON 路由报「20MB」+ 1MB JSON 路由不回归）实跑 PASS |

## Critical Issues

（无。安全面逐项复核：SQL 全参数化、存储键路径逃逸防御经 `TestLocalStorageKeyEscapeDefense` 实证、下载 Content-Disposition 经 PathEscape 无头注入、Oplog 掩码先于截断、body/paging/连接池三重钳位齐备——未发现可证明的 Critical 缺陷。）

## Warnings

### WR-07: 文件分类「移至根」后响应 parentId 为 "0" 而非 null——Create/Update 根语义不对称（实测实证）

**File:** `internal/service/file_category.go:69-73`
**Issue:**
Update 的 `parentId:0` 分支写 `cols["parent_id"] = 0`，落库后回读经 `ToFileCategoryResp`（`dto/file.go:77-80`，`m.ParentID != nil` 即回显）输出 `"parentId":"0"`。而 **Create 走根分支时 ParentID 恒 nil → DB NULL → 响应 `parentId:null`**。同一资源「根」状态存在两种出网形态：

- 创建根分类：`"parentId":null`；
- 对任意节点显式 `PUT {"parentId":0}` 移至根：`"parentId":"0"`；且对已是根的分类提交一次看似无操作的 `{"parentId":0}` 更新，会把 `null` 翻转为 `"0"`。

消费端影响真实存在：JS 里字符串 `"0"` 为 truthy，前端以 `parentId` 判根/取父的分支会把根节点误判为「有父分类 0」，随后的父节点查询必然 404。**实测证据**（插桩集成测试捕获真实响应体）：

```json
{"code":0,"message":"ok","data":{"id":"2116918787730051072","parentId":"0","parentName":null,...}}
```

实现者自己的测试意图（`file_integration_test.go:353` `assert.Nil(t, cleared.ParentID)`）即为「移至根后 parentId 应为 null」，故此行为偏离的是实现方自己声明并试图锁定的契约，而非审阅者外加的标准。service 内注释「与 Create 语义对齐」对 parent_id 一列不成立（Create 落 NULL 非 0）。**同型缺口跨域存在**：`service/menu.go:83-85`、`service/dept.go:63-67` 同样写 `parent_id=0`（不在本轮文件清单，修复时应一并核对）。

**Fix:**
```go
if pid == 0 {
    // 显式移至根：parent_id/parent_name 均清 NULL（与 Create 根语义一致；
    // 树查询对 NULL 与 0 同按根处理——po 注释「NULL=根；0 亦按根处理」，
    // 改 NULL 不影响 tree.Build 孤儿提升行为）
    cols["parent_id"] = nil
    cols["parent_name"] = nil
}
```
（替代方案：在 `ToFileCategoryResp` 将 `*m.ParentID == 0` 归一化为 nil——但 DB 层 NULL 化更干净，且使 menu/dept 域可复用同一修法。）

### WR-08: TestFileCategoryTree 移至根断言真空通过——envelope 解包形态错误，断言永不失败（实测实证）

**File:** `internal/handler/file_integration_test.go:348-354`
**Issue:**
`cleared` 结构体直接对**完整 envelope** `body` 反序列化，却在**顶层**找 `parentId`/`parentName`：

```go
var cleared struct {
    ParentID   *string `json:"parentId"`
    ParentName *string `json:"parentName"`
}
require.NoError(t, json.Unmarshal(body, &cleared))
assert.Nil(t, cleared.ParentID)
assert.Nil(t, cleared.ParentName, "显式移至根后 parent_name 必须清空")
```

真实响应中这两个键位于 `data` 之内，顶层只有 `code/message/data`——两个字段**永不填充**，两条 `assert.Nil` 对任意响应（包括完全错误的响应）恒通过，属真空断言（等效于断言缺失）。**实测证据**：在 /tmp 副本插桩 `t.Logf` 复跑，响应 `"parentId":"0"`（违反被测意图）而 `assert.Nil(t, cleared.ParentID)` 依然 PASS——即 WR-07 正是该测试本应拦截却因解包错误漏掉的问题。对照同文件其余解包结构（`up1`/`up5`/`listEnv` 等均正确内嵌 `Data` 层），仅此一处形态错误。

**Fix:**
```go
var cleared struct {
    Code int `json:"code"`
    Data struct {
        ParentID   *string `json:"parentId"`
        ParentName *string `json:"parentName"`
    } `json:"data"`
}
require.NoError(t, json.Unmarshal(body, &cleared))
require.Equal(t, 0, cleared.Code)
// WR-07 修复后此断言才真正回归"锁定 null 契约"的本意
assert.Nil(t, cleared.Data.ParentID)
assert.Nil(t, cleared.Data.ParentName, "显式移至根后 parent_name 必须清空")
```

## Info

### IN-03: NewOpLogWriter（db 直连变体）零调用方——死代码

**File:** `internal/middleware/op_log.go:91-95`
**Issue:** 全库 grep（含测试）无任何 `NewOpLogWriter(` 调用点——生产装配走 `NewOpLogWriterWithSink(ologRepo.InsertBatch, ...)`（`app.go:158`），测试走 `newTestWriter`。该导出构造器为死代码，且其 `db.CreateInBatches` 内联实现绕过了数据访问层收口（repo.InsertBatch）——留着会诱导未来调用方绕层。
**Fix:** 删除该函数；若确需保留直连形态，补一个使用它的单测或改为内部小写。

### IN-04: App.Stop 对 oplog 连续 Flush 两次，最坏排空预算文档口径偏窄

**File:** `internal/app/app.go:255-260`（显式 `Flush(5s)`）+ `internal/middleware/op_log.go:152-162`（`Stop()` 内部再 `Flush(5s)`）
**Issue:** `app.Stop` 先显式 `Flush`（5s 预算）再调 `ologW.Stop()`——`Stop` 内部（started 时）又执行一次同预算 Flush。稳态下第二次空转无成本，但最坏情况（首次 Flush 超时、DB 慢）oplog 排空窗口实为 10s，叠加 15s HTTP 排空后总关停耗时可达 25s——`app.go:30` 注释只声明「systemd TimeoutStopSec 须大于 15s」，未覆盖该上界。
**Fix:** `app.Stop` 去掉显式 Flush、直接 `a.ologW.Stop()`（其内部已按 D-08 次序先 Flush 再关停），或将注释的最坏关停耗时修订为 25s 并对齐 systemd 配置说明。

### IN-05: repository/file_info.go List 注释与代码排序方向矛盾

**File:** `internal/repository/file_info.go:104-106`
**Issue:** 注释写「ORDER BY create_time DESC, id DESC（源排序，**id ASC** tiebreaker 保证同秒稳定序）」——同句内先说 id DESC 又说 id ASC，代码实现为 `Order("create_time DESC, id DESC")`（`file_info.go:126`）。纯注释笔误，但 tiebreaker 方向恰是排文档易误读处（integration 断言锁定的是 DESC）。
**Fix:** 注释改为「id DESC tiebreaker 保证同秒稳定序」。

---

## 复审方法与覆盖说明

- **真增量（4 文件）**：逐行通读 + 与 `02acc9f` diff 对照；`bindJSON` 文案链（`MaxBytesError.Limit` 全层值域 1MB/20MB 整 MB 右移安全性）、`truncateBytes` 新实现逐分支推演（合法切点碎片 ≤3 字节回退、连续 continuation 字节最坏 O(n) 每迭代 O(UTFMax)、非法输入单次 O(n) 定位、合法 U+FFFD size==3 不误剥）——各分支均有对应实跑通过的直测。
- **实测手段**：Docker testcontainers 复跑集成测试；对疑点在 `/tmp` 副本插桩取证（被审树零改动）；`TestGracefulShutdownDrains -count=6` flaky 探测（关停洪流竞态假说被实证否定，不立 finding）。
- **安全网复审**：config fail-fast 键位与测试断言逐条对上（含 koanf 空值 env 覆盖路径）；路由表全量核对 perm/oplog 挂载与 gin 静态/参数段共存；上传判重五语义、存储双实现幂等删除、分页钳位与 envelope [] 非 null 契约均与集成断言闭环。

---

_Reviewed: 2026-11-01T15:59:30Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Iteration: 3（全新复审：既有 10 项 9 fixed + 1 deferred 无回归；新报 2W + 3I，两个 Warning 含实测取证）_
