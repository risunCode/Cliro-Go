<script lang="ts">
  import { ChevronDown, ChevronUp, Network, Play, Power, PowerOff, RefreshCw, Save } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import ToggleSwitch from '@/components/common/ToggleSwitch.svelte'
  import type { ProxyStatus } from '@/services/wails-api'

  interface EndpointPreset {
    id: string
    label: string
    method: 'GET' | 'POST'
    path: string
    defaultBody: string
  }

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let onRefreshStatus: () => Promise<void>
  export let onStartProxy: () => Promise<void>
  export let onStopProxy: () => Promise<void>
  export let onSetProxyPort: (port: number) => Promise<void>
  export let onSetAllowLAN: (enabled: boolean) => Promise<void>
  export let onSetAutoStartProxy: (enabled: boolean) => Promise<void>

  const endpointPresets: EndpointPreset[] = [
    { id: 'health', label: 'GET /health', method: 'GET', path: '/health', defaultBody: '' },
    { id: 'models', label: 'GET /v1/models', method: 'GET', path: '/v1/models', defaultBody: '' },
    { id: 'stats', label: 'GET /v1/stats', method: 'GET', path: '/v1/stats', defaultBody: '' },
    {
      id: 'chat-completions',
      label: 'POST /v1/chat/completions',
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
      label: 'POST /v1/completions',
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
    }
  ]

  let portInput = '8095'
  let portInputDirty = false
  let allowLanInput = false
  let autoStartProxyInput = true
  let selectedEndpointId = endpointPresets[0].id
  let selectedEndpoint: EndpointPreset = endpointPresets[0]
  let requestBody = endpointPresets[0].defaultBody
  let testerLoading = false
  let testerStatus = '-'
  let testerResponse = ''
  let testerError = ''
  let testerExpanded = false

  $: if (proxyStatus?.port && !portInputDirty) {
    portInput = String(proxyStatus.port)
  }

  $: if (proxyStatus) {
    allowLanInput = proxyStatus.allowLan
    autoStartProxyInput = proxyStatus.autoStartProxy
  }

  $: selectedEndpoint = endpointPresets.find((endpoint) => endpoint.id === selectedEndpointId) || endpointPresets[0]

  $: if (selectedEndpoint && selectedEndpoint.method === 'GET') {
    requestBody = ''
  }

  const applySelectedEndpoint = (): void => {
    if (selectedEndpoint.method === 'POST') {
      requestBody = selectedEndpoint.defaultBody
    }
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

    try {
      const target = `${proxyStatus.url}${selectedEndpoint.path}`
      const options: RequestInit = {
        method: selectedEndpoint.method,
        headers: {}
      }

      if (selectedEndpoint.method === 'POST') {
        ;(options.headers as Record<string, string>)['Content-Type'] = 'application/json'
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

  const updateAllowLan = async (): Promise<void> => {
    await onSetAllowLAN(allowLanInput)
  }

  const updateAutoStartProxy = async (): Promise<void> => {
    await onSetAutoStartProxy(autoStartProxyInput)
  }
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
        <Button variant="secondary" size="sm" on:click={onRefreshStatus} disabled={busy}>
          <RefreshCw size={14} class="mr-1" />
          Refresh
        </Button>
      </div>
    </div>

    <div class="grid gap-3 lg:grid-cols-2">
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
        <p class="mt-2 truncate text-xs text-text-secondary">Active URL: {proxyStatus?.url || '-'}</p>
      </div>

      <div class="rounded-sm border border-border bg-app p-3">
        <p class="mb-2 text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Port</p>
        <div class="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end">
          <input
            id="router-port"
            class="h-10 rounded-sm border border-border bg-surface px-3 text-sm text-text-primary outline-none focus:border-text-secondary"
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

      <div class="rounded-sm border border-border bg-app p-3 lg:col-span-2">
        <p class="mb-2 text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Network & Startup</p>
        <div class="grid gap-2 sm:grid-cols-2">
          <ToggleSwitch label="Allow on LAN" bind:checked={allowLanInput} on:change={updateAllowLan} disabled={busy} />
          <ToggleSwitch
            label="Auto Start on launch"
            bind:checked={autoStartProxyInput}
            on:change={updateAutoStartProxy}
            disabled={busy}
          />
        </div>
      </div>
    </div>
  </SurfaceCard>

  <SurfaceCard className="p-4">
    <button
      type="button"
      class="mb-3 flex w-full items-center justify-between rounded-sm border border-transparent px-1 py-1 text-left transition hover:border-border hover:bg-app"
      on:click={toggleTesterExpanded}
      aria-expanded={testerExpanded}
      aria-label="Toggle endpoint tester"
    >
      <div class="flex items-center gap-2">
        <Network size={15} class="text-text-secondary" />
        <p class="text-sm font-semibold text-text-primary">Endpoint Tester</p>
      </div>
      <div class="flex items-center gap-2">
        <StatusBadge tone="info">Proxy Integration</StatusBadge>
        {#if testerExpanded}
          <ChevronUp size={14} class="text-text-secondary" />
        {:else}
          <ChevronDown size={14} class="text-text-secondary" />
        {/if}
      </div>
    </button>

    {#if testerExpanded}
      <div class="grid gap-3 lg:grid-cols-2">
        <div class="space-y-3">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
            <select
              bind:value={selectedEndpointId}
              class="h-10 rounded-sm border border-border bg-surface px-3 text-sm text-text-primary outline-none focus:border-text-secondary sm:flex-1"
              on:change={applySelectedEndpoint}
            >
              {#each endpointPresets as endpoint}
                <option value={endpoint.id}>{endpoint.label}</option>
              {/each}
            </select>
            <Button
              variant="primary"
              size="sm"
              className="self-start whitespace-nowrap"
              on:click={runEndpointTest}
              disabled={testerLoading || !proxyStatus?.running}
            >
              <Play size={14} class="mr-1" />
              Run Test
            </Button>
          </div>

          <div class="rounded-sm border border-border bg-app p-3 text-xs">
            <p class="mb-2 text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Request Payload</p>
            {#if selectedEndpoint.method === 'POST'}
              <textarea
                bind:value={requestBody}
                class="min-h-[12rem] w-full rounded-sm border border-border bg-surface p-3 font-mono text-xs text-text-primary outline-none focus:border-text-secondary"
              ></textarea>
            {:else}
              <p class="text-text-secondary">This endpoint is GET and does not require a request body.</p>
            {/if}
          </div>
        </div>

        <div class="rounded-sm border border-border bg-app p-3 text-xs">
          <p class="mb-2 text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">Response</p>
          <div class="mb-2 flex items-center gap-2">
            <StatusBadge tone={testerStatus.startsWith('2') ? 'success' : testerStatus === '-' ? 'neutral' : 'warning'}>{testerStatus}</StatusBadge>
            {#if testerLoading}
              <span class="text-text-secondary">Request in progress...</span>
            {/if}
          </div>
          {#if testerError}
            <p class="text-error">{testerError}</p>
          {:else if testerResponse}
            <pre class="no-scrollbar max-h-[20rem] overflow-auto whitespace-pre-wrap break-words text-text-secondary">{testerResponse}</pre>
          {:else}
            <p class="text-text-secondary">Run a request to inspect proxy responses.</p>
          {/if}
        </div>
      </div>
    {/if}
  </SurfaceCard>
</div>
