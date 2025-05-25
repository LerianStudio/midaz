/**
 * Main generator orchestration
 */

import { MidazClient } from 'midaz-sdk/src';
import { VOLUME_METRICS } from './config';
import { DependencyError, GenerationError } from './errors';
import {
  AccountGenerator,
  AssetGenerator,
  LedgerGenerator,
  OrganizationGenerator,
  PortfolioGenerator,
  SegmentGenerator,
  TransactionGenerator,
} from './generators';
import { initializeClient } from './services/client';
import { Logger } from './services/logger';
import { GeneratorOptions, GeneratorConfig, VolumeSize } from './types';
import { StateManager } from './utils/state';

/**
 * Main generator class
 */
export class Generator {
  private client: MidazClient;
  private logger: Logger;
  private stateManager: StateManager;
  private options: GeneratorOptions;

  // Entity generators
  private organizationGenerator: OrganizationGenerator;
  private ledgerGenerator: LedgerGenerator;
  private assetGenerator: AssetGenerator;
  private portfolioGenerator: PortfolioGenerator;
  private segmentGenerator: SegmentGenerator;
  private accountGenerator: AccountGenerator;
  private transactionGenerator: TransactionGenerator;

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

    // Initialize client
    this.client = initializeClient(this.options);
    this.logger.info(
      `Initialized Midaz client connecting to ${this.options.baseUrl}:${this.options.onboardingPort} and ${this.options.baseUrl}:${this.options.transactionPort}`
    );

    // Get state manager
    this.stateManager = StateManager.getInstance();
    this.stateManager.reset();

    // Initialize entity generators
    this.organizationGenerator = new OrganizationGenerator(this.client, this.logger);
    this.ledgerGenerator = new LedgerGenerator(this.client, this.logger);
    this.assetGenerator = new AssetGenerator(this.client, this.logger);
    this.portfolioGenerator = new PortfolioGenerator(this.client, this.logger);
    this.segmentGenerator = new SegmentGenerator(this.client, this.logger);
    this.accountGenerator = new AccountGenerator(this.client, this.logger);
    this.transactionGenerator = new TransactionGenerator(this.client, this.logger);
  }

  /**
   * Validate that dependencies exist before generating dependent entities
   */
  private async validateDependencies(ledgerId: string): Promise<boolean> {
    const assets = this.stateManager.getAssetCodes(ledgerId);
    if (assets.length === 0) {
      this.logger.error(`No assets found for ledger ${ledgerId}`);
      return false;
    }
    return true;
  }

  /**
   * Verify that generation produced expected results
   */
  private async verifyGeneration<T>(
    entities: T[],
    entityType: string,
    minimumRequired: number = 1
  ): void {
    if (entities.length < minimumRequired) {
      throw new GenerationError(
        `Failed to generate minimum ${minimumRequired} ${entityType}s, got ${entities.length}`,
        entityType
      );
    }
  }

  /**
   * Generate entities with retry logic
   */
  private async generateWithRetry<T>(
    generator: () => Promise<T[]>,
    entityType: string,
    maxRetries: number = 3
  ): Promise<T[]> {
    for (let attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        const result = await generator();
        if (result.length > 0) return result;
        
        this.logger.warn(`${entityType} generation returned empty result, attempt ${attempt}/${maxRetries}`);
      } catch (error) {
        this.logger.error(`${entityType} generation failed, attempt ${attempt}/${maxRetries}`, error as Error);
        
        if (attempt === maxRetries) {
          throw new GenerationError(
            `Failed to generate ${entityType} after ${maxRetries} attempts`,
            entityType,
            undefined,
            { lastError: error }
          );
        }
        
        // Exponential backoff
        const delay = Math.pow(2, attempt) * 1000;
        this.logger.debug(`Waiting ${delay}ms before retry...`);
        await new Promise(resolve => setTimeout(resolve, delay));
      }
    }
    return [];
  }

  /**
   * Run the generator with the provided options
   */
  public async run(): Promise<void> {
    this.logger.info(`Starting data generation with volume: ${this.options.volume}`);

    try {
      // Get the metrics for the selected volume
      const volumeMetrics = VOLUME_METRICS[this.options.volume];

      // Generate organizations with retry
      const organizations = await this.generateWithRetry(
        () => this.organizationGenerator.generate(volumeMetrics.organizations),
        'organization'
      );
      
      await this.verifyGeneration(organizations, 'organization');

      // For each organization, generate ledgers and their nested entities
      for (const org of organizations) {
        this.logger.info(`Generating data for organization: ${org.id} (${org.legalName})`);

        // Generate ledgers for this organization with retry
        const ledgers = await this.generateWithRetry(
          () => this.ledgerGenerator.generate(volumeMetrics.ledgersPerOrg, org.id),
          'ledger'
        );
        
        await this.verifyGeneration(ledgers, 'ledger');

        // For each ledger, generate assets, portfolios, segments, accounts, and transactions
        for (const ledger of ledgers) {
          this.logger.info(`Generating data for ledger: ${ledger.id} (${ledger.name})`);

          // Generate assets for this ledger - CRITICAL for accounts
          const assets = await this.generateWithRetry(
            () => this.assetGenerator.generate(volumeMetrics.assetsPerLedger, ledger.id, org.id),
            'asset'
          );
          
          await this.verifyGeneration(assets, 'asset');

          // Generate portfolios for this ledger (optional entities)
          const portfolios = await this.portfolioGenerator.generate(
            volumeMetrics.portfoliosPerLedger, 
            ledger.id,
            org.id
          );

          // Generate segments for this ledger (optional entities)
          const segments = await this.segmentGenerator.generate(
            volumeMetrics.segmentsPerLedger, 
            ledger.id,
            org.id
          );

          // Validate dependencies before generating accounts
          if (!await this.validateDependencies(ledger.id)) {
            this.logger.error(`Skipping account generation for ledger ${ledger.id} due to missing dependencies`);
            this.stateManager.incrementErrorCount('account');
            continue;
          }

          // Generate accounts for this ledger
          const accounts = await this.accountGenerator.generate(
            volumeMetrics.accountsPerLedger, 
            ledger.id,
            org.id
          );
          
          // Accounts are critical - verify we have at least some
          if (accounts.length === 0) {
            this.logger.error(`No accounts generated for ledger ${ledger.id}, skipping transactions`);
            this.stateManager.incrementErrorCount('transaction');
            continue;
          }

          // Generate transactions for this ledger
          await this.transactionGenerator.generate(
            volumeMetrics.transactionsPerAccount, 
            ledger.id,
            org.id
          );
        }
      }

      // Generation complete, log metrics
      const finalMetrics = this.stateManager.completeGeneration();
      this.logger.metrics({
        totalOrganizations: finalMetrics.totalOrganizations,
        totalLedgers: finalMetrics.totalLedgers,
        totalAssets: finalMetrics.totalAssets,
        totalPortfolios: finalMetrics.totalPortfolios,
        totalSegments: finalMetrics.totalSegments,
        totalAccounts: finalMetrics.totalAccounts,
        totalTransactions: finalMetrics.totalTransactions,
        // Include error counts per entity type
        organizationErrors: finalMetrics.organizationErrors,
        ledgerErrors: finalMetrics.ledgerErrors,
        assetErrors: finalMetrics.assetErrors,
        portfolioErrors: finalMetrics.portfolioErrors,
        segmentErrors: finalMetrics.segmentErrors,
        accountErrors: finalMetrics.accountErrors,
        transactionErrors: finalMetrics.transactionErrors,
        errors: finalMetrics.errors,
        retries: finalMetrics.retries,
        duration: finalMetrics.duration(),
      });
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
