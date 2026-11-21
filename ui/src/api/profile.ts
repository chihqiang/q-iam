// 个人中心 API
import { put } from './client'

export interface ChangeOwnPasswordPayload {
  old_password: string
  new_password: string
}

// 当前登录账号修改自己的密码
export const changeOwnPassword = (payload: ChangeOwnPasswordPayload) =>
  put<{ message: string }>('/auth/password', payload)
