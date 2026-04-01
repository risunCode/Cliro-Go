import { GetCLISyncFileContent, GetCLISyncStatuses, GetLocalModelCatalog, GetModelAliases, GetProxyStatus, InstallCloudflared, RefreshCloudflaredStatus, RegenerateProxyAPIKey, SaveCLISyncFileContent, SetAllowLAN, SetAuthorizationMode, SetAutoStartProxy, SetCloudflaredConfig, SetModelAliases, SetProxyAPIKey, SetProxyPort, SetSchedulingMode, StartCloudflared, StartProxy, StopCloudflared, StopProxy, SyncCLIConfig } from '@/shared/api/wails/client'
import { toCliSyncResult, toCliSyncStatus, toLocalModelCatalogItem, toProxyStatus } from '@/features/router/api/adapters'
import { buildEndpointTarget, getEndpointPreset } from '@/features/router/lib/endpoint-tester'
import { getErrorMessage } from '@/shared/lib/error'
import type { CliSyncAppID, CliSyncResult, CliSyncStatus, EndpointTestRequest, EndpointTestResult, LocalModelCatalogItem, ProxyStatus } from '@/features/router/types'

const fetchProxyModelCatalog = async (baseUrl: string, apiKey: string): Promise<LocalModelCatalogItem[]> => {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, '')
  if (!normalizedBaseUrl) {
    return []
  }

  const headers: Record<string, string> = {}
  const normalizedApiKey = apiKey.trim()
  if (normalizedApiKey) {
    headers.Authorization = `Bearer ${normalizedApiKey}`
  }

  const response = await fetch(`${normalizedBaseUrl}/v1/models`, { headers })
  if (!response.ok) {
    throw new Error(`Proxy model catalog request failed with status ${response.status}.`)
  }

  const payload = (await response.json()) as { data?: Array<{ id?: string; ownedBy?: string; owned_by?: string }> }
  if (!Array.isArray(payload.data)) {
    return []
  }

  return payload.data
    .map((item) => ({
      id: typeof item.id === 'string' ? item.id : '',
      ownedBy: typeof item.ownedBy === 'string' ? item.ownedBy : typeof item.owned_by === 'string' ? item.owned_by : ''
    }))
    .filter((item) => item.id)
}

const getEffectiveModelCatalog = async (baseUrl: string, apiKey: string): Promise<LocalModelCatalogItem[]> => {
  try {
    const localModels = (await GetLocalModelCatalog()).map(toLocalModelCatalogItem)
    if (localModels.length > 0) {
      return localModels
    }
  } catch {
    // Fall through to proxy model catalog lookup.
  }

  return fetchProxyModelCatalog(baseUrl, apiKey)
}

const executeEndpointTest = async ({ baseUrl, apiKey, endpointId, body = '' }: EndpointTestRequest): Promise<EndpointTestResult> => {
  const endpoint = getEndpointPreset(endpointId)
  const target = buildEndpointTarget(baseUrl, endpoint.path)
  const headers: Record<string, string> = {}
  const normalizedApiKey = apiKey.trim()
  if (normalizedApiKey) {
    headers.Authorization = `Bearer ${normalizedApiKey}`
    headers['X-API-Key'] = normalizedApiKey
  }

  const options: RequestInit = {
    method: endpoint.method,
    headers
  }

  if (endpoint.method === 'POST') {
    headers['Content-Type'] = 'application/json'
    options.body = body
  }

  try {
    const response = await fetch(target, options)
    const contentType = response.headers.get('content-type') || ''
    const responseText = contentType.includes('application/json')
      ? JSON.stringify(await response.json(), null, 2)
      : await response.text()

    return {
      status: `${response.status} ${response.statusText}`,
      responseText
    }
  } catch (error) {
    throw new Error(getErrorMessage(error, 'Request failed'))
  }
}

export const routerApi = {
  getProxyStatus: async (): Promise<ProxyStatus> => toProxyStatus(await GetProxyStatus()),
  refreshCloudflaredStatus: async (): Promise<ProxyStatus> => toProxyStatus(await RefreshCloudflaredStatus()),
  getEffectiveModelCatalog,
  getCliSyncStatuses: async (): Promise<CliSyncStatus[]> =>
    (await GetCLISyncStatuses())
      .map(toCliSyncStatus)
      .filter((status): status is CliSyncStatus => status !== null),
  getCliSyncFileContent: (appId: CliSyncAppID, path: string): Promise<string> => GetCLISyncFileContent(appId, path),
  saveCliSyncFileContent: (appId: CliSyncAppID, path: string, content: string): Promise<void> => SaveCLISyncFileContent(appId, path, content),
  syncCLIConfig: async (appId: CliSyncAppID, model: string): Promise<CliSyncResult> => toCliSyncResult(await SyncCLIConfig(appId, model)),
  startProxy: (): Promise<void> => StartProxy(),
  stopProxy: (): Promise<void> => StopProxy(),
  setProxyPort: (port: number): Promise<void> => SetProxyPort(port),
  setAllowLAN: (enabled: boolean): Promise<void> => SetAllowLAN(enabled),
  setAutoStartProxy: (enabled: boolean): Promise<void> => SetAutoStartProxy(enabled),
  setProxyAPIKey: (apiKey: string): Promise<void> => SetProxyAPIKey(apiKey),
  regenerateProxyAPIKey: (): Promise<string> => RegenerateProxyAPIKey(),
  setAuthorizationMode: (enabled: boolean): Promise<void> => SetAuthorizationMode(enabled),
  setSchedulingMode: (mode: string): Promise<void> => SetSchedulingMode(mode),
  setCloudflaredConfig: (mode: string, token: string, useHttp2: boolean): Promise<void> => SetCloudflaredConfig(mode, token, useHttp2),
  installCloudflared: (): Promise<void> => InstallCloudflared(),
  startCloudflared: (): Promise<void> => StartCloudflared(),
  stopCloudflared: (): Promise<void> => StopCloudflared(),
  executeEndpointTest,
  getModelAliases: (): Promise<Record<string, string>> => GetModelAliases(),
  setModelAliases: (aliases: Record<string, string>): Promise<void> => SetModelAliases(aliases)
}
