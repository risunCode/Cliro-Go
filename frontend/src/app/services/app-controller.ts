import { get, writable, type Readable } from 'svelte/store'
import { logsApi } from '@/backend/gateways/logs-gateway'
import { systemApi } from '@/backend/gateways/system-gateway'
import { initializeAppBootstrap, type AppBootstrapHandle } from '@/app/bootstrap/app-bootstrap'
import { bindAppActivityEvents, bindAppRuntimeEvents } from '@/app/bootstrap/app-events'
import type { AppTabId } from '@/app/utils/tabs'
import { subscribeToRingLogs } from '@/app/services/logs-subscription'
import { mapStartupWarnings, type StartupWarningEntry } from '@/app/services/startup-warnings'
import type { AppState, LogEntry, UpdateInfo } from '@/app/types'
import { accountsApi } from '@/backend/gateways/accounts-gateway'
import { accountsAuthApi } from '@/backend/gateways/auth-gateway'
import { createAuthSessionController } from '@/features/accounts/utils/auth-session'
import type {
  Account,
  AccountSyncResult,
  AuthSession,
  KiroAuthSession,
  SyncTargetID
} from '@/features/accounts/types'
import { routerApi } from '@/backend/gateways/router-gateway'
import type { CliSyncAppID, CliSyncResult, ProxyStatus } from '@/features/router/types'
import {
  assertBackupPayloadRestorable,
  parseBackupNumber,
  validateBackupPayload,
  type BackupPayload,
  type RestoreProgress
} from '@/features/settings/utils/backup'
import { downloadJSONFile } from '@/shared/utils/browser'
import { getErrorMessage } from '@/shared/utils/error'
import { toastStore } from '@/shared/stores/toast'

const SYSTEM_LOG_LIMIT = 500
const PROXY_HEARTBEAT_INTERVAL_MS = 5000
const ACTIVE_TAB_REFRESH_INTERVAL_MS = 5000
const AUTO_START_PROXY_RETRY_DELAYS_MS = [1500, 4000]
interface ProxyRefreshOptions {
  loading?: boolean
}

interface LogsRefreshOptions {
  loading?: boolean
}

interface SnapshotRefreshOptions {
  loading?: boolean
}

interface AppActionToast {
  title: string
  message: string
}

interface AppActionOptions<T> {
  action: () => Promise<T>
  refresh?: () => Promise<void>
  successToast?: AppActionToast
  onSuccess?: (result: T) => Promise<void> | void
  onError?: (error: unknown) => Promise<void> | void
  errorTitle?: string
  rethrow?: boolean
}

interface BulkMutationResult {
  total: number
  failures: string[]
}

type BatchBehaviorMode = 'parallel' | 'sequential'

export interface AppShellState {
  activeTab: AppTabId
  state: AppState | null
  accounts: Account[]
  proxyStatus: ProxyStatus | null
  logs: LogEntry[]
  authSession: AuthSession | null
  kiroAuthSession: KiroAuthSession | null
  loadingDashboard: boolean
  loadingProxyStatus: boolean
  loadingLogs: boolean
  clearingLogs: boolean
  proxyBusy: boolean
  authWorking: boolean
  refreshingAllQuotas: boolean
  waitingForProxyAutostart: boolean
  busyAccountIds: string[]
}

export interface AppOverlayState {
  showClosePrompt: boolean
  showUpdatePrompt: boolean
  showConfigurationErrorModal: boolean
  startupWarnings: StartupWarningEntry[]
  updateInfo: UpdateInfo | null
  traySupported: boolean
  trayAvailable: boolean
}

export interface AppActions {
  setActiveTab: (tabId: AppTabId) => void
  dismissClosePrompt: () => void
  confirmQuit: () => Promise<void>
  hideToTray: () => Promise<void>
  dismissConfigurationErrorModal: () => void
  dismissUpdatePrompt: () => void
  openUpdateReleasePage: () => Promise<void>
  openExternalURL: (url: string) => Promise<void>
}

export interface AccountsActions {
  startConnect: () => Promise<void>
  cancelConnect: () => Promise<void>
  startKiroConnect: (method: 'device' | 'google' | 'github') => Promise<void>
  cancelKiroConnect: () => Promise<void>
  refreshAllQuotas: () => Promise<void>
  forceRefreshAllQuotas: () => Promise<void>
  toggleAccount: (accountId: string, enabled: boolean) => Promise<void>
  bulkToggleAccounts: (accountIds: string[], enabled: boolean) => Promise<void>
  bulkDeleteAccounts: (accountIds: string[]) => Promise<void>
  importAccounts: (accounts: Account[]) => Promise<number>
  syncAccountAuth: (accountId: string, target: SyncTargetID) => Promise<AccountSyncResult>
  refreshAccountWithQuota: (accountId: string) => Promise<void>
  deleteAccount: (accountId: string) => Promise<void>
}

export interface RouterActions {
  startProxy: () => Promise<void>
  stopProxy: () => Promise<void>
  setProxyPort: (port: number) => Promise<void>
  setAllowLAN: (enabled: boolean) => Promise<void>
  setAutoStartProxy: (enabled: boolean) => Promise<void>
  setProxyAPIKey: (apiKey: string) => Promise<void>
  regenerateProxyAPIKey: () => Promise<string>
  setAuthorizationMode: (enabled: boolean) => Promise<void>
  setSchedulingMode: (mode: string) => Promise<void>
  getModelAliases: () => Promise<Record<string, string>>
  setModelAliases: (aliases: Record<string, string>) => Promise<void>
  refreshProxyStatus: () => Promise<void>
  refreshCloudflaredStatus: () => Promise<void>
  setCloudflaredConfig: (mode: string, token: string, useHttp2: boolean) => Promise<void>
  installCloudflared: () => Promise<void>
  startCloudflared: () => Promise<void>
  stopCloudflared: () => Promise<void>
  getCliSyncStatuses: () => Promise<import('@/features/router/types').CliSyncStatus[]>
  getCliSyncFileContent: (appId: CliSyncAppID, path: string) => Promise<string>
  saveCliSyncFileContent: (appId: CliSyncAppID, path: string, content: string) => Promise<void>
  syncCLIConfig: (appId: CliSyncAppID, model: string) => Promise<CliSyncResult>
}

export interface LogsActions {
  refresh: (limit?: number) => Promise<void>
  clear: () => Promise<void>
}

export interface SettingsActions {
  openDataDir: () => Promise<void>
  exportBackup: () => Promise<void>
  restoreBackup: (payload: BackupPayload, onProgress?: (progress: RestoreProgress) => void) => Promise<void>
}

export interface AppController {
  shell: Readable<AppShellState>
  overlays: Readable<AppOverlayState>
  appActions: AppActions
  accountsActions: AccountsActions
  routerActions: RouterActions
  logsActions: LogsActions
  settingsActions: SettingsActions
  initialize: () => Promise<void>
  destroy: () => void
}

const initialShellState: AppShellState = {
  activeTab: 'dashboard',
  state: null,
  accounts: [],
  proxyStatus: null,
  logs: [],
  authSession: null,
  kiroAuthSession: null,
  loadingDashboard: false,
  loadingProxyStatus: false,
  loadingLogs: false,
  clearingLogs: false,
  proxyBusy: false,
  authWorking: false,
  refreshingAllQuotas: false,
  waitingForProxyAutostart: false,
  busyAccountIds: []
}

const initialOverlayState: AppOverlayState = {
  showClosePrompt: false,
  showUpdatePrompt: false,
  showConfigurationErrorModal: false,
  startupWarnings: [],
  updateInfo: null,
  traySupported: false,
  trayAvailable: false
}

export function createAppController(): AppController {
  const shell = writable<AppShellState>(initialShellState)
  const overlays = writable<AppOverlayState>(initialOverlayState)

  let startupWarningsShown = false
  let unsubscribeLogs: (() => void) | null = null
  let bootstrapHandle: AppBootstrapHandle | null = null
  let proxyHeartbeatTimer: number | null = null
  let activeTabRefreshTimer: number | null = null
  let proxyAutostartRetryTimers: number[] = []
  const inFlightRefreshes = new Map<string, Promise<void>>()

  const patchShell = (patch: Partial<AppShellState>): void => {
    shell.update((current) => ({ ...current, ...patch }))
  }

  const patchOverlays = (patch: Partial<AppOverlayState>): void => {
    overlays.update((current) => ({ ...current, ...patch }))
  }

  const notifyError = (title: string, error: unknown): void => {
    toastStore.push('error', title, getErrorMessage(error, 'Unexpected operation failure.'))
  }

  const notifySuccess = (title: string, message: string): void => {
    toastStore.push('success', title, message)
  }

  const runSingleFlight = (key: string, task: () => Promise<void>): Promise<void> => {
    const inFlight = inFlightRefreshes.get(key)
    if (inFlight) {
      return inFlight
    }

    const promise = (async () => {
      await task()
    })().finally(() => {
      if (inFlightRefreshes.get(key) === promise) {
        inFlightRefreshes.delete(key)
      }
    })

    inFlightRefreshes.set(key, promise)
    return promise
  }

  const throwIfEverythingFailed = (results: PromiseSettledResult<void>[]): void => {
    const failure = results.find((result): result is PromiseRejectedResult => result.status === 'rejected')
    if (failure && results.every((result) => result.status === 'rejected')) {
      throw failure.reason
    }
  }

  const isDocumentVisible = (): boolean => {
    if (typeof document === 'undefined') {
      return true
    }
    return document.visibilityState !== 'hidden'
  }

  const clearProxyHeartbeat = (): void => {
    if (proxyHeartbeatTimer !== null) {
      clearInterval(proxyHeartbeatTimer)
      proxyHeartbeatTimer = null
    }
  }

  const clearActiveTabRefreshTimer = (): void => {
    if (activeTabRefreshTimer !== null) {
      clearInterval(activeTabRefreshTimer)
      activeTabRefreshTimer = null
    }
  }

  const clearProxyAutostartRetries = (): void => {
    proxyAutostartRetryTimers.forEach((timer) => clearTimeout(timer))
    proxyAutostartRetryTimers = []
  }

  const stopPolling = (): void => {
    clearProxyHeartbeat()
    clearActiveTabRefreshTimer()
  }

  const resetClosePromptFlow = ({ hidePrompt }: { hidePrompt: boolean } = { hidePrompt: false }): void => {
    patchOverlays({
      ...(hidePrompt ? { showClosePrompt: false } : {})
    })
  }

  const markAccountBusy = (accountId: string, busy: boolean): void => {
    shell.update((current) => {
      if (busy) {
        if (current.busyAccountIds.includes(accountId)) {
          return current
        }
        return { ...current, busyAccountIds: [...current.busyAccountIds, accountId] }
      }

      return {
        ...current,
        busyAccountIds: current.busyAccountIds.filter((item) => item !== accountId)
      }
    })
  }

  const setAuthWorking = (busy: boolean): void => {
    patchShell({ authWorking: busy })
  }

  const setRefreshingAllQuotas = (busy: boolean): void => {
    patchShell({ refreshingAllQuotas: busy })
  }

  const normalizeAccountIDs = (accountIds: string[]): string[] => {
    return [...new Set(accountIds.map((id) => id.trim()).filter((id) => id.length > 0))]
  }

  const currentBatchBehavior = (): BatchBehaviorMode => 'parallel'

  const runBulkAccountMutation = async (
    accountIds: string[],
    action: (accountId: string) => Promise<void>,
    behavior: BatchBehaviorMode = currentBatchBehavior()
  ): Promise<BulkMutationResult> => {
    if (behavior === 'parallel') {
      const settled = await Promise.allSettled(accountIds.map((accountId) => action(accountId)))
      const failures = settled
        .map((result, index) => (result.status === 'fulfilled' ? '' : accountIds[index]))
        .filter((accountId) => accountId.length > 0)

      return {
        total: accountIds.length,
        failures
      }
    }

    const failures: string[] = []

    for (const accountId of accountIds) {
      try {
        await action(accountId)
      } catch {
        failures.push(accountId)
      }
    }

    return {
      total: accountIds.length,
      failures
    }
  }

  async function withBusyFlag<T>(setBusy: (busy: boolean) => void, action: () => Promise<T>): Promise<T> {
    setBusy(true)
    try {
      return await action()
    } finally {
      setBusy(false)
    }
  }

  async function withAccountBusy<T>(accountId: string, action: () => Promise<T>): Promise<T> {
    return withBusyFlag((busy) => markAccountBusy(accountId, busy), action)
  }

  const syncStartupWarnings = (nextState: AppState | null): void => {
    const nextWarnings = mapStartupWarnings(nextState)
    if (!startupWarningsShown && nextWarnings.length > 0) {
      patchOverlays({
        startupWarnings: nextWarnings,
        showConfigurationErrorModal: true
      })
      startupWarningsShown = true
      return
    }

    if (!startupWarningsShown) {
      patchOverlays({ startupWarnings: nextWarnings })
    }
  }

  const syncTrayAvailability = (nextState: AppState | null): void => {
    patchOverlays({
      traySupported: nextState?.traySupported === true,
      trayAvailable: nextState?.trayAvailable === true
    })
  }

  const cancelProxyAutostartWait = (): void => {
    clearProxyAutostartRetries()
    if (get(shell).waitingForProxyAutostart) {
      patchShell({ waitingForProxyAutostart: false })
    }
  }

  const syncProxyAutostartWait = (nextState?: AppState | null, nextProxyStatus?: ProxyStatus | null): void => {
    if (!get(shell).waitingForProxyAutostart) {
      return
    }

    const resolvedState = nextState ?? get(shell).state
    const resolvedProxyStatus = nextProxyStatus ?? get(shell).proxyStatus
    const proxyRunning = resolvedProxyStatus?.running ?? resolvedState?.proxyRunning ?? false
    const autoStartProxy = resolvedState?.autoStartProxy ?? false

    if (proxyRunning || !autoStartProxy) {
      cancelProxyAutostartWait()
    }
  }

  const refreshState = async (): Promise<void> => {
    await runSingleFlight('state', async () => {
      const nextState = await systemApi.getState()
      patchShell({
        state: nextState,
        accounts: nextState.accounts || []
      })
      syncStartupWarnings(nextState)
      syncTrayAvailability(nextState)
      syncProxyAutostartWait(nextState)
    })
  }

  const refreshStateSafe = async (): Promise<void> => {
    try {
      await refreshState()
    } catch (error) {
      notifyError('Refresh Snapshot Failed', error)
    }
  }

  const refreshStateForClosePrompt = async (): Promise<void> => {
    try {
      const nextState = await systemApi.getState()
      patchShell({
        state: nextState,
        accounts: nextState.accounts || []
      })
      syncTrayAvailability(nextState)
    } catch {
      // Best effort: if state refresh fails, keep current snapshot and still show close prompt.
    }
  }

  const refreshAccountsState = async (): Promise<void> => {
    await refreshState()
  }

  const refreshAccountsStateSafe = async (): Promise<void> => {
    try {
      await refreshAccountsState()
    } catch (error) {
      notifyError('Refresh Snapshot Failed', error)
    }
  }

  const refreshProxyStatus = async ({ loading = false }: ProxyRefreshOptions = {}): Promise<void> => {
    await runSingleFlight('proxy-status', async () => {
      if (loading) {
        patchShell({ loadingProxyStatus: true })
      }

      try {
        const nextProxyStatus = await routerApi.getProxyStatus()
        patchShell({ proxyStatus: nextProxyStatus })
        syncProxyAutostartWait(undefined, nextProxyStatus)
      } finally {
        if (loading) {
          patchShell({ loadingProxyStatus: false })
        }
      }
    })
  }

  const refreshCloudflaredStatus = async ({ loading = false }: ProxyRefreshOptions = {}): Promise<void> => {
    if (loading) {
      patchShell({ loadingProxyStatus: true })
    }

    try {
      const nextProxyStatus = await routerApi.refreshCloudflaredStatus()
      patchShell({ proxyStatus: nextProxyStatus })
      syncProxyAutostartWait(undefined, nextProxyStatus)
    } finally {
      if (loading) {
        patchShell({ loadingProxyStatus: false })
      }
    }
  }

  const refreshProxyStatusSafe = async (): Promise<void> => {
    try {
      await refreshProxyStatus()
    } catch (error) {
      notifyError('Refresh Proxy Status Failed', error)
    }
  }

  const refreshCloudflaredStatusSafe = async (): Promise<void> => {
    try {
      await refreshCloudflaredStatus()
    } catch (error) {
      notifyError('Refresh Cloudflared Status Failed', error)
    }
  }

  const refreshProxySnapshot = async (): Promise<void> => {
    const results = await Promise.allSettled([refreshState(), refreshCloudflaredStatus()])
    throwIfEverythingFailed(results)
  }

  const refreshProxySnapshotSafe = async (): Promise<void> => {
    try {
      await refreshProxySnapshot()
    } catch (error) {
      notifyError('Refresh Snapshot Failed', error)
    }
  }

  const refreshLogs = async (limit = SYSTEM_LOG_LIMIT, { loading = true }: LogsRefreshOptions = {}): Promise<void> => {
    await runSingleFlight(`logs:${limit}`, async () => {
      if (loading) {
        patchShell({ loadingLogs: true })
      }

      try {
        patchShell({ logs: await logsApi.getLogs(limit) })
      } finally {
        if (loading) {
          patchShell({ loadingLogs: false })
        }
      }
    })
  }

  const refreshCore = async ({ loading = false }: SnapshotRefreshOptions = {}): Promise<void> => {
    if (loading) {
      patchShell({ loadingDashboard: true })
    }

    try {
      const results = await Promise.allSettled([refreshState(), refreshProxyStatus({ loading })])
      throwIfEverythingFailed(results)
    } finally {
      if (loading) {
        patchShell({ loadingDashboard: false })
      }
    }
  }

  const refreshUsageSnapshot = async (): Promise<void> => {
    const results = await Promise.allSettled([refreshState(), refreshProxyStatus(), refreshLogs(SYSTEM_LOG_LIMIT, { loading: false })])
    throwIfEverythingFailed(results)
  }

  const refreshCoreSilently = async (): Promise<void> => {
    try {
      await refreshCore()
    } catch {
      // Background refresh is best-effort.
    }
  }

  const refreshProxyStatusSilently = async (): Promise<void> => {
    try {
      await refreshProxyStatus()
    } catch {
      // Background refresh is best-effort.
    }
  }

  const refreshUsageSnapshotSilently = async (): Promise<void> => {
    try {
      await refreshUsageSnapshot()
    } catch {
      // Background refresh is best-effort.
    }
  }

  const refreshAccountsSnapshotSilently = async (): Promise<void> => {
    try {
      await refreshState()
    } catch {
      // Background refresh is best-effort.
    }
  }

  const refreshActiveTabSilently = async (activeTab = get(shell).activeTab): Promise<void> => {
    if (!isDocumentVisible()) {
      return
    }

    switch (activeTab) {
      case 'dashboard':
        await refreshCoreSilently()
        return
      case 'accounts':
        await refreshAccountsSnapshotSilently()
        return
      case 'api-router':
        await refreshProxyStatusSilently()
        return
      case 'usage':
        await refreshUsageSnapshotSilently()
        return
      default:
        return
    }
  }

  const scheduleProxyAutostartRetries = (): void => {
    clearProxyAutostartRetries()

    AUTO_START_PROXY_RETRY_DELAYS_MS.forEach((delay, index) => {
      const timer = window.setTimeout(() => {
        if (!get(shell).waitingForProxyAutostart) {
          return
        }

        void (async () => {
          await refreshCoreSilently()

          const currentShell = get(shell)
          const proxyRunning = currentShell.proxyStatus?.running ?? currentShell.state?.proxyRunning ?? false
          if (proxyRunning || index === AUTO_START_PROXY_RETRY_DELAYS_MS.length - 1) {
            cancelProxyAutostartWait()
          }
        })()
      }, delay)

      proxyAutostartRetryTimers.push(timer)
    })
  }

  const maybeWaitForProxyAutostart = (): void => {
    const currentShell = get(shell)
    const state = currentShell.state
    const proxyRunning = currentShell.proxyStatus?.running ?? state?.proxyRunning ?? false
    const shouldWait = state?.autoStartProxy === true && !proxyRunning

    if (!shouldWait) {
      cancelProxyAutostartWait()
      return
    }

    patchShell({ waitingForProxyAutostart: true })
    scheduleProxyAutostartRetries()
  }

  const startProxyHeartbeat = (): void => {
    clearProxyHeartbeat()
    if (typeof window === 'undefined') {
      return
    }

    proxyHeartbeatTimer = window.setInterval(() => {
      if (!isDocumentVisible()) {
        return
      }
      void refreshProxyStatusSilently()
    }, PROXY_HEARTBEAT_INTERVAL_MS)
  }

  const startActiveTabRefreshLoop = (): void => {
    clearActiveTabRefreshTimer()
    if (typeof window === 'undefined') {
      return
    }

    activeTabRefreshTimer = window.setInterval(() => {
      void refreshActiveTabSilently()
    }, ACTIVE_TAB_REFRESH_INTERVAL_MS)
  }

  const handleSecondInstanceNotice = (payload: unknown): void => {
    const record = typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {}
    const message =
      typeof record.message === 'string' && record.message.trim().length > 0
        ? record.message.trim()
        : 'CLIRO was already running. Restored the existing window.'
    toastStore.push('info', 'App Reopened', message)
  }

  const handleCloseRequested = (): void => {
    if (get(overlays).showClosePrompt) {
      void handleConfirmQuit()
      return
    }

    resetClosePromptFlow()
    patchOverlays({ showClosePrompt: true })
    void (async () => {
      await refreshStateForClosePrompt()
    })()
  }

  const handleWindowRestored = (): void => {
    void refreshCoreSilently()
    void refreshActiveTabSilently()
  }

  const handleProxyStateChanged = (): void => {
    cancelProxyAutostartWait()
    void refreshCoreSilently()
  }

  const handleTrayStateChanged = (): void => {
    void refreshStateSafe()
  }

  function runAppAction<T>(options: AppActionOptions<T> & { rethrow: true }): Promise<T>
  function runAppAction<T>(options: AppActionOptions<T>): Promise<T | undefined>
  async function runAppAction<T>(options: AppActionOptions<T>): Promise<T | undefined> {
    const { action, refresh, successToast, onSuccess, onError, errorTitle, rethrow = false } = options

    try {
      const result = await action()

      if (refresh) {
        await refresh()
      }

      if (onSuccess) {
        await onSuccess(result)
      }

      if (successToast) {
        notifySuccess(successToast.title, successToast.message)
      }

      return result
    } catch (error) {
      if (onError) {
        await onError(error)
      } else if (errorTitle) {
        notifyError(errorTitle, error)
      }

      if (rethrow) {
        throw error
      }

      return undefined
    }
  }

  const runProxyAction = async (title: string, action: () => Promise<void>, doneMessage: string): Promise<void> => {
    cancelProxyAutostartWait()
    await withBusyFlag(
      (busy) => patchShell({ proxyBusy: busy }),
      async () => {
        await runAppAction({
          action,
          refresh: refreshProxySnapshot,
          successToast: {
            title,
            message: doneMessage
          },
          onError: async (error) => {
            notifyError(title, error)
            await refreshProxySnapshotSafe()
          }
        })
      }
    )
  }

  const handleToggleAccount = async (accountId: string, enabled: boolean): Promise<void> => {
    await withAccountBusy(accountId, async () => {
      await runAppAction({
        action: () => accountsApi.runAccountAction({ accountId, action: enabled ? 'enable' : 'disable' }),
        refresh: refreshAccountsState,
        successToast: {
          title: 'Account Updated',
          message: `Account ${enabled ? 'enabled' : 'disabled'} successfully.`
        },
        errorTitle: 'Toggle Account Failed'
      })
    })
  }

  const handleBulkToggleAccounts = async (accountIds: string[], enabled: boolean): Promise<void> => {
    const uniqueIDs = normalizeAccountIDs(accountIds)
    if (uniqueIDs.length === 0) {
      return
    }

    const result = await runBulkAccountMutation(uniqueIDs, (accountId) =>
      accountsApi.runAccountAction({ accountId, action: enabled ? 'enable' : 'disable' })
    )
    await refreshAccountsState()

    const successCount = result.total - result.failures.length
    if (successCount > 0) {
      notifySuccess('Bulk Account Update', `${successCount} account(s) ${enabled ? 'enabled' : 'disabled'}.`)
    }
    if (result.failures.length > 0) {
      throw new Error(`${result.failures.length} account(s) failed to update.`)
    }
  }

  const handleRefreshAccountWithQuota = async (accountId: string): Promise<void> => {
    await withAccountBusy(accountId, async () => {
      await runAppAction({
        action: () => accountsApi.runAccountAction({ accountId, action: 'refresh-with-quota' }),
           successToast: {
             title: 'Account Refreshed',
             message: 'Token and quota refreshed successfully.'
   },
        errorTitle: 'Refresh Account Failed'
      })

      await refreshAccountsStateSafe()
    })
  }

  const handleDeleteAccount = async (accountId: string): Promise<void> => {
    await withAccountBusy(accountId, async () => {
      await runAppAction({
        action: () => accountsApi.runAccountAction({ accountId, action: 'delete' }),
        refresh: refreshAccountsState,
        successToast: {
          title: 'Account Deleted',
          message: 'Account removed from local storage.'
        },
        errorTitle: 'Delete Account Failed'
      })
    })
  }

  const handleBulkDeleteAccounts = async (accountIds: string[]): Promise<void> => {
    const uniqueIDs = normalizeAccountIDs(accountIds)
    if (uniqueIDs.length === 0) {
      return
    }

    const result = await runBulkAccountMutation(uniqueIDs, (accountId) =>
      accountsApi.runAccountAction({ accountId, action: 'delete' })
    )
    await refreshAccountsState()

    const successCount = result.total - result.failures.length
    if (successCount > 0) {
      notifySuccess('Bulk Delete Complete', `${successCount} account(s) deleted.`)
    }
    if (result.failures.length > 0) {
      throw new Error(`${result.failures.length} account(s) failed to delete.`)
    }
  }

  const handleImportAccounts = async (importedAccounts: Account[]): Promise<number> => {
    return runAppAction<number>({
      action: () => accountsApi.importAccounts(importedAccounts),
      refresh: refreshAccountsState,
      onSuccess: (importedCount) => {
        notifySuccess('Accounts Imported', `${importedCount} account(s) imported successfully.`)
      },
      rethrow: true
    })
  }

  const handleRefreshAllQuotas = async (): Promise<void> => {
    await withBusyFlag(setRefreshingAllQuotas, async () => {
      await runAppAction({
        action: () => accountsApi.runQuotaAction({ action: 'refresh-all' }),
        successToast: {
          title: 'Quotas Refreshed',
          message: 'Eligible account quota snapshots were refreshed. Exhausted accounts still waiting for reset were skipped.'
        },
        errorTitle: 'Refresh All Quotas Failed'
      })

      await refreshAccountsStateSafe()
    })
  }

  const handleForceRefreshAllQuotas = async (): Promise<void> => {
    await withBusyFlag(setRefreshingAllQuotas, async () => {
      await runAppAction({
        action: () => accountsApi.runQuotaAction({ action: 'force-refresh-all' }),
        successToast: {
          title: 'Quotas Force Refreshed',
          message: 'Every configured account quota snapshot was refreshed, including accounts normally skipped by smart refresh.'
        },
        errorTitle: 'Force Refresh All Quotas Failed'
      })

      await refreshAccountsStateSafe()
    })
  }

  const syncSuccessTitleByTarget: Record<SyncTargetID, string> = {
    'kilo-cli': 'Kilo CLI Synced',
    'opencode-cli': 'Opencode Synced',
    'codex-cli': 'Codex CLI Synced'
  }

  const syncErrorTitleByTarget: Record<SyncTargetID, string> = {
    'kilo-cli': 'Kilo CLI Sync Failed',
    'opencode-cli': 'Opencode Sync Failed',
    'codex-cli': 'Codex CLI Sync Failed'
  }

  const handleSyncAccountAuth = async (accountId: string, target: SyncTargetID): Promise<AccountSyncResult> => {
    return withAccountBusy(accountId, async () => {
      return runAppAction<AccountSyncResult>({
        action: () => accountsApi.syncAccountAuth(accountId, target),
        onSuccess: (result) => {
          notifySuccess(syncSuccessTitleByTarget[target], `Auth file updated at ${result.targetPath}.`)
        },
        errorTitle: syncErrorTitleByTarget[target],
        rethrow: true
      })
    })
  }

	const authController = createAuthSessionController({
		getSession: (sessionId) => accountsAuthApi.getAuthSession('codex', sessionId),
    onSession: (session) => {
      patchShell({ authSession: session })
    },
    onSuccess: async () => {
      await refreshAccountsState()
    },
    onSessionError: (session) => {
      toastStore.push('error', 'Authentication Failed', session.error || 'OAuth flow returned an error.')
    },
    onPollingError: (error) => {
      notifyError('Authentication Poll Failed', error)
      const currentSession = get(shell).authSession
      if (currentSession) {
        patchShell({ authSession: { ...currentSession, status: 'error' } })
      }
    }
  })

  const kiroAuthController = createAuthSessionController({
    getSession: (sessionId) => accountsAuthApi.getAuthSession('kiro', sessionId),
    onSession: (session) => {
      patchShell({ kiroAuthSession: session })
    },
    onSuccess: async (session) => {
      await refreshAccountsState()
      notifySuccess('Kiro Account Connected', session.email ? `Connected ${session.email}.` : 'KiroAI account connected successfully.')
    },
    onSessionError: (session) => {
      const fallback = session.authMethod === 'social' ? 'Social login returned an error.' : 'Device authorization returned an error.'
      toastStore.push('error', 'Kiro Authentication Failed', session.error || fallback)
    },
    onPollingError: (error) => {
      notifyError('Kiro Authentication Poll Failed', error)
      const currentSession = get(shell).kiroAuthSession
      if (currentSession) {
        patchShell({ kiroAuthSession: { ...currentSession, status: 'error' } })
      }
    }
  })

  const handleStartConnect = async (): Promise<void> => {
    const currentSession = get(shell).authSession
    if (currentSession?.sessionId && currentSession.status === 'pending') {
      authController.start(currentSession.sessionId)
      return
    }

    await withBusyFlag(setAuthWorking, async () => {
		const started = await runAppAction({
			action: () => accountsAuthApi.startAuth('codex'),
			errorTitle: 'Authentication Start Failed'
		})

      if (!started) {
        return
      }

      patchShell({
        authSession: {
          sessionId: started.sessionId,
          authUrl: started.authUrl,
          callbackUrl: started.callbackUrl,
          status: started.status
        }
      })
      authController.start(started.sessionId)
      toastStore.push('info', 'Authentication Started', 'Open the provided auth link to complete the OAuth callback flow.')
    })
  }

  const handleStartKiroConnect = async (method: 'device' | 'google' | 'github' = 'device'): Promise<void> => {
    const currentSession = get(shell).kiroAuthSession

    if (currentSession && currentSession.status !== 'pending') {
      patchShell({ kiroAuthSession: null })
    }

    const pendingSession = get(shell).kiroAuthSession
    if (pendingSession?.sessionId && pendingSession.status === 'pending') {
      kiroAuthController.start(pendingSession.sessionId)
      return
    }

    await withBusyFlag(setAuthWorking, async () => {
		const started = await runAppAction<Awaited<ReturnType<typeof accountsAuthApi.startAuth>>>({
			action: () => (method === 'device' ? accountsAuthApi.startAuth('kiro') : accountsAuthApi.startSocialAuth('kiro', method)),
			errorTitle: 'Kiro Authentication Start Failed',
			rethrow: true
		})

      patchShell({
        kiroAuthSession: {
          sessionId: started.sessionId,
          authUrl: started.authUrl,
          verificationUrl: started.verificationUrl,
          userCode: started.userCode,
          expiresAt: started.expiresAt,
          status: started.status,
          authMethod: started.authMethod,
          provider: started.provider
        }
      })
      kiroAuthController.start(started.sessionId)
      if (method === 'device') {
        toastStore.push('info', 'Kiro Device Auth Started', 'Open AWS Builder ID and enter the displayed device code.')
      } else {
        const providerLabel = method === 'google' ? 'Google' : 'GitHub'
        toastStore.push('info', 'Kiro Social Auth Started', `Open the ${providerLabel} sign-in link to connect your Kiro account.`)
      }
    })
  }

  const handleCancelConnect = async (): Promise<void> => {
    const currentSession = get(shell).authSession
    if (!currentSession?.sessionId) {
      return
    }

	await withBusyFlag(setAuthWorking, async () => {
		await runAppAction({
			action: () => accountsAuthApi.cancelAuth('codex', currentSession.sessionId),
        onSuccess: () => {
          authController.stop()
          patchShell({ authSession: null })
          notifySuccess('Authentication Cancelled', 'Connect flow stopped.')
        },
        errorTitle: 'Cancel Authentication Failed'
      })
    })
  }

  const handleCancelKiroConnect = async (): Promise<void> => {
    const currentSession = get(shell).kiroAuthSession
    if (!currentSession?.sessionId) {
      return
    }

	await withBusyFlag(setAuthWorking, async () => {
		await runAppAction({
			action: () => accountsAuthApi.cancelAuth('kiro', currentSession.sessionId),
        onSuccess: () => {
          kiroAuthController.stop()
          patchShell({ kiroAuthSession: null })
          notifySuccess('Kiro Authentication Cancelled', 'Kiro device authorization stopped.')
        },
        errorTitle: 'Cancel Kiro Authentication Failed'
      })
    })
  }

  const handleOpenExternalURL = async (url: string): Promise<void> => {
    if (!url || url.trim().length === 0) {
      return
    }

    await runAppAction({
      action: () => systemApi.openExternalURL(url),
      errorTitle: 'Open URL Failed'
    })
  }

  const handleConfirmQuit = async (): Promise<void> => {
    resetClosePromptFlow({ hidePrompt: true })
    await runAppAction({
      action: () => systemApi.runAction('confirm-quit'),
      errorTitle: 'Close App Failed'
    })
  }

  const handleHideToTray = async (): Promise<void> => {
    await runAppAction({
      action: () => systemApi.runAction('hide-to-tray'),
      onSuccess: () => {
        resetClosePromptFlow({ hidePrompt: true })
      },
      errorTitle: 'Minimize to Tray Failed'
    })
  }

  const handleSetAllowLAN = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Proxy Network Mode Updated',
      async () => {
        await routerApi.updateProxySettings({ allowLan: enabled })
      },
      enabled ? 'Proxy now accepts LAN traffic.' : 'Proxy now listens only on localhost.'
    )
  }

  const handleSetAutoStartProxy = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Proxy Startup Updated',
      async () => {
        await routerApi.updateProxySettings({ autoStartProxy: enabled })
      },
      enabled ? 'Proxy will start automatically on app launch.' : 'Proxy autostart disabled.'
    )
  }

  const handleSetProxyAPIKey = async (apiKey: string): Promise<void> => {
    await runProxyAction('Proxy API Key Updated', async () => {
      await routerApi.updateProxySettings({ proxyApiKey: apiKey })
    }, 'Proxy API key has been updated.')
  }

  const handleRegenerateProxyAPIKey = async (): Promise<string> => {
    return withBusyFlag(
      (busy) => patchShell({ proxyBusy: busy }),
      async () => {
        const result = await runAppAction<{ generatedApiKey?: string }>({
          action: () => routerApi.updateProxySettings({ regenerateApiKey: true }),
          refresh: async () => {
            await Promise.all([refreshState(), refreshProxyStatus()])
          },
          successToast: {
            title: 'Proxy API Key Regenerated',
            message: 'A new API key has been generated for proxy access.'
          },
          errorTitle: 'Regenerate API Key Failed',
          rethrow: true
        })

        const regenerated = result.generatedApiKey || ''
        if (!regenerated) {
          throw new Error('Regenerated API key was not returned by backend.')
        }
        return regenerated
      }
    )
  }

  const handleSetAuthorizationMode = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Authorization Mode Updated',
      async () => {
        await routerApi.updateProxySettings({ authorizationMode: enabled })
      },
      enabled ? 'All proxy routes now require the configured API key.' : 'Proxy routes are open again unless a client sends its own API key header.'
    )
  }

  const handleSetSchedulingMode = async (mode: string): Promise<void> => {
    const label = mode
      .split('_')
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ')

    await runProxyAction('Scheduling Mode Updated', async () => {
      await routerApi.updateProxySettings({ schedulingMode: mode })
    }, `${label} routing mode is now active.`)
  }

  const handleSetModelAliases = async (aliases: Record<string, string>): Promise<void> => {
    await runProxyAction('Model Aliases Updated', () => routerApi.setModelAliases(aliases), `Model aliases updated (${Object.keys(aliases).length} mappings).`)
  }

  const handleSetCloudflaredConfig = async (mode: string, token: string, useHttp2: boolean): Promise<void> => {
    await withBusyFlag(
      (busy) => patchShell({ proxyBusy: busy }),
      async () => {
        await runAppAction({
          action: () => routerApi.updateCloudflaredSettings({ mode: mode as 'quick' | 'auth', token, useHttp2 }),
          refresh: refreshCloudflaredStatus,
          onError: async (error) => {
            notifyError('Cloudflared Config Failed', error)
            await refreshProxySnapshotSafe()
          }
        })
      }
    )
  }

  const handleInstallCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Installed', async () => {
      await routerApi.runCloudflaredAction('install')
    }, 'Cloudflared binary downloaded and verified.')
  }

  const handleStartCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Started', async () => {
      await routerApi.runCloudflaredAction('start')
    }, 'Public tunnel started for the local proxy.')
  }

  const handleStopCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Stopped', async () => {
      await routerApi.runCloudflaredAction('stop')
    }, 'Public tunnel stopped.')
  }

  const handleSyncCLIConfig = async (appId: CliSyncAppID, model: string): Promise<CliSyncResult> => {
    return runAppAction<CliSyncResult>({
      action: () => routerApi.runCliSync({ target: appId, model }),
      onSuccess: (result) => {
        const targetPath = result.files[0]?.path || result.currentBaseUrl || 'the target config'
        notifySuccess(`${result.label} Synced`, `Configuration updated at ${targetPath}.`)
      },
      errorTitle: 'CLI Sync Failed',
      rethrow: true
    })
  }

  const handleExportBackup = async (): Promise<void> => {
    const currentShell = get(shell)
    const payload: BackupPayload = {
      version: 1,
      exportedAt: new Date().toISOString(),
      state: currentShell.state,
      accounts: currentShell.accounts
    }
    downloadJSONFile(payload, `cliro-backup-${Date.now()}.json`)
    notifySuccess('Backup Exported', 'Configuration and account snapshot exported.')
  }

  const handleRestoreBackup = async (
    payload: BackupPayload,
    onProgress?: (progress: RestoreProgress) => void
  ): Promise<void> => {
    const normalizedPayload = validateBackupPayload(payload)
    assertBackupPayloadRestorable(normalizedPayload)

    const backupState = normalizedPayload.state
    const backupAccounts = normalizedPayload.accounts
    const restoreSteps: Array<{ label: string; run: () => Promise<void> }> = []

    if (backupState) {
      restoreSteps.push({
        label: 'Scheduling mode',
        run: async () => {
          await routerApi.updateProxySettings({ schedulingMode: String(backupState.schedulingMode || 'balance') })
        }
      })
      restoreSteps.push({
        label: 'Authorization mode',
        run: async () => {
          await routerApi.updateProxySettings({ authorizationMode: backupState.authorizationMode ?? false })
        }
      })
      restoreSteps.push({
        label: 'LAN visibility',
        run: async () => {
          await routerApi.updateProxySettings({ allowLan: backupState.allowLan ?? false })
        }
      })
      restoreSteps.push({
        label: 'Proxy auto-start',
        run: async () => {
          await routerApi.updateProxySettings({ autoStartProxy: backupState.autoStartProxy ?? true })
        }
      })
      restoreSteps.push({
        label: 'Proxy port',
        run: async () => {
          await routerApi.updateProxySettings({ port: parseBackupNumber(backupState.proxyPort, 8095) })
        }
      })
    }

    if (backupAccounts.length > 0) {
      restoreSteps.push({
        label: `Import ${backupAccounts.length} account(s)`,
        run: async () => {
          await accountsApi.importAccounts(backupAccounts)
        }
      })
    }

    const reportProgress = (index: number, step: string): void => {
      onProgress?.({
        step,
        index,
        total: restoreSteps.length
      })
    }

    try {
      for (let index = 0; index < restoreSteps.length; index++) {
        const step = restoreSteps[index]
        reportProgress(index + 1, step.label)
        try {
          await step.run()
        } catch (error) {
          throw new Error(`Restore step ${index + 1}/${restoreSteps.length} failed (${step.label}): ${getErrorMessage(error, 'Unknown restore error.')}`)
        }
      }
    } finally {
      try {
        await refreshCore()
        maybeWaitForProxyAutostart()
        await refreshLogs(SYSTEM_LOG_LIMIT)
        bindLogsSubscription(SYSTEM_LOG_LIMIT)
      } catch (error) {
        notifyError('Refresh Snapshot Failed', error)
      }
    }

    notifySuccess('Backup Restored', 'Settings and accounts were restored from backup payload.')
  }

  const handleClearLogs = async (): Promise<void> => {
    await withBusyFlag(
      (busy) => patchShell({ clearingLogs: busy }),
      async () => {
        patchShell({ logs: [] })

        try {
          await logsApi.clearLogs()
          await refreshLogs(SYSTEM_LOG_LIMIT)
          notifySuccess('Logs Cleared', 'System logs were cleared successfully.')
        } catch (error) {
          notifyError('Clear Logs Failed', error)
          await refreshLogs(SYSTEM_LOG_LIMIT)
        }
      }
    )
  }

  const handleOpenDataDir = async (): Promise<void> => {
    await runAppAction({
      action: () => systemApi.runAction('open-data-dir'),
      onSuccess: () => {
        toastStore.push('info', 'Data Folder', 'Opened local CLIRO data folder.')
      },
      errorTitle: 'Open Data Folder Failed'
    })
  }

  const checkForUpdates = async (): Promise<void> => {
    try {
      const result = await systemApi.getUpdateInfo()
      if (!result?.updateAvailable) {
        return
      }

      patchOverlays({
        updateInfo: result,
        showUpdatePrompt: true
      })
    } catch {
      // Update checks are best-effort and should not interrupt app startup.
    }
  }

  const bindLogsSubscription = (limit: number): void => {
    unsubscribeLogs?.()
    unsubscribeLogs = subscribeToRingLogs(
      () => get(shell).logs,
      (nextLogs) => {
        patchShell({ logs: nextLogs })
      },
      limit
    )
  }

  const bindRuntimeEvents = (): (() => void) => {
    return bindAppRuntimeEvents({
      onSecondInstanceNotice: handleSecondInstanceNotice,
      onCloseRequested: handleCloseRequested,
      onWindowRestored: handleWindowRestored,
      onProxyStateChanged: handleProxyStateChanged,
      onTrayStateChanged: handleTrayStateChanged
    })
  }

  const bindActivityEvents = (): (() => void) => {
    return bindAppActivityEvents({
      isDocumentVisible,
      onVisible: () => {
        void refreshProxyStatusSilently()
        void refreshActiveTabSilently()
      },
      onFocus: () => {
        void refreshProxyStatusSilently()
        void refreshActiveTabSilently()
      }
    })
  }

  const appActions: AppActions = {
    setActiveTab: (tabId) => {
      patchShell({ activeTab: tabId })
      void refreshActiveTabSilently(tabId)
    },
    dismissClosePrompt: () => {
      resetClosePromptFlow({ hidePrompt: true })
    },
    confirmQuit: handleConfirmQuit,
    hideToTray: handleHideToTray,
    dismissConfigurationErrorModal: () => {
      patchOverlays({ showConfigurationErrorModal: false })
    },
    dismissUpdatePrompt: () => {
      patchOverlays({ showUpdatePrompt: false })
    },
    openUpdateReleasePage: async () => {
      const releaseUrl = get(overlays).updateInfo?.releaseUrl || ''
      if (!releaseUrl) {
        return
      }

      await handleOpenExternalURL(releaseUrl)
    },
    openExternalURL: handleOpenExternalURL
  }

  const accountsActions: AccountsActions = {
    startConnect: handleStartConnect,
    cancelConnect: handleCancelConnect,
    startKiroConnect: handleStartKiroConnect,
    cancelKiroConnect: handleCancelKiroConnect,
    refreshAllQuotas: handleRefreshAllQuotas,
    forceRefreshAllQuotas: handleForceRefreshAllQuotas,
    toggleAccount: handleToggleAccount,
    bulkToggleAccounts: handleBulkToggleAccounts,
    bulkDeleteAccounts: handleBulkDeleteAccounts,
    importAccounts: handleImportAccounts,
    syncAccountAuth: handleSyncAccountAuth,
    refreshAccountWithQuota: handleRefreshAccountWithQuota,
    deleteAccount: handleDeleteAccount
  }

  const routerActions: RouterActions = {
    startProxy: () => runProxyAction('Proxy Started', () => routerApi.runProxyAction('start'), 'Proxy service started.'),
    stopProxy: () => runProxyAction('Proxy Stopped', () => routerApi.runProxyAction('stop'), 'Proxy service stopped.'),
    setProxyPort: (port) => runProxyAction('Proxy Port Updated', async () => {
      await routerApi.updateProxySettings({ port })
    }, `Proxy port set to ${port}.`),
    setAllowLAN: handleSetAllowLAN,
    setAutoStartProxy: handleSetAutoStartProxy,
    setProxyAPIKey: handleSetProxyAPIKey,
    regenerateProxyAPIKey: handleRegenerateProxyAPIKey,
    setAuthorizationMode: handleSetAuthorizationMode,
    setSchedulingMode: handleSetSchedulingMode,
    getModelAliases: routerApi.getModelAliases,
    setModelAliases: handleSetModelAliases,
    refreshProxyStatus: refreshProxyStatusSafe,
    refreshCloudflaredStatus: refreshCloudflaredStatusSafe,
    setCloudflaredConfig: handleSetCloudflaredConfig,
    installCloudflared: handleInstallCloudflared,
    startCloudflared: handleStartCloudflared,
    stopCloudflared: handleStopCloudflared,
    getCliSyncStatuses: routerApi.getCliSyncStatuses,
    getCliSyncFileContent: (appId, path) => routerApi.getCliSyncFile({ target: appId, path }),
    saveCliSyncFileContent: (appId, path, content) => routerApi.saveCliSyncFile({ target: appId, path, content }),
    syncCLIConfig: handleSyncCLIConfig
  }

  const logsActions: LogsActions = {
    refresh: refreshLogs,
    clear: handleClearLogs
  }

  const settingsActions: SettingsActions = {
    openDataDir: handleOpenDataDir,
    exportBackup: handleExportBackup,
    restoreBackup: handleRestoreBackup
  }

  return {
    shell,
    overlays,
    appActions,
    accountsActions,
    routerActions,
    logsActions,
    settingsActions,
    initialize: async () => {
      bootstrapHandle?.dispose()
      bootstrapHandle = await initializeAppBootstrap({
        bindRuntimeEvents,
        bindActivityEvents,
        startProxyHeartbeat,
        startActiveTabRefreshLoop,
        refreshCore: () => refreshCore({ loading: true }),
        maybeWaitForProxyAutostart,
        refreshLogs: () => refreshLogs(SYSTEM_LOG_LIMIT),
        bindLogsSubscription: () => bindLogsSubscription(SYSTEM_LOG_LIMIT),
        checkForUpdates,
        onInitializeError: (error) => {
          notifyError('Initial Load Failed', error)
        }
      })
    },
    destroy: () => {
      authController.stop()
      kiroAuthController.stop()
      bootstrapHandle?.dispose()
      bootstrapHandle = null
      stopPolling()
      cancelProxyAutostartWait()
      resetClosePromptFlow({ hidePrompt: true })
      unsubscribeLogs?.()
      unsubscribeLogs = null
    }
  }
}
