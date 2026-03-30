import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { LogEntry } from '@/services/wails-api'

const appendLogEntryWithLimit = (entries: LogEntry[], entry: LogEntry, limit = 1000): LogEntry[] => {
  return [...entries, entry].slice(-limit)
}

export const subscribeToRingLogs = (
  getLogs: () => LogEntry[],
  setLogs: (entries: LogEntry[]) => void,
  limit = 1000
): (() => void) => {
  return EventsOn('log:entry', (payload: unknown) => {
    if (payload && typeof payload === 'object') {
      setLogs(appendLogEntryWithLimit(getLogs(), payload as LogEntry, limit))
    }
  })
}
