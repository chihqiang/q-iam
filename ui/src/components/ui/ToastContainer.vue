<script setup lang="ts">
import { CheckCircle2, XCircle, Info } from '@lucide/vue'
import { storeToRefs } from 'pinia'
import { useToastStore } from '@/stores/toast'

const store = useToastStore()
const { toasts } = storeToRefs(store)

const icons = {
  success: CheckCircle2,
  error: XCircle,
  info: Info,
}
</script>

<template>
  <Teleport to="body">
    <div class="pointer-events-none fixed right-4 top-4 z-60 flex w-80 flex-col gap-2">
      <TransitionGroup name="toast">
        <div
          v-for="t in toasts"
          :key="t.id"
          class="pointer-events-auto flex items-start gap-3 rounded-lg border border-border bg-card px-4 py-3 shadow-lg"
        >
          <component
            :is="icons[t.type]"
            class="mt-0.5 h-4.5 w-4.5 shrink-0"
            :class="{
              'text-emerald-500': t.type === 'success',
              'text-destructive': t.type === 'error',
              'text-primary': t.type === 'info',
            }"
          />
          <p class="text-sm leading-snug text-foreground">{{ t.message }}</p>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.25s ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(16px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(16px);
}
</style>
