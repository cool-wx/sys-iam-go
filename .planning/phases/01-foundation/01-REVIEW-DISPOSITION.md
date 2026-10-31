---
phase: 01
review: 01-REVIEW.md
titles: json
findings:
  - id: CR-01
    severity: critical
    disposition: open
    title: "同秒零变更更新被误判 404 —— Patch 以 RowsAffected 判定存在性"
  - id: WR-01
    severity: warning
    disposition: open
    title: "IAM_ 环境变量覆盖对 snake_case 键静默失效（三步转换链把下划线全转点）"
  - id: WR-02
    severity: warning
    disposition: open
    title: "http.Server 无传输超时、请求体无大小上限（slowloris / 大 payload DoS 面）"
  - id: WR-03
    severity: warning
    disposition: open
    title: "page 无上界，(page-1)*size 整型溢出 → GORM 静默省略 OFFSET，返回第 1 页数据却回显超大页码"
  - id: WR-04
    severity: warning
    disposition: open
    title: "连接池参数无校验，0/负值语义为\"无限制\"——架空 T-1-04 声明的连接池兜底"
  - id: IN-01
    severity: info
    disposition: open
    title: "绑定失败回显 validator 原始错误，与 T-1-01 防泄漏纪律不一致"
  - id: IN-02
    severity: info
    disposition: open
    title: "入站 X-Request-ID 无长度/字符集约束即回显并入库日志"
  - id: IN-03
    severity: info
    disposition: open
    title: "Recovery 中 `c.Abort()` 冗余"
  - id: IN-04
    severity: info
    disposition: open
    title: "testutil 镜像探测用字典序选\"最新\" tag，会误选旧版本并误匹配变体镜像"
  - id: IN-05
    severity: info
    disposition: open
    title: "关停集成测试断言 (c) 以响应完成时刻归属观察值，存在微秒级 flake 窗口"
  - id: IN-06
    severity: info
    disposition: open
    title: "ddl-dev 明文密码进进程列表"
  - id: IN-07
    severity: info
    disposition: open
    title: "systemd unit 以 root 运行，加固项偏少"
  - id: IN-08
    severity: info
    disposition: open
    title: "builtin 管理字段可经公开 PUT 任意翻转（依赖 Phase 2 收紧，需跨阶段追踪）"
open: 13
total: 13
recorded: 2026-10-31T09:33:47.921Z
---

# Phase 01: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| CR-01 | critical | open | - |
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| WR-03 | warning | open | - |
| WR-04 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| IN-06 | info | open | - |
| IN-07 | info | open | - |
| IN-08 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
