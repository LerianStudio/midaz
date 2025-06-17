/**
 * Circuit breaker monitoring utility
 */

import { Logger } from '../services/logger';

export interface CircuitBreakerEvent {
  timestamp: Date;
  type: 'open' | 'close' | 'half-open' | 'failure' | 'success';
  endpoint?: string;
  error?: string;
}

export class CircuitBreakerMonitor {
  private events: CircuitBreakerEvent[] = [];
  private openCircuits: Set<string> = new Set();
  private logger: Logger;

  constructor(logger: Logger) {
    this.logger = logger;
  }

  /**
   * Record a circuit breaker event
   */
  recordEvent(event: CircuitBreakerEvent): void {
    this.events.push(event);
    
    // Track open circuits
    if (event.type === 'open' && event.endpoint) {
      this.openCircuits.add(event.endpoint);
      this.logger.warn(`⚠️  Circuit breaker OPENED for ${event.endpoint}`);
    } else if (event.type === 'close' && event.endpoint) {
      this.openCircuits.delete(event.endpoint);
      this.logger.info(`✅ Circuit breaker CLOSED for ${event.endpoint}`);
    }
  }

  /**
   * Check if error is circuit breaker related
   */
  checkError(error: Error): boolean {
    if (error.message.includes('Circuit breaker is OPEN')) {
      // Extract endpoint from error message if possible
      const match = error.message.match(/endpoint: (.+)/);
      const endpoint = match ? match[1] : 'unknown';
      
      this.recordEvent({
        timestamp: new Date(),
        type: 'open',
        endpoint,
        error: error.message
      });
      
      return true;
    }
    return false;
  }

  /**
   * Get summary of circuit breaker events
   */
  getSummary(): {
    totalEvents: number;
    openCircuits: number;
    eventsByType: Record<string, number>;
    recentEvents: CircuitBreakerEvent[];
  } {
    const eventsByType: Record<string, number> = {};
    
    this.events.forEach(event => {
      eventsByType[event.type] = (eventsByType[event.type] || 0) + 1;
    });
    
    return {
      totalEvents: this.events.length,
      openCircuits: this.openCircuits.size,
      eventsByType,
      recentEvents: this.events.slice(-10) // Last 10 events
    };
  }

  /**
   * Print circuit breaker status
   */
  printStatus(): void {
    const summary = this.getSummary();
    
    this.logger.info('\n=== Circuit Breaker Status ===');
    this.logger.info(`Total events: ${summary.totalEvents}`);
    this.logger.info(`Currently open circuits: ${summary.openCircuits}`);
    
    if (summary.openCircuits > 0) {
      this.logger.warn('Open circuits:');
      this.openCircuits.forEach(circuit => {
        this.logger.warn(`  - ${circuit}`);
      });
    }
    
    this.logger.info('\nEvent breakdown:');
    Object.entries(summary.eventsByType).forEach(([type, count]) => {
      this.logger.info(`  ${type}: ${count}`);
    });
    
    if (summary.recentEvents.length > 0) {
      this.logger.info('\nRecent events:');
      summary.recentEvents.forEach(event => {
        const time = event.timestamp.toISOString().split('T')[1].split('.')[0];
        this.logger.info(`  [${time}] ${event.type} - ${event.endpoint || 'general'}`);
      });
    }
  }
}