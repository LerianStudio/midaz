/**
 * Main generator orchestration
 */

import { MidazClient } from '@lerianstudio/midaz-sdk';
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
   * Run the generator with the provided options
   */
  public async run(): Promise<void> {
    this.logger.info(`Starting data generation with volume: ${this.options.volume}`);

    try {
      // Get the metrics for the selected volume
      const volumeMetrics = VOLUME_METRICS[this.options.volume];

      // Generate organizations
      const organizations = await this.organizationGenerator.generate(volumeMetrics.organizations);
      await this.addProcessDelay('organizations');

      // For each organization, generate ledgers
      for (const org of organizations) {
        this.logger.info(`Generating ledgers for organization: ${org.id} (${org.legalName})`);
        await this.ledgerGenerator.generate(volumeMetrics.ledgersPerOrg, org.id);
      }
      await this.addProcessDelay('ledgers');

      // For each organization, process its ledgers
      for (const org of organizations) {
        const ledgers = this.stateManager.getLedgerIds(org.id);
        
        // Generate assets, portfolios, and segments for all ledgers
        for (const ledgerId of ledgers) {
          this.logger.info(`Generating assets, portfolios, and segments for ledger: ${ledgerId}`);
          await this.assetGenerator.generate(volumeMetrics.assetsPerLedger, ledgerId);
          await this.portfolioGenerator.generate(volumeMetrics.portfoliosPerLedger, ledgerId);
          await this.segmentGenerator.generate(volumeMetrics.segmentsPerLedger, ledgerId);
        }
      }
      await this.addProcessDelay('assets, portfolios, and segments');

      // Generate accounts for all ledgers
      for (const org of organizations) {
        const ledgers = this.stateManager.getLedgerIds(org.id);
        for (const ledgerId of ledgers) {
          this.logger.info(`Generating accounts for ledger: ${ledgerId}`);
          await this.accountGenerator.generate(volumeMetrics.accountsPerLedger, ledgerId);
        }
      }
      await this.addProcessDelay('accounts');

      // Generate transactions for all ledgers
      for (const org of organizations) {
        const ledgers = this.stateManager.getLedgerIds(org.id);
        for (const ledgerId of ledgers) {
          this.logger.info(`Generating transactions for ledger: ${ledgerId}`);
          await this.transactionGenerator.generate(volumeMetrics.transactionsPerAccount, ledgerId);
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
