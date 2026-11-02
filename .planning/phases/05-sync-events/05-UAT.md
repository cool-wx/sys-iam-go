---
status: complete
phase: 05-名称同步事件与验收收尾
source: [05-VERIFICATION.md]
started: 2026-11-02T04:05:00Z
updated: 2026-11-02T08:35:49.726Z
---

## Current Test

[testing complete]

## Tests

### 1. Q1 裁决复核——UsernameChanged 事件验收口径
expected: 确认按源语义口径验收（UsernameChanged 契约齐备但无发布点系源死代码语义原样移植，05-01 flagged_assumptions 显式挂起）；若按字面「6 事件全部可触发」口径则需重开裁决
result: pass
evidence: "用户拍板（2026-11-02）：username 不可修改，去除 UsernameChanged 事件（超出两选项的第三裁决：不保留契约死代码形态）。已落地：事件常量 + OnUsernameChanged handler + RegisterAll 注册行删除，SYNC-01 口径改为 5 个名称变更事件（REQUIREMENTS/ROADMAP 已同步，VERIFICATION.md Q1 段补裁决记录）；build/vet/internal-event 单测全绿。落地提交 16658dc。"

### 2. 分批等价性认可——-p 1 串行 14 批全量回归
expected: 认可 05-03 记录的全量回归门（2026-11-02T02:23:47Z—03:06:07Z，-p 1 串行 14 批，handler 85+1SKIP + service 5 + app 2 + 单元 15 包 + make build/verify-static 全绿）在 1.6GB 宿主机资源约束下等价于单命令全量回归（批间无代码变更、容器每批独立创建销毁且核查零残留；verifier 已独立活体复跑 13 个行为测试全绿佐证）
result: pass
evidence: "用户认可（2026-11-02）：「认可」。"

### 3. 增量 fix 冷启动 smoke + WR-02 空白真名服务端拦截实测
expected: CR-01/WR-01/WR-02 增量修复（df9a63c/bfbc6fd/411b574）后：静态二进制空工作目录冷启动 healthz/readyz 双 200；admin 登录 200 拿 64hex token；无 token 401；Create/Update 传空串或纯空白 realName 一律 400/code 10400「真名不能为空」；正常真名 Create/Update 200
result: pass
evidence: "Claude 实测（2026-11-02，真实 server：make build 静态二进制 45M statically linked，IAM_* env 覆盖连本机 mysql sys_iam_uat 库 17 表 + 种子三件套，Redis db=5 隔离，端口 18085）：healthz/readyz 双 200 {\"status\":\"ok\"}，boot 仅预期 config 缺省 WARN、0 error；admin/admin123 登录 200 拿 64hex token；无 token GET /api/v1/users → 401；Create realName=\"\" 与 realName=\"   \" 均 400/10400「真名不能为空」；Update（PUT /api/v1/users/{id}）realName=\"\" 与 \"  \" 均 400/10400；正常真名 Create（验证用户甲）200（雪花 ID 2117158861046550528）、Update 改名 200。SIGTERM 优雅退出「shutdown complete」。测试员失误 2 笔已甄别：首建带不存在 deptId=100 → 400「部门不存在: 100」（域校验正常工作的直证，非缺陷）；bash 只读变量 UID 误用致一次 Update 打到 admin（行为等价，已恢复）。"

### 4. CR-01 重试幂等自愈语义活体实测（真实 server 事件链）
expected: 改名后冗余展示名列（sys_user_role.username，实存展示名）自动同步新真名（RealNameChanged 事件链）；对同一用户再次 PUT 相同新名（主行已是新名的重试形态——旧守卫下零发布、自愈断裂；新语义下重发事件、幂等 no-op）返回 200 且冗余列无破坏
result: pass
evidence: "Claude 实测（2026-11-02）：PUT /users/{id}/roles 挂 super_admin 后 sys_user_role.username=「验证用户甲」（AssignRoles 展示名回退初值）；PUT realName=「新名乙」→ 200 且 sys_user_role.username 自动同步=「新名乙」（事件链直证）；再次 PUT 相同「新名乙」（重试形态）→ 200，冗余列保持「新名乙」无破坏（幂等 no-op），主行 real_name 终值一致；admin 真名恢复 PUT「管理员」→ 200 且其 sys_user_role.username 同步回「管理员」（事件链二次直证）。与集成测试 TestNameChangePublish{Role,Menu,DictType,UserRealName} 4/4 PASS（CR-01 行为锁定断言，每测试独立容器零残留）互证。"

### 5. 增量 fix 后全量回归门（单元 + 集成分批）
expected: CR-01/WR-02 改动 internal/event + internal/service + internal/model/dto 后：全量单元测试（15 包，无容器）+ 集成测试受影响包（service/handler/app，testcontainers 串行 -p 1、任意时刻 ≤1 MySQL 测试容器、用完即删）全绿，无跨 phase 回归
result: pass
evidence: "Claude 执行（2026-11-02，Q2 认可的分批等价形态）：批 0 单元 `go test ./... -count=1` 15 包全绿；批 1a service 集成（TestNameChangePublish* 4 项 + TestUser 系）PASS 68.7s，每测试独立容器创建→销毁零残留；批 2 handler 全量集成首轮 2 FAIL：TestDeptNameSync（确定性——CR-01 语义变更后 handler 层同名重放断言未随 service 层同步更新，系 code-review-fix 遗漏面，非产品缺陷，断言已按新契约对齐修复）+ TestUpdateUserSameSecondIdempotentRetry（61.85s 异常超长后失败、单独复现 PASS 20.3s、-count=3 稳定性验证，判环境 flaky——1.6GB 内存下 85 测试连跑的容器就绪压力）；批 2 复跑全量 85+1SKIP 全绿；批 3 app 集成 2/2 PASS（TestReadinessFlipBeforeDrain 34.07s + TestGracefulShutdownDrains 22.32s）。每批 testcontainers 容器独立销毁，全程与常驻 mysql/redis 容器无冲突。"

## Summary

total: 5
passed: 5
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
