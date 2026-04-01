import { getBooleanStorageItem, getStorageItem, setBooleanStorageItem, setStorageItem } from '@/shared/lib/storage'

export type AccountsViewMode = 'card' | 'table'

export interface AccountsPreferences {
  showExhausted: boolean
  showDisabled: boolean
  view: AccountsViewMode
}

const SHOW_EXHAUSTED_STORAGE_KEY = 'accounts-show-exhausted'
const SHOW_DISABLED_STORAGE_KEY = 'accounts-show-disabled'
const VIEW_STORAGE_KEY = 'accounts-view'

export const defaultAccountsPreferences: AccountsPreferences = {
  showExhausted: true,
  showDisabled: true,
  view: 'card'
}

const parseViewMode = (value: string | null): AccountsViewMode => {
  return value === 'table' ? 'table' : 'card'
}

export const loadAccountsPreferences = (): AccountsPreferences => {
  return {
    showExhausted: getBooleanStorageItem(SHOW_EXHAUSTED_STORAGE_KEY, defaultAccountsPreferences.showExhausted),
    showDisabled: getBooleanStorageItem(SHOW_DISABLED_STORAGE_KEY, defaultAccountsPreferences.showDisabled),
    view: parseViewMode(getStorageItem(VIEW_STORAGE_KEY))
  }
}

export const saveAccountsPreferences = (preferences: AccountsPreferences): void => {
  setBooleanStorageItem(SHOW_EXHAUSTED_STORAGE_KEY, preferences.showExhausted)
  setBooleanStorageItem(SHOW_DISABLED_STORAGE_KEY, preferences.showDisabled)
  setStorageItem(VIEW_STORAGE_KEY, preferences.view)
}
