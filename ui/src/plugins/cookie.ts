// 全局 Cookie 插件：CookieManager 类 + 全局配置 + 单例实例 + Vue 插件。
//
// 设计目标：
//   - CookieManager 类封装 js-cookie 的读写删，统一 cookie 属性（路径/域/SameSite/Secure）；
//   - 全局配置集中一处（子域共享域名、路径、Secure、SameSite），所有模块共用；
//   - 应用内统一通过同一个 CookieManager 实例读写 cookie，避免各处各自 new 造成配置漂移；
//   - 组件内通过 useCookies() 组合式函数获取实例（配合 cookiePlugin 的 provide/inject）。
//
// 用法：
//   // 任意模块（store / api / 工具）
//   import { appCookie } from '@/plugins/cookie'
//   appCookie.set('theme', 'dark')
//
//   // Vue 组件
//   const cookies = useCookies()
//   cookies.get('theme')
//
// 重要安全说明：
//   - 写入的 cookie 是 JS 可读（非 httpOnly），同域/子域的脚本都能读取；
//   - 若存敏感数据（如 JWT token），会暴露给同域所有 JS，存在 XSS 窃取风险；
//     仅应在同一信任域内共享，且应设置较短的有效期。
import Cookies from 'js-cookie'
import type { App, InjectionKey } from 'vue'
import { inject } from 'vue'

/** cookie 配置选项（每字段都可被调用方按次覆盖）。 */
export interface CookieOptions {
  /** 有效期（秒）。默认 1 天。 */
  maxAge?: number
  /** 路径，默认 "/"。 */
  path?: string
  /** 域名（覆盖默认的子域共享配置）。 */
  domain?: string
  /** 是否仅 HTTPS 传输。默认跟随当前协议（https 时 true）。 */
  secure?: boolean
  /** SameSite 策略，默认 "lax"。 */
  sameSite?: 'strict' | 'lax' | 'none'
}

/**
 * 面向对象的 cookie 管理器。
 *
 * 构造时传入默认属性（如全局子域、默认有效期），之后所有实例方法都套用这些默认值，
 * 也可在单次调用时覆盖。适合：
 *   - 令牌 cookie：`new CookieManager({ maxAge: 7200, path: '/', domain: '.example.com' })`
 *   - 普通偏好 cookie：`new CookieManager({ maxAge: 365 * 24 * 3600 })`
 */
export class CookieManager {
  private readonly defaults: CookieOptions

  constructor(defaults: CookieOptions = {}) {
    this.defaults = defaults
  }

  /** 合并实例默认配置与单次调用覆盖，生成 js-cookie 写入属性。 */
  private buildAttrs(overrides?: CookieOptions): Cookies.CookieAttributes {
    const maxAge = overrides?.maxAge ?? this.defaults.maxAge ?? 24 * 3600
    const domain = overrides?.domain ?? this.defaults.domain
    return {
      path: overrides?.path ?? this.defaults.path ?? '/',
      ...(domain ? { domain } : {}),
      sameSite: overrides?.sameSite ?? this.defaults.sameSite ?? 'lax',
      secure: overrides?.secure ?? this.defaults.secure ?? location.protocol === 'https:',
      expires: new Date(Date.now() + maxAge * 1000),
    }
  }

  /** 写入 cookie。非字符串值（对象/数字等）自动 JSON 序列化。 */
  set(name: string, value: unknown, opts?: CookieOptions) {
    const raw = typeof value === 'string' ? value : JSON.stringify(value)
    Cookies.set(name, raw, this.buildAttrs(opts))
  }

  /** 读取 cookie 原始字符串；不存在返回空串。 */
  get(name: string): string {
    return Cookies.get(name) || ''
  }

  /** 读取并解析为对象（cookie 值为 JSON 时）；非 JSON 或不存在返回 null。 */
  getJSON<T = unknown>(name: string): T | null {
    const raw = Cookies.get(name)
    if (!raw) return null
    try {
      return JSON.parse(raw) as T
    } catch {
      return null
    }
  }

  /** 读取并解析为数字（cookie 值为数字时）；非法或不存在返回 0。 */
  getNumber(name: string): number {
    const v = Number(Cookies.get(name))
    return Number.isFinite(v) ? v : 0
  }

  /** 删除 cookie（需与写入时相同的 path/domain 才能删掉）。 */
  remove(name: string, opts?: Pick<CookieOptions, 'path' | 'domain'>) {
    const path = opts?.path ?? this.defaults.path ?? '/'
    const domain = opts?.domain ?? this.defaults.domain
    Cookies.remove(name, {
      path,
      ...(domain ? { domain } : {}),
    })
  }

  /** 清空当前管理器负责的一组 cookie 名。 */
  removeAll(names: Iterable<string>, opts?: Pick<CookieOptions, 'path' | 'domain'>) {
    for (const name of names) {
      this.remove(name, opts)
    }
  }
}

// 全局 cookie 配置
export interface CookieGlobalConfig {
  /** 子域共享域名（如 ".example.com"），留空则仅当前主机。 */
  domain?: string
  /** 路径，默认 "/"。 */
  path?: string
  /** 是否仅 HTTPS 传输。默认跟随当前协议。 */
  secure?: boolean
  /** SameSite 策略，默认 "lax"。 */
  sameSite?: CookieOptions['sameSite']
}

// 默认全局配置：子域共享域名由 VITE_AUTH_DOMAIN 提供（可选），其余用 CookieManager 默认值。
const GLOBAL_CONFIG: CookieGlobalConfig = {
  domain: (import.meta.env.VITE_AUTH_DOMAIN as string | undefined) || undefined,
  path: '/',
}

// 应用内全局统一的 cookie 实例（所有模块/组件共用同一配置）
export const appCookie = new CookieManager({
  path: GLOBAL_CONFIG.path,
  ...(GLOBAL_CONFIG.domain ? { domain: GLOBAL_CONFIG.domain } : {}),
  ...(GLOBAL_CONFIG.secure !== undefined ? { secure: GLOBAL_CONFIG.secure } : {}),
  ...(GLOBAL_CONFIG.sameSite ? { sameSite: GLOBAL_CONFIG.sameSite } : {}),
})

/** Vue 插件注入键。 */
export const cookieInjectionKey: InjectionKey<CookieManager> = Symbol('app-cookie')

/**
 * Vue 插件：将全局 cookie 实例注入应用，供 useCookies() 使用。
 *
 *   app.use(cookiePlugin)
 */
export const cookiePlugin = {
  install(app: App) {
    app.provide(cookieInjectionKey, appCookie)
  },
}

/**
 * 组件内获取全局 cookie 实例。
 * 需在 app.use(cookiePlugin) 之后使用，否则抛出错误。
 *
 *   const cookies = useCookies()
 */
export function useCookies(): CookieManager {
  const cookies = inject(cookieInjectionKey, null)
  if (!cookies) {
    throw new Error('useCookies() 需在 app.use(cookiePlugin) 之后调用')
  }
  return cookies
}
