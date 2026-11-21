<script lang="ts">
import { defineComponent, inject } from 'vue'
// 与 <script setup> 同处模块作用域，hook 用别名避免重复标识符（TS2300）
import { onMounted as _onMounted, onBeforeUnmount as _onBeforeUnmount } from 'vue'

// SelectOption：纯注册占位组件（渲染 null，仅向父级 <Select> 注册选项）。
// 与 Select 同文件定义并导出，使用方按需引入：
//   import Select, { SelectOption } from '@/components/ui/Select.vue'
//   <Select v-model="v"><SelectOption :value="1" label="研发部" :disabled="false" /></Select>
export const SelectOption = defineComponent({
  name: 'SelectOption',
  props: {
    value: { type: [String, Number], required: true },
    label: { type: String, default: undefined },
    disabled: { type: Boolean, default: false },
  },
  setup(props) {
    const register = inject<
      ((opt: { value: string | number; label?: string; disabled?: boolean }) => void) | undefined
    >('select.register')
    const unregister = inject<((value: string | number) => void) | undefined>('select.unregister')
    _onMounted(() =>
      register?.({ value: props.value, label: props.label, disabled: props.disabled })
    )
    _onBeforeUnmount(() => unregister?.(props.value))
    return () => null
  },
})
</script>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, provide } from 'vue'
import { Check, ChevronDown, X } from '@lucide/vue'

// ---------- 选项类型（供 <SelectOption> 与 options prop 共用） ----------
export interface SelectOptionData {
  value: string | number
  label?: string
  disabled?: boolean
}

// 允许 undefined：单选初始未赋值（如表单字段缺省）也合法
type SelectValue = string | number | (string | number)[] | null | undefined

// ---------- Props / Emits ----------
const props = withDefaults(
  defineProps<{
    modelValue: SelectValue
    placeholder?: string
    disabled?: boolean
    /** 是否多选（多选时 modelValue 为数组） */
    multiple?: boolean
    /** 是否可搜索过滤 */
    filterable?: boolean
    /** 是否可一键清空 */
    clearable?: boolean
    /** 选项（也可用 <SelectOption> 子组件插槽方式声明） */
    options?: SelectOptionData[]
  }>(),
  {
    placeholder: '请选择',
    disabled: false,
    multiple: false,
    filterable: false,
    clearable: false,
    options: () => [],
  }
)

const emit = defineEmits<{
  'update:modelValue': [value: SelectValue]
  change: [value: SelectValue]
  open: []
  close: []
}>()

// ---------- 选项注册（子组件 <SelectOption> 注入） ----------
const slotOptions = ref<SelectOptionData[]>([])
provide('select.register', (opt: SelectOptionData) => {
  slotOptions.value = slotOptions.value.filter((o) => o.value !== opt.value)
  slotOptions.value.push(opt)
})
provide('select.unregister', (value: string | number) => {
  slotOptions.value = slotOptions.value.filter((o) => o.value !== value)
})

// 插槽选项优先于 options prop（便于自定义），两者取其一
const allOptions = computed<SelectOptionData[]>(() =>
  slotOptions.value.length > 0 ? slotOptions.value : props.options
)

// ---------- 选中值 ----------
const selectedValues = computed<(string | number)[]>(() => {
  if (props.multiple) {
    return Array.isArray(props.modelValue) ? props.modelValue : []
  }
  return props.modelValue != null && props.modelValue !== '' ? [props.modelValue as string | number] : []
})

function optionLabel(opt: SelectOptionData): string {
  return opt.label ?? String(opt.value)
}

function findOption(value: string | number): SelectOptionData | undefined {
  return allOptions.value.find((o) => o.value === value)
}

function labelOf(value: string | number): string {
  const opt = findOption(value)
  return opt ? optionLabel(opt) : String(value)
}

// 多选已选 label（供 tag 展示）
const selectedLabels = computed<string[]>(() => selectedValues.value.map(labelOf))

// ---------- 下拉状态 ----------
const open = ref(false)
const search = ref('')
const triggerRef = ref<HTMLDivElement | null>(null)
const dropdownRef = ref<HTMLDivElement | null>(null)
const searchRef = ref<HTMLInputElement | null>(null)
const activeIndex = ref(-1)

// 可选项（过滤后）
const filteredOptions = computed<SelectOptionData[]>(() => {
  const kw = search.value.trim().toLowerCase()
  if (!kw) return allOptions.value
  return allOptions.value.filter(
    (o) => optionLabel(o).toLowerCase().includes(kw) || String(o.value).toLowerCase().includes(kw)
  )
})

// 定位（fixed 基于 viewport 坐标，Teleport 到 body 避免父级 overflow 裁剪）
const dropdownStyle = ref<{ top: string; left: string; width: string; maxHeight: string }>({
  top: '0px',
  left: '0px',
  width: '0px',
  maxHeight: '256px',
})

function updateDropdownPos() {
  const el = triggerRef.value
  if (!el) return
  const r = el.getBoundingClientRect()
  const spaceBelow = window.innerHeight - r.bottom
  dropdownStyle.value = {
    top: `${r.bottom + 4}px`,
    left: `${r.left}px`,
    width: `${r.width}px`,
    maxHeight: spaceBelow > 240 ? '256px' : `${Math.max(spaceBelow - 8, 120)}px`,
  }
}

function openDropdown() {
  if (props.disabled) return
  open.value = true
  search.value = ''
  activeIndex.value = -1
  updateDropdownPos()
  emit('open')
  // 可搜索时自动聚焦
  if (props.filterable) {
    requestAnimationFrame(() => searchRef.value?.focus())
  }
}

function toggle() {
  if (open.value) closeDropdown()
  else openDropdown()
}

function closeDropdown() {
  if (!open.value) return
  open.value = false
  emit('close')
}

// ---------- 选择 ----------
function isSelected(v: string | number): boolean {
  return selectedValues.value.includes(v)
}

function selectOption(opt: SelectOptionData) {
  if (opt.disabled) return
  if (props.multiple) {
    const cur = Array.isArray(props.modelValue) ? [...props.modelValue] : []
    const idx = cur.indexOf(opt.value)
    if (idx >= 0) cur.splice(idx, 1)
    else cur.push(opt.value)
    emit('update:modelValue', cur)
    emit('change', cur)
    // 多选保持下拉与搜索词，便于连续勾选
    if (props.filterable) searchRef.value?.focus()
  } else {
    emit('update:modelValue', opt.value)
    emit('change', opt.value)
    closeDropdown()
  }
}

function clearAll(e: Event) {
  e.stopPropagation()
  if (props.disabled) return
  emit('update:modelValue', props.multiple ? [] : null)
  emit('change', props.multiple ? [] : null)
}

function removeTag(value: string | number, e: Event) {
  e.stopPropagation()
  const cur = Array.isArray(props.modelValue) ? [...props.modelValue] : []
  const idx = cur.indexOf(value)
  if (idx >= 0) {
    cur.splice(idx, 1)
    emit('update:modelValue', cur)
    emit('change', cur)
  }
}

// ---------- 外部点击关闭 ----------
// 注意：不能用 triggerRef.contains(e.target) 判断——filterable 下拉打开时，
// 模板会从「非可搜索分支」切换到「可搜索分支（搜索框）」，点击目标（如占位
// 文本 SPAN）会在事件冒泡到 document 前被移出 DOM，contains 对已移除节点返回
// false，导致刚打开的下拉被误判为「外部点击」而立即关闭。
// 改用坐标判断：点击位置是否落在触发元素矩形内（不依赖 DOM 节点存活）。
function onDocClick(e: MouseEvent) {
  if (!open.value) return
  const el = triggerRef.value
  if (el) {
    const r = el.getBoundingClientRect()
    const inside =
      e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom
    if (inside) return
  }
  // 下拉面板内点击不关闭（面板在 open 后渲染，其内元素不会因分支切换被移除）
  const t = e.target as Node
  if (t && dropdownRef.value?.contains(t)) return
  closeDropdown()
}

// ---------- 键盘导航 ----------
function onKeydown(e: KeyboardEvent) {
  if (props.disabled) return
  switch (e.key) {
    case 'Escape':
      closeDropdown()
      break
    case 'Enter': {
      if (open.value && activeIndex.value >= 0 && activeIndex.value < filteredOptions.value.length) {
        selectOption(filteredOptions.value[activeIndex.value])
        e.preventDefault()
      } else if (!open.value) {
        openDropdown()
      }
      break
    }
    case 'ArrowDown':
      if (!open.value) {
        openDropdown()
        break
      }
      e.preventDefault()
      if (filteredOptions.value.length === 0) break
      activeIndex.value = (activeIndex.value + 1) % filteredOptions.value.length
      scrollActiveIntoView()
      break
    case 'ArrowUp':
      if (!open.value) {
        openDropdown()
        break
      }
      e.preventDefault()
      if (filteredOptions.value.length === 0) break
      activeIndex.value =
        (activeIndex.value - 1 + filteredOptions.value.length) % filteredOptions.value.length
      scrollActiveIntoView()
      break
  }
}

function scrollActiveIntoView() {
  requestAnimationFrame(() => {
    const item = dropdownRef.value?.querySelector('[data-active="true"]') as HTMLElement | null
    item?.scrollIntoView({ block: 'nearest' })
  })
}

onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))

// 是否显示清空按钮：可清空 + 有选中值 + 未禁用 + 未禁用选项
const showClear = computed(
  () =>
    props.clearable &&
    !props.disabled &&
    (props.multiple ? selectedValues.value.length > 0 : props.modelValue != null && props.modelValue !== '')
)

// 是否为空（无选中值）
const isEmpty = computed(() => selectedValues.value.length === 0)
</script>

<template>
  <div class="relative" ref="triggerRef" @keydown="onKeydown">
    <!-- ===== 触发元素 ===== -->
    <div
      tabindex="0"
      role="combobox"
      :aria-expanded="open"
      :aria-disabled="disabled"
      class="flex min-h-9 w-full cursor-pointer select-none items-center gap-1 rounded-md border border-border bg-background px-3 text-sm outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/20"
      :class="[disabled ? 'cursor-not-allowed opacity-60' : open ? 'border-primary ring-2 ring-primary/20' : 'hover:border-primary/50']"
      @click="toggle"
    >
      <!-- 可搜索：下拉打开时显示搜索框 -->
      <input
        v-if="open && filterable"
        ref="searchRef"
        v-model="search"
        type="text"
        :placeholder="selectedLabels.length ? selectedLabels[0] : placeholder"
        class="h-7 min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        @click.stop
        @keydown.stop
      />

      <!-- 非可搜索：多选 tags / 单选 label / 占位 -->
      <template v-else>
        <!-- 多选：已选 tags（可换行、占满剩余空间，按钮组固定靠右） -->
        <template v-if="multiple && !isEmpty">
          <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1">
            <span
              v-for="(label, i) in selectedLabels"
              :key="selectedValues[i]"
              class="inline-flex max-w-40 items-center gap-1 rounded bg-accent px-1.5 py-0.5 text-xs font-medium text-accent-foreground"
            >
              <span class="truncate">{{ label }}</span>
              <button
                type="button"
                class="shrink-0 rounded p-0.5 hover:bg-primary/10"
                :disabled="disabled"
                @click="removeTag(selectedValues[i], $event)"
              >
                <X class="h-3 w-3" />
              </button>
            </span>
          </div>
        </template>
        <!-- 单选 / 空 -->
        <span
          v-else
          class="min-w-0 flex-1 truncate"
          :class="isEmpty ? 'text-muted-foreground' : 'text-foreground'"
        >
          {{ isEmpty ? placeholder : selectedLabels[0] }}
        </span>
      </template>

      <!-- 右侧按钮组：清空 + 下拉箭头（打包靠右，紧凑排列，避免两处 ml-auto 对分空隙） -->
      <div class="ml-auto flex shrink-0 items-center gap-0.5">
        <!-- 清空按钮 -->
        <button
          v-if="showClear"
          type="button"
          class="rounded p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          @click="clearAll($event)"
        >
          <X class="h-3.5 w-3.5" />
        </button>
        <ChevronDown
          class="h-4 w-4 shrink-0 text-muted-foreground transition-transform"
          :class="open && 'rotate-180'"
        />
      </div>
    </div>

    <!-- ===== 下拉面板（Teleport 到 body，fixed 定位） ===== -->
    <Teleport to="body">
      <Transition name="select-dropdown">
        <div
          v-if="open"
          ref="dropdownRef"
          :style="dropdownStyle"
          class="fixed z-50 overflow-hidden rounded-md border border-border bg-card shadow-xl"
        >
          <div class="max-h-full overflow-y-auto py-1">
            <!-- 空态 -->
            <div v-if="filteredOptions.length === 0" class="px-3 py-6 text-center text-sm text-muted-foreground">
              无匹配数据
            </div>

            <!-- 选项列表 -->
            <div
              v-for="(opt, i) in filteredOptions"
              :key="String(opt.value)"
              :data-active="i === activeIndex"
              role="option"
              :aria-selected="isSelected(opt.value)"
              class="flex cursor-pointer items-center gap-2 px-3 py-2 text-sm transition-colors"
              :class="[
                opt.disabled
                  ? 'cursor-not-allowed text-muted-foreground/60'
                  : i === activeIndex
                    ? 'bg-accent text-accent-foreground'
                    : 'hover:bg-muted',
              ]"
              @click="selectOption(opt)"
              @mouseenter="activeIndex = i"
            >
              <span class="min-w-0 flex-1 truncate">
                <!-- 自定义选项模板 -->
                <slot name="option" :option="opt" :label="optionLabel(opt)">
                  {{ optionLabel(opt) }}
                </slot>
              </span>
              <Check
                v-if="isSelected(opt.value) && !opt.disabled"
                class="h-4 w-4 shrink-0 text-primary"
              />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 渲染默认插槽（<SelectOption> 等纯注册组件，无 DOM 输出，仅用于挂载注册） -->
    <slot />
  </div>
</template>

<style scoped>
.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition:
    opacity 0.12s ease,
    transform 0.12s ease;
  transform-origin: top;
}
.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
