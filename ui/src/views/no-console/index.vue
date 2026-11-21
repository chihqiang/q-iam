<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ShieldCheck, ShieldX, ArrowLeft } from '@lucide/vue'

const router = useRouter()
</script>

<template>
  <div
    class="relative flex min-h-screen items-center justify-center overflow-hidden bg-linear-to-br from-slate-50 via-white to-cyan-50/60 p-4"
  >
    <!-- 浅色网格纹理（中心清晰、边缘渐隐） -->
    <div class="tech-grid-light pointer-events-none absolute inset-0" />

    <!-- 柔和霓虹光斑 -->
    <div class="pointer-events-none absolute inset-0">
      <div class="absolute -left-40 -top-40 h-112 w-md rounded-full bg-cyan-400/20 blur-3xl" />
      <div class="absolute -bottom-40 -right-40 h-112 w-md rounded-full bg-blue-400/20 blur-3xl" />
    </div>

    <!-- 顶部霓虹光带 -->
    <div
      class="pointer-events-none absolute left-1/2 top-0 h-0.5 w-184 max-w-full -translate-x-1/2 rounded-full bg-linear-to-r from-transparent via-cyan-400/80 to-transparent"
    />

    <div class="relative w-full max-w-md">
      <div class="mb-8 flex flex-col items-center gap-3">
        <div
          class="relative flex h-14 w-14 items-center justify-center rounded-2xl bg-linear-to-br from-cyan-500 to-blue-600 text-white shadow-[0_8px_30px_-6px_rgba(6,182,212,0.5)]"
        >
          <ShieldCheck class="h-7 w-7" />
          <div class="absolute inset-0 rounded-2xl ring-1 ring-cyan-400/50" />
        </div>
        <div class="text-center">
          <h1 class="text-2xl font-bold tracking-wide text-slate-900">q-iam</h1>
          <p class="mt-1 text-sm tracking-[0.2em] text-cyan-600/70">权限管理控制台</p>
        </div>
      </div>

      <div
        class="relative overflow-hidden rounded-2xl border border-slate-200/80 bg-white/70 p-6 shadow-[0_8px_40px_-12px_rgba(15,23,42,0.15)] backdrop-blur-xl"
      >
        <!-- 卡片顶部霓虹光条 -->
        <div
          class="absolute inset-x-0 top-0 mx-auto h-0.5 w-3/5 rounded-full bg-linear-to-r from-transparent via-cyan-400/80 to-transparent"
        />

        <div class="flex flex-col items-center gap-4 py-4 text-center">
          <div
            class="flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/10 text-amber-500"
          >
            <ShieldX class="h-6 w-6" />
          </div>
          <div>
            <h2 class="text-base font-semibold text-slate-900">无控制台访问权限</h2>
            <p class="mt-2 text-sm leading-relaxed text-slate-500">
              当前账号仅用于 OAuth2 授权登录，
              <br />
              无法进入管理控制台。
            </p>
          </div>
        </div>

        <div class="mt-2 space-y-2.5">
          <button
            class="flex h-10 w-full items-center justify-center gap-2 rounded-md border border-cyan-500/30 bg-cyan-500/5 text-sm text-cyan-600 transition-all hover:border-cyan-500/50 hover:bg-cyan-500/10"
            @click="router.push('/auth')"
          >
            <ArrowLeft class="h-4 w-4" />
            返回登录
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
