/**
 * Circuit breaker pattern implementation for handling cascading failures
 */

export enum CircuitState {
  CLOSED = 'CLOSED',
  OPEN = 'OPEN',
  HALF_OPEN = 'HALF_OPEN',
}

export interface CircuitBreakerOptions {
  /** Number of failures before opening the circuit */
  failureThreshold: number;
  /** Time in milliseconds to wait before attempting recovery */
  recoveryTimeout: number;
  /** Time window in milliseconds for failure counting */
  monitoringPeriod: number;
  /** Minimum number of requests in monitoring period before evaluating */
  minimumRequests: number;
  /** Success rate threshold (0-1) for closing circuit in half-open state */
  successThreshold: number;
}

export interface CircuitBreakerStats {
  state: CircuitState;
  failures: number;
  successes: number;
  requests: number;
  lastFailureTime?: Date;
  lastSuccessTime?: Date;
}

export class CircuitBreaker {
  private state: CircuitState = CircuitState.CLOSED;
  private failures: number = 0;
  private successes: number = 0;
  private requests: number = 0;
  private lastFailureTime?: Date;
  private lastSuccessTime?: Date;
  private nextAttemptTime?: Date;

  constructor(private options: CircuitBreakerOptions) {}

  /**
   * Execute a function with circuit breaker protection
   */
  async execute<T>(fn: () => Promise<T>): Promise<T> {
    if (this.state === CircuitState.OPEN) {
      if (this.shouldAttemptReset()) {
        this.state = CircuitState.HALF_OPEN;
      } else {
        throw new Error(`Circuit breaker is OPEN. Next attempt at ${this.nextAttemptTime?.toISOString()}`);
      }
    }

    try {
      const result = await fn();
      this.onSuccess();
      return result;
    } catch (error) {
      this.onFailure();
      throw error;
    }
  }

  /**
   * Handle successful execution
   */
  private onSuccess(): void {
    this.successes++;
    this.requests++;
    this.lastSuccessTime = new Date();

    if (this.state === CircuitState.HALF_OPEN) {
      const successRate = this.successes / this.requests;
      if (successRate >= this.options.successThreshold) {
        this.reset();
      }
    }

    this.cleanupOldStats();
  }

  /**
   * Handle failed execution
   */
  private onFailure(): void {
    this.failures++;
    this.requests++;
    this.lastFailureTime = new Date();

    if (this.state === CircuitState.HALF_OPEN) {
      this.openCircuit();
    } else if (this.shouldOpenCircuit()) {
      this.openCircuit();
    }

    this.cleanupOldStats();
  }

  /**
   * Check if circuit should be opened
   */
  private shouldOpenCircuit(): boolean {
    return (
      this.requests >= this.options.minimumRequests &&
      this.failures >= this.options.failureThreshold
    );
  }

  /**
   * Check if we should attempt to reset the circuit
   */
  private shouldAttemptReset(): boolean {
    return (
      this.nextAttemptTime !== undefined &&
      new Date() >= this.nextAttemptTime
    );
  }

  /**
   * Open the circuit
   */
  private openCircuit(): void {
    this.state = CircuitState.OPEN;
    this.nextAttemptTime = new Date(Date.now() + this.options.recoveryTimeout);
  }

  /**
   * Reset the circuit to closed state
   */
  private reset(): void {
    this.state = CircuitState.CLOSED;
    this.failures = 0;
    this.successes = 0;
    this.requests = 0;
    this.nextAttemptTime = undefined;
  }

  /**
   * Clean up old statistics outside the monitoring period
   */
  private cleanupOldStats(): void {
    const cutoffTime = new Date(Date.now() - this.options.monitoringPeriod);
    
    // Reset counters if the last activity was before the monitoring period
    if (
      this.lastFailureTime && this.lastFailureTime < cutoffTime &&
      this.lastSuccessTime && this.lastSuccessTime < cutoffTime
    ) {
      this.failures = 0;
      this.successes = 0;
      this.requests = 0;
    }
  }

  /**
   * Get current circuit breaker statistics
   */
  getStats(): CircuitBreakerStats {
    return {
      state: this.state,
      failures: this.failures,
      successes: this.successes,
      requests: this.requests,
      lastFailureTime: this.lastFailureTime,
      lastSuccessTime: this.lastSuccessTime,
    };
  }

  /**
   * Manually reset the circuit breaker
   */
  manualReset(): void {
    this.reset();
  }

  /**
   * Check if the circuit is available for requests
   */
  isAvailable(): boolean {
    if (this.state === CircuitState.CLOSED || this.state === CircuitState.HALF_OPEN) {
      return true;
    }
    
    return this.shouldAttemptReset();
  }
}

/**
 * Factory function to create circuit breakers with sensible defaults
 */
export function createCircuitBreaker(
  partialOptions: Partial<CircuitBreakerOptions>
): CircuitBreaker {
  const defaultOptions: CircuitBreakerOptions = {
    failureThreshold: 5,
    recoveryTimeout: 60000, // 1 minute
    monitoringPeriod: 300000, // 5 minutes
    minimumRequests: 3,
    successThreshold: 0.5, // 50% success rate
  };

  const options = { ...defaultOptions, ...partialOptions };
  return new CircuitBreaker(options);
}

/**
 * Decorator function to add circuit breaker protection to methods
 */
export function withCircuitBreaker<T extends (...args: any[]) => Promise<any>>(
  fn: T,
  circuitBreaker: CircuitBreaker
): T {
  return (async (...args: any[]) => {
    return circuitBreaker.execute(() => fn(...args));
  }) as T;
}