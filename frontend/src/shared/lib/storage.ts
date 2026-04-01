const hasStorage = (): boolean => {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

export const getStorageItem = (key: string): string | null => {
  if (!hasStorage()) {
    return null
  }

  return window.localStorage.getItem(key)
}

export const setStorageItem = (key: string, value: string): void => {
  if (!hasStorage()) {
    return
  }

  window.localStorage.setItem(key, value)
}

export const getBooleanStorageItem = (key: string, fallback: boolean): boolean => {
  const value = getStorageItem(key)
  if (value === null) {
    return fallback
  }

  return value === 'true'
}

export const setBooleanStorageItem = (key: string, value: boolean): void => {
  setStorageItem(key, String(value))
}
