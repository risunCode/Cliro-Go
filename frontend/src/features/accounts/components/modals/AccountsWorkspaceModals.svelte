<script lang="ts">
  import ConnectPromptModal from '@/features/accounts/components/connect/ConnectPromptModal.svelte'
  import KiroConnectModal from '@/features/accounts/components/connect/KiroConnectModal.svelte'
  import AccountDetailModal from './AccountDetailModal.svelte'
  import AccountSyncModal from './AccountSyncModal.svelte'
  import BatchDeleteModal from './BatchDeleteModal.svelte'
  import { isPendingAuthSession } from '@/features/accounts/lib/workspace'
  import type {
    Account,
    AccountSyncResult,
    AuthSession,
    KiroAuthSession,
    SyncTargetID
  } from '@/features/accounts/types'

  export let showConnectPrompt = false
  export let showKiroConnectModal = false
  export let authSession: AuthSession | null = null
  export let kiroAuthSession: KiroAuthSession | null = null
  export let authWorking = false
  export let canCopyLink = false
  export let detailAccount: Account | null = null
  export let showSyncModal = false
  export let syncAccount: Account | null = null
  export let syncBusy = false
  export let syncError = ''
  export let syncResult: AccountSyncResult | null = null
  export let syncTargetID: SyncTargetID = 'kilo-cli'
  export let showBulkDeleteModal = false
  export let selectedCount = 0
  export let showBannedDeleteModal = false
  export let bannedCount = 0
  export let bulkBusy = false

  export let onOpenAuthLink: () => Promise<void>
  export let onCopyAuthLink: () => Promise<void>
  export let onSubmitCodexAuthCode: (event: CustomEvent<{ code: string }>) => Promise<void>
  export let onDismissModal: () => void
  export let onCancelFromModal: () => Promise<void>
  export let onStartKiroDeviceAuth: () => Promise<void>
  export let onStartKiroGoogleAuth: () => Promise<void>
  export let onStartKiroGithubAuth: () => Promise<void>
  export let onOpenKiroAuthLink: () => Promise<void>
  export let onCopyKiroAuthLink: () => Promise<void>
  export let onCopyKiroUserCode: () => Promise<void>
  export let onDismissKiroModal: () => void
  export let onCancelKiroModal: () => Promise<void>
  export let onCloseDetailModal: () => void
  export let onCloseSyncModal: () => void
  export let onConfirmSync: (event: CustomEvent<SyncTargetID>) => Promise<void>
  export let onCancelBulkDelete: () => void
  export let onConfirmBulkDelete: () => Promise<void>
  export let onCancelDeleteBanned: () => void
  export let onConfirmDeleteBanned: () => Promise<void>
</script>

<ConnectPromptModal
  open={showConnectPrompt}
  authUrl={authSession?.authUrl || ''}
  busy={authWorking}
  pending={isPendingAuthSession(authSession)}
  {canCopyLink}
  on:openLink={onOpenAuthLink}
  on:copyLink={onCopyAuthLink}
  on:submitCode={onSubmitCodexAuthCode}
  on:dismiss={onDismissModal}
  on:cancel={onCancelFromModal}
/>

<KiroConnectModal
  open={showKiroConnectModal}
  authUrl={kiroAuthSession?.authUrl || ''}
  userCode={kiroAuthSession?.userCode || ''}
  authMethod={kiroAuthSession?.authMethod || ''}
  busy={authWorking}
  pending={isPendingAuthSession(kiroAuthSession)}
  {canCopyLink}
  on:startDevice={onStartKiroDeviceAuth}
  on:startGoogle={onStartKiroGoogleAuth}
  on:startGithub={onStartKiroGithubAuth}
  on:openLink={onOpenKiroAuthLink}
  on:copyLink={onCopyKiroAuthLink}
  on:copyCode={onCopyKiroUserCode}
  on:dismiss={onDismissKiroModal}
  on:cancel={onCancelKiroModal}
/>

<AccountDetailModal open={Boolean(detailAccount)} account={detailAccount} on:dismiss={onCloseDetailModal} />

<AccountSyncModal
  open={showSyncModal}
  account={syncAccount}
  loading={syncBusy}
  error={syncError}
  result={syncResult}
  selectedTargetID={syncTargetID}
  on:close={onCloseSyncModal}
  on:confirm={onConfirmSync}
/>

<BatchDeleteModal
  open={showBulkDeleteModal}
  count={selectedCount}
  busy={bulkBusy}
  title="Delete Selected Accounts"
  description="This action will remove selected records from local storage."
  summaryLabel="selected account(s)"
  confirmLabel="Delete Selected"
  on:cancel={onCancelBulkDelete}
  on:confirm={onConfirmBulkDelete}
/>

<BatchDeleteModal
  open={showBannedDeleteModal}
  count={bannedCount}
  busy={bulkBusy}
  title="Delete Banned Accounts"
  description="This action removes all accounts explicitly marked as banned."
  summaryLabel="banned account(s)"
  confirmLabel="Delete Banned"
  on:cancel={onCancelDeleteBanned}
  on:confirm={onConfirmDeleteBanned}
/>
