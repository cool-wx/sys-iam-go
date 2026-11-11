/**
 * 布局壳：Sider 菜单（路由聚合器 PAGES 按 group 分组生成，hasPerm 过滤无权限项）——
 * 手风琴模式：分组为可展开 submenu，受控 openKeys 保证同一时刻最多一个一级分组展开。
 * + Header（折叠钮 / 系统名 / 当前用户昵称 / 退出登录）+ Content 渲染子路由。
 * 刷新后 zustand store 清空——兜底调 menuPermissions() 从 menus/tree 重派生权限表
 * （Go 后端无用户信息端点；派生仅 UX 显隐层，安全边界在后端 RequirePerm）。
 */
import { useEffect, useMemo, useState } from 'react'
import { App, Button, Layout as AntLayout, Menu, Space } from 'antd'
import { LogoutOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore.js'
import { hasPerm } from '../utils/perm.js'
import { menuPermissions } from '../api/iam.js'
import { PAGES } from './routes.jsx'

const { Header, Sider, Content } = AntLayout

/** 由路由路径反查所属分组（PAGES 精确匹配 path；无命中返回 undefined——如 '/' 未重定向前的瞬时挂载） */
function groupOfPath(pathname) {
  return PAGES.find((p) => p.path === pathname)?.group
}

export default function Layout() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const location = useLocation()
  const [collapsed, setCollapsed] = useState(false)
  // 手风琴受控展开态：惰性初始化——刷新/深链进入时默认展开当前路由所属分组（无命中则零展开）
  // 已知边界（最简化裁决，不加自动跟随 effect）：'/' 经 index Navigate 重定向落地时 Layout 先以
  // pathname='/' 挂载，groupOfPath 为 undefined → 初始零分组展开，符合「最多一个」语义；
  // 路由后续变化不自动展开分组，保持用户手动状态。
  const [openKeys, setOpenKeys] = useState(() => {
    const g = groupOfPath(location.pathname)
    return g ? [g] : []
  })
  const user = useAuthStore((s) => s.user)
  const permissions = useAuthStore((s) => s.permissions)
  const loginSuccess = useAuthStore((s) => s.loginSuccess)
  const logout = useAuthStore((s) => s.logout)

  // 刷新后重派生权限表（user 为空才拉；失败静默——http 拦截器 401 双路径兜底跳登录）
  useEffect(() => {
    if (user) return
    let cancelled = false
    menuPermissions()
      .then((perms) => {
        if (!cancelled) loginSuccess(null, perms || [])
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [user, loginSuccess])

  // 菜单：PAGES 过滤无权限项后按 group 分组（分组= submenu 标题中文，叶子 key=路由 path）
  const menuItems = useMemo(() => {
    const groups = new Map()
    for (const p of PAGES) {
      if (!hasPerm(p.perm)) continue
      if (!groups.has(p.group)) groups.set(p.group, [])
      groups.get(p.group).push({ key: p.path, icon: p.icon, label: p.label })
    }
    return [...groups.entries()].map(([group, children]) => ({
      key: group,
      label: group,
      children,
    }))
  }, [permissions])

  // 手风琴核心：antd onOpenChange 回吐变更后的完整 openKeys 数组且新开键 append 在末位。
  // 菜单严格一层嵌套（children 全为叶子项、无嵌套 submenu）——取末位即「新开一组、收起其余」；
  // 回吐空数组（点击已展开组标题收起）则清空。后人若加嵌套分组，此末位取值不再成立，须改截取逻辑。
  const onMenuOpenChange = (keys) => setOpenKeys(keys.length ? [keys[keys.length - 1]] : [])

  const onLogout = () => {
    logout()
    message.success('已退出登录')
    navigate('/login', { replace: true })
  }

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Sider collapsible collapsed={collapsed} trigger={null} width={208}>
        <div
          style={{
            color: '#fff',
            fontSize: collapsed ? 14 : 16,
            fontWeight: 600,
            textAlign: 'center',
            padding: '16px 8px',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
          }}
        >
          {collapsed ? '权限' : '权限管理系统'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          items={menuItems}
          selectedKeys={[location.pathname]}
          onClick={({ key }) => navigate(key)}
          openKeys={collapsed ? [] : openKeys}
          onOpenChange={onMenuOpenChange}
        />
      </Sider>
      <AntLayout>
        <Header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '0 24px',
            background: '#fff',
          }}
        >
          <Space>
            <Button
              type="text"
              aria-label={collapsed ? '展开菜单' : '折叠菜单'}
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={() => setCollapsed((c) => !c)}
            />
            <span style={{ fontSize: 16, fontWeight: 600 }}>权限管理系统</span>
          </Space>
          <Space>
            <span>{user?.realName || user?.username || ''}</span>
            <Button icon={<LogoutOutlined />} onClick={onLogout}>
              退出登录
            </Button>
          </Space>
        </Header>
        <Content style={{ padding: 16 }}>
          <Outlet />
        </Content>
      </AntLayout>
    </AntLayout>
  )
}
