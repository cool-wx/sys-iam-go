// @vitest-environment node —— 纯函数，node 直测（perm.test.js 同侧）
/**
 * 权限过滤纯函数。
 *
 * 数据源 = GET /api/v1/menus/tree 树中 permission 非空节点的并集
 * （api/iam.js menuPermissions() 派生，string[]）；
 * 标识格式 {模块}:{业务}:{操作}（如 system:user:list，与后端 RequirePerm 常量逐字对齐）。
 * 红线：前端 hasPerm 仅 UX 层显隐过滤，安全边界在后端 requirePerm——
 * 越权请求被后端 401/403 拦截，本模块不得作为安全缓解措施记录。
 */

/** 模块级单一真相源（非 React 状态，纯函数读） */
let perms = new Set()

/** 写入权限表（空/非法输入归一为空集——坏值不污染） */
export function setPermissions(list) {
  perms = new Set(
    Array.isArray(list) ? list.filter((c) => typeof c === 'string' && c !== '') : [],
  )
}

/** 清空权限表（登出/会话失效） */
export function clearPermissions() {
  perms = new Set()
}

/** 精确匹配（非字符串入参恒 false） */
export function hasPerm(code) {
  return typeof code === 'string' && perms.has(code)
}

/** 任一命中即真（codes 内短路） */
export function hasAny(...codes) {
  return codes.some((c) => hasPerm(c))
}

/** 只读快照（调试/测试用） */
export function getPermissions() {
  return [...perms]
}
