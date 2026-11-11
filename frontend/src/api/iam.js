/**
 * sys-iam-go API 层。端点对照 internal/app/router.go（Go 后端路由面实测）：
 * http baseURL='/api/v1' 已含基路径，此处写相对路径（RESTful 资源段）。
 * 响应信封已由 http 拦截器解包（成功返回 {code,message,data} 原对象）。
 * 分页端点：data={items,total,page,size}（pkg/paging PageData）——归一为
 * {data: items, total} 供 CrudTable/页面既有解包口径零改动消费。
 */
import { http } from './http.js'

// —— auth 段（/api/v1/auth）——
/** POST /auth/login body {username,password} → data={token,user} */
export const login = (body) => http.post('/auth/login', body)
/** POST /auth/logout（Bearer 会话撤销自身） */
export const logout = () => http.post('/auth/logout')

/**
 * 权限表派生（Go 后端无用户信息端点的替代，裁决口径）：
 * GET /menus/tree 后深遍历 children，收集 permission 非空节点的并集。
 * 仅 UX 层显隐数据源；安全边界在后端 RequirePerm。
 */
export async function menuPermissions() {
  const r = await menuTree()
  const perms = []
  const walk = (nodes) =>
    nodes?.forEach((n) => {
      if (typeof n.permission === 'string' && n.permission !== '') perms.push(n.permission)
      walk(n.children)
    })
  walk(r?.data)
  return perms
}

// —— user 段（/api/v1/users）——
/** GET /users?page=&size= → data={items,total,...} 归一 {data, total} */
export async function listUsers(params) {
  const r = await http.get('/users', { params })
  return { data: r?.data?.items ?? r?.data, total: r?.data?.total }
}
/** GET /users/:id */
export const getUser = (id) => http.get(`/users/${id}`)
/** POST /users body {username,password,realName,...} → data=id */
export const addUser = (body) => http.post('/users', body)
/** PUT /users/:id body 全量直传（body 中 id 键后端白名单忽略） */
export const editUser = (body) => http.put(`/users/${body.id}`, body)
/** PUT /users/:id/status body {status}（UsageStatus 枚举 name：ENABLED/DISABLED） */
export const toggleStatus = (id, status) => http.put(`/users/${id}/status`, { status })
/** PUT /users/:id/reset-password（无 body，后端重置并踢存量会话） */
export const resetPwd = (id) => http.put(`/users/${id}/reset-password`)
/** DELETE /users/:id */
export const deleteUser = (id) => http.delete(`/users/${id}`)
/** PUT /users/:id/roles body {roleIds}（角色全量替换子资源） */
export const assignUserRoles = (id, roleIds) =>
  http.put(`/users/${id}/roles`, { roleIds })

// —— dept 段（/api/v1/depts）——
/** GET /depts/tree → data=Dept[]{id,deptName,parentId,sort,status,children}（部门树）
 *  注意 id/parentId 序列化为字符串、status 为枚举 name（'ENABLED'/'DISABLED'） */
export const deptTree = () => http.get('/depts/tree')
/** POST /depts body=Dept 实体 {parentId?, deptName, sort?} → data=id（status 服务端默认启用） */
export const addDept = (body) => http.post('/depts', body)
/** PUT /depts/:id body=Dept 实体（含 id；id 键仅作 URL 定位，白名单不消费） */
export const editDept = (body) => http.put(`/depts/${body.id}`, body)
/** DELETE /depts/:id（后端无子级校验——孤儿外键守卫在前端） */
export const deleteDept = (id) => http.delete(`/depts/${id}`)

// —— role 段（/api/v1/roles）——
/** GET /roles?page=&size=&roleName= → data={items,total,...} 归一 {data, total} */
export async function listRoles(params) {
  const r = await http.get('/roles', { params })
  return { data: r?.data?.items ?? r?.data, total: r?.data?.total }
}
/** GET /roles/:id */
export const getRole = (id) => http.get(`/roles/${id}`)
/** GET /roles/:id/menus → data=该角色已分配菜单 ID 列表 */
export const getRoleMenus = (id) => http.get(`/roles/${id}/menus`)
/** PUT /roles/:id/menus body {menuIds[]}（分配菜单全量替换；权限 system:role:edit） */
export const assignRoleMenus = (id, menuIds) =>
  http.put(`/roles/${id}/menus`, { menuIds })
/** POST /roles body {roleName, roleCode, dataScope?, remark?} → data=id（builtin 服务端守卫） */
export const addRole = (body) => http.post('/roles', body)
/** PUT /roles/:id body {roleName?, dataScope?, status?, remark?}（id 键仅作 URL 定位） */
export const editRole = (body) => http.put(`/roles/${body.id}`, body)
/** DELETE /roles/:id */
export const deleteRole = (id) => http.delete(`/roles/${id}`)

// —— menu 段（/api/v1/menus）——
/** GET /menus/tree → data=Menu[]（自引用 children 全量树） */
export const menuTree = () => http.get('/menus/tree')
/** GET /menus/:id */
export const getMenu = (id) => http.get(`/menus/${id}`)
/** POST /menus body=Menu 实体 {parentId?, name, type, path?, ...} → data=id（status 服务端默认启用） */
export const addMenu = (body) => http.post('/menus', body)
/** PUT /menus/:id body=Menu 实体（含 id；id 键仅作 URL 定位） */
export const editMenu = (body) => http.put(`/menus/${body.id}`, body)
/** DELETE /menus/:id */
export const deleteMenu = (id) => http.delete(`/menus/${id}`)

// —— dictType 段（/api/v1/dict-types）——
/** GET /dict-types?page=1&size=100 → 归一 {data: items}（全量数组口径；后端分页钳位 100，
 *  字典类型规模远小于该上界，超出部分需要时再翻页） */
export async function listDictTypes() {
  const r = await http.get('/dict-types', { params: { page: 1, size: 100 } })
  return { data: r?.data?.items ?? r?.data }
}
/** POST /dict-types body=DictType 实体 {dictName!,dictType!} → data=id（status 服务端默认） */
export const addDictType = (body) => http.post('/dict-types', body)
/** PUT /dict-types/:id body=DictType 实体（含 id；id 键仅作 URL 定位） */
export const editDictType = (body) => http.put(`/dict-types/${body.id}`, body)
/** DELETE /dict-types/:id */
export const deleteDictType = (id) => http.delete(`/dict-types/${id}`)

// —— dictData 段（/api/v1/dict-datas）——
/** GET /dict-datas?dict_type_id=&page=1&size=100 → 归一 {data: items}（参数名 snake_case） */
export async function listDictData(dictTypeId) {
  const params = { page: 1, size: 100 }
  if (dictTypeId != null) params.dict_type_id = dictTypeId
  const r = await http.get('/dict-datas', { params })
  return { data: r?.data?.items ?? r?.data }
}
/** POST /dict-datas body=DictData 实体 {dictTypeId!,label,value,sort} → data=id */
export const addDictData = (body) => http.post('/dict-datas', body)
/** PUT /dict-datas/:id body=DictData 实体（含 id；id 键仅作 URL 定位） */
export const editDictData = (body) => http.put(`/dict-datas/${body.id}`, body)
/** DELETE /dict-datas/:id */
export const deleteDictData = (id) => http.delete(`/dict-datas/${id}`)

// —— config 段（/api/v1/configs）——
/** GET /configs?page=1&size=100 → 归一 {data: items}（全量数组口径，钳位 100 同 dictType） */
export async function listConfigs() {
  const r = await http.get('/configs', { params: { page: 1, size: 100 } })
  return { data: r?.data?.items ?? r?.data }
}
/** POST /configs body=SysConfig 实体 {configKey!,configValue!,configType?} → data=id */
export const addConfig = (body) => http.post('/configs', body)
/** PUT /configs/:id body=SysConfig 实体（含 id；id 键仅作 URL 定位） */
export const editConfig = (body) => http.put(`/configs/${body.id}`, body)
/** DELETE /configs/:id */
export const deleteConfig = (id) => http.delete(`/configs/${id}`)

// —— file 段（/api/v1/files）——
/** GET /files?category_id=&page=&size= → data={items,total,...} 归一 {data, total}
 *  （query 参数名 snake_case：categoryId→category_id；省略即不过滤） */
export async function listFiles(params) {
  const query = { ...params }
  if (query.categoryId != null) {
    query.category_id = query.categoryId
    delete query.categoryId
  }
  const r = await http.get('/files', { params: query })
  return { data: r?.data?.items ?? r?.data, total: r?.data?.total }
}
/** POST /files/upload multipart（file 必填 + category_id?）→ data=FileInfo
 *  FormData 调用方原样透传，api 层把 categoryId 键改名 category_id 后重挂
 *  （handler file.go PostForm("category_id") 口径）；不手工设 Content-Type */
export async function uploadFile(formData) {
  const categoryId = formData.get('categoryId')
  if (categoryId != null) {
    formData.delete('categoryId')
    formData.append('category_id', String(categoryId))
  }
  return http.post('/files/upload', formData)
}
/** GET /files/download/:id → octet-stream 附件（无信封；responseType:'blob' 走拦截器旁路
 *  返回完整 axios resp——读 resp.data 为 blob 即下载内容。响应带 Content-Disposition
 *  （RFC 5987 文件名），页面文件名仍取行数据 fileName，两端不冲突） */
export const downloadFile = (id) =>
  http.get(`/files/download/${id}`, { responseType: 'blob' })
/** DELETE /files/:id */
export const deleteFile = (id) => http.delete(`/files/${id}`)

// —— fileCategory 段（/api/v1/file-categories）——
/** GET /file-categories/tree → data=FileCategory[]（自引用 children 树；权限点同 file list） */
export const fileCategoryTree = () => http.get('/file-categories/tree')
/** POST /file-categories body=FileCategory 实体 {parentId?,categoryName,categoryType?,sort?} → data=id */
export const addFileCategory = (body) => http.post('/file-categories', body)
/** PUT /file-categories/:id body=FileCategory 实体（含 id；id 键仅作 URL 定位） */
export const editFileCategory = (body) => http.put(`/file-categories/${body.id}`, body)
/** DELETE /file-categories/:id */
export const deleteFileCategory = (id) => http.delete(`/file-categories/${id}`)

// —— 操作日志段（/api/v1/logs）——
/** GET /logs?page=&size=&module=&operation=&real_name= → data={items,total,...} 归一 {data, total}
 *  （分页特例缺省 1/10 由后端钳位；query 参数名 snake_case：操作人过滤 real_name） */
export async function operationLog(params) {
  const r = await http.get('/logs', { params })
  return { data: r?.data?.items ?? r?.data, total: r?.data?.total }
}
