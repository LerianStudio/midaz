/**
 * Base generator class with common functionality
 */

import { MidazClient } from 'midaz-sdk/src';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { StateManager } from '../utils/state';
import { CircuitBreaker, createCircuitBreaker } from '../utils/circuit-breaker';
import { Validator, ValidationResult } from '../validation/validator';
import { ValidationError } from '../errors';
import { ProgressReporter, ProgressReportOptions } from '../utils/progress-reporter';
import { z } from 'zod';

/**
 * Abstract base class for all entity generators
 * Provides common functionality like error handling, retries, and conflict resolution
 */
export abstract class BaseGenerator<T> implements EntityGenerator<T> {
  protected circuitBreaker: CircuitBreaker;

  constructor(
    protected client: MidazClient,
    protected logger: Logger,
    protected stateManager: StateManager,
    circuitBreakerOptions?: Partial<import('../utils/circuit-breaker').CircuitBreakerOptions>
  ) {
    this.circuitBreaker = createCircuitBreaker({
      failureThreshold: 3,
      recoveryTimeout: 30000,
      monitoringPeriod: 120000,
      minimumRequests: 2,
      successThreshold: 0.6,
      ...circuitBreakerOptions,
    });
  }

  /**
   * Generate multiple entities
   * Must be implemented by concrete generators
   */
  abstract generate(count: number, parentId?: string, organizationId?: string): Promise<T[]>;

  /**
   * Generate a single entity
   * Must be implemented by concrete generators
   */
  abstract generateOne(parentId?: string, organizationId?: string, options?: any): Promise<T>;

  /**
   * Check if an entity exists (optional implementation)
   */
  exists?(id: string, parentId?: string): Promise<boolean>;

  /**
   * Handle conflict errors by attempting to retrieve existing entity
   */
  protected async handleConflict<E>(
    error: Error,
    entityName: string,
    retriever: () => Promise<E>
  ): Promise<E | null> {
    if (this.isConflictError(error)) {
      this.logger.warn(`${entityName} may already exist, attempting retrieval`);
      try {
        return await retriever();
      } catch (retrieveError) {
        this.logger.error(`Failed to retrieve existing ${entityName}`, retrieveError as Error);
        return null;
      }
    }
    throw error;
  }

  /**
   * Check if an error is a conflict error (entity already exists)
   */
  protected isConflictError(error: Error): boolean {
    return error.message.includes('already exists') || 
           error.message.includes('conflict') ||
           error.message.includes('409');
  }

  /**
   * Execute an operation with retry logic
   */
  protected async withRetry<R>(
    operation: () => Promise<R>,
    operationName: string,
    maxRetries: number = 3
  ): Promise<R> {
    let lastError: Error;

    for (let attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        return await operation();
      } catch (error) {
        lastError = error as Error;
        
        if (attempt < maxRetries) {
          const delay = Math.min(100 * Math.pow(2, attempt), 2000);
          this.logger.debug(
            `Retry ${attempt}/${maxRetries} for ${operationName} after ${delay}ms`
          );
          await new Promise(resolve => setTimeout(resolve, delay));
        }
      }
    }

    throw lastError!;
  }

  /**
   * Validate required parameters
   */
  protected validateRequired(value: any, paramName: string): void {
    if (!value) {
      throw new Error(`${paramName} is required`);
    }
  }

  /**
   * Get organization ID with fallback
   */
  protected getOrganizationId(providedOrgId?: string): string {
    const orgId = providedOrgId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      throw new Error('Cannot proceed without an organization ID');
    }
    return orgId;
  }

  /**
   * Track an error in the state manager
   */
  protected trackError(
    entityType: string,
    parentId: string,
    error: Error,
    context?: Record<string, any>
  ): void {
    this.stateManager.trackGenerationError(entityType, parentId, error, context);
    this.stateManager.incrementErrorCount(entityType);
  }

  /**
   * Log progress with consistent formatting
   */
  protected logProgress(
    entityType: string,
    current: number,
    total: number,
    parentId?: string
  ): void {
    const parentContext = parentId ? ` for ${parentId}` : '';
    this.logger.progress(`${entityType}s created${parentContext}`, current, total);
  }

  /**
   * Log completion with consistent formatting
   */
  protected logCompletion(
    entityType: string,
    count: number,
    parentId?: string
  ): void {
    const parentContext = parentId ? ` for ${parentId}` : '';
    this.logger.info(`Successfully generated ${count} ${entityType}s${parentContext}`);
  }

  /**
   * Generate a safe entity name
   */
  protected generateSafeName(baseName: string, maxLength: number = 100): string {
    if (baseName.length <= maxLength) {
      return baseName;
    }
    return baseName.substring(0, maxLength - 3) + '...';
  }

  /**
   * Wait for a specified duration
   */
  protected async wait(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  /**
   * Validate data against a schema before processing
   */
  protected validateData<V>(
    schema: z.ZodSchema<V>,
    data: unknown,
    entityType: string
  ): V {
    return Validator.validateOrThrow(schema, data, entityType);
  }

  /**
   * Validate data safely without throwing
   */
  protected safeValidateData<V>(
    schema: z.ZodSchema<V>,
    data: unknown,
    entityType: string
  ): ValidationResult<V> {
    return Validator.validate(schema, data, entityType);
  }

  /**
   * Validate an array of data
   */
  protected validateBatchData<V>(
    schema: z.ZodSchema<V>,
    dataArray: unknown[],
    entityType: string
  ): V[] {
    return Validator.validateBatchOrThrow(schema, dataArray, entityType);
  }

  /**
   * Execute operation with circuit breaker protection
   */
  protected async executeWithCircuitBreaker<R>(
    operation: () => Promise<R>,
    operationName: string
  ): Promise<R> {
    try {
      return await this.circuitBreaker.execute(operation);
    } catch (error) {
      this.logger.error(`Circuit breaker protected operation '${operationName}' failed`, error as Error);
      throw error;
    }
  }

  /**
   * Execute operation with both circuit breaker and retry logic
   */
  protected async executeWithProtection<R>(
    operation: () => Promise<R>,
    operationName: string,
    maxRetries: number = 3
  ): Promise<R> {
    return this.executeWithCircuitBreaker(
      () => this.withRetry(operation, operationName, maxRetries),
      operationName
    );
  }

  /**
   * Get circuit breaker statistics
   */
  protected getCircuitStats() {
    return this.circuitBreaker.getStats();
  }

  /**
   * Check if the circuit breaker is available
   */
  protected isCircuitAvailable(): boolean {
    return this.circuitBreaker.isAvailable();
  }

  /**
   * Manually reset the circuit breaker
   */
  protected resetCircuit(): void {
    this.circuitBreaker.manualReset();
    this.logger.info('Circuit breaker manually reset');
  }

  /**
   * Create a progress reporter for batch operations
   */
  protected createProgressReporter(
    entityType: string,
    totalItems: number,
    options?: Partial<ProgressReportOptions>
  ): ProgressReporter {
    return new ProgressReporter(entityType, totalItems, this.logger, options);
  }

  /**
   * Execute a batch operation with progress reporting
   */
  protected async executeBatchWithProgress<T>(
    entityType: string,
    items: any[],
    batchSize: number,
    processor: (batch: any[], batchIndex: number) => Promise<T[]>,
    progressOptions?: Partial<ProgressReportOptions>
  ): Promise<T[]> {
    const progressReporter = this.createProgressReporter(entityType, items.length, progressOptions);
    progressReporter.start();

    const results: T[] = [];
    const batches = this.createBatches(items, batchSize);
    
    try {
      for (let i = 0; i < batches.length; i++) {
        const batch = batches[i];
        const batchStartTime = Date.now();
        
        try {
          const batchResults = await processor(batch, i);
          results.push(...batchResults);
          
          const batchEndTime = Date.now();
          const batchProcessingTime = batchEndTime - batchStartTime;
          
          progressReporter.reportBatchCompleted(batchResults.length, batchProcessingTime);
          
        } catch (error) {
          this.logger.error(`Batch ${i + 1}/${batches.length} failed`, error as Error);
          
          // Mark all items in failed batch as failed
          for (let j = 0; j < batch.length; j++) {
            progressReporter.reportItemFailed();
          }
          
          // Continue with next batch instead of failing completely
          continue;
        }
      }
    } finally {
      progressReporter.stop();
    }
    
    return results;
  }

  /**
   * Create batches from an array
   */
  protected createBatches<T>(items: T[], batchSize: number): T[][] {
    const batches: T[][] = [];
    for (let i = 0; i < items.length; i += batchSize) {
      batches.push(items.slice(i, i + batchSize));
    }
    return batches;
  }

  /**
   * Execute items one by one with progress reporting and individual error handling
   */
  protected async executeSequentialWithProgress<T, R>(
    entityType: string,
    items: T[],
    processor: (item: T, index: number) => Promise<R>,
    progressOptions?: Partial<ProgressReportOptions>
  ): Promise<R[]> {
    const progressReporter = this.createProgressReporter(entityType, items.length, progressOptions);
    progressReporter.start();

    const results: R[] = [];
    
    try {
      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        const itemStartTime = Date.now();
        
        try {
          const result = await processor(item, i);
          results.push(result);
          
          const itemEndTime = Date.now();
          const itemProcessingTime = itemEndTime - itemStartTime;
          
          progressReporter.reportItemCompleted(itemProcessingTime);
          
        } catch (error) {
          this.logger.error(`Item ${i + 1}/${items.length} failed`, error as Error);
          progressReporter.reportItemFailed();
          
          // Continue with next item instead of failing completely
          continue;
        }
      }
    } finally {
      progressReporter.stop();
    }
    
    return results;
  }
}