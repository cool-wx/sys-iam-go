/**
 * token 单点存取。
 *
 * http.js 请求拦截从这里取 token——避免双处直摸 localStorage。
 * 存储格式 JSON.stringify 包一层：getToken 读侧 JSON.parse 容错回退 null
 * （坏值不外泄，手改 localStorage 成非法 JSON 视为未登录）。
 * storage 以参数注入（默认惰性取 window.localStorage），node 侧 fake 可测。
 */

export const STORAGE_KEY = 'iam.token'

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

/** 清 token（幂等） */
export function clearToken(storage = defaultStorage()) {
  if (!storage) return
  try {
    storage.removeItem(STORAGE_KEY)
  } catch {
    /* 容错 */
  }
}
