<script setup lang="ts">
import { Search, RotateCcw } from '@lucide/vue'

// 页面工具栏：搜索框 + 状态筛选 + 搜索/重置按钮，右侧通过默认插槽放操作按钮
withDefaults(
  defineProps<{
    keyword: string
    status?: string
    placeholder?: string
  }>(),
  {
    status: 'all',
    placeholder: '搜索…',
  }
)

const emit = defineEmits<{
  'update:keyword': [value: string]
  'update:status': [value: string]
  search: []
  reset: []
}>()
</script>

<template>
  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
      <div class="relative">
        <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          :value="keyword"
          type="text"
          :placeholder="placeholder"
          class="h-9 w-64 rounded-md border border-border bg-background pl-9 pr-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-2 focus:ring-primary/20"
          @input="emit('update:keyword', ($event.target as HTMLInputElement).value)"
          @keyup.enter="emit('search')"
        />
      </div>
      <select
        :value="status"
        class="h-9 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary"
        @change="emit('update:status', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">全部状态</option>
        <option value="true">已启用</option>
        <option value="false">已禁用</option>
      </select>
      <!-- 额外筛选（如类型下拉） -->
      <slot name="extra" />
      <button
        class="h-9 rounded-md bg-primary px-4 text-sm font-medium text-white transition-opacity hover:opacity-90"
        @click="emit('search')"
      >
        搜索
      </button>
      <button
        class="flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        @click="emit('reset')"
      >
        <RotateCcw class="h-3.5 w-3.5" />
        重置
      </button>
    </div>
    <slot />
  </div>
</template>
