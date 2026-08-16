<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Plus, Pencil, Trash2, Lock } from '@lucide/vue'
import PageToolbar from '@/components/ui/PageToolbar.vue'
import DataTable, { type TableColumn } from '@/components/ui/DataTable.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import PolicyFormDialog from '@/components/policy/PolicyFormDialog.vue'
import { useToastStore } from '@/stores/toast'
import { listPolicies, deletePolicy } from '@/api/policies'
import type { Policy } from '@/types'

const toast = useToastStore()
const queryClient = useQueryClient()

// ===== 筛选 =====
const keyword = ref('')
const statusFilter = ref('all')
const typeFilter = ref('all')
const page = ref(1)
const size = ref(10)

const statusParam = computed(() => {
  if (statusFilter.value === 'true') return true
  if (statusFilter.value === 'false') return false
  return undefined
})

const typeParam = computed(() => (typeFilter.value === 'all' ? undefined : typeFilter.value))

function handleSearch() {
  page.value = 1
}

function handleResetFilters() {
  keyword.value = ''
  statusFilter.value = 'all'
  typeFilter.value = 'all'
  page.value = 1
}

// ===== 列表 =====
const { data, isLoading } = useQuery({
  queryKey: ['policies', { page, size, keyword, status: statusParam, type: typeParam }],
  queryFn: () =>
    listPolicies({
      page: page.value,
      size: size.value,
      key: keyword.value || undefined,
      status: statusParam.value,
      type: typeParam.value,
    }),
  placeholderData: (prev) => prev,
})

const policies = computed(() => data.value?.data ?? [])
const total = computed(() => data.value?.total ?? 0)

const columns: TableColumn[] = [
  { key: 'id', label: 'ID', width: '64px' },
  { key: 'name', label: '策略名' },
  { key: 'type', label: '类型' },
  { key: 'description', label: '描述' },
  { key: 'status', label: '状态' },
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
  target: Policy | null
}>({ open: false, mode: 'create', target: null })

function openCreate() {
  formDialog.value = { open: true, mode: 'create', target: null }
}

function openEdit(policy: Policy) {
  formDialog.value = { open: true, mode: 'edit', target: policy }
}

function refreshList() {
  queryClient.invalidateQueries({ queryKey: ['policies'] })
  queryClient.invalidateQueries({ queryKey: ['policies-all'] })
}

// ===== 删除 =====
const deleteOpen = ref(false)
const deleteTarget = ref<Policy | null>(null)
const deleteSaving = ref(false)

function openDelete(policy: Policy) {
  deleteTarget.value = policy
  deleteOpen.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleteSaving.value = true
  try {
    await deletePolicy(deleteTarget.value.id)
    toast.success('策略已删除')
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
      v-model:status="statusFilter"
      placeholder="搜索策略名 / 描述"
      @search="handleSearch"
      @reset="handleResetFilters"
    >
      <template #extra>
        <select
          v-model="typeFilter"
          class="h-9 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary"
        >
          <option value="all">全部类型</option>
          <option value="custom">自定义</option>
          <option value="system">系统</option>
        </select>
      </template>
      <button
        class="flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        新增策略
      </button>
    </PageToolbar>

      <!-- 表格 -->
      <DataTable
        :columns="columns"
        :data="policies"
        :loading="isLoading"
        :total="total"
        v-model:page="page"
        :page-size="size"
      >
        <template #cell-name="{ row }">
          <span class="font-medium text-foreground">{{ (row as Policy).name }}</span>
        </template>
        <template #cell-type="{ row }">
          <span
            class="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium"
            :class="
              (row as Policy).type === 'system'
                ? 'bg-amber-500/10 text-amber-600'
                : 'bg-primary/10 text-primary'
            "
          >
            <Lock v-if="(row as Policy).type === 'system'" class="h-3 w-3" />
            {{ (row as Policy).type === 'system' ? '系统' : '自定义' }}
          </span>
        </template>
        <template #cell-description="{ row }">
          <span
            class="block max-w-64 truncate text-muted-foreground"
            :title="(row as Policy).description || ''"
          >
            {{ (row as Policy).description || '—' }}
          </span>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge :value="(row as Policy).status" />
        </template>
        <template #cell-created_at="{ value }">
          {{ (value as string)?.slice(0, 10) || '—' }}
        </template>
        <template #cell-actions="{ row }">
          <div class="flex items-center justify-end gap-1">
            <button
              class="rounded-md p-1.5 text-muted-foreground transition-colors disabled:cursor-not-allowed disabled:opacity-30"
              :class="
                (row as Policy).type === 'system' ? '' : 'hover:bg-primary/10 hover:text-primary'
              "
              title="编辑"
              :disabled="(row as Policy).type === 'system'"
              @click="openEdit(row as Policy)"
            >
              <Pencil class="h-4 w-4" />
            </button>
            <button
              class="rounded-md p-1.5 text-muted-foreground transition-colors disabled:cursor-not-allowed disabled:opacity-30"
              :class="
                (row as Policy).type === 'system'
                  ? ''
                  : 'hover:bg-destructive/10 hover:text-destructive'
              "
              title="删除"
              :disabled="(row as Policy).type === 'system'"
              @click="openDelete(row as Policy)"
            >
              <Trash2 class="h-4 w-4" />
            </button>
          </div>
        </template>
      </DataTable>

      <!-- 弹窗组件 -->
      <PolicyFormDialog
        :open="formDialog.open"
        :mode="formDialog.mode"
        :target="formDialog.target"
        @close="formDialog.open = false"
        @saved="refreshList"
      />

      <!-- 删除确认 -->
      <ConfirmDialog
        :open="deleteOpen"
        title="删除权限策略"
        :message="`确定要删除权限策略「${deleteTarget?.name}」吗？删除后所有绑定该策略的授权关系将一并清除，该操作不可恢复。`"
        confirm-text="删除"
        danger
        :loading="deleteSaving"
        @confirm="handleDelete"
        @cancel="deleteOpen = false"
      />
  </div>
</template>
