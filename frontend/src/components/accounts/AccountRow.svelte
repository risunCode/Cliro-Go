<script lang="ts">
  import { ArrowLeftRight, Download, Info, Power, PowerOff, RefreshCw, Trash2 } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import codexIcon from '@/assets/icons/codex-icon.png'
  import type { Account } from '@/services/wails-api'
  import {
    formatBucketLabel,
    formatQuotaDateTime,
    formatRelativeReset,
    getOverviewQuotaMetrics,
    getPercentColor,
    hasValidReset,
    metricPercent,
    deriveQuotaDisplayStatus,
    quotaStatusLabel
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

  $: providerID = normalizeProviderID(account.provider)
  $: meta = providerMeta(providerID)
  $: isCodex = providerID === 'codex'
  $: metrics = getOverviewQuotaMetrics(account.quota)
  $: metricsWithReset = metrics.filter((metric) => hasValidReset(metric.resetAt))
  $: displayName = account.email || account.id
  $: quotaStatus = deriveQuotaDisplayStatus(account.quota)
  $: canSync = isCodex
</script>

<tr class={`table-row ${selected ? 'is-selected' : ''}`}>
  <td class="col-check">
    <input class="table-checkbox" type="checkbox" checked={selected} on:change={() => onToggleSelection(account.id)} aria-label="Select account" />
  </td>

  <td>
    <div class="table-account">
      <p class="table-account-name">{displayName}</p>
      <p class="table-account-sub">{account.planType || 'plan-unknown'}</p>
      {#if account.lastError}
        <p class="table-error">{account.lastError}</p>
      {/if}
    </div>
  </td>

  <td>
    <span class="provider-chip">
      <span class="provider-chip-dot" style={isCodex ? undefined : `background:${meta.tint}`}>
        {#if isCodex}
          <img src={codexIcon} alt="Codex" class="provider-chip-image" loading="lazy" decoding="async" />
        {:else}
          {meta.marker}
        {/if}
      </span>
      <span>{meta.label}</span>
    </span>
  </td>

  <td>
    <div class="reset-stack">
      {#if metricsWithReset.length > 0}
        {#each metricsWithReset as metric}
          <div class="reset-item">
            <span class="reset-name">{formatBucketLabel(metric.name)}</span>
            <span class="reset-relative">{formatRelativeReset(metric.resetAt)}</span>
            <span class="reset-absolute">{formatQuotaDateTime(metric.resetAt)}</span>
          </div>
        {/each}
      {:else}
        <span class="reset-none">-</span>
      {/if}
    </div>
  </td>

  <td>
    <div class="status-block">
      <div class="status-row">
        <span class={`status-pill ${account.enabled ? 'enabled' : 'disabled'}`}>{account.enabled ? 'Enabled' : 'Disabled'}</span>
        <span class={`status-pill quota-${quotaStatus}`}>{quotaStatusLabel(quotaStatus)}</span>
      </div>
      <div class="table-metrics">
        {#each metrics as metric}
          {@const percent = metricPercent(metric)}
          {@const tone = getPercentColor(percent)}
          <div class={`table-metric table-metric-${tone}`}>
            <div class="table-metric-head">
              <span>{formatBucketLabel(metric.name)}</span>
              <span>{percent.toFixed(0)}%</span>
            </div>
            <div class="table-progress">
              <span style={`width:${percent}%`}></span>
            </div>
          </div>
        {:else}
          <span class="reset-none">No quota data</span>
        {/each}
      </div>
    </div>
  </td>

  <td class="col-actions">
    {#if confirmingDelete}
      <div class="table-confirm">
        <Button variant="danger" size="sm" className="table-text-btn" disabled={deleteInProgress} on:click={() => onConfirmRemove(account.id)}>
          {deleteInProgress ? 'Removing...' : 'Confirm'}
        </Button>
        <Button variant="ghost" size="sm" className="table-text-btn" on:click={onCancelRemove}>Cancel</Button>
      </div>
    {:else}
      <div class="table-actions">
        <Button variant="ghost" size="sm" className="table-icon-btn" title="Details" on:click={() => onInfo(account.id)}>
          <Info size={13} />
        </Button>
        {#if canSync}
          <Button
            variant="ghost"
            size="sm"
            className="table-icon-btn sync-btn"
            disabled={busy}
            title="Sync account to CLI auth"
            on:click={() => onStartSync(account.id)}
          >
            <ArrowLeftRight size={13} />
          </Button>
        {/if}
        <Button
          variant="ghost"
          size="sm"
          className={`table-icon-btn ${account.enabled ? 'power-on' : 'power-off'}`}
          title={account.enabled ? 'Disable account' : 'Enable account'}
          on:click={() => onToggleAccount(account.id, !account.enabled)}
        >
          {#if account.enabled}
            <Power size={13} />
          {:else}
            <PowerOff size={13} />
          {/if}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="table-icon-btn"
          disabled={busy || refreshing}
          title="Refresh account and check quota"
          on:click={() => onRefreshWithQuota(account.id)}
        >
          <RefreshCw size={13} class={refreshing ? 'is-spinning' : ''} />
        </Button>
        <Button variant="ghost" size="sm" className="table-icon-btn" title="Export" on:click={() => onExport(account.id)}>
          <Download size={13} />
        </Button>
        <Button variant="ghost" size="sm" className="table-icon-btn danger" disabled={busy} title="Delete" on:click={() => onStartRemove(account.id)}>
          <Trash2 size={13} />
        </Button>
      </div>
    {/if}
  </td>
</tr>
