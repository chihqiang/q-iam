// q-iam 类型定义（对齐后端 DTO）

// ===== 认证 =====

export interface LoginResponse {
  id: number
  account_name: string
  display_name: string
  access_token: string
  token_type: string
  // 访问令牌有效秒数（OAuth 标准语义），过期时间戳需前端自行计算
  expires_in: number
  refresh_token?: string
  allow_console: boolean
  // 密码是否已超过有效期（password_policy.expire_days），为 true 时应引导立即修改
  password_expired?: boolean
}

export interface TokenResponse {
  access_token: string
  token_type: string
  // 访问令牌有效秒数
  expires_in: number
  scope?: string
}

export interface Profile {
  id: number
  account_name: string
  display_name: string
  email: string
  mobile: string
  status: boolean
  remark: string
  allow_console: boolean
  // 密码是否已超过有效期（password_policy.expire_days）
  password_expired?: boolean
  // 当前账号生效的权限规则（供菜单/按钮按权限过滤）
  permissions?: PermissionStatement[]
}

// 一条生效的权限规则（由策略解析而来，含来源策略名与数据范围）
export interface PermissionStatement {
  effect: Effect
  action: string
  source?: string
  data_scopes?: DataScope[]
}

// ===== 身份管理 =====

export interface Account {
  id: number
  account_name: string
  display_name: string
  email: string | null
  mobile: string | null
  status: boolean
  allow_console: boolean
  remark: string
  created_at: string
  groups?: Group[]
}

export interface Group {
  id: number
  name: string
  display_name: string
  description: string
  status: boolean
  created_at?: string
  accounts?: Account[]
}

// ===== 策略 / 授权语句（语句池）=====

// 授权效果常量（统一引用，避免字符串字面量散落各处）。
// 后端存储与对外展示统一为这两个标准值。
export const EFFECT_ALLOW = 'Allow' as const
export const EFFECT_DENY = 'Deny' as const

export type Effect = typeof EFFECT_ALLOW | typeof EFFECT_DENY
export type PrincipalType = 'account' | 'group' | 'app'

export interface Policy {
  id: number
  name: string
  version: string
  description: string
  type: 'system' | 'custom'
  status: boolean
  // 关联的授权语句（语句池共享引用，策略只负责关联）
  statements: Statement[]
}

// 授权语句（语句池，独立菜单管理）
// 定义一条完整的授权规则：效果（Allow/Deny）+ 操作（Action）+ 资源（Resource）
// + 数据范围（Scopes）。可被多个策略共享引用，修改后所有关联策略同步生效。
export interface Statement {
  id: number
  // 语句描述（小标题，说明本条授权规则的用途）
  description?: string
  effect: Effect
  action: string
  // 资源（支持 * 通配，默认 * 表示全部资源）
  resource?: string
  // 数据范围（数据权限：可见/操作哪部分数据）
  scopes?: DataScope[]
  sort: number
  // 创建者用户 ID（0 表示系统内置）
  created_by?: number
  created_at?: string
  updated_at?: string
}

// ===== 数据范围（DataScope）=====

export type DataScopeType = 'all' | 'group' | 'self' | 'attribute'

export interface DataScope {
  id?: number
  scope_type: DataScopeType
  group_id?: number
  owner_field?: string
  attr_key?: string
  attr_value?: string
  sort: number
}

export interface PolicyAttachment {
  id: number
  principal_type: PrincipalType
  principal_id: number
  policy_id: number
  created_by: number
  created_at: string
}

// ===== 集成管理 =====

// 应用授权类型（对齐后端 AppGrantType）
// client_credentials：客户端凭证模式（服务间调用）
// authorization_code：授权码模式（用户参与，Web/SPA 授权登录）
export type GrantType = 'client_credentials' | 'authorization_code'

export interface AppItem {
  id: number
  name: string
  app_id: string
  description: string
  owner_account_id: number
  callback_url?: string
  grant_type: GrantType
  status: boolean
  created_at: string
}

export interface AppCreateResponse extends AppItem {
  app_secret: string
}

// ===== 通用 =====

export interface Paginated<T> {
  data: T[]
  total: number
}

export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data: T
  request_id?: string
}
