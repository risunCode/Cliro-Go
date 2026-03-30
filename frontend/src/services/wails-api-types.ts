import type { auth, config, logger, main } from '../../wailsjs/go/models'

export type AppState = main.State & {
  startupWarnings?: Array<{
    code?: string
    filePath?: string
    backupPath?: string
    message?: string
  }>
  autoRefreshQuotaPolicy?: string
  autoRefreshMinutes?: number
  showExhaustedDefault?: boolean
  showDisabledDefault?: boolean
  batchBehavior?: string
  schedulingMode?: string
  circuitBreaker?: boolean
  circuitSteps?: number[]
  authorizationMode?: boolean
  logMaxEntries?: number
  logFileSizeCapMb?: number
  logVerbosity?: string
  importExportPolicy?: string
  networkTimeoutSeconds?: number
  retryLimit?: number
  quotaRefreshWorkers?: number
}
export type Account = config.Account & {
  banned?: boolean
  bannedReason?: string
}
export type AuthSession = auth.CodexAuthSessionView
export interface KiroAuthSession {
  sessionId: string
  authUrl: string
  status: string
  error?: string
  accountId?: string
  email?: string
  verificationUrl?: string
  userCode?: string
  expiresAt?: number
  authMethod?: string
  provider?: string
}
export type LogEntry = logger.Entry
export interface ClearLogsResult {
  memoryCleared: boolean
  fileCleared: boolean
  pendingRetry: boolean
  error: string
}

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
  proxyApiKey: string
  authorizationMode: boolean
  schedulingMode: string
  circuitBreaker: boolean
  circuitSteps: number[]
  cloudflared: CloudflaredState
  autoRefreshQuotaPolicy: string
  autoRefreshMinutes: number
  showExhaustedDefault: boolean
  showDisabledDefault: boolean
  batchBehavior: string
  logMaxEntries: number
  logFileSizeCapMb: number
  logVerbosity: string
  importExportPolicy: string
  networkTimeoutSeconds: number
  retryLimit: number
  quotaRefreshWorkers: number
}

export interface CloudflaredState {
  enabled: boolean
  mode: 'quick' | 'auth'
  token: string
  useHttp2: boolean
  installed: boolean
  version: string
  running: boolean
  url: string
  error: string
}

export type CliSyncAppID = 'claude-code' | 'opencode-cli' | 'codex-ai' | 'gemini-cli'

export interface CliSyncFile {
  name: string
  path: string
}

export interface CliSyncStatus {
  id: CliSyncAppID
  label: string
  installed: boolean
  version?: string
  synced: boolean
  currentBaseUrl?: string
  currentModel?: string
  files: CliSyncFile[]
}

export interface CliSyncResult {
  id: CliSyncAppID
  label: string
  model?: string
  currentBaseUrl?: string
  files: CliSyncFile[]
}

export interface LocalModelCatalogItem {
  id: string
  ownedBy: string
}

export interface UpdateInfo {
  currentVersion: string
  latestVersion: string
  releaseName: string
  releaseUrl: string
  publishedAt: string
  updateAvailable: boolean
}

const asString = (value: unknown): string => {
  return typeof value === 'string' ? value : String(value ?? '')
}

const asBoolean = (value: unknown): boolean => {
  return Boolean(value)
}

const asNumber = (value: unknown): number => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }

  const parsed = Number(value ?? 0)
  return Number.isFinite(parsed) ? parsed : 0
}

const asStringArray = (value: unknown): string[] => {
  if (!Array.isArray(value)) {
    return []
  }
  return value.map((item) => String(item))
}

const asNumberArray = (value: unknown): number[] => {
  if (!Array.isArray(value)) {
    return []
  }

  return value
    .map((item) => {
      if (typeof item === 'number' && Number.isFinite(item)) {
        return item
      }
      const parsed = Number(item)
      return Number.isFinite(parsed) ? parsed : 0
    })
    .filter((item) => item > 0)
}

const asRecord = (value: unknown): Record<string, unknown> => {
  if (typeof value === 'object' && value !== null) {
    return value as Record<string, unknown>
  }
  return {}
}

const pick = (payload: Record<string, unknown>, camelKey: string, snakeKey: string): unknown => {
  return payload[camelKey] ?? payload[snakeKey]
}

const toSyncResultBase = (payload: unknown): SyncResultBase => {
  const record = asRecord(payload)

  return {
    targetPath: asString(pick(record, 'targetPath', 'target_path')),
    fileExisted: asBoolean(pick(record, 'fileExisted', 'file_existed')),
    updatedFields: asStringArray(pick(record, 'updatedFields', 'updated_fields')),
    accountID: asString(pick(record, 'accountID', 'account_id')),
    provider: asString(record.provider)
  }
}

const toOAuthSyncResult = (
  payload: unknown,
  target: 'kilo-cli' | 'opencode-cli'
): KiloAuthSyncResult | OpencodeAuthSyncResult => {
  const record = asRecord(payload)
  const base = toSyncResultBase(payload)
  const result = {
    ...base,
    target,
    openAICreated: asBoolean(pick(record, 'openAICreated', 'openai_created')),
    syncedExpires: asNumber(pick(record, 'syncedExpires', 'synced_expires')),
    syncedExpiresAt: asString(pick(record, 'syncedExpiresAt', 'synced_expires_at')) || undefined
  }

  if (target === 'kilo-cli') {
    return result as KiloAuthSyncResult
  }

  return result as OpencodeAuthSyncResult
}

export const toProxyStatus = (payload: unknown): ProxyStatus => {
  const record = asRecord(payload)
  const cloudflared = asRecord(record.cloudflared)
  const cloudflaredMode = asString(cloudflared.mode) === 'auth' ? 'auth' : 'quick'

  return {
    running: asBoolean(record.running),
    port: asNumber(record.port),
    url: asString(record.url),
    bindAddress: asString(record.bindAddress),
    allowLan: asBoolean(record.allowLan),
    autoStartProxy: asBoolean(record.autoStartProxy),
    proxyApiKey: asString(record.proxyApiKey),
    authorizationMode: asBoolean(record.authorizationMode),
    schedulingMode: asString(record.schedulingMode) || 'balance',
    circuitBreaker: asBoolean(record.circuitBreaker),
    circuitSteps: asNumberArray(record.circuitSteps).length > 0 ? asNumberArray(record.circuitSteps) : [10, 30, 60],
    cloudflared: {
	  enabled: asBoolean(cloudflared.enabled),
	  mode: cloudflaredMode,
	  token: asString(cloudflared.token),
	  useHttp2: !('useHttp2' in cloudflared) || asBoolean(cloudflared.useHttp2),
	  installed: asBoolean(cloudflared.installed),
	  version: asString(cloudflared.version),
	  running: asBoolean(cloudflared.running),
	  url: asString(cloudflared.url),
	  error: asString(cloudflared.error)
	},
    autoRefreshQuotaPolicy: asString(record.autoRefreshQuotaPolicy),
    autoRefreshMinutes: asNumber(record.autoRefreshMinutes),
    showExhaustedDefault: asBoolean(record.showExhaustedDefault),
    showDisabledDefault: asBoolean(record.showDisabledDefault),
    batchBehavior: asString(record.batchBehavior),
    logMaxEntries: asNumber(record.logMaxEntries),
    logFileSizeCapMb: asNumber(record.logFileSizeCapMb),
    logVerbosity: asString(record.logVerbosity),
    importExportPolicy: asString(record.importExportPolicy),
    networkTimeoutSeconds: asNumber(record.networkTimeoutSeconds),
    retryLimit: asNumber(record.retryLimit),
    quotaRefreshWorkers: asNumber(record.quotaRefreshWorkers)
  }
}

export const toKiloAuthSyncResult = (payload: unknown): KiloAuthSyncResult => {
  return toOAuthSyncResult(payload, 'kilo-cli') as KiloAuthSyncResult
}

export const toOpencodeAuthSyncResult = (payload: unknown): OpencodeAuthSyncResult => {
  return toOAuthSyncResult(payload, 'opencode-cli') as OpencodeAuthSyncResult
}

export const toCodexAuthSyncResult = (payload: unknown): CodexAuthSyncResult => {
  const record = asRecord(payload)

  return {
    ...toSyncResultBase(record),
    target: 'codex-cli',
    backupPath: asString(pick(record, 'backupPath', 'backup_path')) || undefined,
    backupCreated: asBoolean(pick(record, 'backupCreated', 'backup_created')),
    syncedAt: asString(pick(record, 'syncedAt', 'synced_at'))
  }
}

export const toUpdateInfo = (payload: unknown): UpdateInfo => {
  const record = asRecord(payload)

  return {
    currentVersion: asString(record.currentVersion),
    latestVersion: asString(record.latestVersion),
    releaseName: asString(record.releaseName),
    releaseUrl: asString(record.releaseUrl),
    publishedAt: asString(record.publishedAt),
    updateAvailable: asBoolean(record.updateAvailable)
  }
}

const toCliSyncFile = (payload: unknown): CliSyncFile => {
  const record = asRecord(payload)
  return {
    name: asString(record.name),
    path: asString(record.path)
  }
}

export const toCliSyncStatus = (payload: unknown): CliSyncStatus => {
  const record = asRecord(payload)
  const rawID = asString(record.id)
  const id: CliSyncAppID = rawID === 'opencode-cli' || rawID === 'codex-ai' || rawID === 'gemini-cli' ? rawID : 'claude-code'
  const files = Array.isArray(record.files) ? record.files.map(toCliSyncFile) : []
  return {
    id,
    label: asString(record.label),
    installed: asBoolean(record.installed),
    version: asString(record.version) || undefined,
    synced: asBoolean(record.synced),
    currentBaseUrl: asString(record.currentBaseUrl) || undefined,
    currentModel: asString(record.currentModel) || undefined,
    files
  }
}

export const toCliSyncResult = (payload: unknown): CliSyncResult => {
  const record = asRecord(payload)
  const files = Array.isArray(record.files) ? record.files.map(toCliSyncFile) : []
  const rawID = asString(record.id)
  const id: CliSyncAppID = rawID === 'opencode-cli' || rawID === 'codex-ai' || rawID === 'gemini-cli' ? rawID : 'claude-code'
  return {
    id,
    label: asString(record.label),
    model: asString(record.model) || undefined,
    currentBaseUrl: asString(record.currentBaseUrl) || undefined,
    files
  }
}

export const toLocalModelCatalogItem = (payload: unknown): LocalModelCatalogItem => {
  const record = asRecord(payload)
  return {
    id: asString(record.id),
    ownedBy: asString(record.ownedBy)
  }
}
