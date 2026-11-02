---
phase: 05-sync-events
verified: 2026-11-02T08:45:44Z
status: passed
score: 14/14 must-haves verified
covered_files:
  - .planning/phases/05-sync-events/05-01-PLAN.md
  - .planning/phases/05-sync-events/05-01-SUMMARY.md
  - .planning/phases/05-sync-events/05-02-PLAN.md
  - .planning/phases/05-sync-events/05-02-SUMMARY.md
  - .planning/phases/05-sync-events/05-03-PLAN.md
  - .planning/phases/05-sync-events/05-03-SUMMARY.md
  - internal/app/app.go
  - internal/event/dispatcher.go
  - internal/event/dispatcher_test.go
  - internal/event/event.go
  - internal/event/listeners.go
  - internal/handler/file_integration_test.go
  - internal/handler/name_sync_integration_test.go
  - internal/handler/olog_integration_test.go
  - internal/model/dto/user.go
  - internal/service/dept.go
  - internal/service/dict.go
  - internal/service/file_category.go
  - internal/service/menu.go
  - internal/service/name_sync_service_integration_test.go
  - internal/service/role.go
  - internal/service/user.go
  - internal/service/user_integration_test.go
covered_digest: "v4:sha256:53a42135025b331a4779cd7d24daab2d836a358b803592d37927e5b3b9a13681"
behavior_unverified: 0
overrides_applied: 1
overrides:
  - must_have: "05-02 P1 prohibition：MUST NOT 无条件发布事件——发布条件必须按『请求带名 且 新名≠旧行名』双条件检测（源五处发布条件逐字移植）"
    reason: "机制子句经 CR-01 裁决显式偏离（df9a63c，05-CONTEXT.md D-03 修订在案）：去除『新名≠旧行名』短路、恢复 D-03 重试幂等自愈（原守卫使 Publish 失败后重试永不重发、26 条冗余列永久陈旧）。禁令的保护性意图——非名字段改动零发布——完整保留：5 挂点显式带名门（old != nil && req.X != nil）grep 复核各恰 1 处，仅改 remark/phone 形态 (b) 零发布经活体测试直证（TestNameChangePublishRole 本轮 PASS）。显式带名的同名请求现仍发布 1 条事件、经 handler `<> newName` 幂等 UPDATE 退化为 no-op——非无条件发布。偏离契约文档（event.go + 5 服务文件）与行为锁定测试已同步改写为真。"
    accepted_by: "user（经编排者 CR-01 裁决链：05-REVIEW-FIX.md 按 critical 修复 + 05-CONTEXT.md D-03 修订记录 + 真实 server UAT test 4 活体实测 200 幂等 no-op，05-UAT.md 5/5 complete）"
    accepted_at: "2026-11-02T06:55:02Z"
re_verification:
  previous_status: passed
  previous_score: 14/14
  gaps_closed: []
  gaps_remaining: []
  regressions: []
---

# Phase 5: 名称同步事件与验收收尾 — 验证报告（Re-verification，STALE 失效后重出）

**Phase Goal:** 5 个名称变更事件的冗余字段同步全部生效（横跨 8 个域，须等所有域 CRUD 稳定后统一实现；源 UsernameChangedEvent 系死代码经 UAT Q1 裁决不移植，2026-11-02），并完成全量验收回归
**Verified:** 2026-11-02T08:45:44Z
**Status:** passed（14/14 全部验证通过，人工项 0）
**Re-verification:** Yes — 上轮报告（06:11:08Z）因增量 code-review 修复轮（CR-01/WR-01/WR-02，df9a63c/bfbc6fd/411b574/520ace4）覆盖源码变更而失效（stale），本报告按当前 HEAD 重出

**Mode:** mvp（用户故事验证收窄沿用前两轮口径：05-01-PLAN objective 内规划者语义等价转写经 `user-story.validate` 为 valid，本验证按该计划内转写执行）

---

## 变更面认定（git 直证）

上轮验证后 `internal/` 实际变更（`git log --since=2026-11-02T06:11:08Z --name-only`）：

| Commit | 变更文件 | 内容 |
|--------|----------|------|
| df9a63c（CR-01） | event.go / dept.go / dict.go / menu.go / role.go / user.go / name_sync_service_integration_test.go | 5 个发布点去除「新名≠旧名」短路（显式带名即发布，恢复 D-03 重试自愈）；契约文档随修复改写；行为锁定测试形态 (c) 改为仍发布恰 1 条 |
| bfbc6fd（WR-01） | event.go | 包注释追加「已知限制」段：同实体并发改名事件无顺序保证（与源 REQUIRES_NEW 同险，单机低并发接受，封闭方案须重过 SYNC-02 prohibition 评审） |
| 411b574（WR-02） | user.go / model/dto/user.go（**新增 covered**） | `validateRealName` helper（空白真名 → 400/10400「真名不能为空」）前置 Create 与 Update 两入口 |
| 520ace4 | handler/name_sync_integration_test.go | TestDeptNameSync 同名重放断言对齐 CR-01 契约（哨兵值被幂等 UPDATE 拉齐，非保持） |

未变更面（listeners.go / dispatcher.go / app.go / file_category.go / 其余测试文件）按 re-verification 分级做存在性 + 基本健全性回归检查，变更面（5 事件契约形状 + WR-02 门）做完整三级验证。

---

## User Flow Coverage

User story: «As a 依赖冗余名称字段的列表/回显/报表消费方, I want to 修改部门/字典类型/菜单/角色名或用户真名后所有关联表冗余名称同步更新, so that Go 版 sys-iam 达到 v1 验收状态（14 域接口 + 自动化测试全部通过）。»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| 修改部门名（PUT /depts/:id） | sys_user.dept_name（dept_id 锚）与 sys_dept.parent_name（parent_id 锚）同步为新名；仅改 remark 零同步 | **TestDeptNameSync 本轮活体 PASS（27.33s，520ace4 对齐后 HEAD 直证）**——含改名双表同步、特殊字符、仅改备注零同步、重放自愈、CR-01 同名重放哨兵值拉齐五语义 | ✓ |
| 修改角色名（PUT /roles/:id） | sys_role_menu / sys_user_role / sys_role_dept 三表 role_name 同步（第三表 Go 扩展，Q2 裁决） | **TestNameChangePublishRole 本轮活体 PASS（20.87s，CR-01 新契约直证：改名 1 条、仅改 remark 零发布、同名仍发布、404 零发布）**；三表端到端 TestRoleNameSync 于 520ace4 后全量 handler 复跑（85+1SKIP 全绿，05-UAT.md test 5）在案 | ✓ |
| 修改菜单名 / 字典类型名 | sys_role_menu.menu_name + sys_menu.parent_name 同步；sys_dict_data.dict_type_name 同步 | menu.go/dict.go 本轮变更同构（短路去除 + 门保留，grep 复核）；TestNameChangePublish{Menu,DictType} 于 service 集成批（PASS 68.7s，05-UAT.md test 5）在案；TestDictTypeNameSync 端到端于全量 handler 复跑在案 | ✓ |
| 修改用户真名（PUT /users/:id realName） | 14 张表 26 列冗余全部同步（逐表断言非抽样）；sys_user_role.username 写展示名（源命名错位原样移植） | user.go 变更面完整复核：validateRealName 双入口前置（:101/:163）、显式带名门 :233、Patch :217→Publish :244 时序在位；TestRealNameSync14Tables + TestNameChangePublishUserRealName + TestUser* 于复跑批次在案；**CR-01 真实 server 活体（05-UAT.md test 4）：改名 → sys_user_role.username 自动同步、同名重试 PUT → 200 幂等 no-op 无破坏** | ✓ |
| WR-02 空白真名防护（新增门） | Create/Update 传空串/纯空白 realName → 400/10400「真名不能为空」 | 代码直证（validateRealName :85 纯函数、双入口先于回读/Patch/发布、DTO 例外注释）；**真实 server 活体（05-UAT.md test 3）：4 面（Create/Update × 空串/空白）全部 400/10400，正常真名 200** | ✓ |
| 误改防护（消费方不被误伤） | 非名字段请求零发布；404 零发布；载荷三字段与 DB 一致 | 5 挂点显式带名门 grep 各恰 1 处（:134/:169/:169/:114/:233）；全仓零残留 `!= old` 短路；形态 (b)/(d) 本轮活体直证（role 域） | ✓ |
| 异常语义（消费方一致性兜底） | NULL 冗余行恒不中；陈旧行下次改名被 `<>` 拉齐；已同步行重放不中 | listeners.go 本轮零改动：`.Update("`=34、`<> ?`=34、`IS NULL`=0 grep 直证；TestNullNameRowsNotSynced/TestIdempotentSelfHeal 于全量复跑在案 | ✓ |
| Outcome：v1 验收态 | 全量回归门全绿；8 项清单逐项过 | **本轮独立复跑：`go test ./... -count=1` 15 包全绿 + build/vet/vet-int 全过 + 2 个变更面集成活体 PASS**；复跑批次（单元 15 包 / service 68.7s / handler 全量 85+1SKIP ok 1544.9s / app 2/2）+ 冷启动 healthz/readyz + SIGTERM 优雅退出（05-UAT.md test 3/5）在案 | ✓ |

**用户流全覆盖：** 五个改名入口 → 全部关联表冗余同步（5 事件 × 关联表全量）→ WR-02 新增门 → 误改防护 → NULL/幂等兜底 → v1 验收 outcome 可观察为真。CR-01 修订后新增的可观察行为（显式带名同名请求仍发布、经幂等 UPDATE 自愈）已由本轮活体测试 + 真实 server UAT 双重直证。

---

## Goal Achievement

### Observable Truths

Re-verification 分级：上轮 14/14 全 VERIFIED 且无 gaps；本轮对未变更面做存在性 + 基本健全性回归，对变更面（CR-01 契约形状 / WR-02 门 / 520ace4 断言）做完整三级验证 + 活体行为直证。

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| T1 | SC1a：dept 改名后 sys_user.dept_name 与 sys_dept.parent_name 同步为新名 | ✓ VERIFIED | **本轮活体 TestDeptNameSync PASS（27.33s，HEAD 直证）**；dept.go 变更面复核：显式带名门 :134、Patch :125→Publish :145 时序在位、零短路残留 |
| T2 | SC1b：role 改名后三表 role_name 同步（第三表 Go 扩展 Q2） | ✓ VERIFIED | role.go 变更面复核：门 :169、Patch :155→Publish :176；**TestNameChangePublishRole 本轮活体 PASS（20.87s）**；三表端到端 TestRoleNameSync 于 520ace4 后全量复跑（85 全绿）在案 |
| T3 | SC1c：menu 改名两表同步 + dictType 改名 dict_type_name 同步 | ✓ VERIFIED | menu.go/dict.go 变更面 grep 复核（门 + 时序 + 零残留）；TestNameChangePublish{Menu,DictType} 于复跑批次（service 集成 PASS 68.7s）在案 |
| T4 | SC1d：realName 改名后 14 表 26 列全量同步（sys_user_role.username 写展示名——源命名错位原样移植） | ✓ VERIFIED | OnUserRealNameChanged 完好（listeners.go:116 起，34 条 `<> ?` UPDATE 中的 26 条）；user.go 变更面复核（门 :233、:217→:244 时序）；TestRealNameSync14Tables 于复跑批次在案 + 05-UAT.md test 4 真实 server 直证 |
| T5 | SC1e：**5 事件契约齐备且零 UsernameChanged 可编译引用**（Q1 裁决移除落地）——event.go 5 常量 + listeners.go 5 handler + RegisterAll 5 注册行 | ✓ VERIFIED | 本轮 grep：常量恰 5（event.go:30-34）、handler 恰 5（:47/:60/:68/:84/:116）、RegisterAll 注册恰 5（:37-41）；全仓 UsernameChanged 命中全为裁决注释（测试文件零命中）；`go build ./... && go vet ./... && go vet -tags=integration ./...` 本轮全过（编译级证明） |
| T6 | SC2a：事件在 service Patch 提交成功之后发布（提交后发布，D-02） | ✓ VERIFIED | 本轮 5 挂点时序逐一复核（CR-01 改写后）：dept :125→:145、role :155→:176、menu（Patch 后）、dict :108 后、user :217→:244——发布语句均物理位于 Patch 之后 |
| T7 | SC2b：handler 同步执行（非 goroutine）且独立事务、首错即返 | ✓ VERIFIED | dispatcher.go 零改动（本轮 grep go/chan=0，仅 sync.RWMutex）；event 单测 4 例本轮活体 ok（0.005s） |
| T8 | SC2c：UPDATE 恒带 `<> newName` 条件，NULL 行不被更新（NULL 语义锁定） | ✓ VERIFIED | 本轮 grep `.Update("`=34、`<> ?`=34、`IS NULL`=0；listeners.go 零改动；TestNullNameRowsNotSynced 于全量复跑在案 |
| T9 | SC2d：幂等自愈 + 发布条件负例（**CR-01 修订后口径**：显式带名即发布，同名重发经幂等 UPDATE 自愈；非名改动/404 零发布不变） | ✓ VERIFIED | **本轮活体双证：TestNameChangePublishRole（形态 b 零发布/c 同名仍发布恰 1 条且载荷 NewName=请求值/d 404 零发布）+ TestDeptNameSync（同名 PUT 哨兵值被拉齐——CR-01 自愈直证）**；其余三域同构测试于复跑批次在案 |
| T10 | SC3a：全量验收回归——14 域全部接口+自动化测试通过 | ✓ VERIFIED | 本轮独立复跑单元 15 包全绿 + build/vet/vet-int 全过 + 2 个变更面集成活体；修复后全量复跑批次（05-UAT.md test 5：service 68.7s、handler 全量 85+1SKIP ok 1544.9s、app 2/2）+ 冷启动/SIGTERM（test 3）在案；上轮 Q2 分批等价认可继续有效 |
| T11 | SC3b：8 项 "Looks Done But Isn't" 清单逐项过 | ✓ VERIFIED | 清单测试文件（olog/file/menu_dept/user 等）不在本轮 diff 内（零改动）；TestOlogSurvivesStop 于全量复跑在案；上轮清单逐项映射（VERIFICATION.md 8 项证据链）结构未变 |
| T12 | 05-03：三域移根 parent_id 落 NULL 唯一形态 + file 移根断言真化（WR-07/WR-08） | ✓ VERIFIED | 三文件（file_category/dept/menu）移根形态本轮零改动；上轮复核旧形态 0/新形态 3 沿用（本轮 diff 不含该三行） |
| T13 | 05-01：dispatcher 基建契约齐备（**5 类型常量**+载荷三元组+Publisher 接口+RegisterAll）与 app.go 装配（ologW.Start :160 → NewDispatcher/RegisterAll :164-165 → userSvc :166） | ✓ VERIFIED | 本轮 grep 复核装配三行在位（:159-166）；`verify.artifacts` 05-01 5/5 本轮复跑全过 |
| T14 | 05-03：验收证据链文档落盘（VERIFICATION.md 8 项映射 + Q1/Q2/WR-07/08 记录 + 回归窗口） | ✓ VERIFIED | 文件直读在案（`verify.artifacts` 05-03 2/2）；05-UAT.md 5/5 PASS（**新增 test 3/4/5 增量修复活体证据**）、05-SECURITY.md threats_open=0 落盘直读 |

**Score:** 14/14 truths verified（0 present-behavior-unverified）

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| internal/event/event.go | 5 类型常量 + NameChangedEvent + Publisher 接口 + D-03 修订/WR-01 已知限制契约文档 | ✓ VERIFIED | 恰 5 常量；CR-01 修订段与 WR-01 已知限制段本轮 diff 直读在位（:3-19） |
| internal/event/dispatcher.go | 同步 Dispatcher（首错即返/未注册 nil） | ✓ VERIFIED | 零改动；go/chan=0 复核 |
| internal/event/listeners.go | 5 handler × 34 条 `<>` UPDATE + RegisterAll | ✓ VERIFIED | 零改动；本轮 grep 5/34/34/IS NULL=0 全中 |
| internal/event/dispatcher_test.go | dispatcher 单测（无容器） | ✓ VERIFIED | 本轮活体 4/4 ok（0.005s） |
| internal/service/dept.go | dept 发布挂点（CR-01 修订形态） | ✓ VERIFIED | :125→:145 时序 + 门 :134 + 零短路（本轮活体 TestDeptNameSync PASS） |
| internal/service/role.go / menu.go / dict.go | 三域发布挂点（同构修订） | ✓ VERIFIED | 各恰 1 处 Publish、零短路；role 域本轮活体 PASS |
| internal/service/user.go | realName 挂点 + WR-02 validateRealName 双入口 | ✓ VERIFIED | :85 helper、:101 Create、:163 Update（均先于回读/Patch/发布）、:233 门、:217→:244 时序 |
| internal/model/dto/user.go | UpdateUserReq.RealName 例外注释（WR-02） | ✓ VERIFIED | **新增 covered 文件**；:31-33 例外于「空串=显式清空」总则注释在位 |
| internal/service/name_sync_service_integration_test.go | 4 域发布条件测试（CR-01 形态 (c) 改写） | ✓ VERIFIED | diff 直读：4 域形态 (c) 均改为「仍发布恰 1 条 + 载荷断言」、(d) 计数 +1；本轮活体 role 域 PASS |
| internal/handler/name_sync_integration_test.go | 7 个端到端同步/锁定/自愈测试（520ace4 对齐） | ✓ VERIFIED | diff 直读：同名重放断言改为哨兵值拉齐（CR-01 自愈）；**本轮活体 TestDeptNameSync PASS（27.33s）** |
| internal/handler/olog_integration_test.go | TestOlogSurvivesStop | ✓ VERIFIED | 零改动；全量复跑在案 |
| internal/handler/file_integration_test.go | WR-08 envelope 真断言 | ✓ VERIFIED | 零改动；上轮直读沿用 |
| .planning/phases/05-sync-events/VERIFICATION.md | 8 项清单证据链 | ✓ VERIFIED | 在案（上轮 Q1 补记段沿用） |

`verify.artifacts` 动词结果（本轮复跑）：05-01 5/5、05-02 6/6、05-03 2/2 全过。
`verify.key-links` 动词：1/3 自动通过；2 条 FAIL 系 `from` 字段带方法后缀的解析限制（"must be a relative file path"），非真实断链——两条均人工复核 WIRED：dept.go :125 Patch→:145 Publish（`s\.pub\.Publish` 在位）；listeners.go :50 `dept_name <> ?`（dept_id 锚）+ :53 parent_name 在位。

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| dept.go Update | dispatcher.Publish | Patch 后 `s.pub.Publish` | ✓ WIRED | :145，位于 :125 Patch 之后（CR-01 改写后复核 + 本轮活体） |
| app.go | listeners.RegisterAll | NewDispatcher + RegisterAll | ✓ WIRED | :164-165 复核在位（动词自动通过） |
| listeners OnDeptNameChanged | sys_user.dept_name / sys_dept.parent_name | 独立事务 `<>` UPDATE | ✓ WIRED | :50 `dept_id = ? AND dept_name <> ?` 本轮直读 |
| role/menu/dict/user Update | dispatcher.Publish | 同构挂点 | ✓ WIRED | 各恰 1 处，均 Patch 后（本轮逐一复核） |
| listeners OnUserRealNameChanged | 14 表冗余列 | 单事务 26 条 `<>` UPDATE | ✓ WIRED | handler 在位（:116 起）；26 条计入总数 34 |
| user Create/Update | validateRealName | 两入口前置调用 | ✓ WIRED | :101/:163（本轮新增面） |
| 三域移根分支 | sys_* parent_id 列 | `cols["parent_id"] = nil` | ✓ WIRED | 零改动；上轮复核沿用 |
| file 移根断言 | PUT /file-categories 响应 envelope | `json:"data"` Data 层解包 | ✓ WIRED | 零改动；上轮直读沿用 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| 5 个发布挂点 | evt{ID,OldName,NewName} | FindByID 回读旧行 + 请求新值（服务端构造） | 是 | ✓ FLOWING |
| 5 个 handler UPDATE（34 条） | evt.NewName | 事件载荷（fakePublisher 四形态断言 + 本轮活体） | 是 | ✓ FLOWING |
| validateRealName | req.RealName | 请求 DTO 指针（TrimSpace 判定） | 是 | ✓ FLOWING |
| 集成测试断言 | 冗余列值 | checkDB 经 DSN 直查真实 MySQL（testcontainers） | 是 | ✓ FLOWING |

无 STATIC/DISCONNECTED/HOLLOW_PROP 面（本轮变更未触及载荷构造——CR-01 仅改发布条件，载荷三元组原样）。

### Behavioral Spot-Checks

本轮活体（变更面直证，均本人独立执行于 HEAD，容器即建即毁、串行、零残留）：

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| 编译（含零可编译 UsernameChanged 引用） | `go build ./...` | BUILD-OK | ✓ PASS |
| 静态检查（含 integration tag） | `go vet ./... && go vet -tags=integration ./...` | VET-OK / VET-INT-OK | ✓ PASS |
| 全量单元测试 | `go test ./... -count=1` | 15 包 ok（含 event 4 例 0.005s） | ✓ PASS |
| role 域 CR-01 发布契约（四形态 + 载荷） | `TZ=UTC go test -tags=integration -p 1 -run '^TestNameChangePublishRole$' ./internal/service/ -count=1` | PASS 20.87s | ✓ PASS |
| dept 域端到端（含 CR-01 同名哨兵拉齐 + 仅改备注零同步） | `TZ=UTC go test -tags=integration -p 1 -run '^TestDeptNameSync$' ./internal/handler/ -count=1` | PASS 27.33s | ✓ PASS |
| 其余行为测试（role/menu/dict/realName 三表与 14 表端到端、NULL/幂等、olog/file） | 修复后全量复跑批次（05-UAT.md test 5：service 68.7s + handler 85+1SKIP ok 1544.9s + app 2/2）+ WR-02/CR-01 真实 server 活体（test 3/4） | 全绿在案 | ✓ PASS（记录） |

### Probe Execution

SKIPPED——本 phase 无 probe 声明（无 scripts/*/tests/probe-*.sh；验收门为 go test 集成/单测 + grep 断言 + make verify-static，已在 Behavioral Spot-Checks 或记录证据中覆盖）。

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| SYNC-01 | 05-01/05-02/05-03（三 plan 均声明） | **5 个名称变更事件**（dept/dictType/menu/role/realName）同步关联表（realName 波及 14 张表）（Q1 裁决后口径，REQUIREMENTS.md:95 已标 [x]） | ✓ SATISFIED | T1–T5 + 34 条 UPDATE 代码 + 本轮活体测试 + 复跑批次全集 |
| SYNC-02 | 05-01/05-02/05-03（三 plan 均声明） | 事务提交后发布、handler 独立事务（防死锁时序）、UPDATE 带 `!= newName` 条件（NULL 语义锁定测试） | ✓ SATISFIED | T6–T9 + 本轮活体（D-02 时序 5 挂点复核 + NULL/幂等语义 grep 与测试）；REQUIREMENTS.md:96 标 [x]、:198 Complete |

Orphaned requirements：无——REQUIREMENTS.md 映射到 Phase 5 的 ID 恰为 SYNC-01/SYNC-02（:197-198），三 plan 的 `requirements:` 字段均完整声明二者。

**SYNC-02 口径说明：** REQ 文本「UPDATE 带 `!= newName` 条件」指 handler 幂等 UPDATE 的 `冗余列 <> newName` 条件（NULL 语义锁定）——该语义经 CR-01 修订**不变**（34 条 `<> ?` grep 直证）；被修订的是**发布条件**（源按值变化检测短路），二者分属 REQ 文本的两端，修订已按「验收口径裁决高于计划文本」惯例记录于 05-CONTEXT.md D-03 并经 override 条目审计。

### Prohibitions 复核（8 条：7 条直接成立 + 1 条机制子句经 override 裁决，保护性意图全部成立）

| Prohibition | 证据 | 结论 |
|-------------|------|------|
| 05-01 P1：禁 goroutine/chan 异步分发、禁事件总线框架 | 本轮 grep go/chan=0（仅 sync.RWMutex）；零新依赖 | ✓ 成立 |
| 05-01 P2：禁 `OR col IS NULL` 放行分支 | 本轮 grep `IS NULL`=0 | ✓ 成立 |
| 05-01 P3：禁"顺手修正"sys_user_role.username 双语义错位 | realName handler 写展示名注释在位；username 登录名 handler 已随 Q1 移除 | ✓ 成立 |
| 05-02 P1：禁无条件发布（双条件检测） | **机制子句（新名≠旧行名）经 CR-01 裁决显式偏离——override 条目在案（frontmatter）**；保护性意图（非名改动零发布）成立：5 挂点显式带名门各恰 1 处 + 形态 (b) 零发布本轮活体直证 | ✓ 成立（override，机制子句被 D-03 修订取代） |
| 05-02 P2：禁增 username 编辑路径/UsernameChanged 发布点 | 全仓零可编译引用（编译 + vet 双证）；cols["username"] 零写入复核 | ✓ 成立 |
| 05-02 P3：禁为绕 NULL 先查后更或补 IS NULL 分支 | 本轮 grep=0；NULL 测试于复跑批次在案 | ✓ 成立 |
| 05-03 P1：禁借收口扩面（不踢线/不刷会话/auth 零改动） | 本轮 4 commit diff 不含 internal/auth；扩面文件零触碰 | ✓ 成立 |
| 05-03 P2：禁以"应该没问题"关闭清单项 | VERIFICATION.md（05-03 交付物）逐项测试映射在案 | ✓ 成立 |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| （无新增） | — | TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER 于本轮变更面 9 文件 + 全 phase 18 文件复扫零命中 | — | — |
| internal/event/event.go:2 | — | 「6 个同步 handler」计数悬空（实为 5）——IN-02 | ℹ️ Info | 已登记 skipped（fix_scope=critical_warning 不含 Info）；注释性失真零行为影响 |
| internal/event/listeners.go:22 | — | 「35 条 UPDATE」计数悬空（实为 34）——IN-01 | ℹ️ Info | 同上 |
| internal/app/app.go:162 | — | 装配注释「全部 6 个同步 handler」悬空——IN-03 | ℹ️ Info | 同上 |
| internal/service/user.go:41 | — | 「字段暂存不用」失真——IN-04 | ℹ️ Info | 同上 |
| internal/service/user.go:219 | — | Patch 错误直返未译域消息（「数据不存在」vs「用户不存在: {id}」）——IN-05 | ℹ️ Info | 同上；仅 RealName 缺席路径可达，零发布、零同步影响 |

（IN-01..IN-05 均为 05-REVIEW-DISPOSITION.md 已登记 skipped 的 Info 项，本轮复核确认现状与登记一致，无新增。）

### 运维与证据卫生记录（非 gap）

| 项 | 现状 | 处置 |
|----|------|------|
| 泄漏测试容器 | docker ps 发现 `condescending_euclid`（testcontainers-go MySQL，labels org.testcontainers lang:go 直证，创建于 07:57:35Z——早于本验证两次活体运行且二者容器均已即建即毁，系上轮回归批次泄漏残留） | **验证者已按用户既定约束（测试容器用完即删、只留 mysql/redis）执行 `docker rm -f` 清除；host 常驻 mysql/redis 容器未触碰（复核在位）**。05-UAT.md test 5「零残留」表述与该残留矛盾——测试结果本身不受影响，属证据卫生口径瑕疵 |
| WR-03（App.Stop 非幂等） | 05-REVIEW-DISPOSITION.md 中 disposition=open（上轮 review 遗留、本轮 review 不再报告，按机制保留记录） | 未 triage、不在 phase must-have 集内（TestGracefulShutdownDrains 于复跑批次 PASS，优雅关停语义本身经活体验证）；建议随下个 review/fix 周期处置，不阻塞本 phase |
| ROADMAP.md:129 plan 行 | 「6 handler × 35 条 `<>` UPDATE」历史计划文本残留（顶部摘要行 :21/SC1 :123/状态表 :149 已随 737858a 修复） | 历史计划描述，权威 Goal/SC 行均已是 5 事件口径；随 metadata 维护顺手修正即可，非行为缺口 |

### Human Verification Required

无——上轮 2 项人工拍板（Q1/Q2）已闭环；本轮 fix 报告标注「requires human verification」的 2 项（CR-01 重试语义、WR-02 新门）已由真实 server 活体 UAT（05-UAT.md test 3/4，编排者按用户指令执行，status complete 5/5）+ 本轮 2 个集成活体测试双重闭环。本轮无新增 PRESENT_BEHAVIOR_UNVERIFIED truths：CR-01 变更契约有活体直证，WR-02 门有真实 server 四面实测，未变更面行为测试于复跑批次全绿。

### Gaps Summary

无 gap。14/14 must-haves 验证通过。变更面（CR-01 五挂点契约修订 / WR-01 已知限制文档 / WR-02 空白真名门 / 520ace4 断言对齐）完整三级验证 + 活体直证：5 挂点零短路残留、显式带名门保留、D-02 时序不变、34 条 `<> ?` UPDATE 语义不变、行为锁定测试随契约同步改写且 PASS、编译/vet 双证零可编译 UsernameChanged 引用。未变更面回归检查全过。工件 13/13、关键链接 8/8（人工复核）、禁令 8/8（1 条机制子句经 override 裁决、意图全成立）、需求 2/2 SATISFIED、零反模式新增、零债务标记。单元 15 包本轮独立复跑全绿 + 变更面 2 个集成活体 PASS；修复后全量复跑批次（service/handler/app）与真实 server UAT 5/5 在案。状态 passed——phase goal（5 事件冗余同步 + 全量验收回归）在 CR-01 修订后契约下达成。

---

_Verified: 2026-11-02T08:45:44Z_
_Verifier: Claude (gsd-verifier)_
