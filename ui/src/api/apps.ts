// 应用管理 API
import { get, post, put, del } from './client'
import type { AppItem, AppCreateResponse, GrantType, Paginated } from '@/types'

export interface AppListParams {
  page: number
  size: number
  key?: string
  status?: boolean
}

export interface AppCreatePayload {
  name: string
  description?: string
  grant_type?: GrantType
  callback_url?: string
  owner_account_id?: number
  status?: boolean
}

export interface AppUpdatePayload {
  name?: string
  description?: string
  grant_type?: GrantType
  callback_url?: string
  status?: boolean
}

export const listApps = (params: AppListParams) =>
  get<Paginated<AppItem>>('/apps', params as unknown as Record<string, unknown>)

// 全部启用应用（授权下拉选择用，避免分页 size 上限导致主体被截断）
export const allApps = () => get<AppItem[]>('/apps/all')

export const getApp = (id: number) => get<AppItem>(`/apps/${id}`)

export const createApp = (payload: AppCreatePayload) => post<AppCreateResponse>('/apps', payload)

export const updateApp = (id: number, payload: AppUpdatePayload) =>
  put<AppItem>(`/apps/${id}`, payload)

export const deleteApp = (id: number) => del<void>(`/apps/${id}`)

export const resetAppSecret = (id: number) =>
  post<{ app_id: string; app_secret: string }>(`/apps/${id}/reset-secret`)
