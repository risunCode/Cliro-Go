<script lang="ts">
  import AccountActions from './AccountActions.svelte'
  import ProviderAvatar from './ProviderAvatar.svelte'
  import type { Account } from '@/features/accounts/types'
  import {
    formatBucketLabel,
    formatQuotaDateTime,
    formatRelativeReset,
    getPercentColor,
    metricPercent,
    quotaStatusLabel
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
  $: metricsWithReset = presentation.metricsWithReset
  $: quotaStatus = presentation.quotaStatus
</script>

<tr class={`table-row ${selected ? 'is-selected' : ''}`}>
  <td class="col-check">
    <input class="table-checkbox" type="checkbox" checked={selected} on:change={() => onToggleSelection(account.id)} aria-label="Select account" />
  </td>

  <td>
    <div class="table-account">
      <p class="table-account-name">{presentation.displayName}</p>
      <p class="table-account-sub">{account.planType || 'plan-unknown'}</p>
      {#if account.banned && account.bannedReason}
        <p class="table-error">{account.bannedReason}</p>
      {/if}
      {#if account.lastError && account.lastError !== account.bannedReason}
        <p class="table-error">{account.lastError}</p>
      {/if}
    </div>
  </td>

  <td>
    <span class="provider-chip">
      <ProviderAvatar provider={account.provider} variant="chip" showLabel />
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
        {#if account.banned}
          <span class="status-pill quota-error">Banned</span>
        {/if}
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
    <AccountActions
      mode="table"
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
  </td>
</tr>
