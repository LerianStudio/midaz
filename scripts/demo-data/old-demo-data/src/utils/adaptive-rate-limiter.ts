/**
 * Adaptive rate limiter that adjusts concurrency based on circuit breaker state
 */

import { Logger } from '../services/logger';

export class AdaptiveRateLimiter {
  private currentConcurrency: number;
  private minConcurrency: number = 1;
  private maxConcurrency: number;
  private consecutiveSuccesses: number = 0;
  private consecutiveFailures: number = 0;
  private lastAdjustmentTime: number = Date.now();
  private adjustmentCooldown: number = 5000; // 5 seconds between adjustments

  constructor(
    private logger: Logger,
    initialConcurrency: number,
    maxConcurrency: number
  ) {
    this.currentConcurrency = initialConcurrency;
    this.maxConcurrency = maxConcurrency;
  }

  /**
   * Get the current concurrency level
   */
  getConcurrency(): number {
    return this.currentConcurrency;
  }

  /**
   * Record a successful operation
   */
  recordSuccess(): void {
    this.consecutiveSuccesses++;
    this.consecutiveFailures = 0;

    // Increase concurrency after 10 consecutive successes
    if (this.consecutiveSuccesses >= 10 && this.canAdjust()) {
      this.increaseConcurrency();
    }
  }

  /**
   * Record a failed operation
   */
  recordFailure(error: Error): void {
    this.consecutiveFailures++;
    this.consecutiveSuccesses = 0;

    // Check if it's a circuit breaker error
    if (error.message.includes('Circuit breaker is OPEN')) {
      // Immediately reduce to minimum concurrency
      this.logger.warn('Circuit breaker opened - reducing to minimum concurrency');
      this.currentConcurrency = this.minConcurrency;
      this.lastAdjustmentTime = Date.now();
    } else if (error.message.includes('queue full') || error.message.includes('timeout')) {
      // Reduce concurrency for queue/timeout errors
      if (this.canAdjust()) {
        this.decreaseConcurrency();
      }
    }
  }

  /**
   * Check if we can adjust concurrency (cooldown period)
   */
  private canAdjust(): boolean {
    return Date.now() - this.lastAdjustmentTime > this.adjustmentCooldown;
  }

  /**
   * Increase concurrency level
   */
  private increaseConcurrency(): void {
    const oldConcurrency = this.currentConcurrency;
    this.currentConcurrency = Math.min(this.currentConcurrency + 1, this.maxConcurrency);
    
    if (oldConcurrency !== this.currentConcurrency) {
      this.logger.debug(`Increasing concurrency from ${oldConcurrency} to ${this.currentConcurrency}`);
      this.lastAdjustmentTime = Date.now();
      this.consecutiveSuccesses = 0;
    }
  }

  /**
   * Decrease concurrency level
   */
  private decreaseConcurrency(): void {
    const oldConcurrency = this.currentConcurrency;
    this.currentConcurrency = Math.max(Math.floor(this.currentConcurrency * 0.7), this.minConcurrency);
    
    if (oldConcurrency !== this.currentConcurrency) {
      this.logger.warn(`Reducing concurrency from ${oldConcurrency} to ${this.currentConcurrency} due to errors`);
      this.lastAdjustmentTime = Date.now();
      this.consecutiveFailures = 0;
    }
  }

  /**
   * Get current stats
   */
  getStats(): { 
    currentConcurrency: number; 
    consecutiveSuccesses: number; 
    consecutiveFailures: number 
  } {
    return {
      currentConcurrency: this.currentConcurrency,
      consecutiveSuccesses: this.consecutiveSuccesses,
      consecutiveFailures: this.consecutiveFailures
    };
  }
}