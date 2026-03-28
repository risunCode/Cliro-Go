<script lang="ts">
  import type { Account } from '@/services/wails-api'
  import AccountRow from '@/components/accounts/AccountRow.svelte'

export let accounts: Account[] = []
export let selectedIds: string[] = []
export let allVisibleSelected = false
export let busyAccountIds: string[] = []
export let refreshingAccountID = ''
export let confirmRemoveAccountID = ''
export let actionAccountID = ''

  export let onToggleSelection: (accountID: string) => void
  export let onToggleSelectAllVisible: () => void
  export let onToggleAccount: (accountID: string, enabled: boolean) => Promise<void>
  export let onStartSync: (accountID: string) => void
  export let onInfo: (accountID: string) => void
  export let onRefreshWithQuota: (accountID: string) => Promise<void>
  export let onExport: (accountID: string) => Promise<void>
  export let onStartRemove: (accountID: string) => void
  export let onConfirmRemove: (accountID: string) => Promise<void>
  export let onCancelRemove: () => void

  const isBusy = (accountID: string): boolean => busyAccountIds.includes(accountID)
</script>

<div class="table-shell">
  <table class="accounts-table">
    <thead>
      <tr>
        <th class="col-check">
          <input class="table-checkbox" type="checkbox" checked={allVisibleSelected} on:change={onToggleSelectAllVisible} aria-label="Toggle all visible" />
        </th>
        <th>Account</th>
        <th>Provider</th>
        <th>Reset Info</th>
        <th>Status / Quota</th>
        <th class="col-actions">Actions</th>
      </tr>
    </thead>
    <tbody>
      {#each accounts as account (account.id)}
        <AccountRow
          {account}
          selected={selectedIds.includes(account.id)}
          busy={isBusy(account.id)}
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
    </tbody>
  </table>
</div>
