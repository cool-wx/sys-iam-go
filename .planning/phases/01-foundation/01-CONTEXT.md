# Phase 1: 工程骨架与横切地基 - Context

**Gathered:** 2026-10-31
**Status:** Ready for planning

<domain>
## Phase Boundary

交付一个在工控机环境可构建、部署、启动、优雅关停的单体 Go 服务（Gin + GORM + koanf + slog，`CGO_ENABLED=0` 静态单二进制 + systemd unit），并把全库共享的横切约定一次性定死：统一 R 响应与错误码分段、雪花 ID 生成与 JSON 字符串化、时间/null 键/枚举序列化契约、指针 DTO 零值保护、审计字段自动填充、分页结构、健康检查与优雅关停、本地 YAML + 环境变量配置。

验证载体为 sys_user 表四件套最小接口（create/update/list/get）。不含：认证会话（Phase 2）、Casbin 授权（Phase 3）、任何其他业务域。

</domain>

<decisions>
## Implementation Decisions

### 骨架自验证方式（用户确认）
- **D-01:** 以真实 `sys_user` 表的四件套接口（create/update/list/get）作为横切约定的端到端验证载体，全部 5 条成功标准经真实接口证明；接口保留为 Phase 2 User 域起点，Phase 2 在此之上补业务规则（分页钳位 1/15、builtin 删除拦截、详情两查询装配、Excel 导入等）— **Reversibility:** costly — 四件套成为 Phase 2 的代码基础，推翻需重写 handler/service/repo 三层与路由
- **D-02:** Phase 1 的验证接口无鉴权（认证中间件属 Phase 2），路由全白名单直通；Phase 2 加中间件时再收紧

### Schema 来源（Claude 自主决策，调研支撑）
- **D-03:** 本机 `sysiam-mysql` 容器（mysql:8.4，库 `sys_iam`）实测为**空库（0 表）**，无存量 schema 可核对；rnacos 配置总数为 0，无 DDL 落点。权威 schema 来源定为**源 PO 确定性推导**：`@TableName`/`@TableField` 注解 + MyBatis-Plus 驼峰→下划线规则映射（规则推导，非猜测）— **Reversibility:** one-way — 建表后列名即事实契约，后续改动需 DDL 迁移且波及全部 GORM 模型
- **D-04:** Phase 1 仅建 `sys_user` 一张表（推导 DDL 导入本机 sysiam-mysql），四件套读写实测即完成列映射校准；其余 12 实体 DDL 在各自域的 Phase 落地时同法建表，缩小单次推导误差面
- **D-05:** 生产库存量 BCrypt hash 抽样验证（REQUIREMENTS AUTH-01 验收项）保持开放 —— 本机无生产库，Phase 2 验收时向用户索取生产库 dump 或抽样 hash；BCrypt `$2a$` 格式兼容性已经研究多源交叉验证，风险低

### 配置与部署形态（Claude 自主决策）
- **D-06:** 配置组织为**单一 `config.yaml` + 环境变量覆盖**（koanf，前缀 `IAM_`，如 `IAM_MYSQL_DSN`），不按环境拆分多文件 —— 工控机单机无多环境复杂度，环境差异全走环境变量
- **D-07:** 配置查找顺序：`-config` flag 显式指定 > 默认 `./config.yaml`（工作目录）；systemd unit 以 `WorkingDirectory` 锚定，不引入 `/etc` 多级 fallback — **Reversibility:** costly — 部署脚本与运维文档成型后改动牵涉交付物
- **D-08:** 日志走 **slog JSON handler → stdout，由 journald 接管**（systemd `StandardOutput=journal` 自带持久化与轮转）；不引入 lumberjack 文件滚动，保安装包小
- **D-09:** systemd unit：`Type=simple` + `Restart=on-failure` + `ExecStop` 依赖 SIGTERM 触发 15s 优雅排空（ENG-06）

### 测试基建（Claude 自主决策，环境实测）
- **D-10:** 本机 Docker 29.1.3 实测可用 → Phase 1 即搭建 testcontainers-go 集成测试：独立 MySQL 容器跑四件套端到端 + 横切约定断言（雪花 ID 字符串化、零值保护、审计填充、分页 envelope、时间格式）
- **D-11:** 单元测试覆盖横切件纯逻辑（errcode 映射、snowflake 唯一性/单调性、序列化契约、审计 hook）；miniredis 留给 Phase 2 会话管理器
- **D-12:** 开发环境手工验证直接用本机 `sysiam-mysql` 容器；自动化测试一律用 testcontainers 独立容器，不污染开发库

### Claude's Discretion
用户在灰区 1 的两问之后明确授权："全部由你自己决定，后面的都不需要问我了，直到项目开发完成，该调研就调研，自动选择最优方案往下推进"。据此：D-03～D-12 为授权范围内的自主决策（均有环境调研支撑）；后续 phase 的 discuss 环节按同一授权执行自主决策，无需再逐项询问。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 项目规划
- `.planning/ROADMAP.md` §Phase 1 — Goal 与 5 条成功标准（验收锚点）
- `.planning/REQUIREMENTS.md` §工程化与横切 — ENG-01..08 需求定义
- `.planning/PROJECT.md` — Constraints 与 Key Decisions（技术栈锁定、Out of Scope）

### 研究结论（本次实现的直接输入）
- `.planning/research/ARCHITECTURE.md` — 目录映射（按层分包+按域分文件、pkg/ 只放 resp/errcode/paging）、R envelope 设计（code=0/1xxxx 分段/HTTP 如实映射）、Pattern 1-2、Anti-Pattern 2/4/5/6
- `.planning/research/PITFALLS.md` — Phase 1 相关：Pitfall 1（雪花 ID 应用层）、2（int64 字符串化）、3（零值更新）、5（JSON 契约三特征）、6（时区 loc=Asia/Shanghai）、8（审计填充+BlockAttack）、14（连接池工控机取值）、15（分页稳定性）、16（goroutine 纪律/优雅退出）
- `.planning/research/STACK.md` — 全部版本号与选型依据（Gin v1.12.0/GORM v1.31.2/koanf v2.4.1/slog 等）
- `.planning/research/SUMMARY.md` — 研究综合结论

### 源系统（schema 推导的唯一依据）
- `/home/wx/workspace/sys-iam` — 13 实体 PO 注解（sys_user 列名从此推导，见 D-03）

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- 无 —— 仓库为 greenfield（仅 `go.mod`：`github.com/cool-wx/sys-iam-go`，go 1.27.3，零 Go 源文件）；研究文档即全部前置资产

### Established Patterns
- 研究已锁定的实现模式：R envelope `{code,message,data}` + data 内含分页 `{items,total,page,size}`（废弃源顶层 total）；错误经 `*errcode.Error` 类型传播、统一出口写出；ctx 私有类型 key + FromContext 访问器；DTO 与 PO 分离（禁 GORM struct 直接出网）

### Integration Points
- **本机开发环境（调研实测，2026-10-31）**：MySQL `127.0.0.1:3306` root/root123 库 `sys_iam`（空，容器 `sysiam-mysql` mysql:8.4）；Redis `127.0.0.1:6379` 无密码（容器 `sysiam-redis`）；Docker 29.1.3（testcontainers 可用）；rnacos `127.0.0.1:8848`（空配置）；RustFS `127.0.0.1:9000`（容器 `rustfs`，Phase 4 用）

</code_context>

<specifics>
## Specific Ideas

- 横切约定的"证明标准"：不是"代码写了"而是"真表真接口端到端可观察"——create 响应回显实体证明雪花 ID 字符串化/时间格式/null 键，update 证明零值保护+审计填充，list 证明分页 envelope，get 证明序列化契约（D-01 的落地判据）
- Phase 1 建表方式本身即校验：推导 DDL → 导入空库 → GORM 读写无报错 = 列映射闭环
- sa-token timeout/active-timeout 实际值无法获取（rnacos 空、Nacos 配置不在仓库）→ Phase 2 以显式配置项 + 合理默认值替代硬编码（沿袭 D-06 配置化路线）

</specifics>

<deferred>
## Deferred Ideas

- 生产库存量 BCrypt hash 抽样验证（20 个 hash 用 Go bcrypt 验证）— Phase 2 验收开放项，届时向用户索取（AUTH-01）
- sa-token 会话超时实际值考证 — Phase 2 用配置项默认值替代（原 STATE.md Blocker 就此转性）
- 其余 12 实体建表 DDL — 各域 Phase 落地时逐表推导（D-04）

None beyond the above — discussion stayed within phase scope.

</deferred>

---

*Phase: 1-工程骨架与横切地基*
*Context gathered: 2026-10-31*
