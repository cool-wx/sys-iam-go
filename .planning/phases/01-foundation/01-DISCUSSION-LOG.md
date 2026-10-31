# Phase 1: 工程骨架与横切地基 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-10-31
**Phase:** 1-工程骨架与横切地基
**Areas discussed:** 骨架自验证方式, Schema 核对途径, 配置与部署形态, 测试基建深度

---

## 骨架自验证方式

| Option | Description | Selected |
|--------|-------------|----------|
| sys_user 最小接口 | 真实 sys_user 表最小读写接口，验证全链路，保留为 Phase 2 起点 | ✓ |
| 临时 demo 域 | 新建 demo 表+临时接口，验收后删除 | |
| 纯单元测试 | 只测横切件本身，无业务接口，端到端验证留给 Phase 2 | |

**User's choice:** sys_user 最小接口
**Notes:** 追问验证深度时的选择：

| Option | Description | Selected |
|--------|-------------|----------|
| 四件套全链路 | create+update+list+get，成功标准 5 条全部端到端可证 | ✓ |
| create+update 二件套 | 更精简但 pkg/paging 无端到端证明 | |

**User's choice:** 四件套全链路

---

## Schema 核对途径 / 配置与部署形态 / 测试基建深度

**User's choice:** （自由文本）"全部由你自己决定，后面的都不需要问我了，直到项目开发完成，该调研就调研，自动选择最优方案往下推进"
**Notes:** 用户授予全量自主决策权。Claude 先做环境调研再拍板（调研证据见 CONTEXT.md code_context）：

- 探测本机环境：发现 Docker 内运行 sysiam-mysql / sysiam-redis / sysiam-nacos / rustfs 四容器（源项目完整本地环境）
- sysiam-mysql 实测：root 常见凭据全拒 → `docker inspect` env 拿到 root/root123 → 库 `sys_iam` 为**空库（0 表）**
- rnacos 实测：v1 API 配置总数 0，无 sa-token 配置可考
- Docker 29.1.3 实测可用 → testcontainers-go 成立

由此产生的自主决策（详见 CONTEXT.md D-03～D-12）：PO 确定性推导 DDL（空库无存量可核对）、单 config.yaml+IAM_ env 覆盖、-config flag 查找、slog JSON→journald、Phase 1 即搭 testcontainers 集成测试。

---

## Claude's Discretion

用户在灰区 1 之后明确授权全量自主决策至项目完成（原文见上）。后续 phase 的 discuss 环节均按此授权执行自主决策 + 环境调研支撑，不再逐项询问。

## Deferred Ideas

- 生产库存量 BCrypt hash 抽样验证 — Phase 2 验收开放项（AUTH-01）
- sa-token timeout/active-timeout 实际值 — rnacos 为空无从考证，Phase 2 以显式配置项默认值替代
- 其余 12 实体建表 DDL — 各域 Phase 落地时逐表推导
