---
phase: 04-domains
plan: 01
subsystem: api
tags: [gin, gorm, dict, config, shift, crud, mysql, testcontainers, rbac]

# Dependency graph
requires:
  - phase: 02-rbac
    provides: 五域模板全套（po/dto/repo/service/handler + newTestApp 双容器集成测试基建 + RequirePerm 中间件 + errcode 分段）
  - phase: 03-authz
    provides: Casbin enforcer 装配与 boot 对账（新路由逐条 RequirePerm 的授权底座）
provides:
  - 六域 8 表 DDL 复刻（scripts/ddl/ 共 17 文件，testutil glob 自动加载零改动）
  - 六域 perm 常量一次加齐（internal/authz/perms.go 58 个 system: 常量；shift/fault 发明串边界注释）
  - Dict 域全链（类型 CRUD + uk_dict_type 1062 翻译 + 数据 CRUD 类型校验/dict_type_name 冗余写 + dict_type_id 可选过滤）
  - Config 域全链（CRUD + GET /configs/key/:key 字符串本体查询 + uk_config_key 1062 翻译 + FindByKey）
  - Shift 域全链（CRUD + 全量数组直出无分页 envelope + HH:MM:SS 字符串直传）
  - 21 条新路由逐条 RequirePerm（router.go 收口 57）
affects: [04-02-file, 04-03-olog, 04-04-fault]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 27000    # chars/4 over the realized diff (28 files, 2425 insertions)
  tasks: 2
  commits: 3
commits: 3
plan_head_before: 229647f900ddbb07d1888ab7d72879464a174c04
plan_head_after: a940b9a3a34a880e96004f486afd2e12b0c93831

# Tech tracking
tech-stack:
  added: []   # 零新增依赖（T-4-SC：标准库 + 既有 go.mod，excelize/minio-go 属 04-02/04-04）
  patterns:
    - "分页闭包无 dataScope 变体：repo List 闭包照 user.go 形态但闭包内无 filter.Apply（D-03 零应用拍板）"
    - "静态段与 :id 参数段共存先例复用：configs/key/:key 先注册静态段（gin 静态优先，与 menus/tree 同构）"
    - "可选过滤分支模板：repo List 闭包内 dictTypeId 非零时 Where（DICT-03），handler 读 query 参数非法 400"

key-files:
  created:
    - scripts/ddl/sys_dict_type.sql
    - scripts/ddl/sys_dict_data.sql
    - scripts/ddl/sys_config.sql
    - scripts/ddl/sys_shift.sql
    - scripts/ddl/sys_file.sql
    - scripts/ddl/sys_file_category.sql
    - scripts/ddl/sys_operation_log.sql
    - scripts/ddl/sys_fault_info.sql
    - internal/model/po/dict.go
    - internal/model/po/sys_config.go
    - internal/model/po/shift.go
    - internal/model/dto/dict.go
    - internal/model/dto/config.go
    - internal/model/dto/shift.go
    - internal/repository/dict.go
    - internal/repository/sys_config.go
    - internal/repository/shift.go
    - internal/service/dict.go
    - internal/service/config.go
    - internal/service/shift.go
    - internal/handler/dict_type.go
    - internal/handler/dict_data.go
    - internal/handler/config.go
    - internal/handler/shift.go
    - internal/handler/dict_config_shift_integration_test.go
  modified:
    - internal/authz/perms.go
    - internal/app/app.go
    - internal/app/router.go

key-decisions:
  - "1062 翻译统一 400/10400 域文案（「字典类型已存在」/「配置键已存在」）——Phase 2 role.go 代码实证惯例，非 409（plan 预判修正已内建）"
  - "dict-datas 五路由复用 PermDictType* 常量（源 dictData 与 dictType 共用 dict:* 权限，不新增权限档）"
  - "Shift list 全量数组直出保留（SHFT-01 源行为直译是验收点）——handler/repo 注释显式标注勿统一为分页"
  - "shift/fault perm 串为发明串（sys_menu 零行实证）——v1 仅 super_admin 可用，Phase 3 auth/apiKey Q2 边界先例"
  - "perms.go 六域常量一次加齐（后续 plan 不再碰本文件，消除 D-02 串行链上的跨 plan 文件冲突）"

patterns-established:
  - "轻 CRUD 域复制模板定版：PO(BeforeCreate 同构)→repo(Patch CR-01 Count 二次确认)→service(nil-skip 白名单+落库回读)→handler(parseID+薄层三段式)，04-02/04-03/04-04 按此套用"
  - "字符串直传列模式：MySQL TIME 列以 *string 承载，无解析层无格式校验，非法串由 DB 报错走既有翻译路径"

requirements-completed: [DICT-01, DICT-02, DICT-03, CONF-01, SHFT-01, SHFT-02]

coverage:
  - id: D1
    description: 字典类型 CRUD 全链：uk_dict_type 重复插入 400/10400「字典类型已存在」、update 零值保护、删除后 404
    requirement: DICT-01
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestDictTypeCRUD
        status: pass
    human_judgment: false
  - id: D2
    description: 字典数据 CRUD：类型存在性前置校验（404/10300「字典类型不存在: {id}」源消息原文）+ dict_type_name 服务端冗余写（resp 与 DB 行双断言）
    requirement: DICT-02
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestDictDataWithTypeCheck
        status: pass
    human_judgment: false
  - id: D3
    description: GET /api/v1/dict-datas?dict_type_id= 仅返回该类型数据（envelope items/total/page/size 齐），不传返回全量分页
    requirement: DICT-03
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestDictDataFilterByType
        status: pass
    human_judgment: false
  - id: D4
    description: 系统配置 CRUD：GET /configs/key/{key} data 为 configValue 字符串本体、uk_config_key 重复 400/10400「配置键已存在」、key 未命中 404
    requirement: CONF-01
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestConfigCRUDAndKey
        status: pass
    human_judgment: false
  - id: D5
    description: GET /api/v1/shifts 响应 data 为 JSON 数组直出（无 items/total 分页键——SHFT-01 源行为直译）
    requirement: SHFT-01
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestShiftFullListNoPaging
        status: pass
    human_judgment: false
  - id: D6
    description: 排班 CRUD 五路由可用，start_time/end_time 以 HH:MM:SS 字符串直传（无解析层/格式校验）
    requirement: SHFT-02
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestShiftFullListNoPaging
        status: pass
    human_judgment: false
  - id: D7
    description: 本 plan 21 条新路由全部逐条 RequirePerm（D-04）；无 token 401/无 perm 403/super_admin 200 三面；结构 gate 计数 57
    requirement: DICT-01
    verification:
      - kind: integration
        ref: internal/handler/dict_config_shift_integration_test.go#TestDictRoutePerm
        status: pass
      - kind: other
        ref: "grep -c middleware.RequirePerm internal/app/router.go == 57"
        status: pass
    human_judgment: false

# Metrics
duration: 37min
completed: 2026-11-01
status: complete
---

# Phase 4 Plan 01: 轻三域 tracer——Dict 全链端到端 + Config/Shift 模板复制 Summary

**Dict/Config/Shift 三域全链交付：8 表 DDL 复刻落目录、六域 perm 常量加齐至 58、21 条路由逐条 RequirePerm（收口 57）、uk 双唯一索引 1062 翻译域文案、Shift 全量无分页直译——六个集成测试全绿且 Phase 1-3 全量回归 13 包 ok**

## Performance

- **Duration:** 37 min
- **Started:** 2026-11-01T09:09:29Z
- **Completed:** 2026-11-01T09:46:30Z
- **Tasks:** 2（含无条件前置独立提交）
- **Files modified:** 28（25 新建 + 3 修改）

## Accomplishments
- 8 表 DDL 以本机 mysql-server SHOW CREATE TABLE 实证逐列复刻（uk_dict_type/uk_config_key 唯两索引、sys_file 非 sys_file_info、sys_operation_log 无四审计列、sys_fault_info bit_index default -1/type default 3 全部落位）——testutil glob 自动加载零改动
- 六域 perm 常量一次加齐（dict 5/config 5/log 1/file 7 实证串 + shift 5/fault 6 发明串边界注释），perms.go 后续 plan 零改动
- Dict 域 tracer 竖切全绿：1062→400/10400 域文案、类型存在性校验 404/10300 源消息原文、dict_type_name 服务端冗余写（resp+DB 双断言）、dict_type_id 可选过滤、perm 三面（401/403/200）
- Config 域 key 查询返回 configValue 字符串本体（非对象包装，源 getByKey 语义）；Shift 域全量数组直出（SHFT-01 直译，测试断言无 items/total 分页键）
- 无条件前置步骤独立提交先行（03-01 先例）：8 DDL + perms 零行为耦合基线先固化（e191c48），单提交体量减半

## Task Commits

Each task was committed atomically:

1. **前置步骤: 8 表 DDL + 六域 perm 常量** - `e191c48` (feat)
2. **Task 1: Dict 域全链端到端 tracer** - `a60a292` (feat)
3. **Task 2: Config 域 + Shift 域模板复制** - `a940b9a` (feat)

**Plan metadata:** 见最终 docs commit（本文件 + STATE/ROADMAP）

## Files Created/Modified
- `scripts/ddl/sys_{dict_type,dict_data,config,shift,file,file_category,operation_log,fault_info}.sql` - 8 表复刻 DDL（头部注释含来源声明/uk 语义/glob 加载说明）
- `internal/model/po/dict.go` - DictTypePO（string 非指针核心列）+ DictDataPO（DictTypeID int64 非指针，业务必有类型）；BeforeCreate 与 menu.go 逐字同构
- `internal/model/po/sys_config.go`、`internal/model/po/shift.go` - Config/Shift PO（Shift 时间列 *string 直传）
- `internal/model/dto/dict.go`、`config.go`、`shift.go` - Create/Update 全指针 + Resp 全键无 omitempty + 手写 ToXxxResp
- `internal/repository/dict.go`、`sys_config.go`、`shift.go` - CRUD + 分页闭包（无 filter.Apply）+ FindByKey（config）/全量 List（shift，shift_code ASC）
- `internal/service/dict.go`、`config.go`、`shift.go` - 1062 翻译/类型校验/冗余写/key 读取/全量直出
- `internal/handler/dict_type.go`、`dict_data.go`、`config.go`、`shift.go` - 薄层三段式 + parseID + paging.Parse
- `internal/authz/perms.go` - 29→58 常量（六域加齐）
- `internal/app/app.go` - 四域 repo→svc→handler 装配链 + handlers 聚合四字段
- `internal/app/router.go` - 21 条新路由逐条 RequirePerm（configs/key/:key 静态段先于 :id）
- `internal/handler/dict_config_shift_integration_test.go` - 6 个集成测试（build tag integration，newTestApp 双容器）

## Decisions Made
- 1062 翻译用 400/10400（CodeInvalidParam）非 409——沿 Phase 2 role.go 代码实证惯例（plan 已预判此修正）
- DictTypePO.DictName/DictType 用 string 非指针（binding required 保证必有值）；DictDataPO.DictTypeID int64 非指针（业务上必有类型，create 前置校验）
- TestShiftFullListNoPaging 的无 envelope 断言用 NotContains `"items"`/`"total"` 于 data 原始 JSON + 数组反序列化成功双证（data 是数组时 map unmarshal 必然失败，rawMap 方案逻辑不成立）
- 集成测试响应 ID 字段用 ids.ID 类型承接（MarshalJSON 输出带引号字符串，UnmarshalJSON 双兼容）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] perms.go grep gate 计数偏差修正**
- **Found during:** 前置步骤提交前自检
- **Issue:** 新注释含 `system:shift*`/`system:fault*` 文本被 `grep -c "system:"` 计入（60≠58 gate）
- **Fix:** 注释措辞改为 `shift:*`/`fault:*` 前缀表述（语义不变），计数精确 58
- **Files modified:** internal/authz/perms.go
- **Verification:** grep -c "system:" == 58
- **Committed in:** e191c48

**2. [Rule 1 - Bug] 测试端 ids.ID 字符串序列化适配**
- **Found during:** Task 1 集成测试首跑
- **Issue:** ids.ID MarshalJSON 输出带引号字符串，测试结构体 int64 字段 unmarshal 失败（`cannot unmarshal string into Go struct field dictTypeRespBody.id`）
- **Fix:** 测试断言结构体 ID/DictTypeID 字段改用 ids.ID 类型（UnmarshalJSON 双兼容），int64ToStr 调用点补 int64() 转换
- **Files modified:** internal/handler/dict_config_shift_integration_test.go
- **Verification:** 四个 Dict 测试全绿
- **Committed in:** a60a292

**3. [Rule 1 - Bug] TestShiftFullListNoPaging envelope 断言逻辑修正**
- **Found during:** Task 2 集成测试首跑
- **Issue:** 对数组形态 data 做 map[string]json.RawMessage unmarshal 必然失败（原方案意图反向）
- **Fix:** 改为 data 原始 JSON NotContains items/total 键 + 数组反序列化成功双断言
- **Files modified:** internal/handler/dict_config_shift_integration_test.go
- **Verification:** TestShiftFullListNoPaging 绿
- **Committed in:** a940b9a

---

**Total deviations:** 3 auto-fixed（3× Rule 1 bug——均为测试侧断言形态与既有序列化契约的对齐，非实现缺陷）
**Impact on plan:** 实现代码零偏差；3 处均为测试断言与 ids.ID 契约/数据形态的对齐修正。无 scope creep。

## Issues Encountered
- handler/dict_type.go 首写漏 import pkg/resp（编译即捕获修复）
- service/dict.go 指针/值类型两处不匹配（DictName string vs DictTypeName *string）编译即捕获修复
- 全量集成回归超 600s 转 background 完成（exit 0，13 包 ok）——testcontainers 双容器 per-test 启动约 13s/测试是既有基建形态

## User Setup Required
None - no external service configuration required（零新增依赖、零新环境变量）。

## Next Phase Readiness
- 四域装配链与 handlers 聚合已扩至 10 域形态，04-02（File/FileCategory）按同构模板复制 + FileStorage 参数化 newTestApp
- perms.go 已含 file 域 7 串（upload/download 04-02 挂）与 fault 域 import 串（04-04 挂）——后续 plan 零常量改动
- 8 表 DDL 已全部落目录，04-02/03/04 的 testcontainers 自动加载即用
- Tracer 验证结论：Phase 2 五域模板对轻 CRUD 域可复制性成立（Dict 竖切全绿，Config/Shift 复制零模板阻力）

---
*Phase: 04-domains*
*Completed: 2026-11-01*

## Self-Check: PASSED

- 9 个关键文件存在性验证全 FOUND（含 8 DDL、四域代码、集成测试、SUMMARY 本体）
- 3 个任务提交（e191c48/a60a292/a940b9a）均为 HEAD 祖先
- 无意外文件删除（3 commits diff-filter=D 为空）；无未跟踪代码文件遗留
- 结构 gate 实测：DDL 17 / perms 58 / RequirePerm 57（三推导式全成立）
- 集成验证：6 个本 plan 测试全绿；全量回归 13 包 ok（exit 0）

