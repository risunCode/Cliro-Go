import { getErrorMessage } from '@/shared/lib/error'

export interface AsyncTaskState<TError = string> {
  busy: boolean
  error: TError | ''
  updatedAt?: number
}

export const createAsyncTaskState = <TError = string>(): AsyncTaskState<TError> => ({
  busy: false,
  error: ''
})

export const runAsyncTask = async <T>(
  setState: (state: AsyncTaskState) => void,
  action: () => Promise<T>,
  fallback = 'Operation failed.'
): Promise<T> => {
  setState({ busy: true, error: '' })

  try {
    const result = await action()
    setState({ busy: false, error: '', updatedAt: Date.now() })
    return result
  } catch (error) {
    setState({ busy: false, error: getErrorMessage(error, fallback), updatedAt: Date.now() })
    throw error
  }
}
