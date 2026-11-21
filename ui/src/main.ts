import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'

import App from './App.vue'
import router from './router'
import { cookiePlugin } from './plugins/cookie'
import { useAuthStore } from './stores/auth'
import './assets/main.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(VueQueryPlugin)
// 全局 cookie 集成：provide 统一的 CookieManager 实例，组件用 useCookies() 获取
app.use(cookiePlugin)

// 恢复登录态：已持久化 token 时拉取用户信息（失败则登出）
const auth = useAuthStore()
if (auth.isLoggedIn && !auth.profile) {
  auth.fetchProfile().catch(() => auth.logout())
}

app.mount('#app')
