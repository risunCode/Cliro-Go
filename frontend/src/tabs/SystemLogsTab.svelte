<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Copy, RefreshCw, Search, Trash2 } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import type { LogEntry } from '@/services/wails-api'
  import { formatDateTime, formatNumber } from '@/utils/formatters'

  type LevelFilter = 'all' | 'info' | 'warning' | 'error' | 'debug' | 'other'
  type BadgeTone = 'neutral' | 'success' | 'error' | 'info' | 'warning'

  export let logs: LogEntry[] = []
  export let loading = false
  export let onRefreshLogs: () => Promise<void>
  export let onClearLogs: () => Promise<void>

  let query = ''
  let levelFilter: LevelFilter = 'all'
  let scopeFilter = 'all'
  let newestFirst = true
  let copied = false
  let copyTimer: ReturnType<typeof setTimeout> | null = null

  const normalizeLevel = (entry: LogEntry): string => {
    return (entry.level || 'info').trim().toLowerCase()
  }

  const normalizeScope = (entry: LogEntry): string => {
    return (entry.scope || 'system').trim().toLowerCase()
  }

  const levelKey = (entry: LogEntry): Exclude<LevelFilter, 'all'> => {
    const level = normalizeLevel(entry)
    if (level === 'warn' || level === 'warning') {
      return 'warning'
    }
    if (level === 'error') {
      return 'error'
    }
    if (level === 'debug') {
      return 'debug'
    }
    if (level === 'info') {
      return 'info'
    }
    return 'other'
  }

  const levelLabel = (entry: LogEntry): string => {
    const key = levelKey(entry)
    if (key === 'warning') {
      return 'WARN'
    }
    return key.toUpperCase()
  }

  const levelTone = (key: Exclude<LevelFilter, 'all'>): BadgeTone => {
    if (key === 'error') {
      return 'error'
    }
    if (key === 'warning') {
      return 'warning'
    }
    if (key === 'info') {
      return 'info'
    }
    if (key === 'debug') {
      return 'neutral'
    }
    return 'neutral'
  }

  const toTerminalLine = (entry: LogEntry): string => {
    const timestamp = formatDateTime(entry.timestamp)
    return `${timestamp} | ${levelLabel(entry)} | ${normalizeScope(entry)} | ${entry.message || ''}`
  }

  const canUseClipboard = (): boolean => {
    return typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function'
  }

  const handleCopyVisible = async (): Promise<void> => {
    if (!canUseClipboard() || visibleLines.length === 0) {
      return
    }

    await navigator.clipboard.writeText(visibleLines.join('\n'))
    copied = true

    if (copyTimer) {
      clearTimeout(copyTimer)
    }
    copyTimer = setTimeout(() => {
      copied = false
      copyTimer = null
    }, 1400)
  }

  onDestroy(() => {
    if (copyTimer) {
      clearTimeout(copyTimer)
    }
  })

  $: scopes = Array.from(new Set(logs.map((entry) => normalizeScope(entry)).filter((value) => value.length > 0))).sort((a, b) =>
    a.localeCompare(b)
  )

  $: levelCounts = logs.reduce(
    (accumulator, entry) => {
      accumulator[levelKey(entry)] += 1
      return accumulator
    },
    {
      info: 0,
      warning: 0,
      error: 0,
      debug: 0,
      other: 0
    }
  )

  $: normalizedQuery = query.trim().toLowerCase()
  $: filteredLogs = logs.filter((entry) => {
    const currentLevel = levelKey(entry)
    const currentScope = normalizeScope(entry)

    if (levelFilter !== 'all' && currentLevel !== levelFilter) {
      return false
    }

    if (scopeFilter !== 'all' && currentScope !== scopeFilter) {
      return false
    }

    if (normalizedQuery.length === 0) {
      return true
    }

    const text = `${formatDateTime(entry.timestamp)} ${currentLevel} ${currentScope} ${entry.message || ''}`.toLowerCase()
    return text.includes(normalizedQuery)
  })

  $: orderedLogs = newestFirst ? [...filteredLogs].reverse() : filteredLogs
  $: visibleLines = orderedLogs.map((entry) => toTerminalLine(entry))
</script>

<div class="space-y-3">
  <SurfaceCard className="p-4">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div class="space-y-1">
        <p class="text-sm font-semibold text-text-primary">System Logs</p>
        <p class="text-xs text-text-secondary">Structured, filterable stream for backend events and runtime diagnostics.</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" className="gap-1.5" disabled={loading} on:click={() => void onRefreshLogs()}>
          <RefreshCw size={14} class={loading ? 'is-spinning' : ''} />
          Refresh
        </Button>
        <Button
          variant="secondary"
          size="sm"
          className="gap-1.5"
          disabled={!canUseClipboard() || visibleLines.length === 0}
          on:click={() => void handleCopyVisible()}
        >
          <Copy size={14} />
          {copied ? 'Copied' : 'Copy Visible'}
        </Button>
        <Button variant="danger" size="sm" className="gap-1.5" disabled={logs.length === 0} on:click={() => void onClearLogs()}>
          <Trash2 size={14} />
          Clear Logs
        </Button>
      </div>
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-2">
      <StatusBadge tone="neutral">Total {formatNumber(logs.length)}</StatusBadge>
      <StatusBadge tone="error">Errors {formatNumber(levelCounts.error)}</StatusBadge>
      <StatusBadge tone="warning">Warnings {formatNumber(levelCounts.warning)}</StatusBadge>
      <StatusBadge tone="info">Info {formatNumber(levelCounts.info)}</StatusBadge>
      <StatusBadge tone="neutral">Shown {formatNumber(filteredLogs.length)}</StatusBadge>
    </div>
  </SurfaceCard>

  <SurfaceCard className="p-3">
    <div class="logs-filters">
      <div class="logs-filter-search relative">
        <Search size={14} class="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
        <input
          type="text"
          bind:value={query}
          placeholder="Search level, scope, timestamp, or message"
          class="w-full rounded-sm border border-border bg-app py-2 pl-9 pr-3 text-xs text-text-primary outline-none transition-colors focus:border-text-secondary"
        />
      </div>

      <label class="filter-label level-filter">
        <span>Level</span>
        <select bind:value={levelFilter} class="filter-select">
          <option value="all">All</option>
          <option value="error">Error</option>
          <option value="warning">Warning</option>
          <option value="info">Info</option>
          <option value="debug">Debug</option>
          <option value="other">Other</option>
        </select>
      </label>

      <label class="filter-label scope-filter">
        <span>Scope</span>
        <select bind:value={scopeFilter} class="filter-select">
          <option value="all">All scopes</option>
          {#each scopes as scope}
            <option value={scope}>{scope}</option>
          {/each}
        </select>
      </label>

      <Button variant="secondary" size="sm" className="order-toggle whitespace-nowrap" on:click={() => (newestFirst = !newestFirst)}>
        {newestFirst ? 'Newest First' : 'Oldest First'}
      </Button>
    </div>
  </SurfaceCard>

  <SurfaceCard className="overflow-hidden p-0">
    <div class="logs-viewport no-scrollbar max-h-[33rem] overflow-auto bg-app">
      {#if logs.length === 0}
        <div class="px-3 py-5 text-center text-xs text-text-secondary">No log entries yet.</div>
      {:else if orderedLogs.length === 0}
        <div class="px-3 py-5 text-center text-xs text-text-secondary">No entries match the current filters.</div>
      {:else}
        <div class="logs-table">
          <div class="logs-grid sticky top-0 z-10 border-b border-border bg-surface px-3 py-2 font-mono text-[10px] uppercase tracking-[0.08em] text-text-secondary">
            <span>Time</span>
            <span>Level</span>
            <span>Scope</span>
            <span>Message</span>
          </div>

          <div class="divide-y divide-border">
            {#each orderedLogs as entry, index (entry.timestamp + ':' + entry.scope + ':' + index)}
              {@const currentLevel = levelKey(entry)}
              <div class={`log-row log-row-${currentLevel}`}>
                <p class="log-time">{formatDateTime(entry.timestamp)}</p>
                <div class="log-level-cell">
                  <StatusBadge tone={levelTone(currentLevel)} className="log-badge">{levelLabel(entry)}</StatusBadge>
                </div>
                <p class="log-scope">{normalizeScope(entry)}</p>
                <p class="log-message" title={entry.message || '-'}>{entry.message || '-'}</p>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </SurfaceCard>
</div>

<style>
  .logs-filters {
    display: grid;
    gap: 0.5rem;
    grid-template-columns: minmax(0, 1fr);
  }

  .logs-filters > * {
    min-width: 0;
  }

  .filter-label {
    display: grid;
    gap: 0.35rem;
  }

  .filter-label span {
    font-size: 10px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-secondary);
    font-weight: 600;
  }

  .filter-select {
    width: 100%;
    min-height: 2rem;
    border-radius: 6px;
    border: 1px solid var(--color-border);
    background: var(--color-app);
    color: var(--color-text-primary);
    font-size: 12px;
    padding: 0 0.55rem;
    outline: none;
    transition: border-color 0.2s ease;
  }

  .filter-select option {
    color: #111827;
    background: #ffffff;
  }

  .filter-select:focus {
    border-color: color-mix(in srgb, var(--color-text-secondary) 70%, var(--color-border));
  }

  .logs-viewport {
    user-select: text;
    -webkit-user-select: text;
    scrollbar-gutter: stable both-edges;
    overflow: auto;
  }

  .logs-table {
    min-width: 58rem;
  }

  .logs-grid {
    display: grid;
    grid-template-columns: 11rem 6rem 8rem minmax(0, 1fr);
    gap: 0.5rem;
  }

  .log-row {
    display: grid;
    grid-template-columns: 11rem 6rem 8rem minmax(0, 1fr);
    gap: 0.5rem;
    align-items: center;
    padding: 0.48rem 0.75rem;
    font-size: 12px;
    line-height: 1.4;
    transition: background-color 0.16s ease;
    background: color-mix(in srgb, var(--color-app) 92%, transparent);
  }

  .log-row:hover {
    background: color-mix(in srgb, var(--color-surface) 82%, transparent);
  }

  .log-row-error {
    background: color-mix(in srgb, var(--color-error) 6%, var(--color-app));
  }

  .log-row-warning {
    background: color-mix(in srgb, var(--color-warning) 6%, var(--color-app));
  }

  .log-level-cell {
    display: inline-flex;
    align-items: center;
    min-height: 1.5rem;
  }

  .log-time,
  .log-scope,
  .log-message {
    margin: 0;
    font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  }

  .log-time,
  .log-scope {
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .log-scope {
    text-transform: lowercase;
  }

  .log-message {
    color: var(--color-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .log-badge {
    align-self: center;
    justify-self: start;
    font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
    font-size: 10px;
    letter-spacing: 0.03em;
    line-height: 1;
    min-height: 1.3rem;
    padding: 0.2rem 0.45rem;
    min-width: 4.6rem;
    justify-content: center;
  }

  .is-spinning {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }

  @media (min-width: 760px) {
    .logs-filters {
      grid-template-columns: minmax(0, 1fr) minmax(10rem, 12rem);
    }

    .scope-filter,
    .order-toggle {
      grid-column: 1 / -1;
    }

    .order-toggle {
      justify-self: start;
    }
  }

  @media (min-width: 1080px) {
    .logs-filters {
      grid-template-columns: minmax(0, 1fr) 10rem 12rem auto;
      align-items: end;
    }

    .scope-filter,
    .order-toggle {
      grid-column: auto;
    }

    .order-toggle {
      justify-self: stretch;
    }
  }

  @media (max-width: 960px) {
    .logs-table {
      min-width: 48rem;
    }

    .logs-grid,
    .log-row {
      grid-template-columns: 9rem 5.5rem 7rem minmax(0, 1fr);
    }
  }
</style>
