// OAuth API
import axios from 'axios'
import { get, post } from './client'
import type { AppItem, DataScope } from '@/types'

// 查询应用信息（公开接口）
export const getOAuthAppInfo = (clientId: string) =>
  get<AppItem>('/oauth/app-info', { client_id: clientId })

export interface AuthorizePayload {
  client_id: string
  redirect_uri: string
  scope?: string
  state?: string
}

export interface AuthorizeResult {
  code: string
  app_id: string
  app_name: string
  redirect_uri: string
  scope: string
  state?: string
}

// 授权确认，签发授权码（需登录态）
export const authorizeOAuth = (payload: AuthorizePayload) =>
  post<AuthorizeResult>('/oauth/authorize', payload)

// ===== UserInfo Endpoint（OAuth 2.0 资源服务器接口）=====

export interface OAuthPermissionStatement {
  effect: string
  action: string
  source?: string
  data_scopes?: DataScope[]
}

export interface OAuthUserDetail {
  account_id: number
  account_name: string
  display_name: string
  email: string
  mobile: string
}

// UserInfo 响应（对齐 OAuth 2.0 / OIDC 规范）
export interface OAuthUserInfo {
  sub: string
  client_id: string
  app_name: string
  scope: string
  aud: string
  user?: OAuthUserDetail
  permissions?: OAuthPermissionStatement[]
}

// 应用凭 access_token 查询用户信息 + 已授权权限
// 注意：这是资源服务器接口（OAuth 2.0 UserInfo Endpoint），应用直接携带自己的 token 调用。
// 因此**不走统一 axios 拦截器**（避免 401 误触发登录态刷新/登出跳转），
// 并适配后端标准的 OAuth 错误格式（RFC 6750）：{ error, error_description } + 非 200 状态码。
export const getOAuthUserInfo = async (accessToken: string): Promise<OAuthUserInfo> => {
  try {
    const resp = await axios.get('/api/v1/oauth/userinfo', {
      headers: { Authorization: `Bearer ${accessToken}` },
    })
    // 成功响应仍为统一包装：{ code: 0, msg: "ok", data: { ... } }
    const body = resp.data as { code: number; data: OAuthUserInfo }
    return body.data
  } catch (e) {
    // 失败响应为标准 OAuth 错误：{ error, error_description }（RFC 6750）
    const data = (e as { response?: { data?: { error?: string; error_description?: string } } })
      .response?.data
    const desc = data?.error_description || data?.error || '请求失败'
    throw new Error(desc)
  }
}
