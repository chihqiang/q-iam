<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Search, RotateCcw, Loader2, User, Users, Blocks, Lock } from '@lucide/vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { useToastStore } from '@/stores/toast'
import { listPoliciesByPrincipal, grantPolicies, revokePolicies } from '@/api/grants'
import { allPolicies } from '@/api/policies'
import { allAccounts } from '@/api/accounts'
import { allGroups } from '@/api/groups'
import { allApps } from '@/api/apps'
import type { Account, AppItem, Group, Policy, PrincipalType } from '@/types'

const toast = useToastStore()
const queryClient = useQueryClient()

// ===== 主体类型 =====
type PrincipalTypeKey = 'account' | 'group' | 'app'

const principalTypes: { value: PrincipalTypeKey; label: string; icon: typeof User }[] = [
  { value: 'account', label: '账号', icon: User },
  { value: 'group', label: '账号组', icon: Users },
  { value: 'app', label: '应用', icon: Blocks },
]

const principalType = ref<PrincipalTypeKey>('account')
const principalId = ref<number | null>(null)

// 各类型主体列表（全部启用：授权下拉不能受分页 size=100 上限截断）
const { data: accounts } = useQuery({
  queryKey: ['accounts-all'],
  queryFn: allAccounts,
  enabled: () => principalType.value === 'account',
})

const { data: groups } = useQuery({
  queryKey: ['groups-all'],
  queryFn: allGroups,
  enabled: () => principalType.value === 'group',
})

const { data: apps } = useQuery({
  queryKey: ['apps-all'],
  queryFn: allApps,
  enabled: () => principalType.value === 'app',
})

// 当前主体的可选项
const principalOptions = computed(() => {
  switch (principalType.value) {
    case 'account':
      return (accounts.value ?? []).map((a) => ({
        id: a.id,
        label: `${a.display_name || a.account_name}（${a.account_name}）`,
      }))
    case 'group':
      return (groups.value ?? []).map((g) => ({
        id: g.id,
        label: `${g.display_name || g.name}（${g.name}）`,
      }))
    case 'app':
      return (apps.value ?? []).map((a) => ({
        id: a.id,
        label: `${a.name}（${a.app_id}）`,
      }))
    default:
      return []
  }
})

function principalLabel(id: number | null): string {
  return principalOptions.value.find((o) => o.id === id)?.label ?? ''
}

// ===== 当前主体展示信息（类型守卫辅助，避免模板内 as any）=====

type PrincipalDetail = Account | Group | AppItem

/** 展示名：账号取 display_name；组取 display_name||name；应用取 name */
function principalDisplayName(p: PrincipalDetail): string {
  if ('account_name' in p) return p.display_name || p.account_name
  if ('app_id' in p) return p.name
  return p.display_name || p.name
}

/** 副标题：账号取 account_name；应用取 app_id；组无则空 */
function principalSubName(p: PrincipalDetail): string {
  if ('account_name' in p) return p.account_name
  if ('app_id' in p) return p.app_id
  return ''
}

/** 状态徽章值（三类主体均有 status 字段） */
function principalStatus(p: PrincipalDetail): boolean {
  return Boolean(p.status)
}

// 切换类型时重置选择
function switchType(type: PrincipalTypeKey) {
  principalType.value = type
  principalId.value = null
}

// 当前主体详情（用于展示）
const currentPrincipal = computed(() => {
  if (principalId.value === null) return null
  switch (principalType.value) {
    case 'account':
      return (accounts.value ?? []).find((a) => a.id === principalId.value) ?? null
    case 'group':
      return (groups.value ?? []).find((g) => g.id === principalId.value) ?? null
    case 'app':
      return (apps.value ?? []).find((a) => a.id === principalId.value) ?? null
    default:
      return null
  }
})

// ===== 全部策略 =====
const { data: allPoliciesData } = useQuery({
  queryKey: ['policies-all'],
  queryFn: allPolicies,
})

// ===== 已绑定策略 =====
const {
  data: boundPolicies,
  isLoading: boundLoading,
} = useQuery({
  queryKey: ['grants', { type: principalType, id: principalId }],
  queryFn: () =>
    listPoliciesByPrincipal(principalType.value as PrincipalType, principalId.value as number),
  enabled: () => principalId.value !== null,
})

const boundIds = computed(() => new Set((boundPolicies.value ?? []).map((p) => p.id)))

// ===== 策略搜索/筛选 =====
const keyword = ref('')
const typeFilter = ref<'all' | 'system' | 'custom'>('all')

const filteredPolicies = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return (allPoliciesData.value ?? []).filter((p: Policy) => {
    if (typeFilter.value !== 'all' && p.type !== typeFilter.value) return false
    if (kw && !p.name.toLowerCase().includes(kw) && !(p.description ?? '').toLowerCase().includes(kw)) {
      return false
    }
    return true
  })
})

function resetFilter() {
  keyword.value = ''
  typeFilter.value = 'all'
}

// ===== 勾选即授权/解绑（即时生效） =====
const pending = ref<Set<number>>(new Set()) // 正在操作的策略 ID
const boundCount = computed(() => boundIds.value.size)

async function togglePolicy(policy: Policy) {
  if (principalId.value === null || pending.value.has(policy.id)) return
  const isBound = boundIds.value.has(policy.id)

  // 标记操作中，防止重复点击
  pending.value.add(policy.id)
  try {
    if (isBound) {
      await revokePolicies({
        principal_type: principalType.value as PrincipalType,
        principal_id: principalId.value,
        policy_ids: [policy.id],
      })
      toast.success(`已解除「${policy.name}」`)
    } else {
      await grantPolicies({
        principal_type: principalType.value as PrincipalType,
        principal_id: principalId.value,
        policy_ids: [policy.id],
      })
      toast.success(`已授权「${policy.name}」`)
    }
    // 刷新绑定列表
    queryClient.invalidateQueries({ queryKey: ['grants', { type: principalType, id: principalId }] })
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    pending.value.delete(policy.id)
  }
}
</script>

<template>
  <div class="space-y-4">
    <!-- 主体选择卡片 -->
    <div class="rounded-lg border border-border bg-card p-4">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center">
        <!-- 类型切换 -->
        <div class="flex shrink-0 items-center gap-1 rounded-lg bg-muted/60 p-1">
          <button
            v-for="t in principalTypes"
            :key="t.value"
            class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
            :class="
              principalType === t.value
                ? 'bg-card text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            "
            @click="switchType(t.value)"
          >
            <component :is="t.icon" class="h-3.5 w-3.5" />
            {{ t.label }}
          </button>
        </div>

        <!-- 主体下拉 -->
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <select
            v-model="principalId"
            class="h-9 w-full max-w-xs rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary"
          >
            <option :value="null">
              请选择{{ principalTypes.find((t) => t.value === principalType)?.label }}…
            </option>
            <option v-for="opt in principalOptions" :key="opt.id" :value="opt.id">
              {{ opt.label }}
            </option>
          </select>
        </div>
      </div>

      <!-- 当前主体信息 -->
      <div
        v-if="principalId !== null && currentPrincipal"
        class="mt-4 flex flex-wrap items-center gap-x-6 gap-y-2 rounded-md bg-muted/40 px-4 py-3 text-sm"
      >
        <span class="flex items-center gap-2 font-medium text-foreground">
          <component
            :is="principalTypes.find((t) => t.value === principalType)?.icon"
            class="h-4 w-4 text-muted-foreground"
          />
          {{ principalDisplayName(currentPrincipal) }}
        </span>
        <span v-if="principalSubName(currentPrincipal)" class="text-muted-foreground">
          {{ principalSubName(currentPrincipal) }}
        </span>
        <span class="ml-auto">
          <StatusBadge :value="principalStatus(currentPrincipal)" />
        </span>
      </div>
    </div>

    <!-- 策略授权卡片 -->
    <div class="rounded-lg border border-border bg-card">
      <!-- 卡片头 -->
      <div
        class="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-muted/30 px-4 py-2.5"
      >
        <div>
          <h3 class="text-sm font-semibold text-foreground">权限策略授权</h3>
          <p class="mt-0.5 text-xs text-muted-foreground">
            <template v-if="principalId !== null">
              已选主体「{{ principalLabel(principalId) }}」已绑定
              <span class="font-medium text-primary">{{ boundCount }}</span> 个策略，勾选 / 取消即生效
            </template>
            <template v-else>请先在上方选择主体</template>
          </p>
        </div>
      </div>

      <!-- 筛选工具栏 -->
      <div class="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
        <div
          class="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md border border-border bg-background px-2.5 sm:max-w-xs"
        >
          <Search class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <input
            v-model="keyword"
            type="text"
            placeholder="搜索策略名 / 描述"
            class="h-full w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
        </div>
        <select
          v-model="typeFilter"
          class="h-8 rounded-md border border-border bg-background px-2 text-sm outline-none focus:border-primary"
        >
          <option value="all">全部类型</option>
          <option value="custom">自定义</option>
          <option value="system">系统</option>
        </select>
        <button
          class="flex h-8 items-center gap-1 rounded-md border border-border px-2.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          @click="resetFilter"
        >
          <RotateCcw class="h-3 w-3" />
          重置
        </button>
      </div>

      <!-- 策略勾选列表 -->
      <div v-if="boundLoading && principalId !== null" class="py-10 text-center">
        <Loader2 class="mx-auto h-5 w-5 animate-spin text-muted-foreground" />
      </div>
      <div
        v-else-if="filteredPolicies.length === 0"
        class="px-4 py-10 text-center text-sm text-muted-foreground"
      >
        {{ principalId === null ? '选择主体后可勾选授权' : '没有匹配的策略' }}
      </div>
      <div v-else class="grid grid-cols-1 gap-1.5 p-3 sm:grid-cols-2">
        <label
          v-for="p in filteredPolicies"
          :key="p.id"
          class="flex cursor-pointer items-center gap-2.5 rounded-md border border-border bg-background px-3 py-2 text-sm transition-colors"
          :class="[
            boundIds.has(p.id) ? 'border-primary bg-primary/5' : 'hover:border-primary/60',
            pending.has(p.id) ? 'pointer-events-none opacity-60' : '',
          ]"
        >
          <input
            type="checkbox"
            class="h-3.5 w-3.5 shrink-0 rounded border-border text-primary focus:ring-primary/30"
            :checked="boundIds.has(p.id)"
            :disabled="principalId === null || pending.has(p.id)"
            @change="togglePolicy(p as Policy)"
          />
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-1.5">
              <span class="truncate font-medium text-foreground">{{ p.name }}</span>
              <Lock v-if="p.type === 'system'" class="h-3 w-3 shrink-0 text-amber-500" />
            </span>
            <span class="block truncate text-xs text-muted-foreground">
              {{ p.description || '暂无描述' }}
            </span>
          </span>
          <span
            v-if="p.type === 'system'"
            class="shrink-0 rounded bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600"
          >
            系统
          </span>
        </label>
      </div>
    </div>
  </div>
</template>
