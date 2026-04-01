<script lang="ts">
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { Eye, EyeOff } from 'lucide-svelte'
  import BaseModal from '@/components/common/BaseModal.svelte'
  import Button from '@/components/common/Button.svelte'
  import ModalWindowHeader from '@/components/common/ModalWindowHeader.svelte'
  import CredentialField from './CredentialField.svelte'
  import StatusBadge from '@/components/common/StatusBadge.svelte'
  import type { Account } from '@/features/accounts/types'
  import { formatNumber, formatUnixSeconds } from '@/shared/lib/formatters'
  import { formatRelativeReset, getNearestFutureResetAt, getQuotaTone } from '@/features/accounts/lib/account-quota'
  import { copyTextToClipboard, hasClipboardWrite } from '@/shared/lib/browser'

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
  $: isBanned = Boolean(account && 'banned' in account && (account as Account & { banned?: boolean }).banned)

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
    if (!normalized || !hasClipboardWrite()) {
      return
    }

    const copied = await copyTextToClipboard(normalized)
    if (!copied) {
      return
    }

    copiedField = field
    scheduleCopyReset()
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
  $: nearestResetAt = getNearestFutureResetAt(account?.quota, account?.cooldownUntil)
  $: resetCountdown = formatRelativeReset(nearestResetAt) || '-'
  $: resetDateTime = formatUnixSeconds(nearestResetAt)
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
  <BaseModal
    open={true}
    overlayClass="items-end justify-center p-2 sm:items-center sm:p-3"
    cardClass="flex max-h-[min(82vh,40rem)] w-full max-w-3xl flex-col overflow-hidden"
    headerClass="border-b border-border px-3 py-2 sm:px-4 sm:py-3"
    bodyClass="overflow-y-auto px-3 py-2 text-xs text-text-secondary sm:px-4 sm:py-3"
    footerClass="flex items-center justify-end border-t border-border px-3 py-2 sm:px-4 sm:py-3"
    on:close={closeModal}
  >
    <svelte:fragment slot="header">
      <ModalWindowHeader
        title="Account Details"
        description={account.email || '-'}
        titleClassName="truncate text-sm font-semibold text-text-primary"
        descriptionClassName="mt-1 truncate text-[11px] text-text-secondary"
      />
    </svelte:fragment>

    <div class="grid gap-3 lg:grid-cols-2">
      <div class="grid auto-rows-min gap-2.5 sm:grid-cols-2">
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
          <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Reset In</p>
          <p class="font-mono font-medium text-text-primary">{resetCountdown}</p>
          <p class="mt-1 text-[11px] text-text-secondary">{resetDateTime}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2">
          <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Updated</p>
          <p class="font-medium text-text-primary">{formatUnixSeconds(account.updatedAt)}</p>
        </div>
        <div class="rounded-sm border border-border bg-app p-2 sm:col-span-2 flex min-h-[11rem] flex-col">
          <p class="mb-1 text-[11px] uppercase tracking-[0.06em] text-text-secondary">Quota Status & Last Error</p>
          <div class="mb-1 flex items-center gap-2">
            <StatusBadge tone={getQuotaTone(account.quota?.status)}>{account.quota?.status || 'unknown'}</StatusBadge>
            {#if isBanned}
              <StatusBadge tone="error">Banned</StatusBadge>
            {/if}
            <p class="text-xs text-text-secondary break-words">{account.quota?.summary || 'No quota summary available.'}</p>
          </div>
          <p class="break-words font-medium {account.lastError ? 'text-error' : 'text-text-primary'}">
            {account.lastError || 'No recent errors.'}
          </p>
        </div>
      </div>

      <div>
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
            <CredentialField
              label="Account ID"
              value={accountIDValue}
              displayValue={accountIDValue || '-'}
              copied={copiedField === 'accountId'}
              canCopy={hasClipboardWrite()}
              onCopy={() => copyField('accountId', accountIDValue)}
            />

            <CredentialField
              label="Access Token"
              value={accessTokenValue}
              displayValue={revealSecrets ? accessTokenValue || '-' : maskSecret(accessTokenValue)}
              copied={copiedField === 'accessToken'}
              canCopy={hasClipboardWrite()}
              onCopy={() => copyField('accessToken', accessTokenValue)}
            />

            <CredentialField
              label="Refresh Token"
              value={refreshTokenValue}
              displayValue={revealSecrets ? refreshTokenValue || '-' : maskSecret(refreshTokenValue)}
              copied={copiedField === 'refreshToken'}
              canCopy={hasClipboardWrite()}
              onCopy={() => copyField('refreshToken', refreshTokenValue)}
            />

            <CredentialField
              label="ID Token"
              value={idTokenValue}
              displayValue={revealSecrets ? idTokenValue || '-' : maskSecret(idTokenValue)}
              copied={copiedField === 'idToken'}
              canCopy={hasClipboardWrite()}
              onCopy={() => copyField('idToken', idTokenValue)}
            />
          </div>
        </div>
      </div>
    </div>

    <svelte:fragment slot="footer">
      <Button variant="secondary" size="sm" on:click={closeModal}>Close</Button>
    </svelte:fragment>
  </BaseModal>
{/if}
