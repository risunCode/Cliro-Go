<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'

  export let open = false
  export let currentVersion = ''
  export let latestVersion = ''
  export let releaseName = ''
  export let releaseUrl = ''

  const dispatch = createEventDispatcher<{ dismiss: void; openRelease: void }>()

  const closeModal = (): void => {
    dispatch('dismiss')
  }
</script>

<BaseModal
  {open}
  overlayClass="items-center justify-center p-4"
  cardClass="w-full max-w-xl overflow-hidden"
  headerClass="border-b border-border px-5 py-4"
  bodyClass="space-y-3 px-5 py-4 text-sm text-text-secondary"
  footerClass="flex items-center justify-end gap-2 border-t border-border px-5 py-4"
  on:close={closeModal}
>
  <svelte:fragment slot="header">
    <h2 class="text-base font-semibold text-text-primary">Update Required</h2>
    <p class="mt-1 text-sm text-text-secondary">This CLIro-Go version is no longer supported. Update to the latest release now.</p>
  </svelte:fragment>

  <div class="grid gap-2 rounded-sm border border-border bg-app p-3 text-xs">
    <p><span class="text-text-primary">Current:</span> <span class="font-mono">{currentVersion || '-'}</span></p>
    <p><span class="text-text-primary">Latest:</span> <span class="font-mono">{latestVersion || '-'}</span></p>
    {#if releaseName}
      <p><span class="text-text-primary">Release:</span> {releaseName}</p>
    {/if}
  </div>

  <p class="text-xs">Download the latest build and replace your current executable from the release page.</p>

  <svelte:fragment slot="footer">
    <Button variant="primary" size="sm" disabled={!releaseUrl} on:click={() => dispatch('openRelease')}>Update Now</Button>
  </svelte:fragment>
</BaseModal>
