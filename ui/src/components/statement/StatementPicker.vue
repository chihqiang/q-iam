<script setup lang="ts">
import { computed, watch } from 'vue'
import { Plus, Trash2, ShieldCheck } from '@lucide/vue'
import { useQuery } from '@tanstack/vue-query'
import Select from '@/components/ui/Select.vue'
import { allStatements } from '@/api/statements'
import { EFFECT_ALLOW, EFFECT_DENY } from '@/types'
import type { Statement } from '@/types'

// 策略关联授权语句选择器：
// 从「授权语句」池中选择已有语句（共享引用），v-model 为所选语句 ID 数组。
const model = defineModel<number[]>({ required: true })

// 语句池数据（受数据权限过滤）
const { data: pool } = useQuery({
  queryKey: ['statements-all'],
  queryFn: () => allStatements(),
  placeholderData: () => [],
})

const poolStatements = computed<Statement[]>(() => pool.value ?? [])

// 可选项 = 语句池 - 已选
const availableOptions = computed(() =>
  poolStatements.value
    .filter((s) => !model.value.includes(s.id))
    .map((s) => ({
      value: s.id,
      label: `${s.description || `语句 #${s.id}`}（${s.effect} ${s.action}）`,
    }))
)

// 已选语句（保持选择顺序，供展示）
const selectedStatements = computed<Statement[]>(() => {
  const byId = new Map(poolStatements.value.map((s) => [s.id, s]))
  return model.value
    .map((id) => byId.get(id))
    .filter((s): s is Statement => s != null)
})

// Select 选择（value 可能为 null/undefined，忽略空值）
function addStatement(value: string | number | null | undefined | (string | number)[]) {
  // 多选数组不适用本组件（单选用），忽略
  if (Array.isArray(value)) return
  const id = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(id) || id <= 0 || model.value.includes(id)) return
  model.value = [...model.value, id]
}

function removeStatement(id: number) {
  model.value = model.value.filter((x) => x !== id)
}

// 仅保留 pool 中存在的 id（清理编辑时可能残留的失效 id）
watch(
  () => poolStatements.value,
  () => {
    const valid = new Set(poolStatements.value.map((s) => s.id))
    model.value = model.value.filter((id) => valid.has(id))
  }
)
</script>

<template>
  <div class="space-y-2.5">
    <div class="flex items-center justify-between">
      <label class="text-sm font-medium text-foreground">
        已关联语句（{{ model.length }}）
      </label>
      <Select
        :model-value="undefined"
        :options="availableOptions"
        placeholder="+ 从语句池选择…"
        filterable
        class="w-64"
        @update:model-value="addStatement"
      />
    </div>

    <!-- 已选语句列表 -->
    <div v-if="selectedStatements.length > 0" class="space-y-2">
      <div
        v-for="s in selectedStatements"
        :key="s.id"
        class="flex items-center gap-2 rounded-md border border-border bg-muted/20 px-3 py-2"
      >
        <ShieldCheck class="h-4 w-4 shrink-0 text-muted-foreground" />
        <div class="min-w-0 flex-1">
          <div class="truncate text-sm text-foreground">
            {{ s.description || `语句 #${s.id}` }}
          </div>
          <div class="truncate font-mono text-xs text-muted-foreground">
            <span
              class="mr-1.5 rounded px-1 py-0.5 text-[10px] font-medium"
              :class="
                s.effect === EFFECT_ALLOW
                  ? 'bg-emerald-500/10 text-emerald-600'
                  : 'bg-destructive/10 text-destructive'
              "
            >
              {{ s.effect === EFFECT_ALLOW ? EFFECT_ALLOW : EFFECT_DENY }}
            </span>
            {{ s.action }}
          </div>
        </div>
        <button
          class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
          title="解除关联"
          @click="removeStatement(s.id)"
        >
          <Trash2 class="h-4 w-4" />
        </button>
      </div>
    </div>

    <div
      v-else
      class="flex items-center justify-center gap-2 rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground"
    >
      <Plus class="h-4 w-4" />
      尚未关联授权语句，请从上方下拉选择
    </div>
  </div>
</template>
