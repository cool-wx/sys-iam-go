# Project Research Summary

**Project:** sys-iam-go（Spring Boot IAM 后端 → Go 全量移植）
**Domain:** Go IAM/admin 后端（工控机单机部署：单二进制 + 本地 MySQL + 本地 Redis）
**Researched:** 2026-10-31
**Confidence:** HIGH（源码行为清单为逐文件一手阅读；Go 侧选型经 module proxy/context7 实证）

## Executive Summary

本项目是将一个 115 文件、14 个功能域的 Spring Boot（DDD 四层 + MyBatis-Plus + Sa-Token + Nacos + Dubbo）IAM 后端整体移植为 Go 1.27 + Gin + GORM 单体服务，部署目标为工控机单机（`CGO_ENABLED=0` 纯静态单二进制 + systemd）。这是 brownfield 行为基准移植而非绿地开发：14 个域的端点、边缘案例、6 个名称同步事件、审计填充、操作日志切面等横切机制已全部从源码逐一提取，验收标准是全部域接口 + 自动化测试通过。研究同时确认了一批"反特性"不必移植——OAuth2（纯死依赖）、Dubbo RPC 4 接口、Nacos、dataScope 数据权限过滤（源只有字段没有实现）。

推荐技术路线：Gin v1.12 + GORM v1.31（显式 TableName 映射、禁用 AutoMigrate）+ go-redis v9；认证用**自建 Redis token-session 管理器**（约 200 行）复刻 Sa-Token 的服务端会话语义——不用 JWT，因为登出即失效、踢人下线、权限快照语义都是硬需求；bcrypt 经验证与存量 Hutool `$2a$` 哈希直接兼容，**存量用户密码零迁移**。存储侧用 `Storage` 接口双实现（本地磁盘默认 + minio-go 接 RustFS）。

最大风险集中在三类隐蔽契约：① GORM 零值更新 ≠ MyBatis-Plus 的 null-skip（指针 DTO + 显式 Select 是唯一解，否则生产数据被静默清空）；② JSON 序列化契约漂移（雪花 ID 必须 JSON 字符串化防 JS 精度丢失、时间输出无时区 `yyyy-MM-dd HH:mm:ss`、枚举三表示面）；③ 时区（DSN 必须 `loc=Asia/Shanghai` + parseTime，否则 UTC 部署机上差 8 小时）。这些全部在 PITFALLS.md 给出了预防方案并映射到具体阶段，风险可控。

## Key Findings

### Recommended Stack（详见 STACK.md）

版本号全部经 Go module proxy 实时验证（2026-10-31）。

- **Go 1.27.3** — 当前稳定版；纯静态编译契合工控机交付
- **Gin v1.12.0** — HTTP 框架；validator v10 内置集成承担全部请求校验
- **GORM v1.31.2 + driver/mysql v1.6.0** — `Tabler` 接口精确映射现有表名；AutoMigrate 禁用；审计填充用 BeforeCreate/BeforeUpdate hooks
- **go-redis v9.23.0** — 会话存储与缓存共用；Hash + TTL 原语齐备
- **x/crypto/bcrypt v0.58.0** — 已验证兼容存量 `$2a$` 哈希，HIGH 置信
- **支持件**：excelize v2.11（Excel 导入导出）、minio-go v7.4.1（S3/RustFS）、koanf v2（配置，替代 Nacos）、slog + slog-gin（日志）、swaggo（OpenAPI 文档）、prometheus client（指标）
- **测试**：testify + testcontainers-go（开发机集成测试）+ miniredis（单元测试）
- **明确不用**：JWT、gorilla/sessions、DDD 目录结构、任何 CGO 依赖、事件总线框架、GORM AutoMigration、软删除

### Expected Features（详见 FEATURES.md——全部为一手源码阅读）

**Must have（14 域 + 4 项横切，缺一验收失败）：**
- RBAC 主链路：Auth（含会话快照 + apikeyLogin 双路径）、User（防 N+1 装配 + builtin 硬拦截）、Role、Menu、Dept（含孤儿提升建树）
- 配置类域：DictType / DictData / Config / ApiKey
- 文件域：FileCategory + File（MD5 判重含 `categoryId IS NULL` 隐藏分支）
- 项目特有：Shift（全量无分页）、FaultInfo（三键 upsert Excel 导入）
- OperationLog 切面（参数掩码、errorMsg 截 500、失败静默）
- 横切四件套：统一 R 响应/异常映射/分页钳位（1/15，Log 例外 1/10）/审计字段自动填充
- 横切名称同步：6 个事件监听器（realName 事件要同步 **14 张表**），全部 UPDATE 带 `!= newName` 条件

**关键语义（必保）**：权限/角色在**登录时快照进 Redis 会话**，改权限需重登录生效——不要"改进"为实时查库。

**Defer（v2+ 或不移植）**：OAuth2（源是死依赖）、Dubbo RPC 等价物、dataScope 过滤、禁用踢会话/密码强度/账号锁定（源均无）；OpenAPI 文档与健康检查为 P2 不阻塞验收。

### Architecture Approach（详见 ARCHITECTURE.md）

Go 标准布局单体：`cmd/server` + `internal/{handler,service,repository,model,middleware,auth,event,storage,health,config,app}` + `pkg/{resp,errcode,paging}`（仅三个跨项目复用小包）。源 DDD 四层按映射表打散归位——GORM 即 repository，不再分层。事件为进程内同步 dispatcher，**事务提交后发布**（刻意修正源的"事务内发布 + REQUIRES_NEW"瑕疵）；用户身份用 `context.Context` 传播（替代 ThreadLocal）；优雅关停 + `/healthz` `/readyz` 对齐 systemd/k3s 探针。

**关键组件**：`internal/auth`（SessionStore + Redis 实现）、`internal/event`（6 个 name-sync handler，独立事务）、`internal/storage`（local/s3 双实现）、`internal/middleware`（auth/oplog/recovery）、GORM hooks 审计填充（无 identity 时跳过，语义="系统行为无操作人"）。

### Critical Pitfalls（详见 PITFALLS.md——18 项全部映射到阶段）

1. **GORM 零值更新 ≠ MP null-skip（最隐蔽数据破坏点）** — 写路径 DTO 一律指针字段，更新走 `Updates(map)`/显式 `Select`，禁裸 `Save()`；集成测试锁定"不传字段保持原值"
2. **雪花 ID 双陷阱** — DB 无 AUTO_INCREMENT，Go 侧必须应用层生成（snowflake 库 + BeforeCreate）；int64 ID 必须 JSON 字符串化防 JS 2^53 精度丢失
3. **JSON 契约三特征漂移** — Long→String、时间 `yyyy-MM-dd HH:mm:ss` 无时区、null 键照常输出（禁 omitempty）、枚举读侧兼容 name|int 且未知值兜底；把源 JacksonConfigTest 翻成 Go 契约测试
4. **时区** — DSN 固化 `charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai`；systemd 显式 TZ；`TZ=UTC` 环境跑集成测试验证
5. **禁加 DeletedAt 软删除 / 不移植乐观锁** — 源是硬删除 + last-write-wins，全库无 @TableLogic/@Version
6. **BCrypt 错误语义** — 畸形 hash 在 Go 返回 error，须封装为 `VerifyPassword → bool`（error 一律 false），否则老账号登录 500
7. **事件必须同步执行**（禁 goroutine 异步），`.ne` 条件 NULL 语义原样保留；**oplog 中间件必须回放 body**（`io.ReadAll` + NopCloser）

## Implications for Roadmap

粒度要求：coarse，3-5 个 phase。综合三份研究的依赖图，建议 **4 个 phase**：

### Phase 1: 工程骨架 + 数据访问/横切地基
**Rationale:** 一切域都站在它上面；雪花 ID、序列化契约、审计填充、分页、错误码是全库共享约定，必须先定死，且 18 个 pitfall 中有 9 个在此阶段一次性预防。
**Delivers:** 可启动的单体服务（config→装配→路由→优雅关停→/healthz /readyz）、R envelope + errcode、雪花 ID 生成 + JSON 字符串化、LocalDateTime 等价类型 + 枚举四件套、审计 hooks、分页 helper、DSN/TZ 固化、连接池默认值、systemd unit
**Addresses:** 横切四件套；Pitfall 1/2/4/5/6/14/15/16/17/18
**Avoids:** 零值更新约定（指针 DTO + Patch helper 在此定义）

### Phase 2: Auth 会话 + RBAC 主链路（User/Role/Menu/Dept/ApiKey）
**Rationale:** Auth 依赖四域数据装配会话；identity 是 oplog、权限校验、审计填充的共同前置；User 域是打通全链路的最佳纵向切片。bcrypt 存量哈希兼容性在此阶段验收。
**Delivers:** 自建 token-session 管理器（Redis、TTL/续期/踢下线索引）、Auth 中间件 + 白名单 + requirePerm（50 个权限串）、五个域完整 CRUD + UserVo 防 N+1 + builtin 守卫
**Addresses:** Auth/User/Role/Menu/Dept/ApiKey 域；Pitfall 3/7/10
**Avoids:** JWT、权限实时查库等语义漂移

### Phase 3: 其余业务域 + OperationLog（DictType/DictData/Config/FileCategory/File/Shift/FaultInfo + oplog 切面 + Excel）
**Rationale:** 这些域只依赖 Phase 1 地基（与 Phase 2 可部分并行）；oplog 依赖 auth 上下文；Excel 设施是 FaultInfo/User 导入的前置。
**Delivers:** Storage 接口（local 先行 + s3 第二实现）、文件域含 MD5 判重与流式下载、oplog 中间件（body 回放 + 掩码 + 元数据声明）、excelize 封装（表头映射 + 单元格清洗 + 整批事务 upsert）
**Addresses:** 7 个域 + OperationLog；Pitfall 11/12/13
**Avoids:** goroutine 写日志、Excel 列序号硬编码

### Phase 4: 名称同步事件网 + 收尾（Swagger/健康指标/验收回归）
**Rationale:** 6 个监听器横跨 8 个域，必须等所有域 CRUD 稳定后统一实现 + 测试；AFTER_COMMIT 等价时序（提交后同步发布、handler 独立事务）是本项目对源瑕疵的显式修正。
**Delivers:** 事件 dispatcher + 6 个 name-sync handler（含 `.ne` NULL 语义锁定测试）、swaggo 文档、Prometheus 指标、全量契约/集成测试回归 + "Looks Done But Isn't" 清单逐项过
**Addresses:** 横切名称同步；Pitfall 9
**Avoids:** 事务内发布、异步事件

### Phase Ordering Rationale

- 依赖链决定顺序：骨架 → auth → 业务域 → 跨域事件（FEATURES.md 依赖图 + ARCHITECTURE.md Build Order 完全一致）
- Pitfall-to-Phase 映射表（PITFALLS.md）可直接作为各 phase 验收项来源
- Phase 3 与 Phase 2 后半段理论上可并行，coarse 粒度下合并为顺序执行即可

### Research Flags

**Phase 规划时需补充研究/确认：**
- **Phase 1:** MySQL 实际 schema 逐列核对（PO 列 tag 是 MyBatis-Plus 下划线命名**推测**而非实证，仓库内无 SQL 文件）——规划时需拿到 DDL 或连库确认
- **Phase 2:** 需向用户索取 Nacos `craft-satoken.yaml` 的 `timeout/active-timeout` 实际值与 `craft-shared.yaml` 连接参数、MySQL collation（仓库不可见，PITFALLS.md 信息缺口）
- **Phase 4:** 无需额外研究——事件语义已从源码逐行提取

**标准模式，可跳过深度研究：**
- Phase 1 骨架（Gin/GORM/systemd 模式已被 ARCHITECTURE.md 7 个 Pattern 完整覆盖）
- Phase 3 的 CRUD 域（源码行为清单完备，按 FEATURES.md 逐端点实现即可）

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | 版本号经 Go module proxy 实时验证；bcrypt 兼容性多源交叉 |
| Features | HIGH | 全部为一手源码逐文件阅读，含源单元测试作行为锚点 |
| Architecture | HIGH | 源码机制实证 + Go 侧官方文档/社区共识交叉（Go 侧单项 MEDIUM，叠加后 HIGH） |
| Pitfalls | HIGH | 18 项均有源文件行级证据；个别标 MEDIUM 见 Gaps |

**Overall confidence:** HIGH

### Gaps to Address

- **PO 列名映射未实证**：MyBatis-Plus 驼峰转下划线是默认行为推测，Phase 1 需对实际 schema 逐列核对
- **Nacos 配置不可见**：sa-token timeout/active-timeout、datasource/redis 参数、MySQL collation——需向用户索取，写进 Go 配置默认值
- **存量 BCrypt hash 抽样验证**：上线前从生产库取 20 个 hash 用 Go 验证（Phase 2 验收项）
- **Redis 切换 runbook**：停机切换前 FLUSHDB 旧 `satoken:` 键或等 TTL 自然过期
- **GORM + Go 1.27 组合**：均为当前最新版，无已知不兼容，但 Phase 1 首周建议先跑通一条真实表的全链路集成测试以消除版本组合残留风险

## Sources

### Primary (HIGH confidence)
- 源码一手审读：`/home/wx/workspace/sys-iam`（14 Controller、14 ApplicationService、13 实体、13 仓储实现、切面、6 监听器、sys-kernel 公共设施、7 个源单元测试）
- Go module proxy（proxy.golang.org）— 全部版本号实时验证，2026-10-31
- Context7：`/gin-gonic/gin`、`/go-gorm/gorm`、`/redis/go-redis`、`/minio/minio-go`、`/xuri/excelize-doc`、`/websites/gorm_io`
- gorm.io 官方文档（transactions/context/update/models/connecting）

### Secondary (MEDIUM confidence)
- go.dev/dl、Wikipedia bcrypt（`$2a$` 跨语言语义）、pkg.go.dev
- 社区共识多源交叉：koanf vs viper、slog vs zap、swaggo vs oapi-codegen、goose vs golang-migrate、project-layout
- sa-token-redis-jackson key 结构（社区源码镜像）

### Tertiary (LOW confidence — 需用户确认)
- Nacos craft-satoken.yaml / craft-shared.yaml 实际配置值（仓库不可见）
- 生产 MySQL DDL / collation

---
*Research completed: 2026-10-31*
*Ready for roadmap: yes*
