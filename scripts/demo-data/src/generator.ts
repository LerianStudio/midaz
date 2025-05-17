/**
 * Main generator orchestration
 */

import { MidazClient } from 'midaz-sdk/src';
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

  constructor(options: GeneratorOptions) {
    this.options = options;
    this.logger = new Logger(options);

    // Initialize client
    this.client = initializeClient(options);
    this.logger.info(
      `Initialized Midaz client connecting to ${options.baseUrl}:${options.onboardingPort} and ${options.baseUrl}:${options.transactionPort}`
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
   * Run the generator with the provided options
   */
  public async run(): Promise<void> {
    this.logger.info(`Starting data generation with volume: ${this.options.volume}`);

    try {
      // Get the metrics for the selected volume
      const volumeMetrics = VOLUME_METRICS[this.options.volume];

      // Generate organizations
      const organizations = await this.organizationGenerator.generate(volumeMetrics.organizations);

      // For each organization, generate ledgers and their nested entities
      for (const org of organizations) {
        this.logger.info(`Generating data for organization: ${org.id} (${org.legalName})`);

        // Generate ledgers for this organization
        const ledgers = await this.ledgerGenerator.generate(volumeMetrics.ledgersPerOrg, org.id);

        // For each ledger, generate assets, portfolios, segments, accounts, and transactions
        for (const ledger of ledgers) {
          this.logger.info(`Generating data for ledger: ${ledger.id} (${ledger.name})`);

          // Generate assets for this ledger
          await this.assetGenerator.generate(volumeMetrics.assetsPerLedger, ledger.id);

          // Generate portfolios for this ledger
          await this.portfolioGenerator.generate(volumeMetrics.portfoliosPerLedger, ledger.id);

          // Generate segments for this ledger
          await this.segmentGenerator.generate(volumeMetrics.segmentsPerLedger, ledger.id);

          // Generate accounts for this ledger
          await this.accountGenerator.generate(volumeMetrics.accountsPerLedger, ledger.id);

          // Generate transactions for this ledger
          await this.transactionGenerator.generate(volumeMetrics.transactionsPerAccount, ledger.id);
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
}
