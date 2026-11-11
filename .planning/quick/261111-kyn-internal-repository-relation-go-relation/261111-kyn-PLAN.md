---
phase: quick-261111-kyn
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/repository/relation_user_role.go
  - internal/repository/relation.go
autonomous: true
requirements: []

estimate:
  tokens: 25000
  raw_tokens: 12000
  tasks: 1
  confidence: low

must_haves:
  truths:
    - "internal/repository/relation.go 文件不再存在（整文件删除，非清空）"
    - "RelationRepo 结构体与 NewRelationRepo 构造器完整落位 relation_user_role.go：两条 doc 注释逐字保留，签名一字不改（type RelationRepo struct{db *gorm.DB}、func NewRelationRepo(db *gorm.DB) *RelationRepo）"
    - "relation_user_role.go 的 import 块为三段式（context / gorm.io/gorm 独立组 / 内部 po 组），与 user.go、role.go 的既有惯例一致"
    - "编译与静态检查全绿：go build ./... 与 go vet ./internal/repository/ 退出码 0、gofmt -l internal/repository/ 输出为空（三者规划时基线已实测全绿）"
    - "零调用方改动：internal/service/user.go、internal/service/role.go、internal/app/app.go 及全部测试文件与 HEAD 零 diff——6 处非测试调用点均用包限定名 repository.NewRelationRepo，同包搬移天然无需改动"
  artifacts:
    - "internal/repository/relation_user_role.go（新增承载：导航注释尾句 + gorm.io/gorm import + RelationRepo 结构体 + NewRelationRepo 构造器，原有类型与方法原样保留）"
  key_links:
    - "调用方→构造器链：internal/service/user.go（2 处）、internal/service/role.go（3 处）、internal/app/app.go（1 处）+ 3 个集成测试文件全部经 repository.NewRelationRepo 引用——搬移后 go build ./... 一次通过即直证链路完好"
---

<objective>
删除 internal/repository/relation.go：把仅剩的 RelationRepo 结构体与 NewRelationRepo 构造器并入分表文件 relation_user_role.go，导航注释内容就地改写安放，然后整文件删除 relation.go。这是 9c4afa7（按关联表拆分三文件）的直接收尾——拆分后 relation.go 只剩 4 行导航注释、struct、构造器与 gorm import，作为独立文件已无存在价值。

Purpose: 同包内纯机械搬移，构造器名与包名均不变，全部调用方零改动；消除「一个结构体横跨四个文件、定义却在只剩壳的文件里」的导航负担。
Output: internal/repository/relation.go 删除；relation_user_role.go 成为 RelationRepo 的定义锚点（结构体 + 构造器 + 7 个 sys_user_role 方法 + 3 个投影类型）；编译/静态检查/格式全绿；零行为变更。
</objective>

<execution_context>
@~/.claude/gsd-core/workflows/execute-plan.md
@~/.claude/gsd-core/templates/summary.md
</execution_context>

<context>
本任务无 ROADMAP 需求归属（quick 任务），无需读取任何 prior SUMMARY。

## 现状快照（规划时实测，git dd34cdb，工作树干净，2026-11-11）

relation.go 全文仅 19 行：4 行导航注释（「按表拆文件……结构定义与构造集中在本文件」）、RelationRepo 结构体（db *gorm.DB 字段）、NewRelationRepo 构造器（db 可为 *gorm.DB 或事务 tx）、gorm.io/gorm import。三个分表文件各自头部已命名自己的表：relation_user_role.go（sys_user_role，7 方法 + RoleRef/UserRule/RoleScope 三类型）、relation_role_menu.go（sys_role_menu，5 方法 + RolePermRule）、relation_role_dept.go（sys_role_dept，4 方法）。

## 目标文件选定与理由

结构体与构造器落位 **relation_user_role.go**：它是拆分族的主文件（现导航注释中排列首位、方法最多 7/16、承载全部共享投影类型），且其内部 doc 注释已有引用构造器之处（DeleteUserRolesByUserID 的「可接事务 tx（NewRelationRepo(tx)）」，relation_user_role.go:131）——定义与该引用同文件后导航最短。

## 搬移内容清单（逐字保留，禁止「顺手改进」）

以下三段从 relation.go 搬入 relation_user_role.go，doc 注释逐字复制：

1. 结构体 doc 注释「// RelationRepo 关联表数据访问（只读最小面；写路径随 Wave 2/3 角色分配需求扩展）。」+ type RelationRepo struct{db *gorm.DB}
2. 构造器 doc 注释「// NewRelationRepo 构造（db 可为 *gorm.DB 或事务 tx）。」+ func NewRelationRepo(db *gorm.DB) *RelationRepo
3. import 增加 "gorm.io/gorm"

注意：doc 注释中的既有措辞（含阶段语汇）按约束逐字保留，本任务唯一允许改写的注释是 relation.go 的 4 行文件头导航注释（其「结构定义与构造集中在本文件」随搬移失效，须改写安放，见 action）。

## import 三段式惯例（照 user.go:6-15 / role.go 同款排布）

stdlib 组（context）→ 空行 → gorm.io/gorm 独立组 → 空行 → 内部组（github.com/cool-wx/sys-iam-go/internal/model/po）。

## 调用方清单（全部零改动，仅供回归认知）

非测试 6 处：internal/service/user.go:523、649；internal/service/role.go:269、354、444；internal/app/app.go:96——全部为包限定名 repository.NewRelationRepo(...)，同包搬移不影响解析。测试 3 文件（internal/service/user_integration_test.go、internal/service/name_sync_service_integration_test.go、internal/handler/perm_integration_test.go）同样零改动。

## 范围红线

- internal/model/po/relation.go 是同名 PO 文件，与本任务无关，禁止触碰。
- relation_role_menu.go、relation_role_dept.go 本任务零改动。
- .planning/ 归档中出现的 relation.go 历史记载不改。
</context>

<tasks>

<task type="auto">
  <name>Task 1: RelationRepo 结构体与构造器并入 relation_user_role.go，删除 relation.go</name>
  <files>internal/repository/relation_user_role.go, internal/repository/relation.go</files>
  <precondition>工作树相对 HEAD 干净（git status --porcelain 为空）；internal/repository/relation.go 存在且仍为拆分后形态（结构体 + 构造器 + 导航注释 + gorm import，约 19 行）。</precondition>
  <action>四处编辑一次完成：（1）relation_user_role.go 的 package doc 注释（现有 3 行「sys_user_role（用户-角色关联）数据访问：……」）原样保留，紧随其后追加两行导航句，承接 relation.go 文件头仍在生效的信息——第一行「RelationRepo 单一结构承载三张关联表，按表拆文件——结构定义与构造在本文件；」第二行「另两张表：sys_role_menu（角色-菜单）见 relation_role_menu.go、sys_role_dept（角色-范围部门）见 relation_role_dept.go。」（换行宽度照现有注释行宽排布即可）。（2）import 块按三段式插入 "gorm.io/gorm" 独立分组，置于 context 组与内部 po 组之间（照 user.go/role.go 惯例）。（3）将 context 节「搬移内容清单」三段逐字搬入：RelationRepo 结构体（连同其 doc 注释）与 NewRelationRepo 构造器（连同其 doc 注释）置于 import 块之后、RoleRef 类型定义之前（repo 文件 struct-first 排布，照 user.go:19）；doc 注释逐字复制，禁止改写、删词或增词（含「只读最小面；写路径随 Wave 2/3 角色分配需求扩展」原措辞）。（4）删除 internal/repository/relation.go 整文件（git rm 或删文件均可，勿留空文件）。禁止触碰 internal/model/po/relation.go（同名 PO 文件）与另两个分表文件；禁止改动任何调用方（service/app/测试）。完成后单次提交：refactor(quick-261111-kyn): RelationRepo 并入 relation_user_role.go，删除 relation.go（署名按用户规则用 wangxiang，不出现 Claude 字样）。</action>
  <verify>
    <automated>o="$(gofmt -l internal/repository/)" &amp;&amp; [ -z "$o" ] &amp;&amp; go build ./... &amp;&amp; go vet ./internal/repository/ &amp;&amp; [ ! -e internal/repository/relation.go ] &amp;&amp; [ "$(grep -rl --include='*.go' 'type RelationRepo struct' internal/repository/)" = "internal/repository/relation_user_role.go" ] &amp;&amp; [ "$(grep -rl --include='*.go' 'func NewRelationRepo' internal/repository/)" = "internal/repository/relation_user_role.go" ] &amp;&amp; grep -q 'gorm.io/gorm' internal/repository/relation_user_role.go &amp;&amp; out="$(git diff HEAD --name-only -- internal/ cmd/ pkg/)" &amp;&amp; [ -z "$(printf '%s\n' "$out" | grep -v -e '^internal/repository/relation\.go$' -e '^internal/repository/relation_user_role\.go$')" ]</automated>
  </verify>
  <done>internal/repository/relation.go 整文件删除；RelationRepo 结构体与 NewRelationRepo 构造器（含逐字保留的 doc 注释）落位 relation_user_role.go 且为全仓唯一定义点；relation_user_role.go 的 import 三段式含 gorm.io/gorm、文件头导航句指向另两个分表文件；go build ./...、go vet ./internal/repository/ 退出码 0 且 gofmt -l internal/repository/ 输出为空；git diff HEAD 中 internal/ cmd/ pkg/ 范围仅上述两文件变更（零调用方改动）；完成单次提交（署名 wangxiang）。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| （无新增） | 同包内纯代码搬移，不触及任何输入解析、认证、网络、存储或部署路径；构造器签名、包名、调用点零变化，运行时行为与攻击面零变化 |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-261111-kyn-SC | Tampering | 包/依赖安装 | high | accept | 本任务零依赖变更、零包安装，包合法性门不触发；go build/go vet 确认无供应链面变化 |

注：本任务为文件级机械搬移，无新增信任边界，无运行时威胁条目。
</threat_model>

<verification>
1. `[ ! -e internal/repository/relation.go ]` 为真——文件整删（非清空壳）。
2. `grep -rl --include='*.go' 'type RelationRepo struct' internal/repository/` 输出恰为 internal/repository/relation_user_role.go（定义点唯一化，规划时基线为 relation.go）。
3. `grep -rl --include='*.go' 'func NewRelationRepo' internal/repository/` 输出恰为 internal/repository/relation_user_role.go。
4. `grep -q 'gorm.io/gorm' internal/repository/relation_user_role.go` 命中（import 已落位）。
5. gofmt -l internal/repository/ 输出为空；go build ./... 与 go vet ./internal/repository/ 退出码 0（三者规划时基线已实测全绿，任何失败均系本次改动引入）。
6. `git diff HEAD --name-only -- internal/ cmd/ pkg/` 经排除上述两文件后为空——零调用方、零越界文件改动。
7. 人工抽查 relation_user_role.go：结构体与构造器的两条 doc 注释与原 relation.go 逐字一致；文件头导航句完整指向 relation_role_menu.go 与 relation_role_dept.go；RoleRef/UserRule/RoleScope 三类型与 7 个方法原样未动。
</verification>

<success_criteria>
- relation.go 消失，RelationRepo 定义锚点收敛到 relation_user_role.go 单文件。
- 编译、静态检查、格式检查全绿；diff 仅涉及 relation_user_role.go（改）与 relation.go（删）两个文件。
- 全部既有 doc 注释逐字保留（唯一改写为安放后的导航句）；零行为变更。
- 单次提交完成（署名 wangxiang）；SUMMARY 落盘。
</success_criteria>

<output>
Create `.planning/quick/261111-kyn-internal-repository-relation-go-relation/261111-kyn-SUMMARY.md` when done
</output>
