/**
 * token 与用户快照的单点存取。
 *
 * http.js 请求拦截从这里取 token——避免双处直摸 localStorage。
 * 存储格式 JSON.stringify 包一层：读取侧 JSON.parse 容错回退 null
 * （坏值不外泄，手改 localStorage 成非法 JSON 视为未登录）。
 * 用户快照（USER_KEY）仅存非敏感展示字段（username/realName/roles），
 * 供刷新后恢复 Header 昵称——Go 后端无用户信息端点；权限不落存储
 * （每次挂载经 menus/tree 重派生，防陈旧放大）。clearToken 级联清快照
 * （登出与 401 双路径都会走到）。
 * storage 以参数注入（默认惰性取 window.localStorage），node 侧 fake 可测。
 */

export const STORAGE_KEY = 'iam.token'
export const USER_KEY = 'iam.user'

function defaultStorage() {
  return typeof window !== 'undefined' && window.localStorage ? window.localStorage : undefined
}

/** 读 token（坏 JSON / 非字符串一律回退 null） */
export function getToken(storage = defaultStorage()) {
  if (!storage) return null
  let raw = null
  try {
    raw = storage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
  if (raw === null || raw === undefined) return null
  try {
    const parsed = JSON.parse(raw)
    return typeof parsed === 'string' ? parsed : null
  } catch {
    return null
  }
}

/** 写 token（非字符串拒写；存储不可用静默容错——隐私模式等场景） */
export function setToken(token, storage = defaultStorage()) {
  if (!storage || typeof token !== 'string' || token === '') return
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify(token))
  } catch {
    /* 存储不可用容错 */
  }
}

/** 清 token，级联清用户快照（幂等；登出与 401 双路径的唯一清除出口） */
export function clearToken(storage = defaultStorage()) {
  if (!storage) return
  try {
    storage.removeItem(STORAGE_KEY)
    storage.removeItem(USER_KEY)
  } catch {
    /* 容错 */
  }
}

/** 写用户快照（空值/null 拒写——loginSuccess(null, perms) 不得覆盖已有快照） */
export function saveUserSnapshot(user, storage = defaultStorage()) {
  if (!storage || !user || typeof user !== 'object') return
  try {
    storage.setItem(USER_KEY, JSON.stringify(user))
  } catch {
    /* 存储不可用容错 */
  }
}

/** 读用户快照（坏 JSON / 非对象一律回退 null） */
export function getUserSnapshot(storage = defaultStorage()) {
  if (!storage) return null
  let raw = null
  try {
    raw = storage.getItem(USER_KEY)
  } catch {
    return null
  }
  if (raw === null || raw === undefined) return null
  try {
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

/** 清用户快照（幂等；独立暴露供不需要动 token 的场景） */
export function clearUserSnapshot(storage = defaultStorage()) {
  if (!storage) return
  try {
    storage.removeItem(USER_KEY)
  } catch {
    /* 容错 */
  }
}
