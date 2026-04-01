<script lang="ts">
  import type { AppActions, AppShellState, AccountsActions } from '@/app/services/app-controller'
  import AccountsWorkspace from '@/features/accounts/components/AccountsWorkspace.svelte'

  export let shell: AppShellState
  export let appActions: AppActions
  export let accountsActions: AccountsActions
</script>

<AccountsWorkspace
  accounts={shell.accounts}
  busyAccountIds={shell.busyAccountIds}
  authSession={shell.authSession}
  kiroAuthSession={shell.kiroAuthSession}
  authWorking={shell.authWorking}
  refreshingAllQuotas={shell.refreshingAllQuotas}
  onStartConnect={accountsActions.startConnect}
  onCancelConnect={accountsActions.cancelConnect}
  onStartKiroConnect={accountsActions.startKiroConnect}
  onCancelKiroConnect={accountsActions.cancelKiroConnect}
  onOpenExternalURL={appActions.openExternalURL}
  onRefreshAllQuotas={accountsActions.refreshAllQuotas}
  onForceRefreshAllQuotas={accountsActions.forceRefreshAllQuotas}
  onToggleAccount={accountsActions.toggleAccount}
  onBulkToggleAccounts={accountsActions.bulkToggleAccounts}
  onBulkDeleteAccounts={accountsActions.bulkDeleteAccounts}
  onImportAccounts={accountsActions.importAccounts}
  onSyncCodexAccountToKiloAuth={accountsActions.syncCodexAccountToKiloAuth}
  onSyncCodexAccountToOpencodeAuth={accountsActions.syncCodexAccountToOpencodeAuth}
  onSyncCodexAccountToCodexCLI={accountsActions.syncCodexAccountToCodexCLI}
  onRefreshAccountWithQuota={accountsActions.refreshAccountWithQuota}
  onDeleteAccount={accountsActions.deleteAccount}
/>
