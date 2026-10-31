---
phase: "01"
slug: "01-foundation"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: "2026-10-31"
---

# Phase 01 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| HTTP 客户端→Gin | 全部请求体/路径参数/query 为不可信输入（Phase 1 无鉴权直通，D-02 临时态） | 任意 JSON / :id / page/size |
| 更新写路径 | 客户端可提交任意 JSON 键——仅白名单列可进 UPDATE | PUT body 键集合 |
| Gin→MySQL | 参数化 SQL 边界（GORM 绑定） | 结构化业务数据 |
| 进程→stdout/journald | 日志输出面（不得含凭据/请求体） | 白名单日志字段 |
| systemd→进程 | SIGTERM/KillSignal 为唯一管理面；unit 硬化项限制进程权限 | 信号 |
| 构建产物→工控机 | 二进制完整性/静态性在构建期断言（出厂门） | ELF 产物 |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-1-01 | Information Disclosure | pkg/resp.Fail / middleware/recovery | high | mitigate | 未知错误固定文案 10001，原始 err 仅 slog（resp.go:28-33）；resp_test 11 处断言原文不出网 | closed |
| T-1-02 | Information Disclosure | middleware/access_log | medium | mitigate | 字段白名单 method/path/status/duration_ms/request_id，不落 body/query（access_log.go:19-23） | closed |
| T-1-03 | Tampering | internal/repository | high | mitigate | GORM 参数化占位符 `Where("id = ?")` ×4 处，repo 层不拼 SQL 字符串；更新列白名单显式 map | closed |
| T-1-04 | DoS | list 分页 | medium | mitigate | paging.Parse size 上限 100（paging.go:18 maxSize=100）+ MaxOpenConns=20 连接池兜底 | closed |
| T-1-05 | Elevation of Privilege | deploy/sys-iam-go.service | medium | mitigate | NoNewPrivileges=true + ProtectSystem=full + 绝对路径 ExecStart（unit :29-30 实读核证；x86 systemd 真实部署实测通过） | closed |
| T-1-06 | Information Disclosure | internal/health | medium | mitigate | /healthz /readyz body 恒 {"status":"ok"|"fail"}（health.go:33-48），不含 DSN/主机/错误细节（2026-10-31 arm64 实测复证） | closed |
| T-1-07 | Tampering | scripts/ddl/sys_user.sql | low | mitigate | 单条 CREATE TABLE IF NOT EXISTS、22 列幂等；无 DROP 无外键（grep 唯一命中为注释自述，假阳性已甄别） | closed |
| T-1-08 | Tampering | Patch 全表更新面 | high | mitigate | Patch 恒带 Where("id = ?")（repository/user.go:54）；GORM 缺 WHERE 拒绝全局更新；RowsAffected==0 回读 Count 判 404（CR-01 修复后） | closed |
| T-1-09 | Tampering | 构建产物（动态链接静默出厂） | high | mitigate | make verify-static 构建期 file 断言 statically linked（Makefile:35）；CGO_ENABLED=0 显式进构建命令；双架构产物实跑验证 | closed |
| T-1-10 | Tampering | repository Patch/List | high | mitigate | 更新列集合=白名单 map 键收敛于指针 DTO（DTO 无审计字段键，防审计伪造）；排序串为代码常量（repository/user.go:93），无 ORDER 注入面 | closed |
| T-1-SC | Tampering | go get 依赖安装 / Makefile 目标 | high | mitigate | 包合法性审计表（01-RESEARCH.md §Package Legitimacy Audit，全部 Approved，无 [SLOP]/[SUS]）；版本全部钉死于 go.mod；Makefile 无新第三方下载步骤 | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|

*Accepted risks do not resurface in future audit runs.*

No accepted risks.

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-31 | 11 | 11 | 0 | Claude (verify-work verify:post step hook, L1 grep-depth per asvs_level=1 short-circuit) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-31
