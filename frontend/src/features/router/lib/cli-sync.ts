import { Bot, Command, Cpu, Sparkles } from 'lucide-svelte'
import type { CliSyncAppID, LocalModelCatalogItem } from '@/features/router/types'

export interface CliSyncCard {
  id: CliSyncAppID
  label: string
  icon: typeof Sparkles
  toneClass: string
}

export const CLI_SYNC_CARDS: CliSyncCard[] = [
  { id: 'claude-code', label: 'Claude Code Config', icon: Sparkles, toneClass: 'text-violet-400' },
  { id: 'opencode-cli', label: 'OpenCode Config', icon: Bot, toneClass: 'text-amber-400' },
  { id: 'kilo-cli', label: 'Kilo CLI Config', icon: Command, toneClass: 'text-emerald-400' },
  { id: 'codex-ai', label: 'Codex AI Config', icon: Cpu, toneClass: 'text-sky-400' }
]

export const getCliSyncProviderLabel = (ownedBy: string): string => {
  return ownedBy === 'kiro' ? 'Kiro' : ownedBy === 'codex' ? 'Codex' : 'Other'
}

export const groupCliModels = (models: LocalModelCatalogItem[]): Array<{ label: string; models: LocalModelCatalogItem[] }> => {
  const labels = ['Kiro', 'Codex', 'Other']
  return labels
    .map((label) => ({
      label,
      models: models.filter((item) => getCliSyncProviderLabel(item.ownedBy) === label)
    }))
    .filter((group) => group.models.length > 0)
}

export const getDefaultCliModel = (targetID: CliSyncAppID, models: LocalModelCatalogItem[]): string => {
  if (targetID === 'claude-code') {
    return models.find((item) => item.ownedBy === 'kiro')?.id || models[0]?.id || ''
  }
  if (targetID === 'codex-ai') {
    return models.find((item) => item.ownedBy === 'codex')?.id || models[0]?.id || ''
  }
  return models[0]?.id || ''
}
