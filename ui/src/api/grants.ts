// 授权管理 API
import { get, post, del } from './client'
import type { Policy, PrincipalType } from '@/types'

export interface GrantPayload {
  principal_type: PrincipalType
  principal_id: number
  policy_ids: number[]
}

export interface RevokePayload {
  principal_type: PrincipalType
  principal_id: number
  policy_ids?: number[]
}

export interface PrincipalAttachment {
  id: number
  principal_type: PrincipalType
  principal_id: number
  policy_id: number
  created_by: number
  created_at: string
}

// 查询某主体已绑定的策略
export const listPoliciesByPrincipal = (type: PrincipalType, id: number) =>
  get<Policy[]>(`/grants/principals/${type}/${id}`)

// 查询某策略被哪些主体绑定
export const listPrincipalsByPolicy = (policyId: number) =>
  get<PrincipalAttachment[]>(`/grants/policies/${policyId}`)

export const grantPolicies = (payload: GrantPayload) => post<void>('/grants', payload)

export const revokePolicies = (payload: RevokePayload) => del<void>('/grants', payload)
