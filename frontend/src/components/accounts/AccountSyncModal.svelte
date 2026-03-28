<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { TriangleAlert, ArrowRightLeft, CircleCheckBig, ChevronRight, LoaderCircle } from 'lucide-svelte'
  import ModalBackdrop from '@/components/common/ModalBackdrop.svelte'
  import Button from '@/components/common/Button.svelte'
  import type { Account, AccountSyncResult, SyncTargetID } from '@/services/wails-api'

  export let open = false
  export let account: Account | null = null
  export let loading = false
  export let result: AccountSyncResult | null = null
  export let error = ''
  export let selectedTargetID: SyncTargetID = 'kilo-cli'

  const dispatch = createEventDispatcher<{ close: void; confirm: SyncTargetID }>()

  const syncTargets: Array<{ id: SyncTargetID; name: string; path: string; description: string }> = [
    {
      id: 'kilo-cli',
      name: 'Kilo CLI',
      path: '~/.local/share/kilo/auth.json',
      description: 'Sync this Codex account into the Kilo CLI auth file.'
    },
    {
      id: 'opencode-cli',
      name: 'Opencode',
      path: '~/.local/share/opencode/auth.json',
      description: 'Sync this Codex account into the Opencode auth file.'
    },
    {
      id: 'codex-cli',
      name: 'Codex CLI',
      path: '~/.codex/auth.json',
      description: 'Sync this Codex account into the Codex CLI auth file.'
    }
  ]

  let targetID: SyncTargetID = 'kilo-cli'
  let wasOpen = false

  $: if (open && !wasOpen) {
    targetID = selectedTargetID
  }
  $: wasOpen = open
  $: activeTarget = syncTargets.find((target) => target.id === targetID) || syncTargets[0]
  $: updatedFields = Array.isArray(result?.updatedFields) ? result.updatedFields : []
  $: accountLabel = account?.email || account?.accountId || account?.id || '-'

  const closeModal = (): void => {
    if (loading) {
      return
    }
    dispatch('close')
  }

  const confirmSync = (): void => {
    if (loading) {
      return
    }
    dispatch('confirm', targetID)
  }
</script>

{#if open}
  <ModalBackdrop on:close={closeModal} />

  <div class="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4">
    <div class="ui-surface-card sync-modal w-full max-w-2xl overflow-hidden">
      <header class="sync-head border-b border-border px-4 py-3">
        <div class="sync-head-main">
          <div class={`sync-icon ${loading ? 'is-loading' : result ? 'is-success' : error ? 'is-error' : ''}`}>
            {#if loading}
              <LoaderCircle size={16} class="is-spinning" />
            {:else if result}
              <CircleCheckBig size={16} />
            {:else if error}
              <TriangleAlert size={16} />
            {:else}
              <ArrowRightLeft size={16} />
            {/if}
          </div>
          <div class="sync-head-copy">
            <h3>{result ? 'Sync Complete' : 'Sync Account'}</h3>
            <p>{result ? 'Target auth file was updated successfully.' : `Selected account: ${accountLabel}`}</p>
          </div>
        </div>
      </header>

      <div class="sync-body px-4 py-3 text-sm text-text-secondary">
        {#if loading}
          <div class="state-panel">
            <LoaderCircle size={16} class="is-spinning" />
            <div>
              <p class="state-title">Syncing {activeTarget.name}...</p>
              <p class="state-copy">Updating the target auth file with the selected account tokens.</p>
            </div>
          </div>
        {:else if result}
          <div class="result-grid">
            <div class="result-item">
              <span class="result-label">Target Path</span>
              <code class="result-value break-all">{result.targetPath || '-'}</code>
            </div>
            <div class="result-item">
              <span class="result-label">File State</span>
              <span class="result-value">{result.fileExisted ? 'Updated existing file' : 'Created new file'}</span>
            </div>
            {#if result.target !== 'codex-cli'}
              <div class="result-item">
                <span class="result-label">OpenAI Block</span>
                <span class="result-value">{result.openAICreated ? 'Created' : 'Updated'}</span>
              </div>
              <div class="result-item">
                <span class="result-label">Synced Expires</span>
                <span class="result-value">{result.syncedExpiresAt || String(result.syncedExpires || 0)}</span>
              </div>
            {:else}
              <div class="result-item">
                <span class="result-label">Backup</span>
                <span class="result-value">{result.backupCreated ? 'Created' : 'Not created'}</span>
              </div>
              <div class="result-item">
                <span class="result-label">Synced At</span>
                <span class="result-value">{result.syncedAt || '-'}</span>
              </div>
              {#if result.backupPath}
                <div class="result-item result-item-full">
                  <span class="result-label">Backup Path</span>
                  <code class="result-value break-all">{result.backupPath}</code>
                </div>
              {/if}
            {/if}
            <div class="result-item result-item-full">
              <span class="result-label">Updated Fields</span>
              <div class="field-list">
                {#each updatedFields as field (field)}
                  <code>{field}</code>
                {:else}
                  <span>-</span>
                {/each}
              </div>
            </div>
          </div>
        {:else}
          <div class="target-list" role="radiogroup" aria-label="Sync target">
            {#each syncTargets as target (target.id)}
              <button
                type="button"
                class={`target-item ${targetID === target.id ? 'is-active' : ''}`}
                role="radio"
                aria-checked={targetID === target.id}
                on:click={() => {
                  targetID = target.id
                }}
              >
                <div class="target-copy">
                  <p class="target-name">{target.name}</p>
                  <p class="target-desc">{target.description}</p>
                  <code class="target-path">{target.path}</code>
                </div>
                <ChevronRight size={16} />
              </button>
            {/each}
          </div>

          {#if error}
            <div class="error-panel">
              <TriangleAlert size={14} />
              <p>{error}</p>
            </div>
          {/if}
        {/if}
      </div>

      <footer class="sync-foot flex items-center justify-end gap-2 border-t border-border px-4 py-3">
        {#if result}
          <Button variant="primary" size="sm" on:click={closeModal}>Done</Button>
        {:else}
          <Button variant="ghost" size="sm" disabled={loading} on:click={closeModal}>Cancel</Button>
          <Button variant="primary" size="sm" disabled={loading} on:click={confirmSync}>
            <ArrowRightLeft size={14} class="mr-1" />
            Sync to {activeTarget.name}
          </Button>
        {/if}
      </footer>
    </div>
  </div>
{/if}

<style>
  .sync-modal {
    max-height: min(82vh, 44rem);
    display: flex;
    flex-direction: column;
  }

  .sync-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .sync-head-main {
    display: flex;
    gap: 0.75rem;
    min-width: 0;
  }

  .sync-head-copy h3 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .sync-head-copy p {
    margin: 0.25rem 0 0;
    font-size: 0.75rem;
    color: var(--color-text-secondary);
  }

  .sync-icon {
    width: 1.9rem;
    height: 1.9rem;
    border-radius: 0.5rem;
    border: 1px solid var(--color-border);
    color: var(--color-text-secondary);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    background: color-mix(in srgb, var(--color-app) 88%, transparent);
  }

  .sync-icon.is-loading {
    border-color: color-mix(in srgb, var(--color-warning) 40%, var(--color-border));
    color: var(--color-warning);
  }

  .sync-icon.is-success {
    border-color: color-mix(in srgb, var(--color-success) 40%, var(--color-border));
    color: var(--color-success);
  }

  .sync-icon.is-error {
    border-color: color-mix(in srgb, var(--color-error) 40%, var(--color-border));
    color: var(--color-error);
  }

  .sync-body {
    overflow-y: auto;
    display: grid;
    gap: 0.75rem;
  }

  .state-panel,
  .error-panel {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    border: 1px solid var(--color-border);
    border-radius: 0.5rem;
    padding: 0.65rem 0.75rem;
    background: color-mix(in srgb, var(--color-app) 88%, transparent);
  }

  .state-title {
    margin: 0;
    color: var(--color-text-primary);
    font-size: 0.82rem;
    font-weight: 600;
  }

  .state-copy {
    margin: 0.25rem 0 0;
    color: var(--color-text-secondary);
    font-size: 0.74rem;
  }

  .target-list {
    display: grid;
    gap: 0.6rem;
  }

  .target-item {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    text-align: left;
    border: 1px solid var(--color-border);
    border-radius: 0.6rem;
    padding: 0.7rem 0.8rem;
    color: var(--color-text-secondary);
    background: color-mix(in srgb, var(--color-app) 86%, transparent);
    transition: border-color 0.2s ease, background-color 0.2s ease, color 0.2s ease;
  }

  .target-item:hover {
    border-color: var(--border-hover);
    color: var(--color-text-primary);
  }

  .target-item.is-active {
    border-color: color-mix(in srgb, var(--primary) 42%, var(--border-hover));
    background: color-mix(in srgb, var(--primary) 12%, var(--color-app));
    color: var(--color-text-primary);
  }

  .target-name {
    margin: 0;
    font-size: 0.82rem;
    color: inherit;
    font-weight: 600;
  }

  .target-desc {
    margin: 0.22rem 0 0;
    font-size: 0.73rem;
    color: var(--color-text-secondary);
  }

  .target-path {
    display: inline-flex;
    margin-top: 0.35rem;
    padding: 0.15rem 0.35rem;
    border-radius: 0.35rem;
    font-size: 0.68rem;
    border: 1px solid var(--color-border);
    background: color-mix(in srgb, var(--color-app) 92%, transparent);
  }

  .error-panel {
    border-color: color-mix(in srgb, var(--color-error) 36%, var(--color-border));
    color: var(--color-error);
    background: color-mix(in srgb, var(--color-error) 12%, transparent);
  }

  .error-panel p {
    margin: 0;
    color: inherit;
    font-size: 0.75rem;
    line-height: 1.45;
  }

  .result-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.6rem;
  }

  .result-item {
    border: 1px solid var(--color-border);
    border-radius: 0.5rem;
    padding: 0.55rem 0.65rem;
    display: grid;
    gap: 0.25rem;
    background: color-mix(in srgb, var(--color-app) 86%, transparent);
  }

  .result-item-full {
    grid-column: 1 / -1;
  }

  .result-label {
    font-size: 0.65rem;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--color-text-secondary);
    font-weight: 600;
  }

  .result-value {
    font-size: 0.78rem;
    color: var(--color-text-primary);
    line-height: 1.4;
    overflow-wrap: anywhere;
  }

  .field-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .field-list code,
  .field-list span {
    display: inline-flex;
    align-items: center;
    min-height: 1.25rem;
    border-radius: 999px;
    border: 1px solid var(--color-border);
    background: color-mix(in srgb, var(--color-app) 92%, transparent);
    color: var(--color-text-primary);
    font-size: 0.68rem;
    padding: 0 0.5rem;
  }

  .is-spinning {
    animation: sync-spin 1s linear infinite;
  }

  @keyframes sync-spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }

  @media (max-width: 720px) {
    .result-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
