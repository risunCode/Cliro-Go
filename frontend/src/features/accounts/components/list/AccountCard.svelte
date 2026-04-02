<script lang="ts">
  import { Zap } from 'lucide-svelte'
  import AccountActions from './AccountActions.svelte'
  import ProviderAvatar from './ProviderAvatar.svelte'
  import type { Account } from '@/features/accounts/types'
  import {
    formatBucketLabel,
    formatMetricValue,
    formatResetHint,
    getPercentColor,
    hasValidReset,
    metricPercent
  } from '@/features/accounts/lib/account-quota'
  import { presentAccount } from '@/features/accounts/lib/presenter'

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

  $: presentation = presentAccount(account)
  $: canSync = presentation.canSync
  $: metrics = presentation.metrics
  $: quotaDisplayMode = presentation.quotaDisplayMode
  $: disabledHint = presentation.disabledHint
  $: quotaHint = presentation.quotaHint
</script>

<article class={`account-card ${selected ? 'is-selected' : ''} ${account.enabled ? '' : 'is-disabled'}`}>
  <div class="card-header">
    <div class="provider-info">
      <ProviderAvatar provider={account.provider} variant="icon" />
      <div class="provider-details">
        <p class="provider-label">{presentation.providerLabel}</p>
        <p class="account-email">{presentation.displayName}</p>
      </div>
    </div>

    <label class="select-toggle">
      <input type="checkbox" checked={selected} on:change={() => onToggleSelection(account.id)} aria-label="Select account" />
      <span>Select</span>
    </label>
  </div>

  {#if account.enabled && (account.quota?.status === 'error' || account.quota?.status === 'warning' || account.quota?.status === 'empty') &&
  (account.quota?.error || account.quota?.summary)}
    <div class={`alert alert-${account.quota?.status || 'empty'}`}>
      {account.quota?.error || account.quota?.summary}
    </div>
  {/if}

  <div class="metrics">
    {#if !account.enabled && disabledHint}
      <div class={`metric-bar metric-status metric-status-surface tone-${disabledHint.tone}`}>
        <div class="metric-status-head">
          <div class="metric-hint">
            <span class={`metric-hint-pill tone-${disabledHint.tone}`}>{disabledHint.text}</span>
          </div>
          {#if disabledHint.metaPillText}
            <span class={`metric-meta-pill tone-${disabledHint.metaPillTone || 'neutral'}`}>{disabledHint.metaPillText}</span>
          {:else if disabledHint.resetText}
            <span class="metric-status-reset">{disabledHint.resetText}</span>
          {/if}
        </div>
        {#if disabledHint.detail}
          <div class="metric-status-detail">{disabledHint.detail}</div>
        {/if}
      </div>
    {:else if quotaDisplayMode === 'status' && quotaHint}
      <div class={`metric-bar metric-status metric-status-surface tone-${quotaHint.tone}`}>
        <div class="metric-status-head">
          <div class="metric-hint">
            <span class={`metric-hint-pill tone-${quotaHint.tone}`}>{quotaHint.text}</span>
          </div>
          {#if quotaHint.resetText}
            <span class="metric-status-reset">{quotaHint.resetText}</span>
          {/if}
        </div>
        {#if quotaHint.detail}
          <div class="metric-status-detail">{quotaHint.detail}</div>
        {/if}
      </div>
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
              <span class="metric-time">{formatResetHint(bucket.resetAt)}</span>
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
