<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Plus, Pencil, Trash2, KeyRound, Blocks, ExternalLink } from '@lucide/vue'
import PageToolbar from '@/components/ui/PageToolbar.vue'
import DataTable, { type TableColumn } from '@/components/ui/DataTable.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import AppFormDialog from '@/components/app/AppFormDialog.vue'
import AppSecretDialog from '@/components/app/AppSecretDialog.vue'
import { useToastStore } from '@/stores/toast'
import { listApps, deleteApp, resetAppSecret } from '@/api/apps'
import type { AppCreateResponse, AppItem } from '@/types'

const toast = useToastStore()
const queryClient = useQueryClient()
const router = useRouter()

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
  queryKey: ['apps', { page, size, keyword, status: statusParam }],
  queryFn: () =>
    listApps({
      page: page.value,
      size: size.value,
      key: keyword.value || undefined,
      status: statusParam.value,
    }),
  placeholderData: (prev) => prev,
})

const apps = computed(() => data.value?.data ?? [])
const total = computed(() => data.value?.total ?? 0)

const columns: TableColumn[] = [
  { key: 'id', label: 'ID', width: '64px' },
  { key: 'name', label: '应用名称' },
  { key: 'app_id', label: '客户端 ID' },
  { key: 'grant_type', label: '授权类型' },
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
  target: AppItem | null
}>({ open: false, mode: 'create', target: null })

const secretDialog = ref<{
  open: boolean
  appId: string
  appSecret: string
  isReset: boolean
}>({ open: false, appId: '', appSecret: '', isReset: false })

function openCreate() {
  formDialog.value = { open: true, mode: 'create', target: null }
}

// 快速跳转到 OAuth 授权页（仅授权码方式应用）
function goAuthorize(app: AppItem) {
  if (app.grant_type !== 'authorization_code') return
  if (!app.callback_url) {
    toast.error('该应用未配置回调地址，无法发起授权')
    return
  }
  router.push({
    path: '/auth',
    query: { client_id: app.app_id, redirect_uri: app.callback_url },
  })
}

function openEdit(app: AppItem) {
  formDialog.value = { open: true, mode: 'edit', target: app }
}

function refreshList() {
  queryClient.invalidateQueries({ queryKey: ['apps'] })
}

function handleFormSaved(created?: AppCreateResponse) {
  // 创建成功时展示明文密钥（仅此一次）
  if (created) {
    secretDialog.value = {
      open: true,
      appId: created.app_id,
      appSecret: created.app_secret,
      isReset: false,
    }
  }
  refreshList()
}

// ===== 重置密钥 =====
const resetOpen = ref(false)
const resetTarget = ref<AppItem | null>(null)
const resetSaving = ref(false)

function openReset(app: AppItem) {
  resetTarget.value = app
  resetOpen.value = true
}

async function handleReset() {
  if (!resetTarget.value) return
  resetSaving.value = true
  try {
    const resp = await resetAppSecret(resetTarget.value.id)
    resetOpen.value = false
    secretDialog.value = {
      open: true,
      appId: resp.app_id,
      appSecret: resp.app_secret,
      isReset: true,
    }
    toast.success('密钥已重置')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    resetSaving.value = false
  }
}

// ===== 删除 =====
const deleteOpen = ref(false)
const deleteTarget = ref<AppItem | null>(null)
const deleteSaving = ref(false)

function openDelete(app: AppItem) {
  deleteTarget.value = app
  deleteOpen.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleteSaving.value = true
  try {
    await deleteApp(deleteTarget.value.id)
    toast.success('应用已删除')
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
      placeholder="搜索应用名 / 客户端 ID"
      @search="handleSearch"
      @reset="handleResetFilters"
    >
      <button
        class="flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="openCreate"
      >
        <Plus class="h-4 w-4" />
        新增应用
      </button>
    </PageToolbar>

    <!-- 表格 -->
    <DataTable
      :columns="columns"
      :data="apps"
      :loading="isLoading"
      :total="total"
      v-model:page="page"
      :page-size="size"
    >
      <template #cell-name="{ row }">
        <span class="flex items-center gap-2 font-medium text-foreground">
          <span class="flex h-6 w-6 items-center justify-center rounded bg-primary/10 text-primary">
            <Blocks class="h-3.5 w-3.5" />
          </span>
          {{ (row as AppItem).name }}
        </span>
      </template>
      <template #cell-app_id="{ row }">
        <code class="font-mono text-xs text-muted-foreground">{{ (row as AppItem).app_id }}</code>
      </template>
      <template #cell-grant_type="{ row }">
        <span
          class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
          :class="
            (row as AppItem).grant_type === 'authorization_code'
              ? 'bg-primary/10 text-primary'
              : 'bg-muted text-muted-foreground'
          "
        >
          {{ (row as AppItem).grant_type === 'authorization_code' ? '授权码' : '客户端凭证' }}
        </span>
      </template>
      <template #cell-status="{ row }">
        <StatusBadge :value="(row as AppItem).status" />
      </template>
      <template #cell-created_at="{ value }">
        {{ (value as string)?.slice(0, 10) || '—' }}
      </template>
      <template #cell-actions="{ row }">
        <div class="flex items-center justify-end gap-1">
          <!-- 仅授权码方式应用：快速跳转 OAuth 授权页 -->
          <button
            v-if="(row as AppItem).grant_type === 'authorization_code'"
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-cyan-500/10 hover:text-cyan-600"
            title="查看授权地址（跳转授权页）"
            @click="goAuthorize(row as AppItem)"
          >
            <ExternalLink class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-amber-500/10 hover:text-amber-500"
            title="重置密钥"
            @click="openReset(row as AppItem)"
          >
            <KeyRound class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
            title="编辑"
            @click="openEdit(row as AppItem)"
          >
            <Pencil class="h-4 w-4" />
          </button>
          <button
            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
            title="删除"
            @click="openDelete(row as AppItem)"
          >
            <Trash2 class="h-4 w-4" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- 弹窗组件 -->
    <AppFormDialog
      :open="formDialog.open"
      :mode="formDialog.mode"
      :target="formDialog.target"
      @close="formDialog.open = false"
      @saved="handleFormSaved"
    />
    <AppSecretDialog
      :open="secretDialog.open"
      :app-id="secretDialog.appId"
      :app-secret="secretDialog.appSecret"
      :is-reset="secretDialog.isReset"
      @close="secretDialog.open = false"
    />

    <!-- 重置密钥确认 -->
    <ConfirmDialog
      :open="resetOpen"
      title="重置密钥"
      :message="`确定要重置应用「${resetTarget?.name}」的客户端密钥吗？重置后旧密钥立即失效，新密钥仅显示一次。`"
      confirm-text="重置"
      danger
      :loading="resetSaving"
      @confirm="handleReset"
      @cancel="resetOpen = false"
    />

    <!-- 删除确认 -->
    <ConfirmDialog
      :open="deleteOpen"
      title="删除应用"
      :message="`确定要删除应用「${deleteTarget?.name}」吗？删除后其授权关系将一并清除，该操作不可恢复。`"
      confirm-text="删除"
      danger
      :loading="deleteSaving"
      @confirm="handleDelete"
      @cancel="deleteOpen = false"
    />
  </div>
</template>
