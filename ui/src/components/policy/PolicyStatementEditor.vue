<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Trash2, X, ChevronDown, ChevronRight } from '@lucide/vue'
import type { PolicyStatementDTO, PolicyScopeDTO } from '@/api/policies'

const EFFECTS = ['Allow', 'Deny'] as const

// 数据范围类型（对齐后端 DataScopeType）
const SCOPE_TYPES = [
  { value: 'all', label: '全部数据' },
  { value: 'group', label: '本用户分组' },
  { value: 'self', label: '仅本人数据' },
  { value: 'attribute', label: '按属性过滤' },
] as const

const model = defineModel<PolicyStatementDTO[]>({ required: true })

function newStatement(): PolicyStatementDTO {
  return {
    description: '',
    effect: 'Allow',
    action: '',
    scopes: [],
    sort: 0,
  }
}

function newScope(): PolicyScopeDTO {
  return { scope_type: 'all', group_id: undefined, owner_field: '', attr_key: '', attr_value: '', sort: 0 }
}

function addScope(stmt: PolicyStatementDTO) {
  stmt.scopes.push(newScope())
}

function removeScope(stmt: PolicyStatementDTO, index: number) {
  stmt.scopes.splice(index, 1)
}

function addStatement() {
  model.value.push(newStatement())
}

function removeStatement(index: number) {
  model.value.splice(index, 1)
}

const collapsed = ref<boolean[]>([])
function toggleCollapse(i: number) {
  collapsed.value[i] = !collapsed.value[i]
}
</script>

<template>
  <div class="space-y-2.5">
    <div class="flex items-center justify-between">
      <label class="text-sm font-medium text-foreground">授权语句（{{ model.length }}）</label>
      <button
        class="flex h-8 items-center gap-1 rounded-md bg-primary px-3 text-xs font-medium text-white transition-opacity hover:opacity-90"
        @click="addStatement"
      >
        <Plus class="h-3.5 w-3.5" />
        添加语句
      </button>
    </div>

    <div v-if="model.length > 0" class="space-y-2.5">
      <div
        v-for="(stmt, si) in model"
        :key="si"
        class="rounded-lg border border-border bg-muted/20"
      >
        <!-- 语句头 -->
        <div class="flex items-center gap-2 border-b border-border/70 px-3 py-2">
          <button
            class="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
            @click="toggleCollapse(si)"
          >
            <ChevronDown v-if="!collapsed[si]" class="h-4 w-4" />
            <ChevronRight v-else class="h-4 w-4" />
          </button>
          <span class="text-xs font-medium text-foreground">语句 {{ si + 1 }}</span>
          <span
            class="rounded px-1.5 py-0.5 text-[11px] font-medium"
            :class="
              stmt.effect === 'Allow'
                ? 'bg-emerald-500/10 text-emerald-600'
                : 'bg-destructive/10 text-destructive'
            "
          >
            {{ stmt.effect }}
          </span>
          <!-- 语句小标题（描述），让每条约款用途一目了然 -->
          <span class="min-w-0 flex-1 truncate text-xs text-foreground">
            {{ stmt.description || '未命名条款' }}
          </span>
          <span class="truncate font-mono text-xs text-muted-foreground">
            {{ stmt.action || '未设置操作' }}
          </span>
          <button
            class="ml-auto rounded p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
            title="删除语句"
            @click="removeStatement(si)"
          >
            <Trash2 class="h-3.5 w-3.5" />
          </button>
        </div>

        <!-- 语句详情 -->
        <div v-if="!collapsed[si]" class="space-y-3 p-3">
          <div class="space-y-1.5">
            <label class="text-sm font-medium text-foreground">描述 Description</label>
            <input
              v-model="stmt.description"
              type="text"
              placeholder="本条授权语句的用途说明，如：允许查看账号列表"
              class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-[140px_1fr]">
            <div class="space-y-1.5">
              <label class="text-sm font-medium text-foreground">效果</label>
              <select
                v-model="stmt.effect"
                class="h-9 w-full rounded-md border border-border bg-background px-2 text-sm outline-none focus:border-primary"
              >
                <option v-for="e in EFFECTS" :key="e" :value="e">{{ e }}</option>
              </select>
            </div>
            <div class="space-y-1.5">
              <label class="text-sm font-medium text-foreground">操作 Action</label>
              <input
                v-model="stmt.action"
                type="text"
                placeholder="如 iam:ListAccounts,iam:GetAccount 或 *（逗号分隔，支持通配 iam:*）"
                class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
          </div>
          <!-- 数据范围（数据权限） -->
          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-foreground">
                数据范围（{{ stmt.scopes.length }}）
              </label>
              <button
                class="flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                @click="addScope(stmt)"
              >
                <Plus class="h-3 w-3" />
                添加数据范围
              </button>
            </div>

            <div
              v-if="stmt.scopes.length === 0"
              class="rounded-md border border-dashed border-border px-3 py-2.5 text-xs text-muted-foreground"
            >
              不限制数据范围（默认全部数据）
            </div>

            <div
              v-for="(scope, sci) in stmt.scopes"
              :key="sci"
              class="space-y-2 rounded-md border border-border bg-background px-2.5 py-2"
            >
              <div class="flex items-center gap-2">
                <span class="shrink-0 text-xs font-medium text-muted-foreground">#{{ sci + 1 }}</span>
                <select
                  v-model="scope.scope_type"
                  class="h-8 w-40 rounded-md border border-border bg-background px-1.5 text-xs outline-none focus:border-primary"
                >
                  <option v-for="t in SCOPE_TYPES" :key="t.value" :value="t.value">{{ t.label }}</option>
                </select>
                <button
                  class="ml-auto shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                  title="删除数据范围"
                  @click="removeScope(stmt, sci)"
                >
                  <X class="h-3.5 w-3.5" />
                </button>
              </div>

              <!-- group：本用户分组 -->
              <div v-if="scope.scope_type === 'group'" class="space-y-1.5">
                <label class="text-xs text-muted-foreground">用户分组 ID（数据记录按分组归属）</label>
                <input
                  v-model.number="scope.group_id"
                  type="number"
                  placeholder="如 5"
                  class="h-8 w-full rounded-md border border-border bg-background px-2 text-xs font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>

              <!-- self：仅本人数据 -->
              <div v-else-if="scope.scope_type === 'self'" class="space-y-1.5">
                <label class="text-xs text-muted-foreground">数据归属字段（值为当前账号 ID）</label>
                <input
                  v-model="scope.owner_field"
                  type="text"
                  placeholder="如 creator_id"
                  class="h-8 w-full rounded-md border border-border bg-background px-2 text-xs font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>

              <!-- attribute：按属性过滤 -->
              <div v-else-if="scope.scope_type === 'attribute'" class="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <div class="space-y-1.5">
                  <label class="text-xs text-muted-foreground">属性键</label>
                  <input
                    v-model="scope.attr_key"
                    type="text"
                    placeholder="如 region"
                    class="h-8 w-full rounded-md border border-border bg-background px-2 text-xs font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div class="space-y-1.5">
                  <label class="text-xs text-muted-foreground">属性值</label>
                  <input
                    v-model="scope.attr_value"
                    type="text"
                    placeholder="如 华东"
                    class="h-8 w-full rounded-md border border-border bg-background px-2 text-xs font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div
      v-else
      class="rounded-lg border border-dashed border-border px-4 py-6 text-center text-sm text-muted-foreground"
    >
      尚未添加授权语句。
      <button class="text-primary hover:underline" @click="addStatement">点击添加</button>
    </div>
  </div>
</template>
