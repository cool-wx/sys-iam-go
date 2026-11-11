/**
 * 布局壳：Sider 菜单（路由聚合器 PAGES 按 group 分组生成，hasPerm 过滤无权限项）——
 * 手风琴模式：分组为可展开 submenu，受控 openKeys 保证同一时刻最多一个一级分组展开。
 * + Header（折叠钮 / 系统名 / 当前用户昵称 / 退出登录）+ Content 渲染子路由。
 * 刷新后 zustand store 清空——昵称经 sessionStorage 用户快照回填（Go 后端无用户信息
 * 端点），权限表经 menuPermissions() 从 menus/tree 重派生（仅 UX 显隐层，
 * 安全边界在后端 RequirePerm）。
 */
import { useEffect, useMemo, useState } from 'react'
import { App, Button, Layout as AntLayout, Menu, Space } from 'antd'
import { LogoutOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore.js'
import { getUserSnapshot } from '../stores/tokenStorage.js'
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

  // 会话恢复第一步（user 为空时）：昵称经 sessionStorage 用户快照回填（非敏感展示字段，
  // 登出与 401 双路径清除）。权限表为空数组占位，随后由下方派生 effect 重拉补齐。
  useEffect(() => {
    if (user) return
    const snapshot = getUserSnapshot()
    if (snapshot) loginSuccess(snapshot, null)
  }, [user, loginSuccess])

  // 会话恢复第二步：权限表重派生（每次挂载重拉，不落存储防陈旧放大）。派生完成时与
  // 此刻 store 中的 user 合并一次性写入。
  //
  // 刻意不设 cancelled cleanup 与 ref 单次守卫——「刷新后菜单恒空」缺陷的修复
  // （浏览器实测：刷新后两次 GET /menus/tree 相隔 6ms 均 200，但权限派生结果未落地）：
  // main.jsx 开启 StrictMode，dev 下 effect 双执行——首跑 cleanup 把取消位置真，
  // StrictMode 重挂载的重跑又被 ref 守卫拦截（ref 在双执行模拟中不复位），在途
  // 结果被无条件丢弃，权限永不落地。此处写入目标是幂等全局态（perm 模块模块级
  // Set + zustand set），重复应用无害——StrictMode 双跑双落地安全。
  // 无循环风险：空 perms 经 loginSuccess 落 EMPTY_PERMS 同一引用，effect 依赖
  // 不变不重跑；非空则上方 length 守卫拦截。
  useEffect(() => {
    if (permissions.length) return
    menuPermissions()
      .then((perms) => {
        loginSuccess(useAuthStore.getState().user, perms || [])
      })
      .catch(() => {})
  }, [permissions, loginSuccess])

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
