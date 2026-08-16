// 数据权限 API（子系统按需拉取当前主体的权限规则 + 数据范围）
import { get } from './client'
import type { DataScope } from '@/types'

export interface DataPermissionStatement {
  effect: 'Allow' | 'Deny'
  action: string
  source?: string
  data_scopes?: DataScope[]
}

export interface DataPermissionUser {
  account_id: number
  account_name: string
  display_name: string
  group_ids: number[]
}

export interface DataPermissionApp {
  app_id: string
  name: string
}

export interface DataPermissions {
  subject_type: 'user' | 'app'
  user?: DataPermissionUser
  app?: DataPermissionApp
  permissions: DataPermissionStatement[]
}

// 当前主体的权限 + 数据范围（Bearer 令牌）
export const getDataPermissions = () => get<DataPermissions>('/auth/data-permissions')
