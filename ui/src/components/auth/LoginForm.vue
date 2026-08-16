<script setup lang="ts">
import { ref } from 'vue'
import { Loader2, AlertCircle } from '@lucide/vue'
import { useAuthStore } from '@/stores/auth'

// 登录表单组件：仅负责账号密码提交与校验，成功后通过 success 事件交由父组件处理跳转。
// 复用场景：
//   1. 统一认证页（/auth）普通登录
//   2. OAuth 授权确认卡（/auth?client_id=...）内嵌登录（title/subtitle 自定义文案）
defineProps<{
  title?: string
  subtitle?: string
}>()

const emit = defineEmits<{
  success: []
  'switch-register': []
}>()

const auth = useAuthStore()

const accountName = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')

async function handleLogin() {
  if (!accountName.value || !password.value) {
    errorMsg.value = '请输入账号名和密码'
    return
  }

  loading.value = true
  errorMsg.value = ''
  try {
    await auth.login(accountName.value, password.value)
    emit('success')
  } catch (err) {
    errorMsg.value = err instanceof Error ? err.message : '登录失败，请重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <h2 class="mb-1 text-base font-semibold text-slate-900">{{ title ?? '欢迎回来' }}</h2>
    <p class="mb-6 text-sm text-slate-500">{{ subtitle ?? '登录以访问管理控制台' }}</p>

    <form class="space-y-4" @submit.prevent="handleLogin">
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">账号名</label>
        <input
          v-model="accountName"
          type="text"
          placeholder="请输入账号名"
          autocomplete="username"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">密码</label>
        <input
          v-model="password"
          type="password"
          placeholder="请输入密码"
          autocomplete="current-password"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>

      <!-- 错误提示 -->
      <div
        v-if="errorMsg"
        class="flex items-center gap-2 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-600"
      >
        <AlertCircle class="h-4 w-4 shrink-0" />
        {{ errorMsg }}
      </div>

      <button
        type="submit"
        :disabled="loading"
        class="flex h-10 w-full items-center justify-center gap-2 rounded-md bg-linear-to-r from-cyan-500 to-blue-600 text-sm font-medium text-white shadow-[0_4px_20px_-4px_rgba(6,182,212,0.5)] transition-all hover:from-cyan-400 hover:to-blue-500 hover:shadow-[0_6px_28px_-4px_rgba(6,182,212,0.6)] disabled:cursor-not-allowed disabled:opacity-60"
      >
        <Loader2 v-if="loading" class="h-4 w-4 animate-spin" />
        {{ loading ? '登录中…' : '登录' }}
      </button>
    </form>

    <!-- 没有账号 -->
    <p class="mt-4 text-center text-sm text-slate-500">
      没有账号？
      <button
        type="button"
        class="font-medium text-cyan-600 transition-colors hover:text-cyan-500 hover:underline"
        @click="emit('switch-register')"
      >
        立即注册
      </button>
    </p>
  </div>
</template>
