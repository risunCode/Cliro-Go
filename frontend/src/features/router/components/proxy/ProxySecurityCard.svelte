<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Copy, KeyRound, Pencil, RefreshCw, Save, X } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import ProxyInlineSwitch from '@/features/router/components/proxy/ProxyInlineSwitch.svelte'
  import type { ProxyStatus } from '@/features/router/types'
  import { copyTextToClipboard, hasClipboardWrite } from '@/shared/lib/browser'

  export let proxyStatus: ProxyStatus | null = null
  export let busy = false
  export let onSetProxyAPIKey: (apiKey: string) => Promise<void>
  export let onRegenerateProxyAPIKey: () => Promise<string>
  export let onSetAuthorizationMode: (enabled: boolean) => Promise<void>

  let authorizationModeInput = false
  let apiKeyInput = ''
  let apiKeyDraft = ''
  let editingAPIKey = false
  let apiKeyDirty = false
  let apiKeyError = ''
  let apiKeyCopied = false
  let apiKeyCopyTimer: ReturnType<typeof setTimeout> | null = null

  $: if (proxyStatus && !busy) {
    authorizationModeInput = proxyStatus.authorizationMode
  }

  $: if (proxyStatus && !editingAPIKey && !apiKeyDirty) {
    apiKeyInput = proxyStatus.proxyApiKey || ''
    apiKeyDraft = proxyStatus.proxyApiKey || ''
  }

  $: protectionSummary = authorizationModeInput
    ? 'Every routed request must include the configured API key.'
    : 'Requests can reach the local runtime without API-key enforcement.'

  const visibleAPIKey = (): string => {
    return editingAPIKey ? apiKeyDraft : apiKeyInput
  }

  const handleAuthorizationChange = async (event: CustomEvent<boolean>): Promise<void> => {
    authorizationModeInput = event.detail
    await onSetAuthorizationMode(authorizationModeInput)
  }

  const startEditingAPIKey = (): void => {
    apiKeyError = ''
    apiKeyDraft = apiKeyInput
    apiKeyDirty = false
    editingAPIKey = true
  }

  const cancelEditingAPIKey = (): void => {
    apiKeyError = ''
    apiKeyDraft = apiKeyInput
    apiKeyDirty = false
    editingAPIKey = false
  }

  const saveProxyAPIKey = async (): Promise<void> => {
    const normalized = apiKeyDraft.trim()
    if (!normalized) {
      apiKeyError = 'API key cannot be empty.'
      return
    }

    apiKeyError = ''
    await onSetProxyAPIKey(normalized)
    apiKeyInput = normalized
    apiKeyDraft = normalized
    apiKeyDirty = false
    editingAPIKey = false
  }

  const regenerateProxyAPIKey = async (): Promise<void> => {
    apiKeyError = ''
    const nextAPIKey = await onRegenerateProxyAPIKey()
    apiKeyInput = nextAPIKey
    apiKeyDraft = nextAPIKey
    apiKeyDirty = false
    editingAPIKey = false
  }

  const copyProxyAPIKey = async (): Promise<void> => {
    const currentAPIKey = visibleAPIKey().trim()
    if (!hasClipboardWrite() || !currentAPIKey) {
      return
    }

    const copied = await copyTextToClipboard(currentAPIKey)
    if (!copied) {
      return
    }

    apiKeyCopied = true
    if (apiKeyCopyTimer) {
      clearTimeout(apiKeyCopyTimer)
    }
    apiKeyCopyTimer = setTimeout(() => {
      apiKeyCopied = false
      apiKeyCopyTimer = null
    }, 1200)
  }

  const updateAPIKeyDraft = (event: Event): void => {
    apiKeyDraft = (event.currentTarget as HTMLInputElement).value
    apiKeyDirty = apiKeyDraft !== apiKeyInput
    apiKeyError = ''
  }

  const previewAPIKey = (value: string): string => {
    if (!value) {
      return 'No API key configured'
    }
    if (value.length <= 25) {
      return value
    }
    return `${value.substring(0, 12)}...${value.substring(value.length - 8)}`
  }

  onDestroy(() => {
    if (apiKeyCopyTimer) {
      clearTimeout(apiKeyCopyTimer)
    }
  })
</script>

<section class="proxy-panel proxy-security-card">
  <div class="proxy-panel-header">
    <div>
      <p class="proxy-panel-kicker">Policy & Credentials</p>
      <h4 class="proxy-panel-title">Request protection and API key</h4>
    </div>
    {#if busy}
      <span class="proxy-panel-spinner">
        <RefreshCw size={14} class="animate-spin" />
      </span>
    {/if}
  </div>

  <div class="proxy-security-banner">
    <div class="proxy-security-copy">
      <span class="proxy-security-label">Protection Mode</span>
      <p class="proxy-security-description">{protectionSummary}</p>
    </div>

    <div class="proxy-security-meta">
      <StatusBadge tone={authorizationModeInput ? 'success' : 'neutral'}>
        {authorizationModeInput ? 'Protected' : 'Open'}
      </StatusBadge>
      <ProxyInlineSwitch checked={authorizationModeInput} on:change={handleAuthorizationChange} disabled={busy} />
    </div>
  </div>

  <div class="proxy-vault-card">
    <div class="proxy-vault-head">
      <div>
        <span class="proxy-vault-label">Proxy API Key</span>
        <p class="proxy-vault-description">Accepted by <code>Authorization: Bearer</code> and <code>X-API-Key</code>.</p>
      </div>

      <StatusBadge tone={apiKeyInput.trim() ? 'success' : 'warning'}>
        {apiKeyInput.trim() ? 'Configured' : 'Missing'}
      </StatusBadge>
    </div>

    {#if editingAPIKey}
      <div class="proxy-vault-edit-shell">
        <input
          type="text"
          value={apiKeyDraft}
          on:input={updateAPIKeyDraft}
          class="ui-control-input ui-control-select proxy-vault-input"
          placeholder="Enter proxy API key"
          disabled={busy}
        />

        <div class="proxy-vault-actions">
          <Button variant="secondary" size="sm" className="proxy-vault-action-btn" on:click={cancelEditingAPIKey} disabled={busy}>
            <X size={13} class="mr-1" /> Cancel
          </Button>
          <Button variant="primary" size="sm" className="proxy-vault-action-btn" on:click={saveProxyAPIKey} disabled={busy || !apiKeyDirty}>
            <Save size={13} class="mr-1" /> Save
          </Button>
        </div>
      </div>
    {:else}
      <div class="proxy-vault-display-shell">
        <div class="proxy-vault-value" title={apiKeyInput}>{previewAPIKey(apiKeyInput)}</div>

        <div class="proxy-vault-actions compact">
          <Button variant="secondary" size="sm" className="proxy-vault-icon-btn" on:click={startEditingAPIKey} disabled={busy} title="Edit API Key">
            <Pencil size={13} />
          </Button>
          <Button variant="secondary" size="sm" className="proxy-vault-icon-btn" on:click={regenerateProxyAPIKey} disabled={busy} title="Regenerate API Key">
            <KeyRound size={13} />
          </Button>
          <Button
            variant="secondary"
            size="sm"
            className="proxy-vault-icon-btn"
            on:click={copyProxyAPIKey}
            disabled={!hasClipboardWrite() || apiKeyInput.trim().length === 0}
            title="Copy API Key"
          >
            {#if apiKeyCopied}
              <span class="proxy-vault-copy-ok">OK</span>
            {:else}
              <Copy size={13} />
            {/if}
          </Button>
        </div>
      </div>
    {/if}

    {#if apiKeyError}
      <p class="proxy-vault-error">{apiKeyError}</p>
    {/if}
  </div>
</section>

<style>
  .proxy-panel {
    display: flex;
    flex-direction: column;
    min-width: 0;
    border-radius: 0.92rem;
    border: 1px solid color-mix(in srgb, #14b8a6 24%, var(--color-border));
    padding: 0.82rem 0.88rem;
    background:
      linear-gradient(180deg, color-mix(in srgb, white 3%, transparent), transparent 34%),
      color-mix(in srgb, var(--color-app) 93%, white 2%);
    box-shadow:
      inset 0 1px 0 color-mix(in srgb, white 4%, transparent),
      0 14px 24px rgba(0, 0, 0, 0.08);
  }

  .proxy-security-card {
    border-color: color-mix(in srgb, #14b8a6 30%, var(--color-border));
  }

  .proxy-panel-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.55rem;
  }

  .proxy-panel-kicker {
    font-size: 0.62rem;
    font-weight: 800;
    letter-spacing: 0.13em;
    text-transform: uppercase;
    color: color-mix(in srgb, #14b8a6 58%, var(--color-text-secondary));
  }

  .proxy-panel-title {
    margin-top: 0.22rem;
    font-size: 0.92rem;
    line-height: 1.2;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--color-text-primary);
  }

  .proxy-panel-spinner {
    color: var(--color-text-secondary);
    flex-shrink: 0;
  }

  .proxy-security-banner,
  .proxy-vault-card {
    padding: 0.62rem 0;
    border-bottom: 1px solid color-mix(in srgb, #14b8a6 12%, var(--color-border));
  }

  .proxy-vault-card {
    margin-top: 0;
    border-bottom: none;
    padding-bottom: 0;
  }

  .proxy-security-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.85rem;
  }

  .proxy-security-label,
  .proxy-vault-label {
    display: block;
    font-size: 0.6rem;
    font-weight: 700;
    letter-spacing: 0.11em;
    text-transform: uppercase;
    color: var(--color-text-secondary);
  }

  .proxy-security-description,
  .proxy-vault-description {
    margin-top: 0.18rem;
    font-size: 0.7rem;
    line-height: 1.4;
    color: var(--color-text-secondary);
  }

  .proxy-security-meta {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .proxy-vault-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.8rem;
  }

  .proxy-vault-display-shell,
  .proxy-vault-edit-shell {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    margin-top: 0.55rem;
  }

  .proxy-vault-value {
    flex: 1;
    min-width: 0;
    padding: 0.68rem 0.78rem;
    border-radius: 0.72rem;
    border: 1px solid color-mix(in srgb, #14b8a6 28%, var(--color-border));
    background:
      linear-gradient(135deg, color-mix(in srgb, #14b8a6 8%, transparent), transparent 64%),
      color-mix(in srgb, var(--color-surface) 96%, white 2%);
    font-family: 'IBM Plex Mono', 'Consolas', monospace;
    font-size: 0.76rem;
    font-weight: 700;
    color: var(--color-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .proxy-vault-input {
    flex: 1;
    min-width: 0;
    font-family: 'IBM Plex Mono', 'Consolas', monospace;
    font-size: 0.76rem;
    background: color-mix(in srgb, var(--color-surface) 96%, white 2%);
  }

  .proxy-vault-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    flex-shrink: 0;
  }

  .proxy-vault-actions.compact {
    gap: 0.35rem;
  }

  :global(.proxy-vault-action-btn) {
    min-width: 4.9rem;
  }

  :global(.proxy-vault-icon-btn) {
    min-width: 2.1rem;
    min-height: 2.1rem;
    padding-inline: 0.55rem;
  }

  .proxy-vault-copy-ok {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 1rem;
    font-size: 0.62rem;
    font-weight: 800;
    letter-spacing: 0.08em;
  }

  .proxy-vault-error {
    margin-top: 0.4rem;
    font-size: 0.68rem;
    color: var(--color-error);
  }

  code {
    font-family: 'IBM Plex Mono', 'Consolas', monospace;
    font-size: 0.72rem;
  }

  @media (max-width: 767px) {
    .proxy-security-banner,
    .proxy-vault-head,
    .proxy-vault-display-shell,
    .proxy-vault-edit-shell {
      flex-direction: column;
      align-items: stretch;
    }

    .proxy-security-meta,
    .proxy-vault-actions {
      width: 100%;
      justify-content: space-between;
    }

    :global(.proxy-vault-action-btn),
    .proxy-vault-input {
      width: 100%;
    }
  }
</style>
