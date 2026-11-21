<script setup lang="ts">
import { Cookie, Check, X } from '@lucide/vue'
import { useConsentStore } from '@/stores/consent'

const consent = useConsentStore()
</script>

<template>
  <!-- 未做出同意选择时，底部弹出 Cookie 同意横幅 -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-300 ease-out"
      enter-from-class="translate-y-full opacity-0"
      enter-to-class="translate-y-0 opacity-100"
      leave-active-class="transition duration-200 ease-in"
      leave-from-class="translate-y-0 opacity-100"
      leave-to-class="translate-y-full opacity-0"
    >
      <div
        v-if="!consent.decided"
        class="fixed inset-x-0 bottom-0 z-[60] flex justify-center p-3 sm:p-4"
      >
        <div
          class="flex w-full max-w-3xl flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-2xl sm:flex-row sm:items-center"
        >
          <div class="flex items-start gap-3">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"
            >
              <Cookie class="h-5 w-5" />
            </div>
            <p class="text-sm leading-relaxed text-muted-foreground">
              我们使用 Cookie 来保障账号安全、维护登录状态并改善您的使用体验。
              继续使用即表示您同意我们的 Cookie 使用政策。
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <button
              class="flex h-9 flex-1 items-center justify-center gap-2 rounded-md border border-border px-4 text-sm text-foreground transition-colors hover:bg-muted sm:flex-none"
              @click="consent.decline()"
            >
              <X class="h-4 w-4" />
              仅必要
            </button>
            <button
              class="flex h-9 flex-1 items-center justify-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 sm:flex-none"
              @click="consent.accept()"
            >
              <Check class="h-4 w-4" />
              同意
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
