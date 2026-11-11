/**
 * 字典管理页（域6 第 4 个业务页）——主从布局。
 *
 * 契约（DictTypeController/DictDataController 源码实测 2026-09-28）：
 * - GET /system/dictType/list → 全量（无分页；dictName/dictType/status）
 * - GET /system/dictData/list?dictTypeId= → 全量（可选过滤；dictTypeId/label/value/sort）
 * - 写端点均为 POST add / PUT edit（实体直传）/ DELETE delete/{id}
 * - 枚举 status 序列化 name（'ENABLED'/'DISABLED'，UserPage 实测口径）
 */
import { useCallback, useEffect, useState } from 'react'
import {
  App,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Tag,
} from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import PermButton from '../../components/PermButton.jsx'
import {
  addDictData,
  addDictType,
  deleteDictData,
  deleteDictType,
  editDictData,
  editDictType,
  listDictData,
  listDictTypes,
} from '../../api/iam.js'

const PERM = {
  add: 'system:dict:add',
  edit: 'system:dict:edit',
  remove: 'system:dict:remove',
}

const STATUS_OPTIONS = [
  { value: 'ENABLED', label: '正常' },
  { value: 'DISABLED', label: '禁用' },
]

function StatusTag({ value }) {
  return value === 'ENABLED' ? (
    <Tag color="success">正常</Tag>
  ) : (
    <Tag color="error">禁用</Tag>
  )
}

export default function DictPage() {
  const { message } = App.useApp()
  // —— 主表（dictType）——
  const [types, setTypes] = useState([])
  const [typesLoading, setTypesLoading] = useState(false)
  const [selectedType, setSelectedType] = useState(null) // 点选的字典类型行
  const [typeModalOpen, setTypeModalOpen] = useState(false)
  const [editingType, setEditingType] = useState(null)
  const [typeSubmitting, setTypeSubmitting] = useState(false)
  const [typeForm] = Form.useForm()

  // —— 从表（dictData）——
  const [datas, setDatas] = useState([])
  const [datasLoading, setDatasLoading] = useState(false)
  const [dataModalOpen, setDataModalOpen] = useState(false)
  const [editingData, setEditingData] = useState(null)
  const [dataSubmitting, setDataSubmitting] = useState(false)
  const [dataForm] = Form.useForm()

  const reloadTypes = useCallback(async () => {
    setTypesLoading(true)
    try {
      const r = await listDictTypes()
      const rows = Array.isArray(r?.data) ? r.data : []
      setTypes(rows)
      // 当前选中项被删后清空选中
      setSelectedType((prev) => (prev && rows.some((t) => t.id === prev.id) ? prev : null))
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setTypesLoading(false)
    }
  }, [])

  const reloadDatas = useCallback(async (dictTypeId) => {
    if (dictTypeId == null) {
      setDatas([])
      return
    }
    setDatasLoading(true)
    try {
      const r = await listDictData(dictTypeId)
      setDatas(Array.isArray(r?.data) ? r.data : [])
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setDatasLoading(false)
    }
  }, [])

  useEffect(() => {
    reloadTypes()
  }, [reloadTypes])

  useEffect(() => {
    reloadDatas(selectedType?.id ?? null)
  }, [selectedType, reloadDatas])

  // —— dictType CRUD ——
  const openTypeCreate = () => {
    setEditingType(null)
    typeForm.resetFields()
    setTypeModalOpen(true)
  }

  const openTypeEdit = (row) => {
    setEditingType(row)
    typeForm.resetFields()
    typeForm.setFieldsValue({ dictName: row.dictName, dictType: row.dictType, status: row.status })
    setTypeModalOpen(true)
  }

  const submitTypeModal = async () => {
    const values = await typeForm.validateFields()
    setTypeSubmitting(true)
    try {
      if (editingType) {
        await editDictType({ id: editingType.id, ...values })
        message.success('修改字典类型成功')
      } else {
        await addDictType(values)
        message.success('新增字典类型成功')
      }
      setTypeModalOpen(false)
      reloadTypes()
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setTypeSubmitting(false)
    }
  }

  const onDeleteType = async (row) => {
    await deleteDictType(row.id)
    message.success('删除字典类型成功')
    reloadTypes()
  }

  // —— dictData CRUD（挂当前选中类型）——
  const openDataCreate = () => {
    setEditingData(null)
    dataForm.resetFields()
    setDataModalOpen(true)
  }

  const openDataEdit = (row) => {
    setEditingData(row)
    dataForm.resetFields()
    dataForm.setFieldsValue({ label: row.label, value: row.value, sort: row.sort })
    setDataModalOpen(true)
  }

  const submitDataModal = async () => {
    const values = await dataForm.validateFields()
    setDataSubmitting(true)
    try {
      if (editingData) {
        await editDictData({ id: editingData.id, ...values })
        message.success('修改字典数据成功')
      } else {
        await addDictData({ ...values, dictTypeId: selectedType.id })
        message.success('新增字典数据成功')
      }
      setDataModalOpen(false)
      reloadDatas(selectedType.id)
    } catch {
      // 错误提示已由 http 拦截器统一弹出
    } finally {
      setDataSubmitting(false)
    }
  }

  const onDeleteData = async (row) => {
    await deleteDictData(row.id)
    message.success('删除字典数据成功')
    reloadDatas(selectedType.id)
  }

  const typeColumns = [
    { title: '字典名称', dataIndex: 'dictName' },
    { title: '类型编码', dataIndex: 'dictType' },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v) => <StatusTag value={v} />,
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: (_, row) => (
        <Space size="small">
          <PermButton
            perm={PERM.edit}
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              openTypeEdit(row)
            }}
          >
            编辑
          </PermButton>
          <Popconfirm
            title="确认删除该字典类型？"
            description="类型下全部字典数据将失去归属"
            onConfirm={(e) => {
              e?.stopPropagation()
              onDeleteType(row)
            }}
            okText="确认"
            cancelText="取消"
          >
            <PermButton
              perm={PERM.remove}
              size="small"
              danger
              onClick={(e) => e.stopPropagation()}
            >
              删除
            </PermButton>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const dataColumns = [
    { title: '标签', dataIndex: 'label' },
    { title: '键值', dataIndex: 'value' },
    { title: '排序', dataIndex: 'sort', width: 80 },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: (_, row) => (
        <Space size="small">
          <PermButton perm={PERM.edit} size="small" onClick={() => openDataEdit(row)}>
            编辑
          </PermButton>
          <Popconfirm
            title="确认删除该字典数据？"
            onConfirm={() => onDeleteData(row)}
            okText="确认"
            cancelText="取消"
          >
            <PermButton perm={PERM.remove} size="small" danger>
              删除
            </PermButton>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Row gutter={16}>
      <Col span={10}>
        <Space style={{ marginBottom: 12 }}>
          <PermButton perm={PERM.add} type="primary" icon={<PlusOutlined />} onClick={openTypeCreate}>
            新增类型
          </PermButton>
        </Space>
        <Table
          rowKey="id"
          size="small"
          columns={typeColumns}
          dataSource={types}
          loading={typesLoading}
          pagination={false}
          rowClassName={(row) => (selectedType?.id === row.id ? 'ant-table-row-selected' : '')}
          onRow={(row) => ({ onClick: () => setSelectedType(row) })}
          locale={{ emptyText: '暂无字典类型' }}
        />
      </Col>
      <Col span={14}>
        <Space style={{ marginBottom: 12 }}>
          <PermButton
            perm={PERM.add}
            type="primary"
            icon={<PlusOutlined />}
            disabled={!selectedType}
            onClick={openDataCreate}
          >
            新增数据{selectedType ? `（${selectedType.dictName}）` : ''}
          </PermButton>
          {!selectedType && <span style={{ color: '#999' }}>← 点选左侧字典类型查看明细</span>}
        </Space>
        <Table
          rowKey="id"
          size="small"
          columns={dataColumns}
          dataSource={datas}
          loading={datasLoading}
          pagination={false}
          locale={{ emptyText: selectedType ? '该类型暂无字典数据' : '请先点选左侧字典类型' }}
        />
      </Col>

      {/* dictType 新增/编辑 */}
      <Modal
        title={editingType ? `编辑字典类型：${editingType.dictName || ''}` : '新增字典类型'}
        open={typeModalOpen}
        onOk={submitTypeModal}
        onCancel={() => setTypeModalOpen(false)}
        confirmLoading={typeSubmitting}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={typeForm} layout="vertical" autoComplete="off">
          <Form.Item
            name="dictName"
            label="字典名称"
            rules={[{ required: true, message: '请输入字典名称' }]}
          >
            <Input placeholder="如：启用状态" />
          </Form.Item>
          <Form.Item
            name="dictType"
            label="类型编码"
            rules={[{ required: true, message: '请输入类型编码' }]}
          >
            <Input placeholder="如：usage_status" />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue="ENABLED">
            <Select options={STATUS_OPTIONS} />
          </Form.Item>
        </Form>
      </Modal>

      {/* dictData 新增/编辑 */}
      <Modal
        title={
          editingData
            ? `编辑字典数据：${editingData.label || ''}`
            : `新增字典数据${selectedType ? `（${selectedType.dictName}）` : ''}`
        }
        open={dataModalOpen}
        onOk={submitDataModal}
        onCancel={() => setDataModalOpen(false)}
        confirmLoading={dataSubmitting}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={dataForm} layout="vertical" autoComplete="off">
          <Form.Item
            name="label"
            label="显示标签"
            rules={[{ required: true, message: '请输入显示标签' }]}
          >
            <Input placeholder="如：正常" />
          </Form.Item>
          <Form.Item
            name="value"
            label="实际值"
            rules={[{ required: true, message: '请输入实际值' }]}
          >
            <Input placeholder="如：ENABLED" />
          </Form.Item>
          <Form.Item name="sort" label="排序">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="数字越小越靠前" />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
