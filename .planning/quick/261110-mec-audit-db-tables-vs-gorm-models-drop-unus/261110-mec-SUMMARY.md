---
phase: quick-261110-mec
plan: 01
subsystem: database
tags: [mysql, ddl, gorm, schema-alignment, drop-table, casbin]

# Dependency graph
requires: []
provides:
  - "sys-iam-go 库 17 表终态（16 张 GORM PO 表 + casbin_rule），与代码一一对应"
  - "补建 sys_role_dept（dataScope 档位 4 存储）与 casbin_rule（authz enforcer 持久层），各 8 列"
affects: [dataScope, authz, phase-03-casbin, 运维/schema 管理]

# Actuals — 本任务零代码文件改动，realized diff 仅本 SUMMARY
actuals:
  tokens: 900
  tasks: 3
  commits: 0

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "DDL 安全链四步：information_schema 动态生成（NOT IN 保留清单）→ 零交集门 → FK 关闭执行 → 17 表计数终态门"
    - "密码纪律：MYSQL_PWD 环境变量传递（/tmp/gsd-mec-m.sh 运行时解析 config.yaml），零命令行参数/零提交文件泄露面"

key-files:
  created:
    - ".planning/quick/261110-mec-audit-db-tables-vs-gorm-models-drop-unus/261110-mec-SUMMARY.md"
  modified: []

key-decisions:
  - "D-01 用户锁定：95 张未使用表直接 DROP 不备份（实际 94 张，见偏差说明）"
  - "D-02 用户锁定：8 张表 DB 多出的 tenant_id 列保留不动"
  - "D-03：scripts/ddl/ 现成幂等 DDL 补建 sys_role_dept 与 casbin_rule"
  - "执行偏差：PLAN 基线列数 sys_file/sys_menu 各差 1，以代码 PO 为事实源修正后零损伤门 17/17 通过"

patterns-established:
  - "DDL 双安全门：反引号定界零交集 grep + 计数不变式（总表数−DROP 行数=17），任一不过即 HALT"
  - "前置存在性断言：幂等 DDL（DROP IF EXISTS）执行前必须断言目标表不存在，防漂移毁数据"

requirements-completed: []

coverage:
  - id: D1
    description: "库 sys-iam-go 清理至 17 表终态：补建 sys_role_dept/casbin_rule（各 8 列），删除 94 张未使用表，FK 残留 0"
    verification:
      - kind: other
        ref: "mysql information_schema：总表数=17、保留清单 IN 计数=17、referential_constraints=0、两新表列数=8 且 COUNT(*) 探活=0"
        status: pass
    human_judgment: false
  - id: D2
    description: "DROP 零附带损伤：17 表逐表 SELECT 探活 + 列数与代码 PO 逐一相符（列名级 DB-vs-PO 比对 17/17）"
    verification:
      - kind: other
        ref: "bash for 循环 17 表探活全通过 + 修正版 UNION ALL 列数门=17 + tenant_id 恰在 D-02 8 表（全库 8/8）"
        status: pass
    human_judgment: false
  - id: D3
    description: "零代码文件改动：go build ./... 通过，git 工作树除 .planning/ 外零变更"
    verification:
      - kind: other
        ref: "go build ./... exit 0 + git status --short 仅 .planning/quick/261110-mec-* 未跟踪目录"
        status: pass
    human_judgment: false

# Metrics
duration: 11min
completed: 2026-11-10
status: complete
---

# Quick Task 261110-mec: 数据库表清理对齐 GORM Models Summary

**库 sys-iam-go 从 109 表（混入 Flowable/MES/WMS/Twin/WCS/LLM 域）清理至与代码完全一致的 17 表终态：补建 sys_role_dept + casbin_rule（各 8 列），DROP 94 张未使用表（FK 残留 0），tenant_id 8/8 保留，零代码文件改动**

## Performance

- **Duration:** 11 min
- **Started:** 2026-11-10T08:23:16Z
- **Completed:** 2026-11-10T08:34:24Z
- **Tasks:** 3
- **Files modified:** 0（代码零改动；数据库 DDL 变更不产生仓库文件；本 SUMMARY 为唯一新增文档）

## Accomplishments

- **Task 1 补建缺失表（D-03）**：前置存在性断言通过（两表计数=0，无漂移）后，分别以独立 mysql 调用执行 `scripts/ddl/sys_role_dept.sql` 与 `scripts/ddl/casbin_rule.sql`。验证门 5/5：两表在库（information_schema 计 2）、各 8 列、SELECT COUNT(*) 探活均返回 0。
- **Task 2 DROP 94 张未使用表（D-01）**：清单由 information_schema 动态生成（`NOT IN` 17 张保留清单，SQL 文件落 /tmp/drop_unused_261110.sql，零凭证零手写表名）。双安全门通过后单会话执行 `SET FOREIGN_KEY_CHECKS=0; …DROP…; SET FOREIGN_KEY_CHECKS=1;`。验证门 3/3：总表数=17、17 张保留表逐一在库、referential_constraints=0（44 个 ACT_*/FLW_* FK 随表清除无孤儿约束）。
- **Task 3 终验四门**：门一 17 表逐表 SELECT 探活全过（在用数据完好：sys_menu 220 行、sys_operation_log 6037 行、sys_role_menu 563 行等）；门二列数全量比对通过（修正基线，见偏差）；门三 tenant_id DISTINCT=8 且全库恰为 D-02 8 张（sys_user/sys_dept/sys_user_role/sys_operation_log/sys_shift/sys_fault_info/sys_file/sys_file_category，不多不少）；门四 `go build ./...` 通过。

## Task Commits

**零 commit——本任务为纯数据库 DDL 操作，三个任务均未触碰任何仓库文件**（DDL 脚本仅执行未修改；DROP 清单在 /tmp 不提交；按任务约束"无代码变更则无 commit"，文档工件由编排器统一提交）。

## 数据库终态证据

- **总表数：109（执行前实测）→ 补建 2 → 删 94 → 17**（PLAN 基线行文 110/95 与实测 109/94 差 1，系基线备注自身算术 slip：其分布明细求和即 94，见偏差说明 2）
- **17 表终态清单**（与保留清单逐字一致）：casbin_rule, sys_api_key, sys_config, sys_dept, sys_dict_data, sys_dict_type, sys_fault_info, sys_file, sys_file_category, sys_menu, sys_operation_log, sys_role, sys_role_dept, sys_role_menu, sys_shift, sys_user, sys_user_role
- **DROP 清单分布**（动态生成，94 行）：wms_* 33、ACT_* 30、twin_* 8、mes_* 8、FLW_* 6、llm_* 3、wcs_* 2、codegen_rule、craft_outbox_event、mq_dead_letter、sys_tenant 各 1
- **在用数据零损伤**：15 张原存表行数实测（sys_dept 3、sys_dict_data 13、sys_dict_type 1、sys_fault_info 196、sys_file_category 2、sys_menu 220、sys_operation_log 6037、sys_role 3、sys_role_menu 563、sys_shift 7、sys_user 4、sys_user_role 4，其余 0）
- **生成文件**：/tmp/drop_unused_261110.sql（动态 DROP 清单）、/tmp/gen_drop_261110.sql（生成查询）、/tmp/gsd-mec-m.sh（连接封装，运行时解析 DSN）——三者均零凭证、不提交

## Decisions Made

- D-01/D-02/D-03 均为用户 AskUserQuestion 锁定决策，照执行未复议。
- 密码纪律全程落实：密码仅经 MYSQL_PWD 环境变量传递（连接封装脚本运行时从 configs/config.yaml 解析，密码含 @ 字符故绝不手写连接串），未出现在任何命令行参数、输出、或落盘文件；schema 名经 `--database=sys-iam-go` + SQL 内 `DATABASE()` 推导，零硬编码库名。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] PLAN 基线列数 sys_file/sys_menu 各差 1，以代码 PO 为事实源修正**
- **Found during:** Task 3（门二列数全量比对返回 15 非 17）
- **Issue:** PLAN 基线记 sys_file=18、sys_menu=18；实测 sys_file=17、sys_menu=19。经列名级核对：FileInfoPO 注释与结构均明示 16 列（BaseWithOperator 7 + 自有 9），DB 17 = 16 + tenant_id（D-02 保留）；MenuPO 注释明示 19 列（BaseWithOperator 7 + 自有 12），DB 19 与代码完全一致。PLAN 两个数字各差 1 且恰好反向抵消，故其自身求和校验未发现。
- **Fix:** 以代码 PO（internal/model/po/*.go 结构体 + base.go BaseWithOperator 7 列 + relation.go/operation_log.go 刻意不内嵌操作人列的注释实证）为事实源修正基线，重跑修正版 17 行 UNION ALL 门 → 输出 17，PASS。另做加强验证：全库含 tenant_id 的表恰为 D-02 8 张（8/8，不多不少）。
- **Files modified:** 无（纯验证基线修正，未改任何代码/DDL）
- **Verification:** 修正版门二 17/17 匹配；15 张原存表列数在 DROP 前后物理不变（DROP 他表不可能改动保留表列，差异在 Task 2 执行前即存在于 PLAN 文档）

**2. [观测 - 非缺陷] 实际 DROP 94 张而非基线行文的 95 张**
- **Found during:** Task 2（动态生成 DROP 行数=94，计数不变式 111−94=17 通过）
- **Issue:** PLAN 基线行文"110 张现存表 / 95 张未用"，实测执行时为 109 张 / 94 张。PLAN 基线自身分布明细求和即 94（30+6+8+33+8+2+3+4 单表），行文的 95/110 为算术 slip。另一可能解释是基线采集后有一张表已被移除（无法回溯判定，前者与明细自洽故采信前者）。
- **Fix:** 无需修复——清单由 information_schema 动态生成，天然适配库漂移；计数不变式门（总表数−DROP 行数=17）与库漂移无关恒成立，实测通过。
- **Files modified:** 无

---

**Total deviations:** 2（1 个 Rule 1 基线修正 + 1 个基线计数观测；均零代码影响）
**Impact on plan:** 两项偏差均为 PLAN 文档数字勘误性质，数据库终态与 must_haves 全部 truth 语义一致（"库中剩余表恰为代码使用的 17 张"逐字成立）。已登记 broken-windows 台账（kind=deviation）。

## Issues Encountered

- Bash 工具 shell 内联函数定义偶发失效（首次前置断言调用 `$M` 展开为空）——改用 /tmp/gsd-mec-m.sh 连接封装脚本（运行时解析 DSN、零凭证）后全程稳定。
- go build 首次运行触发模块下载（go.sum 固定版本，无新增依赖），下载完成后构建通过。

## Go test 不跑的裁定依据（照 PLAN context 记录）

集成测试经 internal/testutil/{mysql,redis}.go 走 testcontainers 独立临时容器，单元测试走 miniredis；全库 _test.go 唯一 config.yaml 引用在 config_test.go 且用 t.TempDir——没有任何测试连 127.0.0.1:3306 的开发库。DDL 变更对测试零影响，`go build ./...` 即代码侧门（本任务零代码改动，build 门为防意外护栏，实测通过）。

## 安全警告（T-quick-03 缓解记录）

`scripts/ddl/sys_role_dept.sql` 与 `scripts/ddl/casbin_rule.sql` 含 `DROP TABLE IF EXISTS`——**不可在两表已含数据的状态下重跑**（sys_role_dept 档位 4 集合数据不可自动重建；casbin_rule 可由 boot 对账重建但策略仍会丢失）。本次执行前已断言两表不存在（计数=0）；未来重跑前必须重做同一断言。

## User Setup Required

None - 无外部服务配置要求（数据库为本机已运行的 MySQL 8.4.6，连接凭证沿用 configs/config.yaml）。

## Next Phase Readiness

- 库与代码一一对应：dataScope 档位 4（sys_role_dept）、角色删除/改名监听器、authz enforcer 启动对账（casbin_rule）的运行时故障面已消除。
- 无阻塞项。若服务启动，boot 对账将把 sys_user_role/sys_role_menu 关系全量导入 casbin g/p 策略（ReplaceAll 幂等），casbin_rule 空表即预期初态。

---
*Quick task: 261110-mec*
*Completed: 2026-11-10*
