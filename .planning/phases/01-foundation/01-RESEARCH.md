# Phase 1: 工程骨架与横切地基 - Research

**Researched:** 2026-10-31
**Domain:** Go 单体服务骨架（Gin + GORM + koanf + slog，CGO_ENABLED=0 静态单二进制 + systemd）+ 全库横切约定（R 响应/雪花 ID/JSON 契约/零值保护/审计填充/分页/健康检查/本地配置）
**Confidence:** HIGH（源码语义逐文件读取；全部 Phase 1 关键 API 经本机 scratch 模块编译+运行实证；版本号经 Go module proxy 实时核验）

> **本次研究的新增实证方式说明：** 除阅读既有研究文档外，本次研究在 `/tmp/gsd-scratch` 建立了真实 Go 1.27.3 模块，安装全部 Phase 1 依赖并**编译+运行**了横切约定的关键代码路径（koanf 配置链、JSON 契约类型、雪花并发唯一性、GORM 连真实 MySQL 8.4、CGO_ENABLED=0 静态链接）。文中以 `[VERIFIED: scratch 运行]` 标注这类一手实证。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions（Implementation Decisions，逐字复制自 01-CONTEXT.md）

- **D-01:** 以真实 `sys_user` 表的四件套接口（create/update/list/get）作为横切约定的端到端验证载体，全部 5 条成功标准经真实接口证明；接口保留为 Phase 2 User 域起点，Phase 2 在此之上补业务规则（分页钳位 1/15、builtin 删除拦截、详情两查询装配、Excel 导入等）— **Reversibility:** costly
- **D-02:** Phase 1 的验证接口无鉴权（认证中间件属 Phase 2），路由全白名单直通；Phase 2 加中间件时再收紧
- **D-03:** 本机 `sysiam-mysql` 容器（mysql:8.4，库 `sys_iam`）实测为**空库（0 表）**，无存量 schema 可核对；rnacos 配置总数为 0，无 DDL 落点。权威 schema 来源定为**源 PO 确定性推导**：`@TableName`/`@TableField` 注解 + MyBatis-Plus 驼峰→下划线规则映射（规则推导，非猜测）— **Reversibility:** one-way ⚠️ **本次研究实测该前提已过时（见 §Schema 现状突变）**
- **D-04:** Phase 1 仅建 `sys_user` 一张表（推导 DDL 导入本机 sysiam-mysql），四件套读写实测即完成列映射校准；其余 12 实体 DDL 在各自域的 Phase 落地时同法建表
- **D-05:** 生产库存量 BCrypt hash 抽样验证（REQUIREMENTS AUTH-01 验收项）保持开放 —— 本机无生产库，Phase 2 验收时向用户索取
- **D-06:** 配置组织为**单一 `config.yaml` + 环境变量覆盖**（koanf，前缀 `IAM_`，如 `IAM_MYSQL_DSN`），不按环境拆分多文件
- **D-07:** 配置查找顺序：`-config` flag 显式指定 > 默认 `./config.yaml`（工作目录）；systemd unit 以 `WorkingDirectory` 锚定，不引入 `/etc` 多级 fallback — **Reversibility:** costly
- **D-08:** 日志走 **slog JSON handler → stdout，由 journald 接管**（systemd `StandardOutput=journal` 自带持久化与轮转）；不引入 lumberjack
- **D-09:** systemd unit：`Type=simple` + `Restart=on-failure` + `ExecStop` 依赖 SIGTERM 触发 15s 优雅排空（ENG-06）
- **D-10:** 本机 Docker 29.1.3 实测可用 → Phase 1 即搭建 testcontainers-go 集成测试：独立 MySQL 容器跑四件套端到端 + 横切约定断言
- **D-11:** 单元测试覆盖横切件纯逻辑（errcode 映射、snowflake 唯一性/单调性、序列化契约、审计 hook）；miniredis 留给 Phase 2 会话管理器
- **D-12:** 开发环境手工验证直接用本机 `sysiam-mysql` 容器；自动化测试一律用 testcontainers 独立容器，不污染开发库

### Claude's Discretion（授权）

用户在灰区 1 的两问之后明确授权："全部由你自己决定，后面的都不需要问我了，直到项目开发完成，该调研就调研，自动选择最优方案往下推进"。D-03～D-12 为授权范围内的自主决策；后续 phase 的 discuss 环节按同一授权执行自主决策。

### Deferred Ideas (OUT OF SCOPE)

- 生产库存量 BCrypt hash 抽样验证 — Phase 2 验收开放项
- sa-token 会话超时实际值考证 — Phase 2 用配置项默认值替代
- 其余 12 实体建表 DDL — 各域 Phase 落地时逐表推导（D-04）

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ENG-01 | 统一 R 响应 {code,message,data} + 错误码分段 + HTTP 状态如实映射 | ARCHITECTURE.md Pattern 1 envelope 设计 + §Pattern 代码示例（errors.As 统一出口）；errcode 分段表直接沿用 |
| ENG-02 | 审计字段自动填充（create_time/update_time/create_by/update_by，无操作人跳过） | 源 AutoFillHandler.java:46-66 语义逐行读取；GORM BeforeCreate/BeforeUpdate + tx.Statement.Context 机制 [CITED: gorm.io/context] + scratch 编译实证 |
| ENG-03 | 雪花 ID 应用层生成（DB 无自增）+ int64 JSON 字符串化 | 真实 DDL 证实 `id bigint NOT NULL` 无自增；bwmarrin/snowflake 16k 并发唯一性 [VERIFIED: scratch 运行]；自定义 ID 类型 Marshal/Unmarshal 双向兼容 [VERIFIED: scratch 运行] |
| ENG-04 | JSON 契约：时间无时区格式、null 键照常输出、枚举 name|int 兼容未知兜底 | 源 JacksonConfig.java:33-48 契约逐行读取；三种枚举输入形态全部通过 [VERIFIED: scratch 运行] |
| ENG-05 | GORM 写路径零值保护（指针 DTO + 显式 Select，禁裸 Save） | GORM Updates(map)/Select 语义 [CITED: gorm.io/finisher-api]；ErrMissingWhereClause 全表防护；指针 DTO nil=未提供语义 |
| ENG-06 | /healthz /readyz + 优雅关停（SIGTERM 15s 排空） | Gin 官方 graceful shutdown 范式 [CITED: gin docs/doc.md]；systemd TimeoutStopSec 须大于排空时间 [CITED: freedesktop systemd.service] |
| ENG-07 | 配置本地 YAML + 环境变量（koanf） | koanf v2.4.1 全链（file→yaml→env/v2 覆盖）[VERIFIED: scratch 运行]；env/v2 新 API（env.Opt）为当前形态 |
| ENG-08 | 单二进制交叉编译（CGO_ENABLED=0）+ systemd unit | 全依赖集 CGO_ENABLED=0 构建产物 ` statically linked` [VERIFIED: scratch 运行]；`time/tzdata` 内嵌时区库为工控机必备（见 Pitfall P9） |

</phase_requirements>

## Summary

Phase 1 是全库共享横切地基：一个可构建/部署/优雅关停的 Gin 单体 + 定死 R 响应、雪花 ID、JSON 序列化契约、零值保护、审计填充、分页、健康检查、本地配置等约定，以 sys_user 四件套接口做端到端证明。核心技术路线在 ARCHITECTURE.md/PITFALLS.md/STACK.md（2026-10-31）已完整给出，本次研究**不改变任何既有选型**，而是：(1) 把关键 API 全部落到本机 Go 1.27.3 + MySQL 8.4 真实编译运行验证；(2) 补齐 Phase 1 特有的落地细节（koanf env/v2 新 API、time/tzdata 内嵌、systemd TimeoutStopSec、GORM TranslateError、雪花库选型定案）；(3) 处理两处**环境突变**——schema 前提已变、开发容器已被替换。

**最重要发现一（schema）：** D-03 记录的"sys_iam 空库"前提已过时——研究实测 `sys_iam` 现有 **15 张表**（DDL-only 导入，行数 0），且真实 `sys_user` 表与源 PO 存在一处偏差：**缺 `builtin` 列**（源 `User.java:62-64` 有该字段且 `create()` 恒写入 → 生产库必有该列 → 本地导入 DDL 不完整）。全库对照还发现 `sys_role` 缺 `builtin`、`sys_file` 缺 `file_hash`、`sys_menu` 多出 `status`/`visible` 两列（后续 Phase 处理）。**Phase 1 动作：`ALTER TABLE sys_iam.sys_user ADD COLUMN builtin tinyint(1) DEFAULT NULL AFTER first_login`**（本地空表零风险），随后 21+1 列映射即闭环。

**最重要发现二（环境）：** 研究进行中本机容器被并发会话重建：`sysiam-mysql`/`sysiam-redis` 已被移除，出现 `mysql-server`(mysql:8.4.8)/`redis-server`(redis:8.6.2)（凭据未知，疑似无关基础设施）。CONTEXT.md 的 Integration Points 快照已失效。**规划含义：** 执行期第一步必须重新探测环境；自动化测试走 testcontainers（D-10）天然免疫此类漂移；不要把任何测试/启动依赖锚定在具名容器上。

**最重要发现三（实证补充）：** scratch 全链验证确认了四个此前只是"文档置信"的关键点——koanf env 覆盖链用 **`providers/env/v2` 的新 `env.Opt` API**（STACK.md 安装行写的 `providers/env@latest` 旧路径仍可用但 API 形态不同）；自定义时间类型若进 GORM PO **必须实现 `driver.Valuer`/`sql.Scanner`**（只写 JSON 方法时 Create 直接报 `unsupported type` ——本次实证踩中）；`encoding/json/v2` 在 **Go 1.27 已转正为标准库**（本机 `go doc` 可解析、GOEXPERIMENT 为空），v1 API（gin c.JSON 所用）仍受支持，但契约测试是防线；`CGO_ENABLED=0` 构建含全部依赖集产出真静态 ELF。

**Primary recommendation:** 按 ARCHITECTURE.md 既定骨架实现，schema 以真实 DDL 为准 + 补 `builtin` 列；执行期第一步重探环境；PO 层用原生 Go 类型（time.Time/int64/int8），自定义序列化类型（ID/LocalDateTime/枚举）只放 DTO 层——让 GORM 永远看不到自定义类型，从根上消除 Valuer/Scanner 陷阱；systemd unit 显式 `TimeoutStopSec=30` + main.go `import _ "time/tzdata"`。

## Project Constraints (from CLAUDE.md)

以下指令提取自 `.claude/CLAUDE.md`（项目级），规划与执行必须遵守：

- **技术栈锁定**：Go 1.27 + Gin + GORM，module 名 `github.com/cool-wx/sys-iam-go`
- **架构**：Go 标准布局（cmd/internal/pkg），禁 DDD 四层目录
- **数据库**：MySQL，schema 沿用源项目不重设计
- **基础设施**：保留 Redis（Phase 2）；去 Nacos；文件存储可配置（Phase 4）
- **API 契约**：允许重新设计（不受源 REST 契约约束）
- **部署**：工控机单机零外部服务依赖，单二进制交付（`CGO_ENABLED=0`）
- **What NOT to Use**（STACK.md 锁定）：JWT、gorilla/sessions、大而全工具库、任何 CGO 依赖、GORM AutoMigrate 常开、Nacos/Dubbo 等价物、重量级事件总线
- **GSD Workflow Enforcement**：仓库改动须经 GSD 工作流（本任务即 GSD research 环节，合规）
- **用户全局**：回复使用中文

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| HTTP 服务生命周期（启动/SIGTERM 优雅关停/15s 排空） | API/Backend（`cmd/server` main + `internal/app`） | systemd（进程监督） | Go 进程自持 Shutdown 逻辑，systemd 只发信号与超时兜底 |
| R 响应 envelope + errcode + HTTP 映射 | API/Backend（`pkg/resp` + `pkg/errcode`） | — | 全部端点共用出口，属进程内横切 |
| JSON 序列化契约（ID 字符串/时间格式/null 键/枚举） | API/Backend（`internal/model/dto` 自定义类型） | — | DTO 层独占（PO 用原生类型，见 §Pattern 1） |
| 雪花 ID 生成 | API/Backend（`pkg/ids` 或 service 层 helper） | Database/Storage（bigint 落库） | DB 无自增（真实 DDL 证实），应用层负责 |
| 零值保护更新（指针 DTO + Select） | API/Backend（DTO 定义 + repository helper） | — | GORM 语义正确使用即可，无 DB 侧改动 |
| 审计字段自动填充 | API/Backend（GORM hooks + ctx identity） | — | hook 读 `tx.Statement.Context` |
| /healthz /readyz | API/Backend（`internal/health`，public 路由组） | systemd/k3s 探针（消费方） | 进程内 MySQL ping；Phase 1 无 Redis 依赖 |
| 配置加载（YAML+env） | API/Backend（`internal/config`） | — | koanf 本地文件，无外部服务 |
| 访问日志/RequestID/Recovery 中间件 | API/Backend（`internal/middleware`） | journald（stdout 汇聚，D-08） | slog JSON → stdout |
| 静态单二进制构建 + systemd unit | 构建/部署层（Makefile + `deploy/`） | — | 纯构建产物，无运行时角色 |
| sys_user DDL 与表 | Database/Storage（本机 MySQL） | API/Backend（PO 映射） | 建表是 DB 侧事实，映射校准靠四件套（D-01） |

## Schema 现状突变（必读——D-03 前提已过时）

### 实测时间线（2026-10-31 本次研究会话）

1. CONTEXT.md（今日早些时候）记录 `sys_iam` 空库（0 表）→ D-03 据此定"PO 推导"为权威。
2. 本次研究实测：`sys_iam` 已有 **15 张表，全部 0 行**（DDL-only 导入）。表清单：sys_api_key, sys_config, sys_dept, sys_dict_data, sys_dict_type, sys_fault_info, sys_file, sys_file_category, sys_menu, sys_operation_log, sys_role, sys_role_menu, sys_shift, sys_user, sys_user_role `[VERIFIED: docker exec SHOW TABLES]`。
3. 研究后半段，**数据**开始出现（sys_menu 41 行、sys_user 1 行 admin、sys_role 1 行等）——另一会话正在导入种子数据。种子行特征：手工小 ID（admin id=1，非雪花）、create_time 为 UTC 墙钟样值（07:18:18 vs 本地 15:18）`[VERIFIED: docker exec SELECT]`。
4. 随后 `sysiam-mysql`/`sysiam-redis` 容器被**移除**，出现 `mysql-server`(mysql:8.4.8)/`redis-server`(redis:8.6.2)（Up 数分钟，凭据未知）`[VERIFIED: docker ps 前后对照]`。

### sys_user 真实 DDL（权威列事实，逐字）

`SHOW CREATE TABLE sys_iam.sys_user` 输出（注释乱码系导入管道编码损伤，列名/类型为准）`[VERIFIED: docker exec SHOW CREATE TABLE，2026-10-31]`：

```sql
CREATE TABLE `sys_user` (
  `id` bigint NOT NULL,
  `username` varchar(50) DEFAULT NULL,
  `password` varchar(100) DEFAULT NULL,
  `real_name` varchar(50) DEFAULT NULL,
  `avatar` varchar(255) DEFAULT NULL,
  `gender` tinyint DEFAULT NULL,
  `phone` varchar(20) DEFAULT NULL,
  `email` varchar(100) DEFAULT NULL,
  `openid` varchar(100) DEFAULT NULL,
  `ext_token` varchar(512) DEFAULT NULL,
  `dept_id` bigint DEFAULT NULL,
  `dept_name` varchar(50) DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `first_login` tinyint(1) DEFAULT NULL,
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  KEY `idx_dept_id` (`dept_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci
```

关键事实：`id` **无 AUTO_INCREMENT**（证实 Pitfall 1 雪花应用层生成）；`username` 有唯一索引（重复创建会 1062 → 建 errcode 映射，见 §Pitfall P4）；除 id/status 外全部 DEFAULT NULL；utf8mb4 + utf8mb4_general_ci（回答了 PITFALLS.md 的 collation 信息缺口 `[VERIFIED]`）。

### PO ↔ DB 全库对照结果（本次系统性 diff）

| 表 | 差异 | 影响 Phase |
|----|------|-----------|
| **sys_user** | **缺 `builtin` 列**（PO `User.java:62-64`：`private Boolean builtin;` 无 `@TableField(exist=false)`；`create()` 恒写入 `user.setBuiltin(...)`，生产库必有此列否则源系统插入即报错 → 导入 DDL 不完整）`[VERIFIED: User.java + SHOW CREATE]` | **Phase 1（本表即验证载体）** |
| sys_role | 缺 `builtin` 列（Role 同为 builtin 守卫对象） | Phase 2 |
| sys_file | 缺 `file_hash` 列（PO `FileInfo.java`：`private String fileHash;`，FILE-01 MD5 判重依赖它） | Phase 4 |
| sys_menu | 多出 `status`、`visible` 两列（PO 无对应字段；GORM 只映射声明字段，读无害） | Phase 2（仅备注） |
| 其余 9 表 | 列完全一致（含 4 个审计列） | — |
| sys_user_role / sys_role_menu / sys_operation_log | 无差异（PO 只继承 BaseAggregate，本来就不该有审计列） | — |

**Phase 1 规定的修复动作**（本地空表，零风险，执行期任务之一）：

```sql
ALTER TABLE sys_iam.sys_user ADD COLUMN builtin tinyint(1) DEFAULT NULL AFTER first_login;
```

修正后 22 列与 PO 完全一致，四件套读写即完成列映射闭环（D-01/D-04 判据）。执行时若容器又变了，用 testcontainers 容器执行同一 DDL 同样成立（D-10 路线不受影响）。

## Standard Stack

### Core（Phase 1 实际安装集——经 Go module proxy 核验 @latest 一致）

| Library | Version | Purpose | Why Standard | Confidence |
|---------|---------|---------|--------------|------------|
| Go | 1.27.3（本机实装） | 语言运行时 | 用户指定；本机 `go version` = go1.27.3 `[VERIFIED]` | HIGH |
| gin-gonic/gin | v1.12.0 | HTTP 框架 | proxy @latest 一致；scratch 编译通过；官方 graceful shutdown 范式 [CITED: gin doc.md] | HIGH |
| gorm.io/gorm | v1.31.2 | ORM | proxy @latest 一致；Tabler/Select/TranslateError 均实证（见 Pitfall/Pattern 节） | HIGH |
| gorm.io/driver/mysql | v1.6.0 | MySQL 方言 | proxy @latest 一致；DSN 全参数本机 8.4 实连通过 [VERIFIED: scratch 运行] | HIGH |
| knadh/koanf/v2 | v2.4.1 | 配置（YAML+env） | proxy @latest 一致；全链 scratch 运行通过 [VERIFIED] | HIGH |
| knadh/koanf/providers/env/v2 | v2.0.2 | env 覆盖 provider | **新 API**（env.Opt 结构）；官方 README 现行形态 [CITED: koanf README] + [VERIFIED: scratch 运行] | HIGH |
| knadh/koanf/providers/file | v1.2.1 | YAML 文件 provider | proxy 解析结果 [VERIFIED] | HIGH |
| knadh/koanf/parsers/yaml | v1.1.1 | YAML parser | proxy 解析结果 [VERIFIED] | HIGH |
| bwmarrin/snowflake | v0.3.0 | 雪花 ID 生成 | **定案**：pkg.go.dev 权威索引（repo github.com/bwmarrin/snowflake，BSD-2，稳定完成态，1641 个包引用，内建 monotonic 防钟漂 + MarshalJSON）[CITED: pkg.go.dev/bwmarrin/snowflake] + [VERIFIED: scratch 运行 16k 并发零重复] | HIGH |

### Supporting（Phase 1 使用）

| Library | Version | Purpose | When to Use | Confidence |
|---------|---------|---------|-------------|------------|
| log/slog（标准库） | Go 1.21+ | 结构化 JSON 日志 | D-08：JSON handler → stdout；访问日志中间件**手写**（需注入 requestID/userID 自定义字段，slog-gin 默认输出不含，自定义后复杂度等同——故 Phase 1 不装 samber/slog-gin，偏离 STACK.md 之处已注明） | HIGH |
| stretchr/testify | v1.12.1 | 测试断言 | proxy @latest 一致；require/assert 分层 | HIGH |
| testcontainers-go | v0.44.0 | MySQL 集成测试容器 | proxy @latest 一致；MySQL 模块 `mysql.Run(ctx, img, ...)` 为 v0.20+ 现行 API [CITED: testcontainers-go docs/modules/mysql.md] | HIGH |
| time/tzdata（标准库） | Go 1.27 stdlib | 内嵌时区库 | `import _ "time/tzdata"`——CGO_ENABLED=0 静态二进制在无 /usr/share/zoneinfo 的工控机上仍能解析 Asia/Shanghai `[VERIFIED: go doc time/tzdata]` | HIGH |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| bwmarrin/snowflake | sony/sonyflake v1.3.0 | sonyflake 位数布局不同（46ms 时间片），序列化/解析生态弱于 bwmarrin；bwmarrin 为 Twitter 兼容经典布局，scratch 实证 16k 并发零重复。定案 bwmarrin |
| 手写访问日志中间件 | samber/slog-gin v1.21.1 | slog-gin 默认字段不含 ctx 里的 userID/requestID；要加就得自定义 formatter，复杂度≈手写 30 行。Phase 1 手写（ARCHITECTURE.md Pattern 5 即此形态）；后续想要再换不迟 |
| koanf | spf13/viper | STACK.md 已裁决 koanf（依赖树小）；env/v2 新 API 实证无阻碍 |
| encoding/json v1 API | encoding/json/v2（Go 1.27 已转正） | v2 有更严默认值与语义差异；gin c.JSON 走 v1 API。**不主动迁 v2**，用契约测试锁行为即可（见 State of the Art） |

**Installation（Phase 1 实际安装命令）：**

```bash
go get github.com/gin-gonic/gin@v1.12.0
go get gorm.io/gorm@v1.31.2 gorm.io/driver/mysql@v1.6.0
go get github.com/knadh/koanf/v2@v2.4.1
go get github.com/knadh/koanf/providers/file@v1.2.1 \
       github.com/knadh/koanf/providers/env/v2@v2.0.2 \
       github.com/knadh/koanf/parsers/yaml@v1.1.1
go get github.com/bwmarrin/snowflake@v0.3.0
go get github.com/stretchr/testify@v1.12.1
go get github.com/testcontainers/testcontainers-go@v0.44.0
# 明确不装（属后续 Phase）：go-redis、miniredis、excelize、minio-go、slog-gin、swaggo、prometheus
```

**Version verification:** 全部版本经 `go list -m -versions <module>` 实时核验，与 STACK.md 钉住版本完全一致，无漂移（2026-10-31）。

## Package Legitimacy Audit

> gsd-tools package-legitimacy check 仅覆盖 npm/pypi/crates 生态；Go 生态的等价权威路径为 **Go module proxy + pkg.go.dev**（Step 2 协议的生态对应命令）。全部 Phase 1 依赖按此验证。

| Package | Registry | Age | Downloads（替代指标） | Source Repo | Verdict | Disposition |
|---------|----------|-----|----------------------|-------------|---------|-------------|
| github.com/gin-gonic/gin | Go proxy / pkg.go.dev | 10+ yrs | Go 生态最广泛 HTTP 框架 | github.com/gin-gonic/gin | OK | Approved |
| gorm.io/gorm | Go proxy / pkg.go.dev | 9+ yrs | Go 最主流 ORM | github.com/go-gorm/gorm | OK | Approved |
| gorm.io/driver/mysql | Go proxy | 5+ yrs | GORM 官方方言 | github.com/go-gorm/mysql | OK | Approved |
| github.com/knadh/koanf/v2（+file/env-v2/yaml） | Go proxy / pkg.go.dev | 6+ yrs | 官方 README 现行文档齐 | github.com/knadh/koanf | OK | Approved |
| github.com/bwmarrin/snowflake | Go proxy / pkg.go.dev | 2019 起稳定 | **1641 个包引用**；README 声明"stable and completed" | github.com/bwmarrin/snowflake（BSD-2） | OK | Approved |
| github.com/stretchr/testify | Go proxy | 10+ yrs | Go 测试事实标准 | github.com/stretchr/testify | OK | Approved |
| github.com/testcontainers/testcontainers-go | Go proxy / pkg.go.dev | 4+ yrs | 官方文档 MySQL 模块齐全 | github.com/testcontainers/testcontainers-go | OK | Approved |

**Packages removed due to [SLOP] verdict:** none（无 hallucinated 包名——全部经权威索引解析且源仓可溯）
**Packages flagged as suspicious [SUS]:** none
**验证方法备注：** 所有包在 scratch 模块中 `go get` + `go build` + 部分运行成功（2026-10-31），注册表解析、源码可得性、许可均实证。

## Architecture Patterns

### System Architecture Diagram

```
                    systemd (工控机单机 / 开发机验证)
                      │ ExecStart(绝对路径)      │ SIGTERM → TimeoutStopSec=30 兜底 SIGKILL
                      ▼                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│ cmd/server/main.go                                                  │
│   1. -config flag / ./config.yaml → koanf(file→yaml, env IAM_* 覆盖) │
│   2. slog JSON handler → stdout（journald 接管，D-08）                │
│   3. GORM 接 MySQL（DSN: charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai）│
│   4. http.Server{Handler: gin engine} ListenAndServe（goroutine）     │
│   5. signal.NotifyContext(SIGINT, SIGTERM)                           │
│      └─ 收到 SIGTERM：readyz 置 false → srv.Shutdown(15s ctx)         │
│                    → 关 sqlDB（逆序）→ 进程退出 0                     │
├─────────────────────────────────────────────────────────────────────┤
│ Gin 中间件链（Phase 1 全部 public，D-02）                             │
│   RequestID → AccessLog(slog) → Recovery(envelope 500)               │
│   ├─ /healthz（liveness：进程在即 200）                               │
│   ├─ /readyz（readiness：MySQL ping）                                │
│   └─ /api/v1/users（sys_user 四件套，验证载体）                        │
├─────────────────────────────────────────────────────────────────────┤
│ handler（绑定 DTO+校验）→ service（事务边界）→ repository（GORM）       │
│   审计 hook 读 tx.Statement.Context（无 identity → 跳过操作人字段）     │
│   BeforeCreate 里 snowflake 生成 ID（DB 无自增）                       │
├─────────────────────────────────────────────────────────────────────┤
│ MySQL（sys_iam.sys_user，22 列真实 DDL + builtin 补列）                │
└─────────────────────────────────────────────────────────────────────┘
```

主用例追踪：systemd ExecStart → main 装配 → HTTP 监听 → POST /api/v1/users 进中间件链 → handler 绑定指针 DTO → service 开事务 → repo Create 触发 BeforeCreate（雪花 ID + 时间 + 审计人）→ INSERT → 响应 `{code:0,...}`，DTO 内 id 为字符串、时间为 `yyyy-MM-dd HH:mm:ss`、null 键照常输出。

### Recommended Project Structure（Phase 1 子集，全库结构见 ARCHITECTURE.md）

```
sys-iam-go/
├── cmd/server/main.go           # 入口：config→装配→ListenAndServe→优雅关停
├── internal/
│   ├── app/app.go, router.go    # App 聚合 + 路由（Phase 1 单 public 组）
│   ├── config/config.go         # Config struct + koanf Load（file→env/v2 覆盖）
│   ├── handler/user.go          # sys_user 四件套（验证载体，Phase 2 User 域起点）
│   ├── service/user.go          # 事务边界 + 业务校验（username 唯一 → errcode）
│   ├── repository/user.go       # GORM 薄封装 + Patch helper（零值保护）
│   ├── model/
│   │   ├── po/user.go           # 原生 Go 类型 + 显式 column tag + TableName
│   │   └── dto/user.go          # 指针字段请求 DTO / 自定义类型响应 DTO
│   ├── middleware/requestid.go, access_log.go, recovery.go
│   └── health/health.go         # /healthz /readyz + SetNotReady()
├── pkg/
│   ├── resp/resp.go             # OK/Fail/PageOK envelope 出口
│   ├── errcode/errcode.go       # *Error{Code,Msg} + HTTPStatus() + 分段常量
│   ├── ids/id.go                # type ID int64（Marshal/Unmarshal 字符串化）
│   ├── types/datetime.go        # type LocalDateTime（仅 JSON 方法）
│   ├── types/enum.go            # UsageStatus/Gender 四件套（DTO 侧）
│   └── paging/paging.go         # Parse 钳位（缺省 1/15，≥1）
├── configs/config.yaml          # 默认配置（部署旁路放置，D-07）
├── deploy/sys-iam-go.service    # systemd unit
├── scripts/ddl/sys_user.sql     # 真实 DDL + builtin 补列（幂等导入脚本）
├── internal/repository/user_test.go        # testcontainers 集成测试
├── pkg/.../*_test.go                        # 横切件单元测试（D-11）
├── Makefile                      # build/test/cross-compile 目标
└── go.mod                        # module github.com/cool-wx/sys-iam-go（已存在）
```

### Pattern 1: DTO 层自定义序列化类型（PO 用原生类型）——本次研究的核心修正

**What:** `ID`/`LocalDateTime`/枚举这类契约类型**只出现在 DTO 层**，PO 层一律用 `int64`/`*time.Time`/`int8` 原生类型；PO→DTO 手写转换处完成 `fromValue` 未知值兜底。
**When to use:** Phase 1 定义全部横切类型时一次定死。
**Why（实证依据）:** scratch 首轮运行踩中——仅实现 JSON 方法的自定义类型进 GORM PO 后 `Create` 直接报 `sql: converting argument $19 type: unsupported type main.LocalDateTime, a struct` `[VERIFIED: scratch 运行]`。给自定义类型补 `driver.Valuer`/`sql.Scanner` 可解（四件套模式），但 PO 层原生类型让该陷阱**根本不存在**，且 GORM 对 `time.Time` 的 DATETIME 读写由 `loc=Asia/Shanghai` 统一管墙钟——时区语义只依赖 DSN 一处。PO 与 DTO 分离本就是已定模式（ARCHITECTURE.md Anti-Pattern 6），此设计是其自然延伸。

```go
// pkg/ids/id.go —— 雪花 ID 契约类型（仅 JSON 面）
type ID int64

func (i ID) MarshalJSON() ([]byte, error) {
    return []byte(`"` + strconv.FormatInt(int64(i), 10) + `"`), nil
}
// UnmarshalJSON：接受 "456" 与 456 双形态（源 Jackson 长整双兼容语义）
func (i *ID) UnmarshalJSON(b []byte) error {
    s := strings.Trim(string(b), `"`)
    if s == "null" { *i = 0; return nil }
    v, err := strconv.ParseInt(s, 10, 64)
    if err != nil { return fmt.Errorf("invalid id %q", s) }
    *i = ID(v); return nil
}

// pkg/types/datetime.go —— 时间契约类型（仅 JSON 面；PO 用 *time.Time）
type LocalDateTime time.Time

const TimeFormat = "2006-01-02 15:04:05" // 源 JacksonConfig DATE_TIME_FORMAT 等价

func (t LocalDateTime) MarshalJSON() ([]byte, error) {
    return []byte(`"` + time.Time(t).In(CST()).Format(TimeFormat) + `"`), nil
}
func CST() *time.Location { loc, _ := time.LoadLocation("Asia/Shanghai"); return loc }

// pkg/types/enum.go —— 枚举四件套在 DTO 侧（Marshal→name；Unmarshal→name|int；未知兜底）
type UsageStatus int
const (
    UsageDisabled UsageStatus = 0 // 源 UsageStatus.java:14 [VERIFIED]
    UsageEnabled  UsageStatus = 1 // 源 UsageStatus.java:15 [VERIFIED]
)
func (s *UsageStatus) UnmarshalJSON(b []byte) error {
    str := strings.Trim(string(b), `"`)
    switch str {
    case "ENABLED", "1":  *s = UsageEnabled
    case "DISABLED", "0": *s = UsageDisabled
    default:              *s = UsageEnabled // 未知值兜底=ENABLED，对齐源 fromValue [VERIFIED: UsageStatus.java:26-31]
    }
    return nil
}
```

`[VERIFIED: scratch 运行]`——以上三类型的行为断言全部通过（含 null 键保留：响应 DTO 不加 omitempty，指针字段 nil 输出 `"key":null`）。

### Pattern 2: koanf 配置链（file → env/v2 覆盖）

**What:** 单一 config.yaml + `IAM_` 前缀环境变量覆盖（D-06/D-07）。
**注意（版本陷阱）:** STACK.md 安装行的 `koanf/providers/env@latest` 是**旧 API**（`env.Provider(prefix, delim, cb)` 三参数）；现行官方形态是 **`providers/env/v2`** 子包 + `env.Provider(delim, env.Opt{...})` `[CITED: koanf 官方 README][VERIFIED: scratch 运行]`。

```go
// internal/config/config.go —— [VERIFIED: scratch 运行全链 PASS]
k := koanf.New(".")
_ = k.Load(file.Provider(cfgPath), yaml.Parser())          // 1. YAML 文件
_ = k.Load(env.Provider(".", env.Opt{                       // 2. env 覆盖（后加载者胜）
    Prefix: "IAM_",
    TransformFunc: func(s, v string) (string, any) {
        // IAM_MYSQL_DSN → mysql.dsn
        return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "IAM_")), "_", "."), v
    },
}), nil)
k.MustInt("server.port")  // IAM_SERVER_PORT=18080 覆盖 yaml 的 8080 —— 实测生效
```

### Pattern 3: 审计填充 hook（读 Statement.Context，无 identity 跳过操作人）

**What:** `BeforeCreate` 生成雪花 ID + 双时间 + 四审计列；`BeforeUpdate` 刷更新时间 + 更新人；identity 从 `tx.Statement.Context` 取，无则跳过操作人字段（保留 NULL），时间照填——与源 AutoFillHandler 逐字对齐 `[VERIFIED: AutoFillHandler.java:46-66（无上下文 return，保持 null）]`。

```go
// internal/model/po/base.go（形态示意；PO 字段用原生类型）
func (u *UserPO) BeforeCreate(tx *gorm.DB) error {
    u.ID = snowflakeGen.NewID()                  // DB 无自增 [VERIFIED: 真实 DDL id bigint NOT NULL]
    now := time.Now()
    u.CreateTime, u.UpdateTime = &now, &now
    if id, ok := auth.FromContext(tx.Statement.Context); ok {
        u.CreateByID, u.CreateByName = &id.UserID, &id.Name
        u.UpdateByID, u.UpdateByName = &id.UserID, &id.Name
    } // 无 identity（后台任务 ctx=Background）→ 审计人保持 NULL
    return nil
}
```

`Statement.Context` 由 `db.WithContext(ctx)` 注入（= `Session{Context: ctx}`）`[CITED: gorm.io Context/Session 文档][VERIFIED: scratch 编译通过]`。

### Pattern 4: 零值保护更新（指针 DTO → map → 显式 Select）

**What:** 请求 DTO 全指针字段（nil=未提供），service 组装 `map[string]any` 仅含显式提供键，repo 用 `Select(cols).Updates(map)` 写出。**禁裸 Save。**
**机制依据:** GORM `Updates(struct)` 跳零值、`Updates(map)` 全写所给键、`Select(...)` 限定更新列、缺 WHERE 时 `ErrMissingWhereClause` `（AllowGlobalUpdate=false 默认）` [CITED: gorm.io finisher-api]；scratch 编译通过全链形态。
**既有真实行验证:** 更新后未提交字段保持原值、`update_by_id/update_by_name` 被 hook 填充、显式空串可清空——此三项为 D-10 集成测试断言集（scratch 运行因环境突变未跑完此段，机制均有文档+编译依据；列为**执行期验收断言**）。

### Pattern 5: 优雅关停 + systemd（15s 排空 / TimeoutStopSec=30）

**What:** Gin 官方范式 [CITED: gin doc.md "Graceful shutdown"]：`http.Server` 进 goroutine、`signal.NotifyContext(SIGINT, SIGTERM)`、`srv.Shutdown(15s ctx)`、`http.ErrServerClosed` 过滤。systemd 侧 `Type=simple`（默认）、默认 KillSignal=SIGTERM、**`TimeoutStopSec` 必须大于应用排空时间**——默认 ~90s（新版）/2min（旧版），超时即 SIGKILL `[CITED: freedesktop systemd.service man + ServerFault 928376]`。15s 排空 ⇒ `TimeoutStopSec=30`（2 倍余量）。信号感知的 Go 服务**无需 ExecStop**（D-09 措辞"ExecStop 依赖 SIGTERM 触发"即此意：不配 ExecStop 命令，靠 SIGTERM 默认信号）。

```ini
# deploy/sys-iam-go.service（骨架）
[Unit]
Description=sys-iam-go IAM service
After=network-online.target mysql.service redis.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/sys-iam-go
ExecStart=/opt/sys-iam-go/sys-iam-go -config /opt/sys-iam-go/config.yaml
Restart=on-failure
RestartSec=3
Environment=TZ=Asia/Shanghai
StandardOutput=journal
StandardError=journal
TimeoutStopSec=30
# 工控机加固（可选但推荐）
NoNewPrivileges=true
ProtectSystem=full

[Install]
WantedBy=multi-user.target
```

### Pattern 6: R envelope + errcode + HTTP 如实映射

沿用 ARCHITECTURE.md Pattern 1 全部设计（`code=0` 成功；`1xxxx` 分段——10001 内部错误/10100 未登录/10200 无权限/10300 数据不存在/10400 参数无效/10401 参数校验失败/10501 密码错误 `[VERIFIED: ARCHITECTURE.md:170 逐字]`）；`errors.As(*errcode.Error)` 统一出口；未知错误固定文案 + 原始错误只进日志（防泄漏，源 GlobalExceptionHandler 兜底语义）。`TranslateError: true` 开启后 `errors.Is(err, gorm.ErrDuplicatedKey)` 捕获 username 重复（MySQL 1062）→ 映射 10400 段 `[VERIFIED: gorm.Config 字段存在，go doc]`。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 雪花 ID 生成算法 | 手写 41/10/12 位组装 | bwmarrin/snowflake | 时钟回拨防护（monotonic）、并发序列管理、边界 case 全部内置；scratch 实证 16k 并发零重复 |
| YAML+env 配置合并 | 手写 os.Getenv 覆盖逻辑 | koanf env/v2 + file | 嵌套键展开、优先级、类型转换已解决 |
| 优雅关停信号管理 | 手写信号 channel 全套 | signal.NotifyContext + http.Server.Shutdown | 标准库组合即正确答案（Gin 官方文档范式） |
| JSON 大整数精度 | 逐字段 string 转换 | 自定义 ID 类型 MarshalJSON（一处定义全库复用） | 类型系统强制，漏不掉 |
| 时间格式化契约 | 每处手写 Format | LocalDateTime 类型（一处定义） | 同上 |
| 集成测试 MySQL 生命周期 | 手写 docker run/清理 | testcontainers-go MySQL 模块 | 端口/等待/清理自动化，对环境漂移免疫（本会话实测其必要性） |
| panic 兜底 | handler 里逐个 recover | gin CustomRecovery 中间件（envelope 500） | 全局一处 |

**Key insight:** Phase 1 的全部横切件都是"一处定义、全库继承"的契约类型与中间件——正确的代价是定义期多写几十行，错误的代价是每个域各自漂移。这正是"地基 Phase"存在的意义。

## Common Pitfalls

### P1: 自定义类型进 GORM PO 缺 Valuer/Scanner（本次实证踩中）
**What goes wrong:** 仅实现 JSON 方法的 `LocalDateTime` 放进 PO 后 `Create` 报 `unsupported type ... struct`，写入失败。
**Why:** GORM 走 `database/sql` 参数绑定，不认识自定义 struct。
**How to avoid:** **PO 层用 `*time.Time` 原生类型**（推荐，根治）；若坚持自定义类型进 PO 则必须四件套（Marshal/Unmarshal/Value/Scan）。
**Warning signs:** `sql: converting argument $N type: unsupported type`。
`[VERIFIED: scratch 运行 2026-10-31]`

### P2: 静态二进制时区库缺失（工控机 8 小时漂移复发路径）
**What goes wrong:** `CGO_ENABLED=0` 纯静态二进制在无 `/usr/share/zoneinfo` 的工控机上，`time.LoadLocation("Asia/Shanghai")` 失败回退 UTC——DSN loc 配置在 DTO 序列化侧失效，8 小时漂移以另一种方式复发。
**Why:** Go 时区解析默认依赖系统 tzdata 文件；静态交叉编译产物不带目标机文件系统假设。
**How to avoid:** main.go `import _ "time/tzdata"`（内嵌 tzdata 进二进制，约 +450KB）；同时 systemd unit 仍设 `Environment=TZ=Asia/Shanghai` 双保险。
**Warning signs:** 目标机日志出现 `unknown time zone`；DTO 时间与 DB 差 8h。
`[VERIFIED: go doc time/tzdata 存在且语义如此]`

### P3: koanf 旧 env API 安装路径
**What goes wrong:** 按 STACK.md 安装行 `go get koanf/providers/env@latest` 拿到旧 API（`env.Provider(prefix, delim, cb)`），与文档/直觉不符，编译报参数不匹配。
**How to avoid:** 用 `providers/env/v2@v2.0.2` + `env.Opt{Prefix, TransformFunc}`。
`[CITED: koanf 官方 README][VERIFIED: scratch 运行]`

### P4: username 唯一索引撞 1062 → 500
**What goes wrong:** 真实 DDL 有 `UNIQUE KEY uk_username`；重复创建走默认路径返回 500（未知错误）。
**How to avoid:** `gorm.Config{TranslateError: true}` + `errors.Is(err, gorm.ErrDuplicatedKey)` → errcode 10400 段（"记录已存在"）。service 层亦可先查后插（两查询装配语义，Phase 2 详情接口复用）。
`[VERIFIED: SHOW CREATE UNIQUE KEY + go doc gorm.Config TranslateError]`

### P5: env 覆盖不分隔导致整串键名
**What goes wrong:** TransformFunc 忘了 `_`→`.` 替换，`IAM_MYSQL_DSN` 变成顶层键 `mysql_dsn` 而非 `mysql.dsn`，覆盖静默无效。
**How to avoid:** TransformFunc 三步链（TrimPrefix→ToLower→ReplaceAll）作为固定模板（Pattern 2 代码）；单元测试断言 `k.String("mysql.dsn")`。
`[VERIFIED: scratch 运行覆盖生效]`

### P6: systemd TimeoutStopSec 小于排空时间
**What goes wrong:** 15s 排空配 `TimeoutStopSec=10` → 排空中途被 SIGKILL，"进行中请求完整返回"验收失败。
**How to avoid:** `TimeoutStopSec=30`（≥2×15s）；验收方法 `systemd-analyze verify` + 真实 `systemctl stop` 计时。
`[CITED: freedesktop systemd.service man]`

### P7: 枚举未知值 500
**What goes wrong:** `{"status": 99}` 反序列化报错或 panic → 500。
**How to avoid:** UnmarshalJSON 内兜底默认（UsageStatus→ENABLED、Gender→UNKNOWN），**永不返回 error**——源语义逐字对齐（`fromValue(int) → orElse(ENABLED)`）。
`[VERIFIED: UsageStatus.java:26-31 逐字][VERIFIED: scratch 三形态断言 PASS]`

### P8: 分页 OFFSET 负数
**What goes wrong:** `page=0`/`size=-1` 直接拼 SQL 报错 500（源是自动钳位，客户端可能依赖）。
**How to avoid:** paging.Parse 固定钳位：缺省 1/15、<1 取默认 `[VERIFIED: PageHelper.java:23-33 逐字：`page != null ? page : 1L` + `if (pageNum < 1) pageNum = 1L`，size 同构]`；契约测试覆盖 page=0/size=-1/超界页码。

### P9: 把集成测试锚在本机具名容器上
**What goes wrong:** 本会话实测 `sysiam-mysql` 研究中途被移除——任何"连 sysiam-mysql 跑测试"的计划在执行期直接失败。
**How to avoid:** 自动化测试一律 testcontainers（D-10 本就如此，实测证明其必要）；手工验证步骤在执行期先重探环境再执行；环境探测作为执行期第一个任务写进计划。
`[VERIFIED: docker ps 前后对照 2026-10-31]`

### P10: 种子数据时间基准混乱（低优先，记录在案）
**What goes wrong:** 正在导入的种子行 create_time 是 UTC 墙钟样值（07:18 vs 本地 15:18）——若手工对照种子行断言时间，会误判"漂移"。
**How to avoid:** 集成测试断言只对**自己写入**的行（testcontainers 容器内自建数据）；种子行时间基准问题留给数据导入方（非 Phase 1 契约）。
`[VERIFIED: SELECT 种子行 2026-10-31]`

## Code Examples

> 以下代码全部在 `/tmp/gsd-scratch`（Go 1.27.3 + gin v1.12.0 + gorm v1.31.2 + koanf v2.4.1 + snowflake v0.3.0）编译通过；标注"运行 PASS"的段落有真实执行证据（2026-10-31）。

### 雪花并发唯一性（运行 PASS：16000/16000 唯一，>2^53）

```go
node, _ := snowflake.NewNode(1)        // 单机固定 workerId（进 config）
id := node.Generate().Int64()          // 8 goroutine × 2000 并发实测零重复
// 断言：id > 1<<53（JS MAX_SAFE_INTEGER 之上）
```

### GORM 接入 + 连接池显式默认（Pitfall 14 工控机取值落地）

```go
db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger:                logger.Default.LogMode(logger.Warn),
    SkipDefaultTransaction: true, // service 层显式 Transaction，关隐式事务
    TranslateError:        true,  // 1062 → gorm.ErrDuplicatedKey [VERIFIED: go doc]
    // 不配 AutoMigrate —— schema 沿用（CLAUDE.md 锁定）
})
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(20)              // 工控机默认（Pitfall 14）
sqlDB.SetMaxIdleConns(5)
sqlDB.SetConnMaxLifetime(30 * time.Minute) // 防 MySQL wait_timeout 坏连接
```

DSN（`[VERIFIED: scratch 对本机 MySQL 8.4 实连成功]`）：

```
root:***@tcp(127.0.0.1:3306)/sys_iam?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai
```

### CGO_ENABLED=0 静态构建（运行 PASS）

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags "-s -w" -o sys-iam-go ./cmd/server
# 实证：含 gin+gorm+koanf+snowflake 全依赖集产物为 statically linked ELF
# (-s -w 剥离后约 17-18MB；未剥离实测 26MB)
```

### envelope 出口（编译 PASS）

```go
// pkg/resp/resp.go
type Envelope struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data"` // data 恒输出（含 null）——禁止 omitempty
}
func OK(c *gin.Context, data any)  { c.JSON(200, Envelope{0, "ok", data}) }
func Fail(c *gin.Context, err error) {
    var ec *errcode.Error
    if !errors.As(err, &ec) {
        ec = errcode.ErrInternal // 固定文案；原始 err 只进 slog
    }
    c.JSON(ec.HTTPStatus(), Envelope{ec.Code, ec.Msg, nil})
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| encoding/json/v2 为 GOEXPERIMENT=jsonv2 实验 | **Go 1.27 转正为标准库**（本机 `go doc encoding/json/v2` 可解析、GOEXPERIMENT 默认空）`[VERIFIED: 本机]` | Go 1.27 | gin c.JSON 走 v1 API 仍受支持；不主动迁 v2，契约测试锁行为防引擎默认值差异（v2 默认更严格） |
| koanf env provider 三参数旧 API | env/v2 子包 + env.Opt 结构 | koanf v2 系列 | STACK.md 安装行需按本文档修正（Pattern 2） |
| 自定义类型四件套进 PO | PO 原生类型 + DTO 契约类型（本次研究定案） | 2026-10-31 | Valuer/Scanner 陷阱根治；PO→DTO 转换处集中兜底逻辑 |

**Deprecated/outdated:**
- `koanf/providers/env`（旧路径）三参数 API：仍在但非文档主形态，Phase 1 不用
- GORM 裸 `Save()` 更新路径：全库禁用（ENG-05）
- envelope 顶层 total + 恒 200：废弃（ARCHITECTURE.md Anti-Pattern 5）

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | 生产库 sys_user/sys_role 有 `builtin` 列（由"源 create() 恒写 builtin 且生产系统正常运行"反推）——本地导入 DDL 缺它是导出方缺陷而非生产事实 | Schema 现状突变 | 若生产确实无此列，源系统插入早报错，此假设几乎不可能错；即便错，Phase 1 本地补列也只影响本地 dev 库 |
| A2 | `mysql-server`/`redis-server`（本机新出现容器）与本工程无关，凭据未索取 | Environment Availability | 若它们才是用户准备的新开发库，执行期环境探测会发现（端口/凭据探测），无计划性风险 |
| A3 | 工控机目标架构 amd64（交叉编译矩阵以 amd64 为主，arm64 留 Makefile 参数） | Code Examples | 若为 arm64，Makefile 改 GOARCH 即可，无架构性影响 |
| A4 | snowflake workerId 单机固定 1（config 可调）足够 | Standard Stack | 多实例扩展时需 workerId 分配机制——项目单机形态下不成立，无风险 |
| A5 | jsonv2 转正后 v1 API 行为完全兼容（go 团队兼容性承诺 + 契约测试防线） | State of the Art | 若有细微差异，ENG-04 契约测试在 Phase 1 即捕获，正是该 Phase 的验收面 |

## Open Questions

1. **`sysiam-mysql` 开发容器去向与新容器身份**
   - What we know: 研究中途 sysiam-mysql/sysiam-redis 被移除；出现 mysql-server(8.4.8)/redis-server(8.6.2)，3306 有 MySQL 在听但 root123 凭据不通过
   - What's unclear: 它们是否为用户重建的开发环境、凭据几何、sys_iam 库与已导入的 DDL/种子是否还在
   - Recommendation: **执行期第一任务做环境重探**（docker ps + 端口探测 + 容器内查询）；若新容器不可用，Phase 1 集成测试全程 testcontainers（D-10 已定），手工验证改用 testcontainers 起一个临时容器（同一 DDL 脚本），计划不阻塞
2. **种子数据导入方与时间基准**
   - What we know: admin 行 id=1 手工 ID、create_time 疑似 UTC 墙钟
   - What's unclear: 谁在导入、是否还会继续改库
   - Recommendation: Phase 1 测试不依赖种子行（testcontainers 自建）；若执行期发现 schema 又变，以执行期实测为准重跑对照（本节 diff 脚本思路可复用）
3. **错误码分段最终表**（10400 vs 10401 的边界、是否需要 10500 段等）
   - What we know: ARCHITECTURE.md 已给分段草案（10001/10100/10200/10300/10400/10401/10501）
   - What's unclear: 草案→定案的编号分配属实现细节
   - Recommendation: planner 直接采纳 ARCHITECTURE.md 草案作为锁定表，Phase 1 定死并写契约测试

## Environment Availability

> 本节为 2026-10-31 快照，**且研究期间环境发生过变更（P9）**——执行期必须重探。

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | 全部 | ✓ | go1.27.3 linux/amd64（本机实装）`[VERIFIED]` | — |
| Docker | testcontainers 集成测试 | ✓ | 29.1.3 `[VERIFIED]` | 无（集成测试必需；单测不依赖） |
| MySQL（dev 手工验证用） | D-12 手工验证 | **✗（漂移）** | sysiam-mysql 已移除；新 mysql-server 凭据未知 | testcontainers 临时容器（同 DDL 脚本）——**推荐直接用此路线** |
| Redis | Phase 2 起 | **✗（漂移）** | sysiam-redis 已移除 | Phase 1 无需；Phase 2 执行期再探 |
| systemd | unit 验证 | ✓ | 255 (255.4-1ubuntu8.18) `[VERIFIED]` | `systemd-analyze verify` 静态校验 |
| make | 构建入口 | ✓ | GNU Make 4.3 `[VERIFIED]` | 直接 go build |
| git | GSD 提交 | ✓ | 仓库正常 `[VERIFIED]` | — |

**Missing dependencies with no fallback:**
- none（全部必需项可用或已有等效 fallback）

**Missing dependencies with fallback:**
- 本机具名 MySQL 容器 → testcontainers 路线（且为 D-10 本来路线，非降级）

## Security Domain

> security_enforcement=true（ASVS Level 1）。Phase 1 为骨架 Phase：认证/会话属 Phase 2，本节聚焦 Phase 1 真实接触面。

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no（Phase 2） | —（D-02 明示 Phase 1 全白名单直通；计划中须注明这是临时态） |
| V3 Session Management | no（Phase 2） | — |
| V4 Access Control | partial | Phase 1 无受保护资源；健康端点暴露面限定为 liveness/readiness 状态，不含内部细节 |
| V5 Input Validation | yes | validator v10 经 Gin ShouldBindJSON binding tag；DTO 指针字段 + required/omitempty 规则；未绑定字段不进 SQL（显式 Select 白名单本身即注入面收敛） |
| V6 Cryptography | no（Phase 1） | bcrypt 属 Phase 2（STACK.md 已验证 $2a$ 兼容）；Phase 1 雪花 ID 非安全随机不用于安全目的 |
| V7 Error Handling & Logging | yes | Recovery 中间件固定文案 500（不回显 SQL/类名/路径，源兜底语义）；原始错误只进 slog；AccessLog 不记录请求体（body 日志是 Phase 4 oplog 的事且带掩码） |

### Known Threat Patterns for {Go 单体 + MySQL}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL 注入 | Tampering | GORM 参数化（`Where("id = ?", x)`）；禁字符串拼 SQL——repo 层 code review 检查项 |
| 错误信息泄漏 | Information Disclosure | envelope 未知错误固定文案；errcode.Error 显式定义的消息才出网 |
| DoS（无界查询） | DoS | 分页 size 钳位上限（UX Pitfall 建议 100）；连接池 MaxOpenConns=20 兜底 |
| 静态二进制被篡改/提权 | Elevation | systemd unit：绝对路径 ExecStart、专用非 root User=、NoNewPrivileges=true、ProtectSystem=full |
| 日志敏感信息 | Information Disclosure | Phase 1 AccessLog 仅 method/path/status/duration/requestID/userID，不落 body/query 敏感键 |

## Sources

### Primary (HIGH confidence)
- **本机 scratch 实证**（`/tmp/gsd-scratch`，Go 1.27.3，2026-10-31）：koanf 全链覆盖、ID/LocalDateTime/枚举三类型契约断言、雪花 16k 并发唯一、GORM+MySQL 8.4 实连、CGO_ENABLED=0 静态 ELF、gin graceful shutdown 编译
- **源码一手读取**（本会话 Read 打开）：
  - `/home/wx/workspace/sys-iam/sys-iam/src/main/java/com/wxinfo/system/iam/domain/entity/User.java:17-64`（@TableName("sys_user")、builtin 字段 62-64 行、create() 恒写 builtin 84 行）
  - `sys-kernel/.../domain/entity/BaseAggregate.java:30-46`（@TableId ASSIGN_ID、createTime/updateTime/remark）
  - `sys-kernel/.../domain/entity/BaseUserEntity.java:17-34`（createById/createByName/updateById/updateByName）
  - `sys-kernel/.../shared/enums/UsageStatus.java:14-31`（DISABLED(0)/ENABLED(1)、fromValue 未知→ENABLED）
  - `sys-iam/.../domain/enums/Gender.java:14-39`（UNKNOWN(0)/MALE(1)/FEMALE(2)、fromValue 未知→UNKNOWN）
  - `sys-kernel/.../config/AutoFillHandler.java:21-66`（insert/update 填充、无上下文跳过操作人）
  - `sys-kernel/.../config/JacksonConfig.java:33-48,86-103`（Long→String、时间三 pattern、FAIL_ON_UNKNOWN 关闭）
  - `sys-kernel/.../persistence/PageHelper.java:23-33`（缺省 1/15、<1 钳位）
- **本机 MySQL 实测**：SHOW TABLES（15 表）、SHOW CREATE TABLE sys_user（逐字 DDL）、information_schema 全库 PO↔DB 列对照 `[VERIFIED: docker exec]`
- **Go module proxy**：`go list -m -versions` 全量核验（2026-10-31）

### Secondary (MEDIUM-HIGH confidence)
- Context7：/knadh/koanf（env/v2 + env.Opt 官方 README）、/go-gorm/gorm（Session/WithContext/Updates/Select、TranslateError）、/gin-gonic/gin（graceful shutdown 官方范式）、/testcontainers/testcontainers-go（MySQL 模块 Run API）
- pkg.go.dev/github.com/bwmarrin/snowflake（官方索引：API、许可、1641 importers）
- go doc（本机）：gorm.Config/TranslateError、time/tzdata、encoding/json/v2
- WebSearch：systemd TimeoutStopSec 语义（freedesktop man + ServerFault + 生产实践文）[CITED: https://www.freedesktop.org/software/systemd/man/systemd.service.html]；jsonv2 时间线（go.dev/blog/jsonv2-exp + 1.27 转正报道，转正事实本机复核）

### Tertiary (LOW confidence — 需执行期复核)
- jsonv2 在 Go 1.27 "默认启用"的第三方表述（本机仅验证包已转正可用；引擎默认开关对 v1 API 行为的影响靠契约测试兜底）
- 并发导入会话的种子数据时间基准

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH —— 版本全部 proxy 核验 + scratch 编译运行
- Architecture: HIGH —— ARCHITECTURE.md 既定 7 Pattern + 本次 3 处实证修正（env/v2 API、PO 原生类型、tzdata 内嵌）
- Pitfalls: HIGH —— P1/P3/P4/P5/P7 为本次实证或源码逐字；P2/P6 官方文档；P9 环境实测
- Schema: HIGH（列事实）/ A1 单项 ASSUMED（生产 builtin 反推，风险极低）

**Research date:** 2026-10-31
**Valid until:** 2026-11-30（版本号 30 天稳定；**环境快照例外——执行期必须重探**）

---
*Research for: Phase 1 工程骨架与横切地基（sys-iam-go）*
