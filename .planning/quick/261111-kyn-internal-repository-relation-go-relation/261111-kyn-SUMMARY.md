---
phase: quick-261111-kyn
plan: 01
subsystem: internal/repository
tags: [关联表数据访问, 文件收敛, 零行为变更, 机械搬移]
requires: []
provides:
  - RelationRepo 定义锚点收敛到 relation_user_role.go 单文件（结构体 + 构造器 + 7 个 sys_user_role 方法 + 3 个投影类型）
  - relation.go 壳文件消除（9c4afa7 三文件拆分的直接收尾）
affects: []
tech-stack:
  added: []
  patterns:
    - repo 文件 struct-first 排布与 import 三段式（stdlib / gorm.io/gorm / 内部组）延续 user.go/role.go 惯例
key-files:
  created: []
  modified:
    - internal/repository/relation_user_role.go
  deleted:
    - internal/repository/relation.go
decisions:
  - 目标文件选定 relation_user_role.go：拆分族主文件（方法最多 7/16、承载全部共享投影类型），且其内部已有 NewRelationRepo(tx) 引用（DeleteUserRolesByUserID doc 注释），定义与引用同文件后导航最短
  - 文件头导航句按计划给定文案改写安放；「只读最小面；写路径随 Wave 2/3」措辞按约束逐字保留（改写留待未来行级任务）
metrics:
  duration: 3min
  completed: 2026-11-11
status: complete
actuals:
  tokens: 562
  tasks: 1
  commits: 1
  plan_head_before: 5c9bed1
  plan_head_after: 6ba8a76
---

# Phase quick-261111-kyn Plan 01: RelationRepo 并入 relation_user_role.go，删除 relation.go Summary

同包内纯机械搬移收尾 9c4afa7 的三文件拆分：把仅剩壳内容（4 行导航注释 + RelationRepo 结构体 + NewRelationRepo 构造器 + gorm import）的 relation.go 整文件删除，结构体与构造器连 doc 注释逐字落位拆分族主文件 relation_user_role.go，导航注释就地改写安放；构造器名与包名不变，全部调用方（service 6 处非测试 + 3 测试文件）零改动。

## 任务完成情况

| Task | 内容 | 文件 | 提交 |
|------|------|------|------|
| 1 | 导航句安放 + gorm import 三段式 + 结构体/构造器逐字搬入 + relation.go 整删 | relation_user_role.go（改）、relation.go（删） | 6ba8a76 |

## 终验门结果（全部通过）

1. `[ ! -e internal/repository/relation.go ]` —— 文件整删（非清空壳）。
2. `grep -rl 'type RelationRepo struct'` 恰为 internal/repository/relation_user_role.go（定义点唯一化）。
3. `grep -rl 'func NewRelationRepo'` 恰为 internal/repository/relation_user_role.go。
4. `grep -q 'gorm.io/gorm'` 命中 relation_user_role.go（import 三段式落位：context / gorm.io/gorm 独立组 / 内部 po 组）。
5. `gofmt -l internal/repository/` 为空；`go build ./...` 与 `go vet ./internal/repository/` 退出码 0。
6. `git diff HEAD --name-only -- internal/ cmd/ pkg/` 经排除两目标文件后为空——零调用方、零越界改动。
7. 逐字校验：HEAD 原 relation.go 第 11-19 行（结构体 + 构造器含 doc 注释）与新文件对应块 `diff -u` 零差异；RoleRef/UserRule/RoleScope 三类型与 7 个方法原样未动；文件头导航句完整指向 relation_role_menu.go 与 relation_role_dept.go。

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None.

## Self-Check: PASSED

- internal/repository/relation_user_role.go 存在（FOUND）
- internal/repository/relation.go 不存在（FOUND，符合预期删除）
- 提交 6ba8a76 为 HEAD 祖先（FOUND，即 HEAD 本身）
