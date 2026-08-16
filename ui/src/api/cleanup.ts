// 历史数据清理 API
import { post } from './client'

export interface CleanupResult {
  /** 清理的审计日志条数 */
  audit_logs: number
  /** 清理的刷新令牌条数（仅已过期） */
  refresh_tokens: number
}

export interface CleanupPayload {
  /** 清理 days 天以前的数据；不传或 <=0 时用默认 30 天 */
  days?: number
}

// 触发历史数据清理（需 iam:system:cleanup 权限）
export const cleanupHistory = (payload: CleanupPayload = {}) =>
  post<CleanupResult>('/cleanup/history', payload)
