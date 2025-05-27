import { createSafeActionClient } from 'next-safe-action'

export const authActionClient = createSafeActionClient({
  handleReturnedServerError(e: unknown) {
    if (e instanceof Error) {
      return e.message
    }
    return 'An unknown error occurred'
  }
})
