<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Loader2 } from '@lucide/vue'
import Modal from '@/components/ui/Modal.vue'
import { useToastStore } from '@/stores/toast'
import { createAccount, updateAccount, getAccount } from '@/api/accounts'
import { allGroups } from '@/api/groups'
import type { Account } from '@/types'

// 新增 / 编辑账号弹窗
const props = withDefaults(
  defineProps<{
    open: boolean
    mode?: 'create' | 'edit'
    target?: Account | null
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

// 账号组下拉（组件内部自己拉取）
const { data: groupOptions } = useQuery({
  queryKey: ['groups-all'],
  queryFn: allGroups,
})

interface AccountForm {
  account_name: string
  display_name: string
  email: string
  mobile: string
  password: string
  status: boolean
  allow_console: boolean
  remark: string
  group_ids: number[]
}

const form = reactive<AccountForm>({
  account_name: '',
  display_name: '',
  email: '',
  mobile: '',
  password: '',
  status: true,
  allow_console: true,
  remark: '',
  group_ids: [],
})

const saving = ref(false)
const error = ref('')
const loadingDetail = ref(false)

function resetForm() {
  form.account_name = ''
  form.display_name = ''
  form.email = ''
  form.mobile = ''
  form.password = ''
  form.status = true
  form.allow_console = true
  form.remark = ''
  form.group_ids = []
  error.value = ''
}

// 打开时初始化：新增直接清空；编辑回填基础字段并异步拉取详情（含所属组）
watch(
  () => props.open,
  async (open) => {
    if (!open) return
    resetForm()
    if (props.mode === 'edit' && props.target) {
      form.account_name = props.target.account_name
      form.display_name = props.target.display_name
      form.email = props.target.email ?? ''
      form.mobile = props.target.mobile ?? ''
      form.status = props.target.status
      form.allow_console = props.target.allow_console
      form.remark = props.target.remark
      loadingDetail.value = true
      try {
        const detail = await getAccount(props.target.id)
        form.group_ids = (detail.groups ?? []).map((g) => g.id)
      } catch (e) {
        toast.error((e as Error).message)
      } finally {
        loadingDetail.value = false
      }
    }
  }
)

function validate(): string {
  if (props.mode === 'create') {
    if (!form.account_name.trim()) return '请输入账号名'
    if (!form.password) return '请输入初始密码'
    if (form.password.length < 8) return '密码长度不能少于 8 位'
  }
  if (form.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) return '邮箱格式不正确'
  return ''
}

async function handleSave() {
  const err = validate()
  if (err) {
    error.value = err
    return
  }
  error.value = ''
  saving.value = true
  try {
    if (props.mode === 'create') {
      await createAccount({
        account_name: form.account_name.trim(),
        display_name: form.display_name,
        email: form.email || undefined,
        mobile: form.mobile || undefined,
        password: form.password,
        status: form.status,
        allow_console: form.allow_console,
        remark: form.remark,
        group_ids: form.group_ids,
      })
      toast.success('账号创建成功')
    } else if (props.target) {
      await updateAccount(props.target.id, {
        display_name: form.display_name,
        email: form.email || undefined,
        mobile: form.mobile || undefined,
        status: form.status,
        allow_console: form.allow_console,
        remark: form.remark,
        group_ids: form.group_ids,
      })
      toast.success('账号已更新')
    }
    emit('saved')
    emit('close')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

const inputCls =
  'h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/20'
</script>

<template>
  <Modal
    :open="open"
    :title="mode === 'create' ? '新增账号' : '编辑账号'"
    width="34rem"
    @close="emit('close')"
  >
    <div v-if="loadingDetail" class="py-10 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-muted-foreground" />
    </div>
    <div v-else class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">
            账号名
            <span class="text-destructive">*</span>
          </label>
          <input
            v-model="form.account_name"
            type="text"
            placeholder="登录名，如 zhangsan"
            :class="inputCls"
            :disabled="mode === 'edit'"
          />
        </div>
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">显示名</label>
          <input v-model="form.display_name" type="text" placeholder="如 张三" :class="inputCls" />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">邮箱</label>
          <input
            v-model="form.email"
            type="text"
            placeholder="user@example.com"
            :class="inputCls"
          />
        </div>
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">手机号</label>
          <input v-model="form.mobile" type="text" placeholder="13800138000" :class="inputCls" />
        </div>
      </div>
      <div v-if="mode === 'create'" class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">
          初始密码
          <span class="text-destructive">*</span>
        </label>
        <input
          v-model="form.password"
          type="password"
          placeholder="至少 8 位，需包含小写字母和数字"
          :class="inputCls"
        />
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">备注</label>
        <textarea
          v-model="form.remark"
          rows="2"
          placeholder="备注信息"
          class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </div>
      <div class="flex items-center gap-2">
        <input
          v-model="form.status"
          id="account-status"
          type="checkbox"
          class="h-4 w-4 rounded border-border text-primary focus:ring-primary/30"
        />
        <label for="account-status" class="text-sm text-foreground">启用该账号</label>
      </div>
      <div class="flex items-center gap-2">
        <input
          v-model="form.allow_console"
          id="account-allow-console"
          type="checkbox"
          class="h-4 w-4 rounded border-border text-primary focus:ring-primary/30"
        />
        <label for="account-allow-console" class="text-sm text-foreground">允许进入控制台</label>
      </div>
      <div class="space-y-1.5">
        <label class="text-sm font-medium text-foreground">所属账号组</label>
        <div
          v-if="groupOptions && groupOptions.length > 0"
          class="flex flex-wrap gap-2 rounded-md border border-border bg-muted/20 p-3"
        >
          <label
            v-for="g in groupOptions"
            :key="g.id"
            class="flex cursor-pointer items-center gap-2 rounded-md border border-border bg-background px-3 py-1.5 text-sm transition-colors hover:border-primary"
            :class="form.group_ids.includes(g.id) ? 'border-primary bg-primary/5' : ''"
          >
            <input
              v-model="form.group_ids"
              type="checkbox"
              :value="g.id"
              class="h-3.5 w-3.5 rounded border-border text-primary focus:ring-primary/30"
            />
            {{ g.display_name || g.name }}
          </label>
        </div>
        <p v-else class="text-xs text-muted-foreground">暂无可用账号组</p>
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
