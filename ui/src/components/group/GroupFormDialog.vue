<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { Loader2 } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'
import { createGroup, updateGroup } from '@/api/groups'
import type { Group } from '@/types'

// 新增 / 编辑账号组弹窗
const props = withDefaults(
  defineProps<{
    open: boolean
    mode?: 'create' | 'edit'
    target?: Group | null
  }>(),
  {
    mode: 'create',
    target: null,
  }
)

const emit = defineEmits<{
  close: []
  saved: []
}>()

const toast = useToastStore()

const form = reactive({
  name: '',
  display_name: '',
  description: '',
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
      form.display_name = props.target.display_name
      form.description = props.target.description
      form.status = props.target.status
    } else {
      form.name = ''
      form.display_name = ''
      form.description = ''
      form.status = true
    }
    error.value = ''
  }
)

async function handleSave() {
  if (props.mode === 'create' && !form.name.trim()) {
    error.value = '请输入账号组名'
    return
  }
  error.value = ''
  saving.value = true
  try {
    if (props.mode === 'create') {
      await createGroup({
        name: form.name.trim(),
        display_name: form.display_name,
        description: form.description,
        status: form.status,
      })
      toast.success('账号组创建成功')
    } else if (props.target) {
      await updateGroup(props.target.id, {
        display_name: form.display_name,
        description: form.description,
        status: form.status,
      })
      toast.success('账号组已更新')
    }
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
  <Modal
    :open="open"
    :title="mode === 'create' ? '新增账号组' : '编辑账号组'"
    width="30rem"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">
          组名
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="form.name"
          type="text"
          placeholder="唯一标识，如 dev-team"
          class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          :disabled="mode === 'edit'"
        />
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">显示名</label>
        <input
          v-model="form.display_name"
          type="text"
          placeholder="如 研发团队"
          class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">描述</label>
        <textarea
          v-model="form.description"
          rows="2"
          placeholder="账号组描述"
          class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>
      <div class="flex items-center gap-2">
        <input
          v-model="form.status"
          id="group-status"
          type="checkbox"
          class="h-4 w-4 rounded border-border text-primary focus:ring-primary/30"
        />
        <label for="group-status" class="text-sm text-foreground">启用该账号组</label>
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
