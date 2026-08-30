<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { Loader2, Link2 } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import StatementPicker from '@/components/statement/StatementPicker.vue'
import { useToastStore } from '@/stores/toast'
import { createPolicy, updatePolicy, getPolicy } from '@/api/policies'
import type { Policy } from '@/types'

// 新增 / 编辑权限策略弹窗。
// 授权语句独立成池管理（见「授权语句」菜单），策略只负责关联（选择已有语句）。
const props = withDefaults(
  defineProps<{
    open: boolean
    mode?: 'create' | 'edit'
    target?: Policy | null
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
  description: '',
  status: true,
  // 关联的授权语句 ID 列表（语句池共享引用）
  statement_ids: [] as number[],
})

const saving = ref(false)
const error = ref('')
const loadingDetail = ref(false)

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    if (props.mode === 'edit' && props.target) {
      form.name = props.target.name
      form.description = props.target.description
      form.status = props.target.status
      form.statement_ids = []
      loadingDetail.value = true
      try {
        const detail = await getPolicy(props.target.id)
        form.statement_ids = (detail.statements ?? []).map((s) => s.id)
      } catch (e) {
        toast.error((e as Error).message)
      } finally {
        loadingDetail.value = false
      }
    } else {
      form.name = ''
      form.description = ''
      form.status = true
      form.statement_ids = []
    }
    error.value = ''
  }
)

async function handleSave() {
  if (!form.name.trim()) {
    error.value = '请输入策略名'
    return
  }
  if (form.statement_ids.length === 0) {
    error.value = '至少关联一条授权语句'
    return
  }
  error.value = ''
  saving.value = true
  try {
    if (props.mode === 'create') {
      await createPolicy({
        name: form.name.trim(),
        description: form.description,
        status: form.status,
        statement_ids: form.statement_ids,
      })
      toast.success('策略创建成功')
    } else if (props.target) {
      await updatePolicy(props.target.id, {
        description: form.description,
        status: form.status,
        statement_ids: form.statement_ids,
      })
      toast.success('策略已更新')
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
    :title="mode === 'create' ? '新增权限策略' : '编辑权限策略'"
    width="42rem"
    @close="emit('close')"
  >
    <div v-if="loadingDetail" class="py-10 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-muted-foreground" />
    </div>
    <div v-else class="space-y-4">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">
            策略名
            <span class="text-destructive">*</span>
          </label>
          <input
            v-model="form.name"
            type="text"
            placeholder="唯一标识，如 EcsReadOnly"
            class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            :disabled="mode === 'edit'"
          />
        </div>
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">状态</label>
          <div
            class="flex h-9 items-center gap-2 rounded-md border border-border bg-background px-3"
          >
            <input
              v-model="form.status"
              id="policy-status"
              type="checkbox"
              class="h-4 w-4 rounded border-border text-primary focus:ring-primary/30"
            />
            <label for="policy-status" class="text-sm text-foreground">启用该策略</label>
          </div>
        </div>
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">描述</label>
        <textarea
          v-model="form.description"
          rows="2"
          placeholder="策略描述"
          class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>

      <!-- 关联授权语句（语句池） -->
      <div class="rounded-lg border border-border bg-card">
        <div class="flex items-center gap-2 border-b border-border bg-muted/30 px-4 py-2.5">
          <Link2 class="h-4 w-4 text-muted-foreground" />
          <div>
            <h3 class="text-sm font-semibold text-foreground">关联授权语句</h3>
            <p class="text-xs text-muted-foreground">
              从「授权语句」池中选择已有语句，语句独立维护，可被多个策略共享
            </p>
          </div>
        </div>
        <div class="p-4">
          <StatementPicker v-model="form.statement_ids" />
        </div>
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
          :disabled="saving || loadingDetail"
          @click="handleSave"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </template>
  </Modal>
</template>
