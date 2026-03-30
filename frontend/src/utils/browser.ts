export const hasClipboardWrite = (): boolean => {
  return typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function'
}

export const copyTextToClipboard = async (text: string): Promise<boolean> => {
  if (!hasClipboardWrite()) {
    return false
  }

  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

const triggerDownload = (blob: Blob, fileName: string): void => {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')

  anchor.href = url
  anchor.download = fileName
  anchor.click()

  URL.revokeObjectURL(url)
}

export const downloadTextFile = (content: string, fileName: string, mimeType = 'text/plain;charset=utf-8'): void => {
  triggerDownload(new Blob([content], { type: mimeType }), fileName)
}

export const downloadJSONFile = (payload: unknown, fileName: string): void => {
  downloadTextFile(JSON.stringify(payload, null, 2), fileName, 'application/json')
}
