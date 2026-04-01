import { get, writable, type Readable } from 'svelte/store'
import { logsApi } from '@/app/api/logs-api'
import { systemApi } from '@/app/api/system-api'
import {
  assertBackupPayloadRestorable,
  parseBackupNumber,
  validateBackupPayload,
  type BackupPayload,
  type RestoreProgress
} from '@/app/lib/backup'
import type { AppTabId } from '@/app/lib/tabs'
import { subscribeToRingLogs } from '@/app/services/logs-subscription'
import { mapStartupWarnings, type StartupWarningEntry } from '@/app/services/startup-warnings'
import type { AppState, LogEntry, UpdateInfo } from '@/app/types'
import { accountsApi } from '@/features/accounts/api/accounts-api'
import { accountsAuthApi } from '@/features/accounts/api/auth-api'
import { createAuthSessionController } from '@/features/accounts/lib/auth-session'
import type {
  Account,
  AuthSession,
  CodexAuthSyncResult,
  KiloAuthSyncResult,
  KiroAuthSession,
  OpencodeAuthSyncResult
} from '@/features/accounts/types'
import { routerApi } from '@/features/router/api/router-api'
import type { CliSyncAppID, CliSyncResult, ProxyStatus } from '@/features/router/types'
import { downloadJSONFile } from '@/shared/lib/browser'
import { getErrorMessage } from '@/shared/lib/error'
import { toastStore } from '@/shared/stores/toast'
import { EventsOn } from '../../../wailsjs/runtime/runtime'

const SYSTEM_LOG_LIMIT = 500

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
  loadingLogs: boolean
  clearingLogs: boolean
  proxyBusy: boolean
  authWorking: boolean
  refreshingAllQuotas: boolean
  busyAccountIds: string[]
}

export interface AppOverlayState {
  showUpdatePrompt: boolean
  showConfigurationErrorModal: boolean
  startupWarnings: StartupWarningEntry[]
  updateInfo: UpdateInfo | null
}

export interface AppActions {
  setActiveTab: (tabId: AppTabId) => void
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
  syncCodexAccountToKiloAuth: (accountId: string) => Promise<KiloAuthSyncResult>
  syncCodexAccountToOpencodeAuth: (accountId: string) => Promise<OpencodeAuthSyncResult>
  syncCodexAccountToCodexCLI: (accountId: string) => Promise<CodexAuthSyncResult>
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
  loadingLogs: false,
  clearingLogs: false,
  proxyBusy: false,
  authWorking: false,
  refreshingAllQuotas: false,
  busyAccountIds: []
}

const initialOverlayState: AppOverlayState = {
  showUpdatePrompt: false,
  showConfigurationErrorModal: false,
  startupWarnings: [],
  updateInfo: null
}

export function createAppController(): AppController {
  const shell = writable<AppShellState>(initialShellState)
  const overlays = writable<AppOverlayState>(initialOverlayState)

  let startupWarningsShown = false
  let unsubscribeLogs: (() => void) | null = null
  let unsubscribeSecondInstance: (() => void) | null = null

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

  const refreshState = async (): Promise<void> => {
    const nextState = await systemApi.getState()
    patchShell({ state: nextState })
    syncStartupWarnings(nextState)
  }

  const refreshAccounts = async (): Promise<void> => {
    patchShell({ accounts: await accountsApi.getAccounts() })
  }

  const refreshAccountsState = async (): Promise<void> => {
    await Promise.all([refreshAccounts(), refreshState()])
  }

  const refreshAccountsStateSafe = async (): Promise<void> => {
    try {
      await refreshAccountsState()
    } catch (error) {
      notifyError('Refresh Snapshot Failed', error)
    }
  }

  const refreshProxyStatus = async (): Promise<void> => {
    patchShell({ proxyStatus: await routerApi.getProxyStatus() })
  }

  const refreshCloudflaredStatus = async (): Promise<void> => {
    patchShell({ proxyStatus: await routerApi.refreshCloudflaredStatus() })
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
    await Promise.all([refreshState(), refreshCloudflaredStatus()])
  }

  const refreshProxySnapshotSafe = async (): Promise<void> => {
    try {
      await refreshProxySnapshot()
    } catch (error) {
      notifyError('Refresh Snapshot Failed', error)
    }
  }

  const refreshLogs = async (limit = SYSTEM_LOG_LIMIT): Promise<void> => {
    patchShell({ loadingLogs: true })
    try {
      patchShell({ logs: await logsApi.getLogs(limit) })
    } finally {
      patchShell({ loadingLogs: false })
    }
  }

  const refreshCore = async (): Promise<void> => {
    patchShell({ loadingDashboard: true })
    try {
      const nextStatePromise = systemApi.getState()
      const nextAccountsPromise = accountsApi.getAccounts()

      let firstError: unknown = null
      let successCount = 0

      try {
        const nextState = await nextStatePromise
        patchShell({ state: nextState })
        syncStartupWarnings(nextState)
        successCount += 1
      } catch (error) {
        firstError = error
      }

      try {
        const nextAccounts = await nextAccountsPromise
        patchShell({ accounts: nextAccounts })
        successCount += 1
      } catch (error) {
        if (firstError === null) {
          firstError = error
        }
      }

      if (successCount === 0 && firstError !== null) {
        throw firstError
      }
    } finally {
      patchShell({ loadingDashboard: false })
    }
  }

  const handleSecondInstanceNotice = (payload: unknown): void => {
    const record = typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {}
    const message =
      typeof record.message === 'string' && record.message.trim().length > 0
        ? record.message.trim()
        : 'CLIro-Go was already running. Restored the existing window.'
    toastStore.push('info', 'App Reopened', message)
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
        action: () => accountsApi.toggleAccount(accountId, enabled),
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

    const result = await runBulkAccountMutation(uniqueIDs, (accountId) => accountsApi.toggleAccount(accountId, enabled))
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
        action: () => accountsApi.refreshAccountWithQuota(accountId),
        successToast: {
          title: 'Account Refreshed',
          message: 'Quota checked. Token refreshed only when expired.'
        },
        errorTitle: 'Refresh Account Failed'
      })

      await refreshAccountsStateSafe()
    })
  }

  const handleDeleteAccount = async (accountId: string): Promise<void> => {
    await withAccountBusy(accountId, async () => {
      await runAppAction({
        action: () => accountsApi.deleteAccount(accountId),
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

    const result = await runBulkAccountMutation(uniqueIDs, (accountId) => accountsApi.deleteAccount(accountId))
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
        action: () => accountsApi.refreshAllQuotas(),
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
        action: () => accountsApi.forceRefreshAllQuotas(),
        successToast: {
          title: 'Quotas Force Refreshed',
          message: 'Every configured account quota snapshot was refreshed, including accounts normally skipped by smart refresh.'
        },
        errorTitle: 'Force Refresh All Quotas Failed'
      })

      await refreshAccountsStateSafe()
    })
  }

  const handleSyncCodexAccountToKiloAuth = async (accountId: string): Promise<KiloAuthSyncResult> => {
    return withAccountBusy(accountId, async () => {
      return runAppAction<KiloAuthSyncResult>({
        action: () => accountsApi.syncCodexAccountToKiloAuth(accountId),
        onSuccess: (result) => {
          notifySuccess('Kilo CLI Synced', `Auth file updated at ${result.targetPath}.`)
        },
        errorTitle: 'Kilo CLI Sync Failed',
        rethrow: true
      })
    })
  }

  const handleSyncCodexAccountToCodexCLI = async (accountId: string): Promise<CodexAuthSyncResult> => {
    return withAccountBusy(accountId, async () => {
      return runAppAction<CodexAuthSyncResult>({
        action: () => accountsApi.syncCodexAccountToCodexCLI(accountId),
        onSuccess: (result) => {
          notifySuccess('Codex CLI Synced', `Auth file updated at ${result.targetPath}.`)
        },
        errorTitle: 'Codex CLI Sync Failed',
        rethrow: true
      })
    })
  }

  const handleSyncCodexAccountToOpencodeAuth = async (accountId: string): Promise<OpencodeAuthSyncResult> => {
    return withAccountBusy(accountId, async () => {
      return runAppAction<OpencodeAuthSyncResult>({
        action: () => accountsApi.syncCodexAccountToOpencodeAuth(accountId),
        onSuccess: (result) => {
          notifySuccess('Opencode Synced', `Auth file updated at ${result.targetPath}.`)
        },
        errorTitle: 'Opencode Sync Failed',
        rethrow: true
      })
    })
  }

  const authController = createAuthSessionController({
    getSession: (sessionId) => accountsAuthApi.getCodexAuthSession(sessionId),
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
    getSession: (sessionId) => accountsAuthApi.getKiroAuthSession(sessionId),
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
        action: () => accountsAuthApi.startCodexAuth(),
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
      const started = await runAppAction<Awaited<ReturnType<typeof accountsAuthApi.startKiroAuth>>>({
        action: () => (method === 'device' ? accountsAuthApi.startKiroAuth() : accountsAuthApi.startKiroSocialAuth(method)),
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
        action: () => accountsAuthApi.cancelCodexAuth(currentSession.sessionId),
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
        action: () => accountsAuthApi.cancelKiroAuth(currentSession.sessionId),
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

  const handleSetAllowLAN = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Proxy Network Mode Updated',
      () => routerApi.setAllowLAN(enabled),
      enabled ? 'Proxy now accepts LAN traffic.' : 'Proxy now listens only on localhost.'
    )
  }

  const handleSetAutoStartProxy = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Proxy Startup Updated',
      () => routerApi.setAutoStartProxy(enabled),
      enabled ? 'Proxy will start automatically on app launch.' : 'Proxy autostart disabled.'
    )
  }

  const handleSetProxyAPIKey = async (apiKey: string): Promise<void> => {
    await runProxyAction('Proxy API Key Updated', () => routerApi.setProxyAPIKey(apiKey), 'Proxy API key has been updated.')
  }

  const handleRegenerateProxyAPIKey = async (): Promise<string> => {
    return withBusyFlag(
      (busy) => patchShell({ proxyBusy: busy }),
      async () => {
        return runAppAction<string>({
          action: () => routerApi.regenerateProxyAPIKey(),
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
      }
    )
  }

  const handleSetAuthorizationMode = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Authorization Mode Updated',
      () => routerApi.setAuthorizationMode(enabled),
      enabled ? 'All proxy routes now require the configured API key.' : 'Proxy routes are open again unless a client sends its own API key header.'
    )
  }

  const handleSetSchedulingMode = async (mode: string): Promise<void> => {
    const label = mode
      .split('_')
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ')

    await runProxyAction('Scheduling Mode Updated', () => routerApi.setSchedulingMode(mode), `${label} routing mode is now active.`)
  }

  const handleSetModelAliases = async (aliases: Record<string, string>): Promise<void> => {
    await runProxyAction('Model Aliases Updated', () => routerApi.setModelAliases(aliases), `Model aliases updated (${Object.keys(aliases).length} mappings).`)
  }

  const handleSetCloudflaredConfig = async (mode: string, token: string, useHttp2: boolean): Promise<void> => {
    await withBusyFlag(
      (busy) => patchShell({ proxyBusy: busy }),
      async () => {
        await runAppAction({
          action: () => routerApi.setCloudflaredConfig(mode, token, useHttp2),
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
    await runProxyAction('Cloudflared Installed', () => routerApi.installCloudflared(), 'Cloudflared binary downloaded and verified.')
  }

  const handleStartCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Started', () => routerApi.startCloudflared(), 'Public tunnel started for the local proxy.')
  }

  const handleStopCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Stopped', () => routerApi.stopCloudflared(), 'Public tunnel stopped.')
  }

  const handleSyncCLIConfig = async (appId: CliSyncAppID, model: string): Promise<CliSyncResult> => {
    return runAppAction<CliSyncResult>({
      action: () => routerApi.syncCLIConfig(appId, model),
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
        run: () => routerApi.setSchedulingMode(String(backupState.schedulingMode || 'balance'))
      })
      restoreSteps.push({
        label: 'Authorization mode',
        run: () => routerApi.setAuthorizationMode(backupState.authorizationMode ?? false)
      })
      restoreSteps.push({
        label: 'LAN visibility',
        run: () => routerApi.setAllowLAN(backupState.allowLan ?? false)
      })
      restoreSteps.push({
        label: 'Proxy auto-start',
        run: () => routerApi.setAutoStartProxy(backupState.autoStartProxy ?? true)
      })
      restoreSteps.push({
        label: 'Proxy port',
        run: () => routerApi.setProxyPort(parseBackupNumber(backupState.proxyPort, 8095))
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
        void refreshProxyStatusSafe()
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
      action: () => systemApi.openDataDir(),
      onSuccess: () => {
        toastStore.push('info', 'Data Folder', 'Opened local CLIro-Go data folder.')
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

  const appActions: AppActions = {
    setActiveTab: (tabId) => {
      patchShell({ activeTab: tabId })
    },
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
    syncCodexAccountToKiloAuth: handleSyncCodexAccountToKiloAuth,
    syncCodexAccountToOpencodeAuth: handleSyncCodexAccountToOpencodeAuth,
    syncCodexAccountToCodexCLI: handleSyncCodexAccountToCodexCLI,
    refreshAccountWithQuota: handleRefreshAccountWithQuota,
    deleteAccount: handleDeleteAccount
  }

  const routerActions: RouterActions = {
    startProxy: () => runProxyAction('Proxy Started', routerApi.startProxy, 'Proxy service started.'),
    stopProxy: () => runProxyAction('Proxy Stopped', routerApi.stopProxy, 'Proxy service stopped.'),
    setProxyPort: (port) => runProxyAction('Proxy Port Updated', () => routerApi.setProxyPort(port), `Proxy port set to ${port}.`),
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
    getCliSyncFileContent: routerApi.getCliSyncFileContent,
    saveCliSyncFileContent: routerApi.saveCliSyncFileContent,
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
      unsubscribeSecondInstance = EventsOn('app:second-instance', handleSecondInstanceNotice)

      try {
        await refreshCore()
        void refreshProxyStatusSafe()
        await refreshLogs(SYSTEM_LOG_LIMIT)
        bindLogsSubscription(SYSTEM_LOG_LIMIT)
        await checkForUpdates()
      } catch (error) {
        notifyError('Initial Load Failed', error)
      }
    },
    destroy: () => {
      authController.stop()
      kiroAuthController.stop()
      unsubscribeLogs?.()
      unsubscribeLogs = null
      unsubscribeSecondInstance?.()
      unsubscribeSecondInstance = null
    }
  }
}
