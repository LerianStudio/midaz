/**
 * Metrics Plugin
 * Tracks generation metrics and performance data
 */

import { BasePlugin, PluginContext, EntityContext, ErrorContext } from '../internal/plugin-interface';
import { GenerationMetrics } from '../../types';

interface EntityMetrics {
  count: number;
  successCount: number;
  errorCount: number;
  totalDuration: number;
  averageDuration: number;
  lastGenerated?: Date;
  errors: Array<{
    timestamp: Date;
    message: string;
    context?: any;
  }>;
}

export class MetricsPlugin extends BasePlugin {
  name = 'MetricsPlugin';
  version = '1.0.0';
  priority = 10; // Run early to capture all metrics

  private entityMetrics = new Map<string, EntityMetrics>();
  private generationStartTime?: Date;
  private checkpointCount = 0;
  private memoryWarnings = 0;

  async onInit(context: PluginContext): Promise<void> {
    context.logger.debug('MetricsPlugin initialized');
  }

  async beforeGeneration(config: any): Promise<void> {
    this.generationStartTime = new Date();
    this.entityMetrics.clear();
    this.checkpointCount = 0;
    this.memoryWarnings = 0;
  }

  async afterGeneration(metrics: GenerationMetrics): Promise<void> {
    const summary = this.generateSummary();
    const logger = (globalThis as any).logger;
    
    if (logger) {
      logger.debug('Generation Metrics Summary:', summary);
    }
  }

  async beforeEntityGeneration(type: string, config: any): Promise<void> {
    if (!this.entityMetrics.has(type)) {
      this.entityMetrics.set(type, {
        count: 0,
        successCount: 0,
        errorCount: 0,
        totalDuration: 0,
        averageDuration: 0,
        errors: [],
      });
    }
  }

  async afterEntityGeneration(context: EntityContext): Promise<void> {
    const metrics = this.entityMetrics.get(context.type);
    if (!metrics) return;

    metrics.count++;
    metrics.successCount++;
    metrics.lastGenerated = new Date();
    
    // Update average duration if processing time is available
    const processingTime = (context as any).processingTime;
    if (processingTime) {
      metrics.totalDuration += processingTime;
      metrics.averageDuration = metrics.totalDuration / metrics.count;
    }
  }

  async onEntityGenerationError(error: ErrorContext): Promise<void> {
    const metrics = this.entityMetrics.get(error.entityType);
    if (!metrics) return;

    metrics.errorCount++;
    metrics.errors.push({
      timestamp: new Date(),
      message: error.error.message,
      context: {
        parentId: error.parentId,
        operation: error.operation,
        attempt: error.attempt,
      },
    });

    // Keep only last 100 errors per entity type
    if (metrics.errors.length > 100) {
      metrics.errors = metrics.errors.slice(-100);
    }
  }

  onMetricsUpdate(metrics: Partial<GenerationMetrics>): void {
    // Track overall metrics updates
  }

  onMemoryWarning(stats: any): void {
    this.memoryWarnings++;
  }

  async onCheckpoint(checkpointId: string): Promise<void> {
    this.checkpointCount++;
  }

  /**
   * Generate a summary of all collected metrics
   */
  private generateSummary(): any {
    const summary: any = {
      generationDuration: this.generationStartTime 
        ? Date.now() - this.generationStartTime.getTime()
        : 0,
      checkpointCount: this.checkpointCount,
      memoryWarnings: this.memoryWarnings,
      entityMetrics: {},
    };

    this.entityMetrics.forEach((metrics, type) => {
      summary.entityMetrics[type] = {
        total: metrics.count,
        successful: metrics.successCount,
        failed: metrics.errorCount,
        successRate: metrics.count > 0 
          ? (metrics.successCount / metrics.count * 100).toFixed(2) + '%'
          : '0%',
        averageGenerationTime: metrics.averageDuration.toFixed(2) + 'ms',
        lastGenerated: metrics.lastGenerated,
        recentErrors: metrics.errors.slice(-5), // Last 5 errors
      };
    });

    return summary;
  }

  /**
   * Get metrics for a specific entity type
   */
  getEntityMetrics(type: string): EntityMetrics | undefined {
    return this.entityMetrics.get(type);
  }

  /**
   * Get all metrics
   */
  getAllMetrics(): Map<string, EntityMetrics> {
    return new Map(this.entityMetrics);
  }

  /**
   * Export metrics to a format suitable for external monitoring
   */
  exportMetrics(): any {
    const metrics: any = {
      timestamp: new Date().toISOString(),
      plugin: {
        name: this.name,
        version: this.version,
      },
      generation: {
        startTime: this.generationStartTime?.toISOString(),
        duration: this.generationStartTime 
          ? Date.now() - this.generationStartTime.getTime()
          : 0,
        checkpoints: this.checkpointCount,
        memoryWarnings: this.memoryWarnings,
      },
      entities: {},
    };

    this.entityMetrics.forEach((entityMetrics, type) => {
      metrics.entities[type] = {
        count: entityMetrics.count,
        success: entityMetrics.successCount,
        errors: entityMetrics.errorCount,
        successRate: entityMetrics.count > 0 
          ? entityMetrics.successCount / entityMetrics.count
          : 0,
        performance: {
          totalDuration: entityMetrics.totalDuration,
          averageDuration: entityMetrics.averageDuration,
        },
      };
    });

    return metrics;
  }
}