<script lang="ts">
  import { Clock, Zap } from 'lucide-svelte'
  import AccountActions from '@/components/accounts/AccountActions.svelte'
  import ProviderAvatar from '@/components/accounts/ProviderAvatar.svelte'
  import type { Account } from '@/services/wails-api'
  import {
    formatBucketLabel,
    formatMetricValue,
    formatRelativeReset,
    getNearestFutureResetAt,
    getOverviewQuotaMetrics,
    getPercentColor,
    hasValidReset,
    metricPercent
  } from '@/utils/account-quota'
  import { normalizeProviderID, providerMeta } from '@/utils/account'

export let account: Account
export let selected = false
export let busy = false
export let refreshing = false
export let confirmingDelete = false
export let deleteInProgress = false

  export let onToggleSelection: (accountID: string) => void
  export let onToggleAccount: (accountID: string, enabled: boolean) => Promise<void>
  export let onStartSync: (accountID: string) => void
  export let onInfo: (accountID: string) => void
  export let onRefreshWithQuota: (accountID: string) => Promise<void>
  export let onExport: (accountID: string) => Promise<void>
  export let onStartRemove: (accountID: string) => void
  export let onConfirmRemove: (accountID: string) => Promise<void>
  export let onCancelRemove: () => void

  const inferDisabledReason = (value: Account): 'exhausted' | 'banned' => {
    const quotaStatus = (value.quota?.status || '').toLowerCase()
    const lastError = (value.lastError || '').toLowerCase()

    if (value.banned || (value.bannedReason || '').trim().length > 0) {
      return 'banned'
    }

    if (quotaStatus === 'exhausted' || /exhaust|usage limit|quota exceeded|insufficient[_\s-]?quota/.test(lastError)) {
      return 'exhausted'
    }

    return 'exhausted'
  }

  const disabledReasonText = (value: Account): string => {
    const reason = inferDisabledReason(value)
    if (reason === 'banned') {
      return value.bannedReason || 'Banned'
    }

    const resetAt = getNearestFutureResetAt(value.quota, value.cooldownUntil)
    const relative = formatRelativeReset(resetAt)
    if (relative) {
      return `Exhausted, Resets ${relative}`
    }

    return 'Exhausted'
  }

  $: providerID = normalizeProviderID(account.provider)
  $: meta = providerMeta(providerID)
  $: canSync = providerID === 'codex'
  $: metrics = getOverviewQuotaMetrics(account.quota)
  $: displayName = account.email || account.id
  $: disabledReason = account.enabled ? '' : disabledReasonText(account)
</script>

<article class={`account-card ${selected ? 'is-selected' : ''} ${account.enabled ? '' : 'is-disabled'}`}>
  <div class="card-header">
    <div class="provider-info">
      <ProviderAvatar provider={account.provider} variant="icon" />
      <div class="provider-details">
        <p class="provider-label">{meta.label}</p>
        <p class="account-email">{displayName}</p>
      </div>
    </div>

    <label class="select-toggle">
      <input type="checkbox" checked={selected} on:change={() => onToggleSelection(account.id)} aria-label="Select account" />
      <span>Select</span>
    </label>
  </div>

  {#if (account.quota?.status === 'error' || account.quota?.status === 'warning' || account.quota?.status === 'empty') &&
  (account.quota?.error || account.quota?.summary)}
    <div class={`alert alert-${account.quota?.status || 'empty'}`}>
      {account.quota?.error || account.quota?.summary}
    </div>
  {/if}

  <div class="metrics">
    {#if !account.enabled}
      <div class="metric-bar metric-disabled-note">Account disabled (Reason: {disabledReason}).</div>
    {:else}
      {#each metrics as bucket}
        {@const percent = metricPercent(bucket)}
        {@const tone = getPercentColor(percent)}
        <div class={`metric-bar metric-${tone}`}>
          <div class="metric-top">
            <div class="metric-left">
              <Zap size={11} class="metric-icon" />
              <span class="metric-name">{formatBucketLabel(bucket.name)}</span>
            </div>
            <div class="metric-right">
              <span class="metric-value">{formatMetricValue(bucket)}</span>
              <span class="metric-percent">{percent.toFixed(0)}%</span>
            </div>
          </div>
          {#if hasValidReset(bucket.resetAt)}
            <div class="metric-reset">
              <Clock size={11} class="metric-icon" />
              <span class="metric-time">{formatRelativeReset(bucket.resetAt)}</span>
            </div>
          {/if}
        </div>
      {:else}
        <div class="metric-bar">No quota buckets reported.</div>
      {/each}
    {/if}
  </div>

  <AccountActions
    mode="card"
    accountID={account.id}
    enabled={account.enabled}
    {canSync}
    {busy}
    {refreshing}
    {confirmingDelete}
    {deleteInProgress}
    {onToggleAccount}
    {onStartSync}
    {onInfo}
    onRefreshWithQuota={onRefreshWithQuota}
    {onExport}
    onStartRemove={onStartRemove}
    onConfirmRemove={onConfirmRemove}
    onCancelRemove={onCancelRemove}
  />
</article>
