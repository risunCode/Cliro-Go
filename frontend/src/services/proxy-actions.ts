export type ProxyAction = () => Promise<void>

export interface ProxyActionRunnerOptions {
  setBusy: (busy: boolean) => void
  refresh: () => Promise<void>
  notifySuccess: (title: string, message: string) => void
  notifyError: (title: string, error: unknown) => void
}

export type RunProxyAction = (title: string, action: ProxyAction, doneMessage: string) => Promise<void>

export const createProxyActionRunner = (options: ProxyActionRunnerOptions): RunProxyAction => {
  return async (title, action, doneMessage) => {
    options.setBusy(true)
    try {
      await action()
      await options.refresh()
      options.notifySuccess(title, doneMessage)
    } catch (error) {
      options.notifyError(title, error)
    } finally {
      options.setBusy(false)
    }
  }
}
