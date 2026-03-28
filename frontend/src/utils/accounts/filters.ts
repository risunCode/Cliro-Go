import type { Account } from '@/services/wails-api'
import { normalizeProviderID } from '@/utils/accounts/provider'

export interface AccountFilterInput {
  providerId: string
  query: string
}

export const filterAccounts = (accounts: Account[], filters: AccountFilterInput): Account[] => {
  const normalizedQuery = filters.query.trim().toLowerCase()

  return accounts.filter((account) => {
    const matchesProvider = filters.providerId === 'all' || normalizeProviderID(account.provider) === filters.providerId
    const matchesSearch =
      normalizedQuery.length === 0 ||
      (account.email && account.email.toLowerCase().includes(normalizedQuery)) ||
      (account.id && account.id.toLowerCase().includes(normalizedQuery))

    return matchesProvider && matchesSearch
  })
}
