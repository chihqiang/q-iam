// 账号管理 API
import { get, post, put, del } from './client'
import type { Account, Paginated } from '@/types'

export interface AccountListParams {
  page: number
  size: number
  key?: string
  status?: boolean
}

export interface AccountCreatePayload {
  account_name: string
  display_name?: string
  email?: string
  mobile?: string
  password: string
  status?: boolean
  allow_console?: boolean
  remark?: string
  group_ids?: number[]
}

export interface AccountUpdatePayload {
  display_name?: string
  email?: string
  mobile?: string
  status?: boolean
  allow_console?: boolean
  remark?: string
  group_ids?: number[] | null
}

export interface ResetPasswordPayload {
  password: string
}

export const listAccounts = (params: AccountListParams) =>
  get<Paginated<Account>>('/accounts', params as unknown as Record<string, unknown>)

// 全部启用账号（授权下拉选择用，避免分页 size 上限导致主体被截断）
export const allAccounts = () => get<Account[]>('/accounts/all')

export const getAccount = (id: number) => get<Account>(`/accounts/${id}`)

export const createAccount = (payload: AccountCreatePayload) => post<Account>('/accounts', payload)

export const updateAccount = (id: number, payload: AccountUpdatePayload) =>
  put<Account>(`/accounts/${id}`, payload)

export const deleteAccount = (id: number) => del<void>(`/accounts/${id}`)

export const resetAccountPassword = (id: number, payload: ResetPasswordPayload) =>
  put<void>(`/accounts/${id}/reset-password`, payload)

export const changeAccountPassword = (
  id: number,
  payload: { old_password: string; new_password: string }
) => put<void>(`/accounts/${id}/password`, payload)
