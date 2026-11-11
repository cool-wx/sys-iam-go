import ConfigPage from './ConfigPage.jsx'
import FilePage from './FilePage.jsx'
import DeptPage from './DeptPage.jsx'
import { ApartmentOutlined, AppstoreOutlined, BookOutlined, FolderOpenOutlined, SettingOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons'
import DictPage from './DictPage.jsx'
import MenuPage from './MenuPage.jsx'
import RolePage from './RolePage.jsx'
import UserPage from './UserPage.jsx'

/**
 * 域6 系统管理页面注册表（被 src/router/routes.jsx 的 import.meta.glob 聚合）。
 * 新页面在此追加，不触碰聚合器。D-07：静态注册即导航，DB 菜单数据仅是权限源头。
 */
export const PAGES = [
  {
    path: '/system/users',
    label: '用户管理',
    perm: 'system:user:list',
    icon: <UserOutlined />,
    group: '系统管理',
    element: <UserPage />,
  },
  {
    path: '/system/roles',
    label: '角色管理',
    perm: 'system:role:list',
    icon: <TeamOutlined />,
    group: '系统管理',
    element: <RolePage />,
  },
  {
    path: '/system/menus',
    label: '菜单管理',
    perm: 'system:menu:list',
    icon: <AppstoreOutlined />,
    group: '系统管理',
    element: <MenuPage />,
  },
  {
    path: '/system/dicts',
    label: '字典管理',
    perm: 'system:dict:list',
    icon: <BookOutlined />,
    group: '系统管理',
    element: <DictPage />,
  },
  {
    path: '/system/configs',
    label: '参数配置',
    perm: 'system:config:list',
    icon: <SettingOutlined />,
    group: '系统管理',
    element: <ConfigPage />,
  },
  {
    // 文档管理统一走 sys-iam /file（F-03：device-doc 已整块移除）
    path: '/system/files',
    label: '文件管理',
    perm: 'system:file:list',
    icon: <FolderOpenOutlined />,
    group: '系统管理',
    element: <FilePage />,
  },
  {
    // 组织管理（quick 261014-l7g）：后端 DeptController 已在线，本页接前端
    path: '/system/depts',
    label: '组织管理',
    perm: 'system:dept:list',
    icon: <ApartmentOutlined />,
    group: '系统管理',
    element: <DeptPage />,
  },
]
