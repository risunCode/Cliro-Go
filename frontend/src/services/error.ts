export const getErrorMessage = (error: unknown, fallback = 'Operation failed'): string => {
  if (typeof error === 'string' && error.trim().length > 0) {
    return error
  }

  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message
  }

  if (typeof error === 'object' && error !== null && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim().length > 0) {
      return message
    }
  }

  return fallback
}
