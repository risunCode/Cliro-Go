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
  cloudflared: CloudflaredState
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

export type CliSyncAppID = 'claude-code' | 'opencode-cli' | 'kilo-cli' | 'codex-ai'

export interface CliSyncFile {
  name: string
  path: string
}

export interface CliSyncStatus {
  id: CliSyncAppID
  label: string
  installed: boolean
  installPath?: string
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

export interface EndpointTestRequest {
  baseUrl: string
  apiKey: string
  endpointId: string
  body?: string
}

export interface EndpointTestResult {
  status: string
  responseText: string
}
