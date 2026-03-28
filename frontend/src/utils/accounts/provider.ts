import type { Account } from '@/services/wails-api'

export interface ProviderMeta {
  label: string
  marker: string
  tint: string
}

export interface ProviderGroup {
  id: string
  name: string
  accounts: Account[]
}

const providerColors: Record<string, string> = {
  codex: '#667eea',
  kiro: '#f5576c',
  anthropic: '#fa709a',
  openai: '#30cfd0',
  copilot: '#22c55e'
}

const providerLabels: Record<string, string> = {
  codex: 'Codex',
  kiro: 'Kiro',
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  copilot: 'Copilot'
}

export const normalizeProviderID = (provider: string): string => {
  return (provider || 'codex').trim().toLowerCase()
}

export const providerMeta = (providerId: string): ProviderMeta => {
  return {
    label: providerLabels[providerId] || providerId.charAt(0).toUpperCase() + providerId.slice(1),
    marker: providerId.substring(0, 2).toUpperCase(),
    tint: providerColors[providerId] || '#667eea'
  }
}

export const groupAccountsByProvider = (accounts: Account[]): ProviderGroup[] => {
  const providerMap = new Map<string, ProviderGroup>()

  accounts.forEach((account) => {
    const id = normalizeProviderID(account.provider)
    if (!providerMap.has(id)) {
      providerMap.set(id, {
        id,
        name: providerMeta(id).label,
        accounts: []
      })
    }
    providerMap.get(id)?.accounts.push(account)
  })

  return Array.from(providerMap.values()).filter((group) => group.accounts.length > 0)
}
