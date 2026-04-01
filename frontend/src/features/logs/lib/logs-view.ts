import type { LogEntry } from '@/app/types'
import { formatDateTime } from '@/shared/lib/formatters'

export type LevelFilter = 'all' | 'info' | 'warning' | 'error' | 'debug' | 'other'
export type BadgeTone = 'neutral' | 'success' | 'error' | 'info' | 'warning'
export type SortField = 'timestamp' | 'level' | 'scope' | 'account' | 'detail'
export type SortDirection = 'asc' | 'desc'

export interface LogFilters {
  query: string
  level: LevelFilter
  scope: string
  sortField: SortField
  sortDirection: SortDirection
}

export interface LogRowView {
  rowID: string
  entry: LogEntry
  normalizedLevel: string
  normalizedScope: string
  accountLabel: string
  detailText: string
  searchableText: string
  copyText: string
  expandable: boolean
}

interface InternalField {
  key: string
  lookupKey: string
  value: string
  compact: string
}

export const LOG_TABLE_MAX_ENTRIES = 500
export const SCOPE_QUICK_FILTER_LIMIT = 6

const LEVEL_SORT_WEIGHT: Record<Exclude<LevelFilter, 'all'>, number> = {
  error: 5,
  warning: 4,
  info: 3,
  debug: 2,
  other: 1
}

const SCOPE_PRIORITY = ['accounts', 'auth', 'proxy', 'cloudflared', 'gateway', 'quota', 'router', 'provider', 'clisync', 'app', 'system']
const ACCOUNT_FIELD_KEYS = ['account', 'email', 'accountid', 'user']
const HIDDEN_DETAIL_FIELD_KEYS = new Set(['requestid', 'account', 'accountid', 'email', 'user', 'provider'])
const DETAIL_FIELD_PRIORITY = [
  'reason',
  'route',
  'status',
  'model',
  'resolvedmodel',
  'requestedmodel',
  'stream',
  'prompttokens',
  'completiontokens',
  'totaltokens',
  'failurecount',
  'cooldownseconds',
  'cooldownuntil',
  'thinkingsource',
  'thinkingrequested',
  'thinkingemitted',
  'error'
]

const logTimeFormatter = new Intl.DateTimeFormat(undefined, {
  hour: '2-digit',
  minute: '2-digit',
  hour12: false
})

export const normalizeLevel = (entry: LogEntry): string => {
  return (entry.level || 'info').trim().toLowerCase()
}

export const normalizeScope = (entry: LogEntry): string => {
  return (entry.scope || 'system').trim().toLowerCase()
}

export const formatLogTime = (value: number | undefined): string => {
  if (!value) {
    return '--:--'
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '--:--'
  }

  return logTimeFormatter.format(date)
}

export const levelKey = (entry: LogEntry): Exclude<LevelFilter, 'all'> => {
  const level = normalizeLevel(entry)
  if (level === 'warn' || level === 'warning') {
    return 'warning'
  }
  if (level === 'error') {
    return 'error'
  }
  if (level === 'debug') {
    return 'debug'
  }
  if (level === 'info') {
    return 'info'
  }
  return 'other'
}

export const levelLabel = (entry: LogEntry): string => {
  const key = levelKey(entry)
  if (key === 'warning') {
    return 'WARN'
  }
  return key.toUpperCase()
}

export const levelTone = (key: Exclude<LevelFilter, 'all'>): BadgeTone => {
  if (key === 'error') {
    return 'error'
  }
  if (key === 'warning') {
    return 'warning'
  }
  if (key === 'info') {
    return 'info'
  }
  return 'neutral'
}

export const createLogRows = (logs: LogEntry[]): LogRowView[] => {
  const occurrences = new Map<string, number>()

  return logs.map((entry) => {
    const normalizedScope = normalizeScope(entry)
    const normalizedLevel = normalizeLevel(entry)
    const requestId = normalizeRequestID(entry)
    const plainMessage = getPlainMessage(entry)
    const fields = buildFieldViews(entry)
    const accountLabel = extractAccountLabel(fields)
    const detailText = buildDetailText(entry, plainMessage, fields)
    const fingerprint = `${entry.timestamp}:${normalizedScope}:${normalizedLevel}:${accountLabel}:${detailText}:${requestId}`
    const nextOccurrence = (occurrences.get(fingerprint) || 0) + 1
    occurrences.set(fingerprint, nextOccurrence)

    return {
      rowID: `${fingerprint}:${nextOccurrence}`,
      entry,
      normalizedLevel,
      normalizedScope,
      accountLabel,
      detailText,
      searchableText: buildSearchableText(entry, normalizedScope, normalizedLevel, accountLabel, detailText, requestId, fields),
      copyText: buildRowCopyText(entry, normalizedScope, accountLabel, detailText, requestId, fields),
      expandable: isExpandableMessage(detailText)
    }
  })
}

export const getScopes = (logs: LogEntry[]): string[] => {
  return Array.from(new Set(logs.map((entry) => normalizeScope(entry)).filter((value) => value.length > 0))).sort((a, b) => a.localeCompare(b))
}

export const getScopeQuickFilters = (scopes: string[]): string[] => {
  if (scopes.length <= SCOPE_QUICK_FILTER_LIMIT) {
    return scopes
  }

  const prioritized = SCOPE_PRIORITY.filter((scope) => scopes.includes(scope))
  const leftovers = scopes.filter((scope) => !prioritized.includes(scope))
  return [...prioritized, ...leftovers].slice(0, SCOPE_QUICK_FILTER_LIMIT)
}

export const getLevelCounts = (logs: LogEntry[]): Record<Exclude<LevelFilter, 'all'>, number> => {
  return logs.reduce(
    (accumulator, entry) => {
      accumulator[levelKey(entry)] += 1
      return accumulator
    },
    {
      info: 0,
      warning: 0,
      error: 0,
      debug: 0,
      other: 0
    }
  )
}

export const filterLogRows = (rows: LogRowView[], filters: LogFilters): LogRowView[] => {
  const normalizedQuery = filters.query.trim().toLowerCase()

  return rows.filter((row) => {
    const currentLevel = levelKey(row.entry)

    if (filters.level !== 'all' && currentLevel !== filters.level) {
      return false
    }

    if (filters.scope !== 'all' && row.normalizedScope !== filters.scope) {
      return false
    }

    if (normalizedQuery.length === 0) {
      return true
    }

    return row.searchableText.includes(normalizedQuery)
  })
}

export const sortLogRows = (rows: LogRowView[], sortField: SortField, sortDirection: SortDirection): LogRowView[] => {
  return [...rows]
    .map((row, index) => ({ row, index }))
    .sort((left, right) => {
      const leftEntry = left.row.entry
      const rightEntry = right.row.entry

      let comparison = 0
      if (sortField === 'timestamp') {
        comparison = compareNumbers(leftEntry.timestamp, rightEntry.timestamp)
      } else if (sortField === 'level') {
        comparison = compareNumbers(LEVEL_SORT_WEIGHT[levelKey(leftEntry)], LEVEL_SORT_WEIGHT[levelKey(rightEntry)])
      } else if (sortField === 'scope') {
        comparison = compareText(left.row.normalizedScope, right.row.normalizedScope)
      } else if (sortField === 'account') {
        comparison = compareText(left.row.accountLabel, right.row.accountLabel)
      } else {
        comparison = compareText(left.row.detailText, right.row.detailText)
      }

      if (comparison === 0) {
        comparison = compareNumbers(leftEntry.timestamp, rightEntry.timestamp)
      }

      if (comparison === 0) {
        comparison = compareNumbers(left.index, right.index)
      }

      return sortDirection === 'asc' ? comparison : -comparison
    })
    .map((item) => item.row)
}

export const getVisibleRows = (rows: LogRowView[]): LogRowView[] => {
  return rows.slice(0, LOG_TABLE_MAX_ENTRIES)
}

export const toTerminalLine = (entry: LogEntry): string => {
  const normalizedScope = normalizeScope(entry)
  const fields = buildFieldViews(entry)
  const accountLabel = extractAccountLabel(fields)
  const detailText = buildDetailText(entry, getPlainMessage(entry), fields)
  return `${formatDateTime(entry.timestamp)} | ${levelLabel(entry)} | ${normalizedScope} | ${accountLabel} | ${detailText}`
}

export const buildVisibleLines = (rows: LogRowView[]): string[] => {
  return rows.map(({ copyText }) => copyText)
}

export const buildExportPayload = (rows: LogRowView[]) => {
  return {
    exportedAt: new Date().toISOString(),
    count: rows.length,
    entries: rows.map(({ entry }) => entry)
  }
}

export const isExpandableMessage = (message: string): boolean => {
  return message.trim().length > 112
}

export const isMessageExpanded = (expandedIDs: string[], rowID: string): boolean => {
  return expandedIDs.includes(rowID)
}

export const toggleMessageExpansion = (expandedIDs: string[], rowID: string): string[] => {
  if (isMessageExpanded(expandedIDs, rowID)) {
    return expandedIDs.filter((id) => id !== rowID)
  }

  return [...expandedIDs, rowID]
}

export const pruneExpandedMessageIDs = (expandedIDs: string[], rows: LogRowView[]): string[] => {
  const visibleIDSet = new Set(rows.map((row) => row.rowID))
  return expandedIDs.filter((id) => visibleIDSet.has(id))
}

export const getNextSortState = (currentField: SortField, currentDirection: SortDirection, nextField: SortField): Pick<LogFilters, 'sortField' | 'sortDirection'> => {
  if (currentField === nextField) {
    return {
      sortField: currentField,
      sortDirection: currentDirection === 'asc' ? 'desc' : 'asc'
    }
  }

  return {
    sortField: nextField,
    sortDirection: nextField === 'timestamp' ? 'desc' : 'asc'
  }
}

export const getSortIndicator = (currentField: SortField, currentDirection: SortDirection, field: SortField): string => {
  if (currentField !== field) {
    return '↕'
  }

  return currentDirection === 'asc' ? '↑' : '↓'
}

export const isScopeQuickFilterVisible = (activeScope: string, scope: string): boolean => {
  if (!scope || activeScope === scope) {
    return true
  }

  return activeScope === 'all'
}

export const quickScopeFilterLabel = (scope: string): string => {
  return scope.charAt(0).toUpperCase() + scope.slice(1)
}

const normalizeRequestID = (entry: LogEntry): string => {
  return (entry.requestId || '').trim()
}

const getPlainMessage = (entry: LogEntry): string => {
  const message = (entry.message || '').trim()
  if (!message) {
    return ''
  }
  if (entry.event?.trim() && message.startsWith('event=')) {
    return ''
  }
  return message
}

const buildFieldViews = (entry: LogEntry): InternalField[] => {
  const fields = entry.fields
  if (!fields || typeof fields !== 'object') {
    return []
  }

  return Object.entries(fields)
    .filter(([key]) => key.trim().length > 0)
    .map(([key, value]) => {
      const formattedValue = formatFieldValue(value)
      return {
        key,
        lookupKey: normalizeLookupKey(key),
        value: formattedValue,
        compact: `${key}=${quoteFieldValue(formattedValue)}`
      }
    })
    .sort((left, right) => compareFieldOrder(left, right))
}

const extractAccountLabel = (fields: InternalField[]): string => {
  for (const key of ACCOUNT_FIELD_KEYS) {
    const match = fields.find((field) => field.lookupKey === key && field.value.trim().length > 0)
    if (match) {
      return match.value.trim()
    }
  }

  return '-'
}

const buildDetailText = (entry: LogEntry, plainMessage: string, fields: InternalField[]): string => {
  const detailFields = fields.filter((field) => !HIDDEN_DETAIL_FIELD_KEYS.has(field.lookupKey))
  const parts: string[] = []
  const event = entry.event?.trim() || ''

  if (event) {
    parts.push(event)
  }

  if (plainMessage) {
    parts.push(plainMessage)
  }

  if (detailFields.length > 0) {
    parts.push(detailFields.map((field) => field.compact).join(' '))
  }

  const detail = parts.join(' ').trim()
  return detail || '-'
}

const buildSearchableText = (
  entry: LogEntry,
  normalizedScope: string,
  normalizedLevel: string,
  accountLabel: string,
  detailText: string,
  requestId: string,
  fields: InternalField[]
): string => {
  return [
    formatDateTime(entry.timestamp),
    normalizedLevel,
    normalizedScope,
    accountLabel,
    detailText,
    requestId,
    fields.map((field) => `${field.key} ${field.value}`).join(' '),
    entry.message || '',
    entry.event || ''
  ]
    .join(' ')
    .toLowerCase()
}

const buildRowCopyText = (
  entry: LogEntry,
  normalizedScope: string,
  accountLabel: string,
  detailText: string,
  requestId: string,
  fields: InternalField[]
): string => {
  const parts = [
    `time=${formatDateTime(entry.timestamp)}`,
    `level=${levelLabel(entry)}`,
    `source=${normalizedScope}`,
    `account=${accountLabel}`,
    `detail=${detailText}`
  ]

  if (requestId) {
    parts.push(`request_id=${requestId}`)
  }

  for (const field of fields) {
    parts.push(field.compact)
  }

  return parts.join('\n')
}

const formatFieldValue = (value: unknown): string => {
  if (value === null || value === undefined) {
    return ''
  }
  if (typeof value === 'string') {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
    return String(value)
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const quoteFieldValue = (value: string): string => {
  if (!value) {
    return '""'
  }
  return /\s|[{}\[\]",]/.test(value) ? JSON.stringify(value) : value
}

const normalizeLookupKey = (key: string): string => {
  return key.trim().toLowerCase().replace(/[_\-\s]/g, '')
}

const compareFieldOrder = (left: InternalField, right: InternalField): number => {
  const leftPriority = DETAIL_FIELD_PRIORITY.indexOf(left.lookupKey)
  const rightPriority = DETAIL_FIELD_PRIORITY.indexOf(right.lookupKey)

  if (leftPriority !== -1 || rightPriority !== -1) {
    if (leftPriority === -1) {
      return 1
    }
    if (rightPriority === -1) {
      return -1
    }
    if (leftPriority !== rightPriority) {
      return leftPriority - rightPriority
    }
  }

  return left.key.localeCompare(right.key, undefined, { sensitivity: 'base' })
}

const compareText = (left: string, right: string): number => {
  return left.localeCompare(right, undefined, { sensitivity: 'base' })
}

const compareNumbers = (left: number, right: number): number => {
  if (left === right) {
    return 0
  }

  return left > right ? 1 : -1
}
