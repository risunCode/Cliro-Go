<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import AppHeader from '@/components/common/AppHeader.svelte'
  import AppFooter from '@/components/common/AppFooter.svelte'
  import ConfigurationErrorModal from '@/components/common/ConfigurationErrorModal.svelte'
  import ToastViewport from '@/components/common/ToastViewport.svelte'
  import UpdatePromptModal from '@/components/common/UpdatePromptModal.svelte'
  import DashboardTab from '@/tabs/DashboardTab.svelte'
  import AccountsTab from '@/tabs/AccountsTab.svelte'
  import ApiRouterTab from '@/tabs/ApiRouterTab.svelte'
  import UsageTab from '@/tabs/UsageTab.svelte'
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
    type CliSyncAppID,
    type CliSyncResult,
    type KiroAuthSession,
    type LogEntry,
    type ProxyStatus,
    type KiloAuthSyncResult,
    type OpencodeAuthSyncResult,
    type CodexAuthSyncResult,
    type UpdateInfo
  } from '@/services/wails-api'
  import { createAuthSessionController } from '@/services/auth-session'
  import { subscribeToRingLogs } from '@/services/logs-subscription'
  import type { AppTabId } from '@/utils/tabs'
  import { downloadJSONFile } from '@/utils/browser'
  import { EventsOn } from '../wailsjs/runtime/runtime'

  let activeTab: AppTabId = 'dashboard'

  let state: AppState | null = null
  let accounts: Account[] = []
  let proxyStatus: ProxyStatus | null = null
  let logs: LogEntry[] = []

  let loadingDashboard = false
  let loadingLogs = false
  let clearingLogs = false
  let proxyBusy = false
  let authWorking = false
  let refreshingAllQuotas = false
  let busyAccountIds: string[] = []

  let authSession: AuthSession | null = null
  let kiroAuthSession: KiroAuthSession | null = null
  let updateInfo: UpdateInfo | null = null
  let showUpdatePrompt = false
  let showConfigurationErrorModal = false
  let startupWarningsShown = false
  let startupWarnings: Array<{ code: string; filePath: string; backupPath?: string; message: string }> = []

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

  interface BackupPayload {
    version: number
    exportedAt: string
    state: AppState | null
    accounts: Account[]
  }

  interface RestoreProgress {
    step: string
    index: number
    total: number
  }

  const markAccountBusy = (accountId: string, busy: boolean): void => {
    if (busy) {
      if (!busyAccountIds.includes(accountId)) {
        busyAccountIds = [...busyAccountIds, accountId]
      }
      return
    }
    busyAccountIds = busyAccountIds.filter((item) => item !== accountId)
  }

  const setAuthWorking = (busy: boolean): void => {
    authWorking = busy
  }

  const setRefreshingAllQuotas = (busy: boolean): void => {
    refreshingAllQuotas = busy
  }

  const normalizeAccountIDs = (accountIds: string[]): string[] => {
    return [...new Set(accountIds.map((id) => id.trim()).filter((id) => id.length > 0))]
  }

  const currentBatchBehavior = (): BatchBehaviorMode => {
    return state?.batchBehavior === 'sequential' ? 'sequential' : 'parallel'
  }

  const runBulkAccountMutation = async (
    accountIds: string[],
    action: (accountId: string) => Promise<void>,
    behavior: BatchBehaviorMode = currentBatchBehavior()
  ): Promise<BulkMutationResult> => {
    if (behavior === 'parallel') {
      const settled = await Promise.allSettled(accountIds.map((accountId) => action(accountId)))
      const failures = settled
        .map((result, index) => {
          if (result.status === 'fulfilled') {
            return ''
          }
          return accountIds[index]
        })
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

  const refreshState = async (): Promise<void> => {
    const nextState = await appService.getState()
    state = nextState
    syncStartupWarnings(nextState)
  }

  const refreshAccounts = async (): Promise<void> => {
    accounts = await appService.getAccounts()
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
    proxyStatus = await appService.getProxyStatus()
  }

  const refreshCloudflaredStatus = async (): Promise<void> => {
    proxyStatus = await appService.refreshCloudflaredStatus()
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
    await Promise.all([refreshState(), refreshProxyStatus()])
  }

  const refreshProxySnapshotSafe = async (): Promise<void> => {
    try {
      await refreshProxySnapshot()
    } catch (error) {
      notifyError('Refresh Snapshot Failed', error)
    }
  }

  const refreshLogs = async (limit = 400): Promise<void> => {
    loadingLogs = true
    try {
      logs = await appService.getLogs(limit)
    } finally {
      loadingLogs = false
    }
  }

  const refreshCore = async (): Promise<void> => {
    loadingDashboard = true
    try {
      const [nextState, nextAccounts, nextProxyStatus] = await Promise.all([
        appService.getState(),
        appService.getAccounts(),
        appService.getProxyStatus()
      ])
      state = nextState
      syncStartupWarnings(nextState)
      accounts = nextAccounts
      proxyStatus = nextProxyStatus
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

  const appendLocalLog = (level: 'info' | 'warn' | 'error', scope: string, message: string): void => {
    const entry: LogEntry = {
      timestamp: Date.now(),
      level,
      scope,
      message
    }
    logs = [...logs, entry].slice(-(state?.logMaxEntries || 1000))
  }

  const syncStartupWarnings = (nextState: AppState | null): void => {
    const nextWarnings = Array.isArray(nextState?.startupWarnings)
      ? nextState.startupWarnings.map((warning) => ({
          code: String(warning.code || 'startup_warning'),
          filePath: String(warning.filePath || '-'),
          backupPath: warning.backupPath ? String(warning.backupPath) : undefined,
          message: String(warning.message || 'Configuration was recovered during startup.')
        }))
      : []
    if (!startupWarningsShown && nextWarnings.length > 0) {
      startupWarnings = nextWarnings
      showConfigurationErrorModal = true
      startupWarningsShown = true
      return
    }

    if (!startupWarningsShown) {
      startupWarnings = nextWarnings
    }
  }

  const dismissConfigurationErrorModal = (): void => {
    showConfigurationErrorModal = false
  }

  const handleSecondInstanceNotice = (payload: unknown): void => {
    const record = typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {}
    const message = typeof record.message === 'string' && record.message.trim().length > 0
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

  const onTabChange = (event: CustomEvent<AppTabId>): void => {
    activeTab = event.detail
  }

  const runProxyAction = async (title: string, action: () => Promise<void>, doneMessage: string): Promise<void> => {
    await withBusyFlag(
      (busy) => {
        proxyBusy = busy
      },
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
        action: () => appService.toggleAccount(accountId, enabled),
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

    const result = await runBulkAccountMutation(uniqueIDs, (accountId) => appService.toggleAccount(accountId, enabled))
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
        action: () => appService.refreshAccountWithQuota(accountId),
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
        action: () => appService.deleteAccount(accountId),
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

    const result = await runBulkAccountMutation(uniqueIDs, (accountId) => appService.deleteAccount(accountId))
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
    const count = await runAppAction<number>({
      action: () => appService.importAccounts(importedAccounts),
      refresh: refreshAccountsState,
      onSuccess: (importedCount) => {
        notifySuccess('Accounts Imported', `${importedCount} account(s) imported successfully.`)
      },
      rethrow: true
    })

    return count
  }

  const handleRefreshAllQuotas = async (): Promise<void> => {
    await withBusyFlag(setRefreshingAllQuotas, async () => {
      await runAppAction({
        action: () => appService.refreshAllQuotas(),
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
        action: () => appService.forceRefreshAllQuotas(),
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
        action: () => appService.syncCodexAccountToKiloAuth(accountId),
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
        action: () => appService.syncCodexAccountToCodexCLI(accountId),
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
        action: () => appService.syncCodexAccountToOpencodeAuth(accountId),
        onSuccess: (result) => {
          notifySuccess('Opencode Synced', `Auth file updated at ${result.targetPath}.`)
        },
        errorTitle: 'Opencode Sync Failed',
        rethrow: true
      })
    })
  }

  const authController = createAuthSessionController({
    getSession: (sessionId) => appService.getCodexAuthSession(sessionId),
    onSession: (session) => {
      authSession = session
    },
    onSuccess: async () => {
      await refreshAccountsState()
    },
    onSessionError: (session) => {
      toastStore.push('error', 'Authentication Failed', session.error || 'OAuth flow returned an error.')
    },
    onPollingError: (error) => {
      notifyError('Authentication Poll Failed', error)
    }
  })

  const kiroAuthController = createAuthSessionController({
    getSession: (sessionId) => appService.getKiroAuthSession(sessionId),
    onSession: (session) => {
      kiroAuthSession = session
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
    }
  })

  const handleStartConnect = async (): Promise<void> => {
    if (authSession?.sessionId && authSession.status === 'pending') {
      authController.start(authSession.sessionId)
      return
    }

    await withBusyFlag(setAuthWorking, async () => {
      const started = await runAppAction({
        action: () => appService.startCodexAuth(),
        errorTitle: 'Authentication Start Failed'
      })

      if (!started) {
        return
      }

      authSession = {
        sessionId: started.sessionId,
        authUrl: started.authUrl,
        callbackUrl: started.callbackUrl,
        status: started.status
      }
      authController.start(started.sessionId)
      toastStore.push('info', 'Authentication Started', 'Open the provided auth link to complete the OAuth callback flow.')
    })
  }

  const handleStartKiroConnect = async (method: 'device' | 'google' | 'github' = 'device'): Promise<void> => {
    if (kiroAuthSession?.sessionId && kiroAuthSession.status === 'pending') {
      kiroAuthController.start(kiroAuthSession.sessionId)
      return
    }

    await withBusyFlag(setAuthWorking, async () => {
      const started = await runAppAction<Awaited<ReturnType<typeof appService.startKiroAuth>>>({
        action: () => (method === 'device' ? appService.startKiroAuth() : appService.startKiroSocialAuth(method)),
        errorTitle: 'Kiro Authentication Start Failed',
        rethrow: true
      })

      kiroAuthSession = {
        sessionId: started.sessionId,
        authUrl: started.authUrl,
        verificationUrl: started.verificationUrl,
        userCode: started.userCode,
        expiresAt: started.expiresAt,
        status: started.status,
        authMethod: started.authMethod,
        provider: started.provider
      }
      kiroAuthController.start(started.sessionId)
      if (method === 'device') {
        toastStore.push('info', 'Kiro Device Auth Started', 'Open AWS Builder ID and enter the displayed device code.')
      } else {
        const providerLabel = method === 'google' ? 'Google' : 'GitHub'
        toastStore.push('info', 'Kiro Social Auth Started', `Open the ${providerLabel} sign-in link to connect your Kiro account.`)
      }
    })
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

  const handleSetProxyAPIKey = async (apiKey: string): Promise<void> => {
    await runProxyAction('Proxy API Key Updated', () => appService.setProxyAPIKey(apiKey), 'Proxy API key has been updated.')
  }

  const handleRegenerateProxyAPIKey = async (): Promise<string> => {
    return withBusyFlag(
      (busy) => {
        proxyBusy = busy
      },
      async () => {
        const apiKey = await runAppAction<string>({
          action: () => appService.regenerateProxyAPIKey(),
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

        return apiKey
      }
    )
  }

  const handleSetAuthorizationMode = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Authorization Mode Updated',
      () => appService.setAuthorizationMode(enabled),
      enabled ? 'All proxy routes now require the configured API key.' : 'Proxy routes are open again unless a client sends its own API key header.'
    )
  }

  const handleSetSchedulingMode = async (mode: string): Promise<void> => {
    const label = mode
      .split('_')
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ')

    await runProxyAction('Scheduling Mode Updated', () => appService.setSchedulingMode(mode), `${label} routing mode is now active.`)
  }

  const handleSetCircuitBreaker = async (enabled: boolean): Promise<void> => {
    await runProxyAction(
      'Circuit Breaker Updated',
      () => appService.setCircuitBreaker(enabled),
      enabled
        ? 'Circuit breaker backoff is enabled for repeated failures.'
        : 'Circuit breaker backoff is disabled.'
    )
  }

  const handleSetCircuitSteps = async (steps: number[]): Promise<void> => {
    await runProxyAction(
      'Circuit Breaker Steps Updated',
      () => appService.setCircuitSteps(steps),
      `Backoff steps updated to ${steps.join('s, ')}s.`
    )
  }

  const handleSetCloudflaredConfig = async (mode: string, token: string, useHttp2: boolean): Promise<void> => {
    await withBusyFlag(
      (busy) => {
        proxyBusy = busy
      },
      async () => {
        await runAppAction({
          action: () => appService.setCloudflaredConfig(mode, token, useHttp2),
          refresh: refreshProxyStatus,
          onError: async (error) => {
            notifyError('Cloudflared Config Failed', error)
            await refreshProxySnapshotSafe()
          }
        })
      }
    )
  }

  const handleInstallCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Installed', () => appService.installCloudflared(), 'Cloudflared binary downloaded and verified.')
  }

  const handleStartCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Started', () => appService.startCloudflared(), 'Public tunnel started for the local proxy.')
  }

  const handleStopCloudflared = async (): Promise<void> => {
    await runProxyAction('Cloudflared Stopped', () => appService.stopCloudflared(), 'Public tunnel stopped.')
  }

  const handleSyncCLIConfig = async (appId: CliSyncAppID, model: string): Promise<CliSyncResult> => {
    return runAppAction<CliSyncResult>({
      action: () => appService.syncCLIConfig(appId, model),
      onSuccess: (result) => {
        const targetPath = result.files[0]?.path || result.currentBaseUrl || 'the target config'
        notifySuccess(`${result.label} Synced`, `Configuration updated at ${targetPath}.`)
      },
      errorTitle: 'CLI Sync Failed',
      rethrow: true
    })
  }

  const handleExportBackup = async (): Promise<void> => {
    const payload: BackupPayload = {
      version: 1,
      exportedAt: new Date().toISOString(),
      state,
      accounts
    }
    downloadJSONFile(payload, `cliro-backup-${Date.now()}.json`)
    notifySuccess('Backup Exported', 'Configuration and account snapshot exported.')
  }

  const parseRestoreNumber = (value: unknown, fallback: number): number => {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : fallback
  }

  const handleRestoreBackup = async (
    payload: BackupPayload,
    onProgress?: (progress: RestoreProgress) => void
  ): Promise<void> => {
    if (!payload || typeof payload !== 'object') {
      throw new Error('Backup payload is invalid.')
    }

    if (!Array.isArray(payload.accounts)) {
      throw new Error('Backup payload accounts must be an array.')
    }

    if (payload.state !== null && payload.state !== undefined && typeof payload.state !== 'object') {
      throw new Error('Backup payload state must be an object or null.')
    }

    const backupState = payload.state
    const backupAccounts = payload.accounts
    const restoreSteps: Array<{ label: string; run: () => Promise<void> }> = []

    if (backupState) {
      restoreSteps.push({
        label: 'Accounts auto-refresh policy',
        run: () => appService.setAutoRefreshQuotaPolicy(String(backupState.autoRefreshQuotaPolicy || 'single'))
      })
      restoreSteps.push({
        label: 'Accounts auto-refresh interval',
        run: () => appService.setAutoRefreshMinutes(parseRestoreNumber(backupState.autoRefreshMinutes, 5))
      })
      restoreSteps.push({
        label: 'Accounts visibility defaults',
        run: () => appService.setAccountsVisibilityDefaults(backupState.showExhaustedDefault ?? true, backupState.showDisabledDefault ?? true)
      })
      restoreSteps.push({
        label: 'Batch behavior',
        run: () => appService.setBatchBehavior(String(backupState.batchBehavior || 'parallel'))
      })
      restoreSteps.push({
        label: 'Log settings',
        run: () =>
          appService.setLogSettings(
            parseRestoreNumber(backupState.logMaxEntries, 1000),
            parseRestoreNumber(backupState.logFileSizeCapMb, 25),
            String(backupState.logVerbosity || 'info')
          )
      })
      restoreSteps.push({
        label: 'Import/export policy',
        run: () => appService.setImportExportPolicy(String(backupState.importExportPolicy || 'merge'))
      })
      restoreSteps.push({
        label: 'Advanced runtime settings',
        run: () =>
          appService.setAdvancedSettings(
            parseRestoreNumber(backupState.networkTimeoutSeconds, 60),
            parseRestoreNumber(backupState.retryLimit, 3),
            parseRestoreNumber(backupState.quotaRefreshWorkers, 4)
          )
      })
      restoreSteps.push({
        label: 'Scheduling mode',
        run: () => appService.setSchedulingMode(String(backupState.schedulingMode || 'balance'))
      })
      restoreSteps.push({
        label: 'Circuit breaker',
        run: () => appService.setCircuitBreaker(backupState.circuitBreaker ?? false)
      })
      restoreSteps.push({
        label: 'Circuit breaker steps',
        run: () => appService.setCircuitSteps(Array.isArray(backupState.circuitSteps) ? backupState.circuitSteps : [10, 30, 60])
      })
      restoreSteps.push({
        label: 'Authorization mode',
        run: () => appService.setAuthorizationMode(backupState.authorizationMode ?? false)
      })
      restoreSteps.push({
        label: 'LAN visibility',
        run: () => appService.setAllowLAN(backupState.allowLan ?? false)
      })
      restoreSteps.push({
        label: 'Proxy auto-start',
        run: () => appService.setAutoStartProxy(backupState.autoStartProxy ?? true)
      })
      restoreSteps.push({
        label: 'Proxy port',
        run: () => appService.setProxyPort(parseRestoreNumber(backupState.proxyPort, 8095))
      })
    }

    if (backupAccounts.length > 0) {
      restoreSteps.push({
        label: `Import ${backupAccounts.length} account(s)`,
        run: async () => {
          await appService.importAccounts(backupAccounts)
        }
      })
    }

    if (restoreSteps.length === 0) {
      throw new Error('Backup payload has no restorable state or account records.')
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
        const logLimit = state?.logMaxEntries || 1000
        await refreshLogs(logLimit)
        bindLogsSubscription(logLimit)
      } catch (error) {
        notifyError('Refresh Snapshot Failed', error)
      }
    }

    notifySuccess('Backup Restored', 'Settings and accounts were restored from backup payload.')
  }

  const handleCancelConnect = async (): Promise<void> => {
    if (!authSession?.sessionId) {
      return
    }

    await withBusyFlag(setAuthWorking, async () => {
      await runAppAction({
        action: () => appService.cancelCodexAuth(authSession.sessionId),
        onSuccess: () => {
          authController.stop()
          authSession = null
          notifySuccess('Authentication Cancelled', 'Connect flow stopped.')
        },
        errorTitle: 'Cancel Authentication Failed'
      })
    })
  }

  const handleCancelKiroConnect = async (): Promise<void> => {
    if (!kiroAuthSession?.sessionId) {
      return
    }

    await withBusyFlag(setAuthWorking, async () => {
      await runAppAction({
        action: () => appService.cancelKiroAuth(kiroAuthSession.sessionId),
        onSuccess: () => {
          kiroAuthController.stop()
          kiroAuthSession = null
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
      action: () => appService.openExternalURL(url),
      errorTitle: 'Open URL Failed'
    })
  }

  const handleClearLogs = async (): Promise<void> => {
    await withBusyFlag(
      (busy) => {
        clearingLogs = busy
      },
      async () => {
        logs = []

        try {
          await appService.clearLogs()
          const logLimit = state?.logMaxEntries || 1000
          await refreshLogs(logLimit)

          notifySuccess('Logs Cleared', 'System logs were cleared successfully.')
        } catch (error) {
          notifyError('Clear Logs Failed', error)
          await refreshLogs(state?.logMaxEntries || 1000)
        }
      }
    )
  }

  const handleOpenDataDir = async (): Promise<void> => {
    await runAppAction({
      action: () => appService.openDataDir(),
      onSuccess: () => {
        toastStore.push('info', 'Data Folder', 'Opened local CLIro-Go data folder.')
      },
      errorTitle: 'Open Data Folder Failed'
    })
  }

  const checkForUpdates = async (): Promise<void> => {
    try {
      const result = await appService.getUpdateInfo()
      if (!result.updateAvailable) {
        return
      }

      updateInfo = result
      showUpdatePrompt = true
    } catch {
      // Update checks are best-effort and should not interrupt app startup.
    }
  }

  const dismissUpdatePrompt = (): void => {
    showUpdatePrompt = false
  }

  const openUpdateReleasePage = async (): Promise<void> => {
    const releaseUrl = updateInfo?.releaseUrl || ''
    if (!releaseUrl) {
      return
    }

    await handleOpenExternalURL(releaseUrl)
  }

  let unsubscribeLogs: (() => void) | null = null
  let unsubscribeSecondInstance: (() => void) | null = null
  let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
  let autoRefreshIntervalMs = 0
  let autoRefreshInFlight = false
  let sessionAutoRefreshMinutes = 5
  let sessionAutoRefreshInitialized = false

  const isAutoRefreshQuotaCooldown = (account: Account): boolean => {
    const quotaStatus = String(account.quota?.status || '').trim().toLowerCase()
    if (quotaStatus !== 'exhausted' && quotaStatus !== 'empty') {
      return false
    }

    const now = Math.floor(Date.now() / 1000)
    const resetCandidates = Array.isArray(account.quota?.buckets)
      ? account.quota.buckets
          .map((bucket) => Number(bucket?.resetAt || 0))
          .filter((value) => Number.isFinite(value) && value > now)
      : []
    return resetCandidates.length > 0
  }

  const shouldSkipAutoRefreshAccount = (account: Account): boolean => {
    if (!account.enabled) {
      return true
    }
    if (account.banned) {
      return true
    }

    const healthState = String(account.healthState || '').trim().toLowerCase()
    if (healthState === 'banned' || healthState === 'disabled_durable') {
      return true
    }

    return isAutoRefreshQuotaCooldown(account)
  }

  const pickLastUsedAutoRefreshAccount = (items: Account[]): Account | null => {
    const eligible = items.filter((account) => !shouldSkipAutoRefreshAccount(account))
    if (eligible.length === 0) {
      return null
    }

    return [...eligible].sort((left, right) => {
      const leftLastUsed = Number(left.lastUsed || 0)
      const rightLastUsed = Number(right.lastUsed || 0)
      if (leftLastUsed !== rightLastUsed) {
        return rightLastUsed - leftLastUsed
      }
      return Number(right.updatedAt || 0) - Number(left.updatedAt || 0)
    })[0]
  }

  const runAutoRefreshTick = async (): Promise<void> => {
    const targetAccount = pickLastUsedAutoRefreshAccount(accounts)
    if (!targetAccount) {
      const message = 'Auto refresh skipped: no eligible last-used account.'
      toastStore.push('info', 'Auto Refresh Skipped', message)
      appendLocalLog('info', 'auto_refresh', message)
      return
    }

    const accountLabel = targetAccount.email || targetAccount.accountId || targetAccount.id
    const startMessage = `Auto refresh checking last-used account ${accountLabel}.`
    toastStore.push('info', 'Auto Refresh Running', startMessage)
    appendLocalLog('info', 'auto_refresh', startMessage)

    try {
      await appService.refreshQuota(targetAccount.id)
      await Promise.all([refreshAccounts(), refreshState(), refreshProxyStatus()])

      if (activeTab === 'system-logs' || activeTab === 'usage') {
        await refreshLogs(state?.logMaxEntries || 1000)
      }

      const successMessage = `Auto refresh completed for ${accountLabel}.`
      toastStore.push('success', 'Auto Refresh Complete', successMessage)
      appendLocalLog('info', 'auto_refresh', successMessage)
    } catch (error) {
      const message = `Auto refresh failed for ${accountLabel}: ${getErrorMessage(error, 'Unexpected refresh failure.')}`
      toastStore.push('error', 'Auto Refresh Failed', message)
      appendLocalLog('error', 'auto_refresh', message)
      throw error
    }
  }

  const handleSetSessionAutoRefreshMinutes = (minutes: number): void => {
    sessionAutoRefreshMinutes = minutes
  }

  const bindLogsSubscription = (limit: number): void => {
    unsubscribeLogs?.()
    unsubscribeLogs = subscribeToRingLogs(
      () => logs,
      (nextLogs) => {
        logs = nextLogs
      },
      limit
    )
  }

  const clearAutoRefreshTimer = (): void => {
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer)
      autoRefreshTimer = null
    }
  }

  const syncAutoRefreshTimer = (intervalMs: number): void => {
    if (intervalMs <= 0) {
      autoRefreshIntervalMs = intervalMs
      clearAutoRefreshTimer()
      return
    }

    if (intervalMs === autoRefreshIntervalMs) {
      return
    }

    autoRefreshIntervalMs = intervalMs
    clearAutoRefreshTimer()

    autoRefreshTimer = setInterval(() => {
      if (autoRefreshInFlight) {
        return
      }

      autoRefreshInFlight = true
      void (async () => {
        try {
          await runAutoRefreshTick()
        } catch {
          // Ignore auto-refresh tick errors and continue next cycle.
        } finally {
          autoRefreshInFlight = false
        }
      })()
    }, intervalMs)
  }

  $: {
    if (!sessionAutoRefreshInitialized && Number(state?.autoRefreshMinutes || 0) > 0) {
      sessionAutoRefreshMinutes = Math.max(1, Number(state?.autoRefreshMinutes || 5))
      sessionAutoRefreshInitialized = true
    }

    const minutes = Math.max(1, Number(sessionAutoRefreshMinutes || 5))
    syncAutoRefreshTimer(minutes * 60 * 1000)
  }

  onMount(() => {
    unsubscribeSecondInstance = EventsOn('app:second-instance', handleSecondInstanceNotice)

    void (async () => {
      try {
        await refreshCore()
        const logLimit = state?.logMaxEntries || 1000
        await refreshLogs(logLimit)
        bindLogsSubscription(logLimit)
        await checkForUpdates()
      } catch (error) {
        notifyError('Initial Load Failed', error)
      }
    })()
  })

  onDestroy(() => {
    authController.stop()
    kiroAuthController.stop()
    unsubscribeLogs?.()
    unsubscribeSecondInstance?.()
    clearAutoRefreshTimer()
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
            {busyAccountIds}
            {authSession}
            {kiroAuthSession}
            {authWorking}
            {refreshingAllQuotas}
            showExhaustedDefault={state?.showExhaustedDefault ?? true}
            showDisabledDefault={state?.showDisabledDefault ?? true}
            onStartConnect={handleStartConnect}
            onCancelConnect={handleCancelConnect}
            onStartKiroConnect={handleStartKiroConnect}
            onCancelKiroConnect={handleCancelKiroConnect}
            onOpenExternalURL={handleOpenExternalURL}
            onRefreshAllQuotas={handleRefreshAllQuotas}
            onForceRefreshAllQuotas={handleForceRefreshAllQuotas}
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
            onStartProxy={() => runProxyAction('Proxy Started', appService.startProxy, 'Proxy service started.')}
            onStopProxy={() => runProxyAction('Proxy Stopped', appService.stopProxy, 'Proxy service stopped.')}
            onSetProxyPort={(port) =>
              runProxyAction('Proxy Port Updated', () => appService.setProxyPort(port), `Proxy port set to ${port}.`)}
            onSetAllowLAN={handleSetAllowLAN}
            onSetAutoStartProxy={handleSetAutoStartProxy}
            onSetProxyAPIKey={handleSetProxyAPIKey}
            onRegenerateProxyAPIKey={handleRegenerateProxyAPIKey}
            onSetAuthorizationMode={handleSetAuthorizationMode}
            onSetSchedulingMode={handleSetSchedulingMode}
            onSetCircuitBreaker={handleSetCircuitBreaker}
            onSetCircuitSteps={handleSetCircuitSteps}
            onRefreshProxyStatus={refreshProxyStatusSafe}
            onRefreshCloudflaredStatus={refreshCloudflaredStatusSafe}
            onSetCloudflaredConfig={handleSetCloudflaredConfig}
            onInstallCloudflared={handleInstallCloudflared}
            onStartCloudflared={handleStartCloudflared}
            onStopCloudflared={handleStopCloudflared}
            onGetCLISyncStatuses={() => appService.getCliSyncStatuses()}
            onGetLocalModelCatalog={() => appService.getLocalModelCatalog()}
            onGetCLISyncFileContent={(appId, path) => appService.getCliSyncFileContent(appId, path)}
            onSaveCLISyncFileContent={(appId, path, content) => appService.saveCliSyncFileContent(appId, path, content)}
            onSyncCLIConfig={handleSyncCLIConfig}
          />
        {:else if activeTab === 'usage'}
          <UsageTab {state} {accounts} {proxyStatus} {logs} />
        {:else if activeTab === 'system-logs'}
          <SystemLogsTab
            logs={logs}
            loading={loadingLogs}
            clearing={clearingLogs}
            onRefreshLogs={refreshLogs}
            onClearLogs={handleClearLogs}
          />
        {:else if activeTab === 'settings'}
          <SettingsTab
            {state}
            onOpenDataDir={handleOpenDataDir}
            currentAutoRefreshMinutes={sessionAutoRefreshMinutes}
            onSetSessionAutoRefreshMinutes={handleSetSessionAutoRefreshMinutes}
            onExportBackup={handleExportBackup}
            onRestoreBackup={handleRestoreBackup}
          />
        {/if}
      </div>
    </section>

    <AppFooter {proxyStatus} />
  </div>

  <ToastViewport />

  <ConfigurationErrorModal
    open={showConfigurationErrorModal}
    warnings={startupWarnings}
    on:dismiss={dismissConfigurationErrorModal}
    on:openDataDir={handleOpenDataDir}
  />

  <UpdatePromptModal
    open={showUpdatePrompt}
    currentVersion={updateInfo?.currentVersion || ''}
    latestVersion={updateInfo?.latestVersion || ''}
    releaseName={updateInfo?.releaseName || ''}
    releaseUrl={updateInfo?.releaseUrl || ''}
    on:dismiss={dismissUpdatePrompt}
    on:openRelease={openUpdateReleasePage}
  />
</main>
