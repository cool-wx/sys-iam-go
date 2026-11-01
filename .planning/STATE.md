---
gsd_state_version: "1.0"
current_phase: 02
current_phase_name: 认证会话与 RBAC 主链路
status: verifying
stopped_at: Completed 02-03-PLAN.md（Phase 2 全部 3 plan 完成，21 需求全齐）
last_updated: "2026-11-01T00:42:51.024Z"
last_activity: 2026-10-31
last_activity_desc: Phase 02 execution started
state_head: 24a6fcdfa4c4ab536bfcd2bc964402cf3dd59cb6
progress:
  total_phases: 5
  completed_phases: 1
  total_plans: 6
  completed_plans: 6
  percent: 20
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-31, Phase 1 transition)

**Core value:** 在工控机上以单个轻量 Go 服务完整复刻 sys-iam 的全部 IAM 能力，且接口实现与自动化测试全部通过。
**Current focus:** Phase 02 — 认证会话与 RBAC 主链路

## Current Position

Phase: 02 (认证会话与 RBAC 主链路) — EXECUTING
Plan: 3 of 3
Status: Phase complete — ready for verification
Last activity: 2026-10-31 — Phase 02 execution started

Progress: [██░░░░░░░░] 20%

## Performance Metrics

**Velocity:**
- Total plans completed: 3
- Average duration: -
- Total execution time: -

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1 | 3 | - | - |

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

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 2 规划前] 需向用户索取 Nacos `craft-satoken.yaml` 的 timeout/active-timeout 实际值、MySQL collation
- [Phase 2 验收] 存量 BCrypt hash 抽样验证：从生产库取 20 个 hash 用 Go 验证（PITFALLS.md Pitfall 7）
- [现场部署] arm64 真实工控机首启 healthz 200 确认（qemu 全链路已代证，随部署三步执行）

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| *(none)* | | | | |

## Session Continuity

Last session: 2026-11-01T00:42:50.966Z
Stopped at: Completed 02-03-PLAN.md（Phase 2 全部 3 plan 完成，21 需求全齐）
Resume file: None
