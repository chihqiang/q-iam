<script setup lang="ts">
import { ref, watch } from 'vue'
import { Copy, Check, KeyRound, TriangleAlert } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'

// 密钥展示弹窗：创建/重置应用后显示明文密钥（仅此一次）
const props = withDefaults(
  defineProps<{
    open: boolean
    appId?: string
    appSecret?: string
    isReset?: boolean
  }>(),
  {
    appId: '',
    appSecret: '',
    isReset: false,
  }
)

const emit = defineEmits<{
  close: []
}>()

const toast = useToastStore()
const copied = ref(false)

watch(
  () => props.open,
  (open) => {
    if (open) copied.value = false
  }
)

async function copyAppId() {
  if (!props.appId) return
  try {
    await navigator.clipboard.writeText(props.appId)
    toast.success('AppID 已复制')
  } catch {
    toast.error('复制失败，请手动复制')
  }
}

async function copySecret() {
  if (!props.appSecret) return
  try {
    await navigator.clipboard.writeText(props.appSecret)
    copied.value = true
    toast.success('密钥已复制')
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    toast.error('复制失败，请手动复制')
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="isReset ? '密钥已重置' : '应用创建成功'"
    width="34rem"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div
        class="flex items-start gap-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3"
      >
        <TriangleAlert class="mt-0.5 h-4.5 w-4.5 shrink-0 text-amber-500" />
        <p class="text-sm leading-relaxed text-amber-700">
          请立即保存客户端密钥，关闭后将无法再次查看。请妥善保管，勿泄露给他人。
        </p>
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">客户端 ID AppID</label>
        <div
          class="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-2.5"
        >
          <KeyRound class="h-4 w-4 shrink-0 text-muted-foreground" />
          <code class="min-w-0 flex-1 break-all font-mono text-sm text-foreground">
            {{ appId }}
          </code>
          <button
            class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="复制"
            @click="copyAppId"
          >
            <Copy class="h-4 w-4" />
          </button>
        </div>
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">客户端密钥 AppSecret</label>
        <div
          class="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-2.5"
        >
          <KeyRound class="h-4 w-4 shrink-0 text-muted-foreground" />
          <code class="min-w-0 flex-1 break-all font-mono text-sm text-foreground">
            {{ appSecret }}
          </code>
          <button
            class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="复制"
            @click="copySecret"
          >
            <Check v-if="copied" class="h-4 w-4 text-emerald-500" />
            <Copy v-else class="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button
          class="rounded-md bg-primary px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
          @click="emit('close')"
        >
          我已保存
        </button>
      </div>
    </template>
  </Modal>
</template>
