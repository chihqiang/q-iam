<script setup lang="ts">
import { ref, watch } from 'vue'
import { Loader2, Users } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'
import { resetAccountPassword } from '@/api/accounts'
import type { Account } from '@/types'

// 管理员重置密码弹窗
const props = withDefaults(
  defineProps<{
    open: boolean
    target?: Account | null
  }>(),
  {
    target: null,
  }
)

const emit = defineEmits<{
  close: []
  saved: []
}>()

const toast = useToastStore()

const password = ref('')
const saving = ref(false)
const error = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) {
      password.value = ''
      error.value = ''
    }
  }
)

async function handleReset() {
  if (!props.target) return
  if (!password.value) {
    error.value = '请输入新密码'
    return
  }
  if (password.value.length < 8) {
    error.value = '密码长度不能少于 8 位'
    return
  }
  error.value = ''
  saving.value = true
  try {
    await resetAccountPassword(props.target.id, { password: password.value })
    toast.success('密码已重置')
    emit('saved')
    emit('close')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Modal :open="open" title="重置密码" width="26rem" @close="emit('close')">
    <div class="space-y-3">
      <div
        class="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2.5 text-sm text-foreground"
      >
        <Users class="h-4 w-4 text-muted-foreground" />
        {{ target?.account_name }}（{{ target?.display_name || '未设置显示名' }}）
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">
          新密码
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="password"
          type="password"
          placeholder="至少 8 位，需包含小写字母和数字"
          class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>
      <p v-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
        {{ error }}
      </p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button
          class="rounded-md border border-border px-4 py-2 text-sm font-medium transition-colors hover:bg-muted"
          @click="emit('close')"
        >
          取消
        </button>
        <button
          class="flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50"
          :disabled="saving"
          @click="handleReset"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          {{ saving ? '重置中…' : '确认重置' }}
        </button>
      </div>
    </template>
  </Modal>
</template>
