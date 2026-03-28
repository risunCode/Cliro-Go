export const APP_TABS = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'accounts', label: 'Accounts' },
  { id: 'api-router', label: 'API Router' },
  { id: 'system-logs', label: 'System Logs' },
  { id: 'settings', label: 'Settings' }
] as const

export type AppTabId = (typeof APP_TABS)[number]['id']
