---
phase: 05
review: 05-REVIEW.md
titles: json
findings:
  - id: CR-01
    severity: critical
    disposition: open
    title: "D-03「客户端重试幂等自愈」契约被发布守卫证伪——Publish 失败后重试永不重发事件，14 表冗余列永久陈旧"
  - id: WR-01
    severity: warning
    disposition: open
    title: "同实体并发改名可乱序应用同步事件——冗余列终值可落后于主行且无自愈"
  - id: WR-02
    severity: warning
    disposition: open
    title: "空串真名可直达 Update 并触发 26 列清空事件，与 sys_user_role.username 展示名回退语义冲突"
  - id: IN-01
    severity: info
    disposition: open
    title: "listeners.go 包注释「35 条 UPDATE」计数悬空——删除事件后实为 34 条"
  - id: IN-02
    severity: info
    disposition: open
    title: "event.go 包注释「6 个同步 handler」计数悬空——实为 5 个"
  - id: IN-03
    severity: info
    disposition: open
    title: "app.go 装配注释「全部 6 个同步 handler 注册」计数悬空（文件在三文件清单之外，跨模块核查发现）"
  - id: IN-04
    severity: info
    disposition: open
    title: "user.go 字段注释「字段暂存不用」已失真——pub 挂点已实装"
  - id: IN-05
    severity: info
    disposition: open
    title: "Update 的 Patch 缺行路径未译域消息——404 文案「数据不存在」与其余路径「用户不存在: {id}」不一致（上轮 WR-01 遗留，仍在）"
  - id: WR-03
    severity: warning
    disposition: open
    title: "App.Stop 非幂等——二次调用在 ologW.Flush 处静默阻塞满 5s；TestOlogSurvivesStop 每次运行触发该路径且注释断言相反"
open: 9
total: 9
recorded: 2026-11-02T06:57:49.029Z
---

# Phase 05: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| CR-01 | critical | open | - |
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| WR-03 | warning | open | - (not in the current review) |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
