<script lang="ts">
  import AccountsConnectSection from '@/features/accounts/components/connect/AccountsConnectSection.svelte'
  import AccountsListSection from '@/features/accounts/components/list/AccountsListSection.svelte'
  import AccountsWorkspaceModals from '@/features/accounts/components/modals/AccountsWorkspaceModals.svelte'
  import { toastStore } from '@/shared/stores/toast'
  import { getErrorMessage } from '@/shared/lib/error'
  import { accountsAuthApi } from '@/features/accounts/api/auth-api'
  import {
    computeAccountsViewState,
    isPendingAuthSession,
    sanitizeSelectedIDs,
    shouldAttachPendingSession,
    shouldDismissPromptAfterSuccess
  } from '@/features/accounts/lib/workspace'
  import { loadAccountsPreferences, saveAccountsPreferences } from '@/features/accounts/lib/preferences'
  import {
    buildAccountExportFileName,
    createInitialWorkspaceState,
    findAccountByID,
    getBannedAccountIDs,
    isBannedAccount,
    readImportedAccountsFile
  } from '@/features/accounts/lib/workspace-controller'
  import { runAccountSyncByTarget, syncTargetName } from '@/features/accounts/lib/sync'
  import type {
    Account,
    AuthSession,
    AccountSyncResult,
    CodexAuthSyncResult,
    KiroAuthSession,
    KiloAuthSyncResult,
    OpencodeAuthSyncResult,
    SyncTargetID
  } from '@/features/accounts/types'
  import { formatNumber } from '@/shared/lib/formatters'
  import { copyTextToClipboard, downloadJSONFile, hasClipboardWrite } from '@/shared/lib/browser'
  import { toggleSelectAllVisible, toggleSelectedID } from '@/features/accounts/lib/account'

  export let accounts: Account[] = []
  export let busyAccountIds: string[] = []
  export let authSession: AuthSession | null = null
  export let kiroAuthSession: KiroAuthSession | null = null
  export let authWorking = false
  export let refreshingAllQuotas = false
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

  const initialState = createInitialWorkspaceState(loadAccountsPreferences())

  let selectedIds = initialState.selectedIds
  let confirmRemoveAccountID = initialState.confirmRemoveAccountID
  let refreshingAccount = initialState.refreshingAccountID
  let actionAccount = initialState.actionAccountID
  let showConnectPrompt = initialState.showConnectPrompt
  let showKiroConnectModal = initialState.showKiroConnectModal
  let connectPromptSessionID = initialState.connectPromptSessionID
  let kiroPromptSessionID = initialState.kiroPromptSessionID
  let connectPanelOpen = initialState.connectPanelOpen
  let detailAccount = initialState.detailAccount
  let syncAccountID = initialState.syncAccountID
  let syncTargetID: SyncTargetID = initialState.syncTargetID
  let syncBusy = initialState.syncBusy
  let syncError = initialState.syncError
  let syncResult: AccountSyncResult | null = initialState.syncResult
  let showSyncModal = initialState.showSyncModal
  let showBulkDeleteModal = initialState.showBulkDeleteModal
  let showBannedDeleteModal = initialState.showBannedDeleteModal
  let searchQuery = initialState.searchQuery
  let selectedProvider = initialState.selectedProvider
  let showExhausted = initialState.showExhausted
  let showDisabled = initialState.showDisabled
  let view = initialState.view
  let bulkBusy = initialState.bulkBusy

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
  $: bannedAccountIDs = getBannedAccountIDs(accounts)
  $: bannedCount = bannedAccountIDs.length
  $: syncAccount = findAccountByID(accounts, syncAccountID)
  $: canCopyLink = hasClipboardWrite()
  $: saveAccountsPreferences({ showExhausted, showDisabled, view })
  $: {
    const nextSelectedIDs = sanitizeSelectedIDs(selectedIds, accounts)
    if (nextSelectedIDs.length !== selectedIds.length) {
      selectedIds = nextSelectedIDs
    }
  }

  $: if (kiroAuthSession?.status === 'error' && showKiroConnectModal) {
    showKiroConnectModal = false
  }

  $: if (authSession?.status === 'error' && showConnectPrompt) {
    showConnectPrompt = false
  }

  function handleToggleSelection(id: string) {
    selectedIds = toggleSelectedID(selectedIds, id)
  }

  function handleToggleSelectAllVisible() {
    selectedIds = toggleSelectAllVisible(selectedIds, visibleAccountIds, allVisibleSelected)
  }

  function handleToggleExhaustedDisabled() {
    const nextVisibility = !showExhaustedDisabled
    showExhausted = nextVisibility
    showDisabled = nextVisibility
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
    detailAccount = findAccountByID(accounts, accountId)
  }

  function closeDetailModal() {
    detailAccount = null
  }

  function openSyncModal(accountID: string) {
    if (!findAccountByID(accounts, accountID)) {
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
    const account = findAccountByID(accounts, accountId)
    if (!account) {
      return
    }

    downloadJSONFile(account, buildAccountExportFileName(account))
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

    downloadJSONFile(
      {
        exportedAt: new Date().toISOString(),
        count: selectedAccounts.length,
        accounts: selectedAccounts
      },
      `accounts-selected-${Date.now()}.json`
    )
  }

  function handleBulkDelete() {
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

  function handleDeleteBannedAccounts() {
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

  async function handleImportFile(file: File) {
    if (bulkBusy) {
      return
    }

    bulkBusy = true
    try {
      const importedAccounts = await readImportedAccountsFile(file)
      await onImportAccounts(importedAccounts)
    } catch (error) {
      toastStore.push('error', 'Import Failed', getErrorMessage(error, 'Unable to import account file.'))
    } finally {
      bulkBusy = false
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

  const handleSelectConnectProvider = async (provider: 'codex' | 'kiro'): Promise<void> => {
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

    if (kiroAuthSession && kiroAuthSession.status !== 'pending') {
      kiroAuthSession = null
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
    try {
      await onStartKiroConnect('google')
    } catch {
      showKiroConnectModal = false
    }
  }

  const handleStartKiroGithubAuth = async (): Promise<void> => {
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
    if (!kiroAuthSession?.authUrl || !canCopyLink) {
      return
    }

    await copyTextToClipboard(kiroAuthSession.authUrl)
  }

  const handleCopyKiroUserCode = async (): Promise<void> => {
    if (!kiroAuthSession?.userCode || !canCopyLink) {
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
    if (!authSession?.authUrl || !canCopyLink) {
      return
    }

    await copyTextToClipboard(authSession.authUrl)
  }

  const handleSubmitCodexAuthCode = async (event: CustomEvent<{ code: string }>): Promise<void> => {
    const code = event.detail.code
    if (!code || !authSession?.sessionId) {
      return
    }
    try {
      await accountsAuthApi.submitCodexAuthCode(authSession.sessionId, code)
      toastStore.push('success', 'Authorization Code Submitted', 'Exchanging code for access token...')
    } catch (error) {
      toastStore.push('error', 'Code Submission Failed', getErrorMessage(error))
    }
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
</script>

<div class="accounts-page space-y-4">
  <AccountsConnectSection bind:open={connectPanelOpen} {authWorking} onSelectProvider={handleSelectConnectProvider} />

  <AccountsListSection
    {accounts}
    {accountsByProvider}
    {filteredAccounts}
    {selectedIds}
    {busyAccountIds}
    {allVisibleSelected}
    {selectedProvider}
    {selectedEnabledCount}
    {hasVisibleAccounts}
    {bannedCount}
    {view}
    {refreshingAllQuotas}
    {bulkBusy}
    {searchQuery}
    {showExhaustedDisabled}
    {exhaustedDisabledFilterLabel}
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
    on:providerChange={(event) => {
      selectedProvider = event.detail
    }}
    on:viewChange={(event) => {
      view = event.detail
    }}
    on:toggleExhaustedDisabled={handleToggleExhaustedDisabled}
    on:toggleSelectAllVisible={handleToggleSelectAllVisible}
    on:refreshAllQuotas={onRefreshAllQuotas}
    on:forceRefreshAllQuotas={handleForceRefreshAllQuotas}
    on:bulkEnable={handleBulkEnable}
    on:bulkDisable={handleBulkDisable}
    on:bulkExport={handleBulkExport}
    on:bulkDelete={handleBulkDelete}
    on:bulkDeleteBanned={handleDeleteBannedAccounts}
    on:searchChange={(event) => {
      searchQuery = event.detail
    }}
    on:importFile={(event) => void handleImportFile(event.detail)}
  />

  <AccountsWorkspaceModals
    {showConnectPrompt}
    {showKiroConnectModal}
    {authSession}
    {kiroAuthSession}
    {authWorking}
    {canCopyLink}
    {detailAccount}
    {showSyncModal}
    {syncAccount}
    {syncBusy}
    {syncError}
    {syncResult}
    {syncTargetID}
    {showBulkDeleteModal}
    selectedCount={selectedIds.length}
    {showBannedDeleteModal}
    {bannedCount}
    {bulkBusy}
    onOpenAuthLink={handleOpenAuthLink}
    onCopyAuthLink={handleCopyAuthLink}
    onSubmitCodexAuthCode={handleSubmitCodexAuthCode}
    onDismissModal={handleDismissModal}
    onCancelFromModal={handleCancelFromModal}
    onStartKiroDeviceAuth={handleStartKiroDeviceAuth}
    onStartKiroGoogleAuth={handleStartKiroGoogleAuth}
    onStartKiroGithubAuth={handleStartKiroGithubAuth}
    onOpenKiroAuthLink={handleOpenKiroAuthLink}
    onCopyKiroAuthLink={handleCopyKiroAuthLink}
    onCopyKiroUserCode={handleCopyKiroUserCode}
    onDismissKiroModal={handleDismissKiroModal}
    onCancelKiroModal={handleCancelKiroModal}
    onCloseDetailModal={closeDetailModal}
    onCloseSyncModal={closeSyncModal}
    onConfirmSync={handleConfirmSync}
    onCancelBulkDelete={handleCancelBulkDelete}
    onConfirmBulkDelete={handleConfirmBulkDelete}
    onCancelDeleteBanned={handleCancelDeleteBanned}
    onConfirmDeleteBanned={handleConfirmDeleteBanned}
  />
</div>
