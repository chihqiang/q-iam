// 操作审计 API
import { get } from './client'
import type { Paginated } from '@/types'

export interface AuditLogItem {
  id: number
  operator_id: number
  operator_name: string
  module: string
  action: string
  method: string
  path: string
  detail: string
  client_ip: string
  user_agent: string
  success: boolean
  error_msg: string
  latency_ms: number
  created_at: string
}

export interface AuditListParams {
  page: number
  size: number
  key?: string
  module?: string
  action?: string
  success?: boolean
  operator?: string
  from?: string
  to?: string
}

export const listAuditLogs = (params: AuditListParams) =>
  get<Paginated<AuditLogItem>>('/audit-logs', params as unknown as Record<string, unknown>)

export const auditModules = () => get<string[]>('/audit-logs/modules')

// 模块中文名
export const MODULE_LABELS: Record<string, string> = {
  auth: '认证',
  account: '账号',
  group: '账号组',
  policy: '策略',
  grant: '授权',
  app: '应用',
  oauth: 'OAuth',
  audit: '审计',
  system: '系统',
}

// 动作中文名
export const ACTION_LABELS: Record<string, string> = {
  login: '登录',
  register: '注册',
  refresh: '刷新令牌',
  logout: '退出登录',
  token: '换取令牌',
  create: '创建',
  update: '更新',
  delete: '删除',
  grant: '授权',
  revoke: '解除授权',
  authorize: 'OAuth 授权',
  reset_secret: '重置密钥',
  reset_password: '重置密码',
  change_password: '修改密码',
  add_member: '添加成员',
  remove_member: '移除成员',
  replace_member: '替换成员',
  cleanup: '数据清理',
  unknown: '未知',
}
