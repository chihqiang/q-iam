/// <reference types="vite/client" />

// 自定义环境变量类型声明（需以 VITE_ 开头才会被 Vite 暴露到 import.meta.env）
interface ImportMetaEnv {
  /** 认证令牌 cookie 的子域共享域名（如 ".example.com"），留空则仅当前主机 */
  readonly VITE_AUTH_DOMAIN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
