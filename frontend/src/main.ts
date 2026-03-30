import './global.css'
import App from '@/App.svelte'
import { getErrorMessage } from '@/services/error'
import { toastStore } from '@/stores/toast'

let lastGlobalErrorSignature = ''
let lastGlobalErrorAt = 0

const reportGlobalError = (title: string, error: unknown): void => {
  const message = getErrorMessage(error, 'Unexpected application error.')
  const signature = `${title}:${message}`
  const now = Date.now()

  if (signature === lastGlobalErrorSignature && now - lastGlobalErrorAt < 2000) {
    return
  }

  lastGlobalErrorSignature = signature
  lastGlobalErrorAt = now
  console.error(`[${title}]`, error)
  toastStore.push('error', title, message, 5000)
}

if (typeof window !== 'undefined') {
  window.addEventListener('error', (event) => {
    const error = event.error ?? event.message
    reportGlobalError('Unhandled Error', error)
  })

  window.addEventListener('unhandledrejection', (event) => {
    reportGlobalError('Unhandled Promise Rejection', event.reason)
  })
}

const app = new App({
  target: document.getElementById('app')
})

export default app
