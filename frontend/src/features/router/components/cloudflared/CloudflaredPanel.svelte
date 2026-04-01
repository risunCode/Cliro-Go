<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Cloud, Copy, RefreshCw } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import CollapsibleSurfaceSection from '@/components/common/CollapsibleSurfaceSection.svelte'
  import ToggleSwitch from '@/components/common/ToggleSwitch.svelte'
  import type { ProxyStatus } from '@/features/router/types'
  import { copyTextToClipboard } from '@/shared/lib/browser'
  import { CLOUDFLARED_MODE_CARDS, type CloudflaredMode } from '@/features/router/lib/cloudflared'

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let onRefreshCloudflaredStatus: () => Promise<void>
  export let onSetCloudflaredConfig: (mode: string, token: string, useHttp2: boolean) => Promise<void>
  export let onInstallCloudflared: () => Promise<void>
  export let onStartCloudflared: () => Promise<void>
  export let onStopCloudflared: () => Promise<void>

  let expanded = false
  let wasExpanded = false
  let modeInput: CloudflaredMode = 'quick'
  let tokenInput = ''
  let useHTTP2Input = true
  let configDirty = false
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let refreshPending = false

  $: canStart = modeInput !== 'auth' || tokenInput.trim().length > 0
  $: toggleDisabled = busy || (!(proxyStatus?.cloudflared.enabled ?? false) && (!proxyStatus?.running || !canStart))

  $: if (proxyStatus && !busy && !configDirty) {
    modeInput = proxyStatus.cloudflared.mode === 'auth' ? 'auth' : 'quick'
    tokenInput = proxyStatus.cloudflared.token || ''
    useHTTP2Input = proxyStatus.cloudflared.useHttp2
  }

  const refreshStatus = async (): Promise<void> => {
    if (refreshPending) {
      return
    }

    refreshPending = true
    try {
      await onRefreshCloudflaredStatus()
    } finally {
      refreshPending = false
    }
  }

  const startPolling = (): void => {
    if (pollTimer) {
      return
    }

    pollTimer = setInterval(() => {
      void refreshStatus().catch(() => {})
    }, 5000)
  }

  const stopPolling = (): void => {
    if (!pollTimer) {
      return
    }

    clearInterval(pollTimer)
    pollTimer = null
  }

  $: if (expanded && !wasExpanded) {
    void refreshStatus().catch(() => {})
    startPolling()
  }

  $: if (!expanded && wasExpanded) {
    stopPolling()
  }

  $: wasExpanded = expanded

  const persistConfig = async (): Promise<void> => {
    configDirty = true
    try {
      await onSetCloudflaredConfig(modeInput, tokenInput, useHTTP2Input)
    } finally {
      configDirty = false
    }
  }

  const selectMode = async (mode: CloudflaredMode): Promise<void> => {
    if (mode === modeInput) {
      return
    }

    modeInput = mode
    await persistConfig()
  }

  const updateToken = (event: Event): void => {
    tokenInput = (event.currentTarget as HTMLInputElement).value
    configDirty = true
  }

  const saveToken = async (): Promise<void> => {
    await persistConfig()
  }

  const updateHTTP2 = async (): Promise<void> => {
    configDirty = true
    await persistConfig()
  }

  const toggleCloudflared = async (): Promise<void> => {
    if (proxyStatus?.cloudflared.enabled) {
      await onStopCloudflared()
      return
    }

    await persistConfig()
    if (!proxyStatus?.cloudflared.installed) {
      await onInstallCloudflared()
    }
    await onStartCloudflared()
  }

  const copyURL = async (): Promise<void> => {
    if (proxyStatus?.cloudflared.url) {
      await copyTextToClipboard(proxyStatus.cloudflared.url)
    }
  }

  onDestroy(() => {
    stopPolling()
  })
</script>

<CollapsibleSurfaceSection
  bind:open={expanded}
  icon={Cloud}
  iconClassName="text-orange-400"
  title="Public Access (Cloudflared)"
  subtitle="Expose the local proxy through a Cloudflare tunnel with quick or named tunnel mode."
  pill={proxyStatus?.cloudflared.running ? 'Running' : proxyStatus?.cloudflared.enabled ? 'Enabled' : 'Disabled'}
  ariaLabel="Toggle public access cloudflared settings"
  bodyClassName="api-cli-sync-body space-y-4"
>
  <svelte:fragment slot="headerRight">
    {#if busy}
      <RefreshCw size={14} class="animate-spin text-text-secondary" />
    {/if}
  </svelte:fragment>

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
          disabled={toggleDisabled || (!(proxyStatus?.cloudflared.installed ?? false) && !(proxyStatus?.cloudflared.enabled ?? false))}
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
          {#each CLOUDFLARED_MODE_CARDS as card}
            <button
              type="button"
              class={`api-rotation-mode-card ${modeInput === card.id ? 'is-active' : ''}`}
              on:click={() => void selectMode(card.id)}
              disabled={busy || (proxyStatus?.cloudflared.running ?? false)}
            >
              <span class="api-rotation-mode-title">{card.label}</span>
              <span class="api-rotation-mode-desc">{card.description}</span>
            </button>
          {/each}
        </div>

        {#if modeInput === 'auth'}
          <div class="mt-3 space-y-2 rounded-sm border border-border bg-surface/50 p-3">
            <p class="api-endpoint-label">Tunnel Token</p>
            <input
              type="password"
              value={tokenInput}
              on:input={updateToken}
              on:blur={saveToken}
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
          label={useHTTP2Input ? 'HTTP/2 enabled' : 'HTTP/2 disabled'}
          bind:checked={useHTTP2Input}
          on:change={updateHTTP2}
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
          <Button variant="secondary" size="sm" on:click={copyURL} disabled={!proxyStatus.cloudflared.url}>
            <Copy size={13} class="mr-1" />
            Copy
          </Button>
        </div>
      {:else}
        <div class="rounded-sm border border-border bg-surface/40 px-3 py-2.5 text-[11px] leading-5 text-text-secondary">
          {#if !proxyStatus?.running && !(proxyStatus?.cloudflared.enabled ?? false)}
            Start the local proxy first before enabling Cloudflared public access.
          {:else if modeInput === 'auth' && !canStart && !(proxyStatus?.cloudflared.enabled ?? false)}
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
</CollapsibleSurfaceSection>
