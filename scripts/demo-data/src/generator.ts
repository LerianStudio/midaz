/**
 * Main generator - Refactored to use OrchestrationService
 */

import { createGeneratorContainer } from './container/generator-factory';
import { OrchestrationService } from './services/orchestration-service';
import { Logger } from './services/logger';
import { GeneratorOptions, GeneratorConfig, VolumeSize } from './types';

/**
 * Main generator class
 * Now acts as a facade for the OrchestrationService
 */
export class Generator {
  private orchestrationService: OrchestrationService;
  private logger: Logger;
  private options: GeneratorOptions;

  private isGeneratorConfig(options: GeneratorOptions | GeneratorConfig): options is GeneratorConfig {
    return 'apiBaseUrl' in options || 'organizations' in options;
  }

  private convertConfigToOptions(config: GeneratorConfig): GeneratorOptions {
    return {
      volume: VolumeSize.SMALL, // Default for test config
      baseUrl: config.apiBaseUrl || 'http://localhost',
      onboardingPort: 8080,
      transactionPort: 8081,
      concurrency: config.batchSize || 10,
      debug: true,
      authToken: undefined,
      seed: undefined,
    };
  }

  constructor(options: GeneratorOptions | GeneratorConfig) {
    // Convert GeneratorConfig to GeneratorOptions if needed
    if (this.isGeneratorConfig(options)) {
      this.options = this.convertConfigToOptions(options);
      this.logger = options.logger || new Logger(this.options);
    } else {
      this.options = options;
      this.logger = new Logger(options);
    }

    // Create DI container and orchestration service
    const container = createGeneratorContainer(this.options);
    this.orchestrationService = new OrchestrationService(container, this.options);

    this.logger.info(
      `Initialized Midaz generator for ${this.options.baseUrl}:${this.options.onboardingPort} ` +
      `and ${this.options.baseUrl}:${this.options.transactionPort}`
    );
  }

  /**
   * Run the generator with the provided options
   */
  public async run(): Promise<void> {
    this.logger.info(`Starting data generation with volume: ${this.options.volume}`);

    try {
      const result = await this.orchestrationService.orchestrateGeneration();

      if (result.success) {
        this.logger.info('Demo data generation completed successfully!');
        
        // Log final metrics
        this.logger.metrics({
          totalOrganizations: result.metrics.totalOrganizations,
          totalLedgers: result.metrics.totalLedgers,
          totalAssets: result.metrics.totalAssets,
          totalPortfolios: result.metrics.totalPortfolios,
          totalSegments: result.metrics.totalSegments,
          totalAccounts: result.metrics.totalAccounts,
          totalTransactions: result.metrics.totalTransactions,
          errors: result.metrics.errors,
          retries: result.metrics.retries,
          duration: result.metrics.duration(),
        });
      } else {
        this.logger.error('Generation failed', result.error!);
        throw result.error;
      }
    } catch (error) {
      this.logger.error('Error during data generation:', error as Error);
      throw error;
    }
  }

  /**
   * Alias for run() method - maintains backward compatibility with tests
   */
  public async generateAll(): Promise<void> {
    return this.run();
  }
}