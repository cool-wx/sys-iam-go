# Phase 5: 名称同步事件与验收收尾 - Pattern Map

**Mapped:** 2026-11-02
**Files analyzed:** 12（新增 4 + 修改 8）
**Analogs found:** 12 / 12

## File Classification

| 新增/修改文件 | Role | Data Flow | Closest Analog | Match Quality |
|---------------|------|-----------|----------------|---------------|
| `internal/event/event.go` | utility（事件类型 + Publisher 接口） | event-driven | 无直接 analog——接口定义仿 `internal/service/role.go` 对 `*authz.Enforcer` 的最小依赖注入风格 | role-match |
| `internal/event/dispatcher.go` | utility（注册表 + 同步分发） | event-driven | 无直接 analog——map[Type][]Listener 自建（D-01，约 60 行）；注册挂载时机仿 `pkg/ids` sync.Once 单例装配心智 | role-match |
| `internal/event/listeners.go` | service（6 个同步 handler） | event-driven + CRUD（确定性 UPDATE） | `internal/repository/user.go` Patch（单 Model+Where+Updates 确定性写）+ `internal/service/role.go` db.Transaction 边界 | exact |
| `internal/event/listeners_test.go`（或 handler 层集成测试） | test | event-driven | `internal/handler/user_integration_test.go`（newTestApp 基建） | exact |
| `internal/service/dept.go` Update | service | request-response + 发布挂点 | 自身现状 + `internal/service/role.go` Update 的"Patch 后回读 + 外部设施同步、失败上抛"段（:150-183） | exact |
| `internal/service/role.go` Update | service | 同上 | 自身（status 守卫已示范 Patch 前条件 FindByID） | exact |
| `internal/service/menu.go` Update | service | 同上 | `internal/service/dept.go` Update（同构域） | exact |
| `internal/service/dict.go` UpdateDictType | service | 同上 | `internal/service/dept.go` Update（单表 Patch） | exact |
| `internal/service/user.go` Update | service | 同上（双事件挂点之一 realName） | `internal/service/dept.go` Update + 自身 validateDept 的"Patch 前条件查库"先例 | exact |
| `internal/app/app.go` 装配 | provider（组装根） | — | 自身 :150-188 构造链（repo → 中间设施 → service → handler 顺序） | exact |
| `internal/service/{file_category,dept,menu}.go` 移根语义（WR-07） | service | request-response | 三处现有 `cols["parent_id"] = 0` 行（file_category.go:72 / dept.go:66 / menu.go:87） | exact |
| `internal/handler/file_integration_test.go`（WR-08 断言修复） | test | — | 自身 :347-353 待修断言 + 同文件 :320-336 的 `env.Data` envelope 正确读法 | exact |

## Pattern Assignments

### `internal/event/listeners.go`（service，event-driven + CRUD）

**Analog A（独立事务边界）:** `internal/service/role.go` assignMenus（db.Transaction 模板，grep :249/:334）

```go
// role.go 既有纪律：db.Transaction{ 删旧 + 插新 }，SkipDefaultTransaction 下
// 唯一显式事务边界。handler 照此：6 个 handler 各自包一个 Transaction（D-02），
// realName handler 的 26 条 UPDATE 全在同一闭包内（照源 REQUIRES_NEW 单事务）。
err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    // 多条确定性 UPDATE，首错 return err → 整体回滚
})
```

**Analog B（确定性 UPDATE 形态）:** `internal/repository/user.go` Patch（:45-70）

```go
// repo.Patch 的确定性风格是 handler UPDATE 的模板：恒带 Where 防 Type 白名单外
// 全表更新（T-1-08），列名为代码常量、值全占位符（T-1-03）。
res := r.db.WithContext(ctx).Model(&po.UserPO{}).Where("id = ?", id).Updates(cols)
```

handler 版三段式（ research Pattern 2，列名常量 + `<>` 条件）：

```go
tx.Model(&po.UserPO{}).
    Where("dept_id = ? AND dept_name <> ?", evt.ID, evt.NewName).
    Update("dept_name", evt.NewName)
```

注意：**不写 `OR col IS NULL`**（Pitfall 3——NULL 行恒不中是验收明文）；**不扩 repository**（26 个专用方法膨胀 relation.go，冗余同步非实体仓储语义；若 planner 偏好收口，可加单个通用 `SyncNameColumn(model, setCol, whereCol, id, newName)`，二选一）。

**错误处理:** 首错即返 → Publish 返回 error → service 上抛 → 500（D-03，沿 role.go 双轨纪律"Patch 成功后外部设施失败 500 + 重试自愈"）。

### `internal/service/dept.go` Update（发布挂点模板，其余 4 域同构）

**Analog（Patch 后回读 + 外部同步、失败上抛）:** `internal/service/role.go` Update :150-183

```go
// role.go 既有形态（照抄时序纪律）：Patch 成功后 → 条件回读 → 外部设施同步 →
// 失败直接 return nil, err（500）。本阶段把"外部设施"从 enforcer 换成 publisher。
if err := s.roleRepo.Patch(ctx, id, cols); err != nil { ... }
if req.Status != nil {
    role, err := s.roleRepo.FindByID(ctx, id)   // Patch 后回读
    if err != nil { return nil, err }
    if err := s.enf.ClearRole(role.RoleCode); err != nil { return nil, err } // 失败 500
}
```

**Analog（Patch 前条件查库）:** role.go Update :113-119（builtin 守卫——`req.Status != nil` 才 FindByID，本阶段照此在 `req.DeptName != nil` 时回读旧名）+ user.go validateDept（查库值不信请求体）。

dept.go Update 改造要点（现状 :56-101 已直读）：

```go
// Patch 之前（发布条件需要旧名——当前无一回读）：
var old *po.DeptPO
if req.DeptName != nil {
    var err error
    old, err = s.repo.FindByID(ctx, id)
    if err != nil { /* ErrNotFound 译 404 域消息，照 Get :108-110 同款提前拦 */ }
}
// ... repo.Patch 成功（err == nil 即已提交，SkipDefaultTransaction 单语句自动提交）之后：
if old != nil && req.DeptName != nil {
    oldName := ""
    if old.DeptName != nil { oldName = *old.DeptName }   // 源 oldName 兜底 ""
    if *req.DeptName != oldName {                        // 源按值变化检测，禁无条件发布
        if err := s.pub.Publish(event.NameChangedEvent{...}); err != nil {
            return nil, err                               // → 500（D-03）
        }
    }
}
```

**构造函数改造:** `NewDeptService(repo)`（dept.go:23）→ 加 `pub event.Publisher`。5 个 service 同构：role.go:40、user.go:42、menu.go、dict.go。

### `internal/event/event.go` + `dispatcher.go`（新包，无直接 analog）

接口最小化风格仿 `internal/service/user.go` 对 `*auth.SessionManager` / `*authz.Enforcer` 的依赖注入：service 只依赖接口不依赖具体 Dispatcher（mock 点）。research Pattern 1 已给出完整建议形态（Type 常量取源 eventType 字符串、NameChangedEvent 三元组载荷、`Publish(evt) error`、map[Type][]Listener 同步逐个调用首错即返）。禁 goroutine 异步、禁 channel 队列。

**对照参考（仅对照，勿模仿）:** `internal/middleware/op_log.go` 的 chan + worker + Flush 是异步落库先例——D-02 明文本阶段**不用** chan/goroutine（异步使 handler 失败脱离请求错误链）。

### `internal/app/app.go` 装配链（:150-188 已直读）

```go
// 现状顺序：userRepo...faultInfoRepo（:150-162）→ ologW（:164-166）→
// userSvc 等全部 service 构造（:167-180）→ handler 构造 → newRouter（:188）
// 插入点：repo 构造之后、service 构造之前（约 :167 前）：
//   dispatcher := event.NewDispatcher()
//   event.RegisterAll(dispatcher, db)          // 6 listener 注册
// 5 个 service 构造各加 dispatcher 实参（dispatcher 实现 Publisher 接口）
```

挂载时机仿既有先例：ologW 在 service 前 fail-fast 构造、sessionMgr/storage 在 service 前——基建先于消费者。

### `internal/service/{file_category,dept,menu}.go` 移根语义（WR-07，方向 A）

三处现状行（已直读）：file_category.go:70-72、dept.go:63-67、menu.go:84-88。

```go
// 现状：cols["parent_id"] = 0   → 响应 "0"（与 Create 根的 null 双形态）
// 改为：cols["parent_id"] = nil → 根 = NULL 成为唯一形态
```

每域 1 行 + 更新紧邻注释（"与 Create 语义对齐"注释在 menu 域名不副实，需改为 Go 内一致性修正的表述）。DTO 零改动——既有 `if m.ParentID != nil`（dto/dept.go:66、dto/menu.go:91、dto/file.go:77）自然输出 null。tree.Build / ensureNotDescendant / resolveParent 对 nil/0 双兼容已核（dept.go:182、186），零回归风险。

### `internal/handler/file_integration_test.go` WR-08（:347-353 已直读）

```go
// 现状 bug：json.Unmarshal(body, &cleared) 漏 {code,message,data} envelope 的
// Data 层——cleared 永不填充、断言永真。同文件 :320-336 已有正确 envelope 读法
// （var env struct{ ... Data []... }）可仿。
// 修复：补 envelope + assert.Nil(env.Data.ParentID) 真化——必须与 WR-07 同任务
// 落地（只修测试不修响应则断言立刻红，Pitfall 4）。
```

### 集成测试（新）与构造签名波及面

**Analog:** `internal/handler/user_integration_test.go:178` newTestApp（per-test testcontainer + 真实 app.New 装配——dispatcher 注入自动生效，逐测试零改动）；service 层单测仿 `internal/service/user_integration_test.go:49` 直构造形态（**该处必须同步加 publisher 实参，传 fake Publisher——顺带成为发布条件的单测 mock 点**，Pitfall 5：漏改则 `go build ./...` 失败）；checkDB 断言沿既有集成测试 `openCheckDB` 先例。

测试三层（research Implementation Guidance）：① service 单测（fake Publisher：改名发一次/仅改备注零发/同名零发/404 零发）；② handler 集成（6 事件断言 + realName 14 表抽样 + **NULL 语义锁定** + **幂等自愈重放**）；③ `go test ./...` 全绿为回归门（D-05）。

## Shared Patterns

### "提交后外部调用 + 失败 500 + 幂等重试自愈"（D-03 双轨纪律）
**Source:** `internal/service/role.go` Update :150-183（casbin 先例）+ `internal/service/role.go` assignMenus :334 注释
**Apply to:** 5 个 service Update 发布挂点 + 6 个 handler——发布语句物理放在 Patch/Transaction 返回 nil 之后；handler 失败错误直传上抛。

### 确定性参数化 UPDATE（T-1-03 / T-1-08）
**Source:** `internal/repository/user.go` Patch :45-55
**Apply to:** 全部 26+ 条同步 UPDATE——Model(&po.XxxPO{}) + Where 占位符 + 常量列名，禁 fmt.Sprintf 拼 SQL。

### 显式事务边界
**Source:** `internal/service/role.go` assignMenus（db.Transaction 全删重建）
**Apply to:** 6 个 handler 各自包 `db.WithContext(ctx).Transaction`（D-02 独立事务）。

### ErrNotFound 译域消息
**Source:** `internal/service/dept.go` Get :105-114 / Update :94-99
**Apply to:** 挂点回读旧行的 404 提前拦截（沿各域既有消息原文）。

### 白名单构造 cols map（ENG-05）
**Source:** `internal/service/dept.go` Update :57-93
**Apply to:** 移根语义修改时保持白名单结构不动，仅改 `parent_id` 值 0→nil。

## No Analog Found

| 文件 | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/event/{event,dispatcher}.go` | utility | event-driven | 仓库无事件机制先例（op_log chan 仅作对照非模板）——直接采用 research Pattern 1 形态（约 60 行，标准库自建，D-01） |

## Metadata

**Analog search scope:** internal/{service,repository,app,middleware,handler,testutil}、pkg
**Files scanned:** ~20（8 个 analog 全文/关键段直读，全部 `git ls-files` 验证为跟踪源文件）
**Pattern extraction date:** 2026-11-02
