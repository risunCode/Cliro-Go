<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import AppHeader from '@/components/common/AppHeader.svelte'
  import AppFooter from '@/components/common/AppFooter.svelte'
  import ToastViewport from '@/components/common/ToastViewport.svelte'
  import DashboardTab from '@/tabs/DashboardTab.svelte'
  import AccountsTab from '@/tabs/AccountsTab.svelte'
  import ApiRouterTab from '@/tabs/ApiRouterTab.svelte'
  import SystemLogsTab from '@/tabs/SystemLogsTab.svelte'
  import SettingsTab from '@/tabs/SettingsTab.svelte'
  import { theme, toggleTheme } from '@/stores/theme'
  import { toastStore } from '@/stores/toast'
  import { getErrorMessage } from '@/services/error'
  import {
    appService,
    type Account,
    type AppState,
    type AuthSession,
    type LogEntry,
    type ProxyStatus,
    type KiloAuthSyncResult,
    type OpencodeAuthSyncResult,
    type CodexAuthSyncResult
  } from '@/services/wails-api'
  import { fetchCoreSnapshot, fetchLogsSnapshot } from '@/services/bootstrap'
  import { createProxyActionRunner } from '@/services/proxy-actions'
  import { createAuthSessionController } from '@/services/auth-session'
  import { subscribeToRingLogs } from '@/services/logs-subscription'
  import type { AppTabId } from '@/utils/tabs'

  let activeTab: AppTabId = 'dashboard'

  let state: AppState | null = null
  let accounts: Account[] = []
  let proxyStatus: ProxyStatus | null = null
  let logs: LogEntry[] = []

  let loadingDashboard = false
  let loadingLogs = false
  let proxyBusy = false
  let authWorking = false
  let refreshingAllQuotas = false
  let busyAccountIds: string[] = []

  let authSession: AuthSession | null = null

  const markAccountBusy = (accountId: string, busy: boolean): void => {
    if (busy) {
      if (!busyAccountIds.includes(accountId)) {
        busyAccountIds = [...busyAccountIds, accountId]
      }
      return
    }
    busyAccountIds = busyAccountIds.filter((item) => item !== accountId)
  }

  const refreshState = async (): Promise<void> => {
    state = await appService.getState()
  }

  const refreshAccounts = async (): Promise<void> => {
    accounts = await appService.getAccounts()
  }

  const refreshProxyStatus = async (): Promise<void> => {
    proxyStatus = await appService.getProxyStatus()
  }

  const refreshLogs = async (limit = 400): Promise<void> => {
    loadingLogs = true
    try {
      logs = await fetchLogsSnapshot(limit)
    } finally {
      loadingLogs = false
    }
  }

  const refreshCore = async (): Promise<void> => {
    loadingDashboard = true
    try {
      const snapshot = await fetchCoreSnapshot()
      state = snapshot.state
      accounts = snapshot.accounts
      proxyStatus = snapshot.proxyStatus
    } finally {
      loadingDashboard = false
    }
  }

  const notifyError = (title: string, error: unknown): void => {
    toastStore.push('error', title, getErrorMessage(error, 'Unexpected operation failure.'))
  }

  const notifySuccess = (title: string, message: string): void => {
    toastStore.push('success', title, message)
  }

  const onTabChange = (event: CustomEvent<AppTabId>): void => {
    activeTab = event.detail
  }

  const runProxyAction = createProxyActionRunner({
    setBusy: (busy) => {
      proxyBusy = busy
    },
    refresh: async () => {
      await Promise.all([refreshState(), refreshProxyStatus()])
    },
    notifySuccess,
    notifyError
  })

  const handleToggleAccount = async (accountId: string, enabled: boolean): Promise<void> => {
    try {
      await appService.toggleAccount(accountId, enabled)
      await Promise.all([refreshAccounts(), refreshState()])
      notifySuccess('Account Updated', `Account ${enabled ? 'enabled' : 'disabled'} successfully.`)
    } catch (error) {
      notifyError('Toggle Account Failed', error)
    }
  }

  const handleBulkToggleAccounts = async (accountIds: string[], enabled: boolean): Promise<void> => {
    const uniqueIDs = [...new Set(accountIds.map((id) => id.trim()).filter((id) => id.length > 0))]
    if (uniqueIDs.length === 0) {
      return
    }

    const failures: string[] = []
    for (const accountId of uniqueIDs) {
      try {
        await appService.toggleAccount(accountId, enabled)
      } catch {
        failures.push(accountId)
      }
    }

    await Promise.all([refreshAccounts(), refreshState()])

    const successCount = uniqueIDs.length - failures.length
    if (successCount > 0) {
      notifySuccess('Bulk Account Update', `${successCount} account(s) ${enabled ? 'enabled' : 'disabled'}.`)
    }
    if (failures.length > 0) {
      throw new Error(`${failures.length} account(s) failed to update.`)
    }
  }

  const handleRefreshAccountWithQuota = async (accountId: string): Promise<void> => {
    markAccountBusy(accountId, true)
    let refreshErr: unknown = null
    try {
      await appService.refreshAccount(accountId)
      await appService.refreshQuota(accountId)
      notifySuccess('Account Refreshed', 'Token refreshed and quota checked.')
    } catch (error) {
      refreshErr = error
      notifyError('Refresh Account Failed', error)
    } finally {
      try {
        await Promise.all([refreshAccounts(), refreshState()])
      } catch (syncError) {
        notifyError('Refresh Snapshot Failed', syncError)
      }
      markAccountBusy(accountId, false)
    }

    if (refreshErr) {
      return
    }
  }

  const handleDeleteAccount = async (accountId: string): Promise<void> => {
    markAccountBusy(accountId, true)
    try {
      await appService.deleteAccount(accountId)
      await Promise.all([refreshAccounts(), refreshState()])
      notifySuccess('Account Deleted', 'Account removed from local storage.')
    } catch (error) {
      notifyError('Delete Account Failed', error)
    } finally {
      markAccountBusy(accountId, false)
    }
  }

  const handleBulkDeleteAccounts = async (accountIds: string[]): Promise<void> => {
    const uniqueIDs = [...new Set(accountIds.map((id) => id.trim()).filter((id) => id.length > 0))]
    if (uniqueIDs.length === 0) {
      return
    }

    const failures: string[] = []
    for (const accountId of uniqueIDs) {
      try {
        await appService.deleteAccount(accountId)
      } catch {
        failures.push(accountId)
      }
    }

    await Promise.all([refreshAccounts(), refreshState()])

    const successCount = uniqueIDs.length - failures.length
    if (successCount > 0) {
      notifySuccess('Bulk Delete Complete', `${successCount} account(s) deleted.`)
    }
    if (failures.length > 0) {
      throw new Error(`${failures.length} account(s) failed to delete.`)
    }
  }

  const handleImportAccounts = async (importedAccounts: Account[]): Promise<number> => {
    const count = await appService.importAccounts(importedAccounts)
    await Promise.all([refreshAccounts(), refreshState()])
    notifySuccess('Accounts Imported', `${count} account(s) imported successfully.`)
    return count
  }

  const handleRefreshAllQuotas = async (): Promise<void> => {
    refreshingAllQuotas = true
    let refreshErr: unknown = null
    try {
      await appService.refreshAllQuotas()
      notifySuccess('Quotas Refreshed', 'All account quota snapshots were refreshed.')
    } catch (error) {
      refreshErr = error
      notifyError('Refresh All Quotas Failed', error)
    } finally {
      try {
        await Promise.all([refreshAccounts(), refreshState()])
      } catch (syncError) {
        notifyError('Refresh Snapshot Failed', syncError)
      }
      refreshingAllQuotas = false
    }

    if (refreshErr) {
      return
    }
  }

  const handleSyncCodexAccountToKiloAuth = async (accountId: string): Promise<KiloAuthSyncResult> => {
    markAccountBusy(accountId, true)
    try {
      const result = await appService.syncCodexAccountToKiloAuth(accountId)
      notifySuccess('Kilo CLI Synced', `Auth file updated at ${result.targetPath}.`)
      return result
    } catch (error) {
      notifyError('Kilo CLI Sync Failed', error)
      throw error
    } finally {
      markAccountBusy(accountId, false)
    }
  }

  const handleSyncCodexAccountToCodexCLI = async (accountId: string): Promise<CodexAuthSyncResult> => {
    markAccountBusy(accountId, true)
    try {
      const result = await appService.syncCodexAccountToCodexCLI(accountId)
      notifySuccess('Codex CLI Synced', `Auth file updated at ${result.targetPath}.`)
      return result
    } catch (error) {
      notifyError('Codex CLI Sync Failed', error)
      throw error
    } finally {
      markAccountBusy(accountId, false)
    }
  }

  const handleSyncCodexAccountToOpencodeAuth = async (accountId: string): Promise<OpencodeAuthSyncResult> => {
    markAccountBusy(accountId, true)
    try {
      const result = await appService.syncCodexAccountToOpencodeAuth(accountId)
      notifySuccess('Opencode Synced', `Auth file updated at ${result.targetPath}.`)
      return result
    } catch (error) {
      notifyError('Opencode Sync Failed', error)
      throw error
    } finally {
      markAccountBusy(accountId, false)
    }
  }

  const authController = createAuthSessionController({
    getSession: (sessionId) => appService.getCodexAuthSession(sessionId),
    onSession: (session) => {
      authSession = session
    },
    onSuccess: async () => {
      await Promise.all([refreshAccounts(), refreshState()])
    },
    onSessionError: (session) => {
      toastStore.push('error', 'Authentication Failed', session.error || 'OAuth flow returned an error.')
    },
    onPollingError: (error) => {
      notifyError('Authentication Poll Failed', error)
    }
  })

  const handleStartConnect = async (): Promise<void> => {
    if (authSession?.sessionId && authSession.status === 'pending') {
      authController.start(authSession.sessionId)
      return
    }

    authWorking = true
    try {
      const started = await appService.startCodexAuth()
      authSession = {
        sessionId: started.sessionId,
        authUrl: started.authUrl,
        callbackUrl: started.callbackUrl,
        status: started.status
      }
      authController.start(started.sessionId)
      toastStore.push('info', 'Authentication Started', 'Open the provided auth link to complete the OAuth callback flow.')
    } catch (error) {
      notifyError('Authentication Start Failed', error)
    } finally {
      authWorking = false
    }
  }

  const handleSetAllowLAN = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Proxy Network Mode Updated',
      () => appService.setAllowLAN(enabled),
      enabled ? 'Proxy now accepts LAN traffic.' : 'Proxy now listens only on localhost.'
    )
  }

  const handleSetAutoStartProxy = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Proxy Startup Updated',
      () => appService.setAutoStartProxy(enabled),
      enabled ? 'Proxy will start automatically on app launch.' : 'Proxy autostart disabled.'
    )
  }

  const handleCancelConnect = async (): Promise<void> => {
    if (!authSession?.sessionId) {
      return
    }

    authWorking = true
    try {
      await appService.cancelCodexAuth(authSession.sessionId)
      authController.stop()
      authSession = null
      notifySuccess('Authentication Cancelled', 'Connect flow stopped.')
    } catch (error) {
      notifyError('Cancel Authentication Failed', error)
    } finally {
      authWorking = false
    }
  }

  const handleOpenExternalURL = async (url: string): Promise<void> => {
    if (!url || url.trim().length === 0) {
      return
    }
    try {
      await appService.openExternalURL(url)
    } catch (error) {
      notifyError('Open URL Failed', error)
    }
  }

  const handleClearLogs = async (): Promise<void> => {
    try {
      await appService.clearLogs()
      await refreshLogs()
      notifySuccess('Logs Cleared', 'System logs were cleared successfully.')
    } catch (error) {
      notifyError('Clear Logs Failed', error)
    }
  }

  const handleOpenDataDir = async (): Promise<void> => {
    try {
      await appService.openDataDir()
      toastStore.push('info', 'Data Folder', 'Opened local CLIro-Go data folder.')
    } catch (error) {
      notifyError('Open Data Folder Failed', error)
    }
  }

  let unsubscribeLogs: (() => void) | null = null

  onMount(() => {
    void refreshCore()
    void refreshLogs()

    unsubscribeLogs = subscribeToRingLogs(
      () => logs,
      (nextLogs) => {
        logs = nextLogs
      },
      1000
    )
  })

  onDestroy(() => {
    authController.stop()
    unsubscribeLogs?.()
  })
</script>

<main class="h-screen overflow-hidden bg-app text-text-primary">
  <div class="flex h-full flex-col">
    <AppHeader activeTab={activeTab} on:tabChange={onTabChange} on:toggleTheme={toggleTheme} theme={$theme} />

    <section class="no-scrollbar min-h-0 flex-1 overflow-y-auto px-4 py-4 md:px-6">
      <div class="space-y-4 pb-1">
        {#if activeTab === 'dashboard'}
          <DashboardTab {state} {accounts} {proxyStatus} loading={loadingDashboard} />
        {:else if activeTab === 'accounts'}
          <AccountsTab
            {accounts}
            {authSession}
            {authWorking}
            {busyAccountIds}
            {refreshingAllQuotas}
            onStartConnect={handleStartConnect}
            onCancelConnect={handleCancelConnect}
            onOpenExternalURL={handleOpenExternalURL}
            onRefreshAllQuotas={handleRefreshAllQuotas}
            onToggleAccount={handleToggleAccount}
            onBulkToggleAccounts={handleBulkToggleAccounts}
            onBulkDeleteAccounts={handleBulkDeleteAccounts}
            onImportAccounts={handleImportAccounts}
            onSyncCodexAccountToKiloAuth={handleSyncCodexAccountToKiloAuth}
            onSyncCodexAccountToOpencodeAuth={handleSyncCodexAccountToOpencodeAuth}
            onSyncCodexAccountToCodexCLI={handleSyncCodexAccountToCodexCLI}
            onRefreshAccountWithQuota={handleRefreshAccountWithQuota}
            onDeleteAccount={handleDeleteAccount}
          />
        {:else if activeTab === 'api-router'}
          <ApiRouterTab
            {proxyStatus}
            busy={proxyBusy}
            onRefreshStatus={refreshProxyStatus}
            onStartProxy={() => runProxyAction('Proxy Started', appService.startProxy, 'Proxy service started.')}
            onStopProxy={() => runProxyAction('Proxy Stopped', appService.stopProxy, 'Proxy service stopped.')}
            onSetProxyPort={(port) =>
              runProxyAction('Proxy Port Updated', () => appService.setProxyPort(port), `Proxy port set to ${port}.`)}
            onSetAllowLAN={handleSetAllowLAN}
            onSetAutoStartProxy={handleSetAutoStartProxy}
          />
        {:else if activeTab === 'system-logs'}
          <SystemLogsTab logs={logs} loading={loadingLogs} onRefreshLogs={refreshLogs} onClearLogs={handleClearLogs} />
        {:else if activeTab === 'settings'}
          <SettingsTab onOpenDataDir={handleOpenDataDir} />
        {/if}
      </div>
    </section>

    <AppFooter />
  </div>

  <ToastViewport />
</main>
