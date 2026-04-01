import type { Account } from '@/features/accounts/types'
import {
  deriveQuotaDisplayStatus,
  formatRelativeReset,
  getNearestFutureResetAt,
  getOverviewQuotaMetrics,
  hasValidReset
} from '@/features/accounts/lib/account-quota'
import { normalizeProviderID, providerMeta } from '@/features/accounts/lib/account'

export interface AccountPresentation {
  displayName: string
  providerID: string
  providerLabel: string
  canSync: boolean
  disabledReason: string
  quotaStatus: ReturnType<typeof deriveQuotaDisplayStatus>
  metrics: ReturnType<typeof getOverviewQuotaMetrics>
  metricsWithReset: ReturnType<typeof getOverviewQuotaMetrics>
}

const inferDisabledReason = (account: Account): 'exhausted' | 'banned' => {
  const quotaStatus = (account.quota?.status || '').toLowerCase()
  const lastError = (account.lastError || '').toLowerCase()

  if (account.banned || (account.bannedReason || '').trim().length > 0) {
    return 'banned'
  }

  if (quotaStatus === 'exhausted' || /exhaust|usage limit|quota exceeded|insufficient[_\s-]?quota/.test(lastError)) {
    return 'exhausted'
  }

  return 'exhausted'
}

export const getAccountDisabledReason = (account: Account): string => {
  if (account.enabled) {
    return ''
  }

  if (inferDisabledReason(account) === 'banned') {
    return account.bannedReason || 'Banned'
  }

  const resetAt = getNearestFutureResetAt(account.quota, account.cooldownUntil)
  const relative = formatRelativeReset(resetAt)
  if (relative) {
    return `Exhausted, Resets ${relative}`
  }

  return 'Exhausted'
}

export const presentAccount = (account: Account): AccountPresentation => {
  const providerID = normalizeProviderID(account.provider)
  const metrics = getOverviewQuotaMetrics(account.quota)

  return {
    displayName: account.email || account.id,
    providerID,
    providerLabel: providerMeta(providerID).label,
    canSync: providerID === 'codex',
    disabledReason: getAccountDisabledReason(account),
    quotaStatus: deriveQuotaDisplayStatus(account.quota),
    metrics,
    metricsWithReset: metrics.filter((metric) => hasValidReset(metric.resetAt))
  }
}
