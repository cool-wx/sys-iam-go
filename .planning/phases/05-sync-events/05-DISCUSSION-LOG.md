# Phase 5: 名称同步事件与验收收尾 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-11-02
**Phase:** 5-名称同步事件与验收收尾
**Areas discussed:** 事件分发形态, 同步失败语义, 改名后会话快照, 回归证据形态

---

## 灰区选择

用户全选 4 个灰区讨论（未沿用"按先例自主"快捷路径）。

---

## 事件分发形态

| Option | Description | Selected |
|--------|-------------|----------|
| 轻量 dispatcher | internal/event 包：Publisher 接口 + 事件类型 + 监听器注册（slice of listeners，同步调用，约 50-80 行）。对齐源 DomainEvent 心智模型，「事务提交后发布」显式化，可 mock 测试 | ✓ |
| service 直接调用 | service 层事务提交后直接调同步函数。最少抽象，但 6 处挂点耦合，「事件」语义隐式 | |
| Claude 定夺（先例） | 按 Phase 2-4 先例由 Claude 在规划期定夺 | |

**User's choice:** 轻量 dispatcher（推荐）
**Notes:** CLAUDE.md 已排除 watermill 等事件总线框架，选型在此约束之下。

---

## 同步失败语义

| Option | Description | Selected |
|--------|-------------|----------|
| 传播 500 重试自愈 | 沿 Phase 3 casbin 双轨先例（D-13）：主事务已提交、handler 失败上抛 500，客户端重试触发 `!= newName` 幂等 UPDATE 自愈 | ✓ |
| log-and-continue | API 返回成功，同步失败仅 slog 留痕。冗余字段可能不一致且无自愈触发 | |
| boot 对账自愈 | 失败记录待同步清单，启动时对账补齐。最强一致性但引入新存储 | |

**User's choice:** 传播 500 重试自愈（推荐）
**Notes:** 单机场景无跨实例竞争，重试即自愈，不引入对账存储。

---

## 改名后会话快照

| Option | Description | Selected |
|--------|-------------|----------|
| 照源不刷新 | 源监听器只同步 DB 冗余列，会话快照下次登录生效。最保真零成本，displayName 短暂陈旧与源一致 | ✓ |
| 改名即刷快照 | 经 iam:user-tokens 反查在线 token，HSET 更新 userName/realName。源无此行为（增强发明） | |
| 改名即踢下线 | 沿 Phase 2 D-09 停用踢人先例强制重登录。最强一致但改名非安全事件，过度 | |

**User's choice:** 照源不刷新（推荐）
**Notes:** D-09 停用踢人不延伸到改名。

---

## 回归证据形态

| Option | Description | Selected |
|--------|-------------|----------|
| 测试全绿+逐项核对 | 既有全量测试全绿（回归即证）+ 8 项 Looks-Done 清单逐项映射到测试或 VERIFICATION 文档证据；WR-07/08 顺带修复 | ✓ |
| 单一回归套件 | 新建独立 regression 包集中 8 项清单的 E2E 断言。集中可审计但与既有分域测试重复 | |
| 仅既有测试全绿 | 清单人工核对进 VERIFICATION 不补测试；WR-07/08 维持挂账 | |

**User's choice:** 测试全绿+逐项核对（推荐）
**Notes:** WR-07（parentId="0"/null 双形态）与 WR-08（永真断言）随本阶段修复，统一方向研究期对齐源行为后定。

---

## 完成确认

用户确认"准备写入 CONTEXT"，未要求继续探索灰区。

## Claude's Discretion

- dispatcher 内部 API 形态（Publish 签名、事件结构体、注册方式）
- 6 事件 × 关联表映射清单提取（researcher 从源监听器逐表提取）
- NULL 语义锁定测试断言形态
- 测试组织与计划切片/wave 划分

## Deferred Ideas

- 改名即刷在线会话快照（v2 若现场要求 displayName 即时一致）
- 改名即踢下线（讨论中显式排除——安全语义过度）
- 同步失败 boot 对账自愈（若重试自愈现场不足）
- 事件机制向其他域扩展（随需注册）
