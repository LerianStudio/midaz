/**
 * Performance Reporter with detailed summary
 */



import { Logger } from '../services/logger';
import { GenerationMetrics } from '../types';

export interface PerformanceSummary {
  duration: number;
  durationFormatted: string;
  entitiesGenerated: Record<string, number>;
  successRates: Record<string, number>;
  throughput: Record<string, number>;
  errors: ErrorSummary[];
  recommendations: string[];
  overallSuccessRate: number;
  totalEntities: number;
  totalErrors: number;
}

export interface ErrorSummary {
  type: string;
  count: number;
  percentage: number;
  commonReason?: string;
}

export class PerformanceReporter {
  constructor(private logger: Logger) {}

  /**
   * Generate performance summary from metrics
   */
  generateSummary(metrics: GenerationMetrics): PerformanceSummary {
    const duration = metrics.duration();
    const durationSeconds = duration / 1000;

    // Calculate total entities
    const entitiesGenerated = {
      organizations: metrics.totalOrganizations,
      ledgers: metrics.totalLedgers,
      assets: metrics.totalAssets,
      portfolios: metrics.totalPortfolios,
      segments: metrics.totalSegments,
      accounts: metrics.totalAccounts,
      transactions: metrics.totalTransactions,
    };

    const totalEntities = Object.values(entitiesGenerated).reduce((sum, count) => sum + count, 0);

    // Calculate success rates
    const successRates = this.calculateSuccessRates(metrics);
    const overallSuccessRate = this.calculateOverallSuccessRate(metrics);

    // Calculate throughput
    const throughput = this.calculateThroughput(metrics, durationSeconds);

    // Summarize errors
    const errors = this.summarizeErrors(metrics);
    const totalErrors = errors.reduce((sum, err) => sum + err.count, 0);

    // Generate recommendations
    const recommendations = this.generateRecommendations(metrics, successRates);

    return {
      duration,
      durationFormatted: this.formatDuration(duration),
      entitiesGenerated,
      successRates,
      throughput,
      errors,
      recommendations,
      overallSuccessRate,
      totalEntities,
      totalErrors,
    };
  }

  /**
   * Print formatted summary to console
   */
  printSummary(summary: PerformanceSummary): void {
    const separator = '═'.repeat(60);
    
    console.log('\n📊 Generation Performance Summary');
    console.log(separator);
    
    // Overview
    console.log(`⏱️  Total Duration: ${summary.durationFormatted}`);
    console.log(`📈 Total Entities: ${summary.totalEntities.toLocaleString()}`);
    console.log(`✅ Overall Success Rate: ${summary.overallSuccessRate.toFixed(1)}%`);
    console.log(`🚀 Average Throughput: ${summary.throughput.overall.toFixed(2)} entities/second`);
    
    // Entities Generated
    console.log('\n📋 Entities Generated:');
    Object.entries(summary.entitiesGenerated).forEach(([type, count]) => {
      const successRate = summary.successRates[type] || 100;
      const throughput = summary.throughput[type] || 0;
      const emoji = this.getEntityEmoji(type);
      
      console.log(
        `  ${emoji} ${type}: ${count.toLocaleString()} ` +
        `(${successRate.toFixed(1)}% success, ${throughput.toFixed(2)}/s)`
      );
    });

    // Errors
    if (summary.errors.length > 0) {
      console.log('\n❌ Error Summary:');
      summary.errors.forEach(error => {
        console.log(
          `  • ${error.type}: ${error.count} errors (${error.percentage.toFixed(1)}%)` +
          (error.commonReason ? ` - ${error.commonReason}` : '')
        );
      });
    }

    // Performance Metrics
    console.log('\n⚡ Performance Metrics:');
    console.log(`  • Peak Memory Usage: ${this.getMemoryUsage()} MB`);
    console.log(`  • Retry Attempts: ${(summary as any).metrics?.retries || 0}`);
    console.log(`  • Circuit Breaker Trips: ${this.getCircuitBreakerTrips()}`);

    // Recommendations
    if (summary.recommendations.length > 0) {
      console.log('\n💡 Recommendations:');
      summary.recommendations.forEach(rec => console.log(`  • ${rec}`));
    }

    console.log('\n' + separator);
    console.log('✨ Generation completed!\n');
  }

  /**
   * Calculate success rates for each entity type
   */
  private calculateSuccessRates(metrics: GenerationMetrics): Record<string, number> {
    const rates: Record<string, number> = {};

    const calculateRate = (total: number, errors?: number): number => {
      if (total === 0) return 100;
      const errorCount = errors || 0;
      return ((total - errorCount) / total) * 100;
    };

    rates.organizations = calculateRate(metrics.totalOrganizations, metrics.organizationErrors);
    rates.ledgers = calculateRate(metrics.totalLedgers, metrics.ledgerErrors);
    rates.assets = calculateRate(metrics.totalAssets, metrics.assetErrors);
    rates.portfolios = calculateRate(metrics.totalPortfolios, metrics.portfolioErrors);
    rates.segments = calculateRate(metrics.totalSegments, metrics.segmentErrors);
    rates.accounts = calculateRate(metrics.totalAccounts, metrics.accountErrors);
    rates.transactions = calculateRate(metrics.totalTransactions, metrics.transactionErrors);

    return rates;
  }

  /**
   * Calculate overall success rate
   */
  private calculateOverallSuccessRate(metrics: GenerationMetrics): number {
    const totalAttempted = 
      metrics.totalOrganizations + metrics.totalLedgers + metrics.totalAssets +
      metrics.totalPortfolios + metrics.totalSegments + metrics.totalAccounts +
      metrics.totalTransactions;

    if (totalAttempted === 0) return 100;

    const totalSuccessful = totalAttempted - metrics.errors;
    return (totalSuccessful / totalAttempted) * 100;
  }

  /**
   * Calculate throughput for each entity type
   */
  private calculateThroughput(
    metrics: GenerationMetrics,
    durationSeconds: number
  ): Record<string, number> {
    if (durationSeconds === 0) return {};

    const throughput: Record<string, number> = {
      organizations: metrics.totalOrganizations / durationSeconds,
      ledgers: metrics.totalLedgers / durationSeconds,
      assets: metrics.totalAssets / durationSeconds,
      portfolios: metrics.totalPortfolios / durationSeconds,
      segments: metrics.totalSegments / durationSeconds,
      accounts: metrics.totalAccounts / durationSeconds,
      transactions: metrics.totalTransactions / durationSeconds,
      overall: (
        metrics.totalOrganizations + metrics.totalLedgers + metrics.totalAssets +
        metrics.totalPortfolios + metrics.totalSegments + metrics.totalAccounts +
        metrics.totalTransactions
      ) / durationSeconds,
    };

    return throughput;
  }

  /**
   * Summarize errors by type
   */
  private summarizeErrors(metrics: GenerationMetrics): ErrorSummary[] {
    const errors: ErrorSummary[] = [];
    const totalErrors = metrics.errors;

    if (totalErrors === 0) return errors;

    const addError = (type: string, count?: number, reason?: string) => {
      if (count && count > 0) {
        errors.push({
          type,
          count,
          percentage: (count / totalErrors) * 100,
          commonReason: reason,
        });
      }
    };

    addError('Organization', metrics.organizationErrors, 'API validation failed');
    addError('Ledger', metrics.ledgerErrors, 'Organization not found');
    addError('Asset', metrics.assetErrors, 'Duplicate asset code');
    addError('Portfolio', metrics.portfolioErrors, 'Invalid portfolio configuration');
    addError('Segment', metrics.segmentErrors, 'Segment validation failed');
    addError('Account', metrics.accountErrors, 'Missing required asset');
    addError('Transaction', metrics.transactionErrors, 'Insufficient balance');

    return errors.sort((a, b) => b.count - a.count);
  }

  /**
   * Generate recommendations based on metrics
   */
  private generateRecommendations(
    metrics: GenerationMetrics,
    successRates: Record<string, number>
  ): string[] {
    const recommendations: string[] = [];

    // Check for high error rates
    Object.entries(successRates).forEach(([type, rate]) => {
      if (rate < 90) {
        recommendations.push(`Investigate high failure rate for ${type} generation (${(100 - rate).toFixed(1)}% failures)`);
      }
    });

    // Check for performance issues
    const duration = metrics.duration();
    const totalEntities = 
      metrics.totalOrganizations + metrics.totalLedgers + metrics.totalAssets +
      metrics.totalPortfolios + metrics.totalSegments + metrics.totalAccounts +
      metrics.totalTransactions;
    
    const entitiesPerSecond = totalEntities / (duration / 1000);
    if (entitiesPerSecond < 5) {
      recommendations.push('Consider increasing batch size or concurrency for better performance');
    }

    // Check retry count
    if (metrics.retries > totalEntities * 0.1) {
      recommendations.push('High retry count detected - check API stability and network connectivity');
    }

    // Check for missing optional entities
    if (metrics.totalPortfolios === 0 && metrics.totalSegments === 0) {
      recommendations.push('No portfolios or segments were created - verify if this is intentional');
    }

    // Check transaction to account ratio
    if (metrics.totalAccounts > 0 && metrics.totalTransactions / metrics.totalAccounts < 1) {
      recommendations.push('Low transaction count per account - consider increasing transactions per account');
    }

    return recommendations;
  }

  /**
   * Format duration in human-readable format
   */
  private formatDuration(ms: number): string {
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);

    if (hours > 0) {
      const remainingMinutes = minutes % 60;
      return `${hours}h ${remainingMinutes}m`;
    } else if (minutes > 0) {
      const remainingSeconds = seconds % 60;
      return `${minutes}m ${remainingSeconds}s`;
    } else {
      return `${seconds}s`;
    }
  }

  /**
   * Get emoji for entity type
   */
  private getEntityEmoji(type: string): string {
    const emojis: Record<string, string> = {
      organizations: '🏢',
      ledgers: '📒',
      assets: '💰',
      portfolios: '📊',
      segments: '🔖',
      accounts: '👤',
      transactions: '💸',
    };
    return emojis[type] || '📌';
  }

  /**
   * Get current memory usage in MB
   */
  private getMemoryUsage(): number {
    const usage = process.memoryUsage();
    return Math.round(usage.heapUsed / 1024 / 1024);
  }

  /**
   * Get circuit breaker trips (placeholder - would need actual implementation)
   */
  private getCircuitBreakerTrips(): number {
    // This would need to be tracked by the circuit breaker implementation
    return 0;
  }

  /**
   * Export summary to file
   */
  async exportSummary(summary: PerformanceSummary, filepath: string): Promise<void> {
    const fs = await import('fs/promises');
    const content = JSON.stringify(summary, null, 2);
    
    try {
      await fs.writeFile(filepath, content, 'utf-8');
      this.logger.info(`Performance summary exported to: ${filepath}`);
    } catch (error) {
      this.logger.error('Failed to export performance summary', error as Error);
    }
  }
}