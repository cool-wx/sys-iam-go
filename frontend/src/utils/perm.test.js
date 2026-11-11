// @vitest-environment node
/**
 * perm 纯函数契约（域6 / D-07）：
 * 模块级 Set 持有权限表；setPermissions 空值归一空集；
 * hasPerm 精确匹配（非字符串恒 false）；hasAny 任一命中即真。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import {
  setPermissions,
  clearPermissions,
  hasPerm,
  hasAny,
  getPermissions,
} from './perm.js'

describe('perm', () => {
  beforeEach(() => {
    clearPermissions()
  })

  it('① 空集（未登录/权限未派生）hasPerm 恒 false', () => {
    setPermissions(undefined)
    expect(hasPerm('system:user:list')).toBe(false)
    setPermissions(null)
    expect(hasPerm('system:user:add')).toBe(false)
    setPermissions([])
    expect(getPermissions()).toEqual([])
  })

  it('② setPermissions 后精确匹配 true', () => {
    setPermissions(['system:user:list', 'system:user:add'])
    expect(hasPerm('system:user:list')).toBe(true)
    expect(hasPerm('system:user:add')).toBe(true)
    expect(getPermissions()).toEqual(['system:user:list', 'system:user:add'])
  })

  it('③ 未授权 code false（含空串/非字符串入参）', () => {
    setPermissions(['system:user:list'])
    expect(hasPerm('system:user:remove')).toBe(false)
    expect(hasPerm('')).toBe(false)
    expect(hasPerm(undefined)).toBe(false)
    expect(hasPerm(123)).toBe(false)
  })

  it('④ hasAny 任一命中即真（短路）/ 全无则假', () => {
    setPermissions(['system:user:edit'])
    expect(hasAny('system:user:add', 'system:user:edit')).toBe(true)
    expect(hasAny('system:user:add', 'system:user:remove')).toBe(false)
    expect(hasAny()).toBe(false)
  })
})
