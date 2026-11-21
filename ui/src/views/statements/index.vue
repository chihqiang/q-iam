<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Plus, Pencil, Trash2, ShieldCheck } from '@lucide/vue'
import PageToolbar from '@/components/ui/PageToolbar.vue'
import DataTable, { type TableColumn } from '@/components/ui/DataTable.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import StatementFormDialog from '@/components/statement/StatementFormDialog.vue'
import { useToastStore } from '@/stores/toast'
import { listStatements, deleteStatement } from '@/api/statements'
import { EFFECT_ALLOW, EFFECT_DENY } from '@/types'
import type { Statement } from '@/types'

const toast = useToastStore()
const queryClient = useQueryClient()

// ===== 筛选 =====
const keyword = ref('')
const effectFilter = ref('all')
const page = ref(1)
const size = ref(10)

const effectParam = computed(() => (effectFilter.value === 'all' ? undefined : effectFilter.value))

function handleSearch() {
  page.value = 1
}

function handleResetFilters() {
  keyword.value = ''
  effectFilter.value = 'all'
  page.value = 1
}

// ===== 列表 =====
const { data, isLoading } = useQuery({
  queryKey: ['statements', { page, size, keyword, effect: effectParam }],
  queryFn: () =>
    listStatements({
      page: page.value,
      size: size.value,
      key: keyword.value || undefined,
      effect: effectParam.value,
    }),
  placeholderData: (prev) => prev,
})

const statements = computed(() => data.value?.data ?? [])
const total = computed(() => data.value?.total ?? 0)

const columns: TableColumn[] = [
  { key: 'id', label: 'ID', width: '64px' },
  { key: 'description', label: '描述' },
  { key: 'effect', label: '效果' },
  { key: 'action', label: '操作' },
  { key: 'resource', label: '资源' },
  { key: 'scopeCount', label: '数据范围' },
  { key: 'created_at', label: '创建时间' },
  { key: 'actions', label: '操作', align: 'right' },
]

watch(page, () => {
  const el = document.querySelector('main')
  el?.scrollTo({ top: 0 })
})

// ===== 弹窗状态 =====
const formDialog = ref<{
  open: boolean
  mode: 'create' | 'edit'
  target: Statement | null
}>({ open: false, mode: 'create', target: null })

function openCreate() {
  formDialog.value = { open: true, mode: 'create', target: null }
}

function openEdit(statement: Statement) {
  formDialog.value = { open: true, mode: 'edit', target: statement }
}

function refreshList() {
  queryClient.invalidateQueries({ queryKey: ['statements'] })
  queryClient.invalidateQueries({ queryKey: ['statements-all'] })
}

// ===== 删除 =====
const deleteOpen = ref(false)
const deleteTarget = ref<Statement | null>(null)
const deleteSaving = ref(false)

function openDelete(statement: Statement) {
  deleteTarget.value = statement
  deleteOpen.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleteSaving.value = true
  try {
    await deleteStatement(deleteTarget.value.id)
    toast.success('授权语句已删除')
    deleteOpen.value = false
    refreshList()
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    deleteSaving.value = false
  }
}
</script>

<template>
  <div class="space-y-4">
    <!-- 工具栏 -->
    <PageToolbar
      v-model:keyword="keyword"
      placeholder="搜索描述 / 操作…"
      @search="handleSearch"
      @reset="handleResetFilters"
    >
      <template #extra>
        <select
          v-model="effectFilter"
          class="h-9 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary"
        >
          <option value="all">全部效果</option>
          <option :value="EFFECT_ALLOW">Allow</option>
          <option :value="EFFECT_DENY">Deny</option>
        </select>
      </template>
      <button
        class="flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        新增授权语句
      </button>
    </PageToolbar>

    <!-- 列表 -->
    <DataTable
      :columns="columns"
      :data="statements"
      :loading="isLoading"
      :total="total"
      v-model:page="page"
      :page-size="size"
    >
      <template #cell-description="{ row }">
        <span
          class="flex items-center gap-1.5 font-medium text-foreground"
          :title="(row as Statement).description || ''"
        >
          <ShieldCheck class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <span class="truncate">{{ (row as Statement).description || `语句 #${(row as Statement).id}` }}</span>
        </span>
      </template>
      <template #cell-effect="{ row }">
        <StatusBadge
          :value="(row as Statement).effect === EFFECT_ALLOW"
          :active-text="EFFECT_ALLOW"
          :inactive-text="EFFECT_DENY"
        />
      </template>
      <template #cell-action="{ row }">
        <span class="block max-w-56 truncate font-mono text-xs text-foreground">
          {{ (row as Statement).action }}
        </span>
      </template>
      <template #cell-resource="{ row }">
        <span class="font-mono text-xs text-muted-foreground">
          {{ (row as Statement).resource || '*' }}
        </span>
      </template>
      <template #cell-scopeCount="{ row }">
        <span v-if="((row as Statement).scopes?.length ?? 0) === 0" class="text-xs text-muted-foreground">
          全部数据
        </span>
        <span v-else class="text-xs text-foreground">{{ (row as Statement).scopes?.length }} 条</span>
      </template>
      <template #cell-created_at="{ value }">
        {{ (value as string)?.slice(0, 10) || '—' }}
      </template>
      <template #cell-actions="{ row }">
        <div class="flex items-center justify-end gap-1">
          <button
            class="rounded p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="编辑"
            @click="openEdit(row as Statement)"
          >
            <Pencil class="h-4 w-4" />
          </button>
          <button
            class="rounded p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
            title="删除"
            @click="openDelete(row as Statement)"
          >
            <Trash2 class="h-4 w-4" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- 新建/编辑弹窗 -->
    <StatementFormDialog
      :open="formDialog.open"
      :mode="formDialog.mode"
      :target="formDialog.target"
      @close="formDialog.open = false"
      @saved="refreshList"
    />

    <!-- 删除确认 -->
    <ConfirmDialog
      :open="deleteOpen"
      title="删除授权语句"
      :message="`确定删除授权语句「${deleteTarget?.description || `#${deleteTarget?.id}`}」吗？被策略关联的语句无法删除。`"
      confirm-text="删除"
      :danger="true"
      :loading="deleteSaving"
      @confirm="handleDelete"
      @cancel="deleteOpen = false"
    />
  </div>
</template>
