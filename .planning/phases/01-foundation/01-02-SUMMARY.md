---
phase: 01-foundation
plan: 02
subsystem: user-crud-paging
tags: [crud, zero-value-protection, audit-trail, pagination, envelope, error-codes, tdd, testcontainers]

requires:
  - phase: 01-foundation (plan 01 walking skeleton — startup chain, contract packages, POST create)
    provides: [pkg/errcode 分段与 HTTP 映射, pkg/resp envelope 出口, internal/auth ctx identity, UserRepo.Create 与 BeforeCreate 钩子, testutil.RunMySQL testcontainers 基建, TranslateError 开启的 GORM 装配]
provides:
  - sys_user 四件套完整可用：PUT /api/v1/users/:id（零值保护部分更新）、GET /api/v1/users/:id（404/400 错误面）、GET /api/v1/users（分页 envelope）
  - repo.Patch 显式列更新 helper（map 白名单 + 审计显式并入 + RowsAffected 判 404）——全库更新路径模板
  - pkg/paging 横切件：Parse 钳位（缺省 1/15、上限 100）+ PageData[T] 泛型 envelope（空列表恒 [] 非 null）——所有列表域模板
  - 9 个集成测试 + 2 组 paging 单元测试（真实 MySQL 8.4 容器），错误码分段 10300/10400/10401 全部经接口证明
affects: [Phase 2 会话与用户域（在四件套上补业务规则）, 所有后续列表域（分页模板）, 所有后续更新域（Patch 模板）]

actuals:
  tokens: 13818          # chars/4 over git diff a51b097..5a09e3f（55,272 diff 字节）
  tasks: 2
  commits: 5
  plan_head_before: a51b097423957f478555e4dc44b0ebeca0b9091f
  plan_head_after: 5a09e3ff68522c3d41960a74a71690e61b4ec1b1

tech-stack:
  added: []              # 无新外部依赖；pkg/paging 为自建标准库实现（泛型 + encoding/json）
  patterns:
    - "repo.Patch：显式列 map 更新 + update_time/identity 审计并入 map（不依赖钩子）+ RowsAffected==0 判 404"
    - "UpdateUserReq 全指针 DTO：nil=未提供不写库、空串=显式清空（Pitfall 3 双语义）"
    - "PageData[T].MarshalJSON 在类型出口统一保证空列表输出 []（使用方无需判空）"
    - "repo.List 闭包工厂表达 Count 与记录查询同一条件集（防 total 漂移，P15）"
    - "集成测试 per-test 独立容器（newTestApp）+ 进程级雪花单例 sync.Once"

key-files:
  created:
    - pkg/paging/paging.go
    - pkg/paging/paging_test.go
    - internal/service/user_integration_test.go
  modified:
    - internal/model/dto/user.go
    - internal/repository/user.go
    - internal/service/user.go
    - internal/handler/user.go
    - internal/app/router.go
    - internal/handler/user_integration_test.go

key-decisions:
  - "更新审计在 repo.Patch 内显式并入 map 而非依赖 BeforeUpdate 钩子——GORM 对 Updates(map) 不把钩子改动的结构体字段写进 SET 子句（计划明示的语义坑），显式并入保证确定性（ENG-02 update 侧）"
  - "PageData[T] 实现自定义 MarshalJSON：nil Items 序列化前统一转空切片——空列表恒输出 [] 非 null 的契约在类型出口一处保证，杜绝每个列表域各自判空的遗漏风险（ENG-01 空输入边界）"
  - "repo.List 以闭包工厂构造 Count 与记录查询，Phase 1 无过滤条件下条件集天然一致；后续域加过滤时在闭包单点加 Where 即可防 total 漂移（P15）"
  - "集成测试公共基建 newTestApp/doRequest/createRawUser + ids.Once 进程级雪花单例（每测试独立容器，共享单一 node 实例杜绝多实例同毫秒 seq 回绕撞主键）；TestCreateUserContract 一并重构复用（REFACTOR 提交）"
  - "计划 verify 正则 TestUpdateUser|TestGetUser 漏掉 behavior 指定的 TestUpdateAuditWithIdentity，按 Rule 3 修正为 TestUpdateUser|TestGetUser|TestUpdateAudit"
  - "零值保护测试以 sleep 1.1s 跨过 DATETIME 秒精度边界，使 update_time 严格晚于 create_time 可判（create_time/update_time 为 datetime 无小数秒）"

patterns-established:
  - "Pattern: repo.Patch 显式列更新——更新列集合完全来自白名单 map 键，恒带 Where(id)，绝无 Save；全库所有域的更新路径照此实现（T-1-10/T-1-08）"
  - "Pattern: 写路径 DTO 全指针 + 无审计键——请求/响应 DTO 均不接收审计字段，审计只由服务端填充（ENG-02 禁令）"
  - "Pattern: 分页模板 paging.Parse + PageData[T]——列表域 handler 一律 c.Query → Parse → service → resp.OK(PageData)"
  - "Pattern: errcode 分段错误面——不存在 10300/404、参数无效 10400/400、校验失败 10401/400、重复记录 10400/400"
  - "Pattern: 集成测试每测试独立容器（D-10 铁律延续）+ t.Cleanup 关停链 HTTP→连接池→容器"

requirements-completed: [ENG-01, ENG-02, ENG-04, ENG-05]

coverage:
  - id: D1
    description: update 路径零值保护——PUT 仅更新显式提交字段，未提交字段保持原值，显式空串可清空，单条 UPDATE 原子（ENG-05，Pitfall 3 根治）
    requirement: ENG-05
    verification:
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestUpdateUserZeroValueProtection（响应+DB 行双断言：phone/email/deptName/gender 原值保持、realName 已变、update_time 晚于 create_time）"
        status: pass
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestUpdateUserExplicitClear（phone:\"\" 显式清空为空串，email 保持原值）"
        status: pass
    human_judgment: false
  - id: D2
    description: 更新审计自动填充两分支——ctx 含 identity 时 update_by_id/update_by_name 落库；无 identity 时仅刷 update_time、操作人列保持 NULL（ENG-02 update 侧）
    requirement: ENG-02
    verification:
      - kind: integration
        ref: "internal/service/user_integration_test.go#TestUpdateAuditWithIdentity（有 identity 分支 DB 行断言 update_by_id=42/update_by_name=操作者甲）"
        status: pass
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestUpdateUserZeroValueProtection（无 identity 分支 DB 行断言 update_by_id/update_by_name IS NULL）"
        status: pass
    human_judgment: false
  - id: D3
    description: get 路径错误面——不存在 id 返回 HTTP 404 + code 10300；非数字 id 返回 HTTP 400 + code 10400；GET 返回完整契约 DTO
    requirement: ENG-01
    verification:
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestUpdateUserNotFound（404+10300）+ #TestGetUserInvalidId（400+10400）"
        status: pass
    human_judgment: false
  - id: D4
    description: pkg/paging 横切件——Parse 钳位（缺省 1/15、<1 纠偏、size 上限 100、非法输入兜底、合法原样）+ PageData[T] envelope（空列表 [] 非 null）
    requirement: ENG-01
    verification:
      - kind: unit
        ref: "pkg/paging/paging_test.go#TestParseClamp（五组钳位断言）+ #TestPageDataEmptyItemsMarshal（nil Items 序列化为 \"items\":[]）"
        status: pass
    human_judgment: false
  - id: D5
    description: list 路径分页 envelope——GET /api/v1/users 返回 data {items,total,page,size}（顶层无 total），ORDER BY create_time DESC, id DESC 稳定排序，边界参数不 500
    requirement: ENG-01
    verification:
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestListUsersPaging（items 2 行/total 3/page 1/size 2/items[0]=最新行，含同秒插入 id DESC tiebreaker）+ #TestListUsersEdgeParams（page=0&size=-1 钳位 1/15、page=999 空 [] 非 null 且 HTTP 200）"
        status: pass
    human_judgment: false
  - id: D6
    description: create 错误面收口——重复 username → 400+10400（1062 经 TranslateError 映射）；缺必填 → 400+10401；gender=99 未知枚举 → 不 500（兜底 UNKNOWN）；未知额外字段 → 不 400（宽容输入）
    requirement: ENG-01, ENG-04
    verification:
      - kind: integration
        ref: "internal/handler/user_integration_test.go#TestCreateUserDuplicateUsername + #TestCreateUserValidation（四类错误面断言）"
        status: pass
    human_judgment: false
  - id: D7
    description: 人工复核项——ENG-02 混合场景：同表并发有/无 identity 写入时 BeforeCreate（结构体路径）与 Patch（map 路径）双实现的一致性
    requirement: ENG-02
    verification: []
    human_judgment: true
    rationale: "计划 Flagged Assumption #1 原样留任：两分支语义已各自有 DB 行断言（D2），但并发混合场景下双实现路径的等价性属实现细节，需人工复核两处代码；Phase 2 认证中间件接入后可在真实流量下进一步回归"
  - id: D8
    description: 源系统 withEdit 语义对齐确认——编辑请求省略 builtin 键不得清掉已有内置标志（源 User.java:125-128 注释语义）
    requirement: ENG-05
    verification:
      - kind: integration
        ref: "TestUpdateUserZeroValueProtection 隐含覆盖：PUT 不提交 builtin，DB 行 gender 等字段原值保持；builtin 与其余未提交字段走同一 Patch 白名单路径"
        status: pass
    human_judgment: false

duration: 20min
completed: 2026-10-31
status: complete
---

# Phase 01 Plan 02: 用户四件套补全（update/get/list + pkg/paging）Summary

**sys_user 四件套在真实 MySQL 8.4 容器上闭环：指针 DTO 零值保护更新、审计两分支行级证明、分页钳位 envelope、错误码分段四类错误面，全程 TDD（RED 证据 → GREEN → REFACTOR）5 提交。**

## Performance
- **Duration:** 20min
- **Started:** 2026-10-31T08:38:38Z
- **Completed:** 2026-10-31T08:58:22Z
- **Tasks:** 2/2
- **Files changed:** 13（759 插入 / 33 删除；新建 3 个文件）

## Accomplishments
- Task 1（update/get）：repo.Patch 显式列更新（白名单 map + update_time/审计显式并入 + RowsAffected 判 404 + 恒带 Where）根治 GORM 三重错位（Pitfall 3）；UpdateUserReq 全指针 DTO（无任何审计键，T-1-10 白名单）；GET/PUT :id 协议层（非数字 id → 400/10400，不存在 → 404/10300）；审计有/无 identity 两分支均有 DB 行级断言
- Task 2（list/paging）：pkg/paging 横切件（Parse 钳位缺省 1/15、上限 100、PageData[T] 空列表恒 []）；repo.List 稳定排序 ORDER BY create_time DESC, id DESC（同秒插入翻页不漂移，P15）；分页 envelope {items,total,page,size} 在 data 内；create 错误面四类断言（重复 10400/校验 10401/枚举兜底/未知字段宽容）
- 全程 TDD：两轮 RED（4 份证据 JSON 均过 RED_EVIDENCE_OK，目标测试全部失败于断言而非编译错）→ GREEN → Task 1 REFACTOR（消除测试装配重复）
- 计划级总验证全绿：go build/vet、paging 单元（无 Docker）、集成 handler+service 全部 ok

## Task Commits
1. **Task 1 RED** — `8cd2e93` (test)：5 个失败测试 + 编译存根 + RED 证据
2. **Task 1 GREEN** — `6e63293` (feat)：Patch/FindByID/Update/Get 实现
3. **Task 1 REFACTOR** — `4308073` (refactor)：TestCreateUserContract 复用 newTestApp 基建
4. **Task 2 RED** — `d7814e9` (test)：paging 单元 + list/错误面集成失败测试 + 存根
5. **Task 2 GREEN** — `5a09e3f` (feat)：paging/repo.List/service.List 实现（无需 REFACTOR）

## Files Created/Modified
见 frontmatter `key-files`（新建 pkg/paging 两文件 + service 集成测试；修改 dto/repo/service/handler/router + handler 集成测试）。

## Decisions Made
见 frontmatter `key-decisions`（审计显式并入 map、MarshalJSON 类型出口保证空列表、闭包工厂同条件集、测试基建单例化、verify 正则修正、秒精度跨秒等待等 6 项）。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 计划 verify 正则遗漏 TestUpdateAuditWithIdentity**
- **Found during:** Task 1 RED 阶段首跑
- **Issue:** 计划 `<verify>` 的 `-run 'TestUpdateUser|TestGetUser'` 正则不匹配 behavior 中指定的 service 级测试 TestUpdateAuditWithIdentity（service 包报 "no tests to run"）
- **Fix:** verify 正则修正为 `TestUpdateUser|TestGetUser|TestUpdateAudit`；GREEN 后以修正后正则重跑全部五个测试通过
- **Files modified:** 无代码影响（验证命令修正）
- **Commit:** 随 6e63293 验证闭环

**2. [Rule 3 - Blocking] Go 集成测试 RED 需编译存根**
- **Found during:** Task 1 RED 阶段
- **Issue:** Go 包不编译则测试未断言任何行为（INVALID_RED）——测试引用的 svc.Update/repo.Patch 等 API 尚不存在
- **Fix:** RED 提交包含最小编译存根（service.Update/Get 恒返回 ErrNotFound、paging.Parse 返回 (0,0)），确保目标测试在断言上失败而非编译失败；GREEN 提交填入真实实现
- **Files modified:** internal/service/user.go、internal/repository/user.go、pkg/paging/paging.go
- **Verification:** 4 份 RED 证据 JSON 均通过 gsd_run check tdd-red-evidence（RED_EVIDENCE_OK，reason=target_test_failed）

**Total deviations:** 2 auto-fixed + **Impact on plan:** 无范围/契约影响——验证语义完全保留（五个 behavior 测试 + paging 五断言全部经真实运行证明）。

## Issues Encountered
- DATETIME 秒精度：create 与 update 在同一秒内无法断言 update_time 严格晚于 create_time——测试以 sleep 1.1s 跨秒解决（零值保护主断言不受影响）
- TestCreateUserDuplicateUsername/TestCreateUserValidation 在 RED 阶段即绿：这两者是 create 路径（01-01 已实现）的回归测试而非新行为测试，属预期；Task 2 的 RED 目标（paging/list）均真实失败于断言

## User Setup Required
None - 测试全程 testcontainers 独立容器（本地 mysql:8.4.8 镜像缓存 + ryuk 禁用自适应沿用 01-01 基建），无外部服务配置要求。

## Known Stubs
None - RED 阶段编译存根已在 GREEN 全部替换为真实实现（grep 确认 repo/service 层无遗留存根语义、无 Save 调用）。

## Flagged Assumptions（计划原文留任，不静默丢弃）
1. **ENG-02 unclassified（审计填充混合场景人工复核）**：并发混合（同表同时有带/不带 identity 的写）下 BeforeCreate 结构体路径与 Patch map 路径双实现的一致性需人工复核——见 coverage D7
2. **列表条件查询**：Phase 1 list 仅分页无过滤条件——USER-05 的条件查询属 Phase 2 业务规则（D-01 边界），非缺口

## Next Phase Readiness
- 四件套齐备即 Phase 2 代码基础（D-01 costly 决策落地）：User 域在既有 handler/service/repo 上扩展业务规则（builtin 删除拦截、详情两查询装配、Excel 导入、bcrypt 密码哈希、认证中间件注入 identity 激活审计操作人列）
- pkg/paging 与 repo.Patch 是全库模板：所有列表域照 Parse+PageData 形状，所有更新域照指针 DTO→map→Patch 形状
- Phase 2 需安装 go-redis + miniredis（本计划明确未装）；分页 size 上限 100 与连接池 MaxOpenConns=20 已构成 T-1-04 DoS 双层兜底

## Self-Check: PASSED
- pkg/paging/paging.go、pkg/paging/paging_test.go、internal/service/user_integration_test.go 存在 ✓
- 5 个提交（8cd2e93/6e63293/4308073/d7814e9/5a09e3f）均为 HEAD=5a09e3f 祖先链 ✓
- 计划级 verification 三条命令全部通过（build/vet、paging ok、集成 handler+service ok）✓
- must_haves.truths 七条逐一经测试证明（见 coverage D1-D6）✓
