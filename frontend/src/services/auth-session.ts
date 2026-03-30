export interface SessionLike {
  sessionId: string
  status: string
  error?: string
}

export interface AuthSessionControllerOptions<T extends SessionLike> {
  intervalMs?: number
  transientRetryLimit?: number
  getSession: (sessionId: string) => Promise<T>
  onSession: (session: T) => void
  onSuccess: (session: T) => Promise<void> | void
  onSessionError: (session: T) => void
  onPollingError: (error: unknown) => void
}

export interface AuthSessionController {
  start: (sessionId: string) => void
  stop: () => void
  poll: (sessionId: string) => Promise<void>
}

export const createAuthSessionController = <T extends SessionLike>(
  options: AuthSessionControllerOptions<T>
): AuthSessionController => {
  const intervalMs = options.intervalMs ?? 1500
  const transientRetryLimit = options.transientRetryLimit ?? 3

  let timer: number | null = null
  let inFlight = false
  let transientErrors = 0

  const stop = (): void => {
    if (timer !== null) {
      window.clearInterval(timer)
      timer = null
    }
    inFlight = false
    transientErrors = 0
  }

  const poll = async (sessionId: string): Promise<void> => {
    if (inFlight) {
      return
    }

    inFlight = true
    try {
      const nextSession = await options.getSession(sessionId)
      transientErrors = 0
      options.onSession(nextSession)

      if (nextSession.status === 'success') {
        stop()
        await options.onSuccess(nextSession)
      }

      if (nextSession.status === 'error') {
        stop()
        options.onSessionError(nextSession)
      }
    } catch (error) {
      transientErrors += 1
      if (transientErrors > transientRetryLimit) {
        stop()
        options.onPollingError(error)
      }
    } finally {
      inFlight = false
    }
  }

  const start = (sessionId: string): void => {
    stop()
    timer = window.setInterval(() => {
      void poll(sessionId)
    }, intervalMs)
  }

  return {
    start,
    stop,
    poll
  }
}
