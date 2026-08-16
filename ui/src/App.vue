<script setup lang="ts">
import CookieConsentBanner from '@/components/ui/CookieConsentBanner.vue'
</script>

<template>
  <router-view />
  <!-- 全局 Cookie 同意横幅：未做出选择时在底部弹出 -->
  <CookieConsentBanner />
</template>
