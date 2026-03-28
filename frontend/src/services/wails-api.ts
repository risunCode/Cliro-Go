import {
  CancelCodexAuth,
  ClearCooldown,
  ClearLogs,
  DeleteAccount,
  GetAccounts,
  GetCodexAuthSession,
  GetHostName,
  GetLogs,
  GetProxyStatus,
  GetState,
  ImportAccounts,
  OpenExternalURL,
  OpenDataDir,
  RefreshAccount,
  RefreshAllQuotas,
  RefreshQuota,
  SetAllowLAN,
  SetAutoStartProxy,
  SetProxyPort,
  SyncCodexAccountToCodexCLI,
  SyncCodexAccountToKiloAuth,
  SyncCodexAccountToOpencodeAuth,
  StartCodexAuth,
  StartProxy,
  StopProxy,
  ToggleAccount
} from '../../wailsjs/go/main/App'
import type { auth, config, logger, main } from '../../wailsjs/go/models'

export type AppState = main.State
export type Account = config.Account
export type AuthSession = auth.CodexAuthSessionView
export type LogEntry = logger.Entry

export type SyncTargetID = 'kilo-cli' | 'opencode-cli' | 'codex-cli'

export interface SyncResultBase {
  targetPath: string
  fileExisted: boolean
  updatedFields: string[]
  accountID: string
  provider: string
}

export interface KiloAuthSyncResult extends SyncResultBase {
  target: 'kilo-cli'
  openAICreated: boolean
  syncedExpires: number
  syncedExpiresAt?: string
}

export interface OpencodeAuthSyncResult extends SyncResultBase {
  target: 'opencode-cli'
  openAICreated: boolean
  syncedExpires: number
  syncedExpiresAt?: string
}

export interface CodexAuthSyncResult extends SyncResultBase {
  target: 'codex-cli'
  backupPath?: string
  backupCreated: boolean
  syncedAt: string
}

export type AccountSyncResult = KiloAuthSyncResult | OpencodeAuthSyncResult | CodexAuthSyncResult

export interface ProxyStatus {
  running: boolean
  port: number
  url: string
  bindAddress: string
  allowLan: boolean
  autoStartProxy: boolean
}

const toProxyStatus = (payload: Record<string, any>): ProxyStatus => ({
  running: Boolean(payload.running),
  port: Number(payload.port ?? 0),
  url: String(payload.url ?? ''),
  bindAddress: String(payload.bindAddress ?? ''),
  allowLan: Boolean(payload.allowLan),
  autoStartProxy: Boolean(payload.autoStartProxy)
})

const toKiloAuthSyncResult = (payload: Record<string, any>): KiloAuthSyncResult => ({
  target: 'kilo-cli',
  targetPath: String(payload.targetPath ?? payload.target_path ?? ''),
  fileExisted: Boolean(payload.fileExisted ?? payload.file_existed),
  openAICreated: Boolean(payload.openAICreated ?? payload.openai_created),
  updatedFields: Array.isArray(payload.updatedFields ?? payload.updated_fields)
    ? (payload.updatedFields ?? payload.updated_fields).map((value: unknown) => String(value))
    : [],
  accountID: String(payload.accountID ?? payload.account_id ?? ''),
  provider: String(payload.provider ?? ''),
  syncedExpires: Number(payload.syncedExpires ?? payload.synced_expires ?? 0),
  syncedExpiresAt: String(payload.syncedExpiresAt ?? payload.synced_expires_at ?? '') || undefined
})

const toOpencodeAuthSyncResult = (payload: Record<string, any>): OpencodeAuthSyncResult => ({
  target: 'opencode-cli',
  targetPath: String(payload.targetPath ?? payload.target_path ?? ''),
  fileExisted: Boolean(payload.fileExisted ?? payload.file_existed),
  openAICreated: Boolean(payload.openAICreated ?? payload.openai_created),
  updatedFields: Array.isArray(payload.updatedFields ?? payload.updated_fields)
    ? (payload.updatedFields ?? payload.updated_fields).map((value: unknown) => String(value))
    : [],
  accountID: String(payload.accountID ?? payload.account_id ?? ''),
  provider: String(payload.provider ?? ''),
  syncedExpires: Number(payload.syncedExpires ?? payload.synced_expires ?? 0),
  syncedExpiresAt: String(payload.syncedExpiresAt ?? payload.synced_expires_at ?? '') || undefined
})

const toCodexAuthSyncResult = (payload: Record<string, any>): CodexAuthSyncResult => ({
  target: 'codex-cli',
  targetPath: String(payload.targetPath ?? payload.target_path ?? ''),
  fileExisted: Boolean(payload.fileExisted ?? payload.file_existed),
  backupPath: String(payload.backupPath ?? payload.backup_path ?? '') || undefined,
  backupCreated: Boolean(payload.backupCreated ?? payload.backup_created),
  updatedFields: Array.isArray(payload.updatedFields ?? payload.updated_fields)
    ? (payload.updatedFields ?? payload.updated_fields).map((value: unknown) => String(value))
    : [],
  accountID: String(payload.accountID ?? payload.account_id ?? ''),
  provider: String(payload.provider ?? ''),
  syncedAt: String(payload.syncedAt ?? payload.synced_at ?? '')
})

export const appService = {
  getState: (): Promise<AppState> => GetState(),
  getAccounts: (): Promise<Account[]> => GetAccounts(),
  getProxyStatus: async (): Promise<ProxyStatus> => toProxyStatus(await GetProxyStatus()),
  getHostName: async (): Promise<string> => String(await GetHostName()),
  getLogs: (limit = 300): Promise<LogEntry[]> => GetLogs(limit),
  importAccounts: (accounts: Account[]): Promise<number> => ImportAccounts(accounts),

  startCodexAuth: (): Promise<auth.CodexAuthStart> => StartCodexAuth(),
  getCodexAuthSession: (sessionId: string): Promise<AuthSession> => GetCodexAuthSession(sessionId),
  cancelCodexAuth: (sessionId: string): Promise<void> => CancelCodexAuth(sessionId),

  refreshAccount: (accountId: string): Promise<void> => RefreshAccount(accountId),
  refreshQuota: (accountId: string): Promise<void> => RefreshQuota(accountId),
  refreshAllQuotas: (): Promise<void> => RefreshAllQuotas(),
  toggleAccount: (accountId: string, enabled: boolean): Promise<void> => ToggleAccount(accountId, enabled),
  deleteAccount: (accountId: string): Promise<void> => DeleteAccount(accountId),
  clearCooldown: (accountId: string): Promise<void> => ClearCooldown(accountId),
  syncCodexAccountToKiloAuth: async (accountId: string): Promise<KiloAuthSyncResult> =>
    toKiloAuthSyncResult(await SyncCodexAccountToKiloAuth(accountId)),
  syncCodexAccountToOpencodeAuth: async (accountId: string): Promise<OpencodeAuthSyncResult> =>
    toOpencodeAuthSyncResult(await SyncCodexAccountToOpencodeAuth(accountId)),
  syncCodexAccountToCodexCLI: async (accountId: string): Promise<CodexAuthSyncResult> =>
    toCodexAuthSyncResult(await SyncCodexAccountToCodexCLI(accountId)),

  startProxy: (): Promise<void> => StartProxy(),
  stopProxy: (): Promise<void> => StopProxy(),
  setProxyPort: (port: number): Promise<void> => SetProxyPort(port),
  setAllowLAN: (enabled: boolean): Promise<void> => SetAllowLAN(enabled),
  setAutoStartProxy: (enabled: boolean): Promise<void> => SetAutoStartProxy(enabled),

  clearLogs: (): Promise<void> => ClearLogs(),
  openExternalURL: (url: string): Promise<void> => OpenExternalURL(url),
  openDataDir: (): Promise<void> => OpenDataDir()
}
