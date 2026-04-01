<script lang="ts">
  import { onMount } from 'svelte'
  import { Info, RefreshCw } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import CollapsibleSurfaceSection from '@/components/common/CollapsibleSurfaceSection.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import CliSyncInfoModal from '@/features/router/components/cli-sync/CliSyncInfoModal.svelte'
  import { routerApi } from '@/features/router/api/router-api'
  import type { CliSyncAppID, CliSyncResult, CliSyncStatus, LocalModelCatalogItem } from '@/features/router/types'
  import { CLI_SYNC_CARDS, groupCliModels } from '@/features/router/lib/cli-sync'

  export let busy = false
  export let proxyBaseURL = ''
  export let proxyAPIKey = ''
  export let onGetCLISyncStatuses: () => Promise<CliSyncStatus[]>
  export let onGetCLISyncFileContent: (appId: CliSyncAppID, path: string) => Promise<string>
  export let onSaveCLISyncFileContent: (appId: CliSyncAppID, path: string, content: string) => Promise<void>
  export let onSyncCLIConfig: (appId: CliSyncAppID, model: string) => Promise<CliSyncResult>

  let expanded = false
  let wasExpanded = false
  let busyTargetID: CliSyncAppID | '' = ''
  let dataLoading = false
  let detectionError = ''
  let results: Partial<Record<CliSyncAppID, CliSyncResult>> = {}
  let errors: Partial<Record<CliSyncAppID, string>> = {}
  let infoTargetID: CliSyncAppID | '' = ''
  let statuses: CliSyncStatus[] = []
  let models: LocalModelCatalogItem[] = []
  let refreshRequestID = 0
  let selectedModels: Record<CliSyncAppID, string> = {
    'claude-code': '',
    'opencode-cli': '',
    'kilo-cli': '',
    'codex-ai': ''
  }

  const emptyErrors = (): Partial<Record<CliSyncAppID, string>> => ({
    'claude-code': '',
    'opencode-cli': '',
    'kilo-cli': '',
    'codex-ai': ''
  })

  $: if (models.length > 0) {
    for (const card of CLI_SYNC_CARDS) {
      const current = selectedModels[card.id]
      const stillExists = models.some((item) => item.id === current)
      if (current && !stillExists) {
        selectedModels = {
          ...selectedModels,
          [card.id]: ''
        }
      }
    }
  }

  $: if (expanded && !wasExpanded && !dataLoading) {
    void refreshData().catch(() => {})
  }

  $: wasExpanded = expanded

  onMount(() => {
    void refreshData().catch(() => {})
  })

  const refreshData = async (): Promise<void> => {
    const requestID = ++refreshRequestID
    dataLoading = true
    detectionError = ''

    try {
      const [statusesResult, modelsResult] = await Promise.allSettled([
        onGetCLISyncStatuses(),
        routerApi.getEffectiveModelCatalog(proxyBaseURL, proxyAPIKey)
      ])

      if (requestID !== refreshRequestID) {
        return
      }

      if (statusesResult.status === 'fulfilled') {
        statuses = statusesResult.value
      } else {
        statuses = []
        detectionError = statusesResult.reason instanceof Error ? statusesResult.reason.message : 'Failed to detect local CLI configuration status.'
      }

      if (modelsResult.status === 'fulfilled') {
        models = modelsResult.value
      } else {
        models = []
        if (!detectionError) {
          detectionError = modelsResult.reason instanceof Error ? modelsResult.reason.message : 'Failed to load proxy model catalog.'
        }
      }

      errors = detectionError
        ? Object.fromEntries(Object.keys(emptyErrors()).map((key) => [key, detectionError])) as Partial<Record<CliSyncAppID, string>>
        : { ...errors, ...emptyErrors() }
    } finally {
      if (requestID === refreshRequestID) {
        dataLoading = false
      }
    }
  }

  const statusFor = (targetID: CliSyncAppID): CliSyncStatus | null => {
    return statuses.find((item) => item.id === targetID) || null
  }

  const installPathFor = (targetID: CliSyncAppID): string => {
    return statusFor(targetID)?.installPath || ''
  }

  const updateModel = (targetID: CliSyncAppID, event: Event): void => {
    selectedModels = {
      ...selectedModels,
      [targetID]: (event.currentTarget as HTMLSelectElement).value
    }
  }

  const syncTarget = async (targetID: CliSyncAppID): Promise<void> => {
    const model = selectedModels[targetID]
    if (!model) {
      errors = { ...errors, [targetID]: 'Select a model before syncing this CLI config.' }
      return
    }

    busyTargetID = targetID
    errors = { ...errors, [targetID]: '' }

    try {
      const result = await onSyncCLIConfig(targetID, model)
      results = { ...results, [targetID]: result }
      await refreshData()
    } catch (error) {
      errors = { ...errors, [targetID]: error instanceof Error ? error.message : 'Sync failed.' }
    } finally {
      busyTargetID = ''
    }
  }

  const openInfo = (targetID: CliSyncAppID): void => {
    infoTargetID = targetID
  }

  const closeInfo = (): void => {
    infoTargetID = ''
  }

  const isDetectingTarget = (): boolean => {
    return dataLoading && expanded && busyTargetID === ''
  }
</script>

<CollapsibleSurfaceSection
  bind:open={expanded}
  icon={RefreshCw}
  title="One-click CLI Sync"
  subtitle="Instantly sync proxy endpoint, API key, and selected model into local AI CLI configs."
  pill="Config Sync"
  ariaLabel="Toggle one-click CLI sync"
  bodyClassName="api-cli-sync-body space-y-2"
>
  <svelte:fragment slot="headerRight">
    {#if dataLoading}
      <RefreshCw size={14} class="animate-spin text-text-secondary" />
    {/if}
  </svelte:fragment>

  <div class="mt-1 grid gap-2 lg:grid-cols-4">
    {#each CLI_SYNC_CARDS as tool (tool.id)}
      <div class="flex h-full min-w-0 flex-col overflow-hidden rounded-sm border border-border bg-app/90 shadow-soft transition hover:border-border/80 hover:bg-surface/70">
        <div class="flex flex-1 flex-col p-2">
          <div class="mb-2 flex items-start justify-between gap-2">
            <div class="flex min-w-0 items-start gap-2">
              <span class={`inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-sm border border-border bg-surface ${tool.toneClass}`}>
                <svelte:component this={tool.icon} size={14} />
              </span>
              <div class="min-w-0">
                <p class="text-sm font-semibold leading-5 text-text-primary">{tool.label}</p>
                <p class="mt-0.5 text-[10px] text-text-secondary">
                  {#if isDetectingTarget()}
                    Detecting...
                  {:else if statusFor(tool.id)?.installed}
                    v{statusFor(tool.id)?.version || 'installed'}
                  {:else}
                    Not detected
                  {/if}
                </p>
              </div>
            </div>
            <StatusBadge tone={statusFor(tool.id)?.synced ? 'success' : errors[tool.id] ? 'warning' : isDetectingTarget() ? 'info' : 'neutral'}>
              {#if isDetectingTarget()}
                Checking
              {:else if statusFor(tool.id)?.synced}
                Synced
              {:else if errors[tool.id]}
                Error
              {:else}
                Not Synced
              {/if}
            </StatusBadge>
          </div>

          <div class="flex flex-1 flex-col space-y-1.5">
            <div class="rounded-sm border border-dashed border-border bg-surface/60 px-1.5 py-1.5">
              <p class="text-[9px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Current Base URL</p>
              <code class="mt-1 block break-all text-[10px] leading-4 text-text-primary">{statusFor(tool.id)?.currentBaseUrl || '---'}</code>
            </div>

            <div class="rounded-sm border border-dashed border-border bg-surface/60 px-1.5 py-1.5">
              <p class="text-[9px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Install Path</p>
              <code class="mt-1 block break-all text-[10px] leading-4 text-text-primary">{installPathFor(tool.id) || '---'}</code>
            </div>

            <div class="space-y-1">
              <p class="text-[9px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Select Model</p>
              <select class="ui-control-input ui-control-select text-[12px]" value={selectedModels[tool.id]} on:change={(event) => updateModel(tool.id, event)} disabled={models.length === 0}>
                {#if models.length === 0}
                  <option value="">No local models available</option>
                {:else}
                  <option value="">Choose a model</option>
                  {#each groupCliModels(models) as group (group.label)}
                    <optgroup label={group.label}>
                      {#each group.models as model (model.id)}
                        <option value={model.id}>{model.id}</option>
                      {/each}
                    </optgroup>
                  {/each}
                {/if}
              </select>
            </div>

            <div class="rounded-sm border border-border bg-surface/40 px-1.5 py-1.5 text-[9.5px] leading-4 text-text-secondary">
              {#if isDetectingTarget()}
                Checking local CLI installation and config files.
              {:else if detectionError}
                {detectionError}
              {:else if errors[tool.id]}
                {errors[tool.id]}
              {:else if statusFor(tool.id)?.currentModel}
                Current model: {statusFor(tool.id)?.currentModel}
              {:else}
                Sync local proxy endpoint, API key, and selected model into this CLI config.
              {/if}
            </div>
          </div>
        </div>

        <div class="mt-auto flex items-center gap-1.5 border-t border-border bg-surface/50 px-2 py-1.5">
          <Button variant="secondary" size="sm" className="px-2.5" on:click={() => openInfo(tool.id)}>
            <Info size={13} class="mr-1" />
            Info
          </Button>
          {#if isDetectingTarget()}
            <Button variant="secondary" size="sm" className="flex-1" disabled={true}>
              <RefreshCw size={13} class="mr-1 animate-spin" />
              Loading...
            </Button>
          {:else if statusFor(tool.id)?.installed}
            <Button
              variant="primary"
              size="sm"
              className="flex-1"
              on:click={() => void syncTarget(tool.id)}
              disabled={busy || busyTargetID !== '' || !selectedModels[tool.id]}
            >
              <RefreshCw size={13} class={`mr-1 ${busyTargetID === tool.id ? 'animate-spin' : ''}`} />
              {busyTargetID === tool.id ? 'Syncing...' : 'Sync Now'}
            </Button>
          {:else}
            <Button variant="secondary" size="sm" className="flex-1" disabled={true}>Not Installed</Button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</CollapsibleSurfaceSection>

<CliSyncInfoModal
  open={infoTargetID !== ''}
  appID={infoTargetID}
  label={CLI_SYNC_CARDS.find((card) => card.id === infoTargetID)?.label || 'CLI Sync Info'}
  selectedModel={infoTargetID ? selectedModels[infoTargetID] || '' : ''}
  proxyBaseURL={proxyBaseURL}
  proxyAPIKey={proxyAPIKey}
  status={infoTargetID ? statusFor(infoTargetID) : null}
  result={infoTargetID ? results[infoTargetID] || null : null}
  availableModels={models}
  onLoadFileContent={onGetCLISyncFileContent}
  onSaveFileContent={onSaveCLISyncFileContent}
  on:close={closeInfo}
  on:saved={() => void refreshData().catch(() => {})}
/>
