import { createSafeActionClient } from 'next-safe-action'

export const authActionClient = createSafeActionClient({
  handleReturnedServerError(e) {
    if (e instanceof Error) {
      return e.message
    }
    return 'An unknown error occurred'
  }
})
