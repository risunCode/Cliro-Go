<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'
  import ModalWindowHeader from '@/components/common/ModalWindowHeader.svelte'

  export let open = false
  export let authUrl = ''
  export let busy = false
  export let pending = false
  export let canCopyLink = true

  const dispatch = createEventDispatcher<{ openLink: void; copyLink: void; submitCode: { code: string }; cancel: void; dismiss: void }>()

  let manualCode = ''

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
    <ModalWindowHeader
      title="Connect Codex Account"
      description="Open the OAuth link manually to continue account connection."
    />
  </svelte:fragment>

  <p>Click "Open Link" to authorize your Codex account.</p>

  <details class="mt-2">
    <summary class="cursor-pointer text-xs text-text-secondary hover:text-text-primary">Show authorization link</summary>
    <div class="mt-2 space-y-2">
      <p class="text-xs uppercase tracking-[0.08em]">Authorization Link</p>
      <p class="break-all rounded-sm border border-border bg-app p-3 font-mono text-xs">{authUrl || '-'}</p>
    </div>
  </details>

  <details class="mt-2">
    <summary class="cursor-pointer text-xs text-text-secondary hover:text-text-primary">Manual code entry (if callback is blocked)</summary>
    <div class="mt-2 space-y-2">
      <p class="text-xs uppercase tracking-[0.08em]">Authorization Code</p>
      <p class="text-xs text-text-secondary mb-1">Copy the <code>code</code> parameter from the callback URL and paste it here:</p>
      <input
        type="text"
        bind:value={manualCode}
        placeholder="Paste authorization code here..."
        disabled={busy}
        class="w-full rounded-sm border border-border bg-app px-3 py-2 font-mono text-sm text-text-primary placeholder-text-secondary focus:border-text-primary focus:outline-none disabled:opacity-50"
      />
    </div>
  </details>

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
    <Button variant="primary" size="sm" disabled={busy || !manualCode.trim()} on:click={() => dispatch('submitCode', { code: manualCode.trim() })}>
      Submit Code
    </Button>
    <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</Button>
  </svelte:fragment>
</BaseModal>
