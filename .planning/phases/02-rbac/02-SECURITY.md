---
phase: "02"
slug: "rbac"
status: verified
threats_open: 0
asvs_level: 1
created: "2026-11-01"
---

# Phase 02 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| HTTP 客户端 → Gin | 公网/局域网请求进入服务（白名单四路径直通，其余需 Bearer token） | 凭据（username/password/apiKey）、token |
| Gin 中间件 → Redis | 会话校验（HGetAll + EXPIRE）；Redis 故障 fail-closed | 会话四元组（userId/userName/realName/roles） |
| Service 层 → MySQL | GORM 显式映射读写（禁 AutoMigrate）；事务包裹多表写 | 全部业务数据；bcrypt 哈希、apiKey 明文（源语义） |
| 登录路径 → bcrypt | 存量 `$2a$` 哈希校验（恒时比较 + dummy 比较防枚举） | 密码明文（仅此边界内，不落日志） |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-2-01 | Spoofing | AuthService.Login | high | mitigate | 统一 10501 防枚举 + dummyHash 恒时比较（auth.go，集成断言两路径逐字节相同）；WR-01 修复后状态校验后置，禁用名单不可经错密码枚举 | CLOSED |
| T-2-02 | Tampering | randToken/session | high | mitigate | crypto/rand 32 字节 hex（session.go，256-bit 熵） | CLOSED |
| T-2-03 | Spoofing/Replay | SessionManager | high | mitigate | 登录必发新 token + Bearer 传递 + EXPIRE 滚动续期 + logout/kickout 即删（TestAuthFlow/TestKickoutFlow 直证） | CLOSED |
| T-2-04 | DoS | middleware/auth.go | high | mitigate | Redis 故障 fail-closed 500 Abort（middleware/auth.go:35-56 三分支；miniredis Close 用例直证） | CLOSED |
| T-2-05 | Information Disclosure | AccessLog/slog/resp | high | mitigate | 访问日志五字段白名单（Phase 1 既有，grep 零敏感值命中）；UserResp 无 password/extToken 键 | CLOSED |
| T-2-06 | Spoofing | sys_api_key 明文存储 | medium | accept | 源语义 + schema 沿用禁改；明文仅 create 响应一次性返回；工控机本机边界内 | CLOSED（accepted） |
| T-2-07 | Elevation of Privilege | POST /auth/kickout | medium | accept | 原 accept 理由：Phase 2 无功能权限体系。**已超额缓解**：WR-05 修复收紧 RequireRoles(super_admin)（aeea5cb），Phase 3 Casbin 全量收口 | CLOSED（accepted+超额缓解） |
| T-2-08 | DoS | bcrypt.VerifyPassword | medium | mitigate | 任何 bcrypt error 一律译 10501 不 500（本库 arm64 畸形值天然 fixture 直证） | CLOSED |
| T-2-09 | DoS | internal/tree.Build | medium | mitigate | 单遍 children 分组 map 不命中归 roots，无递归父链遍历天然免疫（tree.go:28-32；环 fixture 直证）；WR-04 修复另加写路径成环 400 拦截 | CLOSED |
| T-2-10 | Tampering | RoleService.Delete | medium | mitigate | 三连删 db.Transaction 整体回滚（role.go，集成断言零残留） | CLOSED |
| T-2-11 | Tampering | RoleService.AssignMenus | medium | mitigate | 删旧+批插同事务 + menuIds 前置全量校验 | CLOSED |
| T-2-12 | Elevation of Privilege | builtin 守卫绕过 | low | accept | 管理员显式 edit builtin=false 后删除系源同语义特权操作；10601/10602 守卫拦截直接删除 | CLOSED（accepted） |
| T-2-13 | Elevation of Privilege | UpdateStatus 停用 | high | mitigate | 停用即 KickoutUser（TestUserStatusDisableKickout 三段直证） | CLOSED |
| T-2-14 | Elevation of Privilege | ResetPassword | high | mitigate | 重置即踢全部会话（TestResetPasswordFlow 直证 tokenA 401） | CLOSED |
| T-2-15 | Information Disclosure | UserResp/ApiKeyResp | medium | mitigate | UserResp 全读路径无 password/extToken；ApiKey 明文仅 create 一次性 + 管理面 list（源语义） | CLOSED |
| T-2-16 | Spoofing | ApiKeyRepo.FindByKey | high | mitigate | 恒带 status=ENABLED + expire_time 过滤（api_key.go，停用/过期两用例直证）；WR-06 修复补停用/删除即撤销存量会话（KickoutAPIKey MULTI/EXEC 原子） | CLOSED |
| T-2-SC | Tampering | go get 供应链 | high | mitigate | go-redis/miniredis/testcontainers-redis 四包版本 proxy.golang.org 实测锁定；go.sum 校验和；后续 plan 零新增依赖 | CLOSED |
| CR-01 | Elevation of Privilege | UserService.Delete（review 新增） | high | mitigate | review 发现删除不踢会话——cbdfa00 修复：Delete 事务后 KickoutUser（D-09 语义补全） | CLOSED |
| WR-06 | Elevation of Privilege | ApiKey 停用/删除（review 新增） | high | mitigate | a14c918 修复：KickoutAPIKey 原子撤销存量会话（D-09 apikey 轴同构增强） | CLOSED |

---

## Accepted Risks

| ID | Risk | Rationale | Review Date |
|----|------|-----------|-------------|
| T-2-06 | apiKey 明文存储 | 源语义保真（getByApiKey 精确匹配）+ schema 沿用约束；工控机本机 Redis/MySQL 信任边界内；哈希化需源系统联动改造，v1 移植不做 | Phase 3 复核 |
| T-2-07 | 管理端点授权粒度 | 全量 requirePerm 属 Phase 3 Casbin 范围；kickout 已超额收紧 super_admin | Phase 3 收口 |
| T-2-12 | builtin 守卫可经 edit 绕过 | 源同语义管理面特权；10601/10602 拦直接删除已满足 USER-07/ROLE-02 | Phase 3 复核 |

---

## Security Audit

| Field | Value |
|-------|-------|
| Audit date | 2026-11-01 |
| Threats found | 18（plan 登记 17 + review 新增 2 修复项，T-2-SC 三 plan 合并计 1） |
| Closed | 18 |
| Open | 0 |
| Verification depth | L1 grep + 三轮 code review 交叉 + verifier 27/27 行为直证 |
| Short-circuit | asvs_level=1 ∧ register_authored_at_plan_time ∧ threats_open=0 → 免 auditor（Phase 1 先例） |

---

## Notes

- WR-01 修复（登录状态校验后置）是对 D-02 源顺序保真的**有意覆盖**——防枚举优先，已在 auth.go 注释声明。
- IN-14（过期 key 存量会话无自动撤销触发点）留档 Phase 3 反向索引落地时收口——补偿路径充分（显式停用走同一条 Kick 链 + 用户轴撤销兜底）。
