<script lang="ts">
  import { RefreshCw, Save } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import ProxyInlineSwitch from '@/features/router/components/proxy/ProxyInlineSwitch.svelte'
  import type { ProxyStatus } from '@/features/router/types'

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let onStartProxy: () => Promise<void>
  export let onStopProxy: () => Promise<void>
  export let onSetProxyPort: (port: number) => Promise<void>
  export let onSetAllowLAN: (enabled: boolean) => Promise<void>
  export let onSetAutoStartProxy: (enabled: boolean) => Promise<void>

  let portInput = '8095'
  let portInputDirty = false
  let proxyRunningInput = false
  let allowLanInput = false
  let autoStartProxyInput = true

  $: if (proxyStatus && !portInputDirty) {
    portInput = String(proxyStatus.port)
  }

  $: if (proxyStatus && !busy) {
    proxyRunningInput = proxyStatus.running
    allowLanInput = proxyStatus.allowLan
    autoStartProxyInput = proxyStatus.autoStartProxy
  }

  $: endpointLabel = proxyStatus?.url ? proxyStatus.url.replace(/^https?:\/\//, '') : `${proxyStatus?.bindAddress || '127.0.0.1:8095'}/v1`

  const applyProxyPort = async (): Promise<void> => {
    const parsedPort = Number.parseInt(portInput.trim(), 10)
    const nextPort = Number.isFinite(parsedPort) && parsedPort >= 1024 && parsedPort <= 65535 ? parsedPort : 8095
    portInput = String(nextPort)
    portInputDirty = false
    await onSetProxyPort(nextPort)
  }

  const resetProxyPort = (): void => {
    portInput = String(proxyStatus?.port || 8095)
    portInputDirty = false
  }

  const handleRuntimeChange = async (event: CustomEvent<boolean>): Promise<void> => {
    const nextRunning = event.detail
    const previous = proxyRunningInput
    proxyRunningInput = nextRunning

    try {
      if (nextRunning) {
        await onStartProxy()
      } else {
        await onStopProxy()
      }
    } catch {
      proxyRunningInput = previous
    }
  }

  const handleAllowLanChange = async (event: CustomEvent<boolean>): Promise<void> => {
    allowLanInput = event.detail
    await onSetAllowLAN(allowLanInput)
  }

  const handleAutoStartChange = async (event: CustomEvent<boolean>): Promise<void> => {
    autoStartProxyInput = event.detail
    await onSetAutoStartProxy(autoStartProxyInput)
  }
</script>

<section class={`proxy-panel proxy-runtime-card ${proxyStatus?.running ? 'is-live' : ''}`}>
  <div class="proxy-panel-header">
    <div>
      <p class="proxy-panel-kicker">Runtime Control</p>
      <h4 class="proxy-panel-title">Listener, exposure, startup</h4>
    </div>
    {#if busy}
      <RefreshCw size={14} class="proxy-panel-spinner animate-spin" />
    {/if}
  </div>

  <div class="proxy-runtime-row proxy-runtime-row-primary">
    <div class="proxy-field-copy">
      <span class="proxy-field-label">Proxy runtime</span>
      <p class="proxy-field-description">Bring the local endpoint online or take it offline.</p>
    </div>
    <div class="proxy-runtime-primary-control">
      <StatusBadge tone={proxyRunningInput ? 'success' : 'warning'}>{proxyRunningInput ? 'Online' : 'Offline'}</StatusBadge>
      <ProxyInlineSwitch checked={proxyRunningInput} on:change={handleRuntimeChange} disabled={busy} />
    </div>
  </div>

  <div class="proxy-runtime-stack">
    <div class="proxy-field-card">
      <div class="proxy-field-copy">
        <span class="proxy-field-label">Port</span>
        <p class="proxy-field-description">Endpoint {endpointLabel}</p>
      </div>

      <div class="proxy-port-control">
        <input
          id="router-port"
          class="ui-control-input ui-control-select proxy-port-input"
          bind:value={portInput}
          on:input={() => {
            portInputDirty = true
          }}
          on:keydown={(event) => {
            if (event.key === 'Enter') {
              void applyProxyPort()
            }
            if (event.key === 'Escape') {
              resetProxyPort()
            }
          }}
          type="text"
          inputmode="numeric"
          disabled={busy}
        />

        <Button variant="secondary" size="sm" className="proxy-port-button" on:click={applyProxyPort} disabled={busy || !portInputDirty}>
          <Save size={13} class="mr-1.5" /> Apply
        </Button>
      </div>
    </div>

    <div class="proxy-setting-card">
      <div class="proxy-setting-copy">
        <span class="proxy-field-label">Allow on LAN</span>
        <p class="proxy-field-description">Expose the listener to your local network.</p>
      </div>

      <div class="proxy-setting-control">
        <span class="proxy-setting-state">{allowLanInput ? 'LAN + Localhost' : 'Localhost Only'}</span>
        <ProxyInlineSwitch checked={allowLanInput} on:change={handleAllowLanChange} disabled={busy} />
      </div>
    </div>

    <div class="proxy-setting-card">
      <div class="proxy-setting-copy">
        <span class="proxy-field-label">Auto Start Proxy</span>
        <p class="proxy-field-description">Start the proxy automatically with the desktop app.</p>
      </div>

      <div class="proxy-setting-control">
        <span class="proxy-setting-state">{autoStartProxyInput ? 'Boot On Launch' : 'Manual Start'}</span>
        <ProxyInlineSwitch checked={autoStartProxyInput} on:change={handleAutoStartChange} disabled={busy} />
      </div>
    </div>
  </div>
</section>

<style>
  .proxy-panel {
    display: flex;
    flex-direction: column;
    min-width: 0;
    border-radius: 0.92rem;
    border: 1px solid color-mix(in srgb, #f59e0b 28%, var(--color-border));
    padding: 0.82rem 0.88rem;
    background:
      linear-gradient(180deg, color-mix(in srgb, white 3%, transparent), transparent 34%),
      color-mix(in srgb, var(--color-app) 93%, white 2%);
    box-shadow:
      inset 0 1px 0 color-mix(in srgb, white 4%, transparent),
      0 14px 24px rgba(0, 0, 0, 0.08);
  }

  .proxy-runtime-card.is-live {
    border-color: color-mix(in srgb, #f59e0b 42%, var(--color-border));
  }

  .proxy-panel-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.55rem;
  }

  .proxy-panel-kicker {
    font-size: 0.62rem;
    font-weight: 800;
    letter-spacing: 0.13em;
    text-transform: uppercase;
    color: color-mix(in srgb, #f59e0b 62%, var(--color-text-secondary));
  }

  .proxy-panel-title {
    margin-top: 0.22rem;
    font-size: 0.92rem;
    line-height: 1.2;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--color-text-primary);
  }

  .proxy-panel-spinner {
    color: var(--color-text-secondary);
    flex-shrink: 0;
  }

  .proxy-runtime-row-primary,
  .proxy-field-card,
  .proxy-setting-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.65rem;
    padding: 0.62rem 0;
    border-bottom: 1px solid color-mix(in srgb, #f59e0b 12%, var(--color-border));
  }

  .proxy-runtime-stack > :last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  .proxy-runtime-row-primary {
    padding-top: 0.15rem;
    margin-bottom: 0.06rem;
  }

  .proxy-field-label {
    display: block;
    font-size: 0.6rem;
    font-weight: 700;
    letter-spacing: 0.11em;
    text-transform: uppercase;
    color: var(--color-text-secondary);
  }

  .proxy-runtime-primary-control {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .proxy-runtime-stack {
    display: grid;
    gap: 0;
    margin-top: 0.18rem;
  }

  .proxy-field-copy,
  .proxy-setting-copy {
    min-width: 0;
  }

  .proxy-field-description {
    margin-top: 0.18rem;
    max-width: 22rem;
    font-size: 0.7rem;
    line-height: 1.4;
    color: var(--color-text-secondary);
  }

  .proxy-port-control {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .proxy-port-input {
    width: 5.4rem;
    text-align: center;
    font-family: 'IBM Plex Mono', 'Consolas', monospace;
    font-size: 0.78rem;
    background: color-mix(in srgb, var(--color-surface) 94%, white 4%);
  }

  :global(.proxy-port-button) {
    min-width: 5rem;
  }

  .proxy-setting-control {
    display: inline-flex;
    align-items: center;
    gap: 0.55rem;
    flex-shrink: 0;
  }

  .proxy-setting-state {
    font-size: 0.66rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-text-primary);
  }

  @media (max-width: 767px) {
    .proxy-runtime-row-primary,
    .proxy-field-card,
    .proxy-setting-card {
      flex-direction: column;
      align-items: stretch;
    }

    .proxy-runtime-primary-control,
    .proxy-port-control,
    .proxy-setting-control {
      width: 100%;
      justify-content: space-between;
    }

    .proxy-port-input,
    .proxy-port-button {
      width: 100%;
    }
  }
</style>
