// @vitest-environment node
/**
 * tokenStorage 契约（域通用 / T-24-01）：
 * ① setToken/getToken 往返（JSON 包装对调用方透明）；② clearToken 清空（幂等）；
 * ③ 坏 JSON / 非 JSON 字符串容错回退 null（只认本模块写入形态，手改存储不外泄坏值）。
 * storage 参数注入照 settings.test.js fake 形态（node 环境无 window）。
 */
import { describe, it, expect } from 'vitest'
import { STORAGE_KEY, getToken, setToken, clearToken } from './tokenStorage.js'

function fakeStorage() {
  const map = new Map()
  return {
    getItem: (k) => (map.has(k) ? map.get(k) : null),
    setItem: (k, v) => map.set(k, String(v)),
    removeItem: (k) => map.delete(k),
  }
}

describe('tokenStorage', () => {
  it('① setToken/getToken 往返（JSON 包装透明）', () => {
    const storage = fakeStorage()
    setToken('satoken-abc123', storage)

    expect(getToken(storage)).toBe('satoken-abc123')
    expect(storage.getItem(STORAGE_KEY)).toBe(JSON.stringify('satoken-abc123'))
  })

  it('② clearToken 清空（幂等，重复清不抛）', () => {
    const storage = fakeStorage()
    setToken('t1', storage)
    clearToken(storage)
    expect(getToken(storage)).toBeNull()
    clearToken(storage)
    expect(getToken(storage)).toBeNull()
  })

  it('③ 坏 JSON / 裸字符串容错回退 null（坏值不外泄）', () => {
    const storage = fakeStorage()
    storage.setItem(STORAGE_KEY, '{not-json')
    expect(getToken(storage)).toBeNull()

    storage.setItem(STORAGE_KEY, 'plain-token')
    expect(getToken(storage)).toBeNull()

    storage.setItem(STORAGE_KEY, JSON.stringify({ not: 'a-string' }))
    expect(getToken(storage)).toBeNull()
  })

  it('④ storage 缺省（node 无 window）安全返回 null / 拒写', () => {
    expect(getToken(undefined)).toBeNull()
    expect(() => setToken('x', undefined)).not.toThrow()
    expect(() => clearToken(undefined)).not.toThrow()
  })
})
