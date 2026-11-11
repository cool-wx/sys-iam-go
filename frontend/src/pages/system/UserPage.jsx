/**
 * 用户管理页。
 *
 * 契约（Go 后端 /api/v1/users，internal/handler/user.go + internal/model/dto/user.go 实测）：
 * - GET /users?page=&size= → data={items,total,...}（分页信封，api 层归一 {data, total}）
 * - UserResp 字段：username/realName/deptName/status(枚举 name: ENABLED|DISABLED)/roles/builtin
 * - 写端点：POST /users {username!,password!,realName!,...} / PUT /users/:id {realName?,...,
 *   builtin?} / PUT /users/:id/status {status} / PUT /users/:id/reset-password / DELETE /users/:id
 * - 角色装配走子资源 PUT /users/:id/roles {roleIds}（创建/编辑请求体无生效的 roleIds 键——
 *   主请求成功后按需补调，见 submitModal）；builtin 编辑直传生效、创建补调落实
 * - 权限标识与后端 RequirePerm 常量逐字对齐（perms.go USER_*：list/query/add/edit/remove/resetPwd）
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { App, Button, Form, Input, Modal, Popconfirm, Select, Space, Switch, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import CrudTable from '../../components/CrudTable.jsx'
import PermButton from '../../components/PermButton.jsx'
import {
  addUser,
  assignUserRoles,
  deptTree,
  deleteUser,
  editUser,
  listRoles,
  listUsers,
  resetPwd,
  toggleStatus,
} from '../../api/iam.js'

const PERM = {
  list: 'system:user:list',
  add: 'system:user:add',
  edit: 'system:user:edit',
  remove: 'system:user:remove',
  resetPwd: 'system:user:resetPwd',
}

function StatusTag({ value }) {
  return value === 'ENABLED' ? (
    <Tag color="success">启用</Tag>
  ) : (
    <Tag color="error">停用</Tag>
  )
}

export default function UserPage() {
  const { message } = App.useApp()
  const tableRef = useRef(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增，否则 UserVo 行
  const [submitting, setSubmitting] = useState(false)
  const [deptOptions, setDeptOptions] = useState([]) // 部门下拉（后端 add/edit 必填 deptId）
  const [roleOptions, setRoleOptions] = useState([]) // 角色下拉（后端 roleIds 早已支持，前端此前未接）
  const [form] = Form.useForm()

  // 部门树拉平为下拉选项（DeptController GET /dept/tree；deptId 后端必填）
  useEffect(() => {
    deptTree()
      .then((r) => {
        const flat = []
        const walk = (nodes) => nodes?.forEach((n) => { flat.push({ value: Number(n.id), label: n.deptName }); walk(n.children) })
        walk(r?.data)
        setDeptOptions(flat)
      })
      .catch(() => {})
  }, [])

  // 角色下拉（RoleController GET /role/list 为分页端点，无全量端点 → 取大页宽）
  useEffect(() => {
    listRoles({ page: 1, size: 200 })
      .then((r) => {
        const list = Array.isArray(r?.data) ? r.data : []
        setRoleOptions(list.map((role) => ({ value: Number(role.id), label: role.roleName })))
      })
      .catch(() => {})
  }, [])

  const fetchPage = useCallback((page, size) => listUsers({ page, size }), [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (row) => {
    setEditing(row)
    setModalOpen(true)
    // Modal 挂载后回填（destroyOnHidden 下表单随弹窗重建，resetFields 置空 → 必须显式回填，
    // 否则空表单保存会把 realName/phone/email/deptId 清成 null，角色与内置标志同样会被清掉）
    setTimeout(() => {
      form.setFieldsValue({
        realName: row.realName,
        phone: row.phone,
        email: row.email,
        deptId: row.deptId != null ? Number(row.deptId) : undefined,
        roleIds: (row.roles || []).map((r) => Number(r.roleId)),
        builtin: !!row.builtin,
      })
    }, 0)
  }

  /** 行数据 roles → 排序后的 roleId 数字数组（id 出网为带引号字符串，统一转数字比较） */
  const roleIdsOf = (row) =>
    (row?.roles || []).map((r) => Number(r.roleId)).sort((a, b) => a - b)

  const submitModal = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await editUser({ id: editing.id, ...values })
        // 角色装配子资源：roleIds 相对行数据变化时补调（主请求体 roleIds 键被后端
        // 白名单忽略，PUT /users/:id/roles 才是生效通道）；失败弹错不阻断（主编辑已成功）
        const prevIds = roleIdsOf(editing)
        const nextIds = (values.roleIds || []).map(Number).sort((a, b) => a - b)
        const changed =
          prevIds.length !== nextIds.length ||
          prevIds.some((id, i) => id !== nextIds[i])
        if (changed) {
          try {
            await assignUserRoles(editing.id, values.roleIds || [])
          } catch {
            message.error('用户已保存，但角色分配失败，请在编辑中重试')
          }
        }
        message.success('修改用户成功')
      } else {
        const r = await addUser(values)
        const newId = r?.data
        // 创建补充装配（Go UserCreateReq 无生效的 roleIds/builtin 键，未知键静默忽略——
        // 选了角色补调角色子资源；内置标志经 PUT /users/:id {builtin} 落实）。
        // 两路补充失败各自弹错不阻断（用户已创建成功）
        if (Array.isArray(values.roleIds) && values.roleIds.length) {
          try {
            await assignUserRoles(newId, values.roleIds)
          } catch {
            message.error('用户已创建，但角色分配失败，请在编辑中重试')
          }
        }
        if (values.builtin) {
          try {
            await editUser({ id: newId, builtin: true })
          } catch {
            message.error('用户已创建，但内置标志设置失败，请在编辑中重试')
          }
        }
        message.success('新增用户成功')
      }
      setModalOpen(false)
      tableRef.current?.refresh()
    } catch {
      // 错误提示已由 http 拦截器统一弹出（表单校验错误不进此 catch）
    } finally {
      setSubmitting(false)
    }
  }

  const onToggleStatus = async (row) => {
    const next = row.status === 'ENABLED' ? 'DISABLED' : 'ENABLED'
    await toggleStatus(row.id, next)
    message.success(next === 'ENABLED' ? '已启用' : '已停用')
    tableRef.current?.refresh()
  }

  const onResetPwd = async (row) => {
    await resetPwd(row.id)
    message.success(`已重置「${row.username}」的密码`)
  }

  const onDelete = async (row) => {
    // 体验层守卫（权威拦截在后端 IllegalArgumentException → 400）
    if (row.builtin) {
      message.warning('系统内置用户不可删除')
      return
    }
    await deleteUser(row.id)
    message.success('删除用户成功')
    tableRef.current?.refresh()
  }

  const columns = [
    { title: '用户名', dataIndex: 'username' },
    { title: '姓名', dataIndex: 'realName' },
    { title: '部门', dataIndex: 'deptName' },
    {
      title: '角色',
      dataIndex: 'roles',
      render: (roles) =>
        roles?.length ? roles.map((r) => <Tag key={r.roleId}>{r.roleName}</Tag>) : '—',
    },
    {
      title: '系统内置',
      dataIndex: 'builtin',
      width: 100,
      render: (v) => (v ? <Tag color="warning">内置</Tag> : '—'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v) => <StatusTag value={v} />,
    },
    {
      title: '操作',
      key: 'actions',
      width: 280,
      render: (_, row) => (
        <Space size="small" wrap>
          <PermButton perm={PERM.edit} size="small" onClick={() => openEdit(row)}>
            编辑
          </PermButton>
          <PermButton perm={PERM.edit} size="small" onClick={() => onToggleStatus(row)}>
            {row.status === 'ENABLED' ? '停用' : '启用'}
          </PermButton>
          <Popconfirm
            title="确认重置该用户密码？"
            onConfirm={() => onResetPwd(row)}
            okText="确认"
            cancelText="取消"
          >
            <PermButton perm={PERM.resetPwd} size="small">
              重置密码
            </PermButton>
          </Popconfirm>
          {row.builtin ? (
            <PermButton
              perm={PERM.remove}
              size="small"
              danger
              disabled
              title="系统内置用户不可删除"
            >
              删除
            </PermButton>
          ) : (
            <Popconfirm
              title="确认删除该用户？"
              description="删除后不可恢复"
              onConfirm={() => onDelete(row)}
              okText="确认"
              cancelText="取消"
            >
              <PermButton perm={PERM.remove} size="small" danger>
                删除
              </PermButton>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <PermButton perm={PERM.add} type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新增用户
        </PermButton>
      </Space>

      <CrudTable ref={tableRef} fetchPage={fetchPage} columns={columns} />

      <Modal
        title={editing ? `编辑用户：${editing.username}` : '新增用户'}
        open={modalOpen}
        onOk={submitModal}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        okText="保存"
        cancelText="取消"
        destroyOnHidden
      >
        <Form form={form} layout="vertical" autoComplete="off">
          {!editing && (
            <>
              <Form.Item
                name="username"
                label="用户名"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input placeholder="登录用户名" />
              </Form.Item>
              <Form.Item
                name="password"
                label="初始密码"
                rules={[{ required: true, message: '请输入初始密码' }]}
              >
                <Input.Password placeholder="初始密码" />
              </Form.Item>
            </>
          )}
          <Form.Item
            name="deptId"
            label="部门"
            rules={[{ required: true, message: '请选择部门' }]}
          >
            <Select placeholder="选择部门" options={deptOptions} allowClear={false} />
          </Form.Item>
          <Form.Item name="realName" label="姓名">
            <Input placeholder="真实姓名" />
          </Form.Item>
          <Form.Item
            name="phone"
            label="手机号"
            rules={[{ pattern: /^1\d{10}$/, message: '手机号格式不正确' }]}
          >
            <Input placeholder="手机号（选填）" />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[{ type: 'email', message: '邮箱格式不正确' }]}
          >
            <Input placeholder="邮箱（选填）" />
          </Form.Item>
          <Form.Item name="roleIds" label="角色">
            <Select mode="multiple" placeholder="选择角色" options={roleOptions} allowClear />
          </Form.Item>
          <Form.Item name="builtin" label="系统内置" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
