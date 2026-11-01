---
phase: "03"
slug: "casbin"
status: verified
threats_open: 0
asvs_level: 1
created: "2026-11-01"
---

# Phase 03 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| HTTP → RequirePerm | 36 路由逐条 perm 声明（kickout 叠加 super_admin） | perm 串判定请求 |
| RequirePerm → enforcer | g 层实时判定（D-07 纪律：禁会话快照判据）；enforcer 故障 fail-closed 500 | user→role→perm 策略图 |
| 业务事务 → casbin_rule | 双轨同步（事务提交后、幂等全删全加、去重防 1869）；boot 对账冷启动导入 | p/g 策略行 |
| 列表查询 → datascope | 五档 WHERE 注入（列名硬编码 + 值参数化）；空集/未知档 fail-closed | dept_id/create_by 过滤 |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-3-01 | Elevation | 策略管理端点 | high | mitigate | requirePerm 全量 + super_admin 判据走 enforcer g 层（CR-04 修复：会话快照判据洞封死，回收不可被存量会话抵消）+ builtin 分配守卫 10603（CR-03 修复） | CLOSED |
| T-3-02 | DoS/Info | RequirePerm 故障路径 | high | mitigate | enforcer error 一律 500 Abort fail-closed + slog 三键日志（WR-03 修复补日志） | CLOSED |
| T-3-03 | Tampering | matcher 表达式 | critical | mitigate | super_admin 首位短路 + token 名一致（probe 实证）+ enforcer_test 回归锁 | CLOSED |
| T-3-04 | Tampering | casbin_rule 表 | medium | mitigate | 业务零直写（仅 enforcer API）；D-14 无独立管理端点 | CLOSED |
| T-3-05 | DoS | 冷启动空表 | high | mitigate | boot 对账全量导入 fail-fast（绝不全量 403 静默启动） | CLOSED |
| T-3-06 | Tampering | datascope WHERE 构造 | high | mitigate | 列名硬编码白名单 + 全值参数化占位符 | CLOSED |
| T-3-07 | Elevation | 档位解析 | high | mitigate | 未知档/NULL/无角色/空 DeptIDs 全 fail-closed（1=0 / 最窄档）；binding 1..5 校验（IN-02 修复补 400） | CLOSED |
| T-3-08 | Elevation | scopes 端点越权写 | high | mitigate | RequirePerm(system:role:edit) + deptIDs 前置全量校验 + dept_name 查库冗余 | CLOSED |
| T-3-09 | Elevation | 档位值管理 | medium | mitigate | RequirePerm + Mode 显式枚举域外 fail-closed | CLOSED |
| T-3-10 | Elevation | 漏挂路由 | high | mitigate | 34+2 清单对照 + grep 结构断言 RequirePerm==36/RequireRoles==1 + RouteLockdown 每域采样 | CLOSED |
| T-3-11 | Repudiation/DoS | IN-14 撤销链失败 | medium | accept | 尽力而为不阻断 10503 + DEL 防反复 + user 轴兜底 + TTL 上界（人工验证 P7 已复核形态） | CLOSED（accepted） |
| T-3-12 | DoS | apikey-user 死键 | low | accept | 全清理路径覆盖；残余量级=key 行数可忽略 | CLOSED（accepted） |
| T-3-SC | Tampering | casbin/gorm-adapter 供应链 | high | mitigate | Origin.URL 确认 github.com/casbin 官方组织 + 版本精确 tag 锁定 v2.135.0/v3.35.0 | CLOSED |
| WR-05 | Elevation | system:role:edit 自授面 | warning | accept（deferred） | review 裁决部署信任边界：该 perm 仅授管理员角色（部署清单挂账）；CR-03/04 守卫已封死 builtin 提权链部分缓解 | CLOSED（accepted/deferred） |
| WR-06 | Elevation | builtin 角色停用 | warning | mitigate | 10604 前置守卫（fad2be4 修复：锁死不可自愈面消除） | CLOSED |
| WR-07 | Elevation | 停用角色写路径 | warning | mitigate | AssignMenus/AssignRoles 前置拒绝 DISABLED（19e43ef 修复：p/g 行不可经合法 API 复活） | CLOSED |

---

## Accepted Risks

| ID | Risk | Rationale | Review Date |
|----|------|-----------|-------------|
| T-3-11 | IN-14 撤销链 Redis 故障时尽力而为 | 与 WR-02/IN-06 同族已裁决语义；user 轴管理面兜底 + TTL 最终上界 | v2 如需强一致再议 |
| T-3-12 | iam:apikey-user 死键驻留 | 量级=key 行数，全清理路径已覆盖 | Phase 5 验收回归复核 |
| WR-05 | system:role:edit 自授面 | 源语义无此约束；部署清单落实"仅授管理员角色"；CR-03/04 已封 builtin 链 | 现场部署清单 |

---

## Security Audit

| Field | Value |
|-------|-------|
| Audit date | 2026-11-01 |
| Threats found | 14（plan 登记 13 + review 新增 3 修复项 WR-05/06/07 合并计） |
| Closed | 14 |
| Open | 0 |
| Verification depth | L1 grep + 三轮 code review（16 findings→clean）+ verifier 16/16 行为直证 + 2 项人工形态复核闭环 |
| Short-circuit | asvs_level=1 ∧ plan-time register ∧ threats_open=0 → 免 auditor |
