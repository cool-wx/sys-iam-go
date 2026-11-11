// @vitest-environment node
/**
 * http.js resolveR 纯判断函数契约：
 * ① code 0（R_OK_CODE，pkg/resp resp.OK 实证）→ ok 透传信封原对象（data 由调用方解构）；
 * ② code 10100（CodeUnauthenticated）→ needLogin 标记（拦截器消费后清 token 跳登录）；
 * ③ 其余业务码 → ok:false + 中文 message 原样透传。
 * antd mock 掉（node 环境不触碰组件库）。
 */
import { describe, it, expect, vi } from 'vitest'

vi.mock('antd', () => ({
  message: { error: vi.fn() },
}))

import { resolveR, R_OK_CODE } from '../api/http.js'

describe('resolveR', () => {
  it('① code 0 → ok:true 透传信封原对象（data）', () => {
    const r = { code: 0, message: 'ok', data: [{ id: 1 }] }
    const out = resolveR(r)

    expect(R_OK_CODE).toBe(0)
    expect(out.ok).toBe(true)
    expect(out.needLogin).toBe(false)
    expect(out.r).toBe(r)
    expect(out.r.data).toEqual([{ id: 1 }])
  })

  it('② code 10100 → needLogin:true（拦截器清 token 跳 /login 的唯一依据）', () => {
    const out = resolveR({ code: 10100, message: '未登录', data: null })

    expect(out.ok).toBe(false)
    expect(out.needLogin).toBe(true)
  })

  it('③ code 10501 业务失败 → ok:false、非登录问题、中文 message 透传', () => {
    const r = { code: 10501, message: '用户名或密码错误', data: null }
    const out = resolveR(r)

    expect(out.ok).toBe(false)
    expect(out.needLogin).toBe(false)
    expect(out.r.message).toBe('用户名或密码错误')
  })
})
