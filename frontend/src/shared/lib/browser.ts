export { canCopyToClipboard as hasClipboardWrite, copyTextToClipboard } from './copy'

const triggerDownload = (blob: Blob, fileName: string): void => {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')

  anchor.href = url
  anchor.download = fileName
  anchor.click()

  URL.revokeObjectURL(url)
}

export const downloadTextFile = (content: string, fileName: string, mimeType = 'text/plain;charset=utf-8'): void => {
  const blob = new Blob([content], { type: mimeType })
  triggerDownload(blob, fileName)
}

export const downloadJSONFile = (payload: unknown, fileName: string): void => {
  downloadTextFile(JSON.stringify(payload, null, 2), fileName, 'application/json')
}
