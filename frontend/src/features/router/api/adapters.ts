import { asBoolean, asNumber, asRecord, asString } from '@/shared/api/wails/adapters'
import type { CliSyncAppID, CliSyncFile, CliSyncResult, CliSyncStatus, LocalModelCatalogItem, ProxyStatus } from '@/features/router/types'

const toCliSyncAppID = (value: string): CliSyncAppID | null => {
  if (value === 'claude-code' || value === 'opencode-cli' || value === 'kilo-cli' || value === 'codex-ai') {
    return value
  }
  return null
}

const toCliSyncFile = (payload: unknown): CliSyncFile => {
  const record = asRecord(payload)
  return {
    name: asString(record.name),
    path: asString(record.path)
  }
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
    }
  }
}

export const toCliSyncStatus = (payload: unknown): CliSyncStatus | null => {
  const record = asRecord(payload)
  const id = toCliSyncAppID(asString(record.id))
  if (!id) {
    return null
  }
  const files = Array.isArray(record.files) ? record.files.map(toCliSyncFile) : []
  return {
    id,
    label: asString(record.label),
    installed: asBoolean(record.installed),
    installPath: asString(record.installPath) || undefined,
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
  const id = toCliSyncAppID(asString(record.id))
  if (!id) {
    throw new Error('Unsupported CLI sync result id')
  }
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
