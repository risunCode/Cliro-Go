import { ClearLogs, GetLogs } from '@/shared/api/wails/client'
import type { LogEntry } from '@/app/types'

export const logsApi = {
  getLogs: (limit = 500): Promise<LogEntry[]> => GetLogs(limit),
  clearLogs: (): Promise<void> => ClearLogs()
}
