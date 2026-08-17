// 权限策略 API
import { get, post, put, del } from './client'
import type { Policy, Paginated, PolicyStatement } from '@/types'

export interface PolicyListParams {
  page: number
  size: number
  type?: string
  key?: string
  status?: boolean
}

// 策略语句数据范围 DTO（对齐后端 PolicyScopeDTO）
export interface PolicyScopeDTO {
  scope_type: 'all' | 'group' | 'self' | 'attribute'
  group_id?: number
  owner_field?: string
  attr_key?: string
  attr_value?: string
  sort: number
}

// 策略语句 DTO（对齐后端 PolicyStatementDTO）
export interface PolicyStatementDTO {
  // 语句描述（小标题，说明本条授权规则的用途）
  description?: string
  effect: 'Allow' | 'Deny'
  action: string
  // 资源（支持 * 通配，默认 * 表示全部资源）
  resource?: string
  // 数据范围（数据权限）
  scopes: PolicyScopeDTO[]
  sort: number
}

export interface PolicyCreatePayload {
  name: string
  description?: string
  status?: boolean
  statements: PolicyStatementDTO[]
}

export interface PolicyUpdatePayload {
  description?: string
  status?: boolean
  statements?: PolicyStatementDTO[] | null
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

// 从后端模型转换为编辑表单用的语句 DTO
export function statementsToDTO(statements: PolicyStatement[]): PolicyStatementDTO[] {
  return (statements ?? []).map((s, si) => ({
    description: s.description ?? '',
    effect: s.effect,
    action: s.action,
    scopes: (s.scopes ?? []).map((sc, sci) => ({
      scope_type: sc.scope_type,
      group_id: sc.group_id,
      owner_field: sc.owner_field,
      attr_key: sc.attr_key,
      attr_value: sc.attr_value,
      sort: sci,
    })),
    sort: si,
  }))
}
