<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { Copy, Save } from 'lucide-svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'
  import ModalWindowHeader from '@/components/common/ModalWindowHeader.svelte'
  import type { CliSyncAppID, CliSyncResult, CliSyncStatus, LocalModelCatalogItem } from '@/features/router/types'
  import { copyTextToClipboard, hasClipboardWrite } from '@/shared/lib/browser'

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
  let loadRequestID = 0

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

    const requestID = ++loadRequestID
    loading = true
    error = ''

    try {
      const nextContent = await onLoadFileContent(appID, activeFilePath)
      if (requestID !== loadRequestID) {
        return
      }
      editorContent = nextContent
      dirty = false
    } catch (loadError) {
      if (requestID !== loadRequestID) {
        return
      }
      const message = loadError instanceof Error ? loadError.message : 'Failed to load file content.'
      error = message
      dispatch('error', message)
      editorContent = ''
      dirty = false
    } finally {
      if (requestID === loadRequestID) {
        loading = false
      }
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
    ++loadRequestID
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

<BaseModal
  {open}
  overlayClass="items-end justify-center p-2 sm:items-center sm:p-4"
  cardClass="flex max-h-[min(82vh,42rem)] w-full max-w-4xl flex-col overflow-hidden"
  headerClass="border-b border-border px-4 py-3 sm:px-5 sm:py-4"
  bodyClass="overflow-y-auto px-4 py-3 sm:px-5 sm:py-4"
  footerClass="flex flex-wrap items-center justify-end gap-2 border-t border-border px-4 py-3 sm:px-5 sm:py-4"
  on:close={closeModal}
>
  <svelte:fragment slot="header">
    <ModalWindowHeader
      title={modalTitle}
      description="Quick edit for detected CLI config files."
    />
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

    <div class="model-toolbar">
      <label class="model-toolbar-field">
        <span class="meta-text">Model</span>
        <select
          class="ui-control-input ui-control-select-sm model-select"
          bind:value={activeModelID}
          on:change={() => selectModel(activeModelID)}
        >
          {#if availableModels.length === 0}
            <option value="">No models available</option>
          {:else}
            {#each availableModels as model (model.id)}
              <option value={model.id}>{model.id}</option>
            {/each}
          {/if}
        </select>
      </label>

      <Button
        variant="secondary"
        size="sm"
        on:click={() => void copyActiveModel()}
        disabled={!activeModelID || !hasClipboardWrite()}
      >
        <Copy size={13} class="mr-1" />
        Copy Model
      </Button>
    </div>

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
    <Button variant="secondary" size="sm" on:click={() => void copyEditorContent()} disabled={!hasClipboardWrite() || !editorContent.trim() || saving}>
      <Copy size={13} class="mr-1" />
      Copy Content
    </Button>
    <Button variant="ghost" size="sm" on:click={closeModal} disabled={saving}>Done</Button>
    <Button variant="primary" size="sm" on:click={saveFile} disabled={!dirty || !activeFilePath || loading || saving}>
      <Save size={13} class="mr-1" />
      {saving ? 'Saving...' : 'Save'}
    </Button>
  </svelte:fragment>
</BaseModal>

<style>
  .compact-editor {
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
  }

  .pill-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .pill {
    border: 1px solid color-mix(in srgb, var(--color-border) 92%, transparent);
    border-radius: 8px;
    background: color-mix(in srgb, var(--color-surface) 82%, var(--bg-secondary));
    color: var(--color-text-secondary);
    padding: 0.35rem 0.7rem;
    font-size: 0.72rem;
    font-weight: 600;
    transition: border-color 0.15s ease, background-color 0.15s ease, color 0.15s ease;
  }

  .pill:hover:not(:disabled) {
    border-color: color-mix(in srgb, var(--color-text-primary) 28%, var(--color-border));
    color: var(--color-text-primary);
  }

  .pill.is-active {
    border-color: color-mix(in srgb, var(--color-text-primary) 34%, var(--color-border));
    background: color-mix(in srgb, var(--color-text-primary) 10%, var(--color-surface));
    color: var(--color-text-primary);
  }

  .model-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .model-toolbar-field {
    display: grid;
    gap: 0.3rem;
    min-width: 14rem;
    flex: 1;
  }

  .model-select {
    min-height: 1.95rem;
    font-size: 0.74rem;
  }

  .meta-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem 0.75rem;
  }

  .meta-text {
    font-size: 0.72rem;
    color: var(--color-text-secondary);
  }

  .editor-shell {
    overflow: hidden;
    border: 1px solid color-mix(in srgb, var(--color-border) 88%, transparent);
    border-radius: 10px;
    background: color-mix(in srgb, var(--bg-secondary) 86%, var(--color-surface));
  }

  .path-bar {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 88%, transparent);
    background: color-mix(in srgb, var(--bg-secondary) 94%, var(--color-surface));
    color: var(--color-text-secondary);
    padding: 0.45rem 0.7rem;
    font-size: 0.7rem;
  }

  .editor {
    width: 100%;
    min-height: 12rem;
    resize: vertical;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--color-text-primary);
    padding: 0.75rem;
    font-size: 0.78rem;
    line-height: 1.55;
    font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  }

  .editor::placeholder {
    color: var(--color-text-secondary);
  }

  .state {
    border-radius: 8px;
    padding: 0.5rem 0.65rem;
    font-size: 0.74rem;
  }

  .state-error {
    color: var(--color-error);
    background: color-mix(in srgb, var(--color-error) 10%, transparent);
    border: 1px solid color-mix(in srgb, var(--color-error) 26%, var(--color-border));
  }

  .state-info {
    color: var(--color-info);
    background: color-mix(in srgb, var(--color-info) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--color-info) 22%, var(--color-border));
  }

  @media (max-width: 640px) {
    .model-toolbar {
      align-items: stretch;
    }

    .model-toolbar :global(button) {
      width: 100%;
      justify-content: center;
    }
  }
</style>
