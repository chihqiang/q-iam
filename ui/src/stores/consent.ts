// Cookie 同意协议状态管理。
// 用户对「本网站使用 Cookie」的同意选择持久化到 cookie（qiam.cookie_consent），
// 已选择过（同意/拒绝）就不再弹横幅。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { appCookie } from '@/plugins/cookie'

/** Cookie 同意状态：accepted（同意）/ declined（拒绝）/ 空（未选择）。 */
export type CookieConsent = 'accepted' | 'declined' | ''

// 同意选择存于此 cookie（点号层级命名，统一 qiam.* 前缀）
const CONSENT_COOKIE = 'qiam.cookie_consent'
// 同意记录保留时长（1 年）
const CONSENT_MAX_AGE = 365 * 24 * 3600

export const useConsentStore = defineStore('consent', () => {
  // 初始化：从全局 cookie 读取历史选择
  const stored = appCookie.get(CONSENT_COOKIE)
  const consent = ref<CookieConsent>(
    stored === 'accepted' || stored === 'declined' ? stored : ''
  )

  // 是否已做出选择（已选择则不再显示横幅）
  const decided = computed(() => consent.value !== '')

  /** 同意使用 Cookie。 */
  function accept() {
    consent.value = 'accepted'
    appCookie.set(CONSENT_COOKIE, 'accepted', { maxAge: CONSENT_MAX_AGE })
  }

  /** 拒绝使用非必需 Cookie。 */
  function decline() {
    consent.value = 'declined'
    appCookie.set(CONSENT_COOKIE, 'declined', { maxAge: CONSENT_MAX_AGE })
  }

  return { consent, decided, accept, decline }
})
