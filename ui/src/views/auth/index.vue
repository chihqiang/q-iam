<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ShieldCheck } from '@lucide/vue'
import { useAuthStore } from '@/stores/auth'
import LoginForm from '@/components/auth/LoginForm.vue'
import RegisterForm from '@/components/auth/RegisterForm.vue'
import OAuthAuthorizeCard from '@/components/auth/OAuthAuthorizeCard.vue'

// 统一认证页：单路由单页面，由 URL 参数驱动三种模式。
//   /auth                      → 登录（默认）
//   /auth?mode=register        → 注册
//   /auth?client_id=...&redirect_uri=... → OAuth 授权确认（client_id 天然标识授权模式）
// 授权确认本质上也是一种登录：未登录时页内直接输入账号密码，登录成功就地进入授权确认。

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// 当前模式：register / authorize（有 client_id 即授权模式）/ login
const mode = computed(() => {
  const m = (route.query.mode as string) || ''
  if (m === 'register') return 'register'
  if (route.query.client_id) return 'authorize'
  return 'login'
})

const isAuthorize = computed(() => mode.value === 'authorize')

// 页面标题/副标题按模式区分
const pageTitle = computed(() => (isAuthorize.value ? 'q-iam 授权' : 'q-iam'))
const pageSubtitle = computed(() => {
  if (isAuthorize.value) return '安全登录，授权应用访问'
  if (mode.value === 'register') return '创建你的账号'
  return '登录管理控制台'
})

// redirect 是否指向授权模式（授权页发起注册后回跳用）：带 client_id 即为授权。
// 用 URL 解析 query（兼容相对路径 /auth?client_id=... 与完整 URL），
// 替代原来的字符串 includes 判断，避免误判（如 redirect_uri 里恰好含 "client_id="）。
function isAuthorizeRedirect(url: string): boolean {
  try {
    return new URL(url, window.location.origin).searchParams.has('client_id')
  } catch {
    return false
  }
}

// 校验 redirect 为站内路径（前端层开放重定向防护）：
// 相对路径（以 / 开头）或与当前站点同源的完整 URL 才允许回跳。
function isSafeRedirect(url: string): boolean {
  if (!url) return false
  if (url.startsWith('/')) return true
  try {
    return new URL(url).origin === window.location.origin
  } catch {
    return false
  }
}

// 登录/注册成功后的统一分流（保持原语义）：
//   1. redirect 指向授权页 → 回跳授权流程
//   2. 无控制台权限（注册账号） → /no-console
//   3. 否则 → redirect（仅站内）或管理页
function afterAuthSuccess() {
  const redirect = route.query.redirect as string | undefined
  if (redirect && isAuthorizeRedirect(redirect) && isSafeRedirect(redirect)) {
    router.replace(redirect)
    return
  }
  if (!auth.canEnterConsole) {
    router.push('/no-console')
    return
  }
  router.push(redirect && isSafeRedirect(redirect) ? redirect : '/accounts')
}

// 登录/注册成功统一入口：授权模式就地进入授权确认（不跳转），其余走分流
function onAuthSuccess() {
  if (isAuthorize.value) return
  afterAuthSuccess()
}

// 登录 ⇄ 注册模式切换（保留 redirect，更新 URL）
function setMode(m: 'login' | 'register') {
  const q: Record<string, string> = { mode: m }
  const redirect = route.query.redirect as string | undefined
  if (redirect) q.redirect = redirect
  router.replace({ query: q })
}

// 授权卡点击"立即注册"：切到注册模式，并记录回跳本授权页
function goAuthorizeRegister() {
  const q: Record<string, string> = { mode: 'register', redirect: route.fullPath }
  router.replace({ query: q })
}
</script>

<template>
  <div
    class="relative flex min-h-screen items-center justify-center overflow-hidden bg-linear-to-br from-slate-50 via-white to-cyan-50/60 p-4"
  >
    <!-- 浅色网格纹理（中心清晰、边缘渐隐） -->
    <div class="tech-grid-light pointer-events-none absolute inset-0" />

    <!-- 柔和霓虹光斑 -->
    <div class="pointer-events-none absolute inset-0">
      <div class="absolute -left-40 -top-40 h-112 w-md rounded-full bg-cyan-400/20 blur-3xl" />
      <div class="absolute -bottom-40 -right-40 h-112 w-md rounded-full bg-blue-400/20 blur-3xl" />
      <div
        class="absolute left-1/2 top-1/2 h-80 w-80 -translate-x-1/2 -translate-y-1/2 rounded-full bg-violet-300/15 blur-3xl"
      />
    </div>

    <!-- 顶部霓虹光带 -->
    <div
      class="pointer-events-none absolute left-1/2 top-0 h-0.5 w-184 max-w-full -translate-x-1/2 rounded-full bg-linear-to-r from-transparent via-cyan-400/80 to-transparent"
    />

    <!-- 认证/授权卡片 -->
    <div class="relative w-full max-w-md">
      <!-- 顶部品牌 -->
      <div class="mb-8 flex flex-col items-center gap-3">
        <div
          class="relative flex h-14 w-14 items-center justify-center rounded-2xl bg-linear-to-br from-cyan-500 to-blue-600 text-white shadow-[0_8px_30px_-6px_rgba(6,182,212,0.5)]"
        >
          <ShieldCheck class="h-7 w-7" />
          <div class="absolute inset-0 rounded-2xl ring-1 ring-cyan-400/50" />
        </div>
        <div class="text-center">
          <h1 class="text-2xl font-bold tracking-wide text-slate-900">{{ pageTitle }}</h1>
          <p class="mt-1 text-sm tracking-[0.2em] text-cyan-600/70">{{ pageSubtitle }}</p>
        </div>
      </div>

      <div
        class="relative overflow-hidden rounded-2xl border border-slate-200/80 bg-white/70 p-6 shadow-[0_8px_40px_-12px_rgba(15,23,42,0.15)] backdrop-blur-xl"
      >
        <!-- 卡片顶部霓虹光条 -->
        <div
          class="absolute inset-x-0 top-0 mx-auto h-0.5 w-3/5 rounded-full bg-linear-to-r from-transparent via-cyan-400/80 to-transparent"
        />

        <!-- 登录 -->
        <template v-if="mode === 'login'">
          <LoginForm @success="onAuthSuccess" @switch-register="setMode('register')" />
        </template>

        <!-- 注册 -->
        <template v-else-if="mode === 'register'">
          <RegisterForm @success="onAuthSuccess" @switch-login="setMode('login')" />
        </template>

        <!-- OAuth 授权确认（未登录时页内登录） -->
        <template v-else>
          <OAuthAuthorizeCard @switch-register="goAuthorizeRegister" />
        </template>
      </div>
    </div>
  </div>
</template>
