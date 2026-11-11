---
phase: quick-261111-mug
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - .gitignore
  - frontend/package.json
  - frontend/package-lock.json
  - frontend/vite.config.js
  - frontend/index.html
  - frontend/README.md
  - frontend/scripts/smoke-contract.sh
  - frontend/src/main.jsx
  - frontend/src/App.jsx
  - frontend/src/api/http.js
  - frontend/src/api/iam.js
  - frontend/src/components/CrudTable.jsx
  - frontend/src/components/CrudTable.test.jsx
  - frontend/src/components/PermButton.jsx
  - frontend/src/components/PermButton.test.jsx
  - frontend/src/pages/login/LoginPage.jsx
  - frontend/src/pages/login/LoginPage.test.jsx
  - frontend/src/pages/system/UserPage.jsx
  - frontend/src/pages/system/UserPage.test.jsx
  - frontend/src/pages/system/RolePage.jsx
  - frontend/src/pages/system/RolePage.test.jsx
  - frontend/src/pages/system/MenuPage.jsx
  - frontend/src/pages/system/DeptPage.jsx
  - frontend/src/pages/system/DeptPage.test.jsx
  - frontend/src/pages/system/DictPage.jsx
  - frontend/src/pages/system/ConfigPage.jsx
  - frontend/src/pages/system/FilePage.jsx
  - frontend/src/pages/system/FilePage.test.jsx
  - frontend/src/pages/system/routes.jsx
  - frontend/src/pages/logs/OperationLogPage.jsx
  - frontend/src/pages/logs/routes.jsx
  - frontend/src/router/routes.jsx
  - frontend/src/router/Layout.jsx
  - frontend/src/router/Layout.test.jsx
  - frontend/src/router/AuthGuard.jsx
  - frontend/src/stores/authStore.js
  - frontend/src/stores/tokenStorage.js
  - frontend/src/stores/tokenStorage.test.js
  - frontend/src/utils/perm.js
  - frontend/src/utils/perm.test.js
  - frontend/src/utils/httpResolve.test.js
  - frontend/src/test/setup.js
autonomous: false
requirements: []
user_setup: []

estimate:
  tokens: 100000
  raw_tokens: 68000
  tasks: 3
  confidence: low

must_haves:
  truths:
    - "frontend/ 目录位于 sys-iam-go 仓库内：npm --prefix frontend run build 退出码 0，npm --prefix frontend test（vitest 全量）退出码 0"
    - "浏览器经 vite dev 代理 /api → http://localhost:8080 可用真实账号登录进入管理台，看到「系统管理」（用户/角色/菜单/字典/参数配置/文件/组织）与「日志管理」（操作日志）两组页面"
    - "全部请求经 http.js 注入 Authorization: Bearer <token>；信封按 code=0 成功、code=10100 或 HTTP 401 清 token 跳 /login；分页读 data.items/data.total"
    - "前端权限表 = GET /menus/tree 树中 permission 非空字符串的并集；hasPerm/PermButton/菜单过滤仅 UX 层显隐，安全边界在后端 requirePerm"
    - "用户/角色/菜单/部门/字典/参数配置/文件/操作日志八页面全部走 /api/v1/** RESTful 契约；源 /api/system/** 与 /api/twin/** 路径在前端代码中零残留"
    - "已删除域（ai/dashboard/device/info/mes/wcs/wms 页面族、孪生系 API 模块、STOMP/echarts/dompurify/sockjs 运行时依赖）在 frontend/ 内零残留"
  artifacts:
    - "frontend/（完整可构建 Vite+React 应用：package.json 与 package-lock.json 逐字节复制自源项目、vite.config.js 重写代理指向 Go 后端、index.html、src/** 精简集、scripts/smoke-contract.sh 契约冒烟脚本、README.md）"
    - "根 .gitignore 增补 frontend/node_modules/ 与 frontend/dist/ 两行"
  key_links:
    - "frontend/vite.config.js proxy '/api' → http://localhost:8080 ↔ internal/app/router.go 挂载面（configs/config.yaml server.port 默认 8080）"
    - "frontend/src/api/iam.js 端点表 ↔ router.go 路由面（/api/v1/users|roles|menus|depts|dict-types|dict-datas|configs|files|file-categories|logs|auth/login|auth/logout）"
    - "frontend/src/api/http.js 信封适配 ↔ pkg/resp{code,message,data} + pkg/errcode（0 成功/10100 未登录/10501 凭证错误）+ pkg/paging{items,total,page,size}"
    - "LoginPage/Layout → GET /menus/tree 收集 permission → utils/perm.js setPermissions → 菜单 p.perm 过滤与 PermButton 显隐"
    - "UserPage 创建/编辑 roleIds → PUT /api/v1/users/:id/roles（router.go 用户子资源，router.go:86-87）"
---

<objective>
在 sys-iam-go 仓库内新建 frontend/ 目录：从 ~/workspace/craft-twin-java/frontend（React 19 + antd 5 + Vite 8 + zustand + axios，注意是 React 不是 Vue）照搬管理台前端，删除 sys-iam-go 后端不存在的能力域（ai/dashboard/device/info/mes/wcs/wms、孪生系 API 层、SSE/STOMP/echarts/dompurify 全套），并把 API 层适配到 sys-iam-go 的真实契约（/api/v1/** RESTful、code=0 信封、Bearer token、data.items 分页）。Go 后端零改动。

Purpose: 给单机工控机部署的 sys-iam-go 配一个可用的管理台；源前端的系统管理域与本后端一一对应，适配成本远低于重写。
Output: frontend/ 完整可构建应用（vite build 通过 + vitest 全绿）+ 根 .gitignore 增补 + 契约冒烟脚本；不动任何 Go 代码。
</objective>

<execution_context>
@~/.zcode/gsd-core/workflows/execute-plan.md
@~/.zcode/gsd-core/templates/summary.md
</execution_context>

<context>
本任务无 ROADMAP 需求归属（quick 任务），无需读取 prior SUMMARY。

## 契约事实（规划时对两侧源码实测，2026-11-11）

**Go 侧（只读参考，禁止修改）**：
- 路由面 @internal/app/router.go：pub 组 login/apikey-login/healthz/readyz；pri 组 Bearer 认证 + 逐路由 RequirePerm。基路径 /api/v1。
- 信封 @pkg/resp/resp.go：{code, message, data} 三键恒在，成功 code=0 message="ok"。
- 错误码 @pkg/errcode/errcode.go：10100 未登录、10200 无权限、10300 不存在、10400 参数无效、10501 用户名或密码错误。
- 分页 @pkg/paging/paging.go：data={items,total,page,size}，默认 page=1/size=15，钳位 maxSize=100（olog 特例 1/10，fault 1/15）。
- token：Authorization: Bearer <token>（internal/middleware/auth.go bearerPrefix）。
- ID 序列化为带引号十进制字符串（pkg/ids.ID），枚举序列化为 name 字符串（"ENABLED"/"DISABLED"/"DIR"/"MENU"/"BUTTON"）——与源前端既有预期一致。
- 登录响应：data={token, user:UserResp}（internal/model/dto/auth.go LoginResp）；UserResp 含 roles:[{roleId,roleName}]。无用户信息端点、无 refresh 端点。

**源前端侧（~/workspace/craft-twin-java/frontend，只读）**：
- 信封预期 {code=200,message,data,total}、needLogin=401、Authorization 裸 token 无 Bearer（src/api/http.js）。
- 端点全走 /api/system/**、/api/twin/**（src/api/iam.js、twin.js）。
- 权限表唯一来源 = 源用户信息端点的 data.permissions（登录页预拉 + Layout 刷新兜底）。
- CrudTable fetchPage 口径：(page,size,params) → {data, total}；各页面自行从 r.data/r.total 解包。
- token 存 localStorage（craft 前缀 key，JSON.stringify 包裹，读侧 JSON.parse 容错）。

## 适配裁决（规划期定案，执行不再议）

1. **用户信息端点缺口的替代**：Go 无对应端点。权限表改为登录后经 GET /menus/tree（需 system:menu:list，管理台用户均持有；super_admin 经 matcher 豁免恒可调）收集树中 permission 非空节点的并集。刷新恢复：user 快照持久化 sessionStorage，permissions 每次挂载重派生。已知边界（接受并记录）：无 system:menu:list 的用户登录后菜单为空（后端授权仍拦截实际请求）；menus/tree 返回全量菜单，派生权限偏宽仅影响 UX 显隐，后端 requirePerm 是安全边界（perm.js 红线原文保留）。
2. **refresh 能力删除**：源刷新端点无页面消费；Go 认证中间件每次命中 EXPIRE 滚动续期，前端无需保活。
3. **无分页列表页适配**：dictTypes/dictDatas/configs 源为全量数组，Go 分页钳位 100——api 层固定传 page=1&size=100 并归一为 {data: items, total}（字典/配置规模 ≤100 合理；超出部分后续需要时再翻页）。
4. **参数名 snake_case**：dictTypeId→dict_type_id、categoryId（query 与 upload PostForm）→category_id、olog 操作人过滤 real_name（页面未用）。
5. **FilePage 描述字段删除**：Go 上传链路签名 Upload(ctx, fh, category_id) 无 description 参数，FileInfoResp.description 恒 null——上传表单移除描述输入、列表移除「描述」列。
6. **UserPage 角色/内置流适配**：Go UserCreateReq 无 roleIds/builtin 键（未知键静默忽略）——创建成功且选择了角色时补调 PUT /users/:id/roles {roleIds}（router.go:86-87 子资源）；builtin 开关经 PUT /users/:id {builtin}（UpdateUserReq.builtin）落实。请求体保留 roleIds/builtin 附加键直传（后端白名单忽略），页面既有测试断言不动。
7. **品牌文案**：源系统名（含「孪生」字样的六字品牌）→「权限管理系统」（登录卡片、Sider 两处、Header、index.html title）。
8. **默认落点**：'/' index 重定向改 /system/users。
9. **登录响应**：login 返回 data={token,user}——LoginPage 一次调用同时拿 token 与 user，不再预拉用户信息。
10. **文件下载文件名**：Go 下载带 Content-Disposition（RFC 5987），源前端从行数据 fileName 取名——保持源行为不变（照搬）。
</context>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 浏览器 → Go 后端 | 全部管理请求跨此边界；token 经 Authorization 头，后端 requirePerm 是唯一安全边界 |
| localStorage/sessionStorage → 前端状态 | token 与用户快照持久于浏览器存储，可被本机用户篡改 |

## STRIDE Threat Register

（phase 前缀取任务标识 mug；本目录无既有 PLAN，编号自 01 起。安全配置：ASVS L1，block_on=high。）

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-mug-01 | Spoofing | localStorage 中的 Bearer token | medium | accept | 源形态保真（localStorage 存 token）；前端无 dangerouslySetInnerHTML 注入点（sanitize 模块随域删除，import 扫描门验证），XSS 面收敛；后端会话可登出撤销 |
| T-mug-02 | Elevation of Privilege | 前端权限表（perm.js Set / menus/tree 派生） | low | accept | perm.js 红线注释原文保留：前端 hasPerm 仅 UX 层显隐，越权请求被后端 403 拦截；本模块不得作为安全缓解措施记录 |
| T-mug-03 | Information Disclosure | sessionStorage 用户快照（username/realName/roles） | low | accept | 仅非敏感展示字段；登出与 401 双路径清除；不持久 permissions（每次重派生，防陈旧放大） |
| T-mug-04 | Denial of Service | vite dev proxy 无鉴权转发到 8080 | low | accept | 仅开发态本机使用；生产形态为 vite build 产物经静态托管/反向代理，proxy 不入包 |
| T-mug-SC | Tampering | npm ci 依赖安装 | high | mitigate | package.json + package-lock.json 逐字节复制自用户本机正在运行的源项目（同版本集已安装并运行，直接在机证据），npm ci 严格按 lock 安装零重解析；安装前 blocking human checkpoint 显式确认依赖清单 |
</threat_model>

<tasks>

<task type="checkpoint:human-verify" gate="blocking-human">
  <name>Checkpoint: npm 依赖合法性确认（T-mug-SC）</name>
  <files>frontend/package.json</files>
  <what-built>frontend/package.json 与 package-lock.json 已从源项目逐字节复制（未安装任何新包），npm ci 前的最后人工确认点。</what-built>
  <how-to-verify>对照本机 ~/workspace/craft-twin-java/frontend/node_modules（正在运行的同一套前端）核对依赖清单版本一致：运行时 react 19.3 / react-dom 19.3 / react-router 7.9 / antd 5.26 / @ant-design/icons 5.6 / @ant-design/v5-patch-for-react-19 1.0.3 / axios 1.20 / zustand 5.0.15；开发依赖 vite 8.3 / vitest 5.0 / jsdom 30 / @testing-library 三件套；npm ci 将按 lock 逐字节还原同一依赖树，无版本重解析。</how-to-verify>
  <resume-signal>用户回复确认批准（例如「批准安装」）即继续执行 npm ci；如对某个依赖有异议，指出包名并暂停。</resume-signal>
  <action>复制源项目 ~/workspace/craft-twin-java/frontend/package.json 与 package-lock.json 到 frontend/（逐字节，未安装任何新包）。向用户展示运行时依赖清单：react 19.3、react-dom 19.3、react-router 7.9、antd 5.26、@ant-design/icons 5.6、@ant-design/v5-patch-for-react-19 1.0.3、axios 1.20、zustand 5.0.15，开发依赖 vite 8.3、vitest 5.0、jsdom 30、@testing-library 三件套——全部与用户本机 craft-twin-java/frontend 正在运行的安装完全同版本（node_modules 在机可验），npm ci 将按 lock 逐字节还原同一依赖树。等待用户确认后才进入下一任务的 npm ci。整包照搬（含暂未被精简集引用的运行时依赖）是有意为之：避免改 package.json 触发 lock 重解析引入版本漂移，精简留给后续 quick 任务。</action>
  <verify>
    <automated>MISSING — checkpoint 为人工批准：用户明确同意按源项目 lock 安装后，方可在下一任务执行 npm ci</automated>
  </verify>
  <done>用户确认依赖清单与来源（本机正在运行的同一套前端），批准 npm ci。</done>
</task>

<task type="tracer">
  <name>Tracer: 脚手架精简 + 契约层 + 登录主链路一次贯通（build 通过即架构直证）</name>
  <files>.gitignore, frontend/package.json, frontend/package-lock.json, frontend/vite.config.js, frontend/index.html, frontend/src/main.jsx, frontend/src/App.jsx, frontend/src/api/http.js, frontend/src/api/iam.js, frontend/src/components/CrudTable.jsx, frontend/src/components/CrudTable.test.jsx, frontend/src/components/PermButton.jsx, frontend/src/components/PermButton.test.jsx, frontend/src/pages/login/LoginPage.jsx, frontend/src/pages/login/LoginPage.test.jsx, frontend/src/pages/system/UserPage.jsx, frontend/src/pages/system/UserPage.test.jsx, frontend/src/pages/system/RolePage.jsx, frontend/src/pages/system/RolePage.test.jsx, frontend/src/pages/system/MenuPage.jsx, frontend/src/pages/system/DeptPage.jsx, frontend/src/pages/system/DeptPage.test.jsx, frontend/src/pages/system/DictPage.jsx, frontend/src/pages/system/ConfigPage.jsx, frontend/src/pages/system/FilePage.jsx, frontend/src/pages/system/FilePage.test.jsx, frontend/src/pages/system/routes.jsx, frontend/src/pages/logs/OperationLogPage.jsx, frontend/src/pages/logs/routes.jsx, frontend/src/router/routes.jsx, frontend/src/router/Layout.jsx, frontend/src/router/Layout.test.jsx, frontend/src/router/AuthGuard.jsx, frontend/src/stores/authStore.js, frontend/src/stores/tokenStorage.js, frontend/src/stores/tokenStorage.test.js, frontend/src/utils/perm.js, frontend/src/utils/perm.test.js, frontend/src/utils/httpResolve.test.js, frontend/src/test/setup.js</files>
  <action>从 ~/workspace/craft-twin-java/frontend 逐字节复制（read-only 源，禁止改源）到 frontend/ 同相对路径：src/main.jsx、src/App.jsx、components/CrudTable.jsx(+test)、components/PermButton.jsx(+test)、pages/login/LoginPage.jsx(+test)、pages/system/ 七页 + routes.jsx + 五个测试、router/routes.jsx、router/Layout.jsx(+test)、router/AuthGuard.jsx、stores/authStore.js、stores/tokenStorage.js(+test)、utils/perm.js(+test)、utils/httpResolve.test.js、test/setup.js，以及 package.json、package-lock.json（checkpoint 已批准）。**不复制**（整域删除）：src/api/ 下除 http.js、iam.js 外的全部模块及其测试、components/ 下告警滚动条与二维定位两组件及其测试与 wcs 子目录、constants/、hooks/、stomp.js、utils/ 下告警合并/库位布局/订单事件/净化/SSE 解析五个模块及其测试、pages/{ai,dashboard,device,info,mes,wcs,wms} 七域、pages/logs/ 下除 OperationLogPage 外的四页与旧 routes.jsx（重写替代）、dist/、learning/、node_modules/。

  然后做一次贯通登录主链路的改写（每一层都触碰：存储→http→api→登录页→布局→菜单过滤）：
  (a) frontend/vite.config.js 重写：@vitejs/plugin-react；server.proxy 仅 '/api' → http://localhost:8080 changeOrigin（Go 默认端口，configs/config.yaml 实测）；test 块保留 jsdom + setupFiles src/test/setup.js；删除源里的 Node 全局兜底 define 块与两个 ws 代理（本后端无 STOMP/SockJS 端点）。
  (b) frontend/index.html：结构照搬，title 改「权限管理系统」。
  (c) frontend/src/api/http.js 重写（resolveR 纯函数结构与测试导出保留）：baseURL '/api/v1'；R_OK_CODE=0（pkg/resp 成功码实测）；needLogin 判 code===10100（pkg/errcode CodeUnauthenticated），保留 HTTP 401 双路径；请求拦截注入 Authorization: 'Bearer ' + token（middleware/auth.go bearerPrefix 实测带前缀）；错误文案逻辑照搬。
  (d) frontend/src/api/iam.js 重写为「同名函数换端点」——页面在用的导出名全保留（页面零改动），逐函数对照 router.go 换到 /api/v1 RESTful（baseURL 已含 /api/v1）：login→POST /auth/login；logout→POST /auth/logout；源用户信息接口与源刷新接口两个导出删除（前者无后端端点、后者无消费方，见裁决 1/2）；新增 menuPermissions()（调 menuTree 后深遍历 children 收集 permission 非空字符串数组）；listUsers→GET /users（params page/size，归一返回 {data: r.data.items, total: r.data.total}）；getUser→GET /users/:id；addUser→POST /users；editUser→PUT /users/:id（body 中 id 键保留直传，后端白名单忽略）；toggleStatus→PUT /users/:id/status body {status}（UpdateStatusReq 实测）；resetPwd→PUT /users/:id/reset-password 无 body；deleteUser→DELETE /users/:id；新增 assignUserRoles(id, roleIds)→PUT /users/:id/roles body {roleIds}（router.go:86-87 子资源）；deptTree→GET /depts/tree；addDept→POST /depts；editDept→PUT /depts/:id；deleteDept→DELETE /depts/:id；listRoles→GET /roles（page/size[/roleName] 同归一）；getRole→GET /roles/:id；getRoleMenus→GET /roles/:id/menus（data=ID 数组）；assignRoleMenus(id,menuIds)→PUT /roles/:id/menus body {menuIds}（源是 POST + body 带 id，方法与形状都变）；addRole→POST /roles；editRole→PUT /roles/:id；deleteRole→DELETE /roles/:id；menuTree→GET /menus/tree；getMenu→GET /menus/:id；addMenu→POST /menus；editMenu→PUT /menus/:id；deleteMenu→DELETE /menus/:id；listDictTypes→GET /dict-types?page=1&size=100 归一 {data: items}（钳位 100 见裁决 3）；addDictType/editDictType/deleteDictType→POST /dict-types、PUT /dict-types/:id、DELETE /dict-types/:id；listDictData(dictTypeId)→GET /dict-datas?dict_type_id=…&page=1&size=100 归一；addDictData/editDictData/deleteDictData→POST /dict-datas、PUT /dict-datas/:id、DELETE /dict-datas/:id；listConfigs→GET /configs?page=1&size=100 归一；addConfig/editConfig/deleteConfig→POST /configs、PUT /configs/:id、DELETE /configs/:id；listFiles→GET /files?category_id=…&page&size 归一 {data: items, total}；uploadFile→POST /files/upload（FormData 调用方原样透传，api 层把 categoryId 键改名 category_id 后 append——handler file.go PostForm("category_id") 实测）；downloadFile→GET /files/download/:id responseType blob 照搬（裁决 10）；deleteFile→DELETE /files/:id；fileCategoryTree→GET /file-categories/tree；addFileCategory/editFileCategory/deleteFileCategory→POST /file-categories、PUT /file-categories/:id、DELETE /file-categories/:id；新增 operationLog(params)→GET /logs（page/size 归一 {data: items, total}，olog 特例默认 1/10 由后端钳位）。所有归一函数读侧统一 r?.data?.items 兜底 r?.data（后端 PageData.MarshalJSON 空列表恒 []）。字段名兼容性执行时对 internal/model/dto/{user,role,menu,dept,dict,config,file,operation_log}.go 逐一核对（规划期已核对：字段名与源一致，仅 query/form 参数名与信封形状不同）。
  (e) frontend/src/pages/login/LoginPage.jsx 收口：login({username,password}) 成功后从同一响应取 r.data.token 与 r.data.user（裁决 9）；setToken 后调 menuPermissions() 预拉权限集（失败不阻断，Layout 兜底重拉）；loginSuccess(user, perms) 后 navigate('/')；卡片标题与页面头注释的品牌改写（裁决 7），头注释数据源描述同步为新链路。
  (f) frontend/src/router/Layout.jsx 收口：删对源用户信息接口的 import 与 useEffect 调用；恢复逻辑改为 menuPermissions() 重派生权限集后 loginSuccess(当前 user, perms)（user 回填留待下一任务的快照，本任务 Header 昵称渲染空串无害）；两处系统名品牌改写（裁决 7）。
  (g) frontend/src/pages/logs/OperationLogPage.jsx 手术：import 改自 ../../api/iam.js 仅 operationLog；删 Tabs 与入出库页签（单表直出，LogTable 结构保留）；PERM 改 'system:log:list'（Go PermLogList，internal/authz/perms.go:98 实测；源 twin 日志权限串作废）；COLUMNS 首列 dataIndex 'time'→'createTime'（OperationLogResp 实测键名）；fetchPage 归一 {data: r?.data?.items || [], total}。
  (h) frontend/src/pages/logs/routes.jsx 重写：仅一个条目 {path:'/logs/operation', label:'操作日志', perm:'system:log:list', group:'日志管理', icon 沿源 FileSearchOutlined, element:<OperationLogPage/>}。
  (i) frontend/src/router/routes.jsx：index 重定向改 '/system/users'（裁决 8），头注释同步改写不留旧路径字样。
  (j) frontend/src/utils/perm.js 头注释改写：数据源描述由源用户信息端点改为 menus/tree permission 并集（裁决 1）；hasPerm/setPermissions 等函数体与红线段逐字保留。
  (k) tokenStorage 的 localStorage key 由源项目 craft 前缀改为 'iam.token'（JSON 纪律不变）；tokenStorage.test 如有对 key 字面量的断言同步跟改。
  (l) 四个测试文件适配到新链路：Layout.test.jsx——分组断言改现存「系统管理/日志管理」两组（PAGES 经 import.meta.glob 已只剩保留域），并 vi.mock('../../api/iam.js') 提供 menuPermissions 桩（Layout 的 useEffect 请求在 jsdom 无后端，mock 掉保证手风琴断言纯 DOM）；LoginPage.test.jsx——mock 工厂键改为 login + menuPermissions，登录成功断言补对 menuPermissions/loginSuccess 的调用；httpResolve.test.jsx——成功码断言 200→0、needLogin 断言 401→10100；其余保持。
  (m) 根 .gitignore 追加 frontend/node_modules/ 与 frontend/dist/。

  执行 npm ci（checkpoint 已批准）后跑验证。本任务是全计划的 tracer：存储→http→api→登录→布局→菜单过滤一条真实路径全部打通，vite build 的全模块图解析即架构直证。</action>
  <verify>
    <automated>npm --prefix frontend run build && npm --prefix frontend test && ! grep -rEl "from '.*(api/(twin|wcs|wms|mes|llm|sse))|stompjs|sockjs|echarts|dompurify" frontend/src --include="*.js" --include="*.jsx"</automated>
  </verify>
  <done>vite build 退出码 0（全模块图解析直证：保留域零悬空 import）；vitest 全量退出码 0；import 扫描门退出码 0（被删 API 模块与三库零残留；基线：任务前 frontend/ 不存在，门恒真，本门为新建树守门）。</done>
</task>

<task type="auto">
  <name>Task: 会话快照恢复 + 页面级适配收口（FilePage/UserPage）</name>
  <files>frontend/src/stores/tokenStorage.js, frontend/src/stores/tokenStorage.test.js, frontend/src/stores/authStore.js, frontend/src/router/Layout.jsx, frontend/src/pages/system/FilePage.jsx, frontend/src/pages/system/UserPage.jsx, frontend/src/pages/system/UserPage.test.jsx</files>
  <action>(a) 会话快照（裁决 1、T-mug-03）：tokenStorage 新增用户快照三函数（USER_KEY 'iam.user'）——saveUserSnapshot(user)/getUserSnapshot()/clearUserSnapshot()，JSON 纪律与 token 一致（写侧 stringify、读侧坏 JSON 回退 null），clearToken 级联清快照；tokenStorage.test 补快照读写清三用例。authStore.loginSuccess 增 saveUserSnapshot(user) 一行，logout 增快照清除（clearToken 已级联则不重复）。Layout 的恢复 effect 增 user 回填：getUserSnapshot() 非空则先 loginSuccess(snapshot, null) 再走既有 menuPermissions() 重派生。
  (b) FilePage 适配（裁决 5）：上传表单删描述输入及其 append；列表删「描述」列；页面头注释与 Upload 组件说明文案同步（categoryId 状态变量名不动，api 层已负责改名）。
  (c) UserPage 适配（裁决 6）：创建提交 addUser 成功且表单 roleIds 非空时补调 assignUserRoles(新 id, roleIds)；builtin 开关为 true 时补调 editUser({id, builtin:true})；编辑提交 editUser 成功且 roleIds 相对行数据 roles 变化时补调 assignUserRoles；三处补充调用失败各自 message 弹错且刷新表格。
  (d) UserPage.test 增断言：创建后 assignUserRoles 被以新 id 与所选 roleIds 调用（vi.mock 工厂补 assignUserRoles 桩）；既有 roleIds/builtin 直传断言保持不动。</action>
  <verify>
    <automated>npm --prefix frontend test && ! grep -rn "数字孪生\|craft\.token\|userInfo" frontend/src --include="*.js" --include="*.jsx"</automated>
  </verify>
  <done>vitest 全量退出码 0（含 tokenStorage 快照与 UserPage 角色子资源新用例）；品牌旧名、源 token key、源用户信息接口三个字面量在 frontend/src 归零（品牌与旧接口引用在上一任务已清，本门为收口终验；基线：上一任务验证已过，本门应零命中）。</done>
</task>

<task type="auto">
  <name>Task: README + 真后端契约冒烟（boot/healthz/登录失败路径/Bearer 401）</name>
  <files>frontend/README.md, frontend/scripts/smoke-contract.sh</files>
  <action>(a) frontend/README.md：一段式说明——技术栈（React 19 + antd 5 + Vite 8，照搬自 craft-twin-java 管理台）、npm ci / npm run dev（代理 /api → http://localhost:8080）/ npm run build / npm test 四命令、Go 侧前置（configs/config.yaml 默认端口 8080，MySQL/Redis 按其配置就绪）、已知边界两条（菜单显隐依赖 system:menu:list；权限表派生自 menus/tree 仅 UX 层）。
  (b) frontend/scripts/smoke-contract.sh（bash，set -euo pipefail，可执行位）：①若 curl -sf http://localhost:8080/healthz 已通则复用现有实例，否则 go run ./cmd/server 后台启动并记录 pid（日志重定向 /tmp/sys-iam-mug-smoke.log，sleep 4 等就绪）；②断言 healthz 200；③POST /api/v1/auth/login body 用不存在的用户名与错误密码，断言响应含 "code":10501（凭证错误契约 + gin JSON 紧凑信封直证，无需真实凭证）；④无 token GET /api/v1/users，断言含 "code":10100（认证中间件 401 契约直证）；⑤若为本脚本启动的实例则 kill 收尾；任一步不符 exit 1。脚本纯 curl+grep，不依赖 jq。前端即将消费的四条核心契约全部真后端直证。</action>
  <verify>
    <automated>bash frontend/scripts/smoke-contract.sh</automated>
  </verify>
  <done>冒烟脚本退出码 0：后端 boot（或复用运行中实例）、healthz 200、登录失败返回 10501 信封、无 token 访问返回 10100 信封。</done>
  <precondition>开发库 MySQL(127.0.0.1:3306，DSN 见 configs/config.yaml) 与 Redis(127.0.0.1:6379) 容器在运行；8080 端口未被其他进程占用（或已被 sys-iam-go 占用）。</precondition>
  <verify>
    <human-check>npm --prefix frontend run dev 后浏览器打开 http://localhost:5173，用真实管理员账号登录：进入「权限管理系统」台，逐页打开用户/角色/菜单/组织/字典/参数配置/文件管理/操作日志八页无白屏、无 401 跳转；有意点一个无权限操作确认 403 中文 toast；刷新页面确认用户名与菜单恢复（快照+重派生）；退出登录回登录页。</human-check>
  </verify>
</task>

</tasks>

<verification>
- npm --prefix frontend run build 退出码 0（tracer 任务）
- npm --prefix frontend test 退出码 0（tracer 与收口任务各跑一次，终态全量）
- bash frontend/scripts/smoke-contract.sh 退出码 0（真后端四契约）
- 三道 grep 门归零：被删模块 import 扫描（tracer）、品牌旧名/源 key/源用户信息接口三字面量（收口任务）
- git diff --stat 仅含 .gitignore 与 frontend/**（Go 侧零改动）
- 浏览器人工核验单（末任务 human-check）
</verification>

<success_criteria>
frontend/ 成为 sys-iam-go 的可用管理台：源前端的系统管理七页 + 操作日志页全部经 /api/v1/** 真实端点工作，登录/登出/权限显隐/刷新恢复闭环，构建与单测全绿，真后端冒烟四契约直证；已删除域零残留；Go 代码零 diff。
</success_criteria>

<output>
Create .planning/quick/261111-mug-workspace-craft-twin-java-frontend/261111-mug-SUMMARY.md when done
</output>
