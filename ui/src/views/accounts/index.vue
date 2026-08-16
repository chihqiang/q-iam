<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Plus, Pencil, Trash2, KeyRound } from '@lucide/vue'
import PageToolbar from '@/components/ui/PageToolbar.vue'
import DataTable, { type TableColumn } from '@/components/ui/DataTable.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import AccountFormDialog from '@/components/account/AccountFormDialog.vue'
import ResetPasswordDialog from '@/components/account/ResetPasswordDialog.vue'
import { useToastStore } from '@/stores/toast'
import { listAccounts, deleteAccount } from '@/api/accounts'
import type { Account } from '@/types'

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
  queryKey: ['accounts', { page, size, keyword, status: statusParam }],
  queryFn: () =>
    listAccounts({
      page: page.value,
      size: size.value,
      key: keyword.value || undefined,
      status: statusParam.value,
    }),
  placeholderData: (prev) => prev,
})

const accounts = computed(() => data.value?.data ?? [])
const total = computed(() => data.value?.total ?? 0)

const columns: TableColumn[] = [
  { key: 'id', label: 'ID', width: '64px' },
  { key: 'account_name', label: '账号名' },
  { key: 'display_name', label: '显示名' },
  { key: 'email', label: '邮箱' },
  { key: 'mobile', label: '手机号' },
  { key: 'status', label: '状态' },
  { key: 'allow_console', label: '控制台访问' },
  { key: 'created_at', label: '创建时间' },
  { key: 'actions', label: '操作', align: 'right' },
]

// ===== 弹窗状态 =====
const formDialog = ref<{
  open: boolean
  mode: 'create' | 'edit'
  target: Account | null
}>({ open: false, mode: 'create', target: null })
const resetDialog = ref<{ open: boolean; target: Account | null }>({
  open: false,
  target: null,
})

function openCreate() {
  formDialog.value = { open: true, mode: 'create', target: null }
}

function openEdit(account: Account) {
  formDialog.value = { open: true, mode: 'edit', target: account }
}

function openReset(account: Account) {
  resetDialog.value = { open: true, target: account }
}

function refreshList() {
  queryClient.invalidateQueries({ queryKey: ['accounts'] })
}

// ===== 删除 =====
const deleteOpen = ref(false)
const deleteTarget = ref<Account | null>(null)
const deleteSaving = ref(false)

function openDelete(account: Account) {
  deleteTarget.value = account
  deleteOpen.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleteSaving.value = true
  try {
    await deleteAccount(deleteTarget.value.id)
    toast.success('账号已删除')
    deleteOpen.value = false
    refreshList()
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    deleteSaving.value = false
  }
}

// 翻页时回到顶部
watch(page, () => {
  const el = document.querySelector('main')
  el?.scrollTo({ top: 0 })
})
</script>

<template>
  <div class="space-y-4">
    <!-- 工具栏 -->
    <PageToolbar
      v-model:keyword="keyword"
      v-model:status="statusFilter"
      placeholder="搜索账号名 / 显示名 / 邮箱"
      @search="handleSearch"
      @reset="handleResetFilters"
    >
      <button
        class="flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        新增账号
      </button>
    </PageToolbar>

    <!-- 表格 -->
    <DataTable
      :columns="columns"
      :data="accounts"
      :loading="isLoading"
      :total="total"
      v-model:page="page"
      :page-size="size"
    >
      <template #cell-account_name="{ row }">
        <span class="font-medium text-foreground">{{ (row as Account).account_name }}</span>
      </template>
      <template #cell-status="{ row }">
        <StatusBadge :value="(row as Account).status" />
      </template>
      <template #cell-allow_console="{ row }">
        <StatusBadge
          :value="(row as Account).allow_console"
          active-text="允许"
          inactive-text="禁止"
        />
      </template>
      <template #cell-created_at="{ value }">
        {{ (value as string)?.slice(0, 10) || '—' }}
      </template>
      <template #cell-actions="{ row }">
        <div class="flex items-center justify-end gap-1">
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
            title="编辑"
            @click="openEdit(row as Account)"
          >
            <Pencil class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-amber-500/10 hover:text-amber-500"
            title="重置密码"
            @click="openReset(row as Account)"
          >
            <KeyRound class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
            title="删除"
            @click="openDelete(row as Account)"
          >
            <Trash2 class="h-4 w-4" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- 弹窗组件 -->
    <AccountFormDialog
      :open="formDialog.open"
      :mode="formDialog.mode"
      :target="formDialog.target"
      @close="formDialog.open = false"
      @saved="refreshList"
    />
    <ResetPasswordDialog
      :open="resetDialog.open"
      :target="resetDialog.target"
      @close="resetDialog.open = false"
    />

    <!-- 删除确认 -->
    <ConfirmDialog
      :open="deleteOpen"
      title="删除账号"
      :message="`确定要删除账号「${deleteTarget?.account_name}」吗？删除后其授权关系将一并清除，该操作不可恢复。`"
      confirm-text="删除"
      danger
      :loading="deleteSaving"
      @confirm="handleDelete"
      @cancel="deleteOpen = false"
    />
  </div>
</template>
