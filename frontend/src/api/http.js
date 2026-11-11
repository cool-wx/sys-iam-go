/**
 * axios 实例 + 拦截器（token 注入 / 信封解包 / 401 跳登录 / 中文错误提示）。
 *
 * Go 后端响应信封 {code, message, data} 三键恒在（pkg/resp Envelope）：
 * - 成功码恒 0（resp.OK 固定 code=0 message="ok"）
 * - 未登录 = 10100（pkg/errcode CodeUnauthenticated）→ 清 token 跳 /login
 *   （业务码与 HTTP 401 双路径——errcode 映射 10100 → HTTP 401）
 * - Authorization: Bearer <token>（internal/middleware/auth.go bearerPrefix 实测带前缀）
 *
 * resolveR 是纯判断函数（node 可测）：拦截器只做薄胶水消费其结果。
 */
import axios from 'axios'
import { message } from 'antd'
import { getToken, clearToken } from '../stores/tokenStorage.js'

/** 信封成功码（pkg/resp resp.OK 实测值，单点维护） */
export const R_OK_CODE = 0

export const http = axios.create({ baseURL: '/api/v1', timeout: 15000 })

// 请求拦截：注入 Authorization 头（Bearer 前缀，后端 BearerToken 解析器要求）
http.interceptors.request.use((cfg) => {
  const token = getToken()
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

/**
 * 响应信封纯判断函数。
 * @param {object} r 信封原对象 {code,message,data}
 * @returns {{ok: boolean, r: object, needLogin: boolean}}
 *   ok=code 命中成功码；needLogin=未登录(10100)，调用方须清 token 跳登录
 */
export function resolveR(r) {
  const code = r && typeof r === 'object' ? r.code : undefined
  if (code === R_OK_CODE) return { ok: true, r, needLogin: false }
  if (code === 10100) return { ok: false, r, needLogin: true }
  return { ok: false, r, needLogin: false }
}

function gotoLogin() {
  clearToken()
  // 拦截器无 react-router 上下文，SPA 外整页跳转
  window.location.href = '/login'
}

// 响应拦截：信封解包 + 统一错误出口（成功时返回信封原对象，调用方解构 data）
http.interceptors.response.use(
  (resp) => {
    // blob 响应（文件下载 responseType:'blob'）：无信封，旁路解包，
    // 返回完整 resp（调用方读 data=blob 与 headers 文件名）
    if (resp.config?.responseType === 'blob') return resp
    const r = resp.data
    const { ok, needLogin } = resolveR(r)
    if (needLogin) {
      gotoLogin()
      return Promise.reject(new Error('登录已失效，请重新登录'))
    }
    if (!ok) {
      message.error(r?.message || '请求失败')
      return Promise.reject(new Error(r?.message || '请求失败'))
    }
    return r
  },
  (err) => {
    if (err.response?.status === 401) {
      gotoLogin()
      return Promise.reject(err)
    }
    message.error(err.response?.data?.message || '网络异常，请稍后重试')
    return Promise.reject(err)
  },
)

// 分页参数约定：后端 ?page=&size= → data={items,total,page,size}（pkg/paging PageData）
export const pageParams = (page, size) => ({ page, size })
