<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { Copy, X, Save } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import MacOSModal from '@/components/common/MacOSModal.svelte'
  import type { CliSyncAppID, CliSyncResult, CliSyncStatus, LocalModelCatalogItem } from '@/services/wails-api-types'
  import { copyTextToClipboard, hasClipboardWrite } from '@/utils/browser'

  export let open = false
  export let appID: CliSyncAppID | '' = ''
  export let label = ''
  export let selectedModel = ''
  export let proxyBaseURL = ''
  export let proxyAPIKey = ''
  export let status: CliSyncStatus | null = null
  export let result: CliSyncResult | null = null
  export let availableModels: LocalModelCatalogItem[] = []
  export let onLoadFileContent: (appId: CliSyncAppID, path: string) => Promise<string>
  export let onSaveFileContent: (appId: CliSyncAppID, path: string, content: string) => Promise<void>

  const dispatch = createEventDispatcher<{ close: void; saved: void; error: string }>()

  let activeFilePath = ''
  let activeModelID = ''
  let editorContent = ''
  let loading = false
  let saving = false
  let dirty = false
  let error = ''
  let wasOpen = false

  const closeModal = (): void => {
    if (saving) {
      return
    }
    dispatch('close')
  }

  const loadActiveFile = async (): Promise<void> => {
    if (!open || !appID || !activeFilePath) {
      editorContent = ''
      dirty = false
      return
    }

    loading = true
    error = ''
    try {
      editorContent = await onLoadFileContent(appID, activeFilePath)
      dirty = false
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : 'Failed to load file content.'
      error = message
      dispatch('error', message)
      editorContent = ''
      dirty = false
    } finally {
      loading = false
    }
  }

  const selectFile = async (path: string): Promise<void> => {
    if (saving || path === activeFilePath) {
      return
    }
    activeFilePath = path
    await loadActiveFile()
  }

  const selectModel = (modelID: string): void => {
    activeModelID = modelID
  }

  const copyActiveModel = async (): Promise<void> => {
    if (!activeModelID) {
      return
    }
    await copyTextToClipboard(activeModelID)
  }

  const copyEditorContent = async (): Promise<void> => {
    if (!editorContent.trim()) {
      return
    }
    await copyTextToClipboard(editorContent)
  }

  const saveFile = async (): Promise<void> => {
    if (!appID || !activeFilePath || saving || loading || !dirty) {
      return
    }
    saving = true
    error = ''
    try {
      await onSaveFileContent(appID, activeFilePath, editorContent)
      dirty = false
      dispatch('saved')
    } catch (saveError) {
      const message = saveError instanceof Error ? saveError.message : 'Failed to save file content.'
      error = message
      dispatch('error', message)
    } finally {
      saving = false
    }
  }

  $: targetFiles = status?.files || result?.files || []
  $: modalTitle = `${label || 'CLI Config'} Content`

  $: if (!activeModelID) {
    activeModelID = selectedModel || availableModels[0]?.id || ''
  }

  $: if (open && !wasOpen) {
    activeFilePath = targetFiles[0]?.path || ''
    activeModelID = selectedModel || availableModels[0]?.id || ''
    void loadActiveFile()
  }

  $: if (open && activeFilePath && !targetFiles.some((file) => file.path === activeFilePath)) {
    activeFilePath = targetFiles[0]?.path || ''
    void loadActiveFile()
  }

  $: if (!open && wasOpen) {
    activeFilePath = ''
    activeModelID = ''
    editorContent = ''
    loading = false
    saving = false
    dirty = false
    error = ''
  }

  $: wasOpen = open
</script>

<MacOSModal open={open} title={modalTitle} size="lg" on:close={closeModal}>
  <svelte:fragment slot="titleActions">
    <button type="button" class="mini-icon-btn" on:click={() => void copyEditorContent()} disabled={!hasClipboardWrite() || !editorContent.trim()}>
      <Copy size={14} />
    </button>
    <button type="button" class="mini-icon-btn" on:click={closeModal} disabled={saving}>
      <X size={14} />
    </button>
  </svelte:fragment>

  <div class="compact-editor">
    <div class="pill-row file-row">
      {#each targetFiles as file (file.path)}
        <button type="button" class={`pill ${activeFilePath === file.path ? 'is-active' : ''}`} on:click={() => void selectFile(file.path)} disabled={saving}>
          {file.name}
        </button>
      {:else}
        <span class="meta-text">No config files detected</span>
      {/each}
    </div>

    <div class="pill-row model-row">
      {#each availableModels as model (model.id)}
        <button type="button" class={`pill model-pill ${activeModelID === model.id ? 'is-active' : ''}`} on:click={() => selectModel(model.id)}>
          {model.id}
        </button>
      {/each}
    </div>

    <button type="button" class="model-copy-card" on:click={() => void copyActiveModel()} disabled={!activeModelID || !hasClipboardWrite()}>
      <span class="copy-label">Copy Selected Model</span>
      <code>{activeModelID || '-'}</code>
      <span class="copy-hint">Click card to copy</span>
    </button>

    <div class="meta-row">
      <span class="meta-text">Base: {proxyBaseURL || '-'}</span>
      <span class="meta-text">Key: {proxyAPIKey || '-'}</span>
      <span class="meta-text">Current: {status?.currentModel || result?.model || '-'}</span>
    </div>

    <div class="editor-shell">
      <div class="path-bar">{activeFilePath || '-'}</div>
      <textarea
        class="editor"
        bind:value={editorContent}
        on:input={() => {
          dirty = true
        }}
        disabled={!activeFilePath || loading || saving}
        spellcheck="false"
        placeholder={activeFilePath ? 'Edit config content...' : 'No editable file available.'}
      ></textarea>
    </div>

    {#if error}
      <div class="state state-error">{error}</div>
    {:else if loading}
      <div class="state state-info">Loading file content...</div>
    {/if}
  </div>

  <svelte:fragment slot="footer">
    <Button variant="secondary" size="sm" on:click={saveFile} disabled={!dirty || !activeFilePath || loading || saving}>
      <Save size={13} class="mr-1" />
      {saving ? 'Saving...' : 'Save'}
    </Button>
    <Button variant="primary" size="sm" on:click={closeModal} disabled={saving}>Done</Button>
  </svelte:fragment>
</MacOSModal>

<style>
  .compact-editor {
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
    padding: 0.65rem 0.75rem 0.75rem;
  }

  .pill-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .pill {
    border: 1px solid rgba(148, 163, 184, 0.35);
    border-radius: 8px;
    background: #0f172a;
    color: #cbd5e1;
    padding: 0.3rem 0.62rem;
    font-size: 0.72rem;
    font-weight: 600;
    transition: border-color 0.15s ease, background-color 0.15s ease;
  }

  .pill:hover:not(:disabled) {
    border-color: rgba(96, 165, 250, 0.6);
  }

  .pill.is-active {
    border-color: rgba(59, 130, 246, 0.9);
    background: #2563eb;
    color: #eff6ff;
  }

  .model-pill {
    background: #111827;
  }

  .model-copy-card {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    border: 1px solid rgba(96, 165, 250, 0.32);
    border-radius: 10px;
    background: rgba(30, 41, 59, 0.74);
    color: #e2e8f0;
    padding: 0.45rem 0.6rem;
    text-align: left;
    transition: border-color 0.16s ease, background-color 0.16s ease;
  }

  .model-copy-card:hover:not(:disabled) {
    border-color: rgba(96, 165, 250, 0.7);
    background: rgba(30, 41, 59, 0.92);
  }

  .model-copy-card:disabled {
    opacity: 0.6;
  }

  .copy-label {
    font-size: 0.69rem;
    color: #93c5fd;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 700;
    white-space: nowrap;
  }

  .model-copy-card code {
    font-size: 0.75rem;
    color: #f8fafc;
    background: rgba(15, 23, 42, 0.82);
    border: 1px solid rgba(148, 163, 184, 0.2);
    border-radius: 7px;
    padding: 0.25rem 0.5rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }

  .copy-hint {
    margin-left: auto;
    font-size: 0.69rem;
    color: #94a3b8;
    white-space: nowrap;
  }

  .meta-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .meta-text {
    font-size: 0.69rem;
    color: #94a3b8;
  }

  .editor-shell {
    border: 1px solid rgba(148, 163, 184, 0.2);
    border-radius: 10px;
    overflow: hidden;
    background: #0b1220;
  }

  .path-bar {
    padding: 0.42rem 0.6rem;
    font-size: 0.68rem;
    color: #93a1b5;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
    background: #111b2e;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .editor {
    width: 100%;
    min-height: 13rem;
    resize: vertical;
    border: 0;
    outline: none;
    background: #0b1220;
    color: #e2e8f0;
    padding: 0.68rem 0.75rem;
    font-size: 0.76rem;
    line-height: 1.5;
    font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  }

  .editor::placeholder {
    color: #64748b;
  }

  .state {
    border-radius: 8px;
    padding: 0.45rem 0.58rem;
    font-size: 0.72rem;
  }

  .state-error {
    color: #fca5a5;
    background: rgba(127, 29, 29, 0.28);
    border: 1px solid rgba(248, 113, 113, 0.35);
  }

  .state-info {
    color: #93c5fd;
    background: rgba(30, 41, 59, 0.76);
    border: 1px solid rgba(96, 165, 250, 0.28);
  }

  .mini-icon-btn {
    width: 1.55rem;
    height: 1.55rem;
    border-radius: 7px;
    border: 1px solid rgba(15, 23, 42, 0.25);
    background: rgba(255, 255, 255, 0.4);
    color: rgba(15, 23, 42, 0.82);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: background-color 0.15s ease, border-color 0.15s ease;
  }

  .mini-icon-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.6);
    border-color: rgba(15, 23, 42, 0.35);
  }

  .mini-icon-btn:disabled {
    opacity: 0.55;
  }
</style>
