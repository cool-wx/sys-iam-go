---
phase: 05-sync-events
plan: 03
subsystem: verification
tags: [tree-semantics, gorm-null, parent-id, oplog, graceful-shutdown, regression-gate, acceptance]

# Dependency graph
requires:
  - phase: 05-sync-events (05-01/05-02)
    provides: 6 事件发布挂点与同步 handler 全量实装 + newTestApp/assertCol 测试基建 + -p 1 串行分批回归纪律
  - phase: 04-domains
    provides: 三域 Update 移根分支现状（WR-07/WR-08 挂账原始记录）+ 8 项清单 7 项既有测试
provides:
  - 三域（file_category/dept/menu）移根语义统一：根 = parent_id NULL 唯一形态，响应 parentId 恒 null（WR-07 方向 A）
  - file 移根集成断言真化（WR-08：envelope Data 层解包 + assert.Nil 真断言）
  - TestOlogSurvivesStop：入队→真实 App.Stop()→DB 落库断言一体化直证（清单第 8 项 + IN-04 disposition）
  - VERIFICATION.md：8 项 Looks Done But Isn't 逐项测试映射 + Q1/Q2/WR-07/08/IN-04/D-03/D-04 记录 + 全量回归证据——Phase 5 验收一页证据链
affects: [v1 全量验收（ROADMAP 成功标准 3 已关闭）, /gsd-verify-work 05, 后续树形域"根"语义基线]

# Actuals (#2632) — 与 plan estimate（tokens 45000 / tasks 2）同尺度配对
actuals:
  tokens: 4371         # chars/4 over realized diff（17485 chars，53dbed1..1da2202）
  tasks: 2
  commits: 2           # MEASURED: git rev-list --count 53dbed1..HEAD（#3968）

# Tech tracking
tech-stack:
  added: []            # 零新依赖（零 go get，T-5-SC accept 面不变）
  patterns:
    - "移根写 NULL 模板：cols[\"parent_id\"] = nil（GORM Updates(map) 显式 nil 写 NULL）——三域同款，新树形域照此复制"
    - "关停排空直证模板：写操作入队→不手动 Flush→真实 App.Stop()→DSN 直查断言（TestOlogSurvivesStop）"

key-files:
  created:
    - .planning/phases/05-sync-events/VERIFICATION.md
  modified:
    - internal/service/file_category.go
    - internal/service/dept.go
    - internal/service/menu.go
    - internal/handler/file_integration_test.go
    - internal/handler/olog_integration_test.go

key-decisions:
  - "三域移根根形态唯一化采纳方向 A（DB+响应双恒一，Q4 三域同改裁决）：cols[parent_id]=nil 写 NULL——对源 Create null / Update 0 不对称的 Go 内一致性修正（显式偏离源）；存量 0 行不迁移，tree.Build/ensureNotDescendant/resolveParent 双形态兼容读侧零回归"
  - "WR-07/WR-08 同任务落地（Pitfall 4）：断言真化与响应统一一个提交，杜绝中间红态"
  - "IN-04 disposition：Stop 双 Flush 经 stopOnce 幂等、落库不变量已由 TestOlogSurvivesStop 直证——行为正确无需另改"
  - "TestOlogSurvivesStop 经 API PUT 改名触发 Oplog（复用 05-02 链路），真实 App.Stop()（计划文本 Stop(ctx) 系笔误，实际签名为无参 Stop()）；checkDB 断言全部置于 Stop 后（DSN 新开连接不依赖连接池存活）"
  - "资源约束纪律延续 05-02（编排器硬性约束）：全量回归 -p 1 串行 14 批 + 批间容器清理核查——1.6GB 宿主机任意时刻 ≤1 测试容器，开发库 mysql/redis 全程未触碰"

patterns-established:
  - "根表示唯一契约：树形域 Create 根与 Update 移根均落 parent_id NULL——后续新树形域禁止写 0"
  - "VERIFICATION.md 证据链形态：清单项→测试用例名+文件行号，裁决/修复/disposition 逐节记录——验收文档化模板"

requirements-completed: [SYNC-01, SYNC-02]  # 与 05-02 共同声明（shared-ID gate #2388）：末位 plan SUMMARY 落盘后统一标记

coverage:
  - id: D1
    description: "WR-07：三域 Update 移根 parent_id 落 NULL 唯一形态（根 = NULL 与 Create 根同形态，响应 parentId 恒 null，存量 0 行读侧双兼容）"
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "internal/handler/file_integration_test.go#TestFileCategoryTree"
        status: pass
      - kind: other
        ref: "grep 'cols\\[\"parent_id\"\\] = 0' internal/service/ = 0 命中；三文件各 1 处 nil 形态；TestDeptCRUD/TestMenuCRUD 回归绿"
        status: pass
    human_judgment: false
  - id: D2
    description: "WR-08：file 移根断言经 {code,message,data} envelope Data 层解包真化（assert.Nil(env.Data.ParentID) 为真断言）"
    requirement: SYNC-01
    verification:
      - kind: integration
        ref: "internal/handler/file_integration_test.go#TestFileCategoryTree（移根断言段 json:\"data\" 解包）"
        status: pass
    human_judgment: false
  - id: D3
    description: "清单第 8 项一体化直证：TestOlogSurvivesStop 入队→不手动 Flush→真实 App.Stop()→checkDB 断言落库行要素完整；IN-04 disposition 同面直证"
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: "internal/handler/olog_integration_test.go#TestOlogSurvivesStop"
        status: pass
    human_judgment: false
  - id: D4
    description: "VERIFICATION.md 落盘：8 项清单逐项测试映射 + Q1（username 无触发路径系源语义）/Q2（sys_role_dept Go 扩展）/WR-07/08 修复/IN-04/D-03 证据形态/D-04 快照不刷新 + 回归窗口时间戳"
    requirement: SYNC-02
    verification:
      - kind: other
        ref: "12 关键词 grep 门（存量/API Key/内置/分页/枚举/失败请求/SIGTERM/不清空/UsernameChanged/sys_role_dept/WR-07/IN-04）全命中 VERIFICATION-OK"
        status: pass
    human_judgment: false
  - id: D5
    description: "全量回归门（D-05）：Phase 1-5 全部自动化测试全绿——handler 85+1SKIP/service 5/app 2/单元 15 包/verify-static 出厂门"
    requirement: SYNC-02
    verification:
      - kind: integration
        ref: "go test -tags=integration -p 1 -count=1（14 批串行，2026-11-02T02:23:47Z—03:06:07Z，零 FAIL；唯一 SKIP 为 TestFaultImportYf175a 环境条件跳过，fallback 构造回测已覆盖）"
        status: pass
    human_judgment: true
    rationale: "1.6GB 宿主机资源约束下按 -p 1 串行 14 批执行（编排器硬性约束），非单命令 ./... 一次跑完——『全绿』结论依赖对分批等价性的人工认可；批间无代码变更、容器每批独立创建销毁且批间核查零残留，等价性由执行方式保证"

# Metrics
duration: 60min
completed: 2026-11-02
status: complete
plan_head_before: 53dbed16c3ee7a2eb028bdd4cb9f1d1f5a896c8b
plan_head_after: 1da22023a665f25da3fac062b1dfe3f9db13eb19
commits: 2
---

# Phase 05 Plan 03: WR-07/08 修复与全量验收收尾 Summary

**三域移根 parent_id 落 NULL 唯一形态 + file 断言真化（WR-07/08 同任务关闭）+ TestOlogSurvivesStop 端到端直证 + VERIFICATION.md 八项清单证据链，全量回归门全绿——v1 验收状态达成**

## Performance

- **Duration:** 60 min（含全量回归门 14 批串行 ~42 min）
- **Started:** 2026-11-02T02:09:31Z
- **Completed:** 2026-11-02T03:09:59Z
- **Tasks:** 2
- **Files modified:** 6（新建 1 + 修改 5）

## Accomplishments

- WR-07 关闭：file_category/dept/menu 三域 Update 移根分支 `cols["parent_id"]` 由 0 改 nil（GORM 显式写 NULL）——根 = parent_id NULL 唯一形态，与 Create 根同形态，响应 parentId 恒 null（DTO 零改动）；存量 0 行不迁移，读侧 tree.Build/ensureNotDescendant/resolveParent 双形态兼容零回归
- WR-08 关闭：file_integration_test.go 移根断言补 envelope Data 层解包，`assert.Nil(env.Data.ParentID)` 从永真变为真断言且绿（与 WR-07 同提交，Pitfall 4 防中间红态）
- 清单第 8 项升级：TestOlogSurvivesStop 入队→不手动 Flush→真实 App.Stop()→DSN 直查断言落库（module/operation/method/user_id/real_name/status 完整）——IN-04 disposition：Stop 双 Flush 幂等冗余已直证，无需另改
- VERIFICATION.md 落盘：8 项 "Looks Done But Isn't" 逐项测试映射（7 项存量测试 + 第 8 项新直证）、Q1/Q2 裁决、WR-07/08 修复记录、D-03 证据形态、D-04 快照不刷新说明、全量回归窗口时间戳——12 关键词 grep 门全命中
- 全量回归门（D-05）：handler 12 批 85 通过 + service 5 + app 2 + 单元 15 包全绿 + `make build && make verify-static` 出厂门通过——Phase 1-5 全部自动化测试零回归，**ROADMAP 成功标准 3 关闭，Phase 5 三条 Success Criteria 全部收口**

## Task Commits

Each task was committed atomically:

1. **Task 1: WR-07 三域移根 parent_id 落 NULL + WR-08 file 断言真化** - `23303e5` (feat)
2. **Task 2: TestOlogSurvivesStop + 全量回归门 + VERIFICATION.md** - `1da2202` (test)

**Plan metadata:** 见下方 docs 提交（SUMMARY + STATE + ROADMAP + REQUIREMENTS）

## Files Created/Modified

- `internal/service/file_category.go` - Update 移根分支 parent_id=nil + WR-07 一致性注释重写
- `internal/service/dept.go` - 同款（05-01/05-02 发布挂点零触碰，改动仅移根分支）
- `internal/service/menu.go` - 同款（名不副实的"与 Create 对齐"注释改写为一致性表述）
- `internal/handler/file_integration_test.go` - 移根断言 envelope Data 层解包 + 真断言（WR-08）
- `internal/handler/olog_integration_test.go` - 追加 TestOlogSurvivesStop（清单第 8 项 + IN-04 直证）
- `.planning/phases/05-sync-events/VERIFICATION.md` - 新建：Phase 5 验收一页证据链

## Decisions Made

- 三域移根根形态统一采纳方向 A（DB+响应双恒一）——对源不对称行为的显式 Go 内一致性修正，Q4 裁决三域同改防"同仓库两种根语义"残留
- WR-07/WR-08 同任务单提交落地（计划明文 Pitfall 4：拆分即中间红态）
- TestOlogSurvivesStop 选 PUT /depts 改名触发 Oplog（复用 05-02 已通链路，module=DeptController）；断言全部置于 Stop 之后（DSN 直查不依赖 server/连接池存活）
- **资源约束遵守情况**（编排器硬性约束，全程合规）：①任意时刻 ≤1 测试 MySQL 容器——`go test -p 1` 串行 14 批，包内零 `t.Parallel()`（grep 证实）；②每批结束/中断后 `docker ps -a --filter ancestor=mysql:8.4.11` 核查，测试容器零残留；③名为 `mysql`/`redis` 的开发库容器全程未触碰（吸取 05-02 误删教训，清理匹配恒排除通用命名）；④本约束遵守情况记录于本节及 Issues

## Deviations from Plan

None - plan executed exactly as written。

（计划文本 `testApplication.Stop(ctx)` 与实际 API `Stop()` 签名不一致，按真实无参签名实现——意图（真实 Stop 路径）不变，非行为偏离，记录备查。）

## Issues Encountered

- **资源约束处置**（约束遵守记录）：全量回归按 05-02 先例以 `-p 1` 串行 14 批前台执行（编排器硬性约束，非 plan 偏离）；计划 verify 命令的单命令 `./...` 形态由分批等价执行替代，批间容器核查零残留
- `make verify-static` 首跑因 `bin/sys-iam-go` 未构建而失败——按门设计先 `make build` 再断言（T-1-09 既有用法），通过（statically linked）
- `TestFaultImportYf175aData` SKIP（既有状态）：yf175a 真实数据文件在本开发机不可得，按 Phase 4 D-14 设计 `t.Skip`，fallback 构造数据回测 `TestFaultImportUpsert` 已覆盖同一导入链——已在 VERIFICATION.md 留档，非本计划引入

## User Setup Required

None - no external service configuration required。（零新依赖、零新环境变量、零配置面变化。）

## Next Phase Readiness

- **Phase 5 全部 3 个 plan 完成**：SYNC-01 六事件冗余同步 + SYNC-02 时序/NULL/幂等语义 + WR-07/08 关闭 + 8 项清单证据链——v1 全量验收（ROADMAP 成功标准 3）达成
- SYNC-01/SYNC-02 需求标记在本 plan SUMMARY 落盘后执行（shared-ID gate 解锁）
- 后续入口：`/gsd-verify-work 05`（人工 UAT，coverage D5 分批等价性认可面）→ phase 收尾/里程碑流程
- STATE.md 挂账项"[Phase 4] WR-07/WR-08"已随本 plan 关闭（strikethrough 留痕）

## Self-Check: PASSED

- key-files.created 1 文件存在（`[ -f ]` 验证）；key-files.modified 5 文件均在 53dbed1..1da2202 diff 中
- 任务提交 23303e5 / 1da2202 均为 HEAD 祖先（`git rev-list --count` 台账测量 = 2）
- 计划级 `<verification>` 全部重跑：build/vet 通过；WR-07 grep 旧形态 0/新形态 3；TestFileCategoryTree/TestDept/TestMenu/TestFile 全绿；TestOlogSurvivesStop 通过；全量回归 14 批 + 单元包 + verify-static 全绿；VERIFICATION.md 12 关键词 VERIFICATION-OK
- 需求标记：SYNC-01/SYNC-02 经 requirements.ready-ids 判定后统一标记（见 metadata 提交）

---
*Phase: 05-sync-events*
*Completed: 2026-11-02*
