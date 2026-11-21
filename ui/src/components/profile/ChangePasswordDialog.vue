<script setup lang="ts">
import { ref, watch } from 'vue'
import { Loader2 } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'
import { changeOwnPassword } from '@/api/profile'

// 个人中心：当前登录账号修改自己密码的弹窗
const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const toast = useToastStore()

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const saving = ref(false)
const error = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) {
      oldPassword.value = ''
      newPassword.value = ''
      confirmPassword.value = ''
      error.value = ''
    }
  }
)

async function handleChangePassword() {
  if (!oldPassword.value) {
    error.value = '请输入当前密码'
    return
  }
  if (!newPassword.value) {
    error.value = '请输入新密码'
    return
  }
  if (newPassword.value.length < 8) {
    error.value = '新密码长度不能少于 8 位'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = '两次输入的新密码不一致'
    return
  }
  error.value = ''
  saving.value = true
  try {
    await changeOwnPassword({ old_password: oldPassword.value, new_password: newPassword.value })
    toast.success('密码修改成功')
    emit('saved')
    emit('close')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Modal :open="open" title="修改密码" width="440px" @close="emit('close')">
    <div class="space-y-4">
      <div>
        <label class="mb-1.5 block text-sm text-foreground">当前密码</label>
        <input
          v-model="oldPassword"
          type="password"
          class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-ring"
          placeholder="请输入当前密码"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm text-foreground">新密码</label>
        <input
          v-model="newPassword"
          type="password"
          class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-ring"
          placeholder="至少 8 位，需包含字母和数字"
        />
      </div>
      <div>
        <label class="mb-1.5 block text-sm text-foreground">确认新密码</label>
        <input
          v-model="confirmPassword"
          type="password"
          class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-ring"
          placeholder="再次输入新密码"
        />
      </div>
      <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
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
          @click="handleChangePassword"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          {{ saving ? '提交中…' : '确认修改' }}
        </button>
      </div>
    </template>
  </Modal>
</template>
