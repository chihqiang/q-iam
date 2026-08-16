import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { post, get } from '@/api/client'
import type { LoginResponse, Profile } from '@/types'
import { appCookie } from '@/plugins/cookie'

// 认证令牌 cookie 名称（点号层级命名，统一前缀 qiam.*，同域/子域其他系统按此读取）
const ACCESS_TOKEN_COOKIE = 'qiam.access_token'
const REFRESH_TOKEN_COOKIE = 'qiam.refresh_token'
const EXPIRES_AT_COOKIE = 'qiam.expires_at'
// refresh token 有效期更长（默认 7 天），单独设置更长 maxAge
const REFRESH_TOKEN_MAX_AGE = 7 * 24 * 3600

// 认证令牌 cookie：统一使用全局 appCookie（配置见 src/cookie.ts，子域共享域名由 VITE_AUTH_DOMAIN 提供）
const authCookies = appCookie

export const useAuthStore = defineStore('auth', () => {
  // 登录态统一以 cookie 为准：页面加载时从 cookie 恢复，登录/刷新后写回 cookie。
  // （不再使用 localStorage，同域/子域其他系统均通过 cookie 读取 token。）
  const accessToken = ref(authCookies.get(ACCESS_TOKEN_COOKIE))
  const refreshToken = ref(authCookies.get(REFRESH_TOKEN_COOKIE))
  const expiresAt = ref(authCookies.getNumber(EXPIRES_AT_COOKIE))
  const profile = ref<Profile | null>(null)
  // 密码是否已过期（password_policy.expire_days），用于引导用户立即修改密码
  const passwordExpired = ref(false)

  const isLoggedIn = computed(() => !!accessToken.value && Date.now() < expiresAt.value * 1000)

  // 是否允许进入管理控制台（注册账号为 false，仅用于 OAuth2 授权登录）
  const canEnterConsole = computed(() => isLoggedIn.value && (profile.value?.allow_console ?? true))

  // 登录/注册/刷新成功后统一写入令牌 cookie（access + refresh + 过期时间戳）
  function writeAuthCookies(opts: {
    accessToken: string
    refreshToken?: string
    expiresIn: number // 秒
  }) {
    const maxAge = Math.max(opts.expiresIn, 60)
    authCookies.set(ACCESS_TOKEN_COOKIE, opts.accessToken, { maxAge })
    if (opts.refreshToken) {
      authCookies.set(REFRESH_TOKEN_COOKIE, opts.refreshToken, { maxAge: REFRESH_TOKEN_MAX_AGE })
    }
    authCookies.set(EXPIRES_AT_COOKIE, String(Math.floor(Date.now() / 1000) + opts.expiresIn), {
      maxAge,
    })
  }

  // 登录
  async function login(accountName: string, password: string) {
    const resp = await post<LoginResponse>('/auth/login', { account_name: accountName, password })
    accessToken.value = resp.access_token
    refreshToken.value = resp.refresh_token ?? ''
    // expires_in 是访问令牌有效秒数，换算成本地过期时间戳（Unix 秒）
    expiresAt.value = Math.floor(Date.now() / 1000) + resp.expires_in
    passwordExpired.value = resp.password_expired ?? false
    // 写入 cookie，供本应用及同域/子域其他系统读取并解析 token
    writeAuthCookies({
      accessToken: resp.access_token,
      refreshToken: resp.refresh_token,
      expiresIn: resp.expires_in,
    })
    // 加载个人信息
    await fetchProfile()
  }

  // 注册（注册即登录，返回后已携带 token）
  async function register(payload: {
    account_name: string
    display_name?: string
    email?: string
    mobile?: string
    password: string
  }) {
    const resp = await post<LoginResponse>('/auth/register', payload)
    accessToken.value = resp.access_token
    refreshToken.value = resp.refresh_token ?? ''
    expiresAt.value = Math.floor(Date.now() / 1000) + resp.expires_in
    passwordExpired.value = resp.password_expired ?? false
    // 写入 cookie，供本应用及同域/子域其他系统读取并解析 token
    writeAuthCookies({
      accessToken: resp.access_token,
      refreshToken: resp.refresh_token,
      expiresIn: resp.expires_in,
    })
    // 加载个人信息
    await fetchProfile()
  }

  // 获取当前账号信息
  async function fetchProfile() {
    const resp = await get<Profile>('/auth/me')
    profile.value = resp
    passwordExpired.value = resp.password_expired ?? false
    return resp
  }

  // 尝试用 refresh token 刷新
  async function tryRefresh(): Promise<boolean> {
    if (!refreshToken.value) return false
    try {
      const resp = await post<LoginResponse>('/auth/refresh', { refresh_token: refreshToken.value })
      accessToken.value = resp.access_token
      refreshToken.value = resp.refresh_token ?? refreshToken.value
      expiresAt.value = Math.floor(Date.now() / 1000) + resp.expires_in
      passwordExpired.value = resp.password_expired ?? false
      // 刷新后同步更新 cookie（token 已轮换）
      writeAuthCookies({
        accessToken: resp.access_token,
        refreshToken: resp.refresh_token ?? refreshToken.value,
        expiresIn: resp.expires_in,
      })
      return true
    } catch {
      return false
    }
  }

  // 退出
  function logout() {
    const rt = refreshToken.value
    // fire-and-forget：尽力通知后端吊销当前会话的刷新令牌（不阻塞退出、失败忽略）。
    // 已轮换/已吊销时服务端幂等返回成功。注意放在清空本地之前读取 refresh token。
    if (rt) {
      post('/auth/logout', { refresh_token: rt }).catch(() => {})
    }
    accessToken.value = ''
    refreshToken.value = ''
    expiresAt.value = 0
    profile.value = null
    passwordExpired.value = false
    // 清除认证 cookie（同域/子域其他系统将读到空令牌）
    authCookies.removeAll([ACCESS_TOKEN_COOKIE, REFRESH_TOKEN_COOKIE, EXPIRES_AT_COOKIE])
  }

  return {
    accessToken,
    refreshToken,
    expiresAt,
    profile,
    passwordExpired,
    isLoggedIn,
    canEnterConsole,
    login,
    register,
    fetchProfile,
    tryRefresh,
    logout,
  }
})
