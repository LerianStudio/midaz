/**
 * Optimized generator with parallel processing
 */

import { MidazClient, workerPool } from '@lerianstudio/midaz-sdk';
import { VOLUME_METRICS } from './config';
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
import { GeneratorOptions } from './types';
import { StateManager } from './utils/state';

/**
 * Optimized generator class with parallel processing
 */
export class OptimizedGenerator {
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

  constructor(options: GeneratorOptions) {
    this.options = options;
    this.logger = new Logger(options);

    // Initialize client
    this.client = initializeClient(options);
    this.logger.info(
      `Initialized Midaz client connecting to ${options.baseUrl}:${options.onboardingPort} and ${options.baseUrl}:${options.transactionPort}`
    );

    // Warm up the connection pool to prevent initial connection failures
    this.logger.debug('Warming up connection pool...');
    this.warmupConnectionPool();

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
   * Calculate optimal concurrency based on entity type and volume
   */
  private getOptimalConcurrency(entityType: string): number {
    const baseConcurrency: Record<string, number> = {
      organizations: 5,
      ledgers: 8,
      assets: 15,
      portfolios: 15,
      segments: 15,
      accounts: 30,
      transactions: 20,  // Reduced from 100 to avoid circuit breaker issues
    };

    // Adjust based on volume size
    const volumeMultiplier = 
      this.options.volume === 'large' ? 1.5 :   // Reduced from 2
      this.options.volume === 'medium' ? 1.2 :   // Reduced from 1.5
      1;

    // Use configured concurrency as a cap
    const calculated = Math.floor((baseConcurrency[entityType] || 10) * volumeMultiplier);
    return Math.min(calculated, this.options.concurrency || 10);
  }

  /**
   * Add a delay between macro processes
   */
  private async addProcessDelay(processName: string): Promise<void> {
    const delay = this.options.processDelay || 5;
    if (delay > 0) {
      this.logger.info(`Waiting ${delay} seconds for ${processName} to propagate through RabbitMQ/Redis...`);
      await new Promise(resolve => setTimeout(resolve, delay * 1000));
    }
  }

  /**
   * Warm up the connection pool by making a few initial requests
   */
  private async warmupConnectionPool(): Promise<void> {
    try {
      // Make a few lightweight requests to establish connections
      const warmupPromises = [];
      for (let i = 0; i < 3; i++) {
        warmupPromises.push(
          this.client.entities.organizations.listOrganizations({ limit: 1 })
            .catch(() => {}) // Ignore errors during warmup
        );
      }
      await Promise.all(warmupPromises);
      this.logger.debug('Connection pool warmed up');
    } catch (error) {
      // Warmup errors are not critical
      this.logger.debug('Connection pool warmup completed with some errors');
    }
  }

  /**
   * Run the optimized generator with parallel processing
   */
  public async run(): Promise<void> {
    const startTime = Date.now();
    this.logger.info(`Starting optimized data generation with volume: ${this.options.volume}`);

    try {
      // Get the metrics for the selected volume
      const volumeMetrics = VOLUME_METRICS[this.options.volume];

      // Step 1: Generate all organizations in parallel
      this.logger.info(`Generating ${volumeMetrics.organizations} organizations in parallel...`);
      const organizations = await this.generateOrganizationsParallel(volumeMetrics.organizations);
      
      // Wait for organizations to propagate
      await this.addProcessDelay('organizations');
      
      // Step 2: Generate ledgers for all organizations in parallel batches
      this.logger.info(`Generating ledgers for ${organizations.length} organizations...`);
      const ledgersByOrg = await this.generateLedgersParallel(organizations, volumeMetrics.ledgersPerOrg);
      
      // Flatten all ledgers for downstream processing
      const allLedgers = ledgersByOrg.flat();
      this.logger.info(`Total ledgers created: ${allLedgers.length}`);
      
      // Wait for ledgers to propagate
      await this.addProcessDelay('ledgers');

      // Step 3: Process all ledgers in parallel with their nested entities
      this.logger.info(`Processing ${allLedgers.length} ledgers with their entities...`);
      await this.processLedgersParallel(allLedgers, volumeMetrics);

      const endTime = Date.now();
      const duration = (endTime - startTime) / 1000;
      this.logger.info(`✅ Data generation completed in ${duration.toFixed(2)} seconds`);
      
      // Print summary statistics
      this.printSummary();

    } catch (error) {
      this.logger.error('Data generation failed', error as Error);
      throw error;
    }
  }

  /**
   * Generate organizations in parallel
   */
  private async generateOrganizationsParallel(count: number): Promise<any[]> {
    const concurrency = this.getOptimalConcurrency('organizations');
    this.logger.info(`Creating ${count} organizations with concurrency ${concurrency}`);

    const organizations = await workerPool(
      Array(count).fill(null),
      async (_, index) => {
        this.logger.debug(`Creating organization ${index + 1}/${count}`);
        const org = await this.organizationGenerator.generateSingle();
        this.logger.progress('Organizations created', index + 1, count);
        return org;
      },
      { 
        concurrency,
        continueOnError: true,
        preserveOrder: false 
      }
    );

    return organizations.filter(org => org !== null);
  }

  /**
   * Generate ledgers for all organizations in parallel
   */
  private async generateLedgersParallel(organizations: any[], ledgersPerOrg: number): Promise<any[][]> {
    const concurrency = Math.min(this.getOptimalConcurrency('ledgers'), organizations.length);
    this.logger.info(`Creating ledgers for organizations with concurrency ${concurrency}`);

    const ledgersByOrg = await workerPool(
      organizations,
      async (org, orgIndex) => {
        this.logger.debug(`Creating ${ledgersPerOrg} ledgers for organization ${org.legalName}`);
        
        // Create all ledgers for this org in parallel
        const ledgers = await workerPool(
          Array(ledgersPerOrg).fill(null),
          async (_, ledgerIndex) => {
            const ledger = await this.ledgerGenerator.generateSingle(org.id);
            const progress = orgIndex * ledgersPerOrg + ledgerIndex + 1;
            const total = organizations.length * ledgersPerOrg;
            this.logger.progress('Ledgers created', progress, total);
            return ledger;
          },
          { 
            concurrency: 5, // Limit concurrent ledgers per org
            continueOnError: true 
          }
        );

        return ledgers.filter(ledger => ledger !== null);
      },
      { 
        concurrency,
        continueOnError: true,
        preserveOrder: true 
      }
    );

    return ledgersByOrg;
  }

  /**
   * Process all ledgers in parallel with their entities
   */
  private async processLedgersParallel(ledgers: any[], volumeMetrics: any): Promise<void> {
    const concurrency = Math.min(this.getOptimalConcurrency('ledgers'), 5);
    this.logger.info(`Processing ${ledgers.length} ledgers with concurrency ${concurrency}`);

    // Process all ledgers' assets, portfolios, and segments first
    this.logger.info('Generating assets, portfolios, and segments for all ledgers...');
    await workerPool(
      ledgers,
      async (ledger, index) => {
        this.logger.debug(`Processing assets/portfolios/segments for ledger ${index + 1}/${ledgers.length}: ${ledger.name}`);
        
        // Generate assets, portfolios, and segments in parallel (they don't depend on each other)
        await Promise.all([
          this.generateAssetsForLedger(ledger.id, volumeMetrics.assetsPerLedger),
          this.generatePortfoliosForLedger(ledger.id, volumeMetrics.portfoliosPerLedger),
          this.generateSegmentsForLedger(ledger.id, volumeMetrics.segmentsPerLedger),
        ]);
      },
      { 
        concurrency,
        continueOnError: true 
      }
    );
    
    // Wait for assets/portfolios/segments to propagate
    await this.addProcessDelay('assets, portfolios, and segments');

    // Process all ledgers' accounts
    this.logger.info('Generating accounts for all ledgers...');
    await workerPool(
      ledgers,
      async (ledger, index) => {
        this.logger.debug(`Processing accounts for ledger ${index + 1}/${ledgers.length}: ${ledger.name}`);
        await this.generateAccountsForLedger(ledger.id, volumeMetrics.accountsPerLedger);
      },
      { 
        concurrency,
        continueOnError: true 
      }
    );
    
    // Wait for accounts to propagate
    await this.addProcessDelay('accounts');

    // Process all ledgers' transactions - SEQUENTIALLY to avoid overwhelming the server
    this.logger.info('Generating transactions for all ledgers (sequentially to avoid circuit breaker)...');
    for (let index = 0; index < ledgers.length; index++) {
      const ledger = ledgers[index];
      this.logger.info(`Processing transactions for ledger ${index + 1}/${ledgers.length}: ${ledger.name}`);
      await this.generateTransactionsForLedger(ledger.id, volumeMetrics.transactionsPerAccount);
      this.logger.progress('Ledgers fully processed', index + 1, ledgers.length);
      
      // Add delay between ledgers to allow server to process
      if (index < ledgers.length - 1) {
        this.logger.debug('Waiting 2 seconds before processing next ledger...');
        await new Promise(resolve => setTimeout(resolve, 2000));
      }
    }
  }

  /**
   * Generate assets for a ledger with progress tracking
   */
  private async generateAssetsForLedger(ledgerId: string, count: number): Promise<void> {
    try {
      this.logger.debug(`Generating ${count} assets for ledger ${ledgerId}`);
      await this.assetGenerator.generate(count, ledgerId);
    } catch (error) {
      this.logger.error(`Failed to generate assets for ledger ${ledgerId}`, error as Error);
    }
  }

  /**
   * Generate portfolios for a ledger with progress tracking
   */
  private async generatePortfoliosForLedger(ledgerId: string, count: number): Promise<void> {
    try {
      this.logger.debug(`Generating ${count} portfolios for ledger ${ledgerId}`);
      await this.portfolioGenerator.generate(count, ledgerId);
    } catch (error) {
      this.logger.error(`Failed to generate portfolios for ledger ${ledgerId}`, error as Error);
    }
  }

  /**
   * Generate segments for a ledger with progress tracking
   */
  private async generateSegmentsForLedger(ledgerId: string, count: number): Promise<void> {
    try {
      this.logger.debug(`Generating ${count} segments for ledger ${ledgerId}`);
      await this.segmentGenerator.generate(count, ledgerId);
    } catch (error) {
      this.logger.error(`Failed to generate segments for ledger ${ledgerId}`, error as Error);
    }
  }

  /**
   * Generate accounts for a ledger with progress tracking
   */
  private async generateAccountsForLedger(ledgerId: string, count: number): Promise<void> {
    try {
      this.logger.debug(`Generating ${count} accounts for ledger ${ledgerId}`);
      await this.accountGenerator.generate(count, ledgerId);
    } catch (error) {
      this.logger.error(`Failed to generate accounts for ledger ${ledgerId}`, error as Error);
    }
  }

  /**
   * Generate transactions for a ledger with progress tracking
   */
  private async generateTransactionsForLedger(ledgerId: string, transactionsPerAccount: number): Promise<void> {
    try {
      this.logger.debug(`Generating transactions for ledger ${ledgerId}`);
      await this.transactionGenerator.generate(transactionsPerAccount, ledgerId);
    } catch (error) {
      this.logger.error(`Failed to generate transactions for ledger ${ledgerId}`, error as Error);
    }
  }

  /**
   * Print summary statistics
   */
  private printSummary(): void {
    const stats = this.stateManager.getStatistics();
    
    this.logger.info('\n=== Generation Summary ===');
    this.logger.info(`Organizations: ${stats.organizationCount}`);
    this.logger.info(`Ledgers: ${stats.ledgerCount}`);
    this.logger.info(`Assets: ${stats.assetCount}`);
    this.logger.info(`Portfolios: ${stats.portfolioCount}`);
    this.logger.info(`Segments: ${stats.segmentCount}`);
    this.logger.info(`Accounts: ${stats.accountCount}`);
    this.logger.info(`Transactions: ${stats.transactionCount}`);
    
    if (stats.errorCount > 0) {
      this.logger.warn(`\nErrors encountered: ${stats.errorCount}`);
      const errorsByType = this.stateManager.getErrorsByType();
      Object.entries(errorsByType).forEach(([type, count]) => {
        if (count > 0) {
          this.logger.warn(`  ${type}: ${count} errors`);
        }
      });
    }
  }
}