/**
 * Error tracking system for demo data generation
 */

import { GenerationError } from '../errors';

export interface GenerationErrorInfo {
  entityType: string;
  parentId: string;
  error: Error;
  timestamp: Date;
  context: Record<string, any>;
}

export interface ErrorReport {
  totalErrors: number;
  errorsByType: Record<string, number>;
  recentErrors: GenerationErrorInfo[];
  errorSummary: string;
}

/**
 * Tracks errors during generation process
 */
export class ErrorTracker {
  private errors: GenerationErrorInfo[] = [];

  /**
   * Track a generation error
   */
  trackError(errorInfo: GenerationErrorInfo): void {
    this.errors.push(errorInfo);
  }

  /**
   * Get errors by entity type
   */
  getErrorsByType(entityType: string): GenerationErrorInfo[] {
    return this.errors.filter(error => error.entityType === entityType);
  }

  /**
   * Get all tracked errors
   */
  getAllErrors(): GenerationErrorInfo[] {
    return [...this.errors];
  }

  /**
   * Get recent errors (last N errors)
   */
  getRecentErrors(count: number = 10): GenerationErrorInfo[] {
    return this.errors.slice(-count);
  }

  /**
   * Generate an error report
   */
  generateReport(): ErrorReport {
    const errorsByType: Record<string, number> = {};
    
    this.errors.forEach(error => {
      errorsByType[error.entityType] = (errorsByType[error.entityType] || 0) + 1;
    });

    const totalErrors = this.errors.length;
    const recentErrors = this.getRecentErrors(5);
    
    let errorSummary = `Total errors: ${totalErrors}`;
    if (totalErrors > 0) {
      errorSummary += '\nErrors by type:\n';
      Object.entries(errorsByType).forEach(([type, count]) => {
        errorSummary += `  ${type}: ${count}\n`;
      });
    }

    return {
      totalErrors,
      errorsByType,
      recentErrors,
      errorSummary
    };
  }

  /**
   * Clear all tracked errors
   */
  clear(): void {
    this.errors = [];
  }

  /**
   * Check if there are any errors
   */
  hasErrors(): boolean {
    return this.errors.length > 0;
  }

  /**
   * Get error count for specific entity type
   */
  getErrorCount(entityType?: string): number {
    if (!entityType) {
      return this.errors.length;
    }
    return this.errors.filter(error => error.entityType === entityType).length;
  }
}