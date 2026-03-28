import { appService, type Account, type AppState, type LogEntry, type ProxyStatus } from '@/services/wails-api'

export interface CoreSnapshot {
  state: AppState
  accounts: Account[]
  proxyStatus: ProxyStatus
}

export const fetchCoreSnapshot = async (): Promise<CoreSnapshot> => {
  const [state, accounts, proxyStatus] = await Promise.all([appService.getState(), appService.getAccounts(), appService.getProxyStatus()])
  return { state, accounts, proxyStatus }
}

export const fetchLogsSnapshot = (limit = 400): Promise<LogEntry[]> => {
  return appService.getLogs(limit)
}
