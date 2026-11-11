// @vitest-environment node
/**
 * tokenStorage 契约（域通用）：
 * ① setToken/getToken 往返（JSON 包装对调用方透明）；② clearToken 清空（幂等）；
 * ③ 坏 JSON / 非 JSON 字符串容错回退 null（只认本模块写入形态，手改存储不外泄坏值）；
 * ④ storage 缺省（node 无 window）安全返回 null / 拒写；
 * ⑤ 用户快照三函数：save/get 往返 + clear 清除 + clearToken 级联清快照
 *   （快照仅存非敏感展示字段；null 拒写——loginSuccess(null, perms) 不覆盖已有快照）。
 * storage 参数注入 fake 形态（node 环境无 window）。
 */
import { describe, it, expect } from 'vitest'
import {
  STORAGE_KEY,
  USER_KEY,
  getToken,
  setToken,
  clearToken,
  saveUserSnapshot,
  getUserSnapshot,
  clearUserSnapshot,
} from './tokenStorage.js'

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
    setToken('token-abc123', storage)

    expect(getToken(storage)).toBe('token-abc123')
    expect(storage.getItem(STORAGE_KEY)).toBe(JSON.stringify('token-abc123'))
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

  it('⑤ 用户快照 save/get 往返（对象 JSON 包装透明）', () => {
    const storage = fakeStorage()
    const user = { username: 'admin', realName: '管理员', roles: [] }
    saveUserSnapshot(user, storage)

    expect(getUserSnapshot(storage)).toEqual(user)
    expect(storage.getItem(USER_KEY)).toBe(JSON.stringify(user))
  })

  it('⑥ clearUserSnapshot 清除；clearToken 级联清快照', () => {
    const storage = fakeStorage()
    saveUserSnapshot({ username: 'admin' }, storage)
    clearUserSnapshot(storage)
    expect(getUserSnapshot(storage)).toBeNull()

    saveUserSnapshot({ username: 'admin' }, storage)
    clearToken(storage)
    expect(getUserSnapshot(storage)).toBeNull()
  })

  it('⑦ 快照坏 JSON / 非对象回退 null；null 拒写不覆盖已有快照', () => {
    const storage = fakeStorage()
    storage.setItem(USER_KEY, '{not-json')
    expect(getUserSnapshot(storage)).toBeNull()

    storage.setItem(USER_KEY, JSON.stringify('a-string'))
    expect(getUserSnapshot(storage)).toBeNull()

    saveUserSnapshot({ username: 'admin' }, storage)
    saveUserSnapshot(null, storage)
    expect(getUserSnapshot(storage)).toEqual({ username: 'admin' })
    saveUserSnapshot(undefined, storage)
    expect(getUserSnapshot(storage)).toEqual({ username: 'admin' })
  })
})
