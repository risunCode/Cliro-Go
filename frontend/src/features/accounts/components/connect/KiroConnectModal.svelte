<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'
  import ModalWindowHeader from '@/components/common/ModalWindowHeader.svelte'

  export let open = false
  export let authUrl = ''
  export let userCode = ''
  export let authMethod = ''
  export let busy = false
  export let pending = false
  export let canCopyLink = true

  const dispatch = createEventDispatcher<{
    startDevice: void
    startGoogle: void
    startGithub: void
    openLink: void
    copyLink: void
    copyCode: void
    cancel: void
    dismiss: void
  }>()

  $: hasActiveSession = Boolean(authUrl || userCode || pending)
  $: isDeviceFlow = (authMethod || '').toLowerCase() === 'device' || (!!userCode && hasActiveSession)
  $: isSocialFlow = (authMethod || '').toLowerCase() === 'social'

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
    <ModalWindowHeader
      title="Connect KiroAI Account"
      description="Connect with AWS Builder ID device authorization."
    />
  </svelte:fragment>

  {#if !hasActiveSession}
    <div class="space-y-4">
      <p>Choose how you want to connect your Kiro account:</p>

      <div class="space-y-2">
        <Button
          variant="primary"
          size="sm"
          disabled={busy}
          on:click={() => dispatch('startGoogle')}
          class="w-full justify-start"
        >
          <svg class="mr-2 h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
            <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
            <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
            <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
            <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
          </svg>
          Continue with Google
        </Button>

        <Button
          variant="secondary"
          size="sm"
          disabled={busy}
          on:click={() => dispatch('startGithub')}
          class="w-full justify-start"
        >
          <svg class="mr-2 h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
          </svg>
          Continue with GitHub
        </Button>

        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          on:click={() => dispatch('startDevice')}
          class="w-full justify-start"
        >
          <svg class="mr-2 h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
            <line x1="8" y1="21" x2="16" y2="21"/>
            <line x1="12" y1="17" x2="12" y2="21"/>
          </svg>
          AWS Builder ID (Device Auth)
        </Button>
      </div>

      <p class="text-xs text-text-secondary">
        Social auth (Google/GitHub) opens browser for quick login. Device auth requires entering a code manually.
      </p>
    </div>
  {:else}
    {#if isDeviceFlow}
      <p>
        Open the verification link, sign in with AWS Builder ID, then enter the code shown below.
      </p>

      <p class="text-xs uppercase tracking-[0.08em]">Device Code</p>
      <p class="rounded-sm border border-border bg-app p-3 font-mono text-sm font-semibold tracking-[0.2em] text-text-primary">{userCode || '-'}</p>

      <details class="mt-2">
        <summary class="cursor-pointer text-xs text-text-secondary hover:text-text-primary">Show verification link</summary>
        <div class="mt-2 space-y-2">
          <p class="text-xs uppercase tracking-[0.08em]">Verification Link</p>
          <p class="break-all rounded-sm border border-border bg-app p-3 font-mono text-xs">{authUrl || '-'}</p>
        </div>
      </details>

      {#if pending}
        <div class="flex items-center gap-2 rounded-sm border border-border bg-app px-3 py-2">
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-border border-t-text-primary" aria-hidden="true"></span>
          <p class="text-xs font-semibold text-text-primary">Waiting for device authorization...</p>
        </div>
        <p class="text-xs">You can dismiss this modal without cancelling the current Kiro auth session.</p>
      {/if}
    {:else if isSocialFlow}
      <p>
        Click "Open Link" to sign in with your selected provider. CLIro-Go will automatically open and complete the login.
      </p>

      <details class="mt-2">
        <summary class="cursor-pointer text-xs text-text-secondary hover:text-text-primary">Show authorization link</summary>
        <div class="mt-2 space-y-2">
          <p class="text-xs uppercase tracking-[0.08em]">Authorization Link</p>
          <p class="break-all rounded-sm border border-border bg-app p-3 font-mono text-xs">{authUrl || '-'}</p>
        </div>
      </details>

      {#if pending}
        <div class="flex items-center gap-2 rounded-sm border border-border bg-app px-3 py-2">
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-border border-t-text-primary" aria-hidden="true"></span>
          <p class="text-xs font-semibold text-text-primary">
            Continuing authentication process...
          </p>
        </div>
      {/if}
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
