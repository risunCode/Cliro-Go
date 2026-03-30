<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'

  export let open = false
  export let authUrl = ''
  export let userCode = ''
  export let authMethod = ''
  export let provider = ''
  export let busy = false
  export let pending = false
  export let canCopyLink = true

  const dispatch = createEventDispatcher<{
    startDevice: void
    startGoogle: void
    startGitHub: void
    openLink: void
    copyLink: void
    copyCode: void
    cancel: void
    dismiss: void
  }>()

  $: hasActiveSession = Boolean(authUrl || userCode || pending)
  $: isDeviceFlow = (authMethod || '').toLowerCase() === 'device' || (!!userCode && hasActiveSession)
  $: socialProviderLabel = (() => {
    const normalized = (provider || '').toLowerCase()
    if (normalized === 'google') return 'Google'
    if (normalized === 'github') return 'GitHub'
    return 'social provider'
  })()

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
  footerClass="flex flex-wrap items-center justify-end gap-2 border-t border-border px-5 py-4"
  on:close={closeModal}
>
  <svelte:fragment slot="header">
    <div class="mb-2 flex items-center gap-2">
      <span class="h-3 w-3 rounded-full bg-[#ef4444]" aria-hidden="true" />
      <span class="h-3 w-3 rounded-full bg-[#f59e0b]" aria-hidden="true" />
      <span class="h-3 w-3 rounded-full bg-[#22c55e]" aria-hidden="true" />
    </div>
    <h2 class="text-base font-semibold text-text-primary">Connect KiroAI Account</h2>
    <p class="mt-1 text-sm text-text-secondary">Connect with AWS Builder ID device auth or a supported Kiro social login provider.</p>
  </svelte:fragment>

  {#if !hasActiveSession}
    <div class="space-y-3">
      <p>Select how you want to connect your Kiro account.</p>
      <div class="grid gap-2 sm:grid-cols-3">
        <Button variant="primary" size="sm" disabled={busy} on:click={() => dispatch('startGoogle')}>Google</Button>
        <Button variant="secondary" size="sm" disabled={busy} on:click={() => dispatch('startGitHub')}>GitHub</Button>
        <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('startDevice')}>AWS Builder ID</Button>
      </div>
      <p class="text-xs text-text-secondary">Social login opens a browser callback flow. AWS Builder ID uses a device code flow.</p>
    </div>
  {:else}
    <p>
      {#if isDeviceFlow}
        Open the verification link, sign in with AWS Builder ID, then enter the code shown below.
      {:else}
        Open the login link and complete the {socialProviderLabel} sign-in flow in your browser.
      {/if}
    </p>

    <p class="text-xs uppercase tracking-[0.08em]">Verification Link</p>
    <p class="break-all rounded-sm border border-border bg-app p-3 font-mono text-xs">{authUrl || '-'}</p>

    {#if isDeviceFlow}
      <p class="text-xs uppercase tracking-[0.08em]">Device Code</p>
      <p class="rounded-sm border border-border bg-app p-3 font-mono text-sm font-semibold tracking-[0.2em] text-text-primary">{userCode || '-'}</p>
    {/if}

    {#if pending}
      <div class="flex items-center gap-2 rounded-sm border border-border bg-app px-3 py-2">
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-border border-t-text-primary" aria-hidden="true"></span>
        <p class="text-xs font-semibold text-text-primary">
          {#if isDeviceFlow}
            Waiting for device authorization...
          {:else}
            Waiting for browser sign-in callback...
          {/if}
        </p>
      </div>
      <p class="text-xs">You can dismiss this modal without cancelling the current Kiro auth session.</p>
    {/if}
  {/if}

  <svelte:fragment slot="footer">
    {#if hasActiveSession}
      <Button variant="secondary" size="sm" disabled={busy || !authUrl} on:click={() => dispatch('openLink')}>
        Open Link
      </Button>
      <Button variant="secondary" size="sm" disabled={busy || !authUrl || !canCopyLink} on:click={() => dispatch('copyLink')}>
        Copy Link
      </Button>
      {#if isDeviceFlow}
        <Button variant="secondary" size="sm" disabled={busy || !userCode || !canCopyLink} on:click={() => dispatch('copyCode')}>
          Copy Code
        </Button>
      {/if}
      <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</Button>
    {:else}
      <Button variant="ghost" size="sm" disabled={busy} on:click={() => dispatch('dismiss')}>Close</Button>
    {/if}
  </svelte:fragment>
</BaseModal>
