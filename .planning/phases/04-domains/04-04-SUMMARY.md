---
phase: 04-domains
plan: 04
subsystem: api
tags: [fault, crud, excel-import, excelize, upsert, transaction, dedup, multipart]

# Dependency graph
requires:
  - phase: 04-domains
    plan: 01
    provides: sys_fault_info DDL + PermFault 六常量（Add/Edit/Query/List/Remove/Import）+ 轻 CRUD 四层模板
  - phase: 04-domains
    plan: 02
    provides: BodyLimitWithMB 路由级上限 + multipart 上传/独立封顶先例 + doUpload 测试 helper
  - phase: 04-domains
    plan: 03
    provides: Oplog 声明制挂载（FaultInfoController module 名消费）+ RequirePerm 收口 67/Oplog 41 基线
  - phase: 02-rbac
    provides: newTestApp 集成测试基建 + repo.Patch/paging.Parse 模板 + SkipDefaultTransaction 显式事务纪律
provides:
  - Fault 域四层 CRUD（FAUL-01）：五路由 RequirePerm（67→72→73）；list 1/15 分页 + code idx_code 索引列 eq 直传过滤；bitIndex/type 服务端默认 -1/3（GORM nil 指针写 NULL 会绕过 DB DEFAULT——默认值语义由 service 层承担）
  - pkg/types/fault_type.go：INFO/WARNING/FAULT/UNKNOWN 枚举（源 FaultTypeEnum 保真）——name|int 双形态 JSON + ParseFaultType 中文/int 串解析（未知兜底 3 不报错，源 fromValue/fromInfo）+ FaultTypeName 反查
  - Excel 三键 upsert 导入全链（FAUL-02/D-15）：POST /api/v1/faults/import（BodyLimitWithMB(20)→RequirePerm(PermFaultImport)→Oplog(FaultInfoController, import)）→ 8 列契约严格解析（表头逐字全等 + 行数上限 5000 含表头前置）→ FindByThreeKeys 命中 Patch 复用原行 id/未命中 Create → 整批单事务任一行失败全批无痕回滚 → ImportResult{insertCount, updateCount}（FEATURES.md:205 契约）
  - FindByThreeKeys 三列 eq 直传形态 + Q2 照源裁定文档化：location 为 NULL 时 eq 判重恒不中、反复 INSERT 是源已接受既有行为（prohibition 禁改 IS NULL 双分支；TestFaultImportLocationNullNoDedup 断言形态锁定）
  - excelize v2.11.0 依赖落地（go.mod 直接依赖；纯 Go 无 CGO）
affects: [phase-05-verdict]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 17000    # chars/4 over the realized diff (13 files, 1399 insertions / 1 deletion)
  tasks: 2
  commits: 2
commits: 2
plan_head_before: 6aa5ac8d22a4c08eba1fb4629cc08954f9c27f10
plan_head_after: 5e7ca7148140e097eb8e777fc4363420c595e61b

# Tech tracking
tech-stack:
  added:
    - github.com/xuri/excelize/v2 v2.11.0（Excel 导入解析：OpenReader + GetRows("Sheet1")；纯 Go 无 CGO）
  patterns:
    - "导入防线顺序即解析顺序（D-15）：OpenReader→行数上限（GetRows 全量载内存后立即拒绝，含表头计）→表头 8 列逐字全等→逐行解析——超限/错头文件不进用户数据解析"
    - "三键 upsert 事务内 tx 变体 repo：NewFaultInfoRepo(tx) 复用同一查询逻辑，整批 db.Transaction 包裹（SkipDefaultTransaction 下唯一事务边界，照 RoleService 持 db 形态）"
    - "GetRows 行尾空单元格跳过防御：cell(idx) 带 len(row)>idx 边界取值，行长度不一致是常态非异常（04-RESEARCH 重点 6）；测试构造器 nil=不写单元格顺带覆盖短行分支"
    - "源语义缺陷保真模式：location NULL 判重恒不中以「断言形态锁定 + 双处注释引用 Q2 裁定」固化——TestFaultImportLocationNullNoDedup 变红即语义被误改的哨兵"
    - "构造等价回测（D-14 fallback）：契约即测试构造器——buildFaultXlsx 以 8 列契约表头逐字构造，真数据冒烟（yf175a 77 行按列头名映射转换）为非验收增强"

key-files:
  created:
    - internal/model/po/fault_info.go
    - internal/model/dto/fault_info.go
    - internal/repository/fault_info.go
    - internal/service/fault.go
    - internal/service/fault_import.go
    - internal/handler/fault.go
    - internal/handler/fault_integration_test.go
    - pkg/types/fault_type.go
    - pkg/types/fault_type_test.go
  modified:
    - internal/app/app.go
    - internal/app/router.go
    - go.mod
    - go.sum

# Verification evidence
verification:
  - gate: "go build ./... && go vet ./... && go test ./pkg/types/ -count=1"
    result: PASS（Task 1：枚举 name|int/中文/int 串双形态/未知兜底 3 全断言绿）
  - gate: "grep -c middleware.RequirePerm( == 72 && grep -c middleware.Oplog( == 44（Task 1 后）"
    result: PASS（实测 72/44；推导式 67+5 CRUD、41+3 写路由——plan verify 段「04-03 收口 40/43」为笔误，04-03 实际收口 41（20+4+17），acceptance_criteria 的 44 为准，见 deviations）
  - gate: "TZ=UTC go test -tags=integration -run TestFaultCRUD ./internal/handler/"
    result: PASS（服务端默认 -1/3 resp+直查 DB 双断言/零值保护/code eq 过滤/perm 三断言）
  - gate: "go build ./... && go vet ./...（Task 2）"
    result: PASS（excelize API 使用无误——OpenReader/GetRows/CoordinatesToCellName/NewFile/Write）
  - gate: "grep -c faults/import == 1 && grep -c maxImportRows >= 2 && RequirePerm == 73 && Oplog == 45（Task 2 后）"
    result: PASS（1/5 refs/73/45 全命中）
  - gate: "TZ=UTC go test -tags=integration -run TestFaultImport ./internal/handler/"
    result: PASS 6/6（Upsert id 复用直证/LocationNullNoDedup 两行锁定/TxRollback DB 行数不变/HeaderReject 三变体/RowLimit 5001 拒+5000 放边界/Yf175aData 77 行真数据实跑非 skip）
  - gate: "TZ=UTC go test -tags=integration -count=1 ./...（Phase 1-4 全量回归，phase gate）"
    result: PASS 15 包全绿（handler 988s；EXIT=0 零 FAIL——16 项需求验收面最终回归）

# Deviations from plan
deviations:
  - "**[笔误修正] Task 1 verify gate「Oplog 计数 43（04-03 收口 40 + fault 3）」的 40 为笔误**——04-03 实际收口 Oplog=41（20+4+17，已实测 router.go）。按 orchestrator 裁定以实测基线递推：Task 1 后 Oplog=44（41+3，acceptance_criteria 正确）、Task 2 后 45（+import 1）；RequirePerm 演进 67→72→73 无笔误。执行期实测与修正值全一致，零返工"

# Interruption record
notes: >-
  无中断。两 task 按 boundary 原子提交（db4bed0/5e7ca71），全量回归后台跑毕（15 包
  EXIT=0）后才收 Task 2 尾。

requirements:
  - FAUL-01 SATISFIED（五路由 CRUD + list 1/15 分页 + code eq 过滤 + 服务端默认 bitIndex=-1/type=3 落库直证 + perm/Oplog 齐挂——TestFaultCRUD 全绿）
  - FAUL-02 SATISFIED（8 列契约上传→双前置防线→三键 upsert（UPDATE 复用 id/INSERT/空 name skip）→整批单事务回滚直证→ImportResult 契约；location NULL 判重源语义锁定文档化——TestFaultImport 六组全绿 + 全量回归 15 包）

## Self-Check: PASSED

- 10 个 created/关键文件全部存在（FOUND）
- db4bed0/5e7ca71 均为 HEAD 祖先；git rev-list --count 6aa5ac8..HEAD = 2 与 frontmatter commits: 2 实测一致

