// @vitest-environment jsdom
/**
 * Layout 左侧菜单手风琴行为测试（jsdom）：
 * ① 初始单开：initialEntries=['/system/users'] 时「系统管理」分组 aria-expanded=true，
 *    其余分组（日志管理）false，未展开分组的叶子（操作日志）不渲染；
 * ② 新开折叠旧：点击「日志管理」→ 其展开且「系统管理」折叠（手风琴核心断言）；
 * ③ 再切换仍折叠：②之后再点「系统管理」→ 其展开且「日志管理」也折叠；
 * ④ 点击收起：点击当前已展开分组的标题 → 全部分组收起（零展开合法——约束是「最多一个」）；
 * ⑤ 叶子导航不受扰：展开「日志管理」后点叶子「操作日志」→ 该项获得选中态、无渲染崩溃，
 *    且所属分组保持展开（路由变化不折叠用户手动展开态）。
 *
 * api/iam.js mock：Layout 挂载的 menuPermissions() 请求在 jsdom 无后端，mock 掉保证
 * 手风琴断言纯 DOM；其余导出经 importOriginal 透传（import.meta.glob 急切拉起的
 * 各页面模块仍需全部命名导出可解析）。
 */
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App as AntApp } from 'antd'
import { MemoryRouter } from 'react-router'

const { menuPermissionsMock } = vi.hoisted(() => ({
  menuPermissionsMock: vi.fn(),
}))

vi.mock('../api/iam.js', async (importOriginal) => ({
  ...(await importOriginal()),
  menuPermissions: menuPermissionsMock,
}))

import Layout from './Layout.jsx'
import { useAuthStore } from '../stores/authStore.js'

/** 取分组标题的 aria-expanded 属性值（分组非 submenu 结构时返回 null——RED 判据） */
function expandedOf(groupLabel) {
  const li = screen.getByText(groupLabel).closest('li.ant-menu-submenu')
  const title = li?.querySelector('.ant-menu-submenu-title')
  return title?.getAttribute('aria-expanded') ?? null
}

/** 三层脚手架：antd App（App.useApp 的 message 上下文）→ MemoryRouter（无子路由，Outlet 渲染空）→ Layout */
function renderLayout(initialEntries = ['/system/users']) {
  return render(
    <AntApp>
      <MemoryRouter initialEntries={initialEntries}>
        <Layout />
      </MemoryRouter>
    </AntApp>,
  )
}

describe('Layout 手风琴菜单', () => {
  beforeEach(() => {
    menuPermissionsMock.mockReset().mockResolvedValue([])
    // user 非空短路会话恢复 effect（不发请求）；perm 模块级 Set 同步写入——菜单过滤数据源。
    // 覆盖两个现存分组：系统管理（七页 list 串）+ 日志管理（system:log:list）。
    useAuthStore.getState().loginSuccess(
      { realName: '测试用户' },
      [
        'system:user:list',
        'system:role:list',
        'system:menu:list',
        'system:dict:list',
        'system:config:list',
        'system:file:list',
        'system:dept:list',
        'system:log:list',
      ],
    )
  })

  it('① 初始单开：当前路由所属分组默认展开，其余收起且未展开分组叶子不渲染', () => {
    renderLayout()
    expect(expandedOf('系统管理')).toBe('true')
    expect(expandedOf('日志管理')).toBe('false')
    // 辅助断言：收起分组的叶子不出现在文档中
    expect(screen.queryByText('操作日志')).not.toBeInTheDocument()
  })

  it('② 新开折叠旧：点击「日志管理」展开它并折叠「系统管理」（手风琴核心）', () => {
    renderLayout()
    fireEvent.click(screen.getByText('日志管理'))
    expect(expandedOf('日志管理')).toBe('true')
    expect(expandedOf('系统管理')).toBe('false')
  })

  it('③ 再切换仍折叠：切换展开另一组时前一组也折叠', () => {
    renderLayout()
    fireEvent.click(screen.getByText('日志管理'))
    fireEvent.click(screen.getByText('系统管理'))
    expect(expandedOf('系统管理')).toBe('true')
    expect(expandedOf('日志管理')).toBe('false')
  })

  it('④ 点击收起：点击当前已展开分组的标题，全部分组收起（零展开合法）', () => {
    renderLayout()
    fireEvent.click(screen.getByText('系统管理'))
    expect(expandedOf('系统管理')).toBe('false')
    expect(expandedOf('日志管理')).toBe('false')
  })

  it('⑤ 叶子导航不受扰：点击叶子获得选中态且所属分组保持展开', () => {
    renderLayout()
    fireEvent.click(screen.getByText('日志管理'))
    fireEvent.click(screen.getByText('操作日志'))
    const item = screen.getByText('操作日志').closest('li.ant-menu-item')
    expect(item).toHaveClass('ant-menu-item-selected')
    // 路由变化不自动折叠/展开分组——保持用户手动状态
    expect(expandedOf('日志管理')).toBe('true')
  })
})
