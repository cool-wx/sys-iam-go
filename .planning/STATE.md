---
gsd_state_version: "1.0"
current_phase: 05
current_phase_name: 名称同步事件与验收收尾
status: executing
stopped_at: Completed 05-01-PLAN.md
last_updated: "2026-11-01T23:53:06.976Z"
last_activity: 2026-11-02
last_activity_desc: Phase 05 execution started
state_head: f974c4641c95b78dd974ab74c060de2982ddefbb
progress:
  total_phases: 5
  completed_phases: 4
  total_plans: 16
  completed_plans: 13
  percent: 80
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-11-02, Phase 4 transition)

**Core value:** 在工控机上以单个轻量 Go 服务完整复刻 sys-iam 的全部 IAM 能力，且接口实现与自动化测试全部通过。
**Current focus:** Phase 05 — 名称同步事件与验收收尾

## Current Position

Phase: 05 (名称同步事件与验收收尾) — EXECUTING
Plan: 2 of 3
Status: Ready to execute
Last activity: 2026-11-02 — Phase 05 execution started

Progress: [████████░░] 80%

## Performance Metrics

**Velocity:**
- Total plans completed: 13
- Average duration: -
- Total execution time: -

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1 | 3 | - | - |
| 2 | 3 | - | - |
| 3 | 3 | - | - |
| 04 | 4 | - | - |

**Recent Trend:**
- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 01 P01 | 27min | 2 tasks | 32 files |
| Phase 01 P02 | 20min | 2 tasks | 13 files |
| Phase 01 P03 | 17min | 2 tasks | 5 files |
| Phase 02 P02-01 | 43min | 2 tasks | 33 files |
| Phase 02 P02-02 | 35min | 2 tasks | 25 files |
| Phase 02 P03 | 50 min | 2 tasks | 17 files |
| Phase 03 P03-01 | 50min | 2 tasks | 19 files |
| Phase 03 P03-02 | 36min | 2 tasks | 17 files |
| Phase 03 P03-03 | 40min | 2 tasks | 8 files |
| Phase 04 P01 | 37min | 2 tasks | 28 files |
| Phase 04 P02 | 75min | 2 tasks | 24 files |
| Phase 04 P03 | ~90min（含 429 中断） | 2 tasks | 15 files |
| Phase 04 P04 | 36min | 2 tasks | 13 files |
| Phase 05 P01 | 58min | 2 tasks | 12 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: Casbin 授权（PERM-01..06）独立为 Phase 3，紧跟 RBAC 主链路——依赖 Phase 2 的会话身份与用户/角色数据，且属新增工程（非移植），验收标准独立可观察
- 用户决策覆盖研究建议：授权层用 Casbin（原生策略表 + 热重载即时生效 + dataScope 五档行过滤），认证层仍自建 token-session（PITFALLS.md Pitfall 10 的"登录快照"语义被 PERM-02 显式取代）
- Swagger/Prometheus 为 v2，不进任何 v1 phase；ENG-06 healthz/readyz + 优雅关停保留在 Phase 1
- [Phase 01]: 01-01: 受保护分支处置——master is-protected=true 且无 allow_default_branch_commits，按 #2924/#3819 Re-home 到 gsd/phase-01-foundation 执行（master 未动）
- [Phase 01]: 01-01: 测试基建离线自适应——Docker Hub 不可达，mysql:8.4.x 用本地缓存探测（IAM_TEST_MYSQL_IMAGE 可覆盖），ryuk 默认禁用（TESTCONTAINERS_RYUK_DISABLED 可恢复）
- [Phase 01]: 01-01: 环境重探——开发库 sys_user 已被并发会话补上 builtin（22 列齐备），计划内 ALTER 未执行；自动化测试全程 testcontainers 不受漂移影响
- [Phase 01]: 01-02: 更新审计在 repo.Patch 内显式并入 map（不依赖 BeforeUpdate 钩子）——规避 GORM Updates(map) 钩子字段不落 SET 的语义坑，update_time/操作人列确定性落库
- [Phase 01]: 01-02: PageData[T].MarshalJSON 在类型出口统一保证空列表输出 [] 非 null（ENG-01 空输入边界），所有列表域免判空
- [Phase 01]: 01-02: repo.Patch（map 白名单+RowsAffected 判 404+恒带 Where）与 paging.Parse/PageData 确立为全库更新与列表模板
- [Phase 01]: 01-02: 集成测试 per-test 独立容器 + 进程级雪花单例（sync.Once），TestCreateUserContract 重构复用 newTestApp 基建
- [Phase 01]: 01-03: systemd unit 不配 ExecStop——Type=simple 默认 KillSignal=SIGTERM 由进程内 NotifyContext→SetNotReady→Shutdown(15s) 完成，与 D-09 意图语义等价（脚本实证 exit 0）
- [Phase 01]: 01-03: 摘流 503 断言重设计——Go Shutdown 并发关 idle 连接/停 accept 使排空窗口 503 外部观察仅微秒级竞态，改为摘流单步直证（真 handler 200→503）+ 排空窗口'永不 200'强断言（严格强于单点 503）
- [Phase 01]: 01-03: make verify-static 为出厂门（T-1-09）——file 输出缺 statically linked 即非 0 退出（负路径已验证）；CGO_ENABLED=0 显式进每条构建命令，双架构产物 amd64 22.9MB/arm64 21.1MB
- [Phase 01]: 01-03: 验证脚本镜像离线自适应（IAM_TEST_MYSQL_IMAGE>本地 mysql:8.4.x 探测>mysql:8.4 兜底）与 systemd-analyze verify 需按部署第一步放置 /opt/sys-iam-go 后校验（exit 0，校验后清理）
- [Phase 01 UAT]: ROADMAP Phase 1 goal 回写用户故事格式（优于 override）；ENG-07 缺失取舍认可；systemd 不加 User=（现场账号未定）；arm64 以 qemu-aarch64 binfmt 全链路实证（boot/探活/POST/SIGTERM），真机首启留现场部署清单
- [Phase 01 安全]: 11 项威胁 L1 全 CLOSED（asvs=1 短路），01-SECURITY.md 落盘 threats_open=0
- [Phase 02]: [Phase 02] 02-01: Redis key 三前缀（iam:session:/iam:user-tokens:/iam:apikey-token:）为全项目契约，SessionManager 是唯一事实源；ErrSessionNotFound 包级哨兵归一 HGetAll 空 map — go-redis 对不存在 key 返回空 map+nil 而非 redis.Nil——显式归一让中间件 401/500 分支可 errors.Is 判定
- [Phase 02]: [Phase 02] 02-01: apikeyLogin 复用分支用户已删顺延完整路径（精确 10504/10505）；并发双登录双 token 各自有效+映射最后写者赢（CONTEXT 认可的无害残留） — 源语义保真 + 竞态显式决策留档，TestConcurrentApiKeyFullLogin 直证
- [Phase 02]: [Phase 02] 02-02: 环引用语义按源保真——环节点双方 parent 均命中索引不归根（不可达静默不产出），plan 行为文本「两节点均落根」判笔误；tree_test 2s 超时直证不死循环（T-2-09）
- [Phase 02]: [Phase 02] 02-02: 多步关联写单事务模板确立——service 层 db.Transaction + repository.NewXxxRepo(tx) 复用；assignMenus 校验前置优于源"事务内先删后校验靠回滚"（T-2-11），builtin 守卫前置拦截零删除（409/10602）
- [Phase 02]: [Phase 02] 02-02: repo.ErrNotFound 哨兵 + service 域消息翻译模式——「菜单/部门/角色不存在: {id}」（源消息原文），code 恒 10300/HTTP 404 与 Phase 1 契约一致；assignMenus 缺失消息按验收明文带方括号
- [Phase 02]: ApiKey create userId 引用不存在译 400（CodeInvalidParam）——请求参数引用实体 400、操作主体 404 双轨译码惯例成文（同部门/角色校验链）
- [Phase 02]: types.LocalDateTime 补 UnmarshalJSON（主格式源 DATE_TIME_FORMAT + RFC3339 兜底）——契约对称性缺口修复，请求侧时间键自此全域可用
- [Phase 02]: sys_user_role.username 列存 realName 与 ApiKey.username 冗余取 username 两处相反语义均照源移植并双向注释防混
- [Phase 03]: [Phase 03] 03-01: casbin sub 用 role_code（不可改名 + builtin super_admin 恒定）——策略行不因角色改名失效；super_admin 豁免唯一判据在 enforcer g 层 matcher 首位短路（D-07），会话快照判据废弃
- [Phase 03]: [Phase 03] 03-01: 双轨同步纪律确立——业务 db.Transaction 提交成功后调 enforcer API（adapter 独立 Session 不在事务内），失败上抛 500 重试自愈（幂等全删全加）；AddPolicies 前业务层 map 去重防 MySQL 8.4 Error 1869
- [Phase 03]: [Phase 03] 03-01: boot 对账 fail-fast——启动时关系表全量导入 casbin g/p 策略（ReplaceAll 幂等可重入），存量 admin 冷启动即可用；对账失败拒绝启动绝不静默全量 403
- [Phase 03]: super_admin 不做档位特判——严格表驱动；测试种子 data_scope=5 保既有列表行为，生产存量 NULL 行修正走 PUT /roles/:id（现场部署清单项） — 03-02 裁决记录 1/3：档位语义走表驱动实时查询无缓存，super_admin 无豁免分支
- [Phase 03]: '无角色 fail-closed' 经授权面的可观察形态 = 仅 NULL 档角色持 list perm → 本人档；Widest 对域外档位值按 max 语义原样参与、经 Apply default 落 1=0 双保险 — D-10/D-11 值域锁定 + T-3-07/T-3-09 fail-closed 兜底；纯无角色在 RequirePerm 层即 403
- [Phase 03]: dataScope 域接入模板确立——service resolveScope 编排（identity→行→档位聚合→部门集装配）+ repo 闭包 filter.Apply（Count/Find 同闭包防 total 漂移）；Phase 4 各域照此复制 — D-09/D-12 通用件定位：internal/datascope table 参数化无 user 域耦合
- [Phase 03]: [Phase 03] 03-03: 全量收口后既有测试适配走授权链不拆 gate——grantUserPerm 经 API 链同步 enforcer（03-01 的"改探未挂 gate 端点"绕行方式在全量收口后失效，prohibition「修测试种子授权」落地形态）
- [Phase 03]: [Phase 03] 03-03: IN-14 撤销粒度 user 轴（KickoutUser 连坐密码登录会话）为 D-16 拍板语义——反查键无 TTL 恒可反查，apikey-token 映射 TTL 过期恰是缺口本身；撤销尽力而为不阻断 10503（T-3-11 accept）
- [Phase 04]: 04-01: 1062 翻译统一 400/10400 域文案（Phase 2 role.go 实证惯例非 409）；dict-datas 路由复用 PermDictType* 常量（源共用 dict:* 权限档）
- [Phase 04]: 04-01: Shift list 全量数组直出保留（SHFT-01 源行为直译是验收点非缺陷）；shift/fault perm 发明串 v1 仅 super_admin（Phase 3 Q2 先例）
- [Phase 04]: 04-01: perms.go 六域 29 常量一次加齐（后续 plan 零改动消除串行链文件冲突）；DDL 8 文件一次全落（glob 自动加载）
- [Phase 04]: File 域判重五语义按源 FileUploadHashDedupTest 直译：categoryId eq/IS NULL 双分支（唯一索引对 NULL 互不相等），拒绝消息带本次上传原名，判重命中前零存储/DB 副作用
- [Phase 04]: 上传链 Seek(0) 重放替代研究期 TeeReader 表述（D-12 语义不变）：流式 MD5 判重→拒绝零副作用→事务内 Create+Put（Put 失败回滚，源 save DB→存储同序）
- [Phase 04]: multipart 双层封顶：全局 BodyLimit 1MB 按 Content-Type 跳过 multipart + 路由级 BodyLimitWithMB(20)（外层先包内层恒失效）+ MaxMultipartMemory 8MB 落盘
- [Phase 04]: 下载 Content-Disposition 手写 RFC 5987（filename*=UTF-8'' + url.PathEscape）承载中文并消除 CR/LF 头注入面（T-4-03），恒 attachment（T-4-05 缓解面）
- [Phase 04]: FileCategory 上级分类不存在按 04-02 PLAN 拍板 404/10300（与部门域 400 不同）；Update 加 WR-04 成环拦截照 dept 同构
- [Phase 05]: [Phase 05] 05-01: 事件机制零框架自建（map[Type][]Listener 同步逐个分发、首错即返、未注册 nil）——service 提交后发布+handler 独立事务+`<>` 幂等 UPDATE 三语义立稳，SYNC-02 源死锁时序修正如约落地
- [Phase 05]: [Phase 05] 05-01: 5 个 service 构造签名一次改齐注入 Publisher（照 04-01 perms.go 先例），05-02 起零装配层改动；listeners 35 条 UPDATE 逐条内联不做 helper 收敛（逐条形态即审计面，与源监听器逐行对应）

### Pending Todos

None yet.

### Blockers/Concerns

- [现场部署] arm64 真实工控机首启 healthz 200 确认（qemu 全链路已代证，随部署三步执行）
- [现场部署] 生产库 20 条 BCrypt hash 抽样验证（开发库活体直证已代证 D-16；MySQL collation 已实证 utf8mb4_general_ci）
- [现场部署] system:role:edit 仅授予管理员角色（WR-05 部署信任边界——角色菜单编辑即自授面，deferred 挂账）
- [现场部署] 生产存量 super_admin 行 data_scope=NULL 需经 PUT /roles/:id 置 5（03-02 留档）
- [现场部署] Nacos craft-satoken.yaml timeout 真值校正（D-06 已 30d 代理 + IAM_AUTH_SESSION_TIMEOUT 可覆盖；rnacos 空实例确认永不可得）
- [Phase 4] WR-07/WR-08（advisory）：文件分类「移至根」响应 parentId="0" 与 Create 根 null 双形态 + 对应集成断言永真——04-REVIEW-DISPOSITION 记录在案（IN-03..05 同），可 `/gsd-code-review 04 --fix` 或随 Phase 5 处理
- ~~[Phase 3] IN-14 过期 key 存量会话无自动撤销触发点~~ — 03-03 收口（iam:apikey-user: 反查键三件落地，过期 key 登录拒绝即撤销存量会话）
- ~~[Phase 3] 管理端点全量授权~~ — 03-03 收口（34+2 路由逐路由 requirePerm，RequireRoles 仅 kickout 叠加保留）

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| *(none)* | | | | |

## Session Continuity

Last session: 2026-11-01T23:53:06.810Z
Stopped at: Completed 05-01-PLAN.md
Resume file: None
