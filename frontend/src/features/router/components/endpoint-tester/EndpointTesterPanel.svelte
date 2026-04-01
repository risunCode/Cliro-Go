<script lang="ts">
  import { onDestroy } from 'svelte'
  import { ChevronDown, ChevronUp, Copy, Network, Play } from 'lucide-svelte'
  import Button from '@/components/common/Button.svelte'
  import CollapsibleSurfaceSection from '@/components/common/CollapsibleSurfaceSection.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import { routerApi } from '@/features/router/api/router-api'
  import type { ProxyStatus } from '@/features/router/types'
  import { copyTextToClipboard, hasClipboardWrite } from '@/shared/lib/browser'
  import { ENDPOINT_PRESETS, buildTesterStructuredResponse, getEndpointPreset, getEndpointRequestBody, type EndpointPreset, type TesterStructuredResponse } from '@/features/router/lib/endpoint-tester'

  export let proxyStatus: ProxyStatus | null = null
  export let apiKey = ''

  let expanded = false
  let selectedEndpointId = ENDPOINT_PRESETS[0].id
  let selectedEndpoint: EndpointPreset = ENDPOINT_PRESETS[0]
  let requestBody = getEndpointRequestBody(ENDPOINT_PRESETS[0].id)
  let loading = false
  let status = '-'
  let responseText = ''
  let error = ''
  let structuredResponse: TesterStructuredResponse | null = null
  let responseCopied = false
  let responseCopyTimer: ReturnType<typeof setTimeout> | null = null
  let showRawResponse = false
  let thinkingExpanded = false
  let runRequestID = 0

  $: structuredResponse = buildTesterStructuredResponse(responseText)
  $: selectedEndpoint = getEndpointPreset(selectedEndpointId)

  const applySelectedEndpoint = (event?: Event): void => {
    let nextEndpointID = selectedEndpointId
    if (event?.currentTarget instanceof HTMLSelectElement) {
      nextEndpointID = event.currentTarget.value
      selectedEndpointId = nextEndpointID
    }
    requestBody = getEndpointRequestBody(nextEndpointID)
  }

  const runEndpointTest = async (): Promise<void> => {
    if (!proxyStatus?.url) {
      error = 'Proxy URL is not available.'
      return
    }

    const requestID = ++runRequestID
    loading = true
    error = ''
    status = '-'
    responseText = ''
    showRawResponse = false
    thinkingExpanded = false

    try {
      const result = await routerApi.executeEndpointTest({
        baseUrl: proxyStatus.url,
        apiKey,
        endpointId: selectedEndpointId,
        body: requestBody
      })
      if (requestID !== runRequestID) {
        return
      }
      status = result.status
      responseText = result.responseText
    } catch (requestError) {
      if (requestID !== runRequestID) {
        return
      }
      error = requestError instanceof Error ? requestError.message : 'Request failed'
    } finally {
      if (requestID === runRequestID) {
        loading = false
      }
    }
  }

  const copyResponse = async (): Promise<void> => {
    if (!hasClipboardWrite() || !responseText.trim()) {
      return
    }

    const copied = await copyTextToClipboard(responseText)
    if (!copied) {
      return
    }

    responseCopied = true
    if (responseCopyTimer) {
      clearTimeout(responseCopyTimer)
    }
    responseCopyTimer = setTimeout(() => {
      responseCopied = false
      responseCopyTimer = null
    }, 1200)
  }

  const toggleRawResponse = (): void => {
    showRawResponse = !showRawResponse
  }

  const toggleThinking = (): void => {
    thinkingExpanded = !thinkingExpanded
  }

  onDestroy(() => {
    ++runRequestID
    if (responseCopyTimer) {
      clearTimeout(responseCopyTimer)
    }
  })
</script>

<CollapsibleSurfaceSection
  bind:open={expanded}
  icon={Network}
  title="Endpoint Tester"
  subtitle="Run OpenAI-compatible and Anthropic-compatible endpoint probes against your local proxy."
  pill="Proxy Integration"
  ariaLabel="Toggle endpoint tester"
  className="api-cli-sync api-endpoint-tester p-0"
  bodyClassName="api-cli-sync-body api-endpoint-body"
>
  <div class="api-endpoint-grid">
    <div class="api-endpoint-panel ui-panel-soft ui-panel-dashed">
      <div class="api-endpoint-controls ui-inline-controls">
        <select bind:value={selectedEndpointId} class="api-endpoint-select ui-control-input ui-control-select" on:change={applySelectedEndpoint}>
          {#each ENDPOINT_PRESETS as endpoint}
            <option value={endpoint.id}>{endpoint.label}</option>
          {/each}
        </select>
        <Button
          variant="primary"
          size="sm"
          className="api-endpoint-run whitespace-nowrap"
          on:click={runEndpointTest}
          disabled={loading || !proxyStatus?.running || !proxyStatus?.url}
        >
          <Play size={14} class="mr-1" />
          Execute
        </Button>
      </div>

      <div class="api-endpoint-request">
        <p class="api-endpoint-label">Request Payload</p>
        {#if selectedEndpoint.method === 'POST'}
          <textarea bind:value={requestBody} class="api-endpoint-textarea ui-control-input ui-control-textarea"></textarea>
        {:else}
          <p class="text-text-secondary">This endpoint is GET and does not require a request body.</p>
        {/if}
      </div>
    </div>

    <div class="api-endpoint-panel ui-panel-soft ui-panel-dashed">
      <div class="mb-2 flex items-start justify-between gap-2">
        <div>
          <p class="api-endpoint-label">Response</p>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="secondary" size="sm" on:click={toggleRawResponse} disabled={!responseText.trim()}>
            {showRawResponse ? 'Hide Raw' : 'Show Raw'}
          </Button>
          <Button variant="secondary" size="sm" on:click={copyResponse} disabled={!hasClipboardWrite() || !responseText.trim()}>
            <Copy size={13} class="mr-1" />
            {responseCopied ? 'Copied' : 'Copy'}
          </Button>
        </div>
      </div>
      <div class="mb-2 flex items-center gap-2">
        <StatusBadge tone={status.startsWith('2') ? 'success' : status === '-' ? 'neutral' : 'warning'}>{status}</StatusBadge>
        {#if loading}
          <span class="text-text-secondary">Request in progress...</span>
        {/if}
      </div>
      {#if error}
        <p class="text-error">{error}</p>
      {:else if responseText}
        {#if structuredResponse && !showRawResponse}
          <div class="mb-3 space-y-3 rounded-sm border border-border bg-app p-3">
            {#if structuredResponse.thinking}
              <div class="rounded-sm border border-border bg-surface">
                <button
                  type="button"
                  class="flex w-full items-center justify-between gap-2 px-3 py-2 text-left"
                  on:click={toggleThinking}
                >
                  <span>
                    <p class="text-[10px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Thinking</p>
                    <p class="text-[11px] text-text-secondary">Auto-collapsed after completion. Expand to inspect reasoning.</p>
                  </span>
                  <span class="text-text-secondary">
                    {#if thinkingExpanded}
                      <ChevronUp size={15} />
                    {:else}
                      <ChevronDown size={15} />
                    {/if}
                  </span>
                </button>
                {#if thinkingExpanded}
                  <pre class="api-endpoint-response no-scrollbar whitespace-pre-wrap border-t border-border bg-app p-3">{structuredResponse.thinking}</pre>
                {/if}
              </div>
            {/if}

            {#if structuredResponse.message}
              <div>
                <p class="mb-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-text-secondary">Response</p>
                <pre class="api-endpoint-response no-scrollbar whitespace-pre-wrap border border-border bg-surface p-3">{structuredResponse.message}</pre>
              </div>
            {/if}
          </div>
        {/if}
        {#if showRawResponse || !structuredResponse}
          <pre class="api-endpoint-response no-scrollbar">{responseText}</pre>
        {/if}
      {:else}
        <p class="text-text-secondary">Run a request to inspect proxy responses.</p>
      {/if}
    </div>
  </div>
</CollapsibleSurfaceSection>
