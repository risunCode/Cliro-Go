<script lang="ts">
  import { Link2, Search } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import ConnectPromptModal from '@/components/accounts/ConnectPromptModal.svelte'
  import AccountDetailModal from '@/components/accounts/AccountDetailModal.svelte'
  import AccountSyncModal from '@/components/accounts/AccountSyncModal.svelte'
  import BatchDeleteModal from '@/components/accounts/BatchDeleteModal.svelte'
  import AccountsToolbar from '@/components/accounts/AccountsToolbar.svelte'
  import AccountsGrid from '@/components/accounts/AccountsGrid.svelte'
  import AccountsTable from '@/components/accounts/AccountsTable.svelte'
  import { toastStore } from '@/stores/toast'
  import { getErrorMessage } from '@/services/error'
  import type {
    Account,
    AuthSession,
    AccountSyncResult,
    CodexAuthSyncResult,
    KiloAuthSyncResult,
    OpencodeAuthSyncResult,
    SyncTargetID
  } from '@/services/wails-api'
  import { formatNumber } from '@/utils/formatters'
  import { filterAccounts } from '@/utils/accounts/filters'
  import { groupAccountsByProvider } from '@/utils/accounts/provider'
  import { areAllVisibleSelected, toggleSelectAllVisible, toggleSelectedID } from '@/utils/accounts/selection'
  import './AccountsTab.css'

  export let accounts: Account[] = []
  export let authSession: AuthSession | null = null
  export let authWorking = false
  export let busyAccountIds: string[] = []
  export let refreshingAllQuotas = false

  export let onStartConnect: () => Promise<void>
  export let onCancelConnect: () => Promise<void>
  export let onOpenExternalURL: (url: string) => Promise<void>
  export let onRefreshAllQuotas: () => Promise<void>
  export let onToggleAccount: (accountId: string, enabled: boolean) => Promise<void>
  export let onBulkToggleAccounts: (accountIds: string[], enabled: boolean) => Promise<void>
  export let onBulkDeleteAccounts: (accountIds: string[]) => Promise<void>
  export let onImportAccounts: (accounts: Account[]) => Promise<number>
  export let onSyncCodexAccountToKiloAuth: (accountId: string) => Promise<KiloAuthSyncResult>
  export let onSyncCodexAccountToOpencodeAuth: (accountId: string) => Promise<OpencodeAuthSyncResult>
  export let onSyncCodexAccountToCodexCLI: (accountId: string) => Promise<CodexAuthSyncResult>
  export let onRefreshAccountWithQuota: (accountId: string) => Promise<void>
  export let onDeleteAccount: (accountId: string) => Promise<void>

  const isPendingAuth = (session: AuthSession | null): boolean => {
    return (session?.status ?? '') === 'pending'
  }

  let selectedIds: string[] = []
  let confirmRemoveAccountID = ''
  let refreshingAccount = ''
  let actionAccount = ''
  let showConnectPrompt = false
  let connectPromptSessionID = ''
  let detailAccount: Account | null = null
  let syncAccountID = ''
  let syncTargetID: SyncTargetID = 'kilo-cli'
  let syncBusy = false
  let syncError = ''
  let syncResult: AccountSyncResult | null = null
  let showSyncModal = false
  let showBulkDeleteModal = false
  let searchQuery = ''
  let selectedProvider = 'all'
  let view: 'card' | 'table' = 'card'
  let bulkBusy = false
  let importInputEl: HTMLInputElement | null = null

  $: accountsByProvider = groupAccountsByProvider(accounts)
  $: filteredAccounts = filterAccounts(accounts, { providerId: selectedProvider, query: searchQuery })
  $: visibleAccountIds = filteredAccounts.map((account) => account.id)
  $: hasVisibleAccounts = visibleAccountIds.length > 0
  $: allVisibleSelected = areAllVisibleSelected(selectedIds, visibleAccountIds)
  $: selectedAccounts = accounts.filter((account) => selectedIds.includes(account.id))
  $: selectedEnabledCount = selectedAccounts.filter((account) => account.enabled).length
  $: bulkToggleToEnabled = selectedIds.length > 0 && selectedEnabledCount !== selectedIds.length
  $: validAccountIDs = new Set(accounts.map((account) => account.id))
  $: syncAccount = accounts.find((account) => account.id === syncAccountID) || null
  $: {
    const nextSelectedIDs = selectedIds.filter((id) => validAccountIDs.has(id))
    if (nextSelectedIDs.length !== selectedIds.length) {
      selectedIds = nextSelectedIDs
    }
  }

  const canCopyAuthLink = (): boolean => {
    return typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function'
  }

  function handleToggleSelection(id: string) {
    selectedIds = toggleSelectedID(selectedIds, id)
  }

  function handleToggleSelectAllVisible() {
    selectedIds = toggleSelectAllVisible(selectedIds, visibleAccountIds, allVisibleSelected)
  }

  const parseImportedAccounts = (raw: unknown): Account[] => {
    if (Array.isArray(raw)) {
      return raw.filter((item) => item && typeof item === 'object') as Account[]
    }

    if (raw && typeof raw === 'object') {
      const payload = raw as Record<string, unknown>
      if (Array.isArray(payload.accounts)) {
        return payload.accounts.filter((item) => item && typeof item === 'object') as Account[]
      }
      return [raw as Account]
    }

    return []
  }

  function handleSearchInput(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    searchQuery = target.value
  }

  async function handleRefreshWithQuota(accountId: string) {
    refreshingAccount = accountId
    try {
      await onRefreshAccountWithQuota(accountId)
    } finally {
      refreshingAccount = ''
    }
  }

  async function handleRemoveConfirm(accountId: string) {
    actionAccount = accountId
    try {
      await onDeleteAccount(accountId)
      confirmRemoveAccountID = ''
    } finally {
      actionAccount = ''
    }
  }

  function handleRemoveStart(accountId: string) {
    confirmRemoveAccountID = accountId
  }

  function handleRemoveCancel() {
    confirmRemoveAccountID = ''
  }

  function handleAccountInfo(accountId: string) {
    const account = accounts.find((candidate) => candidate.id === accountId)
    if (account) {
      detailAccount = account
    }
  }

  function closeDetailModal() {
    detailAccount = null
  }

  function openSyncModal(accountID: string) {
    const account = accounts.find((candidate) => candidate.id === accountID)
    if (!account) {
      return
    }

    syncAccountID = accountID
    syncTargetID = 'kilo-cli'
    syncBusy = false
    syncError = ''
    syncResult = null
    showSyncModal = true
  }

  function closeSyncModal() {
    if (syncBusy) {
      return
    }

    showSyncModal = false
    syncAccountID = ''
    syncTargetID = 'kilo-cli'
    syncBusy = false
    syncError = ''
    syncResult = null
  }

  async function handleConfirmSync(event: CustomEvent<SyncTargetID>) {
    if (!syncAccountID || syncBusy) {
      return
    }

    const target = event.detail || syncTargetID
    syncTargetID = target
    syncBusy = true
    syncError = ''
    syncResult = null

    try {
      if (target === 'codex-cli') {
        syncResult = await onSyncCodexAccountToCodexCLI(syncAccountID)
      } else if (target === 'opencode-cli') {
        syncResult = await onSyncCodexAccountToOpencodeAuth(syncAccountID)
      } else {
        syncResult = await onSyncCodexAccountToKiloAuth(syncAccountID)
      }
    } catch (error) {
      const targetName = target === 'codex-cli' ? 'Codex CLI' : target === 'opencode-cli' ? 'Opencode' : 'Kilo CLI'
      syncError = getErrorMessage(error, `Unable to sync account to ${targetName} auth file.`)
    } finally {
      syncBusy = false
    }
  }

  async function handleExportAccount(accountId: string) {
    const account = accounts.find((candidate) => candidate.id === accountId)
    if (!account) {
      return
    }

    const data = JSON.stringify(account, null, 2)
    const blob = new Blob([data], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `account-${accountId}.json`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  async function handleBulkTogglePower() {
    if (selectedIds.length === 0 || bulkBusy) {
      return
    }

    bulkBusy = true
    try {
      await onBulkToggleAccounts([...selectedIds], bulkToggleToEnabled)
      selectedIds = []
    } catch (error) {
      toastStore.push('error', 'Bulk Update Failed', getErrorMessage(error, 'Unable to update selected accounts.'))
    } finally {
      bulkBusy = false
    }
  }

  async function handleBulkExport() {
    if (selectedIds.length === 0 || bulkBusy) {
      return
    }

    const payload = {
      exportedAt: new Date().toISOString(),
      count: selectedAccounts.length,
      accounts: selectedAccounts
    }

    const data = JSON.stringify(payload, null, 2)
    const blob = new Blob([data], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `accounts-selected-${Date.now()}.json`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  async function handleBulkDelete() {
    if (selectedIds.length === 0 || bulkBusy) {
      return
    }

    showBulkDeleteModal = true
  }

  async function handleConfirmBulkDelete() {
    if (selectedIds.length === 0 || bulkBusy) {
      showBulkDeleteModal = false
      return
    }

    bulkBusy = true
    try {
      await onBulkDeleteAccounts([...selectedIds])
      selectedIds = []
      showBulkDeleteModal = false
    } catch (error) {
      toastStore.push('error', 'Bulk Delete Failed', getErrorMessage(error, 'Unable to delete selected accounts.'))
    } finally {
      bulkBusy = false
    }
  }

  function handleOpenImportPicker() {
    if (bulkBusy) {
      return
    }
    importInputEl?.click()
  }

  async function handleImportFile(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    const file = target.files?.[0]
    if (!file || bulkBusy) {
      return
    }

    bulkBusy = true
    try {
      const text = await file.text()
      const parsed = JSON.parse(text)
      const importedAccounts = parseImportedAccounts(parsed)
      if (importedAccounts.length === 0) {
        throw new Error('No valid account records found in selected file.')
      }
      await onImportAccounts(importedAccounts)
    } catch (error) {
      toastStore.push('error', 'Import Failed', getErrorMessage(error, 'Unable to import account file.'))
    } finally {
      bulkBusy = false
      target.value = ''
    }
  }

  function handleCancelBulkDelete() {
    if (bulkBusy) {
      return
    }
    showBulkDeleteModal = false
  }

  const handleStartConnect = async (): Promise<void> => {
    showConnectPrompt = true
    if (isPendingAuth(authSession)) {
      connectPromptSessionID = authSession?.sessionId || ''
      return
    }

    connectPromptSessionID = ''
    await onStartConnect()
    connectPromptSessionID = authSession?.sessionId || ''
  }

  const handleOpenAuthLink = async (): Promise<void> => {
    if (!authSession?.authUrl) {
      return
    }
    await onOpenExternalURL(authSession.authUrl)
  }

  const handleCopyAuthLink = async (): Promise<void> => {
    if (!authSession?.authUrl || !canCopyAuthLink()) {
      return
    }

    await navigator.clipboard.writeText(authSession.authUrl)
  }

  const handleCancelFromModal = async (): Promise<void> => {
    showConnectPrompt = false
    connectPromptSessionID = ''
    if (isPendingAuth(authSession)) {
      await onCancelConnect()
    }
  }

  const handleDismissModal = (): void => {
    showConnectPrompt = false
    connectPromptSessionID = ''
  }

  $: if (
    showConnectPrompt &&
    connectPromptSessionID === '' &&
    authSession?.status === 'pending' &&
    authSession?.sessionId
  ) {
    connectPromptSessionID = authSession.sessionId
  }

  $: if (
    showConnectPrompt &&
    connectPromptSessionID !== '' &&
    authSession?.sessionId === connectPromptSessionID &&
    authSession?.status === 'success'
  ) {
    showConnectPrompt = false
    connectPromptSessionID = ''
  }
</script>

<div class="accounts-page space-y-4">
  <section class="rounded-sm border border-border bg-surface p-4">
    <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm font-semibold text-text-primary">Connect Account</p>
        <p class="text-xs text-text-secondary">Use OAuth callback flow to add Codex-compatible accounts.</p>
      </div>
      <Button on:click={handleStartConnect} disabled={authWorking} variant="primary" size="sm">
        <Link2 size={14} class="mr-1" />
        Connect Account
      </Button>
    </div>

    {#if authSession?.status === 'error'}
      <div class="auth-session mt-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="session-pill status-error">error</span>
          <span class="text-[11px] text-text-secondary">Session: {authSession.sessionId || '-'}</span>
        </div>
        {#if authSession.error}
          <p class="mt-1 text-[11px] text-error">{authSession.error}</p>
        {/if}
      </div>
    {/if}
  </section>

  <section class="rounded-sm border border-border bg-surface p-4">
    <input bind:this={importInputEl} type="file" accept=".json,application/json" class="hidden" on:change={handleImportFile} />

    <div class="mb-3">
      <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <p class="text-sm font-semibold text-text-primary">Accounts</p>
          <p class="text-xs text-text-secondary">Total {formatNumber(accounts.length)} accounts in pool.</p>
        </div>

        <div class="relative w-full lg:w-80">
          <Search class="accounts-search-icon absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            on:input={handleSearchInput}
            placeholder="Search by email, account ID, provider"
            class="accounts-search-input w-full rounded-lg py-2 pl-9 pr-3 text-[13px] transition-colors"
          />
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
      {view}
      {refreshingAllQuotas}
      {bulkBusy}
      on:providerChange={(event) => {
        selectedProvider = event.detail
      }}
      on:viewChange={(event) => {
        view = event.detail
      }}
      on:toggleSelectAllVisible={handleToggleSelectAllVisible}
      on:refreshAllQuotas={onRefreshAllQuotas}
      on:importAccounts={handleOpenImportPicker}
      on:bulkTogglePower={handleBulkTogglePower}
      on:bulkExport={handleBulkExport}
      on:bulkDelete={handleBulkDelete}
    />

    {#if filteredAccounts.length === 0}
      <div class="empty-state">No accounts match your current filters.</div>
    {:else if view === 'card'}
      <AccountsGrid
        accounts={filteredAccounts}
        {selectedIds}
        {busyAccountIds}
        refreshingAccountID={refreshingAccount}
        {confirmRemoveAccountID}
        actionAccountID={actionAccount}
        onToggleSelection={handleToggleSelection}
        {onToggleAccount}
        onStartSync={openSyncModal}
        onInfo={handleAccountInfo}
        onRefreshWithQuota={handleRefreshWithQuota}
        onExport={handleExportAccount}
        onStartRemove={handleRemoveStart}
        onConfirmRemove={handleRemoveConfirm}
        onCancelRemove={handleRemoveCancel}
      />
    {:else}
      <AccountsTable
        accounts={filteredAccounts}
        {selectedIds}
        {allVisibleSelected}
        {busyAccountIds}
        refreshingAccountID={refreshingAccount}
        {confirmRemoveAccountID}
        actionAccountID={actionAccount}
        onToggleSelection={handleToggleSelection}
        onToggleSelectAllVisible={handleToggleSelectAllVisible}
        {onToggleAccount}
        onStartSync={openSyncModal}
        onInfo={handleAccountInfo}
        onRefreshWithQuota={handleRefreshWithQuota}
        onExport={handleExportAccount}
        onStartRemove={handleRemoveStart}
        onConfirmRemove={handleRemoveConfirm}
        onCancelRemove={handleRemoveCancel}
      />
    {/if}
  </section>

  <ConnectPromptModal
    open={showConnectPrompt}
    authUrl={authSession?.authUrl || ''}
    busy={authWorking}
    pending={isPendingAuth(authSession)}
    canCopyLink={canCopyAuthLink()}
    on:openLink={handleOpenAuthLink}
    on:copyLink={handleCopyAuthLink}
    on:dismiss={handleDismissModal}
    on:cancel={handleCancelFromModal}
  />

  <AccountDetailModal open={Boolean(detailAccount)} account={detailAccount} on:dismiss={closeDetailModal} />

  <AccountSyncModal
    open={showSyncModal}
    account={syncAccount}
    loading={syncBusy}
    error={syncError}
    result={syncResult}
    selectedTargetID={syncTargetID}
    on:close={closeSyncModal}
    on:confirm={handleConfirmSync}
  />

  <BatchDeleteModal
    open={showBulkDeleteModal}
    count={selectedIds.length}
    busy={bulkBusy}
    on:cancel={handleCancelBulkDelete}
    on:confirm={handleConfirmBulkDelete}
  />
</div>
