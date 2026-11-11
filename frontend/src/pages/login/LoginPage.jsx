import { useState } from 'react'
import { App, Button, Card, Form, Input } from 'antd'
import { useNavigate } from 'react-router'
import { login, menuPermissions } from '../../api/iam.js'
import { setToken } from '../../stores/tokenStorage.js'
import { useAuthStore } from '../../stores/authStore.js'

/**
 * 登录页：POST /api/v1/auth/login（经 http 实例，拦截器已解包信封）。
 * 成功 → 同一响应 data={token,user} 一次拿到令牌与用户（后端无独立用户信息端点）；
 * token 存 tokenStorage → 预拉 menuPermissions()（menus/tree 派生权限集进 authStore/perm 模块）→ 跳首页。
 * 失败提示由 http 拦截器统一中文弹出。
 */
export default function LoginPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const loginSuccess = useAuthStore((s) => s.loginSuccess)
  const [loading, setLoading] = useState(false)

  const onFinish = async (values) => {
    setLoading(true)
    try {
      const r = await login({ username: values.username, password: values.password })
      const token = r?.data?.token
      if (!token) {
        message.error('登录响应缺少 token，请联系管理员')
        return
      }
      setToken(token)
      const user = r?.data?.user
      // 权限表预拉（menus/tree 派生；失败不阻断进入——Layout 挂载后会兜底重派生）
      let perms = []
      try {
        perms = (await menuPermissions()) || []
      } catch {
        /* 预拉失败交由 Layout 兜底 */
      }
      loginSuccess(user, perms)
      navigate('/', { replace: true })
    } catch {
      // 业务/网络错误提示已由 http 拦截器统一弹出，此处只终止 loading
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#f0f2f5',
      }}
    >
      <Card title="权限管理系统" style={{ width: 360 }}>
        <Form layout="vertical" onFinish={onFinish} autoComplete="off">
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password placeholder="密码" autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  )
}
