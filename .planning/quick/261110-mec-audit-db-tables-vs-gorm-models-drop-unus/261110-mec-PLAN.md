---
phase: quick-261110-mec
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - .planning/quick/261110-mec-audit-db-tables-vs-gorm-models-drop-unus/261110-mec-SUMMARY.md
autonomous: true
requirements: []
estimate:
  tokens: 30000
  raw_tokens: 30000
  tasks: 3
  confidence: low

must_haves:
  truths:
    - "库 sys-iam-go 剩余表恰为代码使用的 17 张（16 张 GORM PO TableName + casbin_rule），无一张在用表被误删（D-01）"
    - "sys_role_dept 与 casbin_rule 已按 scripts/ddl/ 现成幂等 DDL 补建（各 8 列），SELECT COUNT(*) 探活通过（D-03）"
    - "95 张未使用表（基线 2026-11-10：ACT_*/FLW_*/mes_*/wms_*/twin_*/wcs_*/llm_*/codegen_rule/craft_outbox_event/mq_dead_letter/sys_tenant）全部 DROP，库中 FK 残留为 0"
    - "8 张表的 tenant_id 列保留未动（D-02：sys_user/sys_dept/sys_user_role/sys_operation_log/sys_shift/sys_fault_info/sys_file/sys_file_category）"
    - "17 张表列数与基线逐一相符（sys_user 23、sys_role 13 … casbin_rule 8）——DROP 操作零附带损伤"
    - "go build ./... 通过；git 工作树除 .planning/ 外零变更（纯 DDL 操作，零代码文件改动）"
  artifacts:
    - "数据库终态：sys-iam-go 库 17 张表（information_schema 可查，基线 110→补 2→删 95→17）"
    - "/tmp/drop_unused_261110.sql 动态生成的 DROP 清单（临时文件，不提交，不含凭证）"
  key_links:
    - "安全链四步在同一次执行中串联成立：information_schema 动态生成（NOT IN 17 张保留清单）→ 保留清单零交集门 → FK 关闭执行 → 17 表计数终态门"
---

<objective>
将 configs/config.yaml 指向的本机 MySQL 库 sys-iam-go 清理到与本项目代码（GORM model + casbin adapter）完全一致的 17 表终态：补建代码需要但缺失的 2 张表（sys_role_dept、casbin_rule），删除 95 张未被本项目使用的表（用户已确认直接 DROP、不备份），8 张表的 tenant_id 列按用户决策保留不动。

Purpose: 本库从源系统库共库漂移而来，混入 Flowable/MES/WMS/Twin/WCS/LLM 等无关域 95 张表；缺 sys_role_dept 会让 dataScope 档位 4、角色删除第四删、角色改名监听器运行时报错，缺 casbin_rule 会让服务启动时 authz enforcer 构造直接失败。清理后库与代码一一对应，消除误用与启动故障面。
Output: 17 表终态的 sys-iam-go 库 + 验证记录 SUMMARY；零代码文件改动。
</objective>

<execution_context>
@~/.claude/gsd-core/workflows/execute-plan.md
@~/.claude/gsd-core/templates/summary.md
</execution_context>

<context>
本任务无 ROADMAP 需求归属（quick 任务）。无需读取任何 prior SUMMARY。

## 用户已锁定决策（AskUserQuestion 确认，不可复议）

- **D-01**：95 张未使用表直接 DROP，不备份。
- **D-02**：8 张表 DB 多出的 tenant_id 列（nullable bigint）保留不动——nullable 不影响 GORM 运行。
- **D-03**：缺失的 sys_role_dept、casbin_rule 执行 scripts/ddl/ 下现成幂等 DDL 补建。

## 连接协议（密码纪律：MYSQL_PWD 环境变量传递，绝不进命令行参数、不进任何提交文件）

DSN 唯一事实源是 configs/config.yaml 的 mysql.dsn。每次执行先解析：

```bash
DSN=$(sed -nE 's/^[[:space:]]*dsn:[[:space:]]*"([^"]+)".*/\1/p' configs/config.yaml | head -1)
DB_USER=${DSN%%:*}
DB_PASS=${DSN#*:}; DB_PASS=${DB_PASS%%@tcp(*}
```

此后所有 mysql 调用统一形态（下文以 `$M` 代指 mysql 命令本体）：

```bash
M() { MYSQL_PWD="$DB_PASS" mysql -h 127.0.0.1 -u "$DB_USER" -N -B --database=sys-iam-go "$@"; }
```

密码含 @ 字符——凡手写连接串必错，必须按上述解析。schema 名 sys-iam-go 含连字符，SQL 内引用一律用 DATABASE()（由 --database 推导），不硬编码库名字符串。/tmp 下生成的 SQL 文件只含 DDL 语句，不含凭证。

## 保留清单（17 张，planner 已对 internal/model/po/*.go 的 TableName() 与 enforcer.go 逐一复核，2026-11-10）

sys_api_key, sys_config, sys_dept, sys_dict_data, sys_dict_type, sys_fault_info, sys_file, sys_file_category, sys_menu, sys_operation_log, sys_role, sys_role_dept, sys_role_menu, sys_shift, sys_user, sys_user_role, casbin_rule

其中 casbin_rule 来自 internal/authz/enforcer.go:43 的 NewAdapterByDBUseTableName(db, "", "casbin_rule")，且 TurnOffAutoMigrate 已关闭 adapter 自动建表——表必须由 DDL 显式建。代码 .Table()/.Raw()/.Exec() 全量 grep 零额外表引用。

## 库现状基线（编排器 2026-11-10 实测，执行时以实时查询为准）

- 110 张现存表：15 张在用 + 95 张未用；2 张在用表缺失（sys_role_dept、casbin_rule）。
- 未用 95 张分布：ACT_*(30)、FLW_*(6)、mes_*(8)、wms_*(33)、twin_*(8)、wcs_*(2)、llm_*(3)、codegen_rule、craft_outout_event 类（craft_outbox_event 4176 行）、mq_dead_letter(17 行)、sys_tenant(1 行)。
- 44 个 FK 全部位于 ACT_*/FLW_* 内部，sys_* 无任何 FK——删 Flowable 族必须 SET FOREIGN_KEY_CHECKS=0。
- scripts/ddl/sys_role_dept.sql（8 列）与 scripts/ddl/casbin_rule.sql（8 列）均为 DROP IF EXISTS + CREATE 幂等脚本、不含外键；两表现在不存在，执行即纯新增。
- 15 张现存表逐列比对结论：7 张与代码完全一致，8 张 DB 多 tenant_id 列（D-02 保留）。基线列数：sys_user 23、sys_role 13、sys_menu 18、sys_dept 14、sys_user_role 9、sys_role_menu 8、sys_dict_type 11、sys_dict_data 13、sys_operation_log 15、sys_config 11、sys_api_key 13、sys_file 18、sys_file_category 16、sys_fault_info 18、sys_shift 14。

## go test 判断（planner 裁定：不跑）

集成测试经 internal/testutil/{mysql,redis}.go 走 testcontainers 独立临时容器，单元测试走 miniredis；全库 _test.go 唯一 config.yaml 引用在 config_test.go 且用 t.TempDir——没有任何测试连 127.0.0.1:3306 的开发库。DDL 变更对测试零影响，go build ./... 即代码侧门（本任务零代码改动，build 门是防意外的护栏）。
</context>

<tasks>

<task type="auto">
  <name>Task 1: 补建缺失表 sys_role_dept 与 casbin_rule（D-03）</name>
  <files>scripts/ddl/sys_role_dept.sql, scripts/ddl/casbin_rule.sql（仅执行，不修改）</files>
  <precondition>MySQL 已在 127.0.0.1:3306 运行且 configs/config.yaml 的 DSN 可连（库 sys-iam-go 存在）。</precondition>
  <action>按 D-03 补建两张缺失表。先跑前置断言：经 information_schema 查 table_name IN ('sys_role_dept','casbin_rule') 计数必须为 0（两表均缺失）——若任一已存在（库状态漂移），立即 HALT 并在输出中报告存在的表及其行数，等待人工裁决后再继续，因为 DDL 脚本含 DROP TABLE IF EXISTS，对已有数据的表执行会毁数据（sys_role_dept 的档位 4 集合数据不可自动重建）。断言通过后，用 <context> 连接协议解析凭证，以文件重定向方式分别执行 scripts/ddl/sys_role_dept.sql 与 scripts/ddl/casbin_rule.sql（每脚本一次独立 mysql 调用，勿拼接）。执行后跑下方验证门。</action>
  <verify>
    <automated>DSN=$(sed -nE 's/^[[:space:]]*dsn:[[:space:]]*"([^"]+)".*/\1/p' configs/config.yaml | head -1); U=${DSN%%:*}; P=${DSN#*:}; P=${P%%@tcp(*; M() { MYSQL_PWD="$P" mysql -h 127.0.0.1 -u "$U" -N -B --database=sys-iam-go "$@"; }; test "$($M -e 'SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ("sys_role_dept","casbin_rule")')" = 2 && test "$($M -e 'SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name="sys_role_dept"')" = 8 && test "$($M -e 'SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name="casbin_rule"')" = 8 && test "$($M -e 'SELECT COUNT(*) FROM sys_role_dept')" = 0 && test "$($M -e 'SELECT COUNT(*) FROM casbin_rule')" = 0</automated>
  </verify>
  <done>sys_role_dept 与 casbin_rule 均存在（information_schema 计 2）、各 8 列、SELECT COUNT(*) 探活返回 0；若遇表已存在的漂移则已 HALT 上报而未执行 DROP。</done>
</task>

<task type="auto">
  <name>Task 2: 动态生成并执行未使用表 DROP（D-01，保留清单硬门）</name>
  <files>/tmp/drop_unused_261110.sql（临时生成，不提交）</files>
  <precondition>Task 1 已完成（17 张在用表全部在库）。</precondition>
  <action>按 D-01 删除全部未使用表，清单必须由 information_schema 动态生成——严禁手写表名清单（防拼写错误、防库状态漂移）。三步走：(1) 生成：用 <context> 连接协议，以单引号包裹的 -e 参数执行 SELECT CONCAT("DROP TABLE IF EXISTS `", table_name, "`;") FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name NOT IN (17 张保留清单，见 <context>，逐字抄录)，-N -B 输出重定向到 /tmp/drop_unused_261110.sql（SQL 内字符串与反引号均在 bash 单引号内，不会被 shell 解释）。(2) 双安全门，任一不过立即 HALT：门一"零交集"——grep -cE 反引号包裹的 17 名交替匹配（\`(sys_api_key|sys_config|sys_dept|sys_dict_data|sys_dict_type|sys_fault_info|sys_file|sys_file_category|sys_menu|sys_operation_log|sys_role|sys_role_dept|sys_role_menu|sys_shift|sys_user|sys_user_role|casbin_rule)\`）作用于生成的 SQL 文件，必须为 0（反引号定界保证 sys_file 不会误配 sys_file_category 行或反之）；门二"计数不变式"——当前库总表数减去文件内 grep -c '^DROP' 行数必须恰等于 17（基线 112-95=17；不变式与库漂移无关地恒成立）。(3) 执行：单次 mysql 会话内按序拼接 SET FOREIGN_KEY_CHECKS=0; + 生成文件内容 + SET FOREIGN_KEY_CHECKS=1; 经 stdin 管道执行（FK 全在 ACT_*/FLW_* 内部，关闭检查后任意顺序可删；SET 为会话级，会话结束自动复原）。执行后跑下方验证门（17 表终态 + 17 张在用表全在 + FK 清零）。</action>
  <verify>
    <automated>DSN=$(sed -nE 's/^[[:space:]]*dsn:[[:space:]]*"([^"]+)".*/\1/p' configs/config.yaml | head -1); U=${DSN%%:*}; P=${DSN#*:}; P=${P%%@tcp(*; M() { MYSQL_PWD="$P" mysql -h 127.0.0.1 -u "$U" -N -B --database=sys-iam-go "$@"; }; test "$($M -e 'SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()')" = 17 && test "$($M -e 'SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ("sys_api_key","sys_config","sys_dept","sys_dict_data","sys_dict_type","sys_fault_info","sys_file","sys_file_category","sys_menu","sys_operation_log","sys_role","sys_role_dept","sys_role_menu","sys_shift","sys_user","sys_user_role","casbin_rule")')" = 17 && test "$($M -e 'SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema=DATABASE()')" = 0</automated>
  </verify>
  <done>库中表总数恰 17；17 张保留清单表逐一在库；information_schema.referential_constraints 计 0（44 个 ACT_*/FLW_* FK 随表清除，无残留孤儿约束）；生成文件经两道安全门后执行，未手写任何 DROP 表名。</done>
</task>

<task type="auto">
  <name>Task 3: 17 表终态验证门（逐表探活 + 列数全量比对 + tenant_id 保留确认 D-02 + go build）</name>
  <files>.planning/quick/261110-mec-audit-db-tables-vs-gorm-models-drop-unus/261110-mec-SUMMARY.md</files>
  <precondition>Task 2 已完成（库已到 17 表终态）。</precondition>
  <action>跑四道终验门并撰写 SUMMARY。(1) 逐表探活：对 17 张表逐一执行 SELECT COUNT(*)（bash for 循环遍历保留清单，任一非 0 退出即失败）——证明每张表存在且可查询，覆盖"表存在 + 一条 SELECT 探活"要求。(2) 列数全量比对：单条 SQL 以 17 行 UNION ALL 内联基线列数（sys_api_key 13、sys_config 11、sys_dept 14、sys_dict_data 13、sys_dict_type 11、sys_fault_info 18、sys_file 18、sys_file_category 16、sys_menu 18、sys_operation_log 15、sys_role 13、sys_role_dept 8、sys_role_menu 8、sys_shift 14、sys_user 23、sys_user_role 9、casbin_rule 8）与 information_schema.columns 实测逐行相等，输出必须为 17——证明 DROP 零附带损伤、补建列集精确。(3) D-02 确认：tenant_id 列在 8 张表（sys_user/sys_dept/sys_user_role/sys_operation_log/sys_shift/sys_fault_info/sys_file/sys_file_category）上 DISTINCT 计数必须为 8——保留未动的直接证据。(4) go build ./... 通过。SUMMARY 记录：实际 DROP 行数（对照基线 95）、四道门输出、go test 不跑的裁定依据（见 <context>）。</action>
  <verify>
    <automated>DSN=$(sed -nE 's/^[[:space:]]*dsn:[[:space:]]*"([^"]+)".*/\1/p' configs/config.yaml | head -1); U=${DSN%%:*}; P=${DSN#*:}; P=${P%%@tcp(*; M() { MYSQL_PWD="$P" mysql -h 127.0.0.1 -u "$U" -N -B --database=sys-iam-go "$@"; }; for T in sys_api_key sys_config sys_dept sys_dict_data sys_dict_type sys_fault_info sys_file sys_file_category sys_menu sys_operation_log sys_role sys_role_dept sys_role_menu sys_shift sys_user sys_user_role casbin_rule; do $M -e "SELECT COUNT(*) FROM \`$T\`" >/dev/null || exit 1; done && test "$($M -e 'SELECT COUNT(*) FROM (SELECT "sys_api_key" t,13 c UNION ALL SELECT "sys_config",11 UNION ALL SELECT "sys_dept",14 UNION ALL SELECT "sys_dict_data",13 UNION ALL SELECT "sys_dict_type",11 UNION ALL SELECT "sys_fault_info",18 UNION ALL SELECT "sys_file",18 UNION ALL SELECT "sys_file_category",16 UNION ALL SELECT "sys_menu",18 UNION ALL SELECT "sys_operation_log",15 UNION ALL SELECT "sys_role",13 UNION ALL SELECT "sys_role_dept",8 UNION ALL SELECT "sys_role_menu",8 UNION ALL SELECT "sys_shift",14 UNION ALL SELECT "sys_user",23 UNION ALL SELECT "sys_user_role",9 UNION ALL SELECT "casbin_rule",8) e WHERE e.c=(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=e.t)')" = 17 && test "$($M -e 'SELECT COUNT(DISTINCT table_name) FROM information_schema.columns WHERE table_schema=DATABASE() AND column_name="tenant_id" AND table_name IN ("sys_user","sys_dept","sys_user_role","sys_operation_log","sys_shift","sys_fault_info","sys_file","sys_file_category")')" = 8 && go build ./...</automated>
  </verify>
  <done>17 张表全部 SELECT 探活通过；17 张表列数与基线逐一相等；tenant_id 在 8 张表上完好保留（D-02 落实）；go build ./... 通过；SUMMARY 已落盘记录验证输出与 go test 不跑的裁定。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 执行者→本机 MySQL | 以 configs/config.yaml DSN 凭证对本机库执行破坏性 DDL（DROP），这是本任务唯一且最大的破坏面 |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-quick-01 | Tampering | DROP 误删在用表 / 打错库 | critical | mitigate | 保留清单唯一事实源（planner 已对 po/*.go TableName() 复核）；清单由 information_schema NOT IN 动态生成杜绝手写拼写错误；零交集门 + 计数不变式门（总表数-DROP 行数=17）双门拦截后才允许执行；schema 经 --database + DATABASE() 推导，不硬编码库名 |
| T-quick-02 | Information Disclosure | MySQL 密码经命令行参数泄露（ps 可见）或落入提交文件 | high | mitigate | 密码仅经 MYSQL_PWD 环境变量传递；运行时从 configs/config.yaml 解析（%%/# 参数展开，不 echo）；PLAN/SUMMARY//tmp 生成文件零凭证；验证命令同构 |
| T-quick-03 | Denial of Service | 幂等 DDL 的 DROP IF EXISTS 语义在未来重跑时清空已有数据（sys_role_dept 档位 4 数据不可自动重建；casbin_rule 可由 boot 对账重建） | medium | mitigate | Task 1 前置存在性断言：两表任一已存在即 HALT 上报，绝不盲目执行 DROP+CREATE；SUMMARY 明示两脚本不可在含数据状态下重跑 |
| T-quick-SC | Tampering | 包安装 | high | accept | 本任务零包安装、零依赖引入、零代码改动，无供应链面；若执行中意外需要安装任何依赖，停止并上报而非继续 |
</threat_model>

<verification>
1. Task 3 的四合一终验门全绿（17 表逐表探活 + 列数 17/17 相符 + tenant_id 8/8 保留 + go build）。
2. Task 2 验证门全绿：总表数 17、17 张在用表全在、referential_constraints 0。
3. `git status` 显示除 .planning/ 外工作树零变更（configs/config.yaml 原有的未提交改动保持原样，不得纳入本任务提交）；无任何代码文件被触碰。
4. SUMMARY 记录实际 DROP 表数（对照基线 95）与库漂移情况（如有）。
</verification>

<success_criteria>
- 库 sys-iam-go 到达 17 表终态：16 张 GORM PO 表 + casbin_rule，逐表 SELECT 探活通过。
- sys_role_dept、casbin_rule 按 scripts/ddl/ 现成 DDL 补建（D-03），各 8 列。
- 95 张未使用表全部删除且零误删（零交集门 + 计数不变式门 + 17 表终态三重证据，D-01）。
- 8 张表 tenant_id 列保留未动（D-02），15 张原存表列数与基线逐一相等——清理零附带损伤。
- go build ./... 通过；零代码文件改动；go test 按裁定不跑（依据记录于 SUMMARY）。
</success_criteria>

<output>
Create `.planning/quick/261110-mec-audit-db-tables-vs-gorm-models-drop-unus/261110-mec-SUMMARY.md` when done
</output>
