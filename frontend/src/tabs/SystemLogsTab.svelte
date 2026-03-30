<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Copy, Download, RefreshCw, Search, Trash2 } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import type { LogEntry } from '@/services/wails-api'
  import { formatDateTime, formatNumber } from '@/utils/formatters'
  import { copyTextToClipboard, downloadJSONFile, hasClipboardWrite } from '@/utils/browser'

  type LevelFilter = 'all' | 'info' | 'warning' | 'error' | 'debug' | 'other'
  type BadgeTone = 'neutral' | 'success' | 'error' | 'info' | 'warning'

  interface LogRow {
    rowID: string
    entry: LogEntry
  }

  export let logs: LogEntry[] = []
  export let loading = false
  export let clearing = false
  export let onRefreshLogs: () => Promise<void>
  export let onClearLogs: () => Promise<void>

  let query = ''
  let levelFilter: LevelFilter = 'all'
  let scopeFilter = 'all'
  let newestFirst = true
  let copied = false
  let copiedMessageID = ''
  let copyTimer: ReturnType<typeof setTimeout> | null = null
  let messageCopyTimer: ReturnType<typeof setTimeout> | null = null
  let expandedMessageIDs: string[] = []

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

  const buildLogRowID = (entry: LogEntry, occurrence: number): string => {
    return `${entry.timestamp}:${normalizeScope(entry)}:${normalizeLevel(entry)}:${entry.message || ''}:${occurrence}`
  }

  const isExpandableMessage = (message: string): boolean => {
    return message.trim().length > 96
  }

  const isMessageExpanded = (rowID: string): boolean => {
    return expandedMessageIDs.includes(rowID)
  }

  const toggleMessageExpansion = (rowID: string): void => {
    if (isMessageExpanded(rowID)) {
      expandedMessageIDs = expandedMessageIDs.filter((id) => id !== rowID)
      return
    }

    expandedMessageIDs = [...expandedMessageIDs, rowID]
  }

  const handleCopyMessage = async (message: string, rowID: string): Promise<void> => {
    if (!hasClipboardWrite() || message.trim().length === 0) {
      return
    }

    const copiedToClipboard = await copyTextToClipboard(message)
    if (!copiedToClipboard) {
      return
    }

    copiedMessageID = rowID
    if (messageCopyTimer) {
      clearTimeout(messageCopyTimer)
    }
    messageCopyTimer = setTimeout(() => {
      copiedMessageID = ''
      messageCopyTimer = null
    }, 1400)
  }

  const handleCopyVisible = async (): Promise<void> => {
    if (!hasClipboardWrite() || visibleLines.length === 0) {
      return
    }

    const copiedToClipboard = await copyTextToClipboard(visibleLines.join('\n'))
    if (!copiedToClipboard) {
      return
    }

    copied = true

    if (copyTimer) {
      clearTimeout(copyTimer)
    }
    copyTimer = setTimeout(() => {
      copied = false
      copyTimer = null
    }, 1400)
  }

  const handleExportVisible = (): void => {
    const payload = {
      exportedAt: new Date().toISOString(),
      count: orderedRows.length,
      entries: orderedRows.map(({ entry }) => entry)
    }
    downloadJSONFile(payload, `cliro-logs-${Date.now()}.json`)
  }

  onDestroy(() => {
    if (copyTimer) {
      clearTimeout(copyTimer)
    }
    if (messageCopyTimer) {
      clearTimeout(messageCopyTimer)
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
  $: allLogRows = (() => {
    const occurrences = new Map<string, number>()
    return logs.map((entry) => {
      const fingerprint = `${entry.timestamp}:${normalizeScope(entry)}:${normalizeLevel(entry)}:${entry.message || ''}`
      const nextOccurrence = (occurrences.get(fingerprint) || 0) + 1
      occurrences.set(fingerprint, nextOccurrence)

      return {
        rowID: buildLogRowID(entry, nextOccurrence),
        entry
      } as LogRow
    })
  })()

  $: filteredRows = allLogRows.filter(({ entry }) => {
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

  $: orderedRows = newestFirst ? [...filteredRows].reverse() : filteredRows
  $: visibleLines = orderedRows.map(({ entry }) => toTerminalLine(entry))
  $: {
    const visibleIDSet = new Set(orderedRows.map((row) => row.rowID))
    const nextExpandedIDs = expandedMessageIDs.filter((id) => visibleIDSet.has(id))
    if (nextExpandedIDs.length !== expandedMessageIDs.length) {
      expandedMessageIDs = nextExpandedIDs
    }
  }
</script>

<div class="system-logs-tab space-y-3">
  <SurfaceCard className="p-4">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div class="space-y-1">
        <p class="text-sm font-semibold text-text-primary">System Logs</p>
        <p class="text-xs text-text-secondary">Structured, filterable stream for backend events and runtime diagnostics.</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" className="gap-1.5" disabled={loading || clearing} on:click={() => void onRefreshLogs()}>
          <RefreshCw size={14} class={loading ? 'system-logs-spinning' : ''} />
          Refresh
        </Button>
        <Button
          variant="secondary"
          size="sm"
          className="gap-1.5"
          disabled={!hasClipboardWrite() || visibleLines.length === 0}
          on:click={() => void handleCopyVisible()}
        >
          <Copy size={14} />
          {copied ? 'Copied' : 'Copy Visible'}
        </Button>
        <Button variant="secondary" size="sm" className="gap-1.5" disabled={orderedRows.length === 0} on:click={handleExportVisible}>
          <Download size={14} />
          Export
        </Button>
        <Button variant="danger" size="sm" className="gap-1.5" disabled={clearing || logs.length === 0} on:click={() => void onClearLogs()}>
          <Trash2 size={14} />
          {clearing ? 'Clearing...' : 'Clear Logs'}
        </Button>
      </div>
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-2">
      <StatusBadge tone="neutral">Total {formatNumber(logs.length)}</StatusBadge>
      <StatusBadge tone="error">Errors {formatNumber(levelCounts.error)}</StatusBadge>
      <StatusBadge tone="warning">Warnings {formatNumber(levelCounts.warning)}</StatusBadge>
      <StatusBadge tone="info">Info {formatNumber(levelCounts.info)}</StatusBadge>
      <StatusBadge tone="neutral">Shown {formatNumber(filteredRows.length)}</StatusBadge>
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
          class="ui-control-input ui-control-select-sm bg-app py-2 pl-9 pr-3 text-xs"
        />
      </div>

      <label class="filter-label level-filter">
        <span>Level</span>
        <select bind:value={levelFilter} class="filter-select ui-control-input ui-control-select-sm">
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
        <select bind:value={scopeFilter} class="filter-select ui-control-input ui-control-select-sm">
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
      {:else if orderedRows.length === 0}
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
            {#each orderedRows as row (row.rowID)}
              {@const entry = row.entry}
              {@const currentLevel = levelKey(entry)}
              {@const rowID = row.rowID}
              {@const message = entry.message || '-'}
              {@const expandable = isExpandableMessage(message)}
              {@const expanded = isMessageExpanded(rowID)}
              <div class={`log-row log-row-${currentLevel}`}>
                <p class="log-time">{formatDateTime(entry.timestamp)}</p>
                <div class="log-level-cell">
                  <StatusBadge tone={levelTone(currentLevel)} className="log-badge">{levelLabel(entry)}</StatusBadge>
                </div>
                <p class="log-scope">{normalizeScope(entry)}</p>
                <div class="log-message-cell">
                  <p class={`log-message ${expanded ? 'is-expanded' : ''}`} title={expanded ? undefined : message}>{message}</p>
                  {#if expandable}
                    <div class="log-message-actions">
                      <button type="button" class="log-message-link" on:click={() => toggleMessageExpansion(rowID)}>
                        {expanded ? 'Show less' : 'Show more'}
                      </button>
                      <button
                        type="button"
                        class="log-message-link"
                        disabled={!hasClipboardWrite()}
                        on:click={() => void handleCopyMessage(message, rowID)}
                      >
                        {copiedMessageID === rowID ? 'Copied' : 'Copy'}
                      </button>
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </SurfaceCard>
</div>
