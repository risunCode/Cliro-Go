import { formatNumber, formatUnixSeconds, nowUnixSeconds as getCurrentUnixSeconds } from '@/utils/formatters'
import type { Account } from '@/services/wails-api'

type QuotaBucket = NonNullable<NonNullable<Account['quota']>['buckets']>[number]
type QuotaDisplayStatus = 'ok' | 'low' | 'exhausted' | 'empty' | 'unknown'
type QuotaTone = 'success' | 'warning' | 'error' | 'neutral'

const preferredOverviewBuckets = ['session', 'weekly']

export const nowUnixSeconds = (): number => {
  return getCurrentUnixSeconds()
}

export const getQuotaTone = (status: string | undefined): QuotaTone => {
  const normalized = (status ?? '').toLowerCase().trim()
  if (normalized === 'healthy' || normalized === 'ok' || normalized === 'ready' || normalized === 'success') {
    return 'success'
  }
  if (normalized === 'low' || normalized === 'warning') {
    return 'warning'
  }
  if (normalized === 'exhausted' || normalized === 'error' || normalized === 'empty') {
    return 'error'
  }
  return 'neutral'
}

export const metricPercent = (bucket: QuotaBucket): number => {
  const remaining = Number(bucket.remaining ?? 0)
  const used = Number(bucket.used ?? 0)
  const total = Number(bucket.total ?? 0)
  if (Number.isFinite(total) && total > 0) {
    if (Number.isFinite(remaining)) {
      return Math.max(0, Math.min(100, (remaining / total) * 100))
    }
    if (Number.isFinite(used)) {
      return Math.max(0, Math.min(100, ((total - used) / total) * 100))
    }
  }

  if (typeof bucket.percent === 'number' && Number.isFinite(bucket.percent)) {
    return Math.max(0, Math.min(100, bucket.percent))
  }

  return 0
}

export const getPercentColor = (percent: number): 'success' | 'warning' | 'danger' => {
  if (percent >= 70) {
    return 'success'
  }
  if (percent >= 30) {
    return 'warning'
  }
  return 'danger'
}

export const formatMetricValue = (bucket: QuotaBucket): string => {
  const used = Number(bucket.used ?? 0)
  const total = Number(bucket.total ?? 0)
  const safeUsed = Number.isFinite(used) ? used : 0
  const safeTotal = Number.isFinite(total) ? total : 0

  if (safeTotal <= 0) {
    return `${formatNumber(safeUsed)} / -`
  }

  return `${formatNumber(safeUsed)} / ${formatNumber(safeTotal)}`
}

export const formatRelativeReset = (resetAt?: number): string => {
  if (typeof resetAt !== 'number' || !Number.isFinite(resetAt)) {
    return ''
  }

  const remaining = Math.max(0, Math.floor(resetAt - nowUnixSeconds()))
  if (remaining <= 0) {
    return 'Expired'
  }

  const days = Math.floor(remaining / 86400)
  const hours = Math.floor((remaining % 86400) / 3600)
  const minutes = Math.floor((remaining % 3600) / 60)
  if (days > 0) {
    return `in ${days}d ${hours}h`
  }
  if (hours > 0) {
    return `in ${hours}h ${minutes}m`
  }
  return `in ${Math.max(1, minutes)}m`
}

export const hasValidReset = (resetAt?: number): boolean => {
  return typeof resetAt === 'number' && Number.isFinite(resetAt) && resetAt > 0
}

export const getNearestFutureResetAt = (quota: Account['quota'], fallbackResetAt?: number): number | undefined => {
  const now = nowUnixSeconds()
  let nearest = Number.POSITIVE_INFINITY

  if (Array.isArray(quota?.buckets)) {
    for (const bucket of quota.buckets) {
      const resetAt = Number(bucket.resetAt ?? 0)
      if (Number.isFinite(resetAt) && resetAt > now && resetAt < nearest) {
        nearest = resetAt
      }
    }
  }

  const fallback = Number(fallbackResetAt ?? 0)
  if (Number.isFinite(fallback) && fallback > now && fallback < nearest) {
    nearest = fallback
  }

  return Number.isFinite(nearest) ? nearest : undefined
}

export const getOverviewQuotaMetrics = (quota: Account['quota']): QuotaBucket[] => {
  if (!quota?.buckets || !Array.isArray(quota.buckets)) {
    return []
  }

  const selected: QuotaBucket[] = []
  const seen = new Set<string>()

  preferredOverviewBuckets.forEach((target) => {
    const match = quota.buckets?.find((bucket) => bucket.name?.toLowerCase() === target)
    if (match && !seen.has(match.name)) {
      selected.push(match)
      seen.add(match.name)
    }
  })

  for (const bucket of quota.buckets) {
    if (selected.length >= 2) {
      break
    }
    if (!seen.has(bucket.name)) {
      selected.push(bucket)
      seen.add(bucket.name)
    }
  }

  return selected.slice(0, 2)
}

export const formatBucketLabel = (name?: string): string => {
  if (!name) {
    return 'Quota'
  }

  return name
    .replace(/[_-]+/g, ' ')
    .trim()
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

export const formatQuotaDateTime = (unixSeconds?: number): string => {
  return formatUnixSeconds(unixSeconds)
}

export const quotaStatusLabel = (status?: string): string => {
  if (!status) {
    return 'Unknown'
  }

  if (status === 'ok') {
    return 'Ok'
  }

  if (status === 'exhausted') {
    return 'Exhausted'
  }

  if (status === 'empty') {
    return 'Empty'
  }

  return status.charAt(0).toUpperCase() + status.slice(1)
}

const bucketLooksExhausted = (bucket: QuotaBucket): boolean => {
  const status = (bucket.status || '').toLowerCase().trim()
  if (status === 'exhausted' || status === 'empty' || status === 'quota_exceeded' || status === 'insufficient_quota') {
    return true
  }

  const total = Number(bucket.total ?? 0)
  const used = Number(bucket.used ?? 0)
  const remaining = Number(bucket.remaining ?? 0)
  const percent = Number(bucket.percent ?? 0)
  const hasReset = hasValidReset(bucket.resetAt)

  if (total > 0) {
    if (remaining <= 0) {
      return true
    }
    if (used >= total && used > 0) {
      return true
    }
  }

  if (total <= 0 && remaining <= 0 && hasReset && percent <= 0) {
    return true
  }

  return false
}

const isQuotaExhausted = (quota: Account['quota']): boolean => {
  if (!quota) {
    return false
  }

  const status = (quota.status || '').toLowerCase().trim()
  if (status === 'exhausted' || status === 'empty') {
    return true
  }

  if (!Array.isArray(quota.buckets) || quota.buckets.length === 0) {
    return false
  }

  return quota.buckets.some((bucket) => bucketLooksExhausted(bucket))
}

export const deriveQuotaDisplayStatus = (quota: Account['quota']): QuotaDisplayStatus => {
  if (!quota) {
    return 'unknown'
  }

  if (isQuotaExhausted(quota)) {
    return 'exhausted'
  }

  const status = (quota.status || '').toLowerCase().trim()
  if (status === 'ok' || status === 'healthy' || status === 'ready' || status === 'success') {
    return 'ok'
  }
  if (status === 'low' || status === 'warning') {
    return 'low'
  }
  if (status === 'empty') {
    return 'empty'
  }

  if (!Array.isArray(quota.buckets) || quota.buckets.length === 0) {
    return status ? 'unknown' : 'empty'
  }

  return 'unknown'
}
