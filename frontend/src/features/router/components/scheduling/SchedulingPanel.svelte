<script lang="ts">
  import { RefreshCw } from 'lucide-svelte'
  import CollapsibleSurfaceSection from '@/components/common/CollapsibleSurfaceSection.svelte'
  import type { ProxyStatus } from '@/features/router/types'
  import { SCHEDULING_MODE_CARDS, toSchedulingMode, type SchedulingMode } from '@/features/router/lib/scheduling'

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let onSetSchedulingMode: (mode: string) => Promise<void>

  let expanded = false
  let schedulingModeInput: SchedulingMode = 'balance'

  $: if (proxyStatus && !busy) {
    schedulingModeInput = toSchedulingMode(proxyStatus.schedulingMode)
  }

  const applySchedulingMode = async (mode: SchedulingMode): Promise<void> => {
    if (mode === schedulingModeInput) {
      return
    }
    schedulingModeInput = mode
    await onSetSchedulingMode(mode)
  }

  $: selectedModeSummary = schedulingModeInput === 'cache_first'
    ? 'Cache-first keeps bound sessions on the same account for stronger locality.'
    : schedulingModeInput === 'balance'
      ? 'Balance favors lower-usage accounts for steadier utilization.'
      : 'Performance rotates accounts round-robin for higher concurrency throughput.'

  $: selectedModeTone = schedulingModeInput === 'cache_first'
    ? 'tone-cache'
    : schedulingModeInput === 'balance'
      ? 'tone-balance'
      : 'tone-performance'
</script>

<CollapsibleSurfaceSection
  bind:open={expanded}
  icon={RefreshCw}
  title="Account Scheduling & Rotation"
  subtitle="Routing mode and temporary failure backoff."
  pill="Routing Policy"
  ariaLabel="Toggle account scheduling and rotation"
>
  <div class="api-rotation-grid">
    <div class="api-rotation-main">
      <div class="api-rotation-modes">
        <p class="api-endpoint-label">Scheduling Mode</p>
        <div class="api-rotation-mode-list">
          {#each SCHEDULING_MODE_CARDS as card}
            <button
              type="button"
              class={`api-rotation-mode-card ${schedulingModeInput === card.id ? 'is-active' : ''}`}
              on:click={() => void applySchedulingMode(card.id)}
              disabled={busy}
            >
              <span class="api-rotation-mode-title">{card.label}</span>
              <span class="api-rotation-mode-desc">{card.description}</span>
            </button>
          {/each}
        </div>
      </div>
    </div>

    <div class="api-rotation-side">
      <div class="api-rotation-side-card api-rotation-summary ui-panel-soft">
        <div class={`api-rotation-summary-banner ${selectedModeTone}`}>
          <span class="api-rotation-summary-badge">Status</span>
          <p>{selectedModeSummary}</p>
        </div>

        <div class="api-rotation-inline-backoff">
          <div class="api-rotation-inline-backoff-copy">
            <span class="api-rotation-inline-label">Automatic Backoff</span>
            <p>Provider retries run first. Temporary backoff starts automatically after the final failure.</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</CollapsibleSurfaceSection>
