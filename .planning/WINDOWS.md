---
schema_version: 1
open_count: 2
waived_count: 0
fixed_count: 0
total_count: 2
last_updated: 2026-11-10T08:34:34.303Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 01 | deviation | .planning/phases/01-foundation/01-02-SUMMARY.md |  | 01-02: 计划 verify 正则遗漏 TestUpdateAuditWithIdentity，已按 Rule 3 修正为 TestUpdateUser\|TestGetUser\|TestUpdateAudit 并闭环验证 | open |  | 2026-10-31T08:59:59.997Z |  |
| 2 | quick-261110-mec | deviation | .planning/quick/261110-mec-audit-db-tables-vs-gorm-models-drop-unus/261110-mec-SUMMARY.md |  | PLAN 基线列数 sys_file(18)/sys_menu(18) 各差 1（正确值 17/19，与代码 PO 一致）；零附带损伤以列名级 DB-vs-PO 比对（17/17）证明 | open |  | 2026-11-10T08:34:34.303Z |  |

````json
[
  {
    "id": 1,
    "kind": "deviation",
    "phase": "01",
    "file": ".planning/phases/01-foundation/01-02-SUMMARY.md",
    "line": null,
    "description": "01-02: 计划 verify 正则遗漏 TestUpdateAuditWithIdentity，已按 Rule 3 修正为 TestUpdateUser|TestGetUser|TestUpdateAudit 并闭环验证",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-10-31T08:59:59.997Z",
    "resolved_at": null,
    "milestone": null
  },
  {
    "id": 2,
    "kind": "deviation",
    "phase": "quick-261110-mec",
    "file": ".planning/quick/261110-mec-audit-db-tables-vs-gorm-models-drop-unus/261110-mec-SUMMARY.md",
    "line": null,
    "description": "PLAN 基线列数 sys_file(18)/sys_menu(18) 各差 1（正确值 17/19，与代码 PO 一致）；零附带损伤以列名级 DB-vs-PO 比对（17/17）证明",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-11-10T08:34:34.303Z",
    "resolved_at": null,
    "milestone": null
  }
]
````
