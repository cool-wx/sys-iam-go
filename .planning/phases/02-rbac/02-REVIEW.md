---
phase: 02-rbac
reviewed: 2026-11-01T02:18:34Z
depth: standard
files_reviewed: 49
files_reviewed_list:
  - internal/auth/session.go
  - internal/auth/session_test.go
  - internal/middleware/auth.go
  - internal/service/auth.go
  - internal/service/user.go
  - internal/service/menu.go
  - internal/service/dept.go
  - internal/service/role.go
  - internal/service/api_key.go
  - internal/handler/auth.go
  - internal/handler/user.go
  - internal/handler/menu.go
  - internal/handler/dept.go
  - internal/handler/role.go
  - internal/handler/api_key.go
  - internal/handler/api_key_integration_test.go
  - internal/model/dto/auth.go
  - internal/model/dto/user.go
  - internal/model/dto/menu.go
  - internal/model/dto/dept.go
  - internal/model/dto/role.go
  - internal/model/dto/api_key.go
  - internal/model/po/role.go
  - internal/model/po/relation.go
  - internal/model/po/api_key.go
  - internal/model/po/menu.go
  - internal/model/po/dept.go
  - internal/repository/user.go
  - internal/repository/relation.go
  - internal/repository/api_key.go
  - internal/repository/menu.go
  - internal/repository/dept.go
  - internal/repository/role.go
  - internal/tree/tree.go
  - internal/testutil/redis.go
  - internal/testutil/mysql.go
  - internal/app/app.go
  - internal/app/router.go
  - internal/config/config.go
  - configs/config.yaml
  - pkg/errcode/errcode.go
  - pkg/types/enum.go
  - pkg/types/menu_type.go
  - pkg/types/datetime.go
  - scripts/ddl/sys_role.sql
  - scripts/ddl/sys_menu.sql
  - scripts/ddl/sys_dept.sql
  - scripts/ddl/sys_api_key.sql
  - scripts/ddl/sys_user_role.sql
  - scripts/ddl/sys_role_menu.sql
findings:
  critical: 0
  warning: 0
  info: 1
  total: 1
status: clean
---

# Phase 2: Code Review Report（iteration 3 终审——收敛判定）

**Reviewed:** 2026-11-01T02:18:34Z
**Depth:** standard（iteration 3/3 最终收敛判定）
**Files Reviewed:** 49（在册全量范围内：本轮精读全部 10 个变更文件 + 5 个跨模块关联文件；其余文件经基线 diff 证明与 iteration 2 全量复审状态逐字节一致）
**Status:** clean（按编排者收敛准则：无 Critical / 无 Warning。唯一 Info 为记录性残留观察，不阻塞收敛，不触发下一轮修复）

## Summary

本轮为 iteration 2 修复后的终审（fixer 提交 a14c918..3c5e48d 共 5 个 commit：WR-06 + IN-09/10/12/13，IN-11 编排者裁决 skipped）。**全部验证门禁独立重跑全绿**：`gofmt -l internal/ pkg/ cmd/` 零输出；`go build ./...`、`go vet ./...` 通过；单元测试全过；**integration 套件 `-count=1` 全量跑通**（本地 mysql-server/redis-server 容器在位，含本轮新增的 TestKickoutAPIKeyRevokesSessionAndMapping 与 TestApiKeyRevokeKillsExistingSession）。

**变更面收敛证明**：`git diff --name-only e665a21..HEAD`（e665a21 = iteration 2 复审所依据的树状态）恰为 5 个修复 commit 触及的 10 个文件——其余 39 个在册文件与 iteration 2 已全量精读并验证的状态**逐字节一致**，其结论继续有效；本轮覆盖即完整覆盖。

### WR-06 修复专项验证（a14c918）——六个维度全部通过

1. **原子性**：`KickoutAPIKey` 的 DEL 会话 + SREM 索引 + DEL 映射收于单 `TxPipelined`（MULTI/EXEC）原子块（`internal/auth/session.go:228-234`），与 Login/Logout 同形状，不存在"会话已删、映射残留"的半态（半态即使出现也只是无害残留方向）。
2. **幂等性**：映射不存在（key 从未签发会话）→ `redis.Nil` → 返回 nil；映射指向死 token → HGet userId 得空串，SREM 落不存在键为 no-op、DEL 幂等，仅剩映射清理，整链不报错。单测三场景（存活撤销/重复撤销/无映射 + 死映射残留）逐一断言。
3. **调用时序**：`UpdateStatus` 前置 `FindByID`（404 消息语义不变）→ Patch 落库 → 仅 `DISABLED` 时 KickoutAPIKey；`Delete` 为 FindByID → 物理删 → KickoutAPIKey（CR-01 用户轴 Delete 同型）。Patch 的 ErrNotFound 防御分支为**活代码**（repo Patch 有 0 行变更 + Count 确认逻辑，`internal/repository/api_key.go:82-91`），非死路径。kick 失败 500 但管理动作已持久——CR-01/IN-06 已裁决语义，残留会话由复用分支 WR-02 状态校验拒绝续命。
4. **撤销覆盖完备性**：router 仅 5 个 api-key 端点（Create/Get/List/UpdateStatus/Delete），Create 恒 ENABLED、无 batch/导入/全量 Update 旁路——生命周期变更写路径只有 UpdateStatus/Delete 两处，均已接线，无遗漏撤销点。
5. **装配链扩参**：`app.go` 中 `sessionMgr` 先于 `NewApiKeyService` 构造（单一 Redis 视图与 AuthService 共享，撤销/绑定/解绑原语一致）；build 全绿证明无其他调用方破坏。
6. **集成直证**：停用 → 旧 token 即刻 401/10100（非仅登录封禁）→ 登录 10503 封禁语义不变 → 重新启用后完整登录换新 token（死映射已清不复用）→ DELETE → 新 token 即刻 401。两轴同构、断言完备。

**并发窗口专项核验**（均不构成缺陷）：①完整登录落在 Get(映射) 与 TxPipelined 之间 → 旧映射被覆盖后又被本原语删除——属**过度撤销**方向（下次登录重新完整登录重建映射），非撤销遗漏；②停用与 apikey-login 毫秒级竞态同 CR-01 已裁决的固有 TOCTOU 窗口，量级与影响一致，可忽略。

### IN-09/IN-10/IN-12/IN-13 顺带修复验证——全部属实、无新缺陷

- **IN-09（e9ced25）**：MenuUpdateReq 注释改为「非空未知值 400/10401（WR-03），null/缺失落 nil 指针」——与 UsageStatus.UnmarshalJSON 实际行为一致，不再描述收紧前旧行为。
- **IN-10（3448073）**：消费方清单经 grep 逐一核实——UpdateStatusReq（dto/user.go:84）/ApiKeyUpdateStatusReq（dto/api_key.go:23）/MenuUpdateReq（dto/menu.go:58）/DeptUpdateReq（dto/dept.go:38）/RoleUpdateReq（dto/role.go:41）五个写 DTO 均持有 `*types.UsageStatus` 字段，注释清单准确。
- **IN-12（d835e96）**：sys_dept.sql 头注释「列集 13 列」与正文 CREATE TABLE 列定义行数（13）相符，修正属实。
- **IN-13（3c5e48d）**：`gofmt -l internal/ pkg/ cmd/` 本轮独立重跑零输出——初审 IN-01 三文件至此全部收口。

### 裁决项处置确认（不重复计）

WR-05 全量授权 deferred Phase 3、IN-11（skipped，visited 集已充分）、WR-01 顺序偏离（已声明有意决策）、kick 失败 500 语义（IN-06 同款已裁决）、源保留缺口（menu/dept 删除带子节点等）——均维持原裁决，本轮不升级、不重复计 finding。

**终审结论**：三轮循环收敛。iteration 1 的 CR-01+WR-01..05 与 iteration 2 的 WR-06+IN-09/10/12/13 全部修复验证通过，无 Critical、无 Warning，无修复引入的新缺陷。Phase 2 代码评审闭环。

## Narrative Findings (AI reviewer)

本轮无 Critical、无 Warning。唯一 Info 为记录性观察，不阻塞 clean 判定（按编排者收敛准则），留档供 Phase 3 反向索引设计一并收口。

### IN-14: 过期 key 的存量会话无自动撤销触发点——与既有「无反向索引」残留同族的记录性边界（记录性，非缺陷，不阻塞）

**File:** `internal/repository/api_key.go:30-42`（FindByKey 恒带 expire_time 过滤——仅封登录）、`internal/middleware/auth.go:46-51`（认证仅校验 Redis 会话存在性）

**Issue:** WR-06 已将**管理动作**（停用/删除）收口为撤销语义；但 `expire_time` 自然过期是被动的——过期时刻无代码执行，已签发会话在认证中间件语义下继续存活且滚动续期（D-07，源未启用 active-timeout）。AKEY-02 契约为登录侧语义（「停用/过期不可登录」），且源系统同构（Sa-Token 会话同样不因 key 过期而失效）。**减轻因素充分故仅记录**：①存在可用补偿路径——过期 key 仍可被管理员显式停用（FindByID 不过滤 expire_time，`internal/service/api_key.go:112` → Patch → KickoutAPIKey 经映射撤销存量会话，与停用轴同链生效）；②用户轴撤销（停用用户/重置密码踢全部会话含 key 签发会话）完整兜底；③与 KickoutAPIKey 注释已声明的「无 token→apiKey 反向索引」残留同族，Phase 3 反向索引（WR-06 Fix ② 预留方案）落地时可一并收口。
**Fix（Phase 3 记录项，本 Phase 无需改动）:** 反向索引落地时将「过期 key 的存量会话撤销」纳入同一收口；或保持现状并在运维规程注明「过期 key 若需即时失效其存量会话，执行一次显式停用」。

---

## 三轮评审收敛台账

| 轮次 | 范围 | 结果 |
|------|------|------|
| 初审（iteration 1） | 64 文件全量 | CR-01 + WR-01..05 + IN-01..08（Info 裁决不修） |
| iteration 1 修复 | cbdfa00..e665a21（7 commits） | 6/6 修复，复审逐一验证通过 |
| iteration 2 复审 | 49 文件全量重读 + 专项 | 0C/1W/5I：新发现 WR-06（存量缺口，非修复引入）；IN-09/10/12/13 注释/格式类；IN-11 裁决 skipped |
| iteration 2 修复 | a14c918..3c5e48d（5 commits） | 5/5 修复（IN-11 skipped 维持裁决） |
| **iteration 3 终审（本轮）** | **10 变更文件精读 + 基线收敛证明 + 全量门禁** | **0C/0W/1I（记录性）→ status: clean，收敛** |

---

_Reviewed: 2026-11-01T02:18:34Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard（iteration 3 终审）_
