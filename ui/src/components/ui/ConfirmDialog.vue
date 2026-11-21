<script setup lang="ts">
import { AlertTriangle } from '@lucide/vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    message: string
    confirmText?: string
    cancelText?: string
    loading?: boolean
    danger?: boolean
  }>(),
  {
    confirmText: '确认',
    cancelText: '取消',
    loading: false,
    danger: false,
  }
)

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
        @click.self="emit('cancel')"
      >
        <div class="absolute inset-0 bg-slate-950/60" />
        <div
          class="relative w-full max-w-sm overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
        >
          <div class="flex gap-4 px-5 py-5">
            <div
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full"
              :class="
                props.danger
                  ? 'bg-destructive/10 text-destructive'
                  : 'bg-amber-500/10 text-amber-500'
              "
            >
              <AlertTriangle class="h-5 w-5" />
            </div>
            <div>
              <h3 class="text-base font-semibold text-foreground">{{ title }}</h3>
              <p class="mt-1.5 text-sm leading-relaxed text-muted-foreground">{{ message }}</p>
            </div>
          </div>
          <div class="flex justify-end gap-2 border-t border-border px-5 py-3.5">
            <button
              class="rounded-md border border-border px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-muted"
              :disabled="props.loading"
              @click="emit('cancel')"
            >
              {{ cancelText }}
            </button>
            <button
              class="rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50"
              :class="
                props.danger ? 'bg-destructive hover:opacity-90' : 'bg-primary hover:opacity-90'
              "
              :disabled="props.loading"
              @click="emit('confirm')"
            >
              {{ loading ? '处理中…' : confirmText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.15s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
.modal-enter-from .relative,
.modal-leave-to .relative {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
</style>
