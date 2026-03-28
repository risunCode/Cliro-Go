<script lang="ts">
  import { ArrowLeftRight, Clock, Download, Info, Power, PowerOff, RefreshCw, Trash2, Zap } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import codexIcon from '@/assets/icons/codex-icon.png'
  import type { Account } from '@/services/wails-api'
  import {
    formatBucketLabel,
    formatMetricValue,
    formatRelativeReset,
    getOverviewQuotaMetrics,
    getPercentColor,
    hasValidReset,
    metricPercent
  } from '@/utils/accounts/quota'
  import { normalizeProviderID, providerMeta } from '@/utils/accounts/provider'

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

  const inferDisabledReason = (value: Account): 'manually' | 'exhausted' | 'banned' => {
    const quotaStatus = (value.quota?.status || '').toLowerCase()
    const lastError = (value.lastError || '').toLowerCase()

    if (quotaStatus === 'exhausted' || /exhaust|usage limit|quota exceeded|insufficient[_\s-]?quota/.test(lastError)) {
      return 'exhausted'
    }

    if (/banned|suspend|forbidden|blocked|deactivat|terminated|closed/.test(lastError)) {
      return 'banned'
    }

    return 'manually'
  }

  $: providerID = normalizeProviderID(account.provider)
  $: meta = providerMeta(providerID)
  $: isCodex = providerID === 'codex'
  $: canSync = isCodex
  $: metrics = getOverviewQuotaMetrics(account.quota)
  $: displayName = account.email || account.id
  $: disabledReason = account.enabled ? '' : inferDisabledReason(account)
</script>

<article class={`account-card ${selected ? 'is-selected' : ''} ${account.enabled ? '' : 'is-disabled'}`}>
  <div class="card-header">
    <div class="provider-info">
      <div class="provider-icon" style={isCodex ? undefined : `background:${meta.tint}`}>
        {#if isCodex}
          <img src={codexIcon} alt="Codex" class="provider-icon-image" loading="lazy" decoding="async" />
        {:else}
          {meta.marker}
        {/if}
      </div>
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
      <div class="metric-bar metric-disabled-note">Account disabled ({disabledReason}).</div>
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

  {#if confirmingDelete}
    <div class="confirm-row">
      <Button variant="danger" size="sm" className="confirm-btn" disabled={deleteInProgress} on:click={() => onConfirmRemove(account.id)}>
        {deleteInProgress ? 'Removing...' : 'Confirm'}
      </Button>
      <Button variant="ghost" size="sm" className="confirm-btn" on:click={onCancelRemove}>Cancel</Button>
    </div>
  {:else}
    <div class="action-bar">
      <button type="button" class="action-btn" title="Account details" on:click={() => onInfo(account.id)}>
        <Info size={15} />
      </button>
      {#if canSync}
        <button
          type="button"
          class="action-btn sync-btn"
          disabled={busy}
          title="Sync account to CLI auth"
          on:click={() => onStartSync(account.id)}
        >
          <ArrowLeftRight size={15} />
        </button>
      {/if}
      <button
        type="button"
        class={`action-btn ${account.enabled ? 'power-on' : 'power-off'}`}
        title={account.enabled ? 'Disable account' : 'Enable account'}
        on:click={() => onToggleAccount(account.id, !account.enabled)}
      >
        {#if account.enabled}
          <Power size={15} />
        {:else}
          <PowerOff size={15} />
        {/if}
      </button>
      <button
        type="button"
        class="action-btn"
        disabled={busy || refreshing}
        title="Refresh account and check quota"
        on:click={() => onRefreshWithQuota(account.id)}
      >
        <RefreshCw size={15} class={refreshing ? 'is-spinning' : ''} />
      </button>
      <button type="button" class="action-btn" title="Export account" on:click={() => onExport(account.id)}>
        <Download size={15} />
      </button>
      <button type="button" class="action-btn delete-btn" disabled={busy} title="Delete account" on:click={() => onStartRemove(account.id)}>
        <Trash2 size={15} />
      </button>
    </div>
  {/if}
</article>
