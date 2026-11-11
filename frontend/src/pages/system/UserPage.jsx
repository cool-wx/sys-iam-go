/**
 * 用户管理页（域6 第一个业务 CRUD 页）。
 *
 * 契约（UserController 源码实测）：
 * - GET /system/user/list?page=&size= → data=UserVo[] + 顶层 total
 * - UserVo 字段：username/realName/deptName/status(枚举 name: ENABLED|DISABLED)/roles
 * - 写端点：POST add {username,password,realName?,phone?,email?} / PUT edit {id,...} /
 *   PUT status {id,status} / PUT resetPwd/{id} / DELETE delete/{id}
 * - 权限标识与后端 SystemPermissions 常量逐字对齐（system:user:list/query/add/edit/remove/resetPwd）
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { App, Button, Form, Input, Modal, Popconfirm, Select, Space, Switch, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import CrudTable from '../../components/CrudTable.jsx'
import PermButton from '../../components/PermButton.jsx'
import {
  addUser,
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

  const submitModal = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      if (editing) {
        await editUser({ id: editing.id, ...values })
        message.success('修改用户成功')
      } else {
        await addUser(values)
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
