export interface CopyState {
  copied: boolean
}

export const createCopyState = (): CopyState => ({
  copied: false
})

export const canCopyToClipboard = (): boolean => {
  return typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function'
}

export const copyTextToClipboard = async (text: string): Promise<boolean> => {
  if (!canCopyToClipboard()) {
    return false
  }

  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}
