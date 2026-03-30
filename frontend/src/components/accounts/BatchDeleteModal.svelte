<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { TriangleAlert, Trash2 } from 'lucide-svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'
  import { formatNumber } from '@/utils/formatters'

  export let open = false
  export let count = 0
  export let busy = false
  export let title = 'Delete Selected Accounts'
  export let description = 'This action will remove selected records from local storage.'
  export let summaryLabel = 'selected account(s)'
  export let confirmLabel = 'Delete Selected'

  const dispatch = createEventDispatcher<{ confirm: void; cancel: void }>()

  const closeModal = (): void => {
    if (busy) {
      return
    }
    dispatch('cancel')
  }
</script>

<BaseModal
  {open}
  overlayClass="items-center justify-center p-3 sm:p-4"
  cardClass="batch-delete-modal w-full max-w-md overflow-hidden"
  headerClass="border-b border-border px-4 py-3"
  bodyClass="space-y-2 px-4 py-3 text-sm text-text-secondary"
  footerClass="flex items-center justify-end gap-2 border-t border-border px-4 py-3"
  on:close={closeModal}
>
  <svelte:fragment slot="header">
    <div class="flex items-start gap-2">
      <span class="danger-icon inline-flex h-7 w-7 items-center justify-center rounded-sm border">
        <TriangleAlert size={14} />
      </span>
      <div>
        <h3 class="text-sm font-semibold text-text-primary">{title}</h3>
        <p class="mt-0.5 text-xs text-text-secondary">{description}</p>
      </div>
    </div>
  </svelte:fragment>

  <p>
    You are about to delete <span class="font-semibold text-text-primary">{formatNumber(count)}</span> {summaryLabel}.
  </p>
  <p class="text-xs">This action cannot be undone.</p>

  <svelte:fragment slot="footer">
    <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</Button>
    <Button variant="danger" size="sm" disabled={busy} on:click={() => dispatch('confirm')}>
      <Trash2 size={14} class="mr-1" />
      {busy ? 'Deleting...' : confirmLabel}
    </Button>
  </svelte:fragment>
</BaseModal>
