<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'

  export let open = false
  export let authUrl = ''
  export let busy = false
  export let pending = false
  export let canCopyLink = true

  const dispatch = createEventDispatcher<{ openLink: void; copyLink: void; cancel: void; dismiss: void }>()

  const closeModal = (): void => {
    dispatch('dismiss')
  }
</script>

<BaseModal
  {open}
  overlayClass="items-center justify-center p-4"
  cardClass="w-full max-w-lg overflow-hidden"
  headerClass="border-b border-border px-5 py-4"
  bodyClass="space-y-3 px-5 py-4 text-sm text-text-secondary"
  footerClass="flex items-center justify-end gap-2 border-t border-border px-5 py-4"
  on:close={closeModal}
>
  <svelte:fragment slot="header">
    <div class="mb-2 flex items-center gap-2">
      <span class="h-3 w-3 rounded-full bg-[#ef4444]" aria-hidden="true" />
      <span class="h-3 w-3 rounded-full bg-[#f59e0b]" aria-hidden="true" />
      <span class="h-3 w-3 rounded-full bg-[#22c55e]" aria-hidden="true" />
    </div>
    <h2 class="text-base font-semibold text-text-primary">Connect Codex Account</h2>
    <p class="mt-1 text-sm text-text-secondary">Open the OAuth link manually to continue account connection.</p>
  </svelte:fragment>

  <p class="text-xs uppercase tracking-[0.08em]">Authorization Link</p>
  <p class="break-all rounded-sm border border-border bg-app p-3 font-mono text-xs">{authUrl || '-'}</p>

  {#if pending}
    <div class="flex items-center gap-2 rounded-sm border border-border bg-app px-3 py-2">
      <span class="h-4 w-4 animate-spin rounded-full border-2 border-border border-t-text-primary" aria-hidden="true"></span>
      <p class="text-xs font-semibold text-text-primary">Waiting for authorization...</p>
    </div>
    <p class="text-xs">You can dismiss this modal without cancelling the current auth session.</p>
  {/if}

  <svelte:fragment slot="footer">
    <Button variant="secondary" size="sm" disabled={busy || !authUrl} on:click={() => dispatch('openLink')}>
      Open Link
    </Button>
    <Button variant="secondary" size="sm" disabled={busy || !authUrl || !canCopyLink} on:click={() => dispatch('copyLink')}>
      Copy Link
    </Button>
    <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</Button>
  </svelte:fragment>
</BaseModal>
