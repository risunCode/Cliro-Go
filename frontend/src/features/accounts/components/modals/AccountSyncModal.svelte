<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { TriangleAlert, ArrowRightLeft, CircleCheckBig, ChevronRight, LoaderCircle } from 'lucide-svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'
  import { ACCOUNT_SYNC_TARGETS } from '@/features/accounts/lib/sync'
  import type { Account, AccountSyncResult, SyncTargetID } from '@/features/accounts/types'

  export let open = false
  export let account: Account | null = null
  export let loading = false
  export let result: AccountSyncResult | null = null
  export let error = ''
  export let selectedTargetID: SyncTargetID = 'kilo-cli'

  const dispatch = createEventDispatcher<{ close: void; confirm: SyncTargetID }>()

  let targetID: SyncTargetID = 'kilo-cli'
  let wasOpen = false

  $: if (open && !wasOpen) {
    targetID = selectedTargetID
  }
  $: wasOpen = open
  $: activeTarget = ACCOUNT_SYNC_TARGETS.find((target) => target.id === targetID) || ACCOUNT_SYNC_TARGETS[0]
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

<BaseModal
  {open}
  overlayClass="items-center justify-center p-3 sm:p-4"
  cardClass="accounts-sync-modal sync-modal w-full max-w-2xl overflow-hidden"
  headerClass="sync-head border-b border-border px-4 py-3"
  bodyClass="sync-body px-4 py-3 text-sm text-text-secondary"
  footerClass="sync-foot flex items-center justify-end gap-2 border-t border-border px-4 py-3"
  on:close={closeModal}
>
  <svelte:fragment slot="header">
    <div class="sync-head-main">
      <div class={`sync-icon ${loading ? 'is-loading' : result ? 'is-success' : error ? 'is-error' : ''}`}>
        {#if loading}
          <LoaderCircle size={16} class="accounts-sync-spinning" />
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
  </svelte:fragment>

  {#if loading}
    <div class="state-panel ui-panel-soft">
      <LoaderCircle size={16} class="accounts-sync-spinning" />
      <div>
        <p class="state-title">Syncing {activeTarget.name}...</p>
        <p class="state-copy">Updating the target auth file with the selected account tokens.</p>
      </div>
    </div>
  {:else if result}
    <div class="result-grid">
      <div class="result-item ui-panel-soft">
        <span class="result-label">Target Path</span>
        <code class="result-value break-all">{result.targetPath || '-'}</code>
      </div>
      <div class="result-item ui-panel-soft">
        <span class="result-label">File State</span>
        <span class="result-value">{result.fileExisted ? 'Updated existing file' : 'Created new file'}</span>
      </div>
      {#if result.target !== 'codex-cli'}
        <div class="result-item ui-panel-soft">
          <span class="result-label">OpenAI Block</span>
          <span class="result-value">{result.openAICreated ? 'Created' : 'Updated'}</span>
        </div>
        <div class="result-item ui-panel-soft">
          <span class="result-label">Synced Expires</span>
          <span class="result-value">{result.syncedExpiresAt || String(result.syncedExpires || 0)}</span>
        </div>
      {:else}
        <div class="result-item ui-panel-soft">
          <span class="result-label">Backup</span>
          <span class="result-value">{result.backupCreated ? 'Created' : 'Not created'}</span>
        </div>
        <div class="result-item ui-panel-soft">
          <span class="result-label">Synced At</span>
          <span class="result-value">{result.syncedAt || '-'}</span>
        </div>
        {#if result.backupPath}
          <div class="result-item result-item-full ui-panel-soft">
            <span class="result-label">Backup Path</span>
            <code class="result-value break-all">{result.backupPath}</code>
          </div>
        {/if}
      {/if}
      <div class="result-item result-item-full ui-panel-soft">
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
      {#each ACCOUNT_SYNC_TARGETS as target (target.id)}
        <button
          type="button"
          class={`target-item ui-panel-soft ${targetID === target.id ? 'is-active' : ''}`}
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
      <div class="error-panel ui-panel-soft">
        <TriangleAlert size={14} />
        <p>{error}</p>
      </div>
    {/if}
  {/if}

  <svelte:fragment slot="footer">
    {#if result}
      <Button variant="primary" size="sm" on:click={closeModal}>Done</Button>
    {:else}
      <Button variant="ghost" size="sm" disabled={loading} on:click={closeModal}>Cancel</Button>
      <Button variant="primary" size="sm" disabled={loading} on:click={confirmSync}>
        <ArrowRightLeft size={14} class="mr-1" />
        Sync to {activeTarget.name}
      </Button>
    {/if}
  </svelte:fragment>
</BaseModal>
