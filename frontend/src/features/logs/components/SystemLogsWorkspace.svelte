<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Copy, Download, RefreshCw, Search, Trash2 } from 'lucide-svelte'
  import type { LogEntry } from '@/app/types'
  import Button from '@/components/common/Button.svelte'
  import SurfaceCard from '@/components/common/SurfaceCard.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import {
    LOG_TABLE_MAX_ENTRIES,
    buildExportPayload,
    buildVisibleLines,
    createLogRows,
    filterLogRows,
    formatLogTime,
    getLevelCounts,
    getNextSortState,
    getScopeQuickFilters,
    getScopes,
    getSortIndicator,
    getVisibleRows,
    isMessageExpanded,
    isScopeQuickFilterVisible,
    levelKey,
    levelLabel,
    levelTone,
    pruneExpandedMessageIDs,
    quickScopeFilterLabel,
    sortLogRows,
    toggleMessageExpansion,
    type LevelFilter,
    type SortDirection,
    type SortField
  } from '@/features/logs/lib/logs-view'
  import { formatDateTime, formatNumber } from '@/shared/lib/formatters'
  import { copyTextToClipboard, downloadJSONFile, hasClipboardWrite } from '@/shared/lib/browser'

  export let logs: LogEntry[] = []
  export let loading = false
  export let clearing = false
  export let onRefreshLogs: () => Promise<void>
  export let onClearLogs: () => Promise<void>

  let query = ''
  let levelFilter: LevelFilter = 'all'
  let scopeFilter = 'all'
  let sortField: SortField = 'timestamp'
  let sortDirection: SortDirection = 'desc'
  let copied = false
  let copiedMessageID = ''
  let copyTimer: ReturnType<typeof setTimeout> | null = null
  let messageCopyTimer: ReturnType<typeof setTimeout> | null = null
  let expandedMessageIDs: string[] = []

  const handleSortFieldChange = (event: Event): void => {
    const target = event.currentTarget as HTMLSelectElement
    const nextSort = getNextSortState(sortField, sortDirection, target.value as SortField)
    sortField = nextSort.sortField
    sortDirection = nextSort.sortDirection
  }

  const handleSortColumn = (field: SortField): void => {
    const nextSort = getNextSortState(sortField, sortDirection, field)
    sortField = nextSort.sortField
    sortDirection = nextSort.sortDirection
  }

  const toggleSortDirection = (): void => {
    sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
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
    downloadJSONFile(buildExportPayload(orderedRows), `cliro-logs-${Date.now()}.json`)
  }

  const handleToggleMessageExpansion = (rowID: string): void => {
    expandedMessageIDs = toggleMessageExpansion(expandedMessageIDs, rowID)
  }

  onDestroy(() => {
    if (copyTimer) {
      clearTimeout(copyTimer)
    }
    if (messageCopyTimer) {
      clearTimeout(messageCopyTimer)
    }
  })

  $: filters = {
    query,
    level: levelFilter,
    scope: scopeFilter,
    sortField,
    sortDirection
  }
  $: scopes = getScopes(logs)
  $: scopeQuickFilters = getScopeQuickFilters(scopes)
  $: levelCounts = getLevelCounts(logs)
  $: allLogRows = createLogRows(logs)
  $: filteredRows = filterLogRows(allLogRows, filters)
  $: orderedRows = getVisibleRows(sortLogRows(filteredRows, sortField, sortDirection))
  $: visibleLines = buildVisibleLines(orderedRows)
  $: {
    const nextExpandedIDs = pruneExpandedMessageIDs(expandedMessageIDs, orderedRows)
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
      <StatusBadge tone="neutral">Filtered {formatNumber(filteredRows.length)}</StatusBadge>
      <StatusBadge tone="neutral">Visible {formatNumber(orderedRows.length)} / {formatNumber(LOG_TABLE_MAX_ENTRIES)}</StatusBadge>
      {#if filteredRows.length > LOG_TABLE_MAX_ENTRIES}
        <StatusBadge tone="warning">Capped to {formatNumber(LOG_TABLE_MAX_ENTRIES)} entries</StatusBadge>
      {/if}
    </div>
  </SurfaceCard>

  <SurfaceCard className="p-3">
    <div class="logs-filters">
      <div class="logs-filter-search relative">
        <Search size={14} class="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
        <input
          type="text"
          bind:value={query}
          placeholder="Search source, account, detail, request ID, or field"
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
        <span>Source</span>
        <select bind:value={scopeFilter} class="filter-select ui-control-input ui-control-select-sm">
          <option value="all">All scopes</option>
          {#each scopes as scope}
            <option value={scope}>{scope}</option>
          {/each}
        </select>
      </label>

      <label class="filter-label sort-field-filter">
        <span>Sort By</span>
        <select value={sortField} class="filter-select ui-control-input ui-control-select-sm" on:change={handleSortFieldChange}>
          <option value="timestamp">Time</option>
          <option value="level">Level</option>
          <option value="scope">Source</option>
          <option value="account">Account</option>
          <option value="detail">Detail</option>
        </select>
      </label>

      <Button variant="secondary" size="sm" className="sort-direction-toggle whitespace-nowrap" on:click={toggleSortDirection}>
        {sortDirection === 'asc' ? 'Ascending' : 'Descending'}
      </Button>
    </div>

    {#if scopeQuickFilters.length > 0}
      <div class="scope-quick-filters mt-2">
        <button
          type="button"
          class={`scope-chip ${scopeFilter === 'all' ? 'is-active' : ''}`}
          on:click={() => {
            scopeFilter = 'all'
          }}
        >
          All
        </button>

        {#each scopeQuickFilters as scope}
          <button
            type="button"
            class={`scope-chip ${scopeFilter === scope ? 'is-active' : ''} ${isScopeQuickFilterVisible(scopeFilter, scope) ? '' : 'is-muted'}`}
            on:click={() => {
              scopeFilter = scope
            }}
          >
            {quickScopeFilterLabel(scope)}
          </button>
        {/each}
      </div>
    {/if}
  </SurfaceCard>

  <SurfaceCard className="system-logs-table-card overflow-hidden p-0">
    <div class="logs-viewport bg-app">
      {#if logs.length === 0}
        <div class="px-3 py-5 text-center text-xs text-text-secondary">No log entries yet.</div>
      {:else if orderedRows.length === 0}
        <div class="px-3 py-5 text-center text-xs text-text-secondary">No entries match the current filters.</div>
      {:else}
        <div class="logs-table">
          <div class="logs-grid sticky top-0 z-10 border-b border-border bg-surface px-3 py-2 font-mono text-[10px] uppercase tracking-[0.08em] text-text-secondary">
            <button type="button" class="logs-head-btn" on:click={() => handleSortColumn('level')}>
              Level
              <span class="logs-head-sort">{getSortIndicator(sortField, sortDirection, 'level')}</span>
            </button>
            <button type="button" class="logs-head-btn" on:click={() => handleSortColumn('scope')}>
              Source
              <span class="logs-head-sort">{getSortIndicator(sortField, sortDirection, 'scope')}</span>
            </button>
            <button type="button" class="logs-head-btn" on:click={() => handleSortColumn('account')}>
              Account
              <span class="logs-head-sort">{getSortIndicator(sortField, sortDirection, 'account')}</span>
            </button>
            <button type="button" class="logs-head-btn" on:click={() => handleSortColumn('detail')}>
              Detail
              <span class="logs-head-sort">{getSortIndicator(sortField, sortDirection, 'detail')}</span>
            </button>
            <button type="button" class="logs-head-btn logs-head-btn-time" on:click={() => handleSortColumn('timestamp')}>
              Time
              <span class="logs-head-sort">{getSortIndicator(sortField, sortDirection, 'timestamp')}</span>
            </button>
          </div>

          <div class="divide-y divide-border">
            {#each orderedRows as row (row.rowID)}
              {@const entry = row.entry}
              {@const currentLevel = levelKey(entry)}
              {@const rowID = row.rowID}
              {@const copyText = row.copyText}
              {@const expanded = isMessageExpanded(expandedMessageIDs, rowID)}
              <div class={`log-row log-row-${currentLevel}`}>
                <div class="log-level-cell">
                  <StatusBadge tone={levelTone(currentLevel)} className="log-badge">{levelLabel(entry)}</StatusBadge>
                </div>
                <p class="log-source" title={row.normalizedScope}>{row.normalizedScope}</p>
                <p class={`log-account ${row.accountLabel === '-' ? 'is-empty' : ''}`} title={row.accountLabel}>{row.accountLabel}</p>
                <div class="log-detail-cell">
                  <p class={`log-detail ${expanded ? 'is-expanded' : ''}`} title={expanded ? undefined : row.detailText}>{row.detailText}</p>
                  <div class="log-detail-actions">
                    {#if row.expandable}
                      <button type="button" class="log-message-link" on:click={() => handleToggleMessageExpansion(rowID)}>
                        {expanded ? 'Show less' : 'Show more'}
                      </button>
                    {/if}
                    <button
                      type="button"
                      class="log-message-link"
                      disabled={!hasClipboardWrite()}
                      on:click={() => void handleCopyMessage(copyText, rowID)}
                    >
                      {copiedMessageID === rowID ? 'Copied' : 'Copy'}
                    </button>
                  </div>
                </div>
                <p class="log-time" title={formatDateTime(entry.timestamp)}>{formatLogTime(entry.timestamp)}</p>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </SurfaceCard>
</div>
