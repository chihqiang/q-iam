<script setup lang="ts">
import { computed, ref } from 'vue'
import { Trash2, TriangleAlert, Loader2, ScrollText, KeyRound, CalendarDays } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import { useToastStore } from '@/stores/toast'
import { cleanupHistory } from '@/api/cleanup'
import type { CleanupResult } from '@/api/cleanup'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const toast = useToastStore()

// 保留天数：默认 30（清理 30 天以前的数据）
const days = ref(30)
const loading = ref(false)
const confirmOpen = ref(false)
const result = ref<CleanupResult | null>(null)

const daysInput = computed({
  get: () => days.value,
  set: (v: string | number) => {
    const n = Number(v)
    days.value = Number.isFinite(n) && n > 0 ? Math.floor(n) : 30
  },
})

// 危险操作二次确认
function openConfirm() {
  confirmOpen.value = true
}

async function handleCleanup() {
  confirmOpen.value = false
  loading.value = true
  result.value = null
  try {
    result.value = await cleanupHistory({ days: days.value })
    toast.success(`已清理 ${days.value} 天前的历史数据`)
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <Modal :open="open" title="清理历史数据" @close="emit('close')">
    <div class="space-y-4">
      <!-- 说明 -->
      <div class="flex items-start gap-3">
        <div
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-destructive/10 text-destructive"
        >
          <Trash2 class="h-5 w-5" />
        </div>
        <div>
          <p class="text-sm leading-relaxed text-muted-foreground">
            清理指定天数以前的历史数据，防止数据库无限膨胀。默认清理
            <span class="font-medium text-foreground">30 天以前</span>的数据。
          </p>
        </div>
      </div>

      <!-- 清理范围 -->
      <div class="space-y-2 rounded-md bg-muted/40 p-4 text-sm">
        <div class="flex items-center gap-2.5">
          <ScrollText class="h-4 w-4 shrink-0 text-muted-foreground" />
          <span class="text-muted-foreground">操作审计日志</span>
          <span class="ml-auto text-foreground">清理 {{ days }} 天以前</span>
        </div>
        <div class="flex items-center gap-2.5">
          <KeyRound class="h-4 w-4 shrink-0 text-muted-foreground" />
          <span class="text-muted-foreground">刷新令牌</span>
          <span class="ml-auto text-foreground">仅清理已过期记录</span>
        </div>
        <div class="flex items-center gap-2.5 text-xs text-muted-foreground">
          <TriangleAlert class="h-4 w-4 shrink-0" />
          <span>刷新令牌只清理已过期的，不影响有效会话；审计日志删除后不可恢复。</span>
        </div>
      </div>

      <!-- 清理结果 -->
      <div v-if="result" class="rounded-md border border-border p-4 text-sm">
        <div class="text-sm font-semibold text-foreground">清理完成</div>
        <div class="mt-3 grid grid-cols-2 gap-3">
          <div class="rounded-md bg-muted/40 p-3">
            <div class="text-xs text-muted-foreground">审计日志</div>
            <div class="mt-1 text-xl font-semibold text-foreground">{{ result.audit_logs }}</div>
          </div>
          <div class="rounded-md bg-muted/40 p-3">
            <div class="text-xs text-muted-foreground">刷新令牌（已过期）</div>
            <div class="mt-1 text-xl font-semibold text-foreground">{{ result.refresh_tokens }}</div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex flex-wrap items-center gap-3">
        <label class="flex items-center gap-2 text-sm text-muted-foreground">
          <CalendarDays class="h-4 w-4" />
          保留天数
        </label>
        <div class="flex items-center gap-1">
          <input
            v-model="daysInput"
            type="number"
            min="1"
            max="3650"
            class="h-9 w-24 rounded-md border border-border bg-background px-3 text-sm text-foreground outline-none focus:border-primary"
          />
          <span class="text-sm text-muted-foreground">天</span>
        </div>
        <button
          class="ml-auto flex h-9 items-center gap-2 rounded-md bg-destructive px-4 text-sm font-medium text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-60"
          :disabled="loading"
          @click="openConfirm"
        >
          <Loader2 v-if="loading" class="h-4 w-4 animate-spin" />
          <Trash2 v-else class="h-4 w-4" />
          {{ loading ? '清理中…' : '立即清理' }}
        </button>
      </div>
    </template>
  </Modal>

  <!-- 二次确认 -->
  <ConfirmDialog
    :open="confirmOpen"
    title="确认清理历史数据"
    :message="`将清理 ${days} 天以前的操作审计日志，以及已过期的刷新令牌记录。审计日志删除后不可恢复，确定继续吗？`"
    confirm-text="确认清理"
    danger
    @cancel="confirmOpen = false"
    @confirm="handleCleanup"
  />
</template>
