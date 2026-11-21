<script setup lang="ts">
import { reactive, ref, computed, onMounted, watch } from 'vue'
import { Loader2, Plus, X, ShieldCheck } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import Select from '@/components/ui/Select.vue'
import { allGroups } from '@/api/groups'
import { createStatement, updateStatement, getStatement, statementToDTO, type StatementDTO, type ScopeDTO } from '@/api/statements'
import { useToastStore } from '@/stores/toast'
import { EFFECT_ALLOW, EFFECT_DENY } from '@/types'
import type { Statement, Group, Effect } from '@/types'

// 新增 / 编辑授权语句（语句池）弹窗
const props = withDefaults(
  defineProps<{
    open: boolean
    mode?: 'create' | 'edit'
    target?: Statement | null
  }>(),
  {
    mode: 'create',
    target: null,
  }
)

const emit = defineEmits<{
  close: []
  saved: []
}>()

const toast = useToastStore()

const EFFECTS = [EFFECT_ALLOW, EFFECT_DENY] as const

// 数据范围类型（对齐后端 DataScopeType）
const SCOPE_TYPES = [
  { value: 'all', label: '全部数据' },
  { value: 'group', label: '本用户分组' },
  { value: 'self', label: '仅本人数据' },
  { value: 'attribute', label: '按属性过滤' },
] as const

// 账号组列表（数据范围 group 类型下拉选择；受数据权限过滤，只能选到可见组）
const groups = ref<Group[]>([])
async function loadGroups() {
  try {
    groups.value = await allGroups()
  } catch {
    groups.value = []
  }
}
onMounted(loadGroups)

const groupSelectOptions = computed(() =>
  groups.value.map((g) => ({
    value: g.id,
    label: `${g.name}${g.display_name && g.display_name !== g.name ? '（' + g.display_name + '）' : ''}`,
  }))
)

const form = reactive({
  description: '',
  effect: EFFECT_ALLOW as Effect,
  action: '',
  resource: '*',
  scopes: [] as ScopeDTO[],
})

// Select 清空（clearable）单选会写回 null，归一到 undefined 以匹配 scope.group_id 类型
// 注意：必须在 form 声明之后 watch，否则 getter 立即求值时访问未初始化的 form（TDZ 报错）
watch(
  () => form.scopes,
  (scopes) => {
    for (const sc of scopes) {
      if (sc.group_id === null) sc.group_id = undefined
    }
  },
  { deep: true }
)

// 下拉无法表达「无选中」时展示的占位组（组已删除但数据里仍引用其 id）
function groupLabel(id: number | undefined): string {
  if (id == null) return ''
  const g = groups.value.find((x) => x.id === id)
  return g ? `${g.name}${g.display_name && g.display_name !== g.name ? '（' + g.display_name + '）' : ''}` : `组 #${id}（已不可用）`
}

const saving = ref(false)
const loadingDetail = ref(false)
const error = ref('')

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    error.value = ''
    if (props.mode === 'edit' && props.target) {
      // 列表接口不返回 scopes 明细，编辑时拉取详情（含数据范围）
      loadingDetail.value = true
      try {
        const detail = await getStatement(props.target.id)
        const dto = statementToDTO(detail)
        form.description = dto.description ?? ''
        form.effect = dto.effect
        form.action = dto.action
        form.resource = dto.resource ?? '*'
        form.scopes = dto.scopes
      } catch (e) {
        toast.error((e as Error).message)
      } finally {
        loadingDetail.value = false
      }
    } else {
      form.description = ''
      form.effect = EFFECT_ALLOW
      form.action = ''
      form.resource = '*'
      form.scopes = []
    }
  }
)

function newScope(): ScopeDTO {
  return { scope_type: 'all', group_id: undefined, owner_field: '', attr_key: '', attr_value: '', sort: 0 }
}

function addScope() {
  form.scopes.push(newScope())
}

function removeScope(index: number) {
  form.scopes.splice(index, 1)
}

function normalizePayload(): StatementDTO {
  return {
    description: form.description,
    effect: form.effect,
    action: form.action.trim(),
    resource: form.resource,
    scopes: form.scopes.map((sc, i) => ({ ...sc, sort: i })),
    sort: 0,
  }
}

async function handleSave() {
  if (!form.action.trim()) {
    error.value = '请输入操作 Action'
    return
  }
  error.value = ''
  saving.value = true
  try {
    if (props.mode === 'create') {
      await createStatement(normalizePayload())
      toast.success('授权语句创建成功')
    } else if (props.target) {
      await updateStatement(props.target.id, normalizePayload())
      toast.success('授权语句已更新')
    }
    emit('saved')
    emit('close')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="mode === 'create' ? '新增授权语句' : '编辑授权语句'"
    width="42rem"
    @close="emit('close')"
  >
    <div v-if="loadingDetail" class="py-10 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-muted-foreground" />
    </div>
    <div v-else class="space-y-4">
      <!-- 语句说明 -->
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">描述 Description</label>
        <input
          v-model="form.description"
          type="text"
          placeholder="本条授权规则的用途说明，如：允许查看账号列表"
          class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-[140px_1fr]">
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">
            效果
            <span class="text-destructive">*</span>
          </label>
          <select
            v-model="form.effect"
            class="h-9 w-full rounded-md border border-border bg-background px-2 text-sm outline-none focus:border-primary"
          >
            <option v-for="e in EFFECTS" :key="e" :value="e">{{ e }}</option>
          </select>
        </div>
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">
            操作 Action
            <span class="text-destructive">*</span>
          </label>
          <input
            v-model="form.action"
            type="text"
            placeholder="如 iam:ListAccounts,iam:GetAccount 或 *（逗号分隔，支持通配 iam:*）"
            class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </div>
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">资源 Resource</label>
        <input
          v-model="form.resource"
          type="text"
          placeholder="如 *（全部资源）或 deptA:account:*（支持通配，默认 *）"
          class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm font-mono outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
        <p class="text-xs text-muted-foreground">
          资源级授权（如按部门区分数据）；留空或 * 表示不限定资源。管理接口仅全资源（*）规则生效。
        </p>
      </div>

      <!-- 数据范围（数据权限） -->
      <div class="rounded-lg border border-border bg-card">
        <div class="flex items-center gap-2 border-b border-border bg-muted/30 px-4 py-2.5">
          <ShieldCheck class="h-4 w-4 text-muted-foreground" />
          <div>
            <h3 class="text-sm font-semibold text-foreground">数据范围</h3>
            <p class="text-xs text-muted-foreground">数据权限：可见/操作哪部分数据</p>
          </div>
          <button
            class="ml-auto flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            @click="addScope"
          >
            <Plus class="h-3 w-3" />
            添加数据范围
          </button>
        </div>
        <div class="space-y-2 p-4">
          <div
            v-if="form.scopes.length === 0"
            class="rounded-md border border-dashed border-border px-3 py-2.5 text-xs text-muted-foreground"
          >
            不限制数据范围（默认全部数据）
          </div>

          <div
            v-for="(scope, sci) in form.scopes"
            :key="sci"
            class="space-y-2 rounded-md border border-border bg-muted/20 px-2.5 py-2"
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
                @click="removeScope(sci)"
              >
                <X class="h-3.5 w-3.5" />
              </button>
            </div>

            <!-- group：本用户分组（从账号组下拉选择，支持模糊搜索） -->
            <div v-if="scope.scope_type === 'group'" class="space-y-1.5">
              <label class="text-xs text-muted-foreground">用户分组（数据记录按分组归属，多行=多组并集）</label>
              <Select
                v-model="scope.group_id"
                :options="groupSelectOptions"
                placeholder="请选择账号组"
                filterable
                clearable
              />
              <p v-if="scope.group_id && !groups.some((g) => g.id === scope.group_id)" class="text-xs text-amber-500">
                当前所选分组（{{ groupLabel(scope.group_id) }}）不在可选项内，保存后仍会按该分组过滤，但可能已被删除或无权限。
              </p>
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

      <p v-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
        {{ error }}
      </p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button
          class="rounded-md border border-border px-4 py-2 text-sm font-medium transition-colors hover:bg-muted"
          @click="emit('close')"
        >
          取消
        </button>
        <button
          class="flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50"
          :disabled="saving || loadingDetail"
          @click="handleSave"
        >
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </template>
  </Modal>
</template>
