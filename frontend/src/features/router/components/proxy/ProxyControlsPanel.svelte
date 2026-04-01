<script lang="ts">
  import ControlWorkspaceCard from '@/components/common/ControlWorkspaceCard.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import ProxyRuntimeCard from '@/features/router/components/proxy/ProxyRuntimeCard.svelte'
  import ProxySecurityCard from '@/features/router/components/proxy/ProxySecurityCard.svelte'
  import type { ProxyStatus } from '@/features/router/types'

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

  $: running = proxyStatus?.running ?? false
  $: bindLabel = proxyStatus?.bindAddress || '-'
</script>

<ControlWorkspaceCard
  kicker="Proxy Service"
  title="Proxy service"
  subtitle="Dense controls for runtime, exposure, startup, and request protection."
  accent="amber"
  className="proxy-workspace-shell"
>
  <svelte:fragment slot="headerAside">
    <div class="proxy-header-stack">
      <StatusBadge tone={running ? 'success' : 'warning'} className="proxy-status-badge">
        {running ? 'Running' : 'Standby'}
      </StatusBadge>
      <div class="proxy-header-chip">
        <span class="proxy-header-chip-label">Bind</span>
        <strong class="proxy-header-chip-value">{bindLabel}</strong>
      </div>
    </div>
  </svelte:fragment>

  <div class="proxy-card-grid">
    <ProxyRuntimeCard
      {proxyStatus}
      {busy}
      {onStartProxy}
      {onStopProxy}
      {onSetProxyPort}
      {onSetAllowLAN}
      {onSetAutoStartProxy}
    />

    <ProxySecurityCard
      {proxyStatus}
      {busy}
      {onSetProxyAPIKey}
      {onRegenerateProxyAPIKey}
      {onSetAuthorizationMode}
    />
  </div>
</ControlWorkspaceCard>

<style>
  :global(.control-workspace-card.proxy-workspace-shell) {
    border-color: color-mix(in srgb, #f59e0b 22%, var(--color-border));
    background: color-mix(in srgb, var(--color-surface) 98%, #f59e0b 2%);
    box-shadow:
      inset 0 1px 0 color-mix(in srgb, white 6%, transparent),
      0 18px 34px rgba(0, 0, 0, 0.1);
  }

  :global(.control-workspace-card.proxy-workspace-shell .workspace-shell) {
    background:
      linear-gradient(180deg, color-mix(in srgb, #f59e0b 6%, transparent) 0%, transparent 26%),
      linear-gradient(180deg, color-mix(in srgb, white 4%, transparent) 0%, transparent 100%),
      color-mix(in srgb, var(--color-surface) 98%, #f59e0b 2%);
  }

  :global(.control-workspace-card.proxy-workspace-shell .workspace-header) {
    border-bottom-color: color-mix(in srgb, #f59e0b 14%, var(--color-border));
  }

  .proxy-card-grid {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .proxy-header-stack {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .proxy-header-chip {
    display: inline-grid;
    gap: 0.18rem;
    min-width: 7.6rem;
    padding: 0.36rem 0.55rem;
    border: 1px solid color-mix(in srgb, #f59e0b 18%, var(--color-border));
    border-radius: 0.72rem;
    background:
      linear-gradient(180deg, color-mix(in srgb, white 3%, transparent), transparent),
      color-mix(in srgb, var(--color-app) 92%, white 2%);
  }

  .proxy-header-chip-label {
    font-size: 0.6rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-text-secondary);
  }

  .proxy-header-chip-value {
    font-family: 'IBM Plex Mono', 'Consolas', monospace;
    font-size: 0.7rem;
    color: var(--color-text-primary);
    word-break: break-all;
  }

  :global(.proxy-status-badge) {
    min-width: 5.2rem;
    justify-content: center;
  }

  @media (max-width: 1023px) {
    .proxy-card-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 767px) {
    .proxy-header-stack {
      align-items: stretch;
    }

    .proxy-header-chip {
      min-width: 0;
      width: 100%;
    }
  }
</style>
