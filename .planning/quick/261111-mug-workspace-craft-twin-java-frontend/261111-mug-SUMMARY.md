---
phase: quick-261111-mug
plan: 01
subsystem: frontend-admin
tags: [frontend, react, antd, vite, api-adaptation, iam]
requires: []
provides:
  - "frontend/ 完整可构建 Vite+React 管理台（系统管理七页 + 操作日志页，/api/v1/** 契约）"
  - "frontend/scripts/smoke-contract.sh 契约冒烟脚本（healthz/10501/10100 真后端直证）"
affects: []
tech-stack:
  added: [react-19, antd-5, vite-8, zustand-5, axios, vitest]
  patterns: [同名函数换端点 API 层, menus/tree 权限派生, sessionStorage 用户快照, 信封归一 {data,total}]
key-files:
  created:
    - frontend/package.json
    - frontend/package-lock.json
    - frontend/vite.config.js
    - frontend/index.html
    - frontend/src/api/http.js
    - frontend/src/api/iam.js
    - frontend/src/router/Layout.jsx
    - frontend/src/pages/logs/OperationLogPage.jsx
    - frontend/src/pages/logs/routes.jsx
    - frontend/src/router/routes.jsx
    - frontend/README.md
    - frontend/scripts/smoke-contract.sh
  modified:
    - .gitignore
decisions:
  - "整包照搬依赖清单（含 4 个暂未引用的运行时依赖）避免 lock 重解析版本漂移——精简留后续 quick 任务"
  - "权限表改由 GET /menus/tree 派生（Go 无用户信息端点）；用户快照 sessionStorage 恢复昵称，权限不落存储"
  - "Layout 恢复链路派生 effect 无取消/无守卫（StrictMode 双执行下取消+ref 守卫会丢弃在途派生致刷新菜单恒空）——写入幂等全局态重复落地无害"
  - "smoke 脚本 go run 需显式 -config configs/config.yaml（默认 ./config.yaml 是部署布局，仓库根不存在）"
metrics:
  duration: 28min
  completed: 2026-11-11
status: complete
actuals:
  tokens: 77036
  tasks: 4
  commits: 5
  plan_head_before: 2b51f00fe045668f726d443865d9def6f08f5e0f
  plan_head_after: 06c5cf7900548733050a7581d8e2c0d4ed630d92
---

# Phase quick-261111-mug Plan 01: workspace craft-twin-java frontend Summary

**Task 1 (checkpoint:human-verify, T-mug-SC) 预先批准**：npm 依赖清单与来源经编排器记录的
AskUserQuestion 于 2026-11-11 获用户「批准安装」——package.json/package-lock.json 逐字节复制自
本机正在运行的源项目（cmp 校验通过），4 个暂未引用的运行时依赖（@stomp/stompjs/sockjs-client/
echarts/dompurify）有意保留以避免 lock 重解析，npm ci 按 lock 零重解析安装 217 包。

## 完成了什么

在 sys-iam-go 仓库内新建 frontend/：从 ~/workspace/craft-twin-java/frontend（只读，全程零改动——
会话窗口内源树修改文件数 0 实证）照搬管理台前端，整域删除 ai/dashboard/device/info/mes/wcs/wms
七页面族、孪生系 API 模块、STOMP/echarts/dompurify/sockjs 运行时依赖，并把 API 层适配到 Go 后端
真实契约。登录主链路（存储→http→api→登录页→布局→菜单过滤）一次贯通，会话快照恢复与
FilePage/UserPage 页面级收口完成，真后端契约冒烟三连直证。

**核心适配**：
- http.js：baseURL `/api/v1`、成功码 0、needLogin=10100 + HTTP 401 双路径清 token 跳 /login、
  `Bearer ` 前缀注入（middleware/auth.go bearerPrefix 实测）
- iam.js：同名函数换端点全对照 router.go；分页归一 `data={items,total}` → `{data,total}`；
  dictTypeId→dict_type_id、categoryId→category_id（query 与 upload PostForm）snake_case 改名；
  dictTypes/dictDatas/configs 固定 page=1&size=100 归一全量数组（钳位裁决）；新增
  menuPermissions()/assignUserRoles()/operationLog()；删 userInfo/refresh 导出
- LoginPage：登录同一响应取 `data={token,user}`，menuPermissions 预拉权限集（失败不阻断）
- Layout：快照回填昵称 + 权限重派生；手风琴菜单两组（系统管理/日志管理）；品牌「权限管理系统」
- OperationLogPage：单表直出、perm=`system:log:list`、首列 createTime、分页 1/10 后端钳位
- UserPage：创建/编辑后按需补调 `PUT /users/:id/roles {roleIds}` 子资源；builtin 创建补调
  `PUT /users/:id {builtin:true}`；三路补充失败各自中文弹错不阻断主成功
- FilePage：删描述输入与「描述」列（Go 上传链路无 description 参数）；下载文件名仍取行数据
  fileName（Go 虽带 RFC 5987 Content-Disposition，页面不依赖响应头）
- 路由 index 重定向 `/system/users`；tokenStorage key `iam.token`/`iam.user`

## 验证证据

| 验证项 | 结果 |
|--------|------|
| `npm --prefix frontend run build` | exit 0（3055 模块全图解析，零悬空 import） |
| `npm --prefix frontend test` | exit 0，vitest 46/46（11 文件；含 StrictMode 缺陷守门用例 ⑥） |
| 被删模块 import 扫描门 | 零命中（api/twin|wcs|wms|mes|llm|sse、stompjs、sockjs、echarts、dompurify） |
| 品牌旧名/craft.token/userInfo 三字面量门 | 零命中 |
| `bash frontend/scripts/smoke-contract.sh` | exit 0：healthz 200、登录失败 10501 信封、无 token 10100 信封；自启实例 graceful shutdown 端口释放实证 |
| Go 后端零 diff | PASS：`git diff --name-only 2b51f00..HEAD` 仅 .gitignore 与 frontend/** |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] smoke 脚本自启后端需显式配置路径**
- **Found during:** Task 4
- **Issue:** `go run ./cmd/server` 默认找 `./config.yaml`（部署布局：配置在二进制旁）；仓库根无
  该文件 → 启动落内置默认 DSN 的 `sys_iam` 库（`Unknown database` boot 失败）
- **Fix:** 自启命令改 `go run ./cmd/server -config configs/config.yaml`（cmd/server/main.go:22
  既有 flag），脚本注释记录两布局差异
- **Files modified:** frontend/scripts/smoke-contract.sh
- **Commit:** 6623924

**2. [Rule 1 - Bug] Layout 恢复链路时序缺陷（计划字面序列不可行）**
- **Found during:** Task 3
- **Issue:** 计划裁决「先 loginSuccess(snapshot, null) 再走既有 menuPermissions() 重派生」——
  两次 state 写入会改变 effect 依赖引用，触发 cleanup 取消在途派生请求，权限表永不落地；
  且 loginSuccess(null, perms) 落地时会把快照回填的 user 抹回 null
- **Fix:** 恢复与派生拆两个 effect；派生完成时与 `useAuthStore.getState().user` 合并一次性写入；
  `derivedOnce` ref 防零权限用户空数组新引用重复拉取；authStore 空权限表改共享常量 `EMPTY_PERMS`
  （引用稳定），快照仅存非敏感展示字段、权限不落存储（T-mug-03 语义保持）
- **Files modified:** frontend/src/router/Layout.jsx, frontend/src/stores/authStore.js
- **Commit:** 2ae5f0a

**3. [Rule 3 - Blocking] 补齐漏复制的 src/pages/system/routes.jsx**
- **Found during:** Task 2
- **Issue:** 复制清单遗漏 system/routes.jsx → import.meta.glob 只聚合 logs 域，PAGES 缺系统管理
  七页，Layout.test 分组断言红（PAGES.length 异常直证）
- **Fix:** 从源逐字节补复制（该文件零改写需求——perm 串与分组标签与 Go 侧一致）
- **Files modified:** frontend/src/pages/system/routes.jsx
- **Commit:** efa2500

**4. [Rule 1 - Bug] OperationLogPage fetchPage 归一口径与计划字面冲突**
- **Found during:** Task 2
- **Issue:** 计划 (g) 字面 `fetchPage 归一 {data: r?.data?.items || [], total}`，但 (d) 已定 iam.js
  operationLog 归一为 `{data: items, total}`——按字面实现 `r.data.items` 恒 undefined → 表格恒空
- **Fix:** 实现语义正确形态 `{data: r?.data || [], total}`（api 层归一 + 页面直读）
- **Files modified:** frontend/src/pages/logs/OperationLogPage.jsx
- **Commit:** efa2500

**5. [Rule 1 - Bug] 权限重派生 effect StrictMode 双执行丢弃结果（浏览器实测发现）**
- **Found during:** 编排器人工核验（Task 4 human-check）
- **Issue:** 整页刷新后 Sider 菜单恒空（SPA 登录正常）。main.jsx StrictMode 下 effect 双执行：
  首跑 cleanup 置 `cancelled=true`、重挂载重跑被 `permsDerivedOnce` ref 守卫拦截（ref 双执行
  模拟中不复位）→ 在途派生结果被无条件丢弃。后端日志两次 GET /menus/tree 相隔 6ms 均 200 佐证
- **Fix:** 删除 cancelled cleanup 与 ref 守卫，无条件应用幂等结果（写入目标为全局态：perm 模块
  级 Set + zustand set，重复落地无害；空 perms 落 EMPTY_PERMS 同引用依赖不变无循环）。Layout.test
  补缺陷守门用例 ⑥（空权限挂载 → 派生必须写入 store 且不丢快照 user）
- **Files modified:** frontend/src/router/Layout.jsx, frontend/src/router/Layout.test.jsx
- **Commit:** 06c5cf7

### 计划外顺带清理（门驱动，非偏离）

- PermButton.jsx/perm.test.js 注释中的 `userInfo` 字样、PermButton.test 的 `wcs:task:cancel` 等
  wcs 域字面量：三字面量收口门要求归零，注释与测试串改为新链路表述/现存域权限串（2ae5f0a）

## Known Stubs

None — 无未接线数据源、无 TODO/FIXME/占位实现。

## 已知边界（计划明示接受，非缺陷）

- 无 `system:menu:list` 的用户登录后左侧菜单为空（后端授权仍拦截实际请求）
- 权限表派生自 menus/tree，仅 UX 层显隐；安全边界在后端 requirePerm（perm.js 红线注释保留）
- Go `UserCreateReq.realName` 为 `binding:"required"` 而源创建表单姓名选填——未填姓名的创建请求
  后端 400/10401「参数校验失败」经拦截器中文弹出。计划裁决保持页面表单与既有测试断言不动，
  该差异留待现场反馈再议
- menus/tree 返回全量菜单，派生权限偏宽仅影响 UX 显隐（计划裁决 1 原文边界）

## PENDING — 人工核验单（Task 4 human-check）

**Status: 已由编排器浏览器实测执行**（核验发现一缺陷，已修复，见下方「编排器浏览器核验结果」）：

`npm --prefix frontend run dev` 后浏览器打开 http://localhost:5173，用真实管理员账号登录：
1. 进入「权限管理系统」台
2. 逐页打开 用户/角色/菜单/组织/字典/参数配置/文件管理/操作日志 八页——无白屏、无 401 跳转
3. 有意点一个无权限操作，确认 403 中文 toast
4. 刷新页面，确认用户名与菜单恢复（快照+重派生）
5. 退出登录，回登录页

自动化前置已通过：`bash frontend/scripts/smoke-contract.sh` exit 0（真后端三契约直证）。

### 编排器浏览器核验结果

1. **页面走查通过**：用户/角色/菜单/组织/字典/参数配置/文件管理七页 + 操作日志页直访全部正常；
   内置用户删除按钮禁用；分页正常。
2. **发现并修复一个真实缺陷**：整页刷新后 Sider 菜单恒空（SPA 登录路径正常）。根因 = main.jsx
   StrictMode 双执行下，权限重派生 effect 的 `cancelled` cleanup 置真 + `permsDerivedOnce` ref
   守卫拦截重跑，在途派生结果被丢弃（后端日志两次 GET /menus/tree 相隔 6ms 均 200 佐证）。
   修复 commit **06c5cf7**：删除取消/守卫逻辑改为无条件应用幂等全局写（perm 模块级 Set +
   zustand），Layout.test 补缺陷守门用例 ⑥；vite build exit 0、vitest 46/46 全绿。
3. **环境数据补全（运行时数据，Go 代码零改动）**：menus/tree 无 system:log:list 权限载体节点，
   已通过 POST /api/v1/menus 以 super 身份创建「操作日志」MENU 节点
   （id 2120453202343104512，父=系统管理 2092924000000001007，path /logs/operation，
   perm system:log:list）。

## Threat Flags

None — 无计划 `<threat_model>` 之外的新增信任面（T-mug-01..04 与 T-mug-SC 缓解面均按计划落位；
T-mug-SC 的安装前人工确认点已经编排器记录的用户批准满足）。

## Commits

| Task | Commit | 内容 |
|------|--------|------|
| 1 checkpoint（已批准） | c885a02 | 依赖清单逐字节复制（T-mug-SC） |
| 2 tracer | efa2500 | 脚手架精简 + 契约层 + 登录主链路贯通（38 文件） |
| 3 auto | 2ae5f0a | 会话快照 + FilePage/UserPage 收口（11 文件） |
| 4 auto | 6623924 | README + 契约冒烟脚本 |
| 人工核验修复 | 06c5cf7 | StrictMode 派生丢弃缺陷修复 + 守门用例 ⑥（编排器浏览器实测发现） |

## Self-Check: PASSED

- 13 个关键产物文件全部在盘（含 frontend/scripts/smoke-contract.sh 可执行位 100755）
- 4 个任务提交全部为 HEAD 祖先（c885a02 / efa2500 / 2ae5f0a / 6623924）
- 工作树干净（仅 SUMMARY.md 未跟踪——按约束不执行器不做 docs 提交，编排器 Step 8 处理）
