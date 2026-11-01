---
phase: 04-domains
plan: 03
subsystem: api
tags: [operation-log, audit, middleware, masking, async-worker, flush, pagination, gin]

# Dependency graph
requires:
  - phase: 04-domains
    plan: 01
    provides: 六域 DDL（含 sys_operation_log）+ perm 常量体系 + Dict/Config/Shift 路由（Oplog 挂载对象）
  - phase: 04-domains
    plan: 02
    provides: BodyLimitWithMB 路由级上限 + multipart 体积封顶先例 + 写路由挂载形态
  - phase: 02-rbac
    provides: auth.FromContext identity 注入（user_id/real_name 取值）+ newTestApp 集成测试基建 + resp/paging 横切件
provides:
  - Oplog 中间件（D-05 声明制参数化 module/operation）：body 回放（非 multipart）→ c.Next() → 七要素组装 → Enqueue
  - MaskSensitive 8 字段正则掩码（password/oldPassword/newPassword/secretKey/apiKey/token/accessToken/refreshToken，只掩字符串值，RE2 线性时间）+ 掩码先于 60 000 字节截断的顺序锁定
  - OpLogWriter：chan 512 异步 + 单 worker 批量落库（批 64/500ms ticker）+ Enqueue select-default 永不阻塞 + Flush 导出（drain+等待落库，测试免 sleep）
  - resp.Fail c.Error(err) 透出机制（gin ErrorTypePrivate，响应体字节不变）；Oplog 取 c.Errors 末条按 rune 截 500
  - paging.ParseWithDefaults(page,size,defPage,defSize) 特例分页（既有 Parse 1/15 行为零变化，纯委托）
  - Olog 查询域四层：GET /api/v1/logs 三 like（module/operation/real_name）+ create_time DESC 分页
  - 41 条写路由声明制挂载（既有五域 20 + auth 4 含 login 未登录 NULL identity 分支 + 本阶段六域 17）；RequirePerm 收口 67
  - App.Stop 链 Flush 次序：srv.Shutdown 之后 sqlDB.Close 之前（D-08——先排空在途 HTTP 再 drain 日志最后关 DB）
affects: [04-04-fault]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 27000    # chars/4 over the realized diff (15 files, 1457 insertions / 61 deletions)
  tasks: 2
  commits: 2
commits: 2
plan_head_before: eb5b8dc5b0e21c65825a34a4630e8f1bce26e5a9
plan_head_after: 82cf1d5cb21d958c674f80c3995c0dcb4e95ae29

# Tech tracking
tech-stack:
  patterns:
    - "声明制审计中间件：路由行内 Oplog(w, Module, operation) 参数化，module 取源 Controller 简名（DictTypeController 形态）保持存量日志 like 查询连续性"
    - "异步日志三段式：Enqueue(select-default 丢弃)→worker 攒批(CreateInBatches 64/500ms)→Flush(drain+sync)——丢弃是设计而非缺陷（D-07），内存上界 512×60KB≈30MB"
    - "错误透出零侵入模式：resp.Fail 写 c.Error(err)（gin 私有 errors 链），Oplog 消费方截 500——横切件不改既有响应契约"
    - "分页特例参数化：ParseWithDefaults 抽取钳位逻辑参数化默认值，Parse 纯委托——新增特例零回归面"
    - "回放式 body 读取：io.ReadAll 后 io.NopCloser(bytes.NewReader) 重置 Request.Body，handler 无感二次读取"

key-files:
  created:
    - internal/middleware/op_log.go
    - internal/middleware/op_log_test.go
    - internal/model/po/operation_log.go
    - internal/model/dto/operation_log.go
    - internal/repository/operation_log.go
    - internal/service/operation_log.go
    - internal/handler/operation_log.go
    - internal/handler/olog_integration_test.go
  modified:
    - pkg/resp/resp.go
    - pkg/resp/resp_test.go
    - pkg/paging/paging.go
    - pkg/paging/paging_test.go
    - internal/app/app.go
    - internal/app/router.go
    - internal/handler/user_integration_test.go

# Verification evidence
verification:
  - gate: "go build ./... && go vet ./... && go test ./internal/middleware/ ./pkg/resp/ ./pkg/paging/ -count=1"
    result: PASS（掩码八字段逐字段/截断顺序/Enqueue 不阻塞/Flush drain/Parse 零回归）
  - gate: "grep -c middleware.Oplog( router.go == 41 && grep -c middleware.RequirePerm( == 67"
    result: PASS（41/67 精确命中；推导式 20+4+17 与 66+1）
  - gate: "TZ=UTC go test -tags=integration -run TestOpLog ./internal/handler/"
    result: PASS 5/5（WriteRecorded 七要素/MaskedParams 八字段/FailedRequest status=0+error_msg/LoginNullIdentity/ListPagination 1/10+like+倒序）
  - gate: "TZ=UTC go test -tags=integration -count=1 ./...（全量回归）"
    result: PASS 15 包全绿（handler 886s——Oplog 叠加后既有路由含 shutdown 链全部不回归）

# Deviations from plan
deviations: []

# Interruption record
notes: >-
  Task 2 执行中途遭 API 429 限额中断（Task 1 已独立提交 5080fd9，Task 2 半成品留工作区）。
  恢复走 quota-interruption playbook：盘点（git log/status + grep 计数定位断点深浅：41/67 已命中）→
  build/vet 权威校验（gopls 中断期误报不采信）→ 四 gate 复验全绿 → 按 task 边界原子提交。
  executor 侧无代码返工，工作区产物质量完整。

requirements:
  - OLOG-01 SATISFIED（写操作自动落库七要素 + body 回放不破坏业务 200——TestOpLogWriteRecorded 双断言）
  - OLOG-02 SATISFIED（8 字段掩码逐字段/失败请求 status=0+error_msg 截 500/chan 满丢弃与落库失败静默——单测+集成双证）
  - OLOG-03 SATISFIED（1/10 特例分页 + 三 like 模糊 + create_time 倒序——TestOpLogListPagination）
