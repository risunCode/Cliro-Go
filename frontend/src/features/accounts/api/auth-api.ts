import type { codex, kiro } from '../../../../wailsjs/go/models'
import { CancelCodexAuth, CancelKiroAuth, GetCodexAuthSession, GetKiroAuthSession, StartCodexAuth, StartKiroAuth, StartKiroSocialAuth, SubmitCodexAuthCode, SubmitKiroAuthCode } from '@/shared/api/wails/client'
import type { AuthSession, KiroAuthSession } from '@/features/accounts/types'

export const accountsAuthApi = {
  startCodexAuth: (): Promise<codex.AuthStart> => StartCodexAuth(),
  getCodexAuthSession: (sessionId: string): Promise<AuthSession> => GetCodexAuthSession(sessionId),
  cancelCodexAuth: (sessionId: string): Promise<void> => CancelCodexAuth(sessionId),
  submitCodexAuthCode: (sessionId: string, code: string): Promise<void> => SubmitCodexAuthCode(sessionId, code),
  startKiroAuth: (): Promise<kiro.AuthStart> => StartKiroAuth(),
  startKiroSocialAuth: (provider: string): Promise<kiro.AuthStart> => StartKiroSocialAuth(provider),
  getKiroAuthSession: (sessionId: string): Promise<KiroAuthSession> => GetKiroAuthSession(sessionId),
  cancelKiroAuth: (sessionId: string): Promise<void> => CancelKiroAuth(sessionId),
  submitKiroAuthCode: (sessionId: string, code: string): Promise<void> => SubmitKiroAuthCode(sessionId, code)
}
