<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Plus, Pencil, Trash2, Users } from '@lucide/vue'
import PageToolbar from '@/components/ui/PageToolbar.vue'
import DataTable, { type TableColumn } from '@/components/ui/DataTable.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import GroupFormDialog from '@/components/group/GroupFormDialog.vue'
import GroupMembersDialog from '@/components/group/GroupMembersDialog.vue'
import { useToastStore } from '@/stores/toast'
import { listGroups, deleteGroup } from '@/api/groups'
import type { Group } from '@/types'

const toast = useToastStore()
const queryClient = useQueryClient()

// ===== 筛选 =====
const keyword = ref('')
const statusFilter = ref('all')
const page = ref(1)
const size = ref(10)

const statusParam = computed(() => {
  if (statusFilter.value === 'true') return true
  if (statusFilter.value === 'false') return false
  return undefined
})

function handleSearch() {
  page.value = 1
}

function handleResetFilters() {
  keyword.value = ''
  statusFilter.value = 'all'
  page.value = 1
}

// ===== 列表 =====
const { data, isLoading } = useQuery({
  queryKey: ['groups', { page, size, keyword, status: statusParam }],
  queryFn: () =>
    listGroups({
      page: page.value,
      size: size.value,
      key: keyword.value || undefined,
      status: statusParam.value,
    }),
  placeholderData: (prev) => prev,
})

const groups = computed(() => data.value?.data ?? [])
const total = computed(() => data.value?.total ?? 0)

const columns: TableColumn[] = [
  { key: 'id', label: 'ID', width: '64px' },
  { key: 'name', label: '组名' },
  { key: 'display_name', label: '显示名' },
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
  target: Group | null
}>({ open: false, mode: 'create', target: null })
const memberDialog = ref<{ open: boolean; target: Group | null }>({
  open: false,
  target: null,
})

function openCreate() {
  formDialog.value = { open: true, mode: 'create', target: null }
}

function openEdit(group: Group) {
  formDialog.value = { open: true, mode: 'edit', target: group }
}

function openMembers(group: Group) {
  memberDialog.value = { open: true, target: group }
}

function refreshList() {
  queryClient.invalidateQueries({ queryKey: ['groups'] })
  queryClient.invalidateQueries({ queryKey: ['groups-all'] })
}

// ===== 删除 =====
const deleteOpen = ref(false)
const deleteTarget = ref<Group | null>(null)
const deleteSaving = ref(false)

function openDelete(group: Group) {
  deleteTarget.value = group
  deleteOpen.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleteSaving.value = true
  try {
    await deleteGroup(deleteTarget.value.id)
    toast.success('账号组已删除')
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
      placeholder="搜索组名 / 显示名"
      @search="handleSearch"
      @reset="handleResetFilters"
    >
      <button
        class="flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        新增账号组
      </button>
    </PageToolbar>

    <!-- 表格 -->
    <DataTable
      :columns="columns"
      :data="groups"
      :loading="isLoading"
      :total="total"
      v-model:page="page"
      :page-size="size"
    >
      <template #cell-name="{ row }">
        <span class="font-medium text-foreground">{{ (row as Group).name }}</span>
      </template>
      <template #cell-description="{ row }">
        <span
          class="block max-w-60 truncate text-muted-foreground"
          :title="(row as Group).description || ''"
        >
          {{ (row as Group).description || '—' }}
        </span>
      </template>
      <template #cell-status="{ row }">
        <StatusBadge :value="(row as Group).status" />
      </template>
      <template #cell-created_at="{ value }">
        {{ (value as string)?.slice(0, 10) || '—' }}
      </template>
      <template #cell-actions="{ row }">
        <div class="flex items-center justify-end gap-1">
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
            title="编辑"
            @click="openEdit(row as Group)"
          >
            <Pencil class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-emerald-500/10 hover:text-emerald-600"
            title="管理成员"
            @click="openMembers(row as Group)"
          >
            <Users class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
            title="删除"
            @click="openDelete(row as Group)"
          >
            <Trash2 class="h-4 w-4" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- 弹窗组件 -->
    <GroupFormDialog
      :open="formDialog.open"
      :mode="formDialog.mode"
      :target="formDialog.target"
      @close="formDialog.open = false"
      @saved="refreshList"
    />
    <GroupMembersDialog
      :open="memberDialog.open"
      :target="memberDialog.target"
      @close="memberDialog.open = false"
      @saved="refreshList"
    />

    <!-- 删除确认 -->
    <ConfirmDialog
      :open="deleteOpen"
      title="删除账号组"
      :message="`确定要删除账号组「${deleteTarget?.display_name || deleteTarget?.name}」吗？删除后组内成员关联与授权关系将一并清除，该操作不可恢复。`"
      confirm-text="删除"
      danger
      :loading="deleteSaving"
      @confirm="handleDelete"
      @cancel="deleteOpen = false"
    />
  </div>
</template>
