<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Search, RotateCcw, ScrollText, CircleCheck, CircleX } from '@lucide/vue'
import DataTable, { type TableColumn } from '@/components/ui/DataTable.vue'
import { listAuditLogs, auditModules, MODULE_LABELS, ACTION_LABELS } from '@/api/audit'
import type { AuditLogItem } from '@/api/audit'

// ===== 筛选 =====
const keyword = ref('')
const moduleFilter = ref('all')
const actionFilter = ref('all')
const successFilter = ref('all')
const operatorFilter = ref('')
const fromDate = ref('')
const toDate = ref('')
const page = ref(1)
const size = ref(20)

// date input 值（YYYY-MM-DD）转为 RFC3339（后端 time.Parse(time.RFC3339) 解析）
function dateToRFC3339(date: string, endOfDay: boolean): string | undefined {
  if (!date) return undefined
  return `${date}T${endOfDay ? '23:59:59' : '00:00:00'}Z`
}

const { data: modules } = useQuery({
  queryKey: ['audit-modules'],
  queryFn: auditModules,
})

const moduleParam = computed(() => (moduleFilter.value === 'all' ? undefined : moduleFilter.value))
const actionParam = computed(() => (actionFilter.value === 'all' ? undefined : actionFilter.value))
const successParam = computed(() => {
  if (successFilter.value === 'true') return true
  if (successFilter.value === 'false') return false
  return undefined
})

function handleSearch() {
  page.value = 1
}

function handleResetFilters() {
  keyword.value = ''
  moduleFilter.value = 'all'
  actionFilter.value = 'all'
  successFilter.value = 'all'
  operatorFilter.value = ''
  fromDate.value = ''
  toDate.value = ''
  page.value = 1
}

// ===== 列表 =====
const { data, isLoading } = useQuery({
  queryKey: [
    'audit-logs',
    {
      page,
      size,
      keyword,
      module: moduleParam,
      action: actionParam,
      success: successParam,
      operator: operatorFilter,
      from: fromDate,
      to: toDate,
    },
  ],
  queryFn: () =>
    listAuditLogs({
      page: page.value,
      size: size.value,
      key: keyword.value || undefined,
      module: moduleParam.value,
      action: actionParam.value,
      success: successParam.value,
      operator: operatorFilter.value || undefined,
      from: dateToRFC3339(fromDate.value, false),
      to: dateToRFC3339(toDate.value, true),
    }),
  placeholderData: (prev) => prev,
})

const logs = computed(() => data.value?.data ?? [])
const total = computed(() => data.value?.total ?? 0)

const columns: TableColumn[] = [
  { key: 'id', label: 'ID', width: '70px' },
  { key: 'created_at', label: '操作时间', width: '170px' },
  { key: 'operator_name', label: '操作人' },
  { key: 'module', label: '模块' },
  { key: 'action', label: '动作' },
  { key: 'detail', label: '操作详情' },
  { key: 'client_ip', label: 'IP' },
  { key: 'latency_ms', label: '耗时' },
  { key: 'result', label: '结果' },
]

watch(page, () => {
  const el = document.querySelector('main')
  el?.scrollTo({ top: 0 })
})

const selectCls =
  'h-9 rounded-md border border-border bg-background px-2 text-sm outline-none focus:border-primary'
</script>

<template>
  <div class="space-y-4">
    <!-- 工具栏 -->
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
        <div class="relative">
          <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            v-model="keyword"
            type="text"
            placeholder="搜索详情 / 路径 / 操作人"
            class="h-9 w-56 rounded-md border border-border bg-background pl-9 pr-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-2 focus:ring-primary/20"
            @keyup.enter="handleSearch"
          />
        </div>
        <select v-model="moduleFilter" :class="selectCls">
          <option value="all">全部模块</option>
          <option v-for="m in modules ?? []" :key="m" :value="m">
            {{ MODULE_LABELS[m] || m }}
          </option>
        </select>
        <select v-model="actionFilter" :class="selectCls">
          <option value="all">全部动作</option>
          <option v-for="(label, value) in ACTION_LABELS" :key="value" :value="value">
            {{ label }}
          </option>
        </select>
        <select v-model="successFilter" :class="selectCls">
          <option value="all">全部结果</option>
          <option value="true">成功</option>
          <option value="false">失败</option>
        </select>
        <input
          v-model="operatorFilter"
          type="text"
          placeholder="操作人"
          class="h-9 w-32 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary"
          @keyup.enter="handleSearch"
        />
        <input
          v-model="fromDate"
          type="date"
          title="开始日期"
          class="h-9 rounded-md border border-border bg-background px-2 text-sm text-muted-foreground outline-none focus:border-primary"
          @change="handleSearch"
        />
        <span class="text-sm text-muted-foreground">至</span>
        <input
          v-model="toDate"
          type="date"
          title="结束日期"
          class="h-9 rounded-md border border-border bg-background px-2 text-sm text-muted-foreground outline-none focus:border-primary"
          @change="handleSearch"
        />
        <button
          class="h-9 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
          @click="handleSearch"
        >
          搜索
        </button>
        <button
          class="flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          @click="handleResetFilters"
        >
          <RotateCcw class="h-3.5 w-3.5" />
          重置
        </button>
      </div>
      <div class="flex items-center gap-2 text-xs text-muted-foreground">
        <ScrollText class="h-4 w-4" />
        共 {{ total }} 条操作记录
      </div>
    </div>

    <!-- 表格 -->
    <DataTable
      :columns="columns"
      :data="logs"
      :loading="isLoading"
      :total="total"
      v-model:page="page"
      :page-size="size"
      empty-text="暂无操作记录"
    >
      <template #cell-created_at="{ value }">
        <span class="font-mono text-xs text-muted-foreground">
          {{ (value as string)?.slice(0, 19) }}
        </span>
      </template>
      <template #cell-operator_name="{ value }">
        <span class="font-medium text-foreground">{{ value || '—' }}</span>
      </template>
      <template #cell-module="{ value }">
        <span
          class="inline-flex rounded px-2 py-0.5 text-xs font-medium"
          :class="
            value === 'auth' ? 'bg-indigo-500/10 text-indigo-500' : 'bg-primary/10 text-primary'
          "
        >
          {{ MODULE_LABELS[value as string] || (value as string) }}
        </span>
      </template>
      <template #cell-action="{ value }">
        <span class="text-foreground">
          {{ ACTION_LABELS[value as string] || (value as string) }}
        </span>
      </template>
      <template #cell-detail="{ row }">
        <span
          class="block max-w-56 truncate text-muted-foreground"
          :title="(row as AuditLogItem).detail || (row as AuditLogItem).path"
        >
          {{ (row as AuditLogItem).detail || (row as AuditLogItem).path }}
        </span>
      </template>
      <template #cell-client_ip="{ value }">
        <code class="font-mono text-xs text-muted-foreground">{{ value || '—' }}</code>
      </template>
      <template #cell-latency_ms="{ value }">
        <span class="text-xs text-muted-foreground">{{ value }}ms</span>
      </template>
      <template #cell-result="{ row }">
        <span
          class="inline-flex items-center gap-1 text-xs font-medium"
          :class="(row as AuditLogItem).success ? 'text-emerald-500' : 'text-destructive'"
          :title="(row as AuditLogItem).error_msg || ''"
        >
          <CircleCheck v-if="(row as AuditLogItem).success" class="h-3.5 w-3.5" />
          <CircleX v-else class="h-3.5 w-3.5" />
          {{ (row as AuditLogItem).success ? '成功' : '失败' }}
        </span>
      </template>
    </DataTable>
  </div>
</template>
