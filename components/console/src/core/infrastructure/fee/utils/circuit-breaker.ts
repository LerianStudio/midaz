/**
 * Circuit Breaker pattern implementation
 * Prevents cascading failures by failing fast when a service is down
 */
export class CircuitBreaker {
  private failureCount: number = 0
  private lastFailureTime: number = 0
  private state: 'CLOSED' | 'OPEN' | 'HALF_OPEN' = 'CLOSED'
  private successCount: number = 0
  private lastStateChange: number = Date.now()

  constructor(
    private options: {
      failureThreshold: number
      recoveryTimeout: number
      monitoringPeriod: number
      successThreshold?: number
    }
  ) {
    this.options.successThreshold = options.successThreshold || 3
  }

  async execute<T>(operation: () => Promise<T>): Promise<T> {
    if (this.state === 'OPEN') {
      if (Date.now() - this.lastStateChange > this.options.recoveryTimeout) {
        this.state = 'HALF_OPEN'
        this.successCount = 0
      } else {
        throw new Error('Circuit breaker is OPEN')
      }
    }

    try {
      const result = await operation()
      this.onSuccess()
      return result
    } catch (error) {
      this.onFailure()
      throw error
    }
  }

  private onSuccess(): void {
    this.failureCount = 0

    if (this.state === 'HALF_OPEN') {
      this.successCount++
      if (this.successCount >= this.options.successThreshold!) {
        this.state = 'CLOSED'
        this.lastStateChange = Date.now()
      }
    }
  }

  private onFailure(): void {
    this.failureCount++
    this.lastFailureTime = Date.now()

    if (this.state === 'HALF_OPEN') {
      this.state = 'OPEN'
      this.lastStateChange = Date.now()
      return
    }

    if (
      this.failureCount >= this.options.failureThreshold &&
      Date.now() - this.lastFailureTime < this.options.monitoringPeriod
    ) {
      this.state = 'OPEN'
      this.lastStateChange = Date.now()
    }
  }

  isOpen(): boolean {
    return this.state === 'OPEN'
  }

  getState(): string {
    return this.state
  }

  reset(): void {
    this.failureCount = 0
    this.successCount = 0
    this.state = 'CLOSED'
    this.lastStateChange = Date.now()
  }
}
