import type { Account } from '@/features/accounts/types'
import {
  deriveQuotaDisplayStatus,
  formatMetricValue,
  formatRelativeReset,
  formatResetHint,
  getNearestFutureResetAt,
  getOverviewQuotaMetrics,
  hasValidReset
} from '@/features/accounts/lib/account-quota'
import { normalizeProviderID, providerMeta } from '@/features/accounts/lib/account'

export type QuotaDisplayMode = 'metrics' | 'status'

export interface AccountPresentation {
  displayName: string
  providerID: string
  providerLabel: string
  canSync: boolean
  disabledKind: DisabledReason | null
  disabledReason: string
  quotaStatus: ReturnType<typeof deriveQuotaDisplayStatus>
  quotaDisplayMode: QuotaDisplayMode
  disabledHint: QuotaHint | null
  quotaHint: QuotaHint | null
  metrics: ReturnType<typeof getOverviewQuotaMetrics>
  metricsWithReset: ReturnType<typeof getOverviewQuotaMetrics>
}

export interface QuotaHint {
  text: string
  tone: 'neutral' | 'warning' | 'error'
  metaPillText?: string
  metaPillTone?: 'neutral' | 'warning' | 'error'
  resetText?: string
  detail?: string
}

const compactHint = (raw: string): string => {
  const normalized = raw.replace(/\s+/g, ' ').trim().replace(/[.。]+$/, '')
  if (!normalized) {
    return ''
  }
  if (normalized.length <= 48) {
    return normalized
  }
  return `${normalized.slice(0, 45).trimEnd()}...`
}

const firstNonEmptyMessage = (...values: Array<string | undefined>): string => {
  for (const value of values) {
    const normalized = compactHint(value || '')
    if (normalized) {
      return normalized
    }
  }
  return ''
}

const isQuotaCooldown = (account: Account, quotaStatus: ReturnType<typeof deriveQuotaDisplayStatus>): boolean => {
  if (account.healthState === 'cooldown_quota') {
    return true
  }
  if (quotaStatus !== 'exhausted' && quotaStatus !== 'empty') {
    return false
  }

  return Boolean(getNearestFutureResetAt(account.quota, account.cooldownUntil))
}

const isAuthReloginState = (account: Account): boolean => {
  const healthReason = (account.healthReason || '').trim().toLowerCase()
  const quotaSummary = (account.quota?.summary || '').trim().toLowerCase()
  return healthReason === 'need re-login' || quotaSummary === 'authentication required'
}

const buildQuotaHint = (account: Account, quotaStatus: ReturnType<typeof deriveQuotaDisplayStatus>): QuotaHint | null => {
  const messagePool = [account.quota?.error, account.lastError, account.healthReason, account.quota?.summary]

  if (isAuthReloginState(account)) {
    return {
      text: 'Need re-login',
      tone: 'error',
      detail: firstNonEmptyMessage(account.quota?.error, account.lastError) || undefined
    }
  }

  if (isQuotaCooldown(account, quotaStatus)) {
    const resetAt = getNearestFutureResetAt(account.quota, account.cooldownUntil)
    const resetHint = formatResetHint(resetAt)
    const fallbackDetail = firstNonEmptyMessage(account.quota?.summary, account.quota?.error, account.healthReason)

    return {
      text: quotaStatus === 'empty' ? 'Quota Empty' : 'Quota Exhausted',
      tone: 'warning',
      resetText: resetHint || undefined,
      detail: resetHint ? undefined : fallbackDetail || undefined
    }
  }

  if (quotaStatus === 'unknown') {
    const detail = firstNonEmptyMessage(...messagePool)
    return {
      text: 'Unknown',
      tone: 'warning',
      detail: detail || undefined
    }
  }

  if (quotaStatus === 'low') {
    const detail = firstNonEmptyMessage(account.quota?.summary, account.quota?.error)
    return detail
      ? {
          text: 'Quota Low',
          tone: 'warning',
          detail
        }
      : null
  }

  return null
}

const pickManualDisabledBucket = (account: Account) => {
  const overview = getOverviewQuotaMetrics(account.quota)
  const allBuckets = Array.isArray(account.quota?.buckets) ? account.quota.buckets : []
  const candidates = overview.length > 0 ? overview : allBuckets

  if (candidates.length === 0) {
    return null
  }

  const now = Math.floor(Date.now() / 1000)
  let picked = candidates[0]
  let bestReset = Number.POSITIVE_INFINITY

  for (const bucket of candidates) {
    const resetAt = Number(bucket.resetAt ?? 0)
    if (Number.isFinite(resetAt) && resetAt > now && resetAt < bestReset) {
      picked = bucket
      bestReset = resetAt
    }
  }

  return picked
}

const buildManualDisabledMetaPill = (account: Account): string => {
  const bucket = pickManualDisabledBucket(account)
  const nearestResetAt = getNearestFutureResetAt(account.quota, account.cooldownUntil)
  const targetReset = bucket?.resetAt || nearestResetAt

  const value = bucket ? formatMetricValue(bucket).replace(/\s+/g, '') : ''
  const relative = formatRelativeReset(targetReset)

  let resetPart = ''
  if (relative === 'Expired') {
    resetPart = 'Quota reset expired'
  } else if (relative) {
    resetPart = `Quota reset ${relative}`
  }

  if (value && resetPart) {
    return `${value} • ${resetPart}`
  }

  return value || resetPart
}

const shouldUseStatusMode = (
  account: Account,
  quotaStatus: ReturnType<typeof deriveQuotaDisplayStatus>,
  quotaHint: QuotaHint | null
): boolean => {
  if (!quotaHint) {
    return false
  }

  if (isQuotaCooldown(account, quotaStatus)) {
    return true
  }

  if (quotaHint.text === 'Need re-login') {
    return true
  }

  if (quotaStatus === 'unknown') {
    return true
  }

  return false
}

type DisabledReason = 'manual' | 'banned' | 'cooldown'

const buildDisabledHint = (account: Account): QuotaHint | null => {
  if (account.enabled) {
    return null
  }

  const reason = inferDisabledReason(account)
  if (reason === 'banned') {
    const detailSource = firstNonEmptyMessage(account.bannedReason, account.quota?.summary, account.lastError)
    const detailLower = detailSource.toLowerCase()
    const text = detailLower.includes('deactivated') ? 'Account Deactivated' : 'Account Banned'
    return {
      text,
      tone: 'error'
    }
  }

  if (reason === 'cooldown') {
    const resetAt = getNearestFutureResetAt(account.quota, account.cooldownUntil)
    return {
      text: 'Quota Exhausted',
      tone: 'warning',
      resetText: formatResetHint(resetAt) || undefined
    }
  }

  const metaPillText = buildManualDisabledMetaPill(account)

  return {
    text: 'Disabled by User',
    tone: 'neutral',
    metaPillText: metaPillText || undefined,
    metaPillTone: 'warning'
  }
}

const inferDisabledReason = (account: Account): DisabledReason => {
  if (account.banned || (account.bannedReason || '').trim().length > 0) {
    return 'banned'
  }

  const quotaStatus = (account.quota?.status || '').trim().toLowerCase()
  if (quotaStatus === 'deactivated' || quotaStatus === 'banned' || quotaStatus === 'suspended' || quotaStatus === 'disabled') {
    return 'banned'
  }

  if (isQuotaCooldown(account, deriveQuotaDisplayStatus(account.quota))) {
    return 'cooldown'
  }

  return 'manual'
}

export const getAccountDisabledReason = (account: Account): string => {
  if (account.enabled) {
    return ''
  }

  const reason = inferDisabledReason(account)
  if (reason === 'banned') {
    return account.bannedReason || account.quota?.summary || 'Banned'
  }

  if (reason === 'cooldown') {
    const resetAt = getNearestFutureResetAt(account.quota, account.cooldownUntil)
    const resetHint = formatResetHint(resetAt)
    if (resetHint) {
      return `Quota exhausted (${resetHint})`
    }
    return 'Quota exhausted'
  }

  return 'Disabled by user'
}

export const presentAccount = (account: Account): AccountPresentation => {
  const providerID = normalizeProviderID(account.provider)
  const quotaStatus = deriveQuotaDisplayStatus(account.quota)
  const quotaHint = buildQuotaHint(account, quotaStatus)
  const disabledHint = buildDisabledHint(account)
  const disabledKind = account.enabled ? null : inferDisabledReason(account)
  const metrics = getOverviewQuotaMetrics(account.quota)

  return {
    displayName: account.email || account.id,
    providerID,
    providerLabel: providerMeta(providerID).label,
    canSync: providerID === 'codex',
    disabledKind,
    disabledReason: getAccountDisabledReason(account),
    quotaStatus,
    quotaDisplayMode: shouldUseStatusMode(account, quotaStatus, quotaHint) ? 'status' : 'metrics',
    disabledHint,
    quotaHint,
    metrics,
    metricsWithReset: metrics.filter((metric) => hasValidReset(metric.resetAt))
  }
}
