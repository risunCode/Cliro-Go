import type { codex, config } from '../../../wailsjs/go/models'

export type Account = config.Account & {
  banned?: boolean
  bannedReason?: string
}

export type AuthSession = codex.AuthSessionView

export interface KiroAuthSession {
  sessionId: string
  authUrl: string
  status: string
  error?: string
  accountId?: string
  email?: string
  verificationUrl?: string
  userCode?: string
  expiresAt?: number
  authMethod?: string
  provider?: string
}

export type SyncTargetID = 'kilo-cli' | 'opencode-cli' | 'codex-cli'

export interface SyncResultBase {
  targetPath: string
  fileExisted: boolean
  updatedFields: string[]
  accountID: string
  provider: string
}

export interface KiloAuthSyncResult extends SyncResultBase {
  target: 'kilo-cli'
  openAICreated: boolean
  syncedExpires: number
  syncedExpiresAt?: string
}

export interface OpencodeAuthSyncResult extends SyncResultBase {
  target: 'opencode-cli'
  openAICreated: boolean
  syncedExpires: number
  syncedExpiresAt?: string
}

export interface CodexAuthSyncResult extends SyncResultBase {
  target: 'codex-cli'
  backupPath?: string
  backupCreated: boolean
  syncedAt: string
}

export type AccountSyncResult = KiloAuthSyncResult | OpencodeAuthSyncResult | CodexAuthSyncResult
