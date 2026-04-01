<script lang="ts">
  import { onMount } from 'svelte'

  import codexIcon from '@/assets/icons/codex-icon.png'
  import cliroIcon from '@/assets/icons/cliro-icon.png'
  import kiroIcon from '@/assets/icons/kiro-icon.png'
  import type { AppState, LogEntry } from '@/app/types'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import type { Account } from '@/features/accounts/types'
  import type { ProxyStatus } from '@/features/router/types'
  import { formatNumber } from '@/shared/lib/formatters'
  import {
    formatRelativeTime,
    getEnabledAccountCount,
    getLastActiveAt,
    getProviderRequestCount,
    getRecentRequests,
    getRequestLogs,
    isProviderActive,
    providerLabel,
    type UsageProvider
  } from '@/features/usage/lib/request-log'

  interface ProviderNode {
    id: UsageProvider
    label: string
    subtitle: string
    icon: string
    enabledAccounts: number
    requests: number
    active: boolean
  }

  export let state: AppState | null = null
  export let accounts: Account[] = []
  export let proxyStatus: ProxyStatus | null = null
  export let logs: LogEntry[] = []

  let now = Date.now()

  onMount(() => {
    const timer = window.setInterval(() => {
      now = Date.now()
    }, 1000)

    return () => {
      window.clearInterval(timer)
    }
  })

  $: stats = state?.stats
  $: totalRequests = stats?.totalRequests ?? 0
  $: successRequests = stats?.successRequests ?? 0
  $: failedRequests = stats?.failedRequests ?? 0
  $: promptTokens = stats?.promptTokens ?? 0
  $: completionTokens = stats?.completionTokens ?? 0
  $: totalTokens = stats?.totalTokens ?? 0
  $: successRate = totalRequests > 0 ? (successRequests / totalRequests) * 100 : 0
  $: proxyOnline = proxyStatus?.running ?? state?.proxyRunning ?? false
  $: requestLogs = getRequestLogs(logs)
  $: recentRequests = getRecentRequests(requestLogs)
  $: codexActiveAt = getLastActiveAt(recentRequests, 'codex')
  $: kiroActiveAt = getLastActiveAt(recentRequests, 'kiro')
  $: codexActive = isProviderActive(proxyOnline, codexActiveAt, now)
  $: kiroActive = isProviderActive(proxyOnline, kiroActiveAt, now)
  $: codexAccounts = getEnabledAccountCount(accounts, 'codex')
  $: kiroAccounts = getEnabledAccountCount(accounts, 'kiro')
  $: codexRequests = getProviderRequestCount(requestLogs, 'codex')
  $: kiroRequests = getProviderRequestCount(requestLogs, 'kiro')
  $: providerNodes = [
    { id: 'codex', label: 'Codex', subtitle: 'OpenAI Codex', icon: codexIcon, enabledAccounts: codexAccounts, requests: codexRequests, active: codexActive },
    { id: 'kiro', label: 'Kiro', subtitle: 'Amazon Kiro', icon: kiroIcon, enabledAccounts: kiroAccounts, requests: kiroRequests, active: kiroActive }
  ] satisfies ProviderNode[]
</script>

<div class="usage-shell space-y-2.5">
  <div class="grid gap-2.5 md:grid-cols-2 lg:grid-cols-4">
    <SurfaceCard className="usage-metric-card p-3.5">
      <p class="usage-metric-label">Total Requests</p>
      <p class="usage-metric-value">{formatNumber(totalRequests)}</p>
    </SurfaceCard>

    <SurfaceCard className="usage-metric-card p-3.5">
      <p class="usage-metric-label">Input Tokens</p>
      <p class="usage-metric-value usage-metric-value-accent">{formatNumber(promptTokens)}</p>
    </SurfaceCard>

    <SurfaceCard className="usage-metric-card p-3.5">
      <p class="usage-metric-label">Output Tokens</p>
      <p class="usage-metric-value">{formatNumber(completionTokens)}</p>
    </SurfaceCard>

    <SurfaceCard className="usage-metric-card p-3.5">
      <p class="usage-metric-label">Success Rate</p>
      <p class="usage-metric-value">{successRate.toFixed(1)}%</p>
      <p class="mt-0.5 text-[10px] text-text-secondary">{formatNumber(failedRequests)} failed · {formatNumber(totalTokens)} total tokens</p>
    </SurfaceCard>
  </div>

  <div class="grid gap-2.5 lg:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.9fr)]">
    <SurfaceCard className="usage-flow-card overflow-hidden p-0">
      <div class="usage-panel-head">
        <p class="usage-panel-title">Provider Flow</p>
        <StatusBadge tone="neutral">Live Routing</StatusBadge>
      </div>

      <div class="usage-flow-map">
        <svg class="usage-flow-svg" viewBox="0 0 860 420" preserveAspectRatio="none" aria-hidden="true">
          <path d="M198 210 C 278 210, 336 210, 392 210" class={`usage-flow-line ${codexActive ? 'is-active' : ''}`} />
          <path d="M468 210 C 524 210, 582 210, 662 210" class={`usage-flow-line ${kiroActive ? 'is-active' : ''}`} />
        </svg>

        <article class="usage-provider-node usage-provider-node-left">
          <div class="usage-provider-icon-shell">
            <img src={providerNodes[0].icon} alt={providerNodes[0].label} class="usage-provider-icon" />
          </div>
          <div>
            <p class="usage-node-title">{providerNodes[0].label}</p>
            <p class="usage-node-subtitle">{providerNodes[0].subtitle}</p>
            <p class="usage-node-meta">{formatNumber(providerNodes[0].enabledAccounts)} enabled · {formatNumber(providerNodes[0].requests)} req</p>
          </div>
        </article>

        <article class="usage-center-node">
          <div class="usage-center-ring"></div>
          <div class="usage-provider-icon-shell usage-center-icon-shell">
            <img src={cliroIcon} alt="CLIRO" class="usage-provider-icon" />
          </div>
          <p class="usage-center-title">CLIRO</p>
          <p class="usage-center-meta">Local proxy router</p>
        </article>

        <article class="usage-provider-node usage-provider-node-right">
          <div class="usage-provider-icon-shell">
            <img src={providerNodes[1].icon} alt={providerNodes[1].label} class="usage-provider-icon" />
          </div>
          <div>
            <p class="usage-node-title">{providerNodes[1].label}</p>
            <p class="usage-node-subtitle">{providerNodes[1].subtitle}</p>
            <p class="usage-node-meta">{formatNumber(providerNodes[1].enabledAccounts)} enabled · {formatNumber(providerNodes[1].requests)} req</p>
          </div>
        </article>
      </div>
    </SurfaceCard>

    <SurfaceCard className="usage-log-card overflow-hidden p-0">
      <div class="usage-panel-head">
        <p class="usage-panel-title">Recent Requests</p>
        <StatusBadge tone="neutral">10 Max</StatusBadge>
      </div>

      <div class="usage-log-table-head">
        <span>Model</span>
        <span>Account</span>
        <span>In / Out</span>
        <span>When</span>
      </div>

      <div class="usage-log-list no-scrollbar">
        {#each recentRequests as request (`${request.timestamp}-${request.provider}-${request.model}`)}
          <div class="usage-log-row">
            <div class="usage-log-model">
              <p>{request.model}</p>
              <span>{providerLabel(request.provider)}</span>
            </div>
            <div class="usage-log-account" title={request.account}>{request.account}</div>
            <div class="usage-log-tokens">
              <span>{formatNumber(request.promptTokens)}</span>
              <span>{formatNumber(request.completionTokens)}</span>
            </div>
            <div class="usage-log-when">{formatRelativeTime(request.timestamp, now)}</div>
          </div>
        {:else}
          <div class="usage-log-empty">No routed request logs yet.</div>
        {/each}
      </div>
    </SurfaceCard>
  </div>
</div>

<style>
  .usage-status-card,
  .usage-metric-card,
  .usage-flow-card,
  .usage-log-card {
    background: radial-gradient(circle at top, color-mix(in srgb, var(--color-surface) 94%, rgba(255, 255, 255, 0.08)), color-mix(in srgb, var(--color-bg) 92%, transparent));
  }

  .usage-metric-label,
  .usage-panel-title {
    font-size: 0.7rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-text-secondary);
  }

  .usage-metric-value {
    margin-top: 0.4rem;
    font-size: 1.7rem;
    line-height: 1;
    font-weight: 800;
    color: var(--color-text-primary);
  }

  .usage-metric-value-accent {
    color: color-mix(in srgb, var(--accent-primary) 84%, #f8fafc);
  }

  .usage-panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.75rem 0.85rem;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 85%, transparent);
  }

  .usage-flow-map {
    position: relative;
    min-height: 340px;
    overflow: hidden;
    background:
      radial-gradient(circle at center, rgba(255, 255, 255, 0.03), transparent 48%),
      linear-gradient(180deg, rgba(255, 255, 255, 0.015), rgba(255, 255, 255, 0.005));
  }

  .usage-flow-svg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }

  .usage-flow-line {
    fill: none;
    stroke: color-mix(in srgb, var(--color-border) 60%, transparent);
    stroke-width: 2.2;
    stroke-linecap: round;
    stroke-dasharray: 2 8;
    opacity: 0.6;
  }

  .usage-flow-line.is-active {
    stroke: color-mix(in srgb, var(--accent-primary) 75%, #f8fafc);
    opacity: 1;
    animation: usage-flow-dash 0.9s linear infinite;
  }

  .usage-provider-node,
  .usage-center-node {
    position: absolute;
    display: flex;
    align-items: center;
    gap: 0.65rem;
    border: 1px solid color-mix(in srgb, var(--color-border) 82%, transparent);
    border-radius: 12px;
    background: color-mix(in srgb, var(--color-surface) 92%, rgba(255, 255, 255, 0.04));
    box-shadow: 0 8px 22px rgba(15, 23, 42, 0.14);
    padding: 0.62rem 0.72rem;
  }

  .usage-provider-node-left {
    left: 6%;
    top: 50%;
    transform: translateY(-50%);
  }

  .usage-provider-node-right {
    right: 6%;
    top: 50%;
    transform: translateY(-50%);
  }

  .usage-center-node {
    left: 50%;
    top: 50%;
    width: 148px;
    flex-direction: column;
    justify-content: center;
    gap: 0.42rem;
    text-align: center;
    transform: translate(-50%, -50%);
    padding: 0.82rem 0.72rem;
    background: color-mix(in srgb, var(--color-surface) 88%, rgba(255, 255, 255, 0.05));
  }

  .usage-center-ring {
    position: absolute;
    inset: -18px;
    border: 1px dashed color-mix(in srgb, var(--accent-primary) 32%, transparent);
    border-radius: 999px;
    opacity: 0.6;
  }

  .usage-provider-icon-shell {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    min-width: 36px;
    border-radius: 10px;
    border: 1px solid color-mix(in srgb, var(--color-border) 82%, transparent);
    background: color-mix(in srgb, var(--color-bg) 88%, transparent);
  }

  .usage-center-icon-shell {
    width: 44px;
    height: 44px;
    min-width: 44px;
    z-index: 1;
  }

  .usage-provider-icon {
    width: 20px;
    height: 20px;
    object-fit: contain;
  }

  .usage-node-title,
  .usage-center-title {
    margin: 0;
    font-size: 0.84rem;
    font-weight: 700;
    color: var(--color-text-primary);
  }

  .usage-node-subtitle,
  .usage-center-meta,
  .usage-node-meta {
    margin: 0.12rem 0 0;
    font-size: 0.67rem;
    color: var(--color-text-secondary);
  }

  .usage-log-table-head,
  .usage-log-row {
    display: grid;
    grid-template-columns: minmax(0, 1.3fr) minmax(0, 1.05fr) 96px 62px;
    gap: 0.55rem;
    align-items: center;
  }

  .usage-log-table-head {
    padding: 0.58rem 0.8rem;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 85%, transparent);
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.07em;
  }

  .usage-log-list {
    max-height: 340px;
    overflow: auto;
  }

  .usage-log-row {
    padding: 0.62rem 0.8rem;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 75%, transparent);
  }

  .usage-log-model p,
  .usage-log-account,
  .usage-log-when {
    margin: 0;
    font-size: 0.74rem;
    color: var(--color-text-primary);
  }

  .usage-log-model span {
    display: block;
    margin-top: 0.15rem;
    font-size: 0.64rem;
    color: var(--color-text-secondary);
  }

  .usage-log-account {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .usage-log-tokens {
    display: flex;
    gap: 0.35rem;
    font-size: 0.7rem;
    color: color-mix(in srgb, var(--accent-primary) 70%, var(--color-text-primary));
  }

  .usage-log-empty {
    padding: 0.9rem;
    font-size: 0.74rem;
    color: var(--color-text-secondary);
  }

  @keyframes usage-flow-dash {
    from {
      stroke-dashoffset: 0;
    }

    to {
      stroke-dashoffset: -24;
    }
  }

  @media (max-width: 1024px) {
    .usage-flow-map {
      min-height: 300px;
    }

    .usage-provider-node-left {
      left: 4%;
    }

    .usage-provider-node-right {
      right: 4%;
    }
  }

  @media (max-width: 768px) {
    .usage-log-table-head,
    .usage-log-row {
      grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr);
    }

    .usage-log-table-head span:nth-child(3),
    .usage-log-table-head span:nth-child(4),
    .usage-log-row > :nth-child(3),
    .usage-log-row > :nth-child(4) {
      display: none;
    }

    .usage-flow-map {
      min-height: 430px;
    }

    .usage-provider-node-left,
    .usage-provider-node-right,
    .usage-center-node {
      left: 50%;
      right: auto;
      transform: translateX(-50%);
    }

    .usage-provider-node-left {
      top: 16%;
    }

    .usage-center-node {
      top: 50%;
      transform: translate(-50%, -50%);
    }

    .usage-provider-node-right {
      top: 82%;
    }

    .usage-flow-svg {
      display: none;
    }
  }
</style>
