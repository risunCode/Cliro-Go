<script lang="ts">
  import { createEventDispatcher, onDestroy, onMount } from 'svelte'
  import { Ban, ChevronDown, Download, Power, RefreshCw, Trash2 } from 'lucide-svelte'
  import { formatNumber } from '@/shared/lib/formatters'
  import type { ProviderGroup } from '@/features/accounts/lib/account'

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
  export let bannedCount = 0

  const dispatch = createEventDispatcher<{
    providerChange: string
    viewChange: 'card' | 'table'
    toggleSelectAllVisible: void
    refreshAllQuotas: void
    forceRefreshAllQuotas: void
    bulkEnable: void
    bulkDisable: void
    bulkExport: void
    bulkDelete: void
    bulkDeleteBanned: void
  }>()

  let bulkMenuOpen = false
  let bulkMenuEl: HTMLDivElement | null = null

  const activeProviderClass = 'provider-tab-active'
  const inactiveProviderClass = 'provider-tab-inactive'

  $: selectionActive = selectedCount > 0 || allVisibleSelected
  $: selectButtonLabel = selectionActive ? `Select [${formatNumber(selectedCount)}]` : 'Select'
  $: canOpenBulkMenu = accountsTotal > 0 || bannedCount > 0
  $: bulkActionLabel = selectedCount > 0 ? `Bulk Actions [${formatNumber(selectedCount)}]` : 'Bulk Actions'
  $: canEnableSelected = selectedCount > selectedEnabledCount
  $: canDisableSelected = selectedEnabledCount > 0

  const closeBulkMenu = (): void => {
    bulkMenuOpen = false
  }

  const toggleBulkMenu = (): void => {
    if (!canOpenBulkMenu || bulkBusy || refreshingAllQuotas) {
      return
    }
    bulkMenuOpen = !bulkMenuOpen
  }

  const handleBulkMenuAction = (action: 'forceRefreshAllQuotas' | 'bulkEnable' | 'bulkDisable' | 'bulkExport' | 'bulkDelete' | 'bulkDeleteBanned'): void => {
    if (bulkBusy || refreshingAllQuotas) {
      return
    }
    dispatch(action)
    closeBulkMenu()
  }

  const handleGlobalPointerDown = (event: MouseEvent): void => {
    if (!bulkMenuOpen) {
      return
    }
    const target = event.target as Node | null
    if (target && bulkMenuEl && !bulkMenuEl.contains(target)) {
      closeBulkMenu()
    }
  }

  const handleGlobalKeyDown = (event: KeyboardEvent): void => {
    if (event.key === 'Escape') {
      closeBulkMenu()
    }
  }

  onMount(() => {
    document.addEventListener('mousedown', handleGlobalPointerDown)
    document.addEventListener('keydown', handleGlobalKeyDown)
  })

  onDestroy(() => {
    document.removeEventListener('mousedown', handleGlobalPointerDown)
    document.removeEventListener('keydown', handleGlobalKeyDown)
  })
</script>

<div class="accounts-toolbar border-b border-border pb-3">
  <div class="toolbar-top-row">
    <div class="provider-tabs segment-control">
      <button
        type="button"
        class={`provider-tab segment-control-item ${selectedProvider === 'all'
          ? activeProviderClass
          : inactiveProviderClass}`}
        on:click={() => dispatch('providerChange', 'all')}
      >
        All ({formatNumber(accountsTotal)})
      </button>

      {#each accountsByProvider as group}
        <button
          type="button"
          class={`provider-tab segment-control-item ${selectedProvider === group.id
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
        <RefreshCw size={14} class={refreshingAllQuotas ? 'accounts-spinning' : ''} />
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

      <div class="bulk-actions-menu" bind:this={bulkMenuEl}>
        <button
          type="button"
          class={`selection-btn tone-primary ${bulkMenuOpen ? 'selection-btn-active' : ''}`}
          disabled={!canOpenBulkMenu || bulkBusy || refreshingAllQuotas}
          aria-expanded={bulkMenuOpen}
          aria-haspopup="menu"
          on:click={toggleBulkMenu}
        >
          <span>{bulkActionLabel}</span>
          <ChevronDown size={14} class={`bulk-menu-chevron ${bulkMenuOpen ? 'is-open' : ''}`} />
        </button>

        {#if bulkMenuOpen}
          <div class="bulk-actions-panel" role="menu" aria-label="Bulk actions menu">
            <button
              type="button"
              class="bulk-action-item"
              role="menuitem"
              disabled={accountsTotal === 0 || bulkBusy || refreshingAllQuotas}
              on:click={() => handleBulkMenuAction('forceRefreshAllQuotas')}
            >
              <RefreshCw size={14} class={refreshingAllQuotas ? 'accounts-spinning' : ''} />
              <span>Force Refresh All Quotas</span>
            </button>

            <div class="bulk-action-divider" />

            <button
              type="button"
              class="bulk-action-item"
              role="menuitem"
              disabled={!canEnableSelected || bulkBusy}
              on:click={() => handleBulkMenuAction('bulkEnable')}
            >
              <Power size={14} />
              <span>Enable Selected</span>
            </button>

            <button
              type="button"
              class="bulk-action-item"
              role="menuitem"
              disabled={!canDisableSelected || bulkBusy}
              on:click={() => handleBulkMenuAction('bulkDisable')}
            >
              <Power size={14} />
              <span>Disable Selected</span>
            </button>

            <button
              type="button"
              class="bulk-action-item"
              role="menuitem"
              disabled={selectedCount === 0 || bulkBusy}
              on:click={() => handleBulkMenuAction('bulkExport')}
            >
              <Download size={14} />
              <span>Export Selected</span>
            </button>

            <button
              type="button"
              class="bulk-action-item tone-danger"
              role="menuitem"
              disabled={selectedCount === 0 || bulkBusy}
              on:click={() => handleBulkMenuAction('bulkDelete')}
            >
              <Trash2 size={14} />
              <span>Delete Selected</span>
            </button>

            <div class="bulk-action-divider" />

            <button
              type="button"
              class="bulk-action-item tone-danger"
              role="menuitem"
              disabled={bannedCount === 0 || bulkBusy}
              on:click={() => handleBulkMenuAction('bulkDeleteBanned')}
            >
              <Ban size={14} />
              <span>Delete Banned Accounts ({formatNumber(bannedCount)})</span>
            </button>
          </div>
        {/if}
      </div>

      <div class="view-toggle segment-control" role="tablist" aria-label="Accounts view toggle">
        <button
          type="button"
          class="view-button segment-control-item"
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
          class="view-button segment-control-item"
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
