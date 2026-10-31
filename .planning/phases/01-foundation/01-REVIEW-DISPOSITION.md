---
phase: 01
review: 01-REVIEW.md
titles: json
findings:
  - id: IN-01
    severity: info
    disposition: open
    title: "绑定失败回显 validator 原始错误，与 T-1-01 防泄漏纪律不一致（结转，仍成立）"
  - id: IN-02
    severity: info
    disposition: open
    title: "入站 X-Request-ID 无长度/字符集约束即回显并入库日志（结转，仍成立）"
  - id: IN-03
    severity: info
    disposition: open
    title: "Recovery 中 `c.Abort()` 冗余（结转，仍成立）"
  - id: IN-04
    severity: info
    disposition: open
    title: "testutil 镜像探测用字典序选\"最新\" tag，会误选旧版本并误匹配变体镜像（结转，仍成立）"
  - id: IN-05
    severity: info
    disposition: open
    title: "关停集成测试断言 (c) 以响应完成时刻归属观察值，存在微秒级 flake 窗口（结转，仍成立）"
  - id: IN-06
    severity: info
    disposition: open
    title: "ddl-dev 明文密码进进程列表（结转，仍成立）"
  - id: IN-07
    severity: info
    disposition: open
    title: "systemd unit 以 root 运行，加固项偏少（结转，仍成立）"
  - id: IN-08
    severity: info
    disposition: open
    title: "builtin 管理字段可经公开 PUT 任意翻转（依赖 Phase 2 收紧，需跨阶段追踪）（结转，仍成立）"
  - id: CR-01
    severity: critical
    disposition: fixed
    title: "同秒零变更更新被误判 404 —— Patch 以 RowsAffected 判定存在性"
  - id: WR-01
    severity: warning
    disposition: fixed
    title: "IAM_ 环境变量覆盖对 snake_case 键静默失效"
  - id: WR-02
    severity: warning
    disposition: fixed
    title: "http.Server 无传输超时、请求体无大小上限"
  - id: WR-03
    severity: warning
    disposition: fixed
    title: "page 无上界，(page-1)*size 整型溢出 → GORM 静默省略 OFFSET 返回第 1 页数据"
  - id: WR-04
    severity: warning
    disposition: fixed
    title: "连接池参数无校验，0/负值语义为\"无限制\"架空 T-1-04 兜底"
  - id: WR-05
    severity: warning
    disposition: fixed
    title: "请求体读取时间无上界——慢速滴漏 body 可无限期占用连接与 goroutine"
  - id: IN-09
    severity: info
    disposition: fixed
    title: "BodyLimit 注释声称的 requestTooLarge 机制在 gin 下不生效——不实注释误导读者"
  - id: IN-10
    severity: info
    disposition: fixed
    title: "1MB 上限值在 handler 侧以字符串字面量重复，与 middleware.maxBodyBytes 解耦"
open: 8
total: 16
recorded: 2026-10-31T11:41:54.179Z
---

# Phase 01: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| IN-06 | info | open | - |
| IN-07 | info | open | - |
| IN-08 | info | open | - |
| CR-01 | critical | fixed | 01-REVIEW-FIX.iter3.md (not in the current review) |
| WR-01 | warning | fixed | 01-REVIEW-FIX.iter3.md (not in the current review) |
| WR-02 | warning | fixed | 01-REVIEW-FIX.iter3.md (not in the current review) |
| WR-03 | warning | fixed | 01-REVIEW-FIX.iter3.md (not in the current review) |
| WR-04 | warning | fixed | 01-REVIEW-FIX.iter3.md (not in the current review) |
| WR-05 | warning | fixed | 01-REVIEW-FIX.md (not in the current review) |
| IN-09 | info | fixed | 01-REVIEW-FIX.md (not in the current review) |
| IN-10 | info | fixed | 01-REVIEW-FIX.md (not in the current review) |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
