<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { Bot, ChevronDown, ChevronUp, Cloud, Copy, Cpu, Globe, Info, KeyRound, Network, Pencil, Play, Power, PowerOff, RefreshCw, Save, Sparkles, Terminal } from 'lucide-svelte'
  import { slide } from 'svelte/transition'
  import Button from '@/components/common/Button.svelte'
  import CliSyncInfoModal from '@/components/common/CliSyncInfoModal.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import ToggleSwitch from '@/components/common/ToggleSwitch.svelte'
  import type { CliSyncAppID, CliSyncResult, CliSyncStatus, LocalModelCatalogItem, ProxyStatus } from '@/services/wails-api-types'
  import { copyTextToClipboard, hasClipboardWrite } from '@/utils/browser'

  interface EndpointPreset {
    id: string
    label: string
    method: 'GET' | 'POST'
    path: string
    defaultBody: string
  }

  interface CliSyncCard {
    id: CliSyncAppID
    label: string
    icon: typeof Terminal
    toneClass: string
  }

  type SchedulingMode = 'cache_first' | 'balance' | 'performance'
  type CloudflaredMode = 'quick' | 'auth'

  interface SchedulingModeCard {
    id: SchedulingMode
    label: string
    description: string
  }

  interface CloudflaredModeCard {
    id: CloudflaredMode
    label: string
    description: string
  }

  interface TesterRenderedBlock {
    kind: 'thinking' | 'text'
    content: string
  }

  interface TesterStructuredResponse {
    thinking: string
    message: string
  }

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let onStartProxy: () => Promise<void>
  export let onStopProxy: () => Promise<void>
  export let onSetProxyPort: (port: number) => Promise<void>
  export let onSetAllowLAN: (enabled: boolean) => Promise<void>
  export let onSetAutoStartProxy: (enabled: boolean) => Promise<void>
  export let onSetProxyAPIKey: (apiKey: string) => Promise<void>
  export let onRegenerateProxyAPIKey: () => Promise<string>
  export let onSetAuthorizationMode: (enabled: boolean) => Promise<void>
  export let onSetSchedulingMode: (mode: string) => Promise<void>
  export let onSetCircuitBreaker: (enabled: boolean) => Promise<void>
  export let onSetCircuitSteps: (steps: number[]) => Promise<void>
  export let onRefreshProxyStatus: () => Promise<void>
  export let onRefreshCloudflaredStatus: () => Promise<void>
  export let onSetCloudflaredConfig: (mode: string, token: string, useHttp2: boolean) => Promise<void>
  export let onInstallCloudflared: () => Promise<void>
  export let onStartCloudflared: () => Promise<void>
  export let onStopCloudflared: () => Promise<void>
  export let onGetCLISyncStatuses: () => Promise<CliSyncStatus[]>
  export let onGetLocalModelCatalog: () => Promise<LocalModelCatalogItem[]>
  export let onGetCLISyncFileContent: (appId: CliSyncAppID, path: string) => Promise<string>
  export let onSaveCLISyncFileContent: (appId: CliSyncAppID, path: string, content: string) => Promise<void>
  export let onSyncCLIConfig: (appId: CliSyncAppID, model: string) => Promise<CliSyncResult>

  const endpointPresets: EndpointPreset[] = [
    { id: 'health', label: 'GET /health', method: 'GET', path: '/health', defaultBody: '' },
    { id: 'models', label: 'GET /v1/models', method: 'GET', path: '/v1/models', defaultBody: '' },
    { id: 'stats', label: 'GET /v1/stats', method: 'GET', path: '/v1/stats', defaultBody: '' },
    {
      id: 'responses',
      label: 'POST /v1/responses (Codex CLI)',
      method: 'POST',
      path: '/v1/responses',
      defaultBody: JSON.stringify(
        {
          model: 'gpt-5.3-codex',
          input: 'Say hello from CLIro responses API.',
          stream: false
        },
        null,
        2
      )
    },
    {
      id: 'chat-completions',
      label: 'POST /v1/chat/completions (OpenAI/compatible)',
      method: 'POST',
      path: '/v1/chat/completions',
      defaultBody: JSON.stringify(
        {
          model: 'gpt-5.3-codex',
          messages: [{ role: 'user', content: 'Say hello from CLIro.' }],
          stream: false
        },
        null,
        2
      )
    },
    {
      id: 'completions',
      label: 'POST /v1/completions (OpenAI/compatible)',
      method: 'POST',
      path: '/v1/completions',
      defaultBody: JSON.stringify(
        {
          model: 'gpt-5.3-codex',
          prompt: 'Write one sentence about local proxy routing.',
          stream: false
        },
        null,
        2
      )
    },
    {
      id: 'messages',
      label: 'POST /v1/messages (Anthropic/compatible)',
      method: 'POST',
      path: '/v1/messages',
      defaultBody: JSON.stringify(
        {
          model: 'claude-haiku-4.5',
          max_tokens: 256,
          stream: false,
          messages: [{ role: 'user', content: 'Say hello from CLIro Anthropic-compatible endpoint.' }]
        },
        null,
        2
      )
    }
  ]

  const cliSyncCards: CliSyncCard[] = [
    { id: 'claude-code', label: 'Claude Code Config', icon: Sparkles, toneClass: 'text-violet-400' },
    { id: 'opencode-cli', label: 'OpenCode Config', icon: Bot, toneClass: 'text-amber-400' },
    { id: 'codex-ai', label: 'Codex AI Config', icon: Cpu, toneClass: 'text-sky-400' },
    { id: 'gemini-cli', label: 'Gemini CLI Config', icon: Globe, toneClass: 'text-emerald-400' }
  ]

  const schedulingModeCards: SchedulingModeCard[] = [
    {
      id: 'cache_first',
      label: 'Cache First',
      description: 'Bind session to the same account for stronger cache locality.'
    },
    {
      id: 'balance',
      label: 'Balance',
      description: 'Spread load across accounts by favoring lower-usage accounts.'
    },
    {
      id: 'performance',
      label: 'Performance',
      description: 'Use pure round-robin ordering for high concurrency throughput.'
    }
  ]

  const cloudflaredModeCards: CloudflaredModeCard[] = [
    {
      id: 'quick',
      label: 'Quick Tunnel',
      description: 'Auto-generated temporary URL (*.trycloudflare.com), no account needed.'
    },
    {
      id: 'auth',
      label: 'Named Tunnel',
      description: 'Use a Cloudflare tunnel token for persistent public access or custom domains.'
    }
  ]

  const toSchedulingMode = (value: string): SchedulingMode => {
    if (value === 'cache_first' || value === 'balance' || value === 'performance') {
      return value
    }
    return 'balance'
  }

  const normalizeCircuitSteps = (steps: number[] | null | undefined): number[] => {
    const defaults = [10, 30, 60]
    if (!Array.isArray(steps) || steps.length === 0) {
      return defaults
    }

    return defaults.map((fallback, index) => {
      const value = Number(steps[index] ?? fallback)
      if (!Number.isFinite(value) || value <= 0) {
        return fallback
      }
      return Math.max(1, Math.min(3600, Math.round(value)))
    })
  }

  let portInput = '8095'
  let portInputDirty = false
  let allowLanInput = false
  let autoStartProxyInput = true
  let authorizationModeInput = false
  let apiKeyInput = ''
  let apiKeyCopied = false
  let apiKeyCopyTimer: ReturnType<typeof setTimeout> | null = null
  let selectedEndpointId = endpointPresets[0].id
  let selectedEndpointOptions: EndpointPreset[] = endpointPresets
  let selectedEndpoint: EndpointPreset = endpointPresets[0]
  let requestBody = endpointPresets[0].defaultBody
  let testerLoading = false
  let testerStatus = '-'
  let testerResponse = ''
  let testerError = ''
  let testerStructuredResponse: TesterStructuredResponse | null = null
  let testerResponseCopied = false
  let testerResponseCopyTimer: ReturnType<typeof setTimeout> | null = null
  let testerShowRawResponse = false
  let testerThinkingExpanded = false
  let testerExpanded = false
  let cliSyncExpanded = false
  let schedulingExpanded = false
  let cloudflaredExpanded = false
  let schedulingModeInput: SchedulingMode = 'balance'
  let circuitBreakerInput = false
  let circuitStepInputs = ['10', '30', '60']
  let circuitStepsDirty = false
  let schedulingError = ''
  let cloudflaredModeInput: CloudflaredMode = 'quick'
  let cloudflaredTokenInput = ''
  let cloudflaredUseHTTP2Input = true
  let cloudflaredConfigDirty = false
  let cliSyncBusyTargetID: CliSyncAppID | '' = ''
  let cliSyncDataLoaded = false
  let cliSyncDataLoading = false
  let cliSyncResults: Partial<Record<CliSyncAppID, CliSyncResult>> = {}
  let cliSyncErrors: Partial<Record<CliSyncAppID, string>> = {}
  let cliSyncInfoTargetID: CliSyncAppID | '' = ''
  let cliSyncStatuses: CliSyncStatus[] = []
  let cliSyncModels: LocalModelCatalogItem[] = []
  let cliSyncSelectedModels: Record<CliSyncAppID, string> = {
    'claude-code': '',
    'opencode-cli': '',
    'codex-ai': '',
    'gemini-cli': ''
  }

  $: testerStructuredResponse = buildTesterStructuredResponse(testerResponse)
  $: cloudflaredCanStart = cloudflaredModeInput !== 'auth' || cloudflaredTokenInput.trim().length > 0
  $: cloudflaredToggleDisabled = busy || (!(proxyStatus?.cloudflared.enabled ?? false) && (!proxyStatus?.running || !cloudflaredCanStart))
  $: if (cliSyncModels.length > 0) {
    for (const card of cliSyncCards) {
      const current = cliSyncSelectedModels[card.id]
      const stillExists = cliSyncModels.some((item) => item.id === current)
      if (!current || !stillExists) {
        cliSyncSelectedModels = {
          ...cliSyncSelectedModels,
          [card.id]: defaultModelForCli(card.id, cliSyncModels)
        }
      }
    }
  }

  $: if (proxyStatus?.port && !portInputDirty) {
    portInput = String(proxyStatus.port)
  }

  $: if (proxyStatus && !busy) {
    allowLanInput = proxyStatus.allowLan
    autoStartProxyInput = proxyStatus.autoStartProxy
    authorizationModeInput = proxyStatus.authorizationMode
    apiKeyInput = proxyStatus.proxyApiKey || ''
    schedulingModeInput = toSchedulingMode(proxyStatus.schedulingMode)
    circuitBreakerInput = proxyStatus.circuitBreaker
    if (!cloudflaredConfigDirty) {
      cloudflaredModeInput = proxyStatus.cloudflared.mode === 'auth' ? 'auth' : 'quick'
      cloudflaredTokenInput = proxyStatus.cloudflared.token || ''
      cloudflaredUseHTTP2Input = proxyStatus.cloudflared.useHttp2
    }
    const normalizedSteps = normalizeCircuitSteps(proxyStatus.circuitSteps)
    if (!circuitStepsDirty) {
      circuitStepInputs = normalizedSteps.map((value) => String(value))
    }
  }

  $: selectedEndpointOptions = endpointPresets

  $: selectedEndpoint = selectedEndpointOptions.find((endpoint) => endpoint.id === selectedEndpointId) || endpointPresets[0]

  $: if (selectedEndpoint && selectedEndpoint.method === 'GET') {
    requestBody = ''
  }

  const applySelectedEndpoint = (event?: Event): void => {
    if (event?.currentTarget instanceof HTMLSelectElement) {
      selectedEndpointId = event.currentTarget.value
    }
    if (selectedEndpoint.method === 'POST') {
      requestBody = selectedEndpoint.defaultBody
    }
  }

  const buildEndpointTarget = (baseURL: string, routePath: string): string => {
    const trimmedBase = baseURL.trim().replace(/\/+$/, '')
    const normalizedPath = routePath.startsWith('/') ? routePath : `/${routePath}`

    if (/\/v1$/i.test(trimmedBase) && /^\/v1(\/|$)/i.test(normalizedPath)) {
      return `${trimmedBase.slice(0, -3)}${normalizedPath}`
    }

    return `${trimmedBase}${normalizedPath}`
  }

  const runEndpointTest = async (): Promise<void> => {
    if (!proxyStatus?.url) {
      testerError = 'Proxy URL is not available.'
      return
    }

    testerLoading = true
    testerError = ''
    testerStatus = '-'
    testerResponse = ''
    testerShowRawResponse = false
    testerThinkingExpanded = false

    try {
      const target = buildEndpointTarget(proxyStatus.url, selectedEndpoint.path)
      const headers: Record<string, string> = {}
      const currentAPIKey = apiKeyInput.trim()
      if (currentAPIKey.length > 0) {
        headers.Authorization = `Bearer ${currentAPIKey}`
        headers['X-API-Key'] = currentAPIKey
      }

      const options: RequestInit = {
        method: selectedEndpoint.method,
        headers
      }

      if (selectedEndpoint.method === 'POST') {
        headers['Content-Type'] = 'application/json'
        options.body = requestBody
      }

      const response = await fetch(target, options)
      testerStatus = `${response.status} ${response.statusText}`

      const contentType = response.headers.get('content-type') || ''
      if (contentType.includes('application/json')) {
        const payload = await response.json()
        testerResponse = JSON.stringify(payload, null, 2)
      } else {
        testerResponse = await response.text()
      }
    } catch (error) {
      testerError = error instanceof Error ? error.message : 'Request failed'
    } finally {
      testerLoading = false
    }
  }

  const applyProxyPort = async (): Promise<void> => {
    const parsedPort = Number.parseInt(portInput.trim(), 10)
    const nextPort = Number.isFinite(parsedPort) && parsedPort >= 1024 && parsedPort <= 65535 ? parsedPort : 8095
    portInput = String(nextPort)
    portInputDirty = false
    await onSetProxyPort(nextPort)
  }

  const toggleTesterExpanded = (): void => {
    testerExpanded = !testerExpanded
  }

  const toggleCliSyncExpanded = (): void => {
    cliSyncExpanded = !cliSyncExpanded
    if (cliSyncExpanded && !cliSyncDataLoaded && !cliSyncDataLoading) {
      void refreshCliSyncData().catch(() => {})
    }
  }

  const toggleSchedulingExpanded = (): void => {
    schedulingExpanded = !schedulingExpanded
  }

  const toggleCloudflaredExpanded = (): void => {
    cloudflaredExpanded = !cloudflaredExpanded
    if (cloudflaredExpanded) {
      void refreshCloudflaredStatus().catch(() => {})
      startCloudflaredPolling()
      return
    }
    stopCloudflaredPolling()
  }

  const updateAllowLan = async (): Promise<void> => {
    await onSetAllowLAN(allowLanInput)
  }

  const updateAutoStartProxy = async (): Promise<void> => {
    await onSetAutoStartProxy(autoStartProxyInput)
  }

  const updateAuthorizationMode = async (): Promise<void> => {
    await onSetAuthorizationMode(authorizationModeInput)
  }

  const refreshCloudflaredStatus = async (): Promise<void> => {
    await onRefreshCloudflaredStatus()
  }

  const applySchedulingMode = async (mode: SchedulingMode): Promise<void> => {
    if (mode === schedulingModeInput) {
      return
    }
    schedulingModeInput = mode
    await onSetSchedulingMode(mode)
  }

  const updateCircuitBreaker = async (): Promise<void> => {
    schedulingError = ''
    await onSetCircuitBreaker(circuitBreakerInput)
  }

  const updateCircuitStepInput = (index: number, value: string): void => {
    const next = [...circuitStepInputs]
    next[index] = value
    circuitStepInputs = next
    circuitStepsDirty = true
  }

  const handleCircuitStepInput = (index: number, event: Event): void => {
    const target = event.currentTarget as HTMLInputElement
    updateCircuitStepInput(index, target.value)
  }

  const applyCircuitSteps = async (): Promise<void> => {
    const parsed = circuitStepInputs.map((value) => Number.parseInt(value.trim(), 10))
    const invalid = parsed.some((value) => !Number.isFinite(value) || value <= 0 || value > 3600)
    if (invalid) {
      schedulingError = 'Each circuit breaker step must be 1-3600 seconds.'
      return
    }

    schedulingError = ''
    const normalized = parsed.map((value) => Math.round(value))
    await onSetCircuitSteps(normalized)
    circuitStepInputs = normalized.map((value) => String(value))
    circuitStepsDirty = false
  }

  const persistCloudflaredConfig = async (): Promise<void> => {
    cloudflaredConfigDirty = true
    try {
      await onSetCloudflaredConfig(cloudflaredModeInput, cloudflaredTokenInput, cloudflaredUseHTTP2Input)
    } finally {
      cloudflaredConfigDirty = false
    }
  }

  const selectCloudflaredMode = async (mode: CloudflaredMode): Promise<void> => {
    if (mode === cloudflaredModeInput) {
      return
    }
    cloudflaredModeInput = mode
    await persistCloudflaredConfig()
  }

  const updateCloudflaredToken = (event: Event): void => {
    const target = event.currentTarget as HTMLInputElement
    cloudflaredTokenInput = target.value
    cloudflaredConfigDirty = true
  }

  const saveCloudflaredToken = async (): Promise<void> => {
    await persistCloudflaredConfig()
  }

  const updateCloudflaredHTTP2 = async (): Promise<void> => {
    cloudflaredConfigDirty = true
    await persistCloudflaredConfig()
  }

  const toggleCloudflared = async (): Promise<void> => {
    if (proxyStatus?.cloudflared.enabled) {
      await onStopCloudflared()
      return
    }
    await persistCloudflaredConfig()
    if (!proxyStatus?.cloudflared.installed) {
      await onInstallCloudflared()
    }
    await onStartCloudflared()
  }

  const copyCloudflaredURL = async (): Promise<void> => {
    if (proxyStatus?.cloudflared.url) {
      await copyTextToClipboard(proxyStatus.cloudflared.url)
    }
  }

  const refreshCliSyncData = async (): Promise<void> => {
    cliSyncDataLoading = true
    try {
      const [statusesResult, modelsResult] = await Promise.allSettled([onGetCLISyncStatuses(), onGetLocalModelCatalog()])
      if (statusesResult.status === 'fulfilled') {
        cliSyncStatuses = statusesResult.value
      }
      if (modelsResult.status === 'fulfilled') {
        cliSyncModels = modelsResult.value
      }
      if (statusesResult.status === 'fulfilled' || modelsResult.status === 'fulfilled') {
        cliSyncDataLoaded = true
      }
    } finally {
      cliSyncDataLoading = false
    }
  }

  function cliSyncStatusFor(targetID: CliSyncAppID): CliSyncStatus | null {
    return cliSyncStatuses.find((item) => item.id === targetID) || null
  }

  function providerLabel(ownedBy: string): string {
    return ownedBy === 'kiro' ? 'Kiro' : ownedBy === 'codex' ? 'Codex' : 'Other'
  }

  function groupedCliModels(): Array<{ label: string; models: LocalModelCatalogItem[] }> {
    const labels = ['Kiro', 'Codex', 'Other']
    return labels
      .map((label) => ({
        label,
        models: cliSyncModels.filter((item) => providerLabel(item.ownedBy) === label)
      }))
      .filter((group) => group.models.length > 0)
  }

  function defaultModelForCli(targetID: CliSyncAppID, models: LocalModelCatalogItem[]): string {
    if (targetID === 'claude-code') {
      return models.find((item) => item.ownedBy === 'kiro')?.id || models[0]?.id || ''
    }
    if (targetID === 'codex-ai') {
      return models.find((item) => item.ownedBy === 'codex')?.id || models[0]?.id || ''
    }
    return models[0]?.id || ''
  }

  const updateCliSyncModel = (targetID: CliSyncAppID, event: Event): void => {
    const target = event.currentTarget as HTMLSelectElement
    cliSyncSelectedModels = {
      ...cliSyncSelectedModels,
      [targetID]: target.value
    }
  }

  const syncCliTarget = async (targetID: CliSyncAppID): Promise<void> => {
    const model = cliSyncSelectedModels[targetID] || defaultModelForCli(targetID, cliSyncModels)
    if (!model) {
      cliSyncErrors = { ...cliSyncErrors, [targetID]: 'No local proxy model is available for sync.' }
      return
    }

    cliSyncBusyTargetID = targetID
    cliSyncErrors = { ...cliSyncErrors, [targetID]: '' }

    try {
      const result = await onSyncCLIConfig(targetID, model)
      cliSyncResults = { ...cliSyncResults, [targetID]: result }
      await refreshCliSyncData()
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Sync failed.'
      cliSyncErrors = { ...cliSyncErrors, [targetID]: message }
    } finally {
      cliSyncBusyTargetID = ''
    }
  }

  const openCliSyncInfo = (targetID: CliSyncAppID): void => {
    cliSyncInfoTargetID = targetID
  }

  const closeCliSyncInfo = (): void => {
    cliSyncInfoTargetID = ''
  }

  const editProxyAPIKey = async (): Promise<void> => {
    const currentValue = apiKeyInput || ''
    const updatedValue = window.prompt('Set proxy API key', currentValue)
    if (updatedValue === null) {
      return
    }

    const normalized = updatedValue.trim()
    if (normalized.length === 0) {
      testerError = 'API key cannot be empty.'
      return
    }

    testerError = ''
    await onSetProxyAPIKey(normalized)
    apiKeyInput = normalized
  }

  const regenerateProxyAPIKey = async (): Promise<void> => {
    testerError = ''
    const nextKey = await onRegenerateProxyAPIKey()
    apiKeyInput = nextKey
  }

  const copyProxyAPIKey = async (): Promise<void> => {
    if (!hasClipboardWrite() || apiKeyInput.trim().length === 0) {
      return
    }

    const copied = await copyTextToClipboard(apiKeyInput)
    if (!copied) {
      return
    }

    apiKeyCopied = true
    if (apiKeyCopyTimer) {
      clearTimeout(apiKeyCopyTimer)
    }
    apiKeyCopyTimer = setTimeout(() => {
      apiKeyCopied = false
      apiKeyCopyTimer = null
    }, 1200)
  }

  const copyTesterResponse = async (): Promise<void> => {
    if (!hasClipboardWrite() || !testerResponse.trim()) {
      return
    }

    const copied = await copyTextToClipboard(testerResponse)
    if (!copied) {
      return
    }

    testerResponseCopied = true
    if (testerResponseCopyTimer) {
      clearTimeout(testerResponseCopyTimer)
    }
    testerResponseCopyTimer = setTimeout(() => {
      testerResponseCopied = false
      testerResponseCopyTimer = null
    }, 1200)
  }

  const toggleTesterRawResponse = (): void => {
    testerShowRawResponse = !testerShowRawResponse
  }

  const toggleTesterThinking = (): void => {
    testerThinkingExpanded = !testerThinkingExpanded
  }

  const buildTesterStructuredResponse = (payload: string): TesterStructuredResponse | null => {
    const trimmed = payload.trim()
    if (!trimmed) {
      return null
    }

    try {
      const parsed = JSON.parse(trimmed) as Record<string, unknown>
      return extractStructuredResponseFromJSON(parsed)
    } catch {
      return extractStructuredResponseFromSSE(trimmed)
    }
  }

  const extractStructuredResponseFromJSON = (payload: Record<string, unknown>): TesterStructuredResponse | null => {
    const blocks = extractMessageBlocks(payload)
    if (blocks.length > 0) {
      const thinking = blocks.filter((block) => block.kind === 'thinking').map((block) => block.content).join('\n\n').trim()
      const message = blocks.filter((block) => block.kind === 'text').map((block) => block.content).join('\n\n').trim()
      if (!thinking && !message) {
        return null
      }
      return { thinking, message }
    }

    const openAIStructured = extractStructuredResponseFromOpenAIJSON(payload)
    if (openAIStructured) {
      return openAIStructured
    }

    const response = payload.response
    if (response && typeof response === 'object') {
      return extractStructuredResponseFromJSON(response as Record<string, unknown>)
    }
    return null
  }

  const extractStructuredResponseFromSSE = (payload: string): TesterStructuredResponse | null => {
    const sections = payload.split(/\r?\n\r?\n/)
    let thinking = ''
    let message = ''

    for (const section of sections) {
      const lines = section.split(/\r?\n/)
      let eventName = ''
      const dataLines: string[] = []
      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventName = line.slice(6).trim()
        } else if (line.startsWith('data:')) {
          dataLines.push(line.slice(5).trim())
        }
      }
      if (dataLines.length === 0) {
        continue
      }

      const rawData = dataLines.join('\n')
      if (rawData === '[DONE]') {
        continue
      }
      try {
        const parsed = JSON.parse(rawData) as Record<string, unknown>
        if (!eventName) {
          const structured = extractStructuredResponseFromOpenAIJSON(parsed)
          if (structured) {
            thinking += structured.thinking
            message += structured.message
          }
          continue
        }
        switch (eventName) {
          case 'content_block_delta': {
            const delta = parsed.delta
            if (delta && typeof delta === 'object') {
              const deltaRecord = delta as Record<string, unknown>
              const deltaType = String(deltaRecord.type || '').trim()
              if (deltaType === 'thinking_delta') {
                thinking += String(deltaRecord.thinking || '')
              }
              if (deltaType === 'text_delta') {
                message += String(deltaRecord.text || '')
              }
            }
            break
          }
          case 'response.output_text.delta':
            message += String(parsed.delta || '')
            break
          case 'response.completed': {
            const response = parsed.response
            if (response && typeof response === 'object') {
              const responseRecord = response as Record<string, unknown>
              if (!message) {
                message = String(responseRecord.output_text || '').trim()
              }
            }
            break
          }
          case 'message_start': {
            const messageRecord = parsed.message
            if (messageRecord && typeof messageRecord === 'object') {
              const structured = extractStructuredResponseFromJSON(messageRecord as Record<string, unknown>)
              if (structured) {
                if (!thinking) thinking = structured.thinking
                if (!message) message = structured.message
              }
            }
            break
          }
          default:
            break
        }
      } catch {
        continue
      }
    }

    const normalizedThinking = thinking.trim()
    const normalizedMessage = message.trim()
    if (!normalizedThinking && !normalizedMessage) {
      return null
    }
    return { thinking: normalizedThinking, message: normalizedMessage }
  }

  const extractStructuredResponseFromOpenAIJSON = (payload: Record<string, unknown>): TesterStructuredResponse | null => {
    const choices = payload.choices
    if (!Array.isArray(choices)) {
      return null
    }

    let thinking = ''
    let message = ''

    for (const choice of choices) {
      if (!choice || typeof choice !== 'object') {
        continue
      }
      const choiceRecord = choice as Record<string, unknown>

      const messageRecord = choiceRecord.message
      if (messageRecord && typeof messageRecord === 'object') {
        const content = extractOpenAIMessageContent(messageRecord as Record<string, unknown>)
        if (content) {
          message += content
        }
      }

      const deltaRecord = choiceRecord.delta
      if (deltaRecord && typeof deltaRecord === 'object') {
        const delta = deltaRecord as Record<string, unknown>
        if (typeof delta.content === 'string') {
          message += delta.content
        }
        if (typeof delta.reasoning === 'string') {
          thinking += delta.reasoning
        }
      }

      if (typeof choiceRecord.text === 'string') {
        message += choiceRecord.text
      }
    }

    const normalizedThinking = thinking.trim()
    const normalizedMessage = message.trim()
    if (!normalizedThinking && !normalizedMessage) {
      return null
    }
    return { thinking: normalizedThinking, message: normalizedMessage }
  }

  const extractOpenAIMessageContent = (payload: Record<string, unknown>): string => {
    const content = payload.content
    if (typeof content === 'string') {
      return content.trim()
    }
    if (!Array.isArray(content)) {
      return ''
    }

    const parts: string[] = []
    for (const item of content) {
      if (!item || typeof item !== 'object') {
        continue
      }
      const record = item as Record<string, unknown>
      if (typeof record.text === 'string' && record.text.trim()) {
        parts.push(record.text.trim())
      }
    }
    return parts.join('\n\n').trim()
  }

  const extractMessageBlocks = (payload: Record<string, unknown>): TesterRenderedBlock[] => {
    const content = payload.content
    if (!Array.isArray(content)) {
      return []
    }

    const blocks: TesterRenderedBlock[] = []
    for (const item of content) {
      if (!item || typeof item !== 'object') {
        continue
      }
      const record = item as Record<string, unknown>
      const type = String(record.type || '').trim()
      if (type === 'thinking') {
        const thinking = String(record.thinking || '').trim()
        if (thinking) {
          blocks.push({ kind: 'thinking', content: thinking })
        }
        continue
      }
      if (type === 'text') {
        const text = String(record.text || '').trim()
        if (text) {
          blocks.push({ kind: 'text', content: text })
        }
      }
    }
    return blocks
  }

  let cloudflaredPollTimer: ReturnType<typeof setInterval> | null = null

  const startCloudflaredPolling = (): void => {
    if (cloudflaredPollTimer) {
      return
    }
    cloudflaredPollTimer = setInterval(() => {
      void refreshCloudflaredStatus().catch(() => {})
    }, 5000)
  }

  const stopCloudflaredPolling = (): void => {
    if (cloudflaredPollTimer) {
      clearInterval(cloudflaredPollTimer)
      cloudflaredPollTimer = null
    }
  }

  onMount(() => {
    void onRefreshProxyStatus().catch(() => {})
  })

  onDestroy(() => {
    stopCloudflaredPolling()
    if (apiKeyCopyTimer) {
      clearTimeout(apiKeyCopyTimer)
    }
    if (testerResponseCopyTimer) {
      clearTimeout(testerResponseCopyTimer)
    }
  })
</script>

<div class="space-y-4">
  <SurfaceCard className="p-4">
    <div class="mb-3 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm font-semibold text-text-primary">Proxy Service</p>
        <p class="text-xs text-text-secondary">Grid controls for runtime, bind mode, and startup behavior.</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <StatusBadge tone={proxyStatus?.running ? 'success' : 'error'}>
          {proxyStatus?.running ? 'Running' : 'Stopped'}
        </StatusBadge>
      </div>
    </div>

    <div class="grid gap-3 lg:grid-cols-2">
      <div class="rounded-sm border border-border bg-app p-3">
        <p class="mb-2 text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Port</p>
        <div class="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end">
          <input
            id="router-port"
            class="ui-control-input ui-control-select px-3 text-sm"
            bind:value={portInput}
            on:input={() => {
              portInputDirty = true
            }}
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
          />
          <Button variant="secondary" size="sm" on:click={applyProxyPort} disabled={busy}>
            <Save size={14} class="mr-1" />
            Apply
          </Button>
        </div>
        <p class="mt-2 truncate text-xs text-text-secondary">Bind Address: {proxyStatus?.bindAddress || '-'}</p>
      </div>

      <div class="rounded-sm border border-border bg-app p-3">
        <p class="mb-2 text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Runtime</p>
        <div class="grid gap-2 sm:grid-cols-2">
          <Button variant="primary" size="sm" on:click={onStartProxy} disabled={busy || proxyStatus?.running}>
            <Power size={14} class="mr-1" />
            Start Proxy
          </Button>
          <Button variant="danger" size="sm" on:click={onStopProxy} disabled={busy || !proxyStatus?.running}>
            <PowerOff size={14} class="mr-1" />
            Stop Proxy
          </Button>
        </div>
        <p class="mt-1 text-[11px] text-text-secondary">Use this as the base URL. Add `/v1` only once when a client requires it.</p>
      </div>

      <div class="rounded-sm border border-border bg-gradient-to-br from-surface/90 to-app p-3 lg:col-span-2">
        <div class="mb-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Network & Startup</p>
            <p class="mt-1 text-[11px] text-text-secondary">Control how the local proxy binds to the network and how it behaves when the desktop app starts.</p>
          </div>
          <span class="rounded-full border border-border bg-app px-2.5 py-1 text-[10px] uppercase tracking-[0.08em] text-text-secondary">Lifecycle Controls</span>
        </div>

        <div class="grid gap-3 lg:grid-cols-2">
          <div class="grid gap-3 rounded-sm border border-border bg-app/90 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div>
              <div class="mb-2 flex items-center gap-2">
                <span class="inline-flex h-8 w-8 items-center justify-center rounded-sm border border-border bg-surface text-text-primary">
                  <Network size={15} />
                </span>
                <p class="text-sm font-semibold text-text-primary">Allow on LAN</p>
              </div>
              <p class="text-[11px] leading-5 text-text-secondary">Expose the proxy to other devices on the same network. Keep this off when the proxy should stay localhost-only.</p>
            </div>
            <div class="min-w-[240px] rounded-sm border border-border bg-surface/70 p-2">
              <ToggleSwitch label={allowLanInput ? 'LAN access enabled' : 'Localhost only'} bind:checked={allowLanInput} on:change={updateAllowLan} disabled={busy} />
            </div>
          </div>

          <div class="grid gap-3 rounded-sm border border-border bg-app/90 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div>
              <div class="mb-2 flex items-center gap-2">
                <span class="inline-flex h-8 w-8 items-center justify-center rounded-sm border border-border bg-surface text-text-primary">
                  <Power size={15} />
                </span>
                <p class="text-sm font-semibold text-text-primary">Auto Start Proxy</p>
              </div>
              <p class="text-[11px] leading-5 text-text-secondary">Bring the local proxy online automatically when CLIro-Go launches so local clients can reconnect without manual intervention.</p>
            </div>
            <div class="min-w-[240px] rounded-sm border border-border bg-surface/70 p-2">
              <ToggleSwitch
                label={autoStartProxyInput ? 'Start proxy on launch' : 'Manual start only'}
                bind:checked={autoStartProxyInput}
                on:change={updateAutoStartProxy}
                disabled={busy}
              />
            </div>
          </div>
        </div>
      </div>

      <div class="rounded-sm border border-border bg-gradient-to-br from-surface/90 to-app p-3 lg:col-span-2">
        <div class="mb-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Security</p>
            <p class="mt-1 text-[11px] text-text-secondary">Manage the proxy access key and decide whether every inbound request must authenticate.</p>
          </div>
          <StatusBadge tone={authorizationModeInput ? 'warning' : 'neutral'}>
            {authorizationModeInput ? 'API Key Required' : 'Open Access'}
          </StatusBadge>
        </div>

        <div class="grid gap-3 lg:grid-cols-2">
          <div class="rounded-sm border border-border bg-app/90 p-3">
            <div class="mb-3 flex items-start justify-between gap-3">
              <div class="min-w-0 flex-1">
                <div class="mb-2 flex items-center gap-2">
                  <p class="text-[10px] font-semibold uppercase tracking-[0.08em] text-text-secondary">API Key Vault</p>
                  <span class="rounded-full border border-border px-2 py-0.5 text-[10px] text-text-secondary">Authorization or X-API-Key</span>
                </div>
                <p class="text-[11px] leading-5 text-text-secondary">Use this key with desktop clients, scripts, or remote access tooling when authorization mode is enabled.</p>
              </div>
              <span class="inline-flex h-10 w-10 items-center justify-center rounded-sm border border-border bg-surface text-text-primary">
                <KeyRound size={16} />
              </span>
            </div>

            <div class="rounded-sm border border-border bg-surface px-3 py-3 font-mono text-xs text-text-primary break-all">
              {apiKeyInput || '-'}
            </div>

            <div class="mt-3 flex flex-wrap items-center gap-2">
              <Button variant="secondary" size="sm" on:click={editProxyAPIKey} disabled={busy}>
                <Pencil size={13} class="mr-1" />
                Edit
              </Button>
              <Button variant="secondary" size="sm" on:click={regenerateProxyAPIKey} disabled={busy}>
                <KeyRound size={13} class="mr-1" />
                Regen
              </Button>
              <Button variant="secondary" size="sm" on:click={copyProxyAPIKey} disabled={!hasClipboardWrite() || apiKeyInput.trim().length === 0}>
                <Copy size={13} class="mr-1" />
                {apiKeyCopied ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>

          <div class="grid gap-3 rounded-sm border border-border bg-app/90 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div>
              <p class="text-sm font-semibold text-text-primary">Authorization Gate</p>
              <p class="mt-1 text-[11px] leading-5 text-text-secondary">Require the configured key for every proxy route while still keeping the full OpenAI-compatible and Anthropic-compatible surface available.</p>
              <div class="mt-3 rounded-sm border border-border bg-surface/40 px-3 py-2.5 text-[11px] leading-5 text-text-secondary">
                Clients can authenticate with `Authorization: Bearer &lt;key&gt;` or `X-API-Key: &lt;key&gt;`.
              </div>
            </div>

            <div class="min-w-[260px] rounded-sm border border-border bg-surface/70 p-2">
              <ToggleSwitch
                label={authorizationModeInput ? 'API key required for all routes' : 'Requests allowed without API key'}
                bind:checked={authorizationModeInput}
                on:change={updateAuthorizationMode}
                disabled={busy}
              />
            </div>
          </div>
        </div>
      </div>

    </div>
  </SurfaceCard>

  <SurfaceCard className="api-cli-sync p-0">
    <button
      type="button"
      class="api-cli-sync-header"
      on:click={toggleCloudflaredExpanded}
      aria-expanded={cloudflaredExpanded}
      aria-label="Toggle public access cloudflared settings"
    >
      <span class="api-cli-sync-left">
        <span class="api-cli-sync-icon-wrap text-orange-400">
          <Cloud size={15} />
        </span>
        <span class="api-cli-sync-copy">
          <span class="api-cli-sync-title-row">
            <span class="api-cli-sync-title">Public Access (Cloudflared)</span>
          </span>
          <span class="api-cli-sync-subtitle">Expose the local proxy through a Cloudflare tunnel with quick or named tunnel mode.</span>
        </span>
      </span>
      <span class="api-cli-sync-header-right gap-2">
        <span class="api-cli-sync-pill">{proxyStatus?.cloudflared.running ? 'Running' : proxyStatus?.cloudflared.enabled ? 'Enabled' : 'Disabled'}</span>
        {#if busy}
          <RefreshCw size={14} class="animate-spin text-text-secondary" />
        {/if}
        <span class="api-cli-sync-chevron">
          {#if cloudflaredExpanded}
            <ChevronUp size={15} />
          {:else}
            <ChevronDown size={15} />
          {/if}
        </span>
      </span>
    </button>

    {#if cloudflaredExpanded}
      <div class="api-cli-sync-body space-y-4" transition:slide={{ duration: 180 }}>
        <div class="space-y-4">
          <div class="flex flex-col gap-3 rounded-sm border border-border bg-app/90 px-3 py-3 md:flex-row md:items-center md:justify-between">
            <div class="min-w-0">
              {#if proxyStatus?.cloudflared.installed}
                <div class="flex items-center gap-2 text-xs text-text-secondary">
                  <span class="inline-flex h-5 w-5 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-400">
                    <span class="h-2 w-2 rounded-full bg-current"></span>
                  </span>
                  <span>Installed: {proxyStatus?.cloudflared.version || 'Unknown version'}</span>
                </div>
              {:else}
                <div>
                  <p class="text-sm font-semibold text-text-primary">Cloudflared not installed</p>
                  <p class="mt-1 text-xs text-text-secondary">Download the Cloudflared binary into your local CLIro data directory before starting a tunnel.</p>
                </div>
              {/if}
            </div>

            <div class="flex flex-wrap items-center gap-3 md:justify-end">
              <ToggleSwitch
                label={proxyStatus?.cloudflared.enabled ? 'Public access on' : 'Public access off'}
                checked={proxyStatus?.cloudflared.enabled ?? false}
                on:change={toggleCloudflared}
                disabled={cloudflaredToggleDisabled || !(proxyStatus?.cloudflared.installed ?? false) && !(proxyStatus?.cloudflared.enabled ?? false)}
              />
              {#if !(proxyStatus?.cloudflared.installed ?? false)}
                <Button variant="primary" size="sm" on:click={onInstallCloudflared} disabled={busy}>
                  {#if busy}
                    <RefreshCw size={13} class="mr-1 animate-spin" />
                  {/if}
                  Install
                </Button>
              {/if}
            </div>
          </div>

          <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(260px,auto)]">
            <div class="rounded-sm border border-border bg-app/90 p-3">
              <p class="mb-3 text-sm font-semibold text-text-primary">Tunnel Routing</p>
              <div class="grid gap-3 md:grid-cols-2">
                {#each cloudflaredModeCards as card}
                  <button
                    type="button"
                    class={`api-rotation-mode-card ${cloudflaredModeInput === card.id ? 'is-active' : ''}`}
                    on:click={() => void selectCloudflaredMode(card.id)}
                    disabled={busy || (proxyStatus?.cloudflared.running ?? false)}
                  >
                    <span class="api-rotation-mode-title">{card.label}</span>
                    <span class="api-rotation-mode-desc">{card.description}</span>
                  </button>
                {/each}
              </div>

              {#if cloudflaredModeInput === 'auth'}
                <div class="mt-3 space-y-2 rounded-sm border border-border bg-surface/50 p-3">
                  <p class="api-endpoint-label">Tunnel Token</p>
                  <input
                    type="password"
                    value={cloudflaredTokenInput}
                    on:input={updateCloudflaredToken}
                    on:blur={saveCloudflaredToken}
                    class="ui-control-input ui-control-select font-mono text-xs"
                    placeholder="eyJhIjoi..."
                    disabled={busy || (proxyStatus?.cloudflared.running ?? false)}
                  />
                  <p class="text-[11px] text-text-secondary">Required only for named tunnels. Leave blank in quick tunnel mode.</p>
                </div>
              {/if}
            </div>

            <div class="rounded-sm border border-border bg-app/90 p-3">
              <ToggleSwitch
                label={cloudflaredUseHTTP2Input ? 'HTTP/2 enabled' : 'HTTP/2 disabled'}
                bind:checked={cloudflaredUseHTTP2Input}
                on:change={updateCloudflaredHTTP2}
                disabled={busy || (proxyStatus?.cloudflared.running ?? false)}
              />
              <p class="mt-2 text-[11px] text-text-secondary">More compatible for constrained networks and unstable routes.</p>
            </div>
          </div>

          <div class={`rounded-sm border p-4 ${proxyStatus?.cloudflared.running ? 'border-emerald-500/40 bg-emerald-500/10' : 'border-border bg-app/90'}`}>
            <div class="mb-2 flex items-center gap-2 text-sm font-semibold text-text-primary">
              <span class={`h-2 w-2 rounded-full ${proxyStatus?.cloudflared.running ? 'animate-pulse bg-emerald-400' : 'bg-text-secondary/50'}`}></span>
              {proxyStatus?.cloudflared.running ? 'Tunnel Running' : 'Tunnel Stopped'}
            </div>

            {#if proxyStatus?.cloudflared.running}
              <div class="flex flex-col gap-2 md:flex-row md:items-center">
                <code class="flex-1 break-all rounded-sm border border-emerald-500/30 bg-app px-3 py-2 text-xs text-text-primary">{proxyStatus.cloudflared.url || 'Waiting for public URL...'}</code>
                <Button variant="secondary" size="sm" on:click={copyCloudflaredURL} disabled={!proxyStatus.cloudflared.url}>
                  <Copy size={13} class="mr-1" />
                  Copy
                </Button>
              </div>
            {:else}
              <div class="rounded-sm border border-border bg-surface/40 px-3 py-2.5 text-[11px] leading-5 text-text-secondary">
                {#if !proxyStatus?.running && !(proxyStatus?.cloudflared.enabled ?? false)}
                  Start the local proxy first before enabling Cloudflared public access.
                {:else if cloudflaredModeInput === 'auth' && !cloudflaredCanStart && !(proxyStatus?.cloudflared.enabled ?? false)}
                  Add a tunnel token before enabling named tunnel mode.
                {:else if proxyStatus?.cloudflared.enabled && !proxyStatus?.running}
                  Cloudflared is enabled, but the local proxy is stopped. Start the proxy service to bring the public tunnel online.
                {:else}
                  The Cloudflared process is managed locally and restarts together with the proxy whenever public access remains enabled.
                {/if}
              </div>
            {/if}
          </div>

          {#if proxyStatus?.cloudflared.error}
            <div class="rounded-sm border border-error/40 bg-error/10 px-3 py-2.5 text-sm text-error">
              {proxyStatus.cloudflared.error}
            </div>
          {/if}
        </div>
      </div>
    {/if}
  </SurfaceCard>

  <SurfaceCard className="api-cli-sync p-0">
    <button
      type="button"
      class="api-cli-sync-header"
      on:click={toggleCliSyncExpanded}
      aria-expanded={cliSyncExpanded}
      aria-label="Toggle one-click CLI sync"
    >
      <span class="api-cli-sync-left">
        <span class="api-cli-sync-icon-wrap">
          <RefreshCw size={15} />
        </span>
        <span class="api-cli-sync-copy">
          <span class="api-cli-sync-title-row">
            <span class="api-cli-sync-title">One-click CLI Sync</span>
          </span>
          <span class="api-cli-sync-subtitle">Instantly sync proxy endpoint, API key, and selected model into local AI CLI configs.</span>
        </span>
      </span>
      <span class="api-cli-sync-header-right">
        <span class="api-cli-sync-pill">Config Sync</span>
        <span class="api-cli-sync-chevron">
          {#if cliSyncExpanded}
            <ChevronUp size={15} />
          {:else}
            <ChevronDown size={15} />
          {/if}
        </span>
      </span>
    </button>

    {#if cliSyncExpanded}
      <div class="api-cli-sync-body space-y-3" transition:slide={{ duration: 180 }}>
        <div class="grid gap-2.5 lg:grid-cols-4">
          {#each cliSyncCards as tool (tool.id)}
            <div class="flex h-full flex-col overflow-hidden rounded-sm border border-border bg-app/90 shadow-soft transition hover:border-border/80 hover:bg-surface/70">
              <div class="flex flex-1 flex-col p-2.5">
                <div class="mb-2.5 flex items-start justify-between gap-2">
                <div class="flex min-w-0 items-start gap-2.5">
                  <span class={`inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-sm border border-border bg-surface ${tool.toneClass}`}>
                    <svelte:component this={tool.icon} size={15} />
                  </span>
                  <div class="min-w-0">
                    <p class="text-[15px] font-semibold leading-5 text-text-primary">{tool.label}</p>
                    <p class="mt-1 text-[10px] text-text-secondary">{cliSyncStatusFor(tool.id)?.installed ? `v${cliSyncStatusFor(tool.id)?.version || 'installed'}` : 'Not detected'}</p>
                  </div>
                </div>
                <StatusBadge tone={cliSyncStatusFor(tool.id)?.synced ? 'success' : cliSyncErrors[tool.id] ? 'warning' : 'neutral'}>
                  {cliSyncStatusFor(tool.id)?.synced ? 'Synced' : cliSyncErrors[tool.id] ? 'Error' : 'Not Synced'}
                </StatusBadge>
              </div>

              <div class="flex flex-1 flex-col space-y-2">
                <div class="rounded-sm border border-dashed border-border bg-surface/60 px-2 py-1.5">
                  <p class="text-[9px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Current Base URL</p>
                  <code class="mt-1 block break-all text-[10px] leading-4 text-text-primary">{cliSyncStatusFor(tool.id)?.currentBaseUrl || '---'}</code>
                </div>

                <div class="space-y-1">
                  <p class="text-[9px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Select Model</p>
                  <select class="ui-control-input ui-control-select text-sm" value={cliSyncSelectedModels[tool.id]} on:change={(event) => updateCliSyncModel(tool.id, event)} disabled={cliSyncModels.length === 0}>
                    {#if cliSyncModels.length === 0}
                      <option value="">No local models available</option>
                    {:else}
                      {#each groupedCliModels() as group (group.label)}
                        <optgroup label={group.label}>
                          {#each group.models as model (model.id)}
                            <option value={model.id}>{model.id}</option>
                          {/each}
                        </optgroup>
                      {/each}
                    {/if}
                  </select>
                </div>

                <div class="rounded-sm border border-border bg-surface/40 px-2 py-1.5 text-[10px] leading-4 text-text-secondary">
                  {#if cliSyncErrors[tool.id]}
                    {cliSyncErrors[tool.id]}
                  {:else if cliSyncStatusFor(tool.id)?.currentModel}
                    Current model: {cliSyncStatusFor(tool.id)?.currentModel}
                  {:else}
                    Sync local proxy endpoint, API key, and selected model into this CLI config.
                  {/if}
                </div>
                </div>
              </div>

              <div class="mt-auto flex items-center gap-2 border-t border-border bg-surface/50 px-2.5 py-2">
                <Button variant="secondary" size="sm" className="px-2.5" on:click={() => openCliSyncInfo(tool.id)}>
                  <Info size={13} class="mr-1" />
                  Info
                </Button>
                {#if cliSyncStatusFor(tool.id)?.installed}
                  <Button
                    variant="primary"
                    size="sm"
                    className="flex-1"
                    on:click={() => void syncCliTarget(tool.id)}
                    disabled={busy || cliSyncBusyTargetID !== '' || !cliSyncSelectedModels[tool.id]}
                  >
                    <RefreshCw size={13} class={`mr-1 ${cliSyncBusyTargetID === tool.id ? 'animate-spin' : ''}`} />
                    {cliSyncBusyTargetID === tool.id ? 'Syncing...' : 'Sync Now'}
                  </Button>
                {:else}
                  <Button variant="secondary" size="sm" className="flex-1" disabled={true}>Not Installed</Button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </SurfaceCard>

  <CliSyncInfoModal
    open={cliSyncInfoTargetID !== ''}
    appID={cliSyncInfoTargetID}
    label={cliSyncCards.find((card) => card.id === cliSyncInfoTargetID)?.label || 'CLI Sync Info'}
    selectedModel={cliSyncInfoTargetID ? cliSyncSelectedModels[cliSyncInfoTargetID] || '' : ''}
    proxyBaseURL={proxyStatus?.url || ''}
    proxyAPIKey={apiKeyInput}
    status={cliSyncInfoTargetID ? cliSyncStatusFor(cliSyncInfoTargetID) : null}
    result={cliSyncInfoTargetID ? cliSyncResults[cliSyncInfoTargetID] || null : null}
    availableModels={cliSyncModels}
    onLoadFileContent={onGetCLISyncFileContent}
    onSaveFileContent={onSaveCLISyncFileContent}
    on:close={closeCliSyncInfo}
    on:saved={() => void refreshCliSyncData().catch(() => {})}
  />

  <SurfaceCard className="api-cli-sync api-endpoint-tester p-0">
    <button
      type="button"
      class="api-cli-sync-header api-endpoint-tester-header"
      on:click={toggleTesterExpanded}
      aria-expanded={testerExpanded}
      aria-label="Toggle endpoint tester"
    >
      <span class="api-cli-sync-left">
        <span class="api-cli-sync-icon-wrap">
          <Network size={15} />
        </span>
        <span class="api-cli-sync-copy">
          <span class="api-cli-sync-title-row">
            <span class="api-cli-sync-title">Endpoint Tester</span>
          </span>
          <span class="api-cli-sync-subtitle">Run OpenAI-compatible and Anthropic-compatible endpoint probes against your local proxy.</span>
        </span>
      </span>
      <span class="api-cli-sync-header-right">
        <span class="api-cli-sync-pill">Proxy Integration</span>
        <span class="api-cli-sync-chevron">
          {#if testerExpanded}
            <ChevronUp size={15} />
          {:else}
            <ChevronDown size={15} />
          {/if}
        </span>
      </span>
    </button>

    {#if testerExpanded}
      <div class="api-cli-sync-body api-endpoint-body" transition:slide={{ duration: 180 }}>
        <div class="api-endpoint-grid">
          <div class="api-endpoint-panel ui-panel-soft ui-panel-dashed">
            <div class="api-endpoint-controls ui-inline-controls">
              <select bind:value={selectedEndpointId} class="api-endpoint-select ui-control-input ui-control-select" on:change={applySelectedEndpoint}>
                {#each selectedEndpointOptions as endpoint}
                  <option value={endpoint.id}>{endpoint.label}</option>
                {/each}
              </select>
              <Button
                variant="primary"
                size="sm"
                className="api-endpoint-run whitespace-nowrap"
                on:click={runEndpointTest}
                disabled={testerLoading || !proxyStatus?.running}
              >
                <Play size={14} class="mr-1" />
                Execute
              </Button>
            </div>

            <div class="api-endpoint-request">
              <p class="api-endpoint-label">Request Payload</p>
              {#if selectedEndpoint.method === 'POST'}
                <textarea bind:value={requestBody} class="api-endpoint-textarea ui-control-input ui-control-textarea"></textarea>
              {:else}
                <p class="text-text-secondary">This endpoint is GET and does not require a request body.</p>
              {/if}
            </div>
          </div>

          <div class="api-endpoint-panel ui-panel-soft ui-panel-dashed">
            <div class="mb-2 flex items-start justify-between gap-2">
              <div>
                <p class="api-endpoint-label">Response</p>
              </div>
              <div class="flex items-center gap-2">
                <Button variant="secondary" size="sm" on:click={toggleTesterRawResponse} disabled={!testerResponse.trim()}>
                  {testerShowRawResponse ? 'Hide Raw' : 'Show Raw'}
                </Button>
                <Button variant="secondary" size="sm" on:click={copyTesterResponse} disabled={!hasClipboardWrite() || !testerResponse.trim()}>
                  <Copy size={13} class="mr-1" />
                  {testerResponseCopied ? 'Copied' : 'Copy'}
                </Button>
              </div>
            </div>
            <div class="mb-2 flex items-center gap-2">
              <StatusBadge tone={testerStatus.startsWith('2') ? 'success' : testerStatus === '-' ? 'neutral' : 'warning'}>{testerStatus}</StatusBadge>
              {#if testerLoading}
                <span class="text-text-secondary">Request in progress...</span>
              {/if}
            </div>
            {#if testerError}
              <p class="text-error">{testerError}</p>
            {:else if testerResponse}
              {#if testerStructuredResponse && !testerShowRawResponse}
                <div class="mb-3 space-y-3 rounded-sm border border-border bg-app p-3">
                  {#if testerStructuredResponse.thinking}
                    <div class="rounded-sm border border-border bg-surface">
                      <button
                        type="button"
                        class="flex w-full items-center justify-between gap-2 px-3 py-2 text-left"
                        on:click={toggleTesterThinking}
                      >
                        <span>
                          <p class="text-[10px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Thinking</p>
                          <p class="text-[11px] text-text-secondary">Auto-collapsed after completion. Expand to inspect reasoning.</p>
                        </span>
                        <span class="text-text-secondary">
                          {#if testerThinkingExpanded}
                            <ChevronUp size={15} />
                          {:else}
                            <ChevronDown size={15} />
                          {/if}
                        </span>
                      </button>
                      {#if testerThinkingExpanded}
                        <pre class="api-endpoint-response no-scrollbar whitespace-pre-wrap border-t border-border bg-app p-3">{testerStructuredResponse.thinking}</pre>
                      {/if}
                    </div>
                  {/if}

                  {#if testerStructuredResponse.message}
                    <div>
                      <p class="mb-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Response</p>
                      <pre class="api-endpoint-response no-scrollbar whitespace-pre-wrap border border-border bg-surface p-3">{testerStructuredResponse.message}</pre>
                    </div>
                  {/if}
                </div>
              {/if}
              {#if testerShowRawResponse || !testerStructuredResponse}
                <pre class="api-endpoint-response no-scrollbar">{testerResponse}</pre>
              {/if}
            {:else}
              <p class="text-text-secondary">Run a request to inspect proxy responses.</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </SurfaceCard>

  <SurfaceCard className="api-cli-sync p-0">
    <button
      type="button"
      class="api-cli-sync-header"
      on:click={toggleSchedulingExpanded}
      aria-expanded={schedulingExpanded}
      aria-label="Toggle account scheduling and rotation"
    >
      <span class="api-cli-sync-left">
        <span class="api-cli-sync-icon-wrap">
          <RefreshCw size={15} />
        </span>
        <span class="api-cli-sync-copy">
          <span class="api-cli-sync-title-row">
            <span class="api-cli-sync-title">Account Scheduling & Rotation</span>
          </span>
          <span class="api-cli-sync-subtitle">Control account routing mode and staged failure backoff behavior.</span>
        </span>
      </span>
      <span class="api-cli-sync-header-right">
        <span class="api-cli-sync-pill">Routing Policy</span>
        <span class="api-cli-sync-chevron">
          {#if schedulingExpanded}
            <ChevronUp size={15} />
          {:else}
            <ChevronDown size={15} />
          {/if}
        </span>
      </span>
    </button>

    {#if schedulingExpanded}
      <div class="api-cli-sync-body" transition:slide={{ duration: 180 }}>
        <div class="api-rotation-grid">
          <div class="api-rotation-modes">
            <p class="api-endpoint-label">Scheduling Mode</p>
            <div class="api-rotation-mode-list">
              {#each schedulingModeCards as card}
                <button
                  type="button"
                  class={`api-rotation-mode-card ${schedulingModeInput === card.id ? 'is-active' : ''}`}
                  on:click={() => void applySchedulingMode(card.id)}
                  disabled={busy}
                >
                  <span class="api-rotation-mode-title">{card.label}</span>
                  <span class="api-rotation-mode-desc">{card.description}</span>
                </button>
              {/each}
            </div>
          </div>

          <div class="api-rotation-side">
            <div class="api-rotation-side-card api-rotation-info ui-panel-soft">
              <p>
                {#if schedulingModeInput === 'cache_first'}
                  Prioritizes bound sessions to maximize cache hit continuity.
                {:else if schedulingModeInput === 'balance'}
                  Favors accounts with lower request/error load for balanced utilization.
                {:else}
                  Uses round-robin order for low-latency throughput at high concurrency.
                {/if}
              </p>
            </div>

            <div class="api-rotation-side-card api-rotation-breaker ui-panel-soft">
              <ToggleSwitch
                label="Circuit Breaker (staged cooldown after repeated failures)"
                bind:checked={circuitBreakerInput}
                on:change={updateCircuitBreaker}
                disabled={busy}
              />
              <p class="api-rotation-footnote">Steps apply in order: #1, #2, #3, then remain on step #3.</p>
              <p class="api-rotation-footnote">Exhausted/usage-limit errors skip to the next account and do not consume circuit steps.</p>
            </div>

            <div class="api-rotation-side-card api-rotation-steps ui-panel-soft">
              <p class="api-endpoint-label">Circuit Steps (seconds)</p>
              <div class="api-rotation-steps-grid">
                <label class="api-rotation-step-item">
                  <span>Step 1</span>
                  <input
                    type="number"
                    min="1"
                    max="3600"
                    class="ui-control-input ui-control-select-sm"
                    value={circuitStepInputs[0]}
                    on:input={(event) => handleCircuitStepInput(0, event)}
                    disabled={busy}
                  />
                </label>

                <label class="api-rotation-step-item">
                  <span>Step 2</span>
                  <input
                    type="number"
                    min="1"
                    max="3600"
                    class="ui-control-input ui-control-select-sm"
                    value={circuitStepInputs[1]}
                    on:input={(event) => handleCircuitStepInput(1, event)}
                    disabled={busy}
                  />
                </label>

                <label class="api-rotation-step-item">
                  <span>Step 3</span>
                  <input
                    type="number"
                    min="1"
                    max="3600"
                    class="ui-control-input ui-control-select-sm"
                    value={circuitStepInputs[2]}
                    on:input={(event) => handleCircuitStepInput(2, event)}
                    disabled={busy}
                  />
                </label>
              </div>

              {#if schedulingError}
                <p class="api-rotation-error">{schedulingError}</p>
              {/if}

              <Button variant="secondary" size="sm" on:click={applyCircuitSteps} disabled={busy || !circuitStepsDirty}>
                Apply Steps
              </Button>
            </div>
          </div>
        </div>
      </div>
    {/if}
  </SurfaceCard>
</div>
