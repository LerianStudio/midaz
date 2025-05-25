/**
 * Advanced progress reporting with real-time updates and metrics
 */

import { Logger } from '../services/logger';

export interface ProgressMetrics {
  totalItems: number;
  completedItems: number;
  failedItems: number;
  skippedItems: number;
  startTime: Date;
  currentTime: Date;
  estimatedCompletion?: Date;
  throughputPerSecond: number;
  averageItemTime: number;
}

export interface ProgressReportOptions {
  /** Update interval in milliseconds */
  updateInterval: number;
  /** Show estimated completion time */
  showETA: boolean;
  /** Show throughput metrics */
  showThroughput: boolean;
  /** Show detailed progress bar */
  showProgressBar: boolean;
  /** Progress bar width in characters */
  progressBarWidth: number;
}

export class ProgressReporter {
  private metrics: ProgressMetrics;
  private intervalId?: any;
  private lastUpdateTime: Date;
  private itemTimes: number[] = [];
  private readonly maxItemTimesSamples = 100;

  constructor(
    private entityType: string,
    private totalItems: number,
    private logger: Logger,
    private options: Partial<ProgressReportOptions> = {}
  ) {
    const now = new Date();
    this.metrics = {
      totalItems,
      completedItems: 0,
      failedItems: 0,
      skippedItems: 0,
      startTime: now,
      currentTime: now,
      throughputPerSecond: 0,
      averageItemTime: 0,
    };
    this.lastUpdateTime = now;

    this.options = {
      updateInterval: 2000, // 2 seconds
      showETA: true,
      showThroughput: true,
      showProgressBar: true,
      progressBarWidth: 30,
      ...this.options,
    };
  }

  /**
   * Start progress reporting with periodic updates
   */
  start(): void {
    this.logger.info(`🚀 Starting generation of ${this.totalItems} ${this.entityType}s`);
    this.updateMetrics();
    this.reportProgress();

    if (this.options.updateInterval && this.options.updateInterval > 0) {
      this.intervalId = setInterval(() => {
        this.updateMetrics();
        this.reportProgress();
      }, this.options.updateInterval);
    }
  }

  /**
   * Report completion of an item
   */
  reportItemCompleted(processingTimeMs?: number): void {
    this.metrics.completedItems++;
    
    if (processingTimeMs !== undefined) {
      this.itemTimes.push(processingTimeMs);
      if (this.itemTimes.length > this.maxItemTimesSamples) {
        this.itemTimes.shift();
      }
    }
  }

  /**
   * Report failure of an item
   */
  reportItemFailed(): void {
    this.metrics.failedItems++;
  }

  /**
   * Report skipping of an item
   */
  reportItemSkipped(): void {
    this.metrics.skippedItems++;
  }

  /**
   * Report completion of multiple items in batch
   */
  reportBatchCompleted(count: number, batchProcessingTimeMs?: number): void {
    this.metrics.completedItems += count;
    
    if (batchProcessingTimeMs !== undefined) {
      const avgTimePerItem = batchProcessingTimeMs / count;
      for (let i = 0; i < count; i++) {
        this.itemTimes.push(avgTimePerItem);
      }
      
      // Trim to max samples
      while (this.itemTimes.length > this.maxItemTimesSamples) {
        this.itemTimes.shift();
      }
    }
  }

  /**
   * Update current metrics
   */
  private updateMetrics(): void {
    const now = new Date();
    this.metrics.currentTime = now;
    
    const elapsedMs = now.getTime() - this.metrics.startTime.getTime();
    const elapsedSeconds = elapsedMs / 1000;
    
    const processedItems = this.metrics.completedItems + this.metrics.failedItems + this.metrics.skippedItems;
    
    // Calculate throughput
    if (elapsedSeconds > 0) {
      this.metrics.throughputPerSecond = processedItems / elapsedSeconds;
    }
    
    // Calculate average item time
    if (this.itemTimes.length > 0) {
      this.metrics.averageItemTime = this.itemTimes.reduce((sum, time) => sum + time, 0) / this.itemTimes.length;
    }
    
    // Calculate estimated completion time
    if (this.options.showETA && this.metrics.throughputPerSecond > 0) {
      const remainingItems = this.metrics.totalItems - processedItems;
      const estimatedRemainingSeconds = remainingItems / this.metrics.throughputPerSecond;
      this.metrics.estimatedCompletion = new Date(now.getTime() + estimatedRemainingSeconds * 1000);
    }
  }

  /**
   * Generate progress bar string
   */
  private generateProgressBar(): string {
    if (!this.options.showProgressBar) return '';
    
    const processedItems = this.metrics.completedItems + this.metrics.failedItems + this.metrics.skippedItems;
    const progress = Math.min(processedItems / this.metrics.totalItems, 1);
    const width = this.options.progressBarWidth || 30;
    
    const filled = Math.floor(progress * width);
    const empty = width - filled;
    
    const bar = '█'.repeat(filled) + '░'.repeat(empty);
    const percentage = (progress * 100).toFixed(1);
    
    return `[${bar}] ${percentage}%`;
  }

  /**
   * Format time duration
   */
  private formatDuration(ms: number): string {
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    
    if (hours > 0) {
      return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
    } else if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`;
    } else {
      return `${seconds}s`;
    }
  }

  /**
   * Report current progress
   */
  private reportProgress(): void {
    const processedItems = this.metrics.completedItems + this.metrics.failedItems + this.metrics.skippedItems;
    const elapsedMs = this.metrics.currentTime.getTime() - this.metrics.startTime.getTime();
    
    let message = `📊 ${this.entityType} Progress: ${processedItems}/${this.metrics.totalItems}`;
    
    if (this.options.showProgressBar) {
      message += ` ${this.generateProgressBar()}`;
    }
    
    // Add success/failure breakdown
    if (this.metrics.failedItems > 0 || this.metrics.skippedItems > 0) {
      message += ` (✅ ${this.metrics.completedItems}`;
      if (this.metrics.failedItems > 0) message += ` ❌ ${this.metrics.failedItems}`;
      if (this.metrics.skippedItems > 0) message += ` ⏭️ ${this.metrics.skippedItems}`;
      message += ')';
    }
    
    // Add timing information
    message += ` | ⏱️ ${this.formatDuration(elapsedMs)}`;
    
    if (this.options.showThroughput && this.metrics.throughputPerSecond > 0) {
      message += ` | 🚀 ${this.metrics.throughputPerSecond.toFixed(1)}/s`;
    }
    
    if (this.options.showETA && this.metrics.estimatedCompletion) {
      const eta = this.metrics.estimatedCompletion.getTime() - this.metrics.currentTime.getTime();
      message += ` | ETA: ${this.formatDuration(eta)}`;
    }
    
    this.logger.info(message);
  }

  /**
   * Stop progress reporting and show final summary
   */
  stop(): void {
    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = undefined;
    }
    
    this.updateMetrics();
    this.showFinalSummary();
  }

  /**
   * Show final summary with detailed metrics
   */
  private showFinalSummary(): void {
    const elapsedMs = this.metrics.currentTime.getTime() - this.metrics.startTime.getTime();
    const processedItems = this.metrics.completedItems + this.metrics.failedItems + this.metrics.skippedItems;
    
    this.logger.info('');
    this.logger.info(`🎯 ${this.entityType} Generation Complete!`);
    this.logger.info('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    this.logger.info(`📈 Total Processed: ${processedItems}/${this.metrics.totalItems}`);
    this.logger.info(`✅ Successful: ${this.metrics.completedItems}`);
    
    if (this.metrics.failedItems > 0) {
      this.logger.warn(`❌ Failed: ${this.metrics.failedItems}`);
    }
    
    if (this.metrics.skippedItems > 0) {
      this.logger.info(`⏭️ Skipped: ${this.metrics.skippedItems}`);
    }
    
    this.logger.info(`⏱️ Total Time: ${this.formatDuration(elapsedMs)}`);
    
    if (this.metrics.throughputPerSecond > 0) {
      this.logger.info(`🚀 Average Throughput: ${this.metrics.throughputPerSecond.toFixed(2)}/s`);
    }
    
    if (this.metrics.averageItemTime > 0) {
      this.logger.info(`⚡ Average Item Time: ${this.metrics.averageItemTime.toFixed(0)}ms`);
    }
    
    const successRate = processedItems > 0 ? (this.metrics.completedItems / processedItems * 100) : 0;
    this.logger.info(`🎯 Success Rate: ${successRate.toFixed(1)}%`);
    this.logger.info('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    this.logger.info('');
  }

  /**
   * Get current metrics
   */
  getMetrics(): Readonly<ProgressMetrics> {
    this.updateMetrics();
    return { ...this.metrics };
  }

  /**
   * Check if generation is complete
   */
  isComplete(): boolean {
    const processedItems = this.metrics.completedItems + this.metrics.failedItems + this.metrics.skippedItems;
    return processedItems >= this.metrics.totalItems;
  }

  /**
   * Force an immediate progress update
   */
  forceUpdate(): void {
    this.updateMetrics();
    this.reportProgress();
  }
}