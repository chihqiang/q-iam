<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ExternalLink,
  Check,
  X,
  Loader2,
  TriangleAlert,
  UserRound,
  Globe,
} from '@lucide/vue'
import { useAuthStore } from '@/stores/auth'
import { getOAuthAppInfo, authorizeOAuth } from '@/api/oauth'
import type { AppItem } from '@/types'
import { parseScopes } from '@/utils/oauth'
import LoginForm from '@/components/auth/LoginForm.vue'

// OAuth 授权确认卡（OAuth2 consent 页）：两步式授权流程。
//   步骤 1 登录：未登录时展示登录表单（授权确认本质上也是登录）
//   步骤 2 授权确认：已登录后展示「以 xxx 身份」+ 允许/取消
// 登录成功后由 auth.isLoggedIn 驱动自动从步骤 1 切到步骤 2，无需跳转。
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// 授权页发起注册（父容器设置 redirect 回跳本授权页）
const emit = defineEmits<{
  'switch-register': []
}>()

// 当前登录账号信息（授权确认步骤展示）
const displayName = computed(() => auth.profile?.display_name || '管理员')
const accountName = computed(() => auth.profile?.account_name || '')
const avatarChar = computed(() => (auth.profile?.display_name || 'A').charAt(0))

// 从 query 读取 OAuth 参数：client_id / redirect_uri / scope / state
const clientId = route.query.client_id as string | undefined
const redirectUri = route.query.redirect_uri as string | undefined
const scope = (route.query.scope as string | undefined) || ''
const state = route.query.state as string | undefined

const appInfo = ref<AppItem | null>(null)
const loading = ref(true)
const error = ref('')
const approving = ref(false)

// 申请的权限范围：解析为逐条权限列表；未申请时展示基础信息
const scopeList = computed(() => parseScopes(scope))

// 授权后跳转地址的主机（安全提示：让用户知道授权后去哪）
const redirectHost = computed(() => {
  try {
    return new URL(redirectUri || '').host
  } catch {
    return ''
  }
})

// 加载应用信息（公开接口，未登录也可先展示）
onMounted(async () => {
  if (!clientId || !redirectUri) {
    error.value = '缺少 client_id 或 redirect_uri 参数'
    loading.value = false
    return
  }
  // 确保登录态与账号信息：
  //   - 已登录但 profile 未加载（如直接打开授权页 URL）→ 拉取一次
  //   - 未登录但有 refresh token → 静默续期，成功后拉取 profile
  if (auth.isLoggedIn && !auth.profile) {
    try {
      await auth.fetchProfile()
    } catch {
      /* 拉取失败不阻塞授权页展示 */
    }
  } else if (!auth.isLoggedIn && auth.refreshToken) {
    const ok = await auth.tryRefresh()
    if (ok && !auth.profile) {
      try {
        await auth.fetchProfile()
      } catch {
        /* 拉取失败不阻塞授权页展示 */
      }
    }
  }
  try {
    appInfo.value = await getOAuthAppInfo(clientId)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})

// 授权确认：向后端签发授权码，跳转 redirect_uri?code=xxx&state=xxx
async function onApprove() {
  if (!clientId || !redirectUri || !appInfo.value) return
  approving.value = true
  error.value = ''
  try {
    const resp = await authorizeOAuth({
      client_id: clientId,
      redirect_uri: redirectUri,
      scope,
      state,
    })
    const target = new URL(resp.redirect_uri)
    target.searchParams.set('code', resp.code)
    if (state) target.searchParams.set('state', state)
    window.location.href = target.toString()
  } catch (e) {
    error.value = (e as Error).message
    approving.value = false
  }
}

// 拒绝授权
function onDeny() {
  if (redirectUri) {
    const target = new URL(redirectUri)
    target.searchParams.set('error', 'access_denied')
    if (state) target.searchParams.set('state', state)
    window.location.href = target.toString()
  } else {
    router.push('/auth')
  }
}

// 切换账号：退出后刷新当前页（回到步骤 1 登录）
function goLogout() {
  auth.logout()
  window.location.reload()
}
</script>

<template>
  <div>
    <!-- 加载中 -->
    <div v-if="loading" class="flex flex-col items-center gap-3 py-10">
      <Loader2 class="h-6 w-6 animate-spin text-slate-400" />
      <p class="text-sm text-slate-500">正在校验应用信息…</p>
    </div>

    <!-- 错误 -->
    <div v-else-if="error" class="space-y-4">
      <div class="flex items-start gap-3 rounded-lg bg-destructive/10 p-4">
        <TriangleAlert class="mt-0.5 h-5 w-5 shrink-0 text-destructive" />
        <div>
          <div class="text-sm font-medium text-slate-900">授权失败</div>
          <div class="mt-1 text-sm text-slate-500">{{ error }}</div>
        </div>
      </div>
      <button
        class="flex h-10 w-full items-center justify-center gap-2 rounded-md bg-linear-to-r from-cyan-500 to-blue-600 text-sm font-medium text-white shadow-[0_4px_20px_-4px_rgba(6,182,212,0.5)] transition-all hover:from-cyan-400 hover:to-blue-500 hover:shadow-[0_6px_28px_-4px_rgba(6,182,212,0.6)]"
        @click="router.push('/auth')"
      >
        返回登录
      </button>
    </div>

    <!-- 授权信息 -->
    <template v-else-if="appInfo">
      <!-- ===== 应用信息 ===== -->
      <div class="mb-5 flex items-center gap-3">
        <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-slate-100">
          <ExternalLink class="h-5 w-5 text-slate-500" />
        </div>
        <div class="min-w-0">
          <div class="truncate text-sm font-semibold text-slate-900">{{ appInfo.name }}</div>
          <div v-if="appInfo.description" class="mt-0.5 truncate text-xs text-slate-500">
            {{ appInfo.description }}
          </div>
          <div class="mt-0.5 flex items-center gap-1 text-xs text-slate-400">
            <code class="font-mono">{{ appInfo.app_id }}</code>
          </div>
        </div>
      </div>

      <!-- 授权后跳转提示（安全透明） -->
      <div
        v-if="redirectHost"
        class="mb-5 flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-500"
      >
        <Globe class="h-3.5 w-3.5 shrink-0 text-slate-400" />
        <span class="min-w-0">
          授权后将跳转至
          <code class="break-all font-mono text-slate-600">{{ redirectHost }}</code>
        </span>
      </div>

      <!-- ===== 申请的权限 ===== -->
      <div class="mb-6">
        <div class="mb-2 text-xs font-medium uppercase tracking-wider text-slate-500">
          应用将获得以下权限
        </div>
        <div class="space-y-2 rounded-lg bg-slate-100/80 p-3">
          <!-- 有 scope：逐条列出 -->
          <template v-if="scopeList.length">
            <div v-for="s in scopeList" :key="s.raw" class="flex items-start gap-2.5">
              <Check class="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
              <div class="min-w-0">
                <div class="text-sm font-medium text-slate-800">{{ s.label }}</div>
                <div class="text-xs text-slate-500">{{ s.desc }}</div>
              </div>
            </div>
          </template>
          <!-- 无 scope：基础信息 -->
          <div v-else class="flex items-center gap-2 text-sm text-slate-600">
            <Check class="h-4 w-4 text-emerald-500" />
            访问你的基础信息（账号标识与权限规则）
          </div>
        </div>
      </div>

      <p v-if="error" class="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
        {{ error }}
      </p>

      <div class="mb-6 border-t border-slate-200" />

      <!-- ===== 步骤 1：登录（未登录） ===== -->
      <div v-if="!auth.isLoggedIn">
        <!-- 步骤指示器 -->
        <div class="mb-5 flex items-center justify-center gap-2 text-xs">
          <span
            class="flex h-6 w-6 items-center justify-center rounded-full bg-linear-to-r from-cyan-500 to-blue-600 text-[11px] font-semibold text-white"
          >
            1
          </span>
          <span class="font-medium text-cyan-600">登录</span>
          <span class="h-px w-6 bg-slate-300" />
          <span
            class="flex h-6 w-6 items-center justify-center rounded-full border border-slate-300 text-[11px] font-semibold text-slate-400"
          >
            2
          </span>
          <span class="text-slate-400">授权确认</span>
        </div>

        <LoginForm
          title="登录以继续"
          :subtitle="`登录 ${appInfo.name} 之前，需要先验证你的账号`"
          @switch-register="emit('switch-register')"
        />
      </div>

      <!-- ===== 步骤 2：授权确认（已登录） ===== -->
      <div v-else>
        <!-- 步骤指示器 -->
        <div class="mb-5 flex items-center justify-center gap-2 text-xs">
          <span
            class="flex h-6 w-6 items-center justify-center rounded-full bg-linear-to-r from-cyan-500 to-blue-600 text-[11px] font-semibold text-white"
          >
            <Check class="h-3.5 w-3.5" />
          </span>
          <span class="font-medium text-slate-600">登录</span>
          <span class="h-px w-6 bg-slate-300" />
          <span
            class="flex h-6 w-6 items-center justify-center rounded-full bg-linear-to-r from-cyan-500 to-blue-600 text-[11px] font-semibold text-white"
          >
            2
          </span>
          <span class="font-medium text-cyan-600">授权确认</span>
        </div>

        <!-- 以当前账号身份 -->
        <div
          class="mb-5 flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5"
        >
          <div class="flex min-w-0 items-center gap-2.5">
            <div
              class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-linear-to-br from-cyan-500 to-blue-600 text-sm font-medium text-white"
            >
              {{ avatarChar }}
            </div>
            <div class="min-w-0 leading-tight">
              <div class="truncate text-sm font-medium text-slate-900">{{ displayName }}</div>
              <div class="truncate text-xs text-slate-500">@{{ accountName }}</div>
            </div>
          </div>
          <button
            class="shrink-0 rounded-md border border-slate-200 px-2 py-1 text-[11px] text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700"
            title="退出登录，切换其他账号"
            @click="goLogout"
          >
            切换账号
          </button>
        </div>

        <!-- 授权说明 -->
        <div class="mb-6 flex items-start gap-2.5">
          <UserRound class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
          <p class="text-sm leading-relaxed text-slate-600">
            授权后，应用
            <span class="font-semibold text-slate-800">{{ appInfo.name }}</span>
            将以 <span class="font-medium text-slate-800">@{{ accountName }}</span> 的身份
            访问上述权限范围内的资源。请确认这是你信任的应用。
          </p>
        </div>

        <!-- 操作：取消 / 允许 -->
        <div class="flex gap-3">
          <button
            class="flex h-10 flex-1 items-center justify-center gap-2 rounded-md border border-slate-200 bg-white/80 text-sm text-slate-600 transition-colors hover:bg-slate-50"
            @click="onDeny"
          >
            <X class="h-4 w-4" />
            取消
          </button>
          <button
            class="flex h-10 flex-1 items-center justify-center gap-2 rounded-md bg-linear-to-r from-cyan-500 to-blue-600 text-sm font-medium text-white shadow-[0_4px_20px_-4px_rgba(6,182,212,0.5)] transition-all hover:from-cyan-400 hover:to-blue-500 hover:shadow-[0_6px_28px_-4px_rgba(6,182,212,0.6)] disabled:opacity-60"
            :disabled="approving"
            @click="onApprove"
          >
            <Loader2 v-if="approving" class="h-4 w-4 animate-spin" />
            <Check v-else class="h-4 w-4" />
            {{ approving ? '授权中…' : `允许 ${appInfo.name}` }}
          </button>
        </div>
      </div>
    </template>
  </div>
</template>
