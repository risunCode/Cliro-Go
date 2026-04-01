import type { LogEntry } from '@/app/types'
import type { Account } from '@/features/accounts/types'

export type UsageProvider = 'codex' | 'kiro'

export interface ParsedRequestLog {
  timestamp: number
  provider: UsageProvider
  model: string
  account: string
  promptTokens: number
  completionTokens: number
  totalTokens: number
}

const ACTIVE_WINDOW_MS = 5000

const parseLogMessage = (message: string): Record<string, string> => {
  const out: Record<string, string> = {}
  const matcher = /(\w+)=((?:"[^"]*")|\S+)/g
  let match: RegExpExecArray | null = null
  while ((match = matcher.exec(message)) !== null) {
    const key = match[1]
    const rawValue = match[2]
    out[key] = rawValue.startsWith('"') && rawValue.endsWith('"') ? rawValue.slice(1, -1) : rawValue
  }
  return out
}

export const normalizeProvider = (value: string): UsageProvider | '' => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'kiro') {
    return 'kiro'
  }
  if (normalized === 'codex' || normalized === 'chatgpt') {
    return 'codex'
  }
  return ''
}

export const toRequestLog = (entry: LogEntry): ParsedRequestLog | null => {
  if (entry.scope !== 'proxy' || !entry.message.includes('phase="provider_completed"')) {
    return null
  }
  const fields = parseLogMessage(entry.message)
  const provider = normalizeProvider(fields.provider || '')
  if (!provider) {
    return null
  }
  return {
    timestamp: Number(entry.timestamp || 0),
    provider,
    model: (fields.model || '-').trim() || '-',
    account: (fields.account || '-').trim() || '-',
    promptTokens: Number(fields.prompt_tokens || 0),
    completionTokens: Number(fields.completion_tokens || 0),
    totalTokens: Number(fields.total_tokens || 0)
  }
}

export const getRequestLogs = (logs: LogEntry[]): ParsedRequestLog[] => logs.map(toRequestLog).filter((item): item is ParsedRequestLog => item !== null)

export const getRecentRequests = (requestLogs: ParsedRequestLog[], limit = 10): ParsedRequestLog[] => [...requestLogs].reverse().slice(0, limit)

export const getLastActiveAt = (requestLogs: ParsedRequestLog[], provider: UsageProvider): number => requestLogs.find((item) => item.provider === provider)?.timestamp || 0

export const isProviderActive = (proxyOnline: boolean, lastActiveAt: number, now: number): boolean => proxyOnline && now - lastActiveAt < ACTIVE_WINDOW_MS

export const getEnabledAccountCount = (accounts: Account[], provider: UsageProvider): number => accounts.filter((account) => account.enabled && normalizeProvider(account.provider || '') === provider).length

export const getProviderRequestCount = (requestLogs: ParsedRequestLog[], provider: UsageProvider): number => requestLogs.filter((item) => item.provider === provider).length

export const formatRelativeTime = (timestamp: number, now: number): string => {
  if (!timestamp) {
    return '-'
  }
  const deltaMs = now - timestamp
  const seconds = Math.max(Math.floor(deltaMs / 1000), 0)
  if (seconds < 5) {
    return 'now'
  }
  if (seconds < 60) {
    return `${seconds}s ago`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ago`
  }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours}h ago`
  }
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export const providerLabel = (provider: UsageProvider): string => (provider === 'kiro' ? 'Kiro' : 'Codex')
