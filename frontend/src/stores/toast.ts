import { writable } from 'svelte/store'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

interface ToastItem {
  id: number
  type: ToastType
  title: string
  message: string
  duration: number
}

const createToastStore = () => {
  const { subscribe, update } = writable<ToastItem[]>([])
  let toastId = 0
  const maxVisibleToasts = 2
  const defaultDuration = 2000

  const push = (
    type: ToastType,
    title: string,
    message: string,
    duration = defaultDuration
  ): number => {
    toastId += 1
    const id = toastId
    const toast: ToastItem = { id, type, title, message, duration }

    update((current) => [...current, toast].slice(-maxVisibleToasts))

    window.setTimeout(() => {
      remove(id)
    }, duration)

    return id
  }

  const remove = (id: number): void => {
    update((current) => current.filter((toast) => toast.id !== id))
  }

  return {
    subscribe,
    push,
    remove
  }
}

export const toastStore = createToastStore()
