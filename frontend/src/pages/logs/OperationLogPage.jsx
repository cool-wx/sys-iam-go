/**
 * 操作日志页：GET /api/v1/logs（sys_operation_log 分页查询，UI 增删改审计——
 * 写路由经 Oplog 中间件声明制落库）。
 *
 * params 内容为后端 Oplog 中间件写入的脱敏后请求参数（后端既有脱敏，
 * 前端原样渲染不二次加工）；duration 单位毫秒；分页特例缺省 1/10 由后端钳位。
 */
import { useCallback } from 'react'
import { ReloadOutlined } from '@ant-design/icons'
import { Space, Tag, Tooltip } from 'antd'
import CrudTable from '../../components/CrudTable.jsx'
import PermButton from '../../components/PermButton.jsx'
import { operationLog } from '../../api/iam.js'

const PERM = 'system:log:list'

const COLUMNS = [
  { title: '操作时间', dataIndex: 'createTime', width: 175, render: (v) => v || '-' },
  { title: '操作人', dataIndex: 'realName', width: 100, render: (v) => v || '-' },
  { title: '模块', dataIndex: 'module', width: 170, ellipsis: true, render: (v) => v || '-' },
  { title: '操作', dataIndex: 'operation', width: 150, ellipsis: true, render: (v) => v || '-' },
  {
    title: '内容摘要',
    dataIndex: 'params',
    ellipsis: { showTitle: false },
    render: (v) => (
      <Tooltip placement="topLeft" title={v || undefined}>
        {v || '-'}
      </Tooltip>
    ),
  },
  { title: 'IP', dataIndex: 'ip', width: 130, render: (v) => v || '-' },
  { title: '耗时', dataIndex: 'duration', width: 90, render: (v) => (v == null ? '-' : `${v} ms`) },
  {
    title: '状态',
    dataIndex: 'status',
    width: 90,
    render: (v, row) =>
      v === 1 ? (
        <Tag color="success">成功</Tag>
      ) : v === 0 ? (
        <Tooltip title={row.errorMsg || '失败'}>
          <Tag color="error">失败</Tag>
        </Tooltip>
      ) : (
        '-'
      ),
  },
]

/** 操作日志表（OlogResp 载体，分页信封 data={items,total} 口径适配）。 */
export default function OperationLogPage() {
  const fetchPage = useCallback(
    (page, size) =>
      operationLog({ page, size }).then((r) => ({
        data: r?.data || [],
        total: Number(r?.total) || 0,
      })),
    [],
  )
  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <div>
        <PermButton perm={PERM} icon={<ReloadOutlined />}>
          刷新
        </PermButton>
      </div>
      <CrudTable
        fetchPage={fetchPage}
        columns={COLUMNS}
        rowKey={(_, i) => i}
        locale={{ emptyText: '暂无操作日志' }}
      />
    </Space>
  )
}
