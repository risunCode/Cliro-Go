<script lang="ts">
  import { onMount } from 'svelte'
  import { Activity, Gauge, Server, Users } from 'lucide-svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import { appService, type Account, type AppState, type ProxyStatus } from '@/services/wails-api'
  import { formatNumber, formatUnixSeconds } from '@/utils/formatters'

  export let state: AppState | null = null
  export let accounts: Account[] = []
  export let proxyStatus: ProxyStatus | null = null
  export let loading = false

  let greeting = 'Hello'
  let hostName = 'This PC'

  const countEnabledAccounts = (items: Account[]): number => items.filter((account) => account.enabled).length

  const countAccountsWithCooldown = (items: Account[]): number => {
    const now = Math.floor(Date.now() / 1000)
    return items.filter((account) => (account.cooldownUntil ?? 0) > now).length
  }

  const getGreeting = (hour: number): string => {
    if (hour < 5) {
      return 'Good night'
    }
    if (hour < 12) {
      return 'Good morning'
    }
    if (hour < 17) {
      return 'Good afternoon'
    }
    if (hour < 21) {
      return 'Good evening'
    }
    return 'Good night'
  }

  const loadHostName = async (): Promise<void> => {
    try {
      const value = (await appService.getHostName()).trim()
      if (value.length > 0) {
        hostName = value
      }
    } catch {
      hostName = 'This PC'
    }
  }

  onMount(() => {
    greeting = getGreeting(new Date().getHours())
    void loadHostName()
  })

  $: enabledCount = countEnabledAccounts(accounts)
  $: disabledCount = Math.max(accounts.length - enabledCount, 0)
  $: cooldownCount = countAccountsWithCooldown(accounts)
  $: stats = state?.stats
  $: totalRequests = stats?.totalRequests ?? 0
  $: successRequests = stats?.successRequests ?? 0
  $: failedRequests = stats?.failedRequests ?? 0
  $: promptTokens = stats?.promptTokens ?? 0
  $: completionTokens = stats?.completionTokens ?? 0
  $: totalTokens = stats?.totalTokens ?? 0
  $: failureRate = totalRequests > 0 ? (failedRequests / totalRequests) * 100 : 0
  $: proxyRunning = proxyStatus?.running ?? state?.proxyRunning ?? false
  $: proxyPort = proxyStatus?.port ?? state?.proxyPort ?? 0
  $: proxyURL = proxyStatus?.url || state?.proxyUrl || '-'
</script>

<div class="space-y-3">
  <SurfaceCard className="p-4">
    <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div class="space-y-1">
        <p class="text-base font-semibold text-text-primary">{greeting}, {hostName}.</p>
        <p class="text-xs text-text-secondary">
          Welcome back. This dashboard gives you a compact overview of account availability, proxy status, and traffic health.
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <StatusBadge tone={proxyRunning ? 'success' : 'error'}>{proxyRunning ? 'Proxy Running' : 'Proxy Stopped'}</StatusBadge>
        <StatusBadge tone="info">Port {proxyPort}</StatusBadge>
        <StatusBadge tone="neutral">{formatNumber(enabledCount)} / {formatNumber(accounts.length)} enabled</StatusBadge>
      </div>
    </div>
  </SurfaceCard>

  <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
    <SurfaceCard className="p-3">
      <div class="mb-2 flex items-center justify-between">
        <p class="text-xs uppercase tracking-[0.08em] text-text-secondary">Accounts</p>
        <Users size={16} class="text-text-secondary" />
      </div>
      <p class="text-2xl font-semibold text-text-primary">{formatNumber(accounts.length)}</p>
      <p class="mt-0.5 text-xs text-text-secondary">{formatNumber(enabledCount)} enabled</p>
    </SurfaceCard>

    <SurfaceCard className="p-3">
      <div class="mb-2 flex items-center justify-between">
        <p class="text-xs uppercase tracking-[0.08em] text-text-secondary">Enabled</p>
        <Activity size={16} class="text-text-secondary" />
      </div>
      <p class="text-2xl font-semibold text-text-primary">{formatNumber(enabledCount)}</p>
      <p class="mt-0.5 text-xs text-text-secondary">{formatNumber(disabledCount)} disabled</p>
    </SurfaceCard>

    <SurfaceCard className="p-3">
      <div class="mb-2 flex items-center justify-between">
        <p class="text-xs uppercase tracking-[0.08em] text-text-secondary">Available Pool</p>
        <Gauge size={16} class="text-text-secondary" />
      </div>
      <p class="text-2xl font-semibold text-text-primary">{formatNumber(state?.availableCount)}</p>
      <p class="mt-0.5 text-xs text-text-secondary">{formatNumber(cooldownCount)} in cooldown</p>
    </SurfaceCard>

    <SurfaceCard className="p-3">
      <div class="mb-2 flex items-center justify-between">
        <p class="text-xs uppercase tracking-[0.08em] text-text-secondary">Proxy</p>
        <Server size={16} class="text-text-secondary" />
      </div>
      <div class="flex items-center gap-2">
        <p class="text-2xl font-semibold text-text-primary">{proxyPort}</p>
        <StatusBadge tone={proxyRunning ? 'success' : 'error'}>
          {proxyRunning ? 'Running' : 'Stopped'}
        </StatusBadge>
      </div>
      <p class="mt-0.5 truncate text-xs text-text-secondary">{proxyURL}</p>
    </SurfaceCard>
  </div>

  <div class="grid gap-3 lg:grid-cols-2">
    <SurfaceCard className="p-3">
      <div class="mb-2 flex items-center justify-between">
        <p class="text-sm font-semibold text-text-primary">Traffic</p>
        <StatusBadge tone="info">Requests</StatusBadge>
      </div>

      <div class="grid gap-2 sm:grid-cols-2">
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Total Requests</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{formatNumber(totalRequests)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Success Requests</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{formatNumber(successRequests)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Failed Requests</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{formatNumber(failedRequests)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Last Request</p>
          <p class="mt-0.5 truncate text-sm font-semibold text-text-primary">{formatUnixSeconds(stats?.lastRequestAt)}</p>
        </div>
      </div>
    </SurfaceCard>

    <SurfaceCard className="p-3">
      <div class="mb-2 flex items-center justify-between">
        <p class="text-sm font-semibold text-text-primary">Tokens</p>
        <StatusBadge tone="neutral">Usage</StatusBadge>
      </div>

      <div class="grid gap-2 sm:grid-cols-2">
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Prompt Tokens</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{formatNumber(promptTokens)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Completion Tokens</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{formatNumber(completionTokens)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Total Tokens</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{formatNumber(totalTokens)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2.5">
          <p class="text-xs text-text-secondary">Failure Rate</p>
          <p class="mt-0.5 text-lg font-semibold text-text-primary">{failureRate.toFixed(1)}%</p>
        </div>
      </div>
    </SurfaceCard>
  </div>

  {#if loading}
    <p class="text-xs text-text-secondary">Refreshing dashboard data...</p>
  {/if}
</div>
