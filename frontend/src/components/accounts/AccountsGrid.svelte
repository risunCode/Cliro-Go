<script lang="ts">
  import type { Account } from '@/services/wails-api'
  import AccountCard from '@/components/accounts/AccountCard.svelte'

export let accounts: Account[] = []
export let selectedIds: string[] = []
export let busyAccountIds: string[] = []
export let refreshingAccountID = ''
export let confirmRemoveAccountID = ''
export let actionAccountID = ''

  export let onToggleSelection: (accountID: string) => void
  export let onToggleAccount: (accountID: string, enabled: boolean) => Promise<void>
  export let onStartSync: (accountID: string) => void
  export let onInfo: (accountID: string) => void
  export let onRefreshWithQuota: (accountID: string) => Promise<void>
  export let onExport: (accountID: string) => Promise<void>
  export let onStartRemove: (accountID: string) => void
  export let onConfirmRemove: (accountID: string) => Promise<void>
  export let onCancelRemove: () => void

</script>

<div class="account-grid">
  {#each accounts as account (account.id)}
    <AccountCard
      {account}
      selected={selectedIds.includes(account.id)}
      busy={busyAccountIds.includes(account.id)}
      refreshing={refreshingAccountID === account.id}
      confirmingDelete={confirmRemoveAccountID === account.id}
      deleteInProgress={actionAccountID === account.id}
      {onToggleSelection}
      {onToggleAccount}
      {onStartSync}
      {onInfo}
      {onRefreshWithQuota}
      {onExport}
      {onStartRemove}
      {onConfirmRemove}
      {onCancelRemove}
    />
  {/each}
</div>
