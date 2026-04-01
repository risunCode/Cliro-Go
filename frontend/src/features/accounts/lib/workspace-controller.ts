import type { Account, AccountSyncResult, SyncTargetID } from '@/features/accounts/types'
import type { AccountsPreferences } from '@/features/accounts/lib/preferences'
import { parseImportedAccounts } from '@/features/accounts/lib/workspace'

export interface AccountsWorkspaceControllerState {
  selectedIds: string[]
  confirmRemoveAccountID: string
  refreshingAccountID: string
  actionAccountID: string
  showConnectPrompt: boolean
  showKiroConnectModal: boolean
  connectPromptSessionID: string
  kiroPromptSessionID: string
  connectPanelOpen: boolean
  detailAccount: Account | null
  syncAccountID: string
  syncTargetID: SyncTargetID
  syncBusy: boolean
  syncError: string
  syncResult: AccountSyncResult | null
  showSyncModal: boolean
  showBulkDeleteModal: boolean
  showBannedDeleteModal: boolean
  searchQuery: string
  selectedProvider: string
  showExhausted: boolean
  showDisabled: boolean
  view: AccountsPreferences['view']
  bulkBusy: boolean
}

export const createInitialWorkspaceState = (preferences: AccountsPreferences): AccountsWorkspaceControllerState => ({
  selectedIds: [],
  confirmRemoveAccountID: '',
  refreshingAccountID: '',
  actionAccountID: '',
  showConnectPrompt: false,
  showKiroConnectModal: false,
  connectPromptSessionID: '',
  kiroPromptSessionID: '',
  connectPanelOpen: false,
  detailAccount: null,
  syncAccountID: '',
  syncTargetID: 'kilo-cli',
  syncBusy: false,
  syncError: '',
  syncResult: null,
  showSyncModal: false,
  showBulkDeleteModal: false,
  showBannedDeleteModal: false,
  searchQuery: '',
  selectedProvider: 'all',
  showExhausted: preferences.showExhausted,
  showDisabled: preferences.showDisabled,
  view: preferences.view,
  bulkBusy: false,
})

export const isBannedAccount = (account: Account): boolean => {
  return Boolean(account.banned)
}

export const findAccountByID = (accounts: Account[], accountID: string): Account | null => {
  return accounts.find((account) => account.id === accountID) || null
}

export const getBannedAccountIDs = (accounts: Account[]): string[] => {
  return accounts.filter((account) => isBannedAccount(account)).map((account) => account.id)
}

const sanitizeFileSegment = (value: string | undefined, fallback: string): string => {
  const normalized = (value || '').trim().toLowerCase()
  if (!normalized) {
    return fallback
  }

  const sanitized = normalized
    .replace(/[@\s]+/g, '_')
    .replace(/[^a-z0-9._-]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^[_\-.]+|[_\-.]+$/g, '')

  return sanitized || fallback
}

export const buildAccountExportFileName = (account: Account): string => {
  const provider = sanitizeFileSegment(account.provider, 'provider')
  const identity = sanitizeFileSegment(account.email || account.id, 'account')
  return `cliro_${provider}_${identity}.json`
}

export const readImportedAccountsFile = async (file: File): Promise<Account[]> => {
  const text = await file.text()
  const parsed = JSON.parse(text)
  const accounts = parseImportedAccounts(parsed)

  if (accounts.length === 0) {
    throw new Error('No valid account records found in selected file.')
  }

  return accounts
}
