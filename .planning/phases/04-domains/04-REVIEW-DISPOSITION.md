---
phase: 04
review: 04-REVIEW.md
titles: json
findings:
  - id: WR-07
    severity: warning
    disposition: open
    title: "文件分类「移至根」后响应 parentId 为 \"0\" 而非 null——Create/Update 根语义不对称（实测实证）"
  - id: WR-08
    severity: warning
    disposition: open
    title: "TestFileCategoryTree 移至根断言真空通过——envelope 解包形态错误，断言永不失败（实测实证）"
  - id: IN-03
    severity: info
    disposition: open
    title: "NewOpLogWriter（db 直连变体）零调用方——死代码"
  - id: IN-04
    severity: info
    disposition: open
    title: "App.Stop 对 oplog 连续 Flush 两次，最坏排空预算文档口径偏窄"
  - id: IN-05
    severity: info
    disposition: open
    title: "repository/file_info.go List 注释与代码排序方向矛盾"
  - id: CR-01
    severity: critical
    disposition: fixed
    title: "BodyLimit 对 multipart 全局跳过，非 upload 路由可被无界请求体打 OOM"
  - id: CR-02
    severity: critical
    disposition: fixed
    title: "file_type varchar(50) 小于 xlsx/docx 标准 MIME 长度，浏览器正常上传必失败"
  - id: WR-01
    severity: warning
    disposition: fixed
    title: "truncateBytes 按字节切断可产生无效 UTF-8，整批日志落库失败"
  - id: WR-02
    severity: warning
    disposition: fixed
    title: "Update 路径唯一键冲突未翻译（500 vs Create 的 400）"
  - id: WR-03
    severity: warning
    disposition: fixed
    title: "Excel 导入行数上限在 GetRows 全量载入之后才生效（zip 炸弹放大）"
  - id: WR-04
    severity: warning
    disposition: fixed
    title: "shift 域 status 绕过全库 UsageStatus 枚举纪律"
  - id: WR-05
    severity: warning
    disposition: fixed
    title: "上传链对 file_name/file_path 无长度钳位，超长文件名 500 而非 400"
  - id: WR-06
    severity: warning
    disposition: fixed
    title: "truncateBytes 回退循环对本身非法的 UTF-8 输入存在 CPU 放大面（~18.8ms/请求）"
  - id: IN-02
    severity: info
    disposition: fixed
    title: "bindJSON 超限文案改由 MaxBytesError.Limit 生成（CR-01 已知残留归零）"
open: 5
total: 14
recorded: 2026-11-01T16:02:02.769Z
---

# Phase 04: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-07 | warning | open | - |
| WR-08 | warning | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| CR-01 | critical | fixed | 04-REVIEW-FIX.md (not in the current review) |
| CR-02 | critical | fixed | 04-REVIEW-FIX.md (not in the current review) |
| WR-01 | warning | fixed | 04-REVIEW-FIX.md (not in the current review) |
| WR-02 | warning | fixed | 04-REVIEW-FIX.md (not in the current review) |
| WR-03 | warning | fixed | 04-REVIEW-FIX.md (not in the current review) |
| WR-04 | warning | fixed | 04-REVIEW-FIX.md (not in the current review) |
| WR-05 | warning | fixed | 04-REVIEW-FIX.md (not in the current review) |
| WR-06 | warning | fixed | 04-REVIEW-FIX.md (not in the current review) |
| IN-02 | info | fixed | 04-REVIEW-FIX.md (not in the current review) |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
