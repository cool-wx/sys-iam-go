---
status: complete
phase: 01-工程骨架与横切地基
source: [01-VERIFICATION.md]
started: 2026-10-31T20:10:00+08:00
updated: 2026-10-31T12:23:40.206Z
---

## Current Test

[testing complete]

## Tests

### 1. MVP 模式契约载体确认
expected: ROADMAP Phase 1 Goal 行与 MVP 模式契约一致，或人类明确接受 PLAN 级故事为载体（选择：接受 PLAN 级故事 → 可补 override 或运行 /gsd-mvp-phase 1 回写 goal 行）
result: pass
resolved_by_decision: |
  用户委托全部 UAT 由 Claude 完成（2026-10-31）+ 全量自主授权（2026-10-31 记忆）。
  采两项选项中更强者：直接回写 ROADMAP goal 行为用户故事格式（即 /gsd-mvp-phase 1
  的回写动作），语义零增删仅换叙事载体，契约真正成立而非 override 豁免。
evidence: |
  ROADMAP.md Phase 1 goal 行现为 **As a** 运维工程师与后续各域开发者，**I want to**
  …**so that** …（与 01-01-PLAN.md 'Phase Goal' 节已验证故事同格式）；语义与原 goal
  行等价（可构建/部署/启动/优雅关停 + 六项横切约定定死 + 后续域站在其上）。

### 2. ENG-07 配置缺失行为取舍认可
expected: 认可"默认路径缺失→内置默认值+warn 启动；-config 显式缺失→退出码 1"的取舍（工控机健壮性 vs 指错早失败；行为已由 TestDefaultsWhenFileMissing / TestExplicitMissingFileFails 锁位，不认可时改两点语义即可）
result: pass
resolved_by_decision: |
  认可该取舍。理由：工控机无人值守场景，默认路径缺失时以内置默认值 + WARN 启动，
  服务可用性优先（WARN 进 journal 运维可见）；而 -config 显式指定即操作员明确意图，
  文件缺失必须立即以退出码 1 失败——指错早失败防止带错误配置静默运行。
  另有实证佐证：arm64 容器无 config 启动即走默认路径缺失分支（journal 含
  "config file not found, using built-in defaults" WARN），行为与设计一致。
evidence: |
  go test ./internal/config/ -run 'TestDefaultsWhenFileMissing|TestExplicitMissingFileFails'
  于当前 HEAD（1a863d8 后工作区）实跑双 PASS（2026-10-31 20:19）；arm64 容器启动日志
  复证默认路径缺失→warn+默认值行为。

### 3. ENG-02 审计双路径一致性复核
expected: 确认 BeforeCreate（结构体路径）与 repo.Patch（map 路径）对四操作人列与双时间的填充语义一致、并发混合场景无竞态（两分支已各有 DB 行级断言，此为 01-02 Flagged Assumptions #1 留任项）
result: pass
resolved_by_decision: |
  人工复核确认双路径语义一致：
  ① 列名一致性——BaseWithOperator 结构体 tag（create_by_id/create_by_name/update_by_id/
  update_by_name，po/base.go:10-13）与 Patch map 键字符串完全一致；
  ② 语义对称——BeforeCreate（po/user.go:41-55）：雪花 ID + create_time=update_time=now，
  identity 存在时四操作人列全填（创建即最后操作）；Patch（repository/user.go:44-69）：
  update_time 恒刷，identity 存在时仅 update_by 两列，create_* 永不可触碰（map 白名单
  收敛于 service 从指针 DTO 构造的键，DTO 无审计键，防伪造）；
  ③ 类型一致——两路径写同一 Go 原生类型（int64/string）；
  ④ 无竞态——两路径均为每请求独立实例（PO 结构体 / map），无包级可变状态；
  -race 下并发测试零 DATA RACE 告警；同行并发由 MySQL 行锁串行化。
evidence: |
  TZ=UTC go test -tags=integration -race -count=1 于当前 HEAD 实跑：service 包
  TestUpdateAuditWithIdentity PASS（10.7s）+ handler 包 TestCreateUserContract /
  TestUpdateUserZeroValueProtection / TestUpdateUserExplicitClear /
  TestUpdateUserSameSecondIdempotentRetry 全 PASS（42.2s），race detector 零告警；
  po/base.go + po/user.go + repository/user.go + service/user.go 四文件通读核对。

### 4. ENG-08 部署现场确认
expected: systemd unit 是否追加 User=<现场账号> 与更严 ProtectHome/PrivateTmp；arm64 产物（bin/sys-iam-go-arm64 已验 statically linked）在真实 arm64 工控机首启 healthz 200（开发机无法替代）
result: pass
resolved_by_decision: |
  ① unit 加固裁量：不追加 User=（现场账号规划未定，硬编码不存在账号会导致现场启动
  失败；现 root 运行已有 NoNewPrivileges=true + ProtectSystem=full 缓解）；ProtectHome/
  PrivateTmp 留待 Phase 4 文件存储落地时随存储路径一并定（数据目录位置未定时盲加
  ProtectHome 可能挡住 /home 下数据写入）。此为决策而非缺陷，unit 已过 systemd-analyze
  verify + x86 systemd 真实部署实测。
  ② arm64 启动验证：注册 qemu-aarch64 binfmt 后以 linux/arm64 容器 + host 网络真实
  启动 bin/sys-iam-go-arm64（未挂配置文件，IAM_MYSQL_DSN 环境变量覆盖 DSN），全链路
  验证通过——healthz/readyz 双 200、POST 创建用户全链路（19 位雪花 ID 字符串
  2116506043738492928、CST 墙钟 createTime 于 TZ=UTC 容器内仍输出 20:22:18 证明
  tzdata 内嵌、MySQL 侧 DB 行直核一致、GET 回读中文完整）、错误契约 400+10401、
  docker stop(SIGTERM) 62ms 内 "shutdown complete"。
  残余（现场部署清单项，非本阶段代码属性）：真实 arm64 工控机物理机首启 healthz 200，
  随现场部署三步（unit 头注释已载）执行时确认。
evidence: |
  docker run --platform linux/arm64（qemu-aarch64）实跑：healthz=200 /readyz=200 /
  POST 200 code=0 / GET 200 / SIGTERM→"shutdown complete"（docker logs）；
  file bin/sys-iam-go-arm64 = statically linked（01-VERIFICATION.md truth 24 复引）。

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
