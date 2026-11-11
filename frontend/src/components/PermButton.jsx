/**
 * 权限按钮：hasPerm(props.perm) 为假渲染 null（整按钮隐藏），
 * 为真渲染 antd Button 透传全部 props。仅 UX 层显隐——安全边界在后端 RequirePerm。
 *
 * **必须自身订阅权限状态**：权限表是异步到达的
 * （硬刷新进页面时 `Layout` 的权限派生 menus/tree 尚未返回），而 `hasPerm` 读的是 `utils/perm.js`
 * 的模块级 Set、本身不可响应。若只依赖父组件重渲，会撞上 React 的 **bailout**——
 * `PAGES[].element` 是模块级常量元素，Layout 因权限更新重渲时 `<Outlet/>` 返回**同一元素引用**，
 * React 对同引用子树跳过重渲，于是首帧落在「权限表为空」时求值的按钮**永远返回 null**，
 * 表现为整列操作按钮空白，须手动触发一次重渲（如点刷新）才出现。
 * 该订阅使权限变化时 PermButton 自行重渲。见 PermButton.test.jsx 用例 ③（守门）。
 */
import { Button } from 'antd'
import { hasPerm } from '../utils/perm.js'
import { useAuthStore } from '../stores/authStore.js'

export default function PermButton({ perm, ...rest }) {
  // 订阅权限表：权限到达后触发自身重渲，不依赖父组件重渲（bailout 屏蔽）。
  // 必须在下方早返 null 之前调用（Hooks 规则）。
  useAuthStore((s) => s.permissions)
  if (!hasPerm(perm)) return null
  return <Button {...rest} />
}
