<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { ChevronDown, Link2, Search, Upload } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import ConnectPromptModal from '@/components/accounts/ConnectPromptModal.svelte'
  import KiroConnectModal from '@/components/accounts/KiroConnectModal.svelte'
  import AccountDetailModal from '@/components/accounts/AccountDetailModal.svelte'
  import AccountSyncModal from '@/components/accounts/AccountSyncModal.svelte'
  import BatchDeleteModal from '@/components/accounts/BatchDeleteModal.svelte'
  import AccountsToolbar from '@/components/accounts/AccountsToolbar.svelte'
  import AccountsGrid from '@/components/accounts/AccountsGrid.svelte'
  import AccountsTable from '@/components/accounts/AccountsTable.svelte'
  import { toastStore } from '@/stores/toast'
  import { getErrorMessage } from '@/services/error'
  import {
    computeAccountsViewState,
    isPendingAuthSession,
    parseImportedAccounts,
    runAccountSyncByTarget,
    sanitizeSelectedIDs,
    shouldAttachPendingSession,
    shouldDismissPromptAfterSuccess,
    syncTargetName
  } from '@/services/accounts'
  import type {
    Account,
    AuthSession,
    AccountSyncResult,
    CodexAuthSyncResult,
    KiroAuthSession,
    KiloAuthSyncResult,
    OpencodeAuthSyncResult,
    SyncTargetID
  } from '@/services/wails-api'
  import { formatNumber } from '@/utils/formatters'
  import { copyTextToClipboard, downloadJSONFile, hasClipboardWrite } from '@/utils/browser'
  import { toggleSelectAllVisible, toggleSelectedID } from '@/utils/account'

  const SHOW_EXHAUSTED_STORAGE_KEY = 'accounts-show-exhausted'
  const SHOW_DISABLED_STORAGE_KEY = 'accounts-show-disabled'

  const readStoredBoolean = (key: string, fallback: boolean): boolean => {
    if (typeof window === 'undefined') {
      return fallback
    }

    const stored = window.localStorage.getItem(key)
    if (stored === 'true') {
      return true
    }
    if (stored === 'false') {
      return false
    }

    return fallback
  }

  const hasStoredBoolean = (key: string): boolean => {
    if (typeof window === 'undefined') {
      return false
    }
    const stored = window.localStorage.getItem(key)
    return stored === 'true' || stored === 'false'
  }

  export let accounts: Account[] = []
  export let busyAccountIds: string[] = []
  export let authSession: AuthSession | null = null
  export let kiroAuthSession: KiroAuthSession | null = null
  export let authWorking = false
  export let refreshingAllQuotas = false
  export let showExhaustedDefault = true
  export let showDisabledDefault = true

  export let onStartConnect: () => Promise<void>
  export let onCancelConnect: () => Promise<void>
  export let onStartKiroConnect: (method: 'device' | 'google' | 'github') => Promise<void>
  export let onCancelKiroConnect: () => Promise<void>
  export let onOpenExternalURL: (url: string) => Promise<void>
  export let onRefreshAllQuotas: () => Promise<void>
  export let onForceRefreshAllQuotas: () => Promise<void>
  export let onToggleAccount: (accountId: string, enabled: boolean) => Promise<void>
  export let onBulkToggleAccounts: (accountIds: string[], enabled: boolean) => Promise<void>
  export let onBulkDeleteAccounts: (accountIds: string[]) => Promise<void>
  export let onImportAccounts: (accounts: Account[]) => Promise<number>
  export let onSyncCodexAccountToKiloAuth: (accountId: string) => Promise<KiloAuthSyncResult>
  export let onSyncCodexAccountToOpencodeAuth: (accountId: string) => Promise<OpencodeAuthSyncResult>
  export let onSyncCodexAccountToCodexCLI: (accountId: string) => Promise<CodexAuthSyncResult>
  export let onRefreshAccountWithQuota: (accountId: string) => Promise<void>
  export let onDeleteAccount: (accountId: string) => Promise<void>

  const isBannedAccount = (account: Account): boolean => {
    return Boolean(account.banned)
  }

  let selectedIds: string[] = []
  let confirmRemoveAccountID = ''
  let refreshingAccount = ''
  let actionAccount = ''
  let showConnectPrompt = false
  let showKiroConnectModal = false
  let connectPromptSessionID = ''
  let kiroPromptSessionID = ''
  let connectMenuOpen = false
  let connectMenuEl: HTMLDivElement | null = null
  let detailAccount: Account | null = null
  let syncAccountID = ''
  let syncTargetID: SyncTargetID = 'kilo-cli'
  let syncBusy = false
  let syncError = ''
  let syncResult: AccountSyncResult | null = null
  let showSyncModal = false
  let showBulkDeleteModal = false
  let showBannedDeleteModal = false
  let searchQuery = ''
  let selectedProvider = 'all'
  let showExhausted = true
  let showDisabled = true
  let visibilityInitialized = false
  let exhaustedDisabledCount = 0
  let view: 'card' | 'table' = 'card'
  let bulkBusy = false
  let importInputEl: HTMLInputElement | null = null

  $: viewState = computeAccountsViewState(accounts, selectedIds, selectedProvider, searchQuery, {
    showExhausted,
    showDisabled
  })
  $: accountsByProvider = viewState.accountsByProvider
  $: filteredAccounts = viewState.filteredAccounts
  $: visibleAccountIds = viewState.visibleAccountIds
  $: hasVisibleAccounts = viewState.hasVisibleAccounts
  $: allVisibleSelected = viewState.allVisibleSelected
  $: selectedAccounts = viewState.selectedAccounts
  $: selectedEnabledCount = viewState.selectedEnabledCount
  $: exhaustedDisabledCount = viewState.exhaustedDisabledCount
  $: showExhaustedDisabled = showExhausted && showDisabled
  $: exhaustedDisabledFilterLabel = `${showExhaustedDisabled ? 'Hide' : 'Show'} Exhausted/Disabled [${formatNumber(exhaustedDisabledCount)}]`
  $: bannedAccountIDs = accounts.filter((account) => isBannedAccount(account)).map((account) => account.id)
  $: bannedCount = bannedAccountIDs.length
  $: syncAccount = accounts.find((account) => account.id === syncAccountID) || null
  $: {
    const nextSelectedIDs = sanitizeSelectedIDs(selectedIds, accounts)
    if (nextSelectedIDs.length !== selectedIds.length) {
      selectedIds = nextSelectedIDs
    }
  }

  function handleToggleSelection(id: string) {
    selectedIds = toggleSelectedID(selectedIds, id)
  }

  function handleToggleSelectAllVisible() {
    selectedIds = toggleSelectAllVisible(selectedIds, visibleAccountIds, allVisibleSelected)
  }

  function handleSearchInput(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    searchQuery = target.value
  }

  const sanitizeFileSegment = (value: string | undefined, fallback: string): string => {
    const normalized = (value || '').trim().toLowerCase()
    if (!normalized) {
      return fallback
    }

    const sanitized = normalized
      .replace(/[@\s]+/g, '_')
      .replace(/[^a-z0-9._-]/g, '_')
      .replace(/_+/g, '_')
      .replace(/^[_\-.]+|[_\-.]+$/g, '')

    return sanitized || fallback
  }

  const buildExportFileName = (account: Account): string => {
    const provider = sanitizeFileSegment(account.provider, 'provider')
    const identity = sanitizeFileSegment(account.email || account.id, 'account')
    return `cliro_${provider}_${identity}.json`
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
      syncResult = await runAccountSyncByTarget(syncAccountID, target, {
        toKilo: onSyncCodexAccountToKiloAuth,
        toOpencode: onSyncCodexAccountToOpencodeAuth,
        toCodex: onSyncCodexAccountToCodexCLI
      })
    } catch (error) {
      syncError = getErrorMessage(error, `Unable to sync account to ${syncTargetName(target)} auth file.`)
    } finally {
      syncBusy = false
    }
  }

  async function handleExportAccount(accountId: string) {
    const account = accounts.find((candidate) => candidate.id === accountId)
    if (!account) {
      return
    }

    downloadJSONFile(account, buildExportFileName(account))
  }

  async function handleBulkEnable() {
    if (selectedIds.length === 0 || bulkBusy) {
      return
    }

    bulkBusy = true
    try {
      await onBulkToggleAccounts([...selectedIds], true)
      selectedIds = []
    } catch (error) {
      toastStore.push('error', 'Bulk Update Failed', getErrorMessage(error, 'Unable to update selected accounts.'))
    } finally {
      bulkBusy = false
    }
  }

  async function handleBulkDisable() {
    if (selectedIds.length === 0 || bulkBusy) {
      return
    }

    bulkBusy = true
    try {
      await onBulkToggleAccounts([...selectedIds], false)
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

    downloadJSONFile(payload, `accounts-selected-${Date.now()}.json`)
  }

  async function handleBulkDelete() {
    if (selectedIds.length === 0 || bulkBusy) {
      return
    }

    showBulkDeleteModal = true
  }

  async function handleForceRefreshAllQuotas() {
    if (bulkBusy || refreshingAllQuotas) {
      return
    }

    bulkBusy = true
    try {
      await onForceRefreshAllQuotas()
    } catch (error) {
      toastStore.push('error', 'Force Refresh Failed', getErrorMessage(error, 'Unable to force refresh all quota snapshots.'))
    } finally {
      bulkBusy = false
    }
  }

  async function handleDeleteBannedAccounts() {
    if (bannedCount === 0 || bulkBusy) {
      return
    }

    showBannedDeleteModal = true
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

  async function handleConfirmDeleteBanned() {
    if (bannedAccountIDs.length === 0 || bulkBusy) {
      showBannedDeleteModal = false
      return
    }

    bulkBusy = true
    try {
      await onBulkDeleteAccounts([...bannedAccountIDs])
      selectedIds = sanitizeSelectedIDs(selectedIds, accounts.filter((account) => !isBannedAccount(account)))
      showBannedDeleteModal = false
    } catch (error) {
      toastStore.push('error', 'Delete Banned Failed', getErrorMessage(error, 'Unable to delete banned accounts.'))
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

  function handleCancelDeleteBanned() {
    if (bulkBusy) {
      return
    }
    showBannedDeleteModal = false
  }

  const handleStartConnect = async (): Promise<void> => {
    showConnectPrompt = true
    if (isPendingAuthSession(authSession)) {
      connectPromptSessionID = authSession?.sessionId || ''
      return
    }

    connectPromptSessionID = ''
    await onStartConnect()
    connectPromptSessionID = authSession?.sessionId || ''
  }

  const closeConnectMenu = (): void => {
    connectMenuOpen = false
  }

  const toggleConnectMenu = (): void => {
    connectMenuOpen = !connectMenuOpen
  }

  const handleSelectConnectProvider = async (provider: 'codex' | 'kiro'): Promise<void> => {
    closeConnectMenu()

    if (provider === 'codex') {
      showKiroConnectModal = false
      await handleStartConnect()
      return
    }

    showConnectPrompt = false
    connectPromptSessionID = ''

    if (isPendingAuthSession(kiroAuthSession)) {
      kiroPromptSessionID = kiroAuthSession?.sessionId || ''
      showKiroConnectModal = true
      return
    }

    kiroPromptSessionID = ''
    showKiroConnectModal = true
  }

  const handleStartKiroDeviceAuth = async (): Promise<void> => {
	showKiroConnectModal = true
	try {
		await onStartKiroConnect('device')
	} catch {
		showKiroConnectModal = false
	}
  }

  const handleStartKiroGoogleAuth = async (): Promise<void> => {
	showKiroConnectModal = true
	try {
		await onStartKiroConnect('google')
	} catch {
		showKiroConnectModal = false
	}
  }

  const handleStartKiroGitHubAuth = async (): Promise<void> => {
	showKiroConnectModal = true
	try {
		await onStartKiroConnect('github')
	} catch {
		showKiroConnectModal = false
	}
  }

  const handleOpenKiroAuthLink = async (): Promise<void> => {
    if (!kiroAuthSession?.authUrl) {
      return
    }
    await onOpenExternalURL(kiroAuthSession.authUrl)
  }

  const handleCopyKiroAuthLink = async (): Promise<void> => {
    if (!kiroAuthSession?.authUrl || !hasClipboardWrite()) {
      return
    }

    await copyTextToClipboard(kiroAuthSession.authUrl)
  }

  const handleCopyKiroUserCode = async (): Promise<void> => {
    if (!kiroAuthSession?.userCode || !hasClipboardWrite()) {
      return
    }

    await copyTextToClipboard(kiroAuthSession.userCode)
  }

  const handleCancelKiroModal = async (): Promise<void> => {
    showKiroConnectModal = false
    kiroPromptSessionID = ''
    if (isPendingAuthSession(kiroAuthSession)) {
      await onCancelKiroConnect()
    }
  }

  const handleDismissKiroModal = (): void => {
    showKiroConnectModal = false
    kiroPromptSessionID = ''
  }

  const handleOpenAuthLink = async (): Promise<void> => {
    if (!authSession?.authUrl) {
      return
    }
    await onOpenExternalURL(authSession.authUrl)
  }

  const handleCopyAuthLink = async (): Promise<void> => {
    if (!authSession?.authUrl || !hasClipboardWrite()) {
      return
    }

    await copyTextToClipboard(authSession.authUrl)
  }

  const handleCancelFromModal = async (): Promise<void> => {
    showConnectPrompt = false
    connectPromptSessionID = ''
    if (isPendingAuthSession(authSession)) {
      await onCancelConnect()
    }
  }

  const handleDismissModal = (): void => {
    showConnectPrompt = false
    connectPromptSessionID = ''
  }

  const handleGlobalPointerDown = (event: MouseEvent): void => {
    if (!connectMenuOpen) {
      return
    }
    const target = event.target as Node | null
    if (target && connectMenuEl && !connectMenuEl.contains(target)) {
      closeConnectMenu()
    }
  }

  const handleGlobalKeyDown = (event: KeyboardEvent): void => {
    if (event.key === 'Escape') {
      closeConnectMenu()
    }
  }

  $: if (shouldAttachPendingSession(showConnectPrompt, connectPromptSessionID, authSession)) {
    connectPromptSessionID = authSession.sessionId
  }

  $: if (shouldDismissPromptAfterSuccess(showConnectPrompt, connectPromptSessionID, authSession)) {
    showConnectPrompt = false
    connectPromptSessionID = ''
  }

  $: if (shouldAttachPendingSession(showKiroConnectModal, kiroPromptSessionID, kiroAuthSession)) {
    kiroPromptSessionID = kiroAuthSession?.sessionId || ''
  }

  $: if (shouldDismissPromptAfterSuccess(showKiroConnectModal, kiroPromptSessionID, kiroAuthSession)) {
    showKiroConnectModal = false
    kiroPromptSessionID = ''
  }

  $: if (!visibilityInitialized) {
    showExhausted = readStoredBoolean(SHOW_EXHAUSTED_STORAGE_KEY, showExhaustedDefault)
    showDisabled = readStoredBoolean(SHOW_DISABLED_STORAGE_KEY, showDisabledDefault)
    visibilityInitialized = true
  }

  $: if (visibilityInitialized && typeof window !== 'undefined') {
    if (!hasStoredBoolean(SHOW_EXHAUSTED_STORAGE_KEY)) {
      showExhausted = showExhaustedDefault
    }
    if (!hasStoredBoolean(SHOW_DISABLED_STORAGE_KEY)) {
      showDisabled = showDisabledDefault
    }
  }

  $: if (typeof window !== 'undefined') {
    window.localStorage.setItem(SHOW_EXHAUSTED_STORAGE_KEY, String(showExhausted))
    window.localStorage.setItem(SHOW_DISABLED_STORAGE_KEY, String(showDisabled))
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

<div class="accounts-page space-y-4">
  <section class="rounded-sm border border-border bg-surface p-4">
    <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm font-semibold text-text-primary">Connect Account</p>
        <p class="text-xs text-text-secondary">Select provider first to avoid modal collisions (Codex OAuth or KiroAI device auth).</p>
      </div>
      <div class="connect-provider-menu" bind:this={connectMenuEl}>
        <Button on:click={toggleConnectMenu} disabled={authWorking} variant="primary" size="sm" className="connect-provider-trigger">
          <Link2 size={14} class="mr-1" />
          Connect Account
          <ChevronDown size={14} class={`connect-provider-chevron ${connectMenuOpen ? 'is-open' : ''}`} />
        </Button>

        {#if connectMenuOpen}
          <div class="connect-provider-panel" role="menu" aria-label="Connect provider menu">
            <button type="button" class="connect-provider-item" role="menuitem" on:click={() => void handleSelectConnectProvider('codex')}>
              <span>Codex (OpenAI)</span>
            </button>
            <button type="button" class="connect-provider-item" role="menuitem" on:click={() => void handleSelectConnectProvider('kiro')}>
              <span>KiroAI (Device Auth)</span>
            </button>
          </div>
        {/if}
      </div>
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

    {#if kiroAuthSession?.status === 'error'}
      <div class="auth-session mt-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="session-pill status-error">error</span>
          <span class="text-[11px] text-text-secondary">Kiro Session: {kiroAuthSession.sessionId || '-'}</span>
        </div>
        {#if kiroAuthSession.error}
          <p class="mt-1 text-[11px] text-error">{kiroAuthSession.error}</p>
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

        <div class="flex w-full items-center gap-2 lg:w-auto">
          <button
            type="button"
            class={`selection-btn tone-warning filter-toggle-btn ${showExhaustedDisabled ? '' : 'selection-btn-active'}`}
            on:click={() => {
              const nextVisibility = !showExhaustedDisabled
              showExhausted = nextVisibility
              showDisabled = nextVisibility
            }}
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
      on:providerChange={(event) => {
        selectedProvider = event.detail
      }}
      on:viewChange={(event) => {
        view = event.detail
      }}
      on:toggleSelectAllVisible={handleToggleSelectAllVisible}
      on:refreshAllQuotas={onRefreshAllQuotas}
      on:forceRefreshAllQuotas={handleForceRefreshAllQuotas}
      on:bulkEnable={handleBulkEnable}
      on:bulkDisable={handleBulkDisable}
      on:bulkExport={handleBulkExport}
      on:bulkDelete={handleBulkDelete}
      on:bulkDeleteBanned={handleDeleteBannedAccounts}
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
        {busyAccountIds}
        {allVisibleSelected}
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
    pending={isPendingAuthSession(authSession)}
    canCopyLink={hasClipboardWrite()}
    on:openLink={handleOpenAuthLink}
    on:copyLink={handleCopyAuthLink}
    on:dismiss={handleDismissModal}
    on:cancel={handleCancelFromModal}
  />

  <KiroConnectModal
    open={showKiroConnectModal}
    authUrl={kiroAuthSession?.authUrl || ''}
    userCode={kiroAuthSession?.userCode || ''}
    authMethod={kiroAuthSession?.authMethod || ''}
    provider={kiroAuthSession?.provider || ''}
    busy={authWorking}
    pending={isPendingAuthSession(kiroAuthSession)}
    canCopyLink={hasClipboardWrite()}
    on:startDevice={handleStartKiroDeviceAuth}
    on:startGoogle={handleStartKiroGoogleAuth}
    on:startGitHub={handleStartKiroGitHubAuth}
    on:openLink={handleOpenKiroAuthLink}
    on:copyLink={handleCopyKiroAuthLink}
    on:copyCode={handleCopyKiroUserCode}
    on:dismiss={handleDismissKiroModal}
    on:cancel={handleCancelKiroModal}
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
    title="Delete Selected Accounts"
    description="This action will remove selected records from local storage."
    summaryLabel="selected account(s)"
    confirmLabel="Delete Selected"
    on:cancel={handleCancelBulkDelete}
    on:confirm={handleConfirmBulkDelete}
  />

  <BatchDeleteModal
    open={showBannedDeleteModal}
    count={bannedCount}
    busy={bulkBusy}
    title="Delete Banned Accounts"
    description="This action removes all accounts explicitly marked as banned."
    summaryLabel="banned account(s)"
    confirmLabel="Delete Banned"
    on:cancel={handleCancelDeleteBanned}
    on:confirm={handleConfirmDeleteBanned}
  />
</div>
