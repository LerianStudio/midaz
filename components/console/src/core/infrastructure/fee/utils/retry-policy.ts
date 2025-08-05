/**
 * Retry policy with exponential backoff
 * Handles transient failures gracefully
 */
export class RetryPolicy {
  constructor(
    private options: {
      maxRetries: number
      initialDelay: number
      maxDelay: number
      backoffMultiplier: number
      retryableErrors?: (error: any) => boolean
    }
  ) {
    this.options.retryableErrors = options.retryableErrors || this.isRetryable
  }

  async execute<T>(operation: () => Promise<T>): Promise<T> {
    let lastError: any
    let delay = this.options.initialDelay

    for (let attempt = 0; attempt <= this.options.maxRetries; attempt++) {
      try {
        return await operation()
      } catch (error) {
        lastError = error

        if (
          attempt === this.options.maxRetries ||
          !this.options.retryableErrors!(error)
        ) {
          throw error
        }

        // Wait before retrying
        await this.sleep(delay)

        // Calculate next delay with exponential backoff
        delay = Math.min(
          delay * this.options.backoffMultiplier,
          this.options.maxDelay
        )
      }
    }

    throw lastError
  }

  private isRetryable(error: any): boolean {
    // Don't retry client errors (4xx) except for 429 (rate limit)
    if (error.statusCode && error.statusCode >= 400 && error.statusCode < 500) {
      return error.statusCode === 429
    }

    // Retry network errors
    if (error.code === 'ECONNREFUSED' || error.code === 'ETIMEDOUT') {
      return true
    }

    // Retry server errors (5xx)
    if (error.statusCode && error.statusCode >= 500) {
      return true
    }

    // Default to not retrying
    return false
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms))
  }
}
