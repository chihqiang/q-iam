import { defineStore } from 'pinia'

export interface Toast {
  id: number
  type: 'success' | 'error' | 'info'
  message: string
}

let seq = 0

export const useToastStore = defineStore('toast', {
  state: () => ({
    toasts: [] as Toast[],
  }),
  actions: {
    push(type: Toast['type'], message: string) {
      const id = ++seq
      this.toasts.push({ id, type, message })
      setTimeout(() => this.remove(id), 3200)
    },
    success(message: string) {
      this.push('success', message)
    },
    error(message: string) {
      this.push('error', message)
    },
    info(message: string) {
      this.push('info', message)
    },
    remove(id: number) {
      this.toasts = this.toasts.filter((t) => t.id !== id)
    },
  },
})
