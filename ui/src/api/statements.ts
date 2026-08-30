// 授权语句（语句池）API —— 独立菜单维护授权规则，策略新增/编辑只负责关联
import { get, post, put, del } from './client'
import type { Statement, Paginated, Effect } from '@/types'
import { EFFECT_ALLOW, EFFECT_DENY } from '@/types'

// 语句数据范围 DTO（对齐后端 ScopeDTO）
export interface ScopeDTO {
  scope_type: 'all' | 'group' | 'self' | 'attribute'
  // group_id 可为 null（Select 清空时写回），提交前由组件归一为 undefined
  group_id?: number | null
  owner_field?: string
  attr_key?: string
  attr_value?: string
  sort: number
}

// 授权语句 DTO（对齐后端 StatementDTO）
export interface StatementDTO {
  // 语句描述（小标题，说明本条授权规则的用途）
  description?: string
  effect: Effect
  action: string
  // 资源（支持 * 通配，默认 * 表示全部资源）
  resource?: string
  // 数据范围（数据权限）
  scopes: ScopeDTO[]
  sort: number
}

export interface StatementListParams {
  page: number
  size: number
  key?: string
  effect?: string
}

export interface StatementCreatePayload extends StatementDTO {}

export interface StatementUpdatePayload extends StatementDTO {}

export const listStatements = (params: StatementListParams) =>
  get<Paginated<Statement>>('/statements', params as unknown as Record<string, unknown>)

// 全部语句（策略关联选择用）
export const allStatements = () => get<Statement[]>('/statements/all')

export const getStatement = (id: number) => get<Statement>(`/statements/${id}`)

export const createStatement = (payload: StatementCreatePayload) =>
  post<Statement>('/statements', payload)

export const updateStatement = (id: number, payload: StatementUpdatePayload) =>
  put<Statement>(`/statements/${id}`, payload)

export const deleteStatement = (id: number) => del<void>(`/statements/${id}`)

// 从后端模型转换为编辑表单用的语句 DTO
// 后端 effect 存储为标准值（Allow/Deny），直接透传
function normalizeEffect(effect: Effect | undefined): Effect {
  return effect === EFFECT_DENY ? EFFECT_DENY : EFFECT_ALLOW
}

export function statementToDTO(s: Statement): StatementDTO {
  return {
    description: s.description ?? '',
    effect: normalizeEffect(s.effect),
    action: s.action,
    resource: s.resource ?? '*',
    scopes: (s.scopes ?? []).map((sc, sci) => ({
      scope_type: sc.scope_type,
      group_id: sc.group_id,
      owner_field: sc.owner_field,
      attr_key: sc.attr_key,
      attr_value: sc.attr_value,
      sort: sci,
    })),
    sort: s.sort,
  }
}
