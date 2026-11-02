---
phase: "05"
slug: "sync-events"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: "2026-11-02"
---

# Phase 05 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| service 层 → event dispatcher（进程内） | 业务事务提交后同步发布 NameChangedEvent；载荷仅服务端构造（查库旧值 + DTO 新值），无客户端直传字段 | ID/新名/旧名（低敏感） |
| dispatcher → handler（进程内） | 同步分发、首错即返（失败上抛 500，D-03）；handler 独立事务执行 UPDATE | 同上 |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-5-01 | Tampering | internal/event/listeners.go 全部 UPDATE | medium | mitigate | 列名恒为代码内字符串常量、值全占位符（grep 实证：34 个 `?` 占位符、无 fmt.Sprintf 拼 SQL——唯一匹配为禁令注释）；集成断言含引号/特殊字符名同步正确（TestDeptNameSync 参数化 T-5-01 直证） | closed |
| T-5-02 | DoS | handler 独立事务（realName 26 条单事务） | low | mitigate | GORM db.Transaction 闭包自动回滚（5 处闭包在案）；无手写 BEGIN/COMMIT 即无悬挂事务；发布时主事务已提交无锁竞争面 | closed |
| T-5-03 | Tampering | NameChangedEvent 载荷 | low | accept | 载荷仅服务端构造，UPDATE 以 ID 为锚不依赖 OldName——进程内无外部入口，伪造面不存在 | closed |
| T-5-04 | Tampering | 发布条件（5 个 service Update） | medium | mitigate | 源五处发布条件逐字移植（5 文件各恰 1 处 `s.pub.Publish(` 在案）+ fakePublisher 四形态单测（仅改 remark 零发布/同名零发布/404 零发布/载荷正确） | closed |
| T-5-05 | Tampering | 三域移根 parent_id NULL 形态变更 | low | mitigate | tree.Build/ensureNotDescendant/resolveParent 对 nil/0 双兼容 + TestFileCategoryTree/TestMenuTree/TestDeptCRUD/TestDeptTreeOrphanPromotion 全量回归兜底；存量 0 行读侧双兼容 | closed |
| T-5-06 | DoS | Stop 排空窗口丢日志 | low | mitigate | App.Stop 的 Shutdown→FlushOpLogs 顺序既有；TestOlogSurvivesStop（olog_integration_test.go:314）固化不变量为回归测试 | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

**UAT Q1 裁决后注记（2026-11-02）**：UsernameChanged 事件契约已整体移除（用户拍板：username 为登录锚点不可改）——其 handler 原不在此威胁登记的组件面内（无发布点、不可触发），移除不改变任何威胁项的处置状态。

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-5-03 | T-5-03 | 事件载荷进程内构造无外部入口，UPDATE 以 ID 为锚——伪造面不存在 | PLAN 05-01 规划期（asvs=1） | 2026-11-02 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-11-02 | 6 | 6 | 0 | gsd-secure-phase L1（asvs=1 短路：register_authored_at_plan_time=true，grep 级全核） |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
