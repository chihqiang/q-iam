<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { AlertTriangle, BadgeCheck, Mail, Phone, AlignLeft, Hash, KeyRound } from '@lucide/vue'
import ChangePasswordDialog from '@/components/profile/ChangePasswordDialog.vue'

const auth = useAuthStore()
const profile = computed(() => auth.profile)
const avatarChar = computed(() => (profile.value?.display_name || 'A').charAt(0))

// 兜底：刷新页面后 profile 可能未加载，自动重新获取
onMounted(() => {
  if (!auth.profile) {
    auth.fetchProfile().catch(() => {})
  }
})

// 修改密码弹窗开关
const changePwdOpen = ref(false)

// 密码修改成功：重新拉取资料，清除过期标记
async function onPasswordSaved() {
  await auth.fetchProfile().catch(() => {})
}
</script>

<template>
  <div class="mx-auto max-w-3xl space-y-6">
    <!-- 密码过期提醒 -->
    <div
      v-if="auth.passwordExpired"
      class="flex items-center justify-between gap-3 rounded-lg border border-warning/40 bg-warning/10 px-4 py-3"
    >
      <div class="flex items-center gap-2 text-sm text-warning">
        <AlertTriangle class="h-4 w-4 shrink-0" />
        <span>你的密码已超过有效期，请立即修改以保障账号安全。</span>
      </div>
      <button
        class="shrink-0 rounded-md bg-warning px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="changePwdOpen = true"
      >
        立即修改
      </button>
    </div>

    <!-- 账号概览 -->
    <div class="rounded-lg border border-border bg-card">
      <div class="border-b border-border px-5 py-4">
        <h2 class="text-sm font-semibold text-foreground">账号概览</h2>
      </div>
      <div class="flex items-center gap-4 px-5 py-6">
        <div
          class="flex h-16 w-16 shrink-0 items-center justify-center rounded-full bg-primary text-2xl font-medium text-primary-foreground"
        >
          {{ avatarChar }}
        </div>
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <span class="truncate text-lg font-semibold text-foreground">
              {{ profile?.display_name || '—' }}
            </span>
            <BadgeCheck v-if="profile?.status" class="h-4 w-4 shrink-0 text-primary" />
          </div>
          <div class="mt-0.5 truncate text-sm text-muted-foreground">
            {{ profile?.account_name || '—' }}
          </div>
        </div>
      </div>
    </div>

    <!-- 基本信息 -->
    <div class="rounded-lg border border-border bg-card">
      <div class="border-b border-border px-5 py-4">
        <h2 class="text-sm font-semibold text-foreground">基本信息</h2>
      </div>
      <div class="divide-y divide-border">
        <div class="flex items-center gap-3 px-5 py-3.5 text-sm">
          <Hash class="h-4 w-4 shrink-0 text-muted-foreground" />
          <span class="w-16 shrink-0 text-muted-foreground">账号 ID</span>
          <span class="text-foreground">{{ profile?.id ?? '—' }}</span>
        </div>
        <div class="flex items-center gap-3 px-5 py-3.5 text-sm">
          <Mail class="h-4 w-4 shrink-0 text-muted-foreground" />
          <span class="w-16 shrink-0 text-muted-foreground">邮箱</span>
          <span class="text-foreground">{{ profile?.email || '—' }}</span>
        </div>
        <div class="flex items-center gap-3 px-5 py-3.5 text-sm">
          <Phone class="h-4 w-4 shrink-0 text-muted-foreground" />
          <span class="w-16 shrink-0 text-muted-foreground">手机号</span>
          <span class="text-foreground">{{ profile?.mobile || '—' }}</span>
        </div>
        <div class="flex items-center gap-3 px-5 py-3.5 text-sm">
          <AlignLeft class="h-4 w-4 shrink-0 text-muted-foreground" />
          <span class="w-16 shrink-0 text-muted-foreground">备注</span>
          <span class="text-foreground">{{ profile?.remark || '—' }}</span>
        </div>
      </div>
    </div>

    <!-- 账号安全：修改密码 -->
    <div class="rounded-lg border border-border bg-card">
      <div class="border-b border-border px-5 py-4">
        <h2 class="text-sm font-semibold text-foreground">账号安全</h2>
      </div>
      <div class="flex items-center justify-between px-5 py-4">
        <div class="flex items-center gap-3">
          <div
            class="flex h-9 w-9 items-center justify-center rounded-md bg-muted text-muted-foreground"
          >
            <KeyRound class="h-4.5 w-4.5" />
          </div>
          <div class="leading-tight">
            <div class="text-sm font-medium text-foreground">登录密码</div>
            <div class="text-xs text-muted-foreground">定期修改密码可提升账号安全性</div>
          </div>
        </div>
        <button
          class="rounded-md border border-border px-3 py-1.5 text-sm text-foreground transition-colors hover:bg-muted"
          @click="changePwdOpen = true"
        >
          修改密码
        </button>
      </div>
    </div>
  </div>

  <!-- 修改密码弹窗：成功后刷新资料清除过期标记 -->
  <ChangePasswordDialog :open="changePwdOpen" @close="changePwdOpen = false" @saved="onPasswordSaved" />
</template>
