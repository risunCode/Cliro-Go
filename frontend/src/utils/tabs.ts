export const APP_TABS = [
  { id: 'dashboard', label: 'Dashboard', icon: 'dashboard' },
  { id: 'accounts', label: 'Accounts', icon: 'accounts' },
  { id: 'api-router', label: 'API Router', icon: 'api-router' },
  { id: 'usage', label: 'Usage', icon: 'usage' },
  { id: 'system-logs', label: 'System Logs', icon: 'system-logs' },
  { id: 'settings', label: 'Settings', icon: 'settings' }
] as const

export type AppTabId = (typeof APP_TABS)[number]['id']
