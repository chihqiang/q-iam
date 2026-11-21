// 权限策略 API
import { get, post, put, del } from './client'
import type { Policy, Paginated } from '@/types'

export interface PolicyListParams {
  page: number
  size: number
  type?: string
  key?: string
  status?: boolean
}

export interface PolicyCreatePayload {
  name: string
  description?: string
  status?: boolean
  // 关联的授权语句 ID 列表（语句池共享引用，至少一条）
  statement_ids: number[]
}

export interface PolicyUpdatePayload {
  description?: string
  status?: boolean
  // 关联的授权语句 ID 列表（传 null/undefined 表示不修改关联，传数组则整体替换）
  statement_ids?: number[] | null
}

export const listPolicies = (params: PolicyListParams) =>
  get<Paginated<Policy>>('/policies', params as unknown as Record<string, unknown>)

// 全部启用策略（授权选择用）
export const allPolicies = () => get<Policy[]>('/policies/all')

export const getPolicy = (id: number) => get<Policy>(`/policies/${id}`)

export const createPolicy = (payload: PolicyCreatePayload) => post<Policy>('/policies', payload)

export const updatePolicy = (id: number, payload: PolicyUpdatePayload) =>
  put<Policy>(`/policies/${id}`, payload)

export const deletePolicy = (id: number) => del<void>(`/policies/${id}`)

