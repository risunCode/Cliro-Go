<script lang="ts">
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { Check, Copy, Eye, EyeOff } from 'lucide-svelte'
  import ModalBackdrop from '@/components/common/ModalBackdrop.svelte'
  import Button from '@/components/common/Button.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import type { Account } from '@/services/wails-api'
  import { formatNumber, formatUnixSeconds, getQuotaTone } from '@/utils/formatters'
  import { formatBucketLabel, getPercentColor, metricPercent, nowUnixSeconds } from '@/utils/accounts/quota'

  export let open = false
  export let account: Account | null = null

  const dispatch = createEventDispatcher<{ dismiss: void }>()

  const closeModal = (): void => {
    dispatch('dismiss')
  }

  type CopyFieldKey = 'accountId' | 'accessToken' | 'refreshToken' | 'idToken'

  let copiedField: CopyFieldKey | '' = ''
  let revealSecrets = false
  let copyResetTimer: ReturnType<typeof setTimeout> | null = null

  const canUseClipboard = (): boolean => {
    return typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function'
  }

  const normalizeValue = (value?: string): string => {
    return (value ?? '').trim()
  }

  const maskSecret = (value?: string): string => {
    const normalized = normalizeValue(value)
    if (!normalized) {
      return '-'
    }
    if (normalized.length <= 16) {
      return normalized
    }
    return `${normalized.slice(0, 8)}...${normalized.slice(-8)}`
  }

  const scheduleCopyReset = (): void => {
    if (copyResetTimer) {
      clearTimeout(copyResetTimer)
    }
    copyResetTimer = setTimeout(() => {
      copiedField = ''
    }, 1400)
  }

  const copyField = async (field: CopyFieldKey, value?: string): Promise<void> => {
    const normalized = normalizeValue(value)
    if (!normalized || !canUseClipboard()) {
      return
    }
    try {
      await navigator.clipboard.writeText(normalized)
      copiedField = field
      scheduleCopyReset()
    } catch {
      // Ignore clipboard errors in desktop webview fallback cases.
    }
  }

  onDestroy(() => {
    if (copyResetTimer) {
      clearTimeout(copyResetTimer)
      copyResetTimer = null
    }
  })

  $: accountIDValue = normalizeValue(account?.accountId) || normalizeValue(account?.id)
  $: accessTokenValue = normalizeValue(account?.accessToken)
  $: refreshTokenValue = normalizeValue(account?.refreshToken)
  $: idTokenValue = normalizeValue(account?.idToken)
  $: if (!open) {
    copiedField = ''
    revealSecrets = false
  }

  const toLabelCase = (value: string): string => {
    if (!value) {
      return '-'
    }
    return value.charAt(0).toUpperCase() + value.slice(1)
  }
</script>

{#if open && account}
  <ModalBackdrop on:close={closeModal} />

  <div class="fixed inset-0 z-50 flex items-end justify-center p-2 sm:items-center sm:p-3">
    <div class="ui-surface-card flex max-h-[min(82vh,40rem)] w-full max-w-3xl flex-col overflow-hidden">
      <header class="border-b border-border px-3 py-2 sm:px-4 sm:py-3">
        <div class="mb-2 flex items-center gap-2">
          <div class="flex items-center gap-2">
            <span class="h-3 w-3 rounded-full bg-[#ef4444]" aria-hidden="true" />
            <span class="h-3 w-3 rounded-full bg-[#f59e0b]" aria-hidden="true" />
            <span class="h-3 w-3 rounded-full bg-[#22c55e]" aria-hidden="true" />
          </div>
        </div>
        <h2 class="truncate text-sm font-semibold text-text-primary">Account Details</h2>
        <p class="mt-1 truncate text-[11px] text-text-secondary">{account.email || account.id}</p>
      </header>

      <div class="overflow-y-auto px-3 py-2 text-xs text-text-secondary sm:px-4 sm:py-3">
        <div class="grid gap-3 lg:grid-cols-2">
          <div class="grid gap-2.5 sm:grid-cols-2">
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Email</p>
              <p class="break-all font-medium text-text-primary">{account.email || '-'}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Provider</p>
              <p class="font-medium text-text-primary">{toLabelCase((account.provider || 'codex').trim().toLowerCase())}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Plan</p>
              <p class="font-medium text-text-primary">{account.planType || '-'}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Requests</p>
              <p class="font-mono font-medium text-text-primary">{formatNumber(account.requestCount)}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Total Tokens</p>
              <p class="font-mono font-medium text-text-primary">{formatNumber(account.totalTokens)}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Last Used</p>
              <p class="font-medium text-text-primary">{formatUnixSeconds(account.lastUsed)}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Updated</p>
              <p class="font-medium text-text-primary">{formatUnixSeconds(account.updatedAt)}</p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2 sm:col-span-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Cooldown</p>
              <p class="font-medium {(account.cooldownUntil ?? 0) > nowUnixSeconds() ? 'text-warning' : 'text-text-primary'}">
                {(account.cooldownUntil ?? 0) > nowUnixSeconds() ? `Active until ${formatUnixSeconds(account.cooldownUntil)}` : 'Not active'}
              </p>
            </div>
            <div class="rounded-sm border border-border bg-app p-2 sm:col-span-2">
              <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Quota Status & Last Error</p>
              <div class="mb-1 flex items-center gap-2">
                <StatusBadge tone={getQuotaTone(account.quota?.status)}>{account.quota?.status || 'unknown'}</StatusBadge>
                <p class="truncate text-xs text-text-secondary">{account.quota?.summary || 'No quota summary available.'}</p>
              </div>
              <p class="break-words font-medium {account.lastError ? 'text-error' : 'text-text-primary'}">
                {account.lastError || 'No recent errors.'}
              </p>
            </div>
          </div>

          <div class="space-y-2">
            <div class="rounded-sm border border-border bg-app p-2">
              <div class="mb-2 flex items-center justify-between gap-2">
                <p class="text-[11px] uppercase tracking-[0.06em] text-text-secondary">Credentials</p>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-[11px]"
                  type="button"
                  on:click={() => {
                    revealSecrets = !revealSecrets
                  }}
                >
                  {#if revealSecrets}
                    <EyeOff size={12} class="mr-1" /> Hide
                  {:else}
                    <Eye size={12} class="mr-1" /> Show
                  {/if}
                </Button>
              </div>

              <div class="space-y-2">
                <div class="rounded-sm border border-border bg-surface p-2">
                  <div class="mb-1 flex items-center justify-between gap-2">
                    <p class="text-[10px] uppercase tracking-[0.06em] text-text-secondary">Account ID</p>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="ui-icon-btn"
                      type="button"
                      disabled={!accountIDValue || !canUseClipboard()}
                      on:click={() => copyField('accountId', accountIDValue)}
                    >
                      {#if copiedField === 'accountId'}
                        <Check size={12} />
                      {:else}
                        <Copy size={12} />
                      {/if}
                    </Button>
                  </div>
                  <p class="break-all font-mono text-[11px] text-text-primary">{accountIDValue || '-'}</p>
                </div>

                <div class="rounded-sm border border-border bg-surface p-2">
                  <div class="mb-1 flex items-center justify-between gap-2">
                    <p class="text-[10px] uppercase tracking-[0.06em] text-text-secondary">Access Token</p>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="ui-icon-btn"
                      type="button"
                      disabled={!accessTokenValue || !canUseClipboard()}
                      on:click={() => copyField('accessToken', accessTokenValue)}
                    >
                      {#if copiedField === 'accessToken'}
                        <Check size={12} />
                      {:else}
                        <Copy size={12} />
                      {/if}
                    </Button>
                  </div>
                  <p class="break-all font-mono text-[11px] text-text-primary">{revealSecrets ? accessTokenValue || '-' : maskSecret(accessTokenValue)}</p>
                </div>

                <div class="rounded-sm border border-border bg-surface p-2">
                  <div class="mb-1 flex items-center justify-between gap-2">
                    <p class="text-[10px] uppercase tracking-[0.06em] text-text-secondary">Refresh Token</p>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="ui-icon-btn"
                      type="button"
                      disabled={!refreshTokenValue || !canUseClipboard()}
                      on:click={() => copyField('refreshToken', refreshTokenValue)}
                    >
                      {#if copiedField === 'refreshToken'}
                        <Check size={12} />
                      {:else}
                        <Copy size={12} />
                      {/if}
                    </Button>
                  </div>
                  <p class="break-all font-mono text-[11px] text-text-primary">{revealSecrets ? refreshTokenValue || '-' : maskSecret(refreshTokenValue)}</p>
                </div>

                <div class="rounded-sm border border-border bg-surface p-2">
                  <div class="mb-1 flex items-center justify-between gap-2">
                    <p class="text-[10px] uppercase tracking-[0.06em] text-text-secondary">ID Token</p>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="ui-icon-btn"
                      type="button"
                      disabled={!idTokenValue || !canUseClipboard()}
                      on:click={() => copyField('idToken', idTokenValue)}
                    >
                      {#if copiedField === 'idToken'}
                        <Check size={12} />
                      {:else}
                        <Copy size={12} />
                      {/if}
                    </Button>
                  </div>
                  <p class="break-all font-mono text-[11px] text-text-primary">{revealSecrets ? idTokenValue || '-' : maskSecret(idTokenValue)}</p>
                </div>
              </div>
            </div>

            <div class="rounded-sm border border-border bg-app p-2">
              <p class="mb-2 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Quota Buckets</p>
              <div class="space-y-2">
                {#each account.quota?.buckets ?? [] as bucket}
                  {@const percent = metricPercent(bucket)}
                  {@const tone = getPercentColor(percent)}
                  <div class="space-y-1">
                    <div class="flex items-center justify-between gap-2 text-xs">
                      <p class="truncate text-text-primary">{formatBucketLabel(bucket.name)}</p>
                      <p
                        class={`font-mono text-[11px] ${tone === 'success'
                          ? 'text-[#4ade80]'
                          : tone === 'warning'
                            ? 'text-[#fbbf24]'
                            : 'text-[#f87171]'}`}
                      >
                        {percent.toFixed(0)}%
                      </p>
                    </div>
                    <div class="h-1 overflow-hidden rounded-full bg-surface">
                      <div
                        class={`h-full rounded-full transition-all ${tone === 'success'
                          ? 'bg-[#34d399]'
                          : tone === 'warning'
                            ? 'bg-[#fbbf24]'
                            : 'bg-[#f87171]'}`}
                        style={`width: ${percent.toFixed(0)}%`}
                      ></div>
                    </div>
                    <p class="font-mono text-[10px] text-text-secondary">
                      {formatNumber(bucket.used ?? 0)} used of {formatNumber(bucket.total ?? 0)}
                    </p>
                  </div>
                {:else}
                  <p class="text-xs text-text-secondary">No quota bucket data available.</p>
                {/each}
              </div>
            </div>
          </div>
        </div>
      </div>

      <footer class="flex items-center justify-end border-t border-border px-3 py-2 sm:px-4 sm:py-3">
        <Button variant="secondary" size="sm" on:click={closeModal}>Close</Button>
      </footer>
    </div>
  </div>
{/if}
