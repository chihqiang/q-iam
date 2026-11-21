<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { Loader2 } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'
import { createApp, updateApp } from '@/api/apps'
import type { AppCreateResponse, AppItem, GrantType } from '@/types'

// 新增 / 编辑应用弹窗
const props = withDefaults(
  defineProps<{
    open: boolean
    mode?: 'create' | 'edit'
    target?: AppItem | null
  }>(),
  {
    mode: 'create',
    target: null,
  }
)

const emit = defineEmits<{
  close: []
  saved: [created?: AppCreateResponse]
}>()

const toast = useToastStore()

const GRANT_TYPES: { value: GrantType; label: string }[] = [
  { value: 'client_credentials', label: '客户端凭证 client_credentials' },
  { value: 'authorization_code', label: '授权码 authorization_code' },
]

const form = reactive<{
  name: string
  description: string
  grant_type: GrantType
  callback_url: string
  status: boolean
}>({
  name: '',
  description: '',
  grant_type: 'client_credentials',
  callback_url: '',
  status: true,
})

const saving = ref(false)
const error = ref('')

watch(
  () => props.open,
  (open) => {
    if (!open) return
    if (props.mode === 'edit' && props.target) {
      form.name = props.target.name
      form.description = props.target.description
      form.grant_type = props.target.grant_type || 'client_credentials'
      form.callback_url = props.target.callback_url || ''
      form.status = props.target.status
    } else {
      form.name = ''
      form.description = ''
      form.grant_type = 'client_credentials'
      form.callback_url = ''
      form.status = true
    }
    error.value = ''
  }
)

async function handleSave() {
  if (!form.name.trim()) {
    error.value = '请输入应用名称'
    return
  }
  if (form.grant_type === 'authorization_code' && !form.callback_url.trim()) {
    error.value = '授权码模式需要填写回调地址'
    return
  }
  error.value = ''
  saving.value = true
  try {
    if (props.mode === 'create') {
      const resp = await createApp({
        name: form.name.trim(),
        description: form.description,
        grant_type: form.grant_type,
        callback_url: form.callback_url,
        status: form.status,
      })
      toast.success('应用创建成功')
      emit('saved', resp)
    } else if (props.target) {
      await updateApp(props.target.id, {
        name: form.name.trim(),
        description: form.description,
        grant_type: form.grant_type,
        callback_url: form.callback_url,
        status: form.status,
      })
      toast.success('应用已更新')
      emit('saved')
    }
    emit('close')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

const inputCls =
  'h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/20'
</script>

<template>
  <Modal
    :open="open"
    :title="mode === 'create' ? '新增应用' : '编辑应用'"
    width="34rem"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">
          应用名称
          <span class="text-destructive">*</span>
        </label>
        <input v-model="form.name" type="text" placeholder="如 web-console" :class="inputCls" />
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">描述</label>
        <textarea
          v-model="form.description"
          rows="2"
          placeholder="应用描述"
          class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">授权类型</label>
        <select
          v-model="form.grant_type"
          class="h-9 w-full rounded-md border border-border bg-background px-2 text-sm outline-none focus:border-primary"
        >
          <option v-for="g in GRANT_TYPES" :key="g.value" :value="g.value">{{ g.label }}</option>
        </select>
      </div>
      <div v-if="form.grant_type === 'authorization_code'" class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">
          回调地址 Callback URL
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="form.callback_url"
          type="text"
          placeholder="https://example.com/oauth/callback"
          :class="inputCls"
        />
      </div>
      <div class="flex items-center gap-2">
        <input
          v-model="form.status"
          id="app-status"
          type="checkbox"
          class="h-4 w-4 rounded border-border text-primary focus:ring-primary/30"
        />
        <label for="app-status" class="text-sm text-foreground">启用该应用</label>
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
          @click="handleSave"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </template>
  </Modal>
</template>
