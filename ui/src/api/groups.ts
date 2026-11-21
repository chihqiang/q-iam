// 账号组 API
import { get, post, put, del } from './client'
import type { Group, Paginated } from '@/types'

export interface GroupListParams {
  page: number
  size: number
  key?: string
  status?: boolean
}

export interface GroupCreatePayload {
  name: string
  display_name?: string
  description?: string
  status?: boolean
}

export interface GroupUpdatePayload {
  display_name?: string
  description?: string
  status?: boolean
}

export const listGroups = (params: GroupListParams) =>
  get<Paginated<Group>>('/groups', params as unknown as Record<string, unknown>)

// 全部启用账号组（下拉选择用）
export const allGroups = () => get<Group[]>('/groups/all')

export const getGroup = (id: number) => get<Group>(`/groups/${id}`)

export const createGroup = (payload: GroupCreatePayload) => post<Group>('/groups', payload)

export const updateGroup = (id: number, payload: GroupUpdatePayload) =>
  put<Group>(`/groups/${id}`, payload)

export const deleteGroup = (id: number) => del<void>(`/groups/${id}`)

export const replaceGroupMembers = (id: number, account_ids: number[]) =>
  put<void>(`/groups/${id}/members`, { account_ids })

export const addGroupMembers = (id: number, account_ids: number[]) =>
  post<void>(`/groups/${id}/members`, { account_ids })

export const removeGroupMembers = (id: number, account_ids: number[]) =>
  del<void>(`/groups/${id}/members`, { account_ids })
