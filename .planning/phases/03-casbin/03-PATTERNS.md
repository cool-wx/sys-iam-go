---
phase: "03"
---

# Phase 3: Casbin 授权与数据权限 - Pattern Map

**Mapped:** 2026-11-01
**Files analyzed:** 10 类（enforcer / requirePerm / DDL / 双轨同步 / dataScope / dept 子树 / apikey 反向索引 / 路由挂载 / 装配链 / 测试）
**Analogs found:** 9 / 10（dataScope 为新类别，给包组织指引）

## File Pattern Map

| 新/改文件 | Role | Data Flow | Closest Analog | Match Quality |
|-----------|------|-----------|----------------|---------------|
| `internal/casbin/enforcer.go`（新） | service（授权引擎） | request-response（Enforce 判定） | `pkg/ids/id.go`（进程级单例）+ `internal/app/app.go:64-77`（装配 fail-fast） | pattern-match（新类别） |
| `internal/casbin/model.go`（新，model.conf 内嵌 const，D-03） | config | — | `internal/auth/session.go:19-32`（Redis key 前缀常量块风格：const + 逐行注释） | pattern-match |
| `internal/middleware/perm.go`（新，requirePerm） | middleware | request-response | `internal/middleware/auth.go`（RequireRoles :64-91 挂载/abort 形态；被替换对象） | exact |
| `scripts/ddl/casbin_rule.sql`（新） | migration | — | `scripts/ddl/sys_role_menu.sql` | exact |
| `internal/service/role.go` 修改（AssignMenus 同步 Casbin，D-13） | service | CRUD（事务） | 自身 `AssignMenus` :159-210 | exact |
| `pkg/datascope/` 或 `internal/datascope/`（新，五档 Scope） | utility（GORM Scope） | transform | 无直接类比；组合风格参考 `internal/repository/user.go` List 的 `q()` 闭包（:76-79） | no-analog（给指引） |
| `internal/repository/dept.go` 扩展（子树 ID 集，D-10 档位 3） | repository | CRUD | 自身 `List` :70-79 + `internal/tree/tree.go` Build | exact |
| `internal/auth/session.go` 扩展（apikey→user 反向索引，D-16/IN-14） | service | event-driven（Redis 原子块） | 自身 `KickoutAPIKey` :211-235 与 `Login` :74-99 | exact |
| `internal/app/router.go` 修改（全量挂 requirePerm） | route | — | 自身 pri 组 :45-95 | exact |
| `internal/app/app.go` 修改（enforcer 装配插入点） | config（装配） | — | 自身 Redis 段 :64-77 | exact |
| `internal/middleware/perm_test.go`（新）+ testutil DDL 扩展 | test | — | `internal/middleware/auth_test.go` + `internal/testutil/mysql.go` | exact |

> 所有类比文件均为 git 跟踪源码（工作区实读，无 gitignored 镜像）。

## Code Excerpts

### 1. requirePerm 中间件 — 类比 `internal/middleware/auth.go`

**挂载/abort 形态**（RequireRoles，:64-91——本阶段被替换，形态照抄、判定逻辑换 enforcer）：

```go
// RequireRoles 授权 gate（WR-05：Phase 3 Casbin requirePerm 落地前的过渡收口）：
// ... 必须挂在 Auth 之后（依赖 identity 注入）；identity 缺失按未登录 401 收敛
func RequireRoles(allowed ...string) gin.HandlerFunc {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = struct{}{}
	}
	return func(c *gin.Context) {
		identity, ok := auth.FromContext(c.Request.Context())
		if !ok {
			resp.Fail(c, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		for _, r := range identity.Roles {
			if _, hit := allowedSet[r]; hit {
				c.Next()
				return
			}
		}
		resp.Fail(c, errcode.ErrForbidden)
		c.Abort()
	}
}
```

**Auth 中间件的 Identity 注入与 fail-closed 错误三分支**（:38-61）：requirePerm 同样依赖 `auth.FromContext`；enforcer 故障分支照 `resp.Fail(c, errcode.ErrInternal); c.Abort()`（绝不静默放行，T-2-04 同款）。403 语义用 `errcode.ErrForbidden`（10200）。

**复制什么：** 闭包工厂签名 `func RequirePerm(perm string) gin.HandlerFunc`、identity 缺失 401、不命中 403 Abort、enforcer 错误 500 fail-closed。**改什么：** 判定从 roles 快照集合匹配换成 `enforcer.Enforce(userKey, perm)`；super_admin 豁免前置判断（D-07，判据与 enforcer g 策略两处一致）。

### 2. enforcer 单例 — 类比 `pkg/ids/id.go:58-84`

```go
var (
	mu   sync.Mutex
	dflt *Node
)

// SetDefault 装配进程级单例节点（main 启动时按配置调用一次）。
func SetDefault(n *Node) {
	mu.Lock()
	defer mu.Unlock()
	dflt = n
}

// Default 返回进程默认节点；未显式装配时惰性创建 ...
func Default() *Node {
	mu.Lock()
	defer mu.Unlock()
	if dflt == nil { ... }
	return dflt
}
```

**复制什么：** 进程级单例的 mu/双函数形态（SetDefault 显式装配 + Default 兜底），包注释先讲契约。**改什么：** CONTEXT 用 sync.Once 亦可（D-02）；enforcer 初始化带 error（LoadPolicy fail-fast），更适合 `New(...) (*Enforcer, error)` 构造 + 显式注入 app，而非惰性 panic——见下一条 app.go 装配先例。

### 3. 装配链 fail-fast 插入点 — 类比 `internal/app/app.go:64-77`

```go
	// Redis 会话存储（go-redis v9；启动 Ping fail-fast 对齐 app.New 的 err 返回风格
	// ——会话基建不可用时服务绝不能以"全量 401"状态静默启动）
	rdb := redis.NewClient(&redis.Options{ ... })
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = rdb.Ping(pingCtx).Err()
	cancel()
	if err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
```

**复制什么：** D-04 enforcer 初始化 + 全量 LoadPolicy 失败 `return nil, fmt.Errorf(...)` fail-fast，插在 Redis 段之后、sessionMgr 之前（enforcer 是 requirePerm 中间件与 roleSvc 的依赖，需在 service 构造 :80-97 之前建好并注入 `service.NewRoleService(...)` 新参）。**改什么：** 连接复用既有 `db`（gorm-adapter 复用 GORM 连接/DSN，D-01）。

### 4. 路由逐路由挂载 — 类比 `internal/app/router.go:45-95`

```go
	pri := engine.Group("/", authMw)
	pri.POST("/api/v1/auth/logout", hs.Auth.Logout)
	// kickout 授权收口（WR-05/T-2-07 过渡态）：仅 super_admin 可调 ... 其余管理端点
	// 的 requirePerm 全量收口在 Phase 3 Casbin 计划内
	pri.POST("/api/v1/auth/kickout", middleware.RequireRoles("super_admin"), hs.Auth.Kickout)
	pri.POST("/api/v1/users", hs.Users.Create)
	...
	pri.PUT("/api/v1/roles/:id/menus", hs.Roles.AssignMenus)
```

**复制什么：** `pri.POST(path, middleware.RequirePerm("sys:user:create"), handler)` 逐路由内联中间件形态（D-08 显式挂载，路由与权限同屏）；kickout 行保留 RequireRoles 叠加（D-15 纵深防御）。**改什么：** 每条管理路由加一个 perm 串参数；:47-49 的过渡注释改为收口完成说明。

### 5. 双轨同步插入点 — 类比 `internal/service/role.go AssignMenus :159-210`

```go
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationTx := repository.NewRelationRepo(tx)
		if err := relationTx.DeleteRoleMenus(ctx, roleID); err != nil {
			return err
		}
		rows := make([]po.RoleMenuPO, 0, len(reqIDs))
		for _, rid := range reqIDs {
			rows = append(rows, po.RoleMenuPO{ ... })
		}
		return relationTx.BatchCreateRoleMenus(ctx, rows)
	})
```

**复制什么：** 全删重建事务骨架、`NewXxxRepo(tx)` 事务内重建 repo 的惯例、前置全量校验→事务内的顺序。**改什么：** 事务成功提交后（或事务内，研究定夺一致性窗口取舍）追加 enforcer 的 `RemoveFilteredPolicies(1, roleKey)` + `AddPolicies(...)` 批量同步（D-13）；注意 Casbin 自有 adapter 连接，不经 tx——同步语义由 service 单点顺序保证。同事务风格先例还有 `UserService.AssignRoles`（user.go :278-293）。

### 6. apikey 反向索引 — 类比 `internal/auth/session.go`

**原子块模式**（Login :85-95）：

```go
	if _, err := sm.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, sessionKey, map[string]any{...})
		p.Expire(ctx, sessionKey, sm.timeout)
		p.SAdd(ctx, userTokensKey, token)
		return nil
	}); err != nil {
		return "", err
	}
```

**反查撤销模式**（KickoutAPIKey :211-235）：`Get 映射 → HGet userId → TxPipelined{Del 会话, SRem 索引, Del 映射}`，redis.Nil 幂等返回 nil。

**复制什么：** key 前缀常量块（:28-32，新前缀 `iam:apikey-user:{apiKey}` 加在此处并注释为全项目契约）、`TxPipelined` 原子块、redis.Nil 幂等、死键惰性收敛注释风格。**改什么：** Login/Unbind 路径增加反向索引写/删；apikey-login 拒绝路径（过期 key）调 KickoutUser（user 轴）撤销存量会话。注意 session.go :40-46 注释明确 permissions 快照不放——Phase 3 即时生效语义即此决策的兑现，勿向 SessionPayload 加 permissions。

### 7. dept 子树 ID 集 — 类比 `internal/repository/dept.go:70-79` + `internal/tree/tree.go:32`

```go
// List 全量查询，ORDER BY sort ASC, id ASC（... id ASC tiebreaker 保证稳定序）
func (r *DeptRepo) List(ctx context.Context) ([]po.DeptPO, error) {
	var depts []po.DeptPO
	err := r.db.WithContext(ctx).Model(&po.DeptPO{}).
		Order("sort ASC, id ASC").
		Find(&depts).Error
	...
}
```

```go
func Build[T any](items []T, idOf func(T) int64, parentIDOf func(T) *int64, attach func(node *T, children []T)) []T
```

**复制什么：** 方案 A——`List` 全量 + 在内存过滤出目标节点的子树（自引用遍历 map[parentID][]id，无需 tree.Build 的树结构，只需扁平祖先闭包）；新方法命名如 `SubtreeIDs(ctx, rootID)` 放 DeptRepo 或 datascope 包内经 DeptRepo.List 组合。**改什么：** 不挂 attach 回调，仅收集 ID 集；孤儿/环语义沿 tree.go 注释已锁定的源语义。

### 8. dataScope Scope（新类别）— 包组织指引

无直接类比。组合风格照 `internal/repository/user.go:76-79` 的查询闭包：

```go
	q := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&po.UserPO{})
	}
```

- **包位置：** `internal/datascope`（依赖 po/auth 内部类型时选 internal；纯通用无依赖可选 pkg——研究定夺，CONTEXT 已留自由度）。公开面建议：`type Scope int8`（1..5 五档，D-10）+ `func Filter(user auth.Identity, deptIDs []int64) func(db *gorm.DB) *gorm.DB`——GORM Scope 函数签名，repo 内 `.Scopes(datascope.Filter(...))` 组合（D-09 不侵入既有查询）。
- **档位 SQL（D-10）：** 1=`create_by_id = ?`；2=`dept_id = ?`；3=`dept_id IN ?`（子树一次查询预取）；4=`dept_id IN ?`（自定义集合）；5=恒 true 不加条件。
- **档位宽取（D-11）：** service 层读用户全部角色的 data_scope 取 min 数值即最宽档（sys_role.data_scope 值域沿源：数字越小权限越宽）。

### 9. casbin_rule DDL — 类比 `scripts/ddl/sys_role_menu.sql`（全文）

```sql
-- sys_role_menu 建表 DDL（Phase 2 RBAC 主链路）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_role_menu，2026-10-31）
-- 列集 8 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。

DROP TABLE IF EXISTS `sys_role_menu`;
CREATE TABLE `sys_role_menu` (
  ...
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

**复制什么：** 头注释三件套（来源/列集说明/幂等声明）、`DROP TABLE IF EXISTS` + 显式列定义、utf8mb4_general_ci。**改什么：** 列集为 gorm-adapter 默认 casbin_rule 六列（id/p_type/v0..v5）； CONTEXT 倾向显式 DDL 与既有表管理一致。**注意：** `internal/testutil/mysql.go:62` 的 `execDDLDir` 自动按文件名序执行 ddl 目录全部 .sql——新文件落 `scripts/ddl/casbin_rule.sql` 后测试容器自动建表，testutil **零改动**。

### 10. 测试 — 类比 `internal/middleware/auth_test.go` + `internal/testutil/mysql.go`

- **中间件单测（无 build tag，miniredis + httptest）**：照 auth_test.go :23-53 的 `newAuthTestEnv`/`doProbe` 探针 harness；perm_test 需要 enforcer——用 gorm-adapter + sqlite 内存库不可行（禁 CGO），改用 miniredis 不适用（策略在 MySQL）；建议 enforcer 可注入 fake Enforcer 接口或单测走 testcontainers MySQL（照 testutil.RunMySQL）。表格驱动断言风格照 :58-83（401/10200/10001 code 直证）。
- **集成测试**：`newTestApp` 双容器契约基建 + per-test 独立容器（CONTEXT Established Patterns）；DDL 目录自动加载见上条。
- **requirePerm 三分支直证模板**（照 TestRequireRolesMiddleware :116-160）：super_admin 会话 200、无权限 403/10200 `"无权限"`、无 token 401——requirePerm 版再加"授权后下一请求生效"（PERM-02）用例：AssignMenus 后**不重登录**直接访问目标端点。

## Shared Patterns

### 错误码语义
**Source:** `internal/middleware/auth.go`（401/ErrUnauthenticated、403/ErrForbidden=10200、500/ErrInternal）、`pkg/errcode`
**Apply to:** perm.go 中间件、service 层译域。403 文案 `"无权限"`（auth_test.go :155 直证既有契约）。

### 审计 ctx 传递
**Source:** `internal/repository/user.go:50-53`（`auth.FromContext` 读 identity 写审计列）
**Apply to:** dataScope 过滤同样经 ctx identity 取 uid/deptId；勿新增全局变量。

### 参数化查询（T-1-03）
**Source:** `internal/repository/relation.go:80-82`（`Where("role_id = ?", roleID)`）
**Apply to:** datascope 注入的 WHERE 子句一律占位符；IN 子句 `Where("dept_id IN ?", ids)`。

### 注释裁决风格
**Source:** 全库惯例（每个导出符号带决策编号注释，如 "D-13"/"T-2-11"）
**Apply to:** 新文件头部注释引用本阶段决策编号（D-01..D-16、PERM-01..06）。

## No Analog Found

| File | Role | Data Flow | Reason | 指引 |
|------|------|-----------|--------|------|
| `internal/casbin/`（enforcer+model） | service | request-response | 代码库无第三方引擎单例先例 | 单例照 pkg/ids、装配照 app.go Redis 段、常量照 session.go key 块 |
| `internal/datascope/`（五档 Scope） | utility | transform | 无行过滤通用件先例 | Scope 函数签名照 GORM 惯例，SQL 注入风格照 relation.go 占位符 |

## Metadata

**Analog search scope:** internal/{middleware,auth,service,repository,app,testutil,tree}, pkg/ids, scripts/ddl
**Files read:** 13（auth.go, auth_test.go, id.go, app.go, router.go, role.go, relation.go, user.go(:180-299), dept.go, session.go, tree.go, testutil/mysql.go, sys_role_menu.sql）
**Pattern extraction date:** 2026-11-01
