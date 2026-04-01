<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { Search, Upload } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import AccountsGrid from './AccountsGrid.svelte'
  import AccountsTable from './AccountsTable.svelte'
  import AccountsToolbar from './AccountsToolbar.svelte'
  import { formatNumber } from '@/shared/lib/formatters'
  import type { Account } from '@/features/accounts/types'
  import type { ProviderGroup } from '@/features/accounts/lib/account'
  import type { AccountsViewMode } from '@/features/accounts/lib/preferences'

  export let accounts: Account[] = []
  export let accountsByProvider: ProviderGroup[] = []
  export let filteredAccounts: Account[] = []
  export let selectedIds: string[] = []
  export let busyAccountIds: string[] = []
  export let allVisibleSelected = false
  export let selectedProvider = 'all'
  export let selectedEnabledCount = 0
  export let hasVisibleAccounts = false
  export let bannedCount = 0
  export let view: AccountsViewMode = 'card'
  export let refreshingAllQuotas = false
  export let bulkBusy = false
  export let searchQuery = ''
  export let showExhaustedDisabled = true
  export let exhaustedDisabledFilterLabel = ''
  export let refreshingAccountID = ''
  export let confirmRemoveAccountID = ''
  export let actionAccountID = ''

  export let onToggleSelection: (accountID: string) => void
  export let onToggleAccount: (accountID: string, enabled: boolean) => Promise<void>
  export let onStartSync: (accountID: string) => void
  export let onInfo: (accountID: string) => void
  export let onRefreshWithQuota: (accountID: string) => Promise<void>
  export let onExport: (accountID: string) => Promise<void>
  export let onStartRemove: (accountID: string) => void
  export let onConfirmRemove: (accountID: string) => Promise<void>
  export let onCancelRemove: () => void

  const dispatch = createEventDispatcher<{
    providerChange: string
    viewChange: AccountsViewMode
    toggleExhaustedDisabled: void
    toggleSelectAllVisible: void
    refreshAllQuotas: void
    forceRefreshAllQuotas: void
    bulkEnable: void
    bulkDisable: void
    bulkExport: void
    bulkDelete: void
    bulkDeleteBanned: void
    searchChange: string
    importFile: File
  }>()

  let importInputEl: HTMLInputElement | null = null

  function handleSearchInput(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    dispatch('searchChange', target.value)
  }

  function handleOpenImportPicker() {
    if (bulkBusy) {
      return
    }

    importInputEl?.click()
  }

  function handleImportChange(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    const file = target.files?.[0]
    if (!file || bulkBusy) {
      return
    }

    dispatch('importFile', file)
    target.value = ''
  }
</script>

<section class="rounded-sm border border-border bg-surface p-4">
  <input bind:this={importInputEl} type="file" accept=".json,application/json" class="hidden" on:change={handleImportChange} />

  <div class="mb-3">
    <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <p class="text-sm font-semibold text-text-primary">Accounts</p>
        <p class="text-xs text-text-secondary">Total {formatNumber(accounts.length)} accounts in pool.</p>
      </div>

      <div class="flex w-full items-center gap-2 lg:w-auto">
        <button
          type="button"
          class={`selection-btn tone-warning filter-toggle-btn ${showExhaustedDisabled ? '' : 'selection-btn-active'}`}
          on:click={() => dispatch('toggleExhaustedDisabled')}
        >
          <span>{exhaustedDisabledFilterLabel}</span>
        </button>

        <Button on:click={handleOpenImportPicker} disabled={bulkBusy} variant="secondary" size="sm" className="whitespace-nowrap">
          <Upload size={14} class="mr-1" />
          Import
        </Button>

        <div class="relative w-full lg:w-80">
          <Search class="accounts-search-icon absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            on:input={handleSearchInput}
            placeholder="Search by email, account ID, provider"
            class="accounts-search-input ui-control-input ui-control-select-sm w-full bg-surface py-2 pl-9 pr-3 text-[13px]"
          />
        </div>
      </div>
    </div>
  </div>

  <AccountsToolbar
    accountsTotal={accounts.length}
    {accountsByProvider}
    {selectedProvider}
    selectedCount={selectedIds.length}
    {selectedEnabledCount}
    {hasVisibleAccounts}
    {allVisibleSelected}
    {bannedCount}
    {view}
    {refreshingAllQuotas}
    {bulkBusy}
    on:providerChange={(event) => dispatch('providerChange', event.detail)}
    on:viewChange={(event) => dispatch('viewChange', event.detail)}
    on:toggleSelectAllVisible={() => dispatch('toggleSelectAllVisible')}
    on:refreshAllQuotas={() => dispatch('refreshAllQuotas')}
    on:forceRefreshAllQuotas={() => dispatch('forceRefreshAllQuotas')}
    on:bulkEnable={() => dispatch('bulkEnable')}
    on:bulkDisable={() => dispatch('bulkDisable')}
    on:bulkExport={() => dispatch('bulkExport')}
    on:bulkDelete={() => dispatch('bulkDelete')}
    on:bulkDeleteBanned={() => dispatch('bulkDeleteBanned')}
  />

  {#if filteredAccounts.length === 0}
    <div class="empty-state">No accounts match your current filters.</div>
  {:else if view === 'card'}
    <AccountsGrid
      accounts={filteredAccounts}
      {selectedIds}
      {busyAccountIds}
      {refreshingAccountID}
      {confirmRemoveAccountID}
      {actionAccountID}
      {onToggleSelection}
      {onToggleAccount}
      {onStartSync}
      {onInfo}
      {onRefreshWithQuota}
      {onExport}
      {onStartRemove}
      {onConfirmRemove}
      {onCancelRemove}
    />
  {:else}
    <AccountsTable
      accounts={filteredAccounts}
      {selectedIds}
      {busyAccountIds}
      {allVisibleSelected}
      {refreshingAccountID}
      {confirmRemoveAccountID}
      {actionAccountID}
      {onToggleSelection}
      onToggleSelectAllVisible={() => dispatch('toggleSelectAllVisible')}
      {onToggleAccount}
      {onStartSync}
      {onInfo}
      {onRefreshWithQuota}
      {onExport}
      {onStartRemove}
      {onConfirmRemove}
      {onCancelRemove}
    />
  {/if}
</section>
