import type {
  Account,
  AccountSyncResult,
  CodexAuthSyncResult,
  KiloAuthSyncResult,
  OpencodeAuthSyncResult,
  SyncTargetID
} from '@/services/wails-api'
import { areAllVisibleSelected, filterAccounts, groupAccountsByProvider, type ProviderGroup } from '@/utils/account'
import { deriveQuotaDisplayStatus } from '@/utils/account-quota'

interface AccountSyncTarget {
  id: SyncTargetID
  name: string
  path: string
  description: string
}

export const ACCOUNT_SYNC_TARGETS: readonly AccountSyncTarget[] = [
  {
    id: 'kilo-cli',
    name: 'Kilo CLI',
    path: '~/.local/share/kilo/auth.json',
    description: 'Sync this Codex account into the Kilo CLI auth file.'
  },
  {
    id: 'opencode-cli',
    name: 'Opencode',
    path: '~/.local/share/opencode/auth.json',
    description: 'Sync this Codex account into the Opencode auth file.'
  },
  {
    id: 'codex-cli',
    name: 'Codex CLI',
    path: '~/.codex/auth.json',
    description: 'Sync this Codex account into the Codex CLI auth file.'
  }
]

export const parseImportedAccounts = (raw: unknown): Account[] => {
  if (Array.isArray(raw)) {
    return raw.filter((item) => item && typeof item === 'object') as Account[]
  }

  if (raw && typeof raw === 'object') {
    const payload = raw as Record<string, unknown>
    if (Array.isArray(payload.accounts)) {
      return payload.accounts.filter((item) => item && typeof item === 'object') as Account[]
    }
    return [raw as Account]
  }

  return []
}

interface SessionLike {
  sessionId?: string
  status?: string
}

export const isPendingAuthSession = (session: SessionLike | null): boolean => {
  return (session?.status ?? '') === 'pending'
}

export const shouldAttachPendingSession = (
  showPrompt: boolean,
  promptSessionID: string,
  session: SessionLike | null
): boolean => {
  return showPrompt && promptSessionID === '' && isPendingAuthSession(session) && Boolean(session?.sessionId)
}

export const shouldDismissPromptAfterSuccess = (
  showPrompt: boolean,
  promptSessionID: string,
  session: SessionLike | null
): boolean => {
  return showPrompt && promptSessionID !== '' && session?.sessionId === promptSessionID && session?.status === 'success'
}

interface AccountSyncHandlers {
  toKilo: (accountId: string) => Promise<KiloAuthSyncResult>
  toOpencode: (accountId: string) => Promise<OpencodeAuthSyncResult>
  toCodex: (accountId: string) => Promise<CodexAuthSyncResult>
}

export const syncTargetName = (target: SyncTargetID): string => {
  return ACCOUNT_SYNC_TARGETS.find((item) => item.id === target)?.name || 'Kilo CLI'
}

export const runAccountSyncByTarget = async (
  accountId: string,
  target: SyncTargetID,
  handlers: AccountSyncHandlers
): Promise<AccountSyncResult> => {
  if (target === 'codex-cli') {
    return handlers.toCodex(accountId)
  }

  if (target === 'opencode-cli') {
    return handlers.toOpencode(accountId)
  }

  return handlers.toKilo(accountId)
}

interface AccountsViewState {
  accountsByProvider: ProviderGroup[]
  filteredAccounts: Account[]
  visibleAccountIds: string[]
  hasVisibleAccounts: boolean
  allVisibleSelected: boolean
  selectedAccounts: Account[]
  selectedEnabledCount: number
  bulkToggleToEnabled: boolean
  exhaustedDisabledCount: number
}

interface AccountsVisibilityFilters {
  showExhausted: boolean
  showDisabled: boolean
}

const defaultVisibilityFilters: AccountsVisibilityFilters = {
  showExhausted: true,
  showDisabled: true
}

const isExhaustedAccount = (account: Account): boolean => {
  return deriveQuotaDisplayStatus(account.quota) === 'exhausted'
}

const isDisabledAccount = (account: Account): boolean => {
  return !account.enabled
}

export const computeAccountsViewState = (
  accounts: Account[],
  selectedIds: string[],
  selectedProvider: string,
  searchQuery: string,
  visibilityFilters: AccountsVisibilityFilters = defaultVisibilityFilters
): AccountsViewState => {
  const accountsByProvider = groupAccountsByProvider(accounts)
  const providerAndSearchFilteredAccounts = filterAccounts(accounts, {
    providerId: selectedProvider,
    query: searchQuery
  })

  const filteredAccounts = providerAndSearchFilteredAccounts.filter((account) => {
    if (!visibilityFilters.showDisabled && isDisabledAccount(account)) {
      return false
    }

    if (!visibilityFilters.showExhausted && isExhaustedAccount(account)) {
      return false
    }

    return true
  })

  const exhaustedDisabledCount = providerAndSearchFilteredAccounts.filter((account) => {
    return isDisabledAccount(account) || isExhaustedAccount(account)
  }).length

  const visibleAccountIds = filteredAccounts.map((account) => account.id)
  const hasVisibleAccounts = visibleAccountIds.length > 0
  const allVisibleSelected = areAllVisibleSelected(selectedIds, visibleAccountIds)

  const selectedSet = new Set(selectedIds)
  const selectedAccounts = accounts.filter((account) => selectedSet.has(account.id))
  const selectedEnabledCount = selectedAccounts.filter((account) => account.enabled).length

  return {
    accountsByProvider,
    filteredAccounts,
    visibleAccountIds,
    hasVisibleAccounts,
    allVisibleSelected,
    selectedAccounts,
    selectedEnabledCount,
    bulkToggleToEnabled: selectedIds.length > 0 && selectedEnabledCount !== selectedIds.length,
    exhaustedDisabledCount
  }
}

export const sanitizeSelectedIDs = (selectedIds: string[], accounts: Account[]): string[] => {
  const validAccountIDs = new Set(accounts.map((account) => account.id))
  return selectedIds.filter((id) => validAccountIDs.has(id))
}
