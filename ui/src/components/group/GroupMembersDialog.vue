<script setup lang="ts">
import { ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Loader2, UserPlus } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'
import { getGroup, replaceGroupMembers } from '@/api/groups'
import { allAccounts } from '@/api/accounts'
import type { Group } from '@/types'

// 账号组成员管理弹窗（勾选后整体覆盖组成员）
const props = withDefaults(
  defineProps<{
    open: boolean
    target?: Group | null
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

// 候选账号（用于勾选成员）
const { data: candidateAccounts } = useQuery({
  queryKey: ['accounts-all'],
  queryFn: allAccounts,
})

const memberIds = ref<number[]>([])
const loading = ref(false)
const saving = ref(false)

watch(
  () => props.open,
  async (open) => {
    if (!open || !props.target) return
    memberIds.value = []
    loading.value = true
    try {
      const detail = await getGroup(props.target.id)
      memberIds.value = (detail.accounts ?? []).map((a) => a.id)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      loading.value = false
    }
  }
)

async function handleSave() {
  if (!props.target) return
  saving.value = true
  try {
    await replaceGroupMembers(props.target.id, memberIds.value)
    toast.success('组成员已更新')
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
    :title="`管理成员 · ${target?.display_name || target?.name || ''}`"
    width="36rem"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div
        class="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2.5 text-sm text-foreground"
      >
        <UserPlus class="h-4 w-4 text-muted-foreground" />
        勾选账号后保存，将覆盖该组的全部成员
      </div>
      <div v-if="loading" class="py-8 text-center">
        <Loader2 class="mx-auto h-5 w-5 animate-spin text-muted-foreground" />
      </div>
      <div v-else-if="candidateAccounts && candidateAccounts.length > 0" class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">
          成员账号（已选 {{ memberIds.length }} 个）
        </label>
        <div
          class="grid max-h-80 grid-cols-1 gap-1.5 overflow-y-auto rounded-md border border-border bg-muted/20 p-3 sm:grid-cols-2"
        >
          <label
            v-for="acc in candidateAccounts"
            :key="acc.id"
            class="flex cursor-pointer items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-sm transition-colors hover:border-primary"
            :class="memberIds.includes(acc.id) ? 'border-primary bg-primary/5' : ''"
          >
            <input
              v-model="memberIds"
              type="checkbox"
              :value="acc.id"
              class="h-3.5 w-3.5 rounded border-border text-primary focus:ring-primary/30"
            />
            <span class="min-w-0">
              <span class="block truncate font-medium text-foreground">
                {{ acc.display_name || acc.account_name }}
              </span>
              <span class="block truncate text-xs text-muted-foreground">
                {{ acc.account_name }}
              </span>
            </span>
          </label>
        </div>
      </div>
      <p v-else class="rounded-md bg-muted/50 px-3 py-4 text-center text-sm text-muted-foreground">
        暂无可用账号，请先在「账号管理」中创建账号
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
          :disabled="saving || loading"
          @click="handleSave"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          {{ saving ? '保存中…' : '保存成员' }}
        </button>
      </div>
    </template>
  </Modal>
</template>
