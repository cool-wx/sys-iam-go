---
phase: 05
review: 05-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: warning
    disposition: open
    title: "UserService.Update 的 Patch 路径未译 404 域消息，与本 phase 新增的 pre-read 分支消息契约不一致"
  - id: WR-02
    severity: warning
    disposition: open
    title: "RoleService.Update 中改名 Publish 排在 status→casbin 同步之前，安全步骤被非安全步骤遮蔽"
  - id: WR-03
    severity: warning
    disposition: open
    title: "App.Stop 非幂等——二次调用在 ologW.Flush 处静默阻塞满 5s；TestOlogSurvivesStop 每次运行触发该路径且注释断言相反"
  - id: IN-01
    severity: info
    disposition: open
    title: "UpdateUserReq.RealName 无下界校验，空串真名会把展示名列与审计列批量刷空"
  - id: IN-02
    severity: info
    disposition: open
    title: "审查范围内 4 个文件 gofmt 不合规"
  - id: IN-03
    severity: info
    disposition: open
    title: "file_integration_test.go 下载头断言在头缺失时以 panic 代替干净失败"
  - id: IN-04
    severity: info
    disposition: open
    title: "OnUsernameChanged 为无发布点的注册死代码（Q1 裁决保真，防误判提示）"
  - id: IN-05
    severity: info
    disposition: open
    title: "停用判定以字面量 `!= 1` 散落两处，未用 types 域常量"
open: 8
total: 8
recorded: 2026-11-02T03:34:49.142Z
---

# Phase 05: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| WR-03 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
