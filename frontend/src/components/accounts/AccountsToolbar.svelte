<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { Download, Power, RefreshCw, Trash2, Upload } from 'lucide-svelte'
  import { formatNumber } from '@/utils/formatters'
  import type { ProviderGroup } from '@/utils/accounts/provider'

  export let accountsTotal = 0
  export let accountsByProvider: ProviderGroup[] = []
  export let selectedProvider = 'all'
  export let selectedCount = 0
  export let selectedEnabledCount = 0
  export let hasVisibleAccounts = false
  export let allVisibleSelected = false
  export let view: 'card' | 'table' = 'card'
  export let refreshingAllQuotas = false
  export let bulkBusy = false

  const dispatch = createEventDispatcher<{
    providerChange: string
    viewChange: 'card' | 'table'
    toggleSelectAllVisible: void
    refreshAllQuotas: void
    importAccounts: void
    bulkTogglePower: void
    bulkExport: void
    bulkDelete: void
  }>()

  const activeProviderClass = 'provider-tab-active'
  const inactiveProviderClass = 'provider-tab-inactive'

  $: selectionActive = selectedCount > 0 || allVisibleSelected
  $: selectButtonLabel = selectionActive ? `Select [${formatNumber(selectedCount)}]` : 'Select'
  $: bulkPowerLabel = selectedCount > 0 && selectedEnabledCount === selectedCount ? 'Disable' : 'Enable'
</script>

<div class="accounts-toolbar border-b border-border pb-3">
  <div class="toolbar-top-row">
    <div class="provider-tabs">
      <button
        type="button"
        class={`provider-tab ${selectedProvider === 'all'
          ? activeProviderClass
          : inactiveProviderClass}`}
        on:click={() => dispatch('providerChange', 'all')}
      >
        All ({formatNumber(accountsTotal)})
      </button>

      {#each accountsByProvider as group}
        <button
          type="button"
          class={`provider-tab ${selectedProvider === group.id
            ? activeProviderClass
            : inactiveProviderClass}`}
          on:click={() => dispatch('providerChange', group.id)}
        >
          {group.name} ({formatNumber(group.accounts.length)})
        </button>
      {/each}
    </div>

    <div class="toolbar-bottom-actions">
      <button type="button" class="selection-btn refresh-btn" disabled={refreshingAllQuotas} on:click={() => dispatch('refreshAllQuotas')}>
        <RefreshCw size={14} class={refreshingAllQuotas ? 'is-spinning' : ''} />
        <span>Refresh All Quotas</span>
      </button>

      <button
        type="button"
        class={`selection-btn ${selectionActive ? 'selection-btn-active' : ''}`}
        disabled={!hasVisibleAccounts}
        on:click={() => dispatch('toggleSelectAllVisible')}
      >
        {selectButtonLabel}
      </button>

      <button type="button" class="selection-btn import-btn" disabled={bulkBusy} on:click={() => dispatch('importAccounts')}>
        <Upload size={14} />
        <span>Import</span>
      </button>

      {#if selectionActive}
        <button type="button" class="selection-btn bulk-action-btn" disabled={bulkBusy} on:click={() => dispatch('bulkTogglePower')}>
          <Power size={14} />
          <span>{bulkPowerLabel}</span>
        </button>

        <button type="button" class="selection-btn bulk-action-btn" disabled={bulkBusy} on:click={() => dispatch('bulkExport')}>
          <Download size={14} />
          <span>Export</span>
        </button>

        <button type="button" class="selection-btn bulk-delete-btn" disabled={bulkBusy} on:click={() => dispatch('bulkDelete')}>
          <Trash2 size={14} />
          <span>Delete</span>
        </button>
      {/if}

      <div class="view-toggle" role="tablist" aria-label="Accounts view toggle">
        <button
          type="button"
          class="view-button"
          class:is-active={view === 'card'}
          role="tab"
          aria-selected={view === 'card'}
          title="Grid View"
          on:click={() => dispatch('viewChange', 'card')}
        >
          <svg class="view-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <rect x="3" y="3" width="7" height="7" rx="1"></rect>
            <rect x="14" y="3" width="7" height="7" rx="1"></rect>
            <rect x="14" y="14" width="7" height="7" rx="1"></rect>
            <rect x="3" y="14" width="7" height="7" rx="1"></rect>
          </svg>
          <span class="view-label">Grid</span>
        </button>

        <button
          type="button"
          class="view-button"
          class:is-active={view === 'table'}
          role="tab"
          aria-selected={view === 'table'}
          title="List View"
          on:click={() => dispatch('viewChange', 'table')}
        >
          <svg class="view-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <line x1="8" y1="6" x2="21" y2="6"></line>
            <line x1="8" y1="12" x2="21" y2="12"></line>
            <line x1="8" y1="18" x2="21" y2="18"></line>
            <line x1="3" y1="6" x2="3.01" y2="6"></line>
            <line x1="3" y1="12" x2="3.01" y2="12"></line>
            <line x1="3" y1="18" x2="3.01" y2="18"></line>
          </svg>
          <span class="view-label">List</span>
        </button>
      </div>
    </div>
  </div>
</div>
