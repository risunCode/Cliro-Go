<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { TriangleAlert, Trash2 } from 'lucide-svelte'
  import ModalBackdrop from '@/components/common/ModalBackdrop.svelte'
  import Button from '@/components/common/Button.svelte'
  import { formatNumber } from '@/utils/formatters'

  export let open = false
  export let count = 0
  export let busy = false

  const dispatch = createEventDispatcher<{ confirm: void; cancel: void }>()

  const closeModal = (): void => {
    if (busy) {
      return
    }
    dispatch('cancel')
  }
</script>

{#if open}
  <ModalBackdrop on:close={closeModal} />

  <div class="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4">
    <div class="ui-surface-card w-full max-w-md overflow-hidden">
      <header class="border-b border-border px-4 py-3">
        <div class="flex items-start gap-2">
          <span class="danger-icon inline-flex h-7 w-7 items-center justify-center rounded-sm border">
            <TriangleAlert size={14} />
          </span>
          <div>
            <h3 class="text-sm font-semibold text-text-primary">Delete Selected Accounts</h3>
            <p class="mt-0.5 text-xs text-text-secondary">This action will remove selected records from local storage.</p>
          </div>
        </div>
      </header>

      <div class="space-y-2 px-4 py-3 text-sm text-text-secondary">
        <p>
          You are about to delete <span class="font-semibold text-text-primary">{formatNumber(count)}</span> selected account(s).
        </p>
        <p class="text-xs">This action cannot be undone.</p>
      </div>

      <footer class="flex items-center justify-end gap-2 border-t border-border px-4 py-3">
        <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</Button>
        <Button variant="danger" size="sm" disabled={busy} on:click={() => dispatch('confirm')}>
          <Trash2 size={14} class="mr-1" />
          {busy ? 'Deleting...' : 'Delete Selected'}
        </Button>
      </footer>
    </div>
  </div>
{/if}

<style>
  .danger-icon {
    border-color: color-mix(in srgb, var(--color-error) 46%, var(--color-border));
    color: var(--color-error);
    background: color-mix(in srgb, var(--color-error) 12%, transparent);
  }
</style>
