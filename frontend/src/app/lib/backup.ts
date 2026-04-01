import type { AppState } from '@/app/types'
import type { Account } from '@/features/accounts/types'

export interface BackupPayload {
  version: number
  exportedAt: string
  state: AppState | null
  accounts: Account[]
}

export interface RestoreProgress {
  step: string
  index: number
  total: number
}

const isRecord = (value: unknown): value is Record<string, unknown> => {
  return typeof value === 'object' && value !== null
}

export const parseBackupNumber = (value: unknown, fallback: number): number => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

export const validateBackupPayload = (value: unknown): BackupPayload => {
  if (!isRecord(value)) {
    throw new Error('Backup payload must be a JSON object.')
  }

  const version = Number(value.version)
  if (!Number.isFinite(version) || version <= 0) {
    throw new Error('Backup payload version is invalid.')
  }

  const rawState = value.state
  if (rawState !== null && rawState !== undefined && !isRecord(rawState)) {
    throw new Error('Backup payload state must be an object or null.')
  }

  if (!Array.isArray(value.accounts)) {
    throw new Error('Backup payload accounts must be an array.')
  }

  const exportedAt = typeof value.exportedAt === 'string' ? value.exportedAt : new Date().toISOString()

  return {
    version,
    exportedAt,
    state: rawState === null || rawState === undefined ? null : (rawState as AppState),
    accounts: value.accounts.filter((entry) => isRecord(entry)) as Account[]
  }
}

export const assertBackupPayloadRestorable = (payload: BackupPayload): void => {
  if (payload.state === null && payload.accounts.length === 0) {
    throw new Error('Backup payload has no restorable state or account records.')
  }
}
