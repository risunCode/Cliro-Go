import type { logger, main } from '../../wailsjs/go/models'

export type AppState = main.State & {
  startupWarnings?: Array<{
    code?: string
    filePath?: string
    backupPath?: string
    message?: string
  }>
}

export type LogEntry = logger.Entry

export interface UpdateInfo {
  currentVersion: string
  latestVersion: string
  releaseName: string
  releaseUrl: string
  publishedAt: string
  updateAvailable: boolean
}
