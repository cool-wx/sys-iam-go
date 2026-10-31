---
phase: 01-foundation
reviewed: 2026-10-31T09:31:46Z
depth: standard
files_reviewed: 39
files_reviewed_list:
  - cmd/server/main.go
  - configs/config.yaml
  - deploy/sys-iam-go.service
  - .gitignore
  - go.mod
  - internal/app/app.go
  - internal/app/router.go
  - internal/app/shutdown_integration_test.go
  - internal/auth/identity.go
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/handler/user.go
  - internal/handler/user_integration_test.go
  - internal/health/health.go
  - internal/middleware/access_log.go
  - internal/middleware/recovery.go
  - internal/middleware/requestid.go
  - internal/model/dto/user.go
  - internal/model/po/base.go
  - internal/model/po/user.go
  - internal/repository/user.go
  - internal/service/user.go
  - internal/service/user_integration_test.go
  - internal/testutil/mysql.go
  - Makefile
  - pkg/errcode/errcode.go
  - pkg/errcode/errcode_test.go
  - pkg/ids/id.go
  - pkg/ids/id_test.go
  - pkg/paging/paging.go
  - pkg/paging/paging_test.go
  - pkg/resp/resp.go
  - pkg/resp/resp_test.go
  - pkg/types/datetime.go
  - pkg/types/datetime_test.go
  - pkg/types/enum.go
  - pkg/types/enum_test.go
  - scripts/ddl/sys_user.sql
  - scripts/test-graceful-shutdown.sh
findings:
  critical: 1
  warning: 4
  info: 8
  total: 13
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-10-31T09:31:46Z
**Depth:** standard
**Files Reviewed:** 39
**Status:** issues_found

## Summary

Phase 1（工程骨架与横切地基）整体质量较高：装配链清晰、PO 全指针原生类型规避 Valuer/Scanner 陷阱、审计字段服务端填充（ENG-02）、零值保护 Patch 显式列白名单（ENG-05）、R envelope 三键恒在、雪花 ID 字符串化双形态兼容、健康端点无内部细节泄漏、优雅关停双重证明齐备。`go build ./...`、`go vet ./...`、`go test ./...` 全部通过。

但对抗性审查发现 1 个 BLOCKER 级正确性缺陷：`repo.Patch` 将 `RowsAffected==0` 一律判定为"记录不存在"，而 go-sql-driver 默认 `clientFoundRows=false`（affected rows = 实际变更行数）且 `update_time` 为秒精度 `datetime` —— 同一秒内对同一用户重复提交相同内容（双击保存、幂等重试）会对**已存在**的用户返回 404。另有 4 个 WARNING：环境变量覆盖对 snake_case 键静默失效、HTTP 层缺传输超时与请求体上限、分页 page 溢出静默错页、连接池参数无校验（0 值 = 无限制，恰好架空 T-1-04 声明的连接池兜底）。

关键判定均经源码级验证：koanf env/v2 源码证实 TransformFunc 收到完整变量名；GORM `clause/limit.go` 证实负/零 Offset 被静默省略；go-sql-driver 证实 clientFoundRows 默认关闭。

范围说明：`go.sum` 为校验和锁定文件，按锁定文件规则排除。无 `<structural_findings>`、无外部审查证据输入。已知且按计划接受的边界（不列为 finding，需后续阶段闭环）：password 明文入库属 D-01 锁定的 Phase 2 边界；全部路由白名单直通属 D-02 临时态（Phase 2 收紧，见 IN-08）。

## Critical Issues

### CR-01: 同秒零变更更新被误判 404 —— Patch 以 RowsAffected 判定存在性

**File:** `internal/repository/user.go:48-54`（联动 `scripts/ddl/sys_user.sql:27`、`internal/service/user.go:98-101`）

**Issue:** `Patch` 中 `res.RowsAffected == 0 → errcode.ErrNotFound`。三个事实叠加使该判定在真实场景下对**已存在行**失效：

1. go-sql-driver/mysql 默认 `clientFoundRows=false`，`UPDATE` 返回的是**实际变更行数**而非匹配行数（DSN 与源码 `dsn.go:497` 均未开启该标志）；
2. `update_time` 列为 `datetime`（秒精度，无小数位），`time.Now()` 写入时被截断到秒；
3. 当请求 payload 与库内现值完全相同（幂等重试、双击保存、`PUT {}` 空 body）时，SET 子句所有值与现值相等 → 0 行变更。

**复现序列**：`PUT /api/v1/users/{id}` body `{"realName":"x"}` 在同一秒内连续两次 → 第二次 `real_name` 已为 "x"、`update_time` 截断后同秒 → affected=0 → HTTP 404 + `code 10300 "数据不存在"`，而用户实际存在。前端会据此误判记录被删。副作用还包括第二次请求的 `update_by_id/update_by_name` 未写入。集成测试未覆盖此场景（`TestUpdateUserNotFound` 只测了真不存在的 id）。

**Fix:**

```go
res := r.db.WithContext(ctx).Model(&po.UserPO{}).Where("id = ?", id).Updates(cols)
if res.Error != nil {
    return res.Error
}
if res.RowsAffected == 0 {
    // 0 变更 ≠ 不存在：同秒重复更新（值全相同）也会命中此处
    var n int64
    if err := r.db.WithContext(ctx).Model(&po.UserPO{}).
        Where("id = ?", id).Count(&n).Error; err != nil {
        return err
    }
    if n == 0 {
        return errcode.ErrNotFound
    }
    return nil // 行存在，仅本轮无值变化
}
return nil
```

不建议改用 DSN `clientFoundRows=true`（会同时改变 Create 等所有语句的 affected 语义）。补集成测试：同秒内两次相同 PUT，第二次应 200。

## Warnings

### WR-01: IAM_ 环境变量覆盖对 snake_case 键静默失效（三步转换链把下划线全转点）

**File:** `internal/config/config.go:74-80`（宣称处 `configs/config.yaml:2`）

**Issue:** TransformFunc 对完整变量名执行 `ToLower → 全部下划线转点`：`IAM_MYSQL_MAX_OPEN_CONNS` → 键 `mysql.max.open.conns`，而 YAML 键为 `mysql.max_open_conns`，两者永不匹配 → 覆盖静默丢失（无报错）。已对照 koanf env/v2 v2.0.2 源码证实：TransformFunc 收到的是**含前缀的完整变量名**，provider 不做任何键预处理。受影响键：`mysql.max_open_conns`、`mysql.max_idle_conns`、`mysql.conn_max_lifetime`（仅单词叶键 `server.port/mysql.dsn/snowflake.node/log.level` 生效）。config.yaml 第 2 行宣称"全部键可被 IAM_ 前缀环境变量覆盖"为假；测试只覆盖了两个单词键（`config_test.go:22-39`），未暴露缺陷。工控机现场若按文档用环境变量调连接池/连接寿命，会拿到默认值且无任何日志提示。

**Fix:** 对多段 snake_case 键建立显式映射表（或把 YAML 键改为单段词/驼峰并同步改文件与测试）：

```go
var envKeyAliases = map[string]string{
    "IAM_MYSQL_MAX_OPEN_CONNS":    "mysql.max_open_conns",
    "IAM_MYSQL_MAX_IDLE_CONNS":    "mysql.max_idle_conns",
    "IAM_MYSQL_CONN_MAX_LIFETIME": "mysql.conn_max_lifetime",
}
TransformFunc: func(s, v string) (string, any) {
    if mapped, ok := envKeyAliases[s]; ok {
        return mapped, v
    }
    return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "IAM_")), "_", "."), v
}
```

并补测试：`t.Setenv("IAM_MYSQL_MAX_OPEN_CONNS", "50")` 必须落到 `cfg.MySQL.MaxOpenConns`。

### WR-02: http.Server 无传输超时、请求体无大小上限（slowloris / 大 payload DoS 面）

**File:** `internal/app/app.go:71-74`（绑定入口 `internal/handler/user.go:29,48`）

**Issue:** `http.Server` 仅设了 Addr/Handler：无 `ReadHeaderTimeout`/`ReadTimeout`/`IdleTimeout` → 慢连接（slowloris）可长期占用连接；请求体无 `http.MaxBytesReader` → `ShouldBindJSON` 会把任意大小 body 读入内存。项目自身威胁清单 T-1-04 明确声明 DoS 防护关切（分页已钳位），但传输层与 body 尺寸两处未设防。工控机内存有限，大 body 可致 OOM。

**Fix:**

```go
srv: &http.Server{
    Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
    Handler:           engine,
    ReadHeaderTimeout: 10 * time.Second,
    IdleTimeout:       120 * time.Second,
},
```

并在绑定前统一包一层（可做成中间件）：

```go
const maxBodyBytes = 1 << 20 // 1MB，按最大契约字段裕量定
c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
```

超限时 MaxBytesReader 返回 413 语义，handler 侧将错误收敛为 `CodeValidation` envelope。

### WR-03: page 无上界，(page-1)*size 整型溢出 → GORM 静默省略 OFFSET，返回第 1 页数据却回显超大页码

**File:** `internal/repository/user.go:71`（联动 `pkg/paging/paging.go:12-27`）

**Issue:** `page` 无上限注释为"超界自然空页"，但极端值走的是另一条静默路径：`GET /api/v1/users?page=9223372036854775807` 经 `strconv.Atoi` 得 int64 最大值，`(page-1)*size` 有符号溢出为负 → GORM `clause/limit.go` 仅在 `Offset > 0` 时写出 OFFSET 子句（已对照 v1.31.2 源码证实），负值被**静默丢弃** → 返回第 1 页数据，envelope 却回显 `page: 9223372036854775807, size: 15`。调用方按 envelope 翻页会拿到静默重复的第 1 页数据——非 500，纯静默错数据，比报错更难排查。

**Fix:** 双保险：`paging.Parse` 给 page 设上限常量（如 `maxPage = 1_000_000`，超界钳位）；`repo.List` 计算后防负：

```go
offset := (page - 1) * size
if offset < 0 { // int64 溢出保护
    offset = 0
}
```

### WR-04: 连接池参数无校验，0/负值语义为"无限制"——架空 T-1-04 声明的连接池兜底

**File:** `internal/config/config.go:111-116`（消费处 `internal/app/app.go:53-55`）

**Issue:** `Load` 仅校验 port 与 DSN 非空。`database/sql` 语义：`SetMaxOpenConns(n≤0)` = 不限制连接数、`SetMaxIdleConns(n≤0)` = 不保留空闲连接、`SetConnMaxLifetime(n≤0)` = 永不回收。配置文件写 `max_open_conns: 0`、键名拼错致 koanf `Int()` 得 0（如 `max_open_conns: "abc"`）、或经 WR-01 失效链传入 0，都会静默把连接池变为无限制——而分页注释（`paging.go:12`）明确把"连接池 MaxOpenConns"列为 DoS 兜底，该兜底可被一行配置静默解除。

**Fix:**

```go
if cfg.MySQL.MaxOpenConns <= 0 ||
    cfg.MySQL.MaxIdleConns < 0 ||
    cfg.MySQL.MaxIdleConns > cfg.MySQL.MaxOpenConns {
    return nil, fmt.Errorf("invalid mysql pool config (open=%d idle=%d)",
        cfg.MySQL.MaxOpenConns, cfg.MySQL.MaxIdleConns)
}
if cfg.MySQL.ConnMaxLifetime <= 0 {
    return nil, fmt.Errorf("invalid mysql.conn_max_lifetime %s (must be > 0)", cfg.MySQL.ConnMaxLifetime)
}
```

## Info

### IN-01: 绑定失败回显 validator 原始错误，与 T-1-01 防泄漏纪律不一致

**File:** `internal/handler/user.go:30,49`

**Issue:** `"参数校验失败: "+err.Error()` 把 validator 原文（含 Go 结构体名如 `UserCreateReq.Username`）出网，而 Recovery 与 `resp.Fail` 都严格收敛固定文案（T-1-01）。泄漏面小但纪律不统一。
**Fix:** 出网仅保留字段路径/tag 摘要或固定文案，原文进 slog。

### IN-02: 入站 X-Request-ID 无长度/字符集约束即回显并入库日志

**File:** `internal/middleware/requestid.go:21-32`

**Issue:** 任意超长/畸形请求头被原样写回响应头、ctx 与访问日志（net/http 会消毒换行注入，但日志体积可被放大）。
**Fix:** 长度上限（如 64）+ 字符白名单（`[0-9a-fA-F-]` 等），不合规则丢弃重生成。

### IN-03: Recovery 中 `c.Abort()` 冗余

**File:** `internal/middleware/recovery.go:25`

**Issue:** `gin.CustomRecovery` 在自定义 handler 返回后自身已执行 Abort（brokenPipe 分支 Abort、其余分支 AbortWithStatus(500)），此处再 Abort 属死代码，易误导读者以为缺了它请求会继续。
**Fix:** 删除该行。

### IN-04: testutil 镜像探测用字典序选"最新" tag，会误选旧版本并误匹配变体镜像

**File:** `internal/testutil/mysql.go:160-173`（同模式 `scripts/test-graceful-shutdown.sh:52-55`）

**Issue:** `line > best` 字典序比较下 `mysql:8.4.9` 会被选中而非 `mysql:8.4.30`；`HasPrefix("mysql:8.4")` 会匹配 `mysql:8.4-oracle` 等变体。仅影响测试基建稳定性。
**Fix:** 提取 `.Tag` 数值段按 semver 排序，或正则精确匹配 `mysql:8\.4\.\d+`。

### IN-05: 关停集成测试断言 (c) 以响应完成时刻归属观察值，存在微秒级 flake 窗口

**File:** `internal/app/shutdown_integration_test.go:230-239`

**Issue:** 洪流循环记录的是**完成时刻** `o.at`：在 `stopStart` 前发出、排空窗口内完成的 200（合法被排空的请求）会被误计入 `postStart200`，理论上可致强断言偶发失败。窗口极窄（SetNotReady 在 stopStart 后微秒内执行）但非零。
**Fix:** 观察结构体增加 `issuedAt`（发出时刻），按发出时刻归属断言。

### IN-06: ddl-dev 明文密码进进程列表

**File:** `Makefile:58`

**Issue:** `mysql -p$(MYSQL_DEV_PASSWORD)` 使密码出现在 `ps` 输出与 shell history。仅本地开发工具，风险低。
**Fix:** 用 `MYSQL_PWD` 环境变量传递或 `--defaults-extra-file`。

### IN-07: systemd unit 以 root 运行，加固项偏少

**File:** `deploy/sys-iam-go.service:18-30`

**Issue:** 已有 `NoNewPrivileges`/`ProtectSystem=full`（T-1-05），但未设 `User=`/`DynamicUser`、`PrivateTmp=`、`ProtectHome=`——进程仍以 root 全能力运行，与其余加固取向不一致。
**Fix:** 增加 `DynamicUser=yes`（或专用低权用户）、`PrivateTmp=yes`、`ProtectHome=read-only`；部署文档同步说明数据目录权限。

### IN-08: builtin 管理字段可经公开 PUT 任意翻转（依赖 Phase 2 收紧，需跨阶段追踪）

**File:** `internal/service/user.go:92-94`（路由白名单 `internal/app/router.go:22-28`）

**Issue:** `builtin`（内置标记）在无认证的 Phase 1 可被任何调用方置 true/false。D-02 已声明全白名单为临时态，但 `builtin`/`status` 属管理语义字段，Phase 2 加认证时必须显式收敛到管理员权限，否则即便登录放开，普通用户仍可改管理标记。建议在 Phase 2 计划中登记该收敛项，避免"临时态"沉淀为常态。
**Fix:** Phase 2 权限模型中把 `builtin`（及后续 status/重置密码类字段）划入管理员专属可更新集合。

---

_Reviewed: 2026-10-31T09:31:46Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
