<script setup lang="ts">
import { ref } from 'vue'
import { Loader2, AlertCircle, Check } from '@lucide/vue'
import { useAuthStore } from '@/stores/auth'

// 注册表单组件：仅负责注册提交与前端校验，成功后通过 success 事件交由父组件处理跳转。
const emit = defineEmits<{
  success: []
  'switch-login': []
}>()

const auth = useAuthStore()

const accountName = ref('')
const displayName = ref('')
const email = ref('')
const mobile = ref('')
const password = ref('')
const confirmPassword = ref('')
const loading = ref(false)
const errorMsg = ref('')

async function handleRegister() {
  if (!accountName.value) {
    errorMsg.value = '请输入账号名'
    return
  }
  if (!password.value) {
    errorMsg.value = '请输入密码'
    return
  }
  if (password.value.length < 8) {
    errorMsg.value = '密码长度不能少于 8 位'
    return
  }
  if (password.value !== confirmPassword.value) {
    errorMsg.value = '两次输入的密码不一致'
    return
  }
  if (email.value && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value)) {
    errorMsg.value = '邮箱格式不正确'
    return
  }

  loading.value = true
  errorMsg.value = ''
  try {
    await auth.register({
      account_name: accountName.value.trim(),
      display_name: displayName.value.trim(),
      email: email.value.trim(),
      mobile: mobile.value.trim(),
      password: password.value,
    })
    emit('success')
  } catch (err) {
    errorMsg.value = err instanceof Error ? err.message : '注册失败，请重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <h2 class="mb-1 text-base font-semibold text-slate-900">创建账号</h2>
    <p class="mb-6 text-sm text-slate-500">注册后可用于登录控制台与授权访问</p>

    <form class="space-y-4" @submit.prevent="handleRegister">
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">
          账号名
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="accountName"
          type="text"
          placeholder="请输入账号名"
          autocomplete="username"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">显示名</label>
        <input
          v-model="displayName"
          type="text"
          placeholder="请输入显示名（可选）"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">邮箱</label>
        <input
          v-model="email"
          type="email"
          placeholder="请输入邮箱（可选）"
          autocomplete="email"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">手机号</label>
        <input
          v-model="mobile"
          type="tel"
          placeholder="请输入手机号（可选）"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">
          密码
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="password"
          type="password"
          placeholder="至少 8 位，需包含小写字母和数字"
          autocomplete="new-password"
          class="h-10 w-full rounded-md border border-slate-200 bg-white/80 px-3 text-sm text-slate-900 placeholder:text-slate-400 outline-none transition-all focus:border-cyan-500 focus:shadow-[0_0_0_3px_rgba(6,182,212,0.12)]"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm font-medium text-slate-600">
          确认密码
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="confirmPassword"
          type="password"
          placeholder="再次输入密码"
          autocomplete="new-password"
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
        <Check v-else class="h-4 w-4" />
        {{ loading ? '注册中…' : '注册' }}
      </button>
    </form>

    <!-- 已有账号 -->
    <p class="mt-4 text-center text-sm text-slate-500">
      已有账号？
      <button
        type="button"
        class="font-medium text-cyan-600 transition-colors hover:text-cyan-500 hover:underline"
        @click="emit('switch-login')"
      >
        去登录
      </button>
    </p>
  </div>
</template>
