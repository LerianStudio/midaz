/**
 * Orchestration Service
 * Manages the overall generation flow with separation of concerns
 */

import { Container, ServiceTokens } from '../container/container';
import { GENERATOR_CONFIG, VOLUME_METRICS } from '../config/generator-config';
import { GeneratorOptions, VolumeSize } from '../types';
import { Logger } from './logger';
import { StateManager } from '../utils/state';
import { CheckpointManager, GenerationProgress } from '../utils/checkpoint-manager';
import { PerformanceReporter } from '../monitoring/performance-reporter';
import { InternalPluginManager } from '../plugins/internal/plugin-manager';

// Import generators
import { OrganizationGenerator } from '../generators/organizations';
import { LedgerGenerator } from '../generators/ledgers';
import { AssetGenerator } from '../generators/assets';
import { PortfolioGenerator } from '../generators/portfolios';
import { SegmentGenerator } from '../generators/segments';
import { AccountGenerator } from '../generators/accounts';
import { TransactionGenerator } from '../generators/transactions';

export interface OrchestrationResult {
  success: boolean;
  metrics: any;
  summary?: any;
  error?: Error;
}

export class OrchestrationService {
  private logger: Logger;
  private stateManager: StateManager;
  private checkpointManager: CheckpointManager;
  private performanceReporter: PerformanceReporter;
  private pluginManager: InternalPluginManager;
  
  // Entity generators
  private organizationGenerator: OrganizationGenerator;
  private ledgerGenerator: LedgerGenerator;
  private assetGenerator: AssetGenerator;
  private portfolioGenerator: PortfolioGenerator;
  private segmentGenerator: SegmentGenerator;
  private accountGenerator: AccountGenerator;
  private transactionGenerator: TransactionGenerator;

  constructor(private container: Container, private options: GeneratorOptions) {
    // Resolve services from container
    this.logger = container.resolve<Logger>(ServiceTokens.Logger);
    this.stateManager = container.resolve<StateManager>(ServiceTokens.StateManager);
    this.checkpointManager = container.resolve<CheckpointManager>(ServiceTokens.CheckpointManager);
    this.performanceReporter = container.resolve<PerformanceReporter>(ServiceTokens.PerformanceReporter);
    this.pluginManager = container.resolve<InternalPluginManager>(ServiceTokens.PluginManager);

    // Resolve generators
    this.organizationGenerator = container.resolve<OrganizationGenerator>(ServiceTokens.OrganizationGenerator);
    this.ledgerGenerator = container.resolve<LedgerGenerator>(ServiceTokens.LedgerGenerator);
    this.assetGenerator = container.resolve<AssetGenerator>(ServiceTokens.AssetGenerator);
    this.portfolioGenerator = container.resolve<PortfolioGenerator>(ServiceTokens.PortfolioGenerator);
    this.segmentGenerator = container.resolve<SegmentGenerator>(ServiceTokens.SegmentGenerator);
    this.accountGenerator = container.resolve<AccountGenerator>(ServiceTokens.AccountGenerator);
    this.transactionGenerator = container.resolve<TransactionGenerator>(ServiceTokens.TransactionGenerator);
  }

  /**
   * Orchestrate the entire generation process
   */
  async orchestrateGeneration(): Promise<OrchestrationResult> {
    try {
      // Initialize plugins
      await this.pluginManager.initialize();

      // Check for existing checkpoint
      const checkpoint = await this.checkpointManager.loadLatestCheckpoint();
      
      if (checkpoint) {
        this.logger.info('📌 Found checkpoint, resuming generation...');
        return await this.resumeFromCheckpoint(checkpoint);
      }

      // Start fresh generation
      return await this.startNewGeneration();
    } catch (error) {
      this.logger.error('Orchestration failed', error as Error);
      
      // Save checkpoint on failure
      await this.saveCheckpoint('error');
      
      return {
        success: false,
        metrics: this.stateManager.getMetrics(),
        error: error as Error,
      };
    }
  }

  /**
   * Start a new generation from scratch
   */
  private async startNewGeneration(): Promise<OrchestrationResult> {
    this.logger.info(`Starting data generation with volume: ${this.options.volume}`);
    
    // Reset state
    this.stateManager.reset();
    
    // Get volume metrics
    const volumeMetrics = VOLUME_METRICS[this.options.volume];
    
    // Notify plugins
    await this.pluginManager.beforeGeneration({
      volume: this.options.volume,
      volumeMetrics,
    });

    try {
      // Phase 1: Organizations
      await this.generateOrganizations(volumeMetrics.organizations);
      await this.saveCheckpoint('organizations');

      // Phase 2: For each organization, generate nested entities
      const organizations = this.stateManager.getOrganizationIds();
      
      for (let orgIndex = 0; orgIndex < organizations.length; orgIndex++) {
        const orgId = organizations[orgIndex];
        
        await this.generateOrganizationEntities(
          orgId,
          volumeMetrics,
          orgIndex,
          organizations.length
        );
      }

      // Complete generation
      const metrics = this.stateManager.completeGeneration();
      
      // Notify plugins
      await this.pluginManager.afterGeneration(metrics);
      
      // Generate and print performance summary
      const summary = this.performanceReporter.generateSummary(metrics);
      this.performanceReporter.printSummary(summary);
      
      // Cleanup old checkpoints
      await this.checkpointManager.cleanupOldCheckpoints();

      return {
        success: true,
        metrics,
        summary,
      };
    } catch (error) {
      this.logger.error('Generation failed', error as Error);
      throw error;
    }
  }

  /**
   * Resume generation from a checkpoint
   */
  private async resumeFromCheckpoint(checkpoint: any): Promise<OrchestrationResult> {
    // Restore state
    const restoredState = this.checkpointManager.restoreState(checkpoint);
    this.stateManager.restoreFromCheckpoint(restoredState);
    
    // Notify plugins
    await this.pluginManager.onRestore(checkpoint.id);
    
    // Determine where to resume
    const resumePoint = this.checkpointManager.determineResumePoint(checkpoint);
    
    this.logger.info(
      `Resuming from phase: ${resumePoint.currentPhase}, ` +
      `skipping ${resumePoint.skipOrganizations} organizations`
    );

    try {
      const volumeMetrics = VOLUME_METRICS[this.options.volume];
      const organizations = this.stateManager.getOrganizationIds();
      
      // Continue from where we left off
      for (let orgIndex = resumePoint.skipOrganizations; orgIndex < organizations.length; orgIndex++) {
        const orgId = organizations[orgIndex];
        
        await this.generateOrganizationEntities(
          orgId,
          volumeMetrics,
          orgIndex,
          organizations.length,
          resumePoint.skipLedgers.get(orgId) || 0
        );
      }

      // Complete generation
      const metrics = this.stateManager.completeGeneration();
      
      // Notify plugins
      await this.pluginManager.afterGeneration(metrics);
      
      // Generate and print performance summary
      const summary = this.performanceReporter.generateSummary(metrics);
      this.performanceReporter.printSummary(summary);

      return {
        success: true,
        metrics,
        summary,
      };
    } catch (error) {
      this.logger.error('Resume failed', error as Error);
      throw error;
    }
  }

  /**
   * Generate organizations
   */
  private async generateOrganizations(count: number): Promise<void> {
    this.logger.info(`Generating ${count} organizations...`);
    
    await this.pluginManager.beforeEntityGeneration('organization', { count });
    
    const organizations = await this.organizationGenerator.generate(count);
    
    for (const org of organizations) {
      await this.pluginManager.afterEntityGeneration({
        type: 'organization',
        entity: org,
      });
    }
  }

  /**
   * Generate all entities for an organization
   */
  private async generateOrganizationEntities(
    orgId: string,
    volumeMetrics: any,
    orgIndex: number,
    totalOrgs: number,
    skipLedgers: number = 0
  ): Promise<void> {
    this.logger.info(
      `Processing organization ${orgIndex + 1}/${totalOrgs}: ${orgId}`
    );

    // Generate ledgers
    const ledgers = await this.ledgerGenerator.generate(
      volumeMetrics.ledgersPerOrg,
      orgId
    );

    // Process each ledger
    const ledgerIds = this.stateManager.getLedgerIds(orgId);
    
    for (let ledgerIndex = skipLedgers; ledgerIndex < ledgerIds.length; ledgerIndex++) {
      const ledgerId = ledgerIds[ledgerIndex];
      
      await this.generateLedgerEntities(
        ledgerId,
        orgId,
        volumeMetrics,
        ledgerIndex,
        ledgerIds.length
      );
      
      // Save checkpoint after each ledger
      await this.saveCheckpoint('ledgers', orgIndex, ledgerIndex);
    }
  }

  /**
   * Generate all entities for a ledger
   */
  private async generateLedgerEntities(
    ledgerId: string,
    orgId: string,
    volumeMetrics: any,
    ledgerIndex: number,
    totalLedgers: number
  ): Promise<void> {
    this.logger.info(
      `Processing ledger ${ledgerIndex + 1}/${totalLedgers}: ${ledgerId}`
    );

    // Generate assets (critical for accounts)
    await this.pluginManager.beforeEntityGeneration('asset', { 
      count: volumeMetrics.assetsPerLedger,
      ledgerId,
    });
    
    const assets = await this.assetGenerator.generate(
      volumeMetrics.assetsPerLedger,
      ledgerId,
      orgId
    );

    // Generate portfolios (optional)
    await this.portfolioGenerator.generate(
      volumeMetrics.portfoliosPerLedger,
      ledgerId,
      orgId
    );

    // Generate segments (optional)
    await this.segmentGenerator.generate(
      volumeMetrics.segmentsPerLedger,
      ledgerId,
      orgId
    );

    // Generate accounts
    const accounts = await this.accountGenerator.generate(
      volumeMetrics.accountsPerLedger,
      ledgerId,
      orgId
    );

    // Generate transactions if accounts exist
    if (accounts.length > 0) {
      await this.transactionGenerator.generate(
        volumeMetrics.transactionsPerAccount,
        ledgerId,
        orgId
      );
    }
  }

  /**
   * Save a checkpoint
   */
  private async saveCheckpoint(
    phase: string,
    orgIndex: number = 0,
    ledgerIndex: number = 0
  ): Promise<void> {
    try {
      const progress: GenerationProgress = {
        phase: phase as any,
        currentOrganizationIndex: orgIndex,
        currentLedgerIndex: ledgerIndex,
        completedSteps: this.getCompletedSteps(),
        failedSteps: this.getFailedSteps(),
      };

      const checkpoint = this.checkpointManager.createCheckpoint(
        `gen-${Date.now()}`,
        this.stateManager.getState(),
        progress,
        {
          volume: this.options.volume,
          organizations: VOLUME_METRICS[this.options.volume].organizations,
          ledgersPerOrg: VOLUME_METRICS[this.options.volume].ledgersPerOrg,
          assetsPerLedger: VOLUME_METRICS[this.options.volume].assetsPerLedger,
          portfoliosPerLedger: VOLUME_METRICS[this.options.volume].portfoliosPerLedger,
          segmentsPerLedger: VOLUME_METRICS[this.options.volume].segmentsPerLedger,
          accountsPerLedger: VOLUME_METRICS[this.options.volume].accountsPerLedger,
          transactionsPerAccount: VOLUME_METRICS[this.options.volume].transactionsPerAccount,
        },
        this.stateManager.getMetrics()
      );

      await this.checkpointManager.saveCheckpoint(checkpoint);
      await this.pluginManager.onCheckpoint(checkpoint.id);
    } catch (error) {
      this.logger.warn('Failed to save checkpoint', error as Error);
    }
  }

  /**
   * Get completed steps for checkpoint
   */
  private getCompletedSteps(): string[] {
    const steps: string[] = [];
    const state = this.stateManager.getState();
    
    if (state.organizationIds.length > 0) steps.push('organizations');
    if (state.ledgerIds.size > 0) steps.push('ledgers');
    if (state.assetIds.size > 0) steps.push('assets');
    if (state.portfolioIds.size > 0) steps.push('portfolios');
    if (state.segmentIds.size > 0) steps.push('segments');
    if (state.accountIds.size > 0) steps.push('accounts');
    if (state.transactionIds.size > 0) steps.push('transactions');
    
    return steps;
  }

  /**
   * Get failed steps for checkpoint
   */
  private getFailedSteps(): string[] {
    const steps: string[] = [];
    const metrics = this.stateManager.getMetrics();
    
    if (metrics.organizationErrors && metrics.organizationErrors > 0) steps.push('organizations');
    if (metrics.ledgerErrors && metrics.ledgerErrors > 0) steps.push('ledgers');
    if (metrics.assetErrors && metrics.assetErrors > 0) steps.push('assets');
    if (metrics.portfolioErrors && metrics.portfolioErrors > 0) steps.push('portfolios');
    if (metrics.segmentErrors && metrics.segmentErrors > 0) steps.push('segments');
    if (metrics.accountErrors && metrics.accountErrors > 0) steps.push('accounts');
    if (metrics.transactionErrors && metrics.transactionErrors > 0) steps.push('transactions');
    
    return steps;
  }
}