import { writable } from 'svelte/store'

export type Theme = 'light' | 'dark' | 'solarized'

const THEME_STORAGE_KEY = 'app-theme'
const themeOrder: Theme[] = ['light', 'dark', 'solarized']

const isTheme = (value: string | null): value is Theme =>
  value === 'light' || value === 'dark' || value === 'solarized'

const getInitialTheme = (): Theme => {
  if (typeof window === 'undefined') {
    return 'light'
  }

  const saved = window.localStorage.getItem(THEME_STORAGE_KEY)
  if (isTheme(saved)) {
    return saved
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export const theme = writable<Theme>(getInitialTheme())

if (typeof window !== 'undefined') {
  theme.subscribe((value) => {
    document.documentElement.dataset.theme = value
    window.localStorage.setItem(THEME_STORAGE_KEY, value)
  })
}

export const toggleTheme = (): void => {
  theme.update((current) => {
    const currentIndex = themeOrder.indexOf(current)
    const nextIndex = currentIndex === -1 ? 0 : (currentIndex + 1) % themeOrder.length
    return themeOrder[nextIndex]
  })
}
