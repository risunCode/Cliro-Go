<script lang="ts">
  import { ArrowLeftRight, Download, Info, Power, PowerOff, RefreshCw, Trash2 } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'

  export let mode: 'card' | 'table' = 'card'
  export let accountID = ''
  export let enabled = false
  export let canSync = false
  export let busy = false
  export let refreshing = false
  export let confirmingDelete = false
  export let deleteInProgress = false

  export let onToggleAccount: (accountID: string, enabled: boolean) => Promise<void> | void
  export let onStartSync: (accountID: string) => Promise<void> | void
  export let onInfo: (accountID: string) => Promise<void> | void
  export let onRefreshWithQuota: (accountID: string) => Promise<void> | void
  export let onExport: (accountID: string) => Promise<void> | void
  export let onStartRemove: (accountID: string) => Promise<void> | void
  export let onConfirmRemove: (accountID: string) => Promise<void> | void
  export let onCancelRemove: () => Promise<void> | void

  $: isCard = mode === 'card'
  $: iconSize = isCard ? 15 : 13
</script>

{#if confirmingDelete}
  {#if isCard}
    <div class="confirm-row">
      <Button variant="danger" size="sm" className="confirm-btn" disabled={deleteInProgress || busy} on:click={() => onConfirmRemove(accountID)}>
        {deleteInProgress ? 'Removing...' : 'Confirm'}
      </Button>
      <Button variant="ghost" size="sm" className="confirm-btn" disabled={deleteInProgress || busy} on:click={onCancelRemove}>Cancel</Button>
    </div>
  {:else}
    <div class="table-confirm">
      <Button variant="danger" size="sm" className="table-text-btn" disabled={deleteInProgress || busy} on:click={() => onConfirmRemove(accountID)}>
        {deleteInProgress ? 'Removing...' : 'Confirm'}
      </Button>
      <Button variant="ghost" size="sm" className="table-text-btn" disabled={deleteInProgress || busy} on:click={onCancelRemove}>Cancel</Button>
    </div>
  {/if}
{:else if isCard}
  <div class="action-bar">
    <button type="button" class="action-btn" title="Account details" disabled={busy} on:click={() => onInfo(accountID)}>
      <Info size={iconSize} />
    </button>
    {#if canSync}
      <button
        type="button"
        class="action-btn tone-sync"
        title="Sync account to CLI auth"
        disabled={busy}
        on:click={() => onStartSync(accountID)}
      >
        <ArrowLeftRight size={iconSize} />
      </button>
    {/if}
    <button
      type="button"
      class={`action-btn ${enabled ? 'tone-success' : 'tone-danger'}`}
      title={enabled ? 'Disable account' : 'Enable account'}
      disabled={busy}
      on:click={() => onToggleAccount(accountID, !enabled)}
    >
      {#if enabled}
        <Power size={iconSize} />
      {:else}
        <PowerOff size={iconSize} />
      {/if}
    </button>
    <button
      type="button"
      class="action-btn"
      title="Refresh account and check quota"
      disabled={busy || refreshing}
      on:click={() => onRefreshWithQuota(accountID)}
    >
      <RefreshCw size={iconSize} class={refreshing ? 'accounts-spinning' : ''} />
    </button>
    <button type="button" class="action-btn" title="Export account" disabled={busy} on:click={() => onExport(accountID)}>
      <Download size={iconSize} />
    </button>
    <button type="button" class="action-btn tone-danger" title="Delete account" disabled={busy} on:click={() => onStartRemove(accountID)}>
      <Trash2 size={iconSize} />
    </button>
  </div>
{:else}
  <div class="table-actions">
    <Button variant="ghost" size="sm" className="table-icon-btn" title="Details" disabled={busy} on:click={() => onInfo(accountID)}>
      <Info size={iconSize} />
    </Button>
    {#if canSync}
      <Button
        variant="ghost"
        size="sm"
        className="table-icon-btn tone-sync"
        title="Sync account to CLI auth"
        disabled={busy}
        on:click={() => onStartSync(accountID)}
      >
        <ArrowLeftRight size={iconSize} />
      </Button>
    {/if}
    <Button
      variant="ghost"
      size="sm"
      className={`table-icon-btn ${enabled ? 'tone-success' : 'tone-danger'}`}
      title={enabled ? 'Disable account' : 'Enable account'}
      disabled={busy}
      on:click={() => onToggleAccount(accountID, !enabled)}
    >
      {#if enabled}
        <Power size={iconSize} />
      {:else}
        <PowerOff size={iconSize} />
      {/if}
    </Button>
    <Button
      variant="ghost"
      size="sm"
      className="table-icon-btn"
      title="Refresh account and check quota"
      disabled={busy || refreshing}
      on:click={() => onRefreshWithQuota(accountID)}
    >
      <RefreshCw size={iconSize} class={refreshing ? 'accounts-spinning' : ''} />
    </Button>
    <Button variant="ghost" size="sm" className="table-icon-btn" title="Export" disabled={busy} on:click={() => onExport(accountID)}>
      <Download size={iconSize} />
    </Button>
    <Button variant="ghost" size="sm" className="table-icon-btn tone-danger" title="Delete" disabled={busy} on:click={() => onStartRemove(accountID)}>
      <Trash2 size={iconSize} />
    </Button>
  </div>
{/if}
