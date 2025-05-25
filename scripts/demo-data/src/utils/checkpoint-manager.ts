/**
 * Checkpoint Manager for resume capability
 * Saves and restores generation state to enable resuming after failures
 */



import * as fs from 'fs/promises';
import * as path from 'path';
import { Logger } from '../services/logger';
import { GeneratorState, GeneratorConfig, GenerationMetrics } from '../types';

export interface Checkpoint {
  id: string;
  timestamp: Date;
  state: GeneratorState;
  progress: GenerationProgress;
  config: CheckpointConfig;
  metrics: Partial<GenerationMetrics>;
}

export interface GenerationProgress {
  phase: 'organizations' | 'ledgers' | 'assets' | 'portfolios' | 'segments' | 'accounts' | 'transactions';
  currentOrganizationIndex: number;
  currentLedgerIndex: number;
  completedSteps: string[];
  failedSteps: string[];
}

export interface CheckpointConfig {
  volume: string;
  organizations: number;
  ledgersPerOrg: number;
  assetsPerLedger: number;
  portfoliosPerLedger: number;
  segmentsPerLedger: number;
  accountsPerLedger: number;
  transactionsPerAccount: number;
}

export class CheckpointManager {
  constructor(
    private logger: Logger,
    private checkpointDir: string = './checkpoints'
  ) {}

  /**
   * Ensure checkpoint directory exists
   */
  private async ensureCheckpointDir(): Promise<void> {
    try {
      await fs.access(this.checkpointDir);
    } catch {
      await fs.mkdir(this.checkpointDir, { recursive: true });
      this.logger.debug(`Created checkpoint directory: ${this.checkpointDir}`);
    }
  }

  /**
   * Save a checkpoint
   */
  async saveCheckpoint(checkpoint: Checkpoint): Promise<void> {
    await this.ensureCheckpointDir();

    const filename = this.generateCheckpointFilename(checkpoint.id, checkpoint.timestamp);
    const filepath = path.join(this.checkpointDir, filename);

    try {
      await fs.writeFile(filepath, JSON.stringify(checkpoint, null, 2), 'utf-8');
      this.logger.info(`Checkpoint saved: ${filename}`);
    } catch (error) {
      this.logger.error('Failed to save checkpoint', error as Error);
      throw error;
    }
  }

  /**
   * Load the latest checkpoint
   */
  async loadLatestCheckpoint(): Promise<Checkpoint | null> {
    try {
      await this.ensureCheckpointDir();
      const files = await fs.readdir(this.checkpointDir);
      
      const checkpointFiles = files
        .filter((f: string) => f.startsWith('checkpoint-') && f.endsWith('.json'))
        .sort()
        .reverse();

      if (checkpointFiles.length === 0) {
        this.logger.debug('No checkpoints found');
        return null;
      }

      const latestFile = checkpointFiles[0];
      const filepath = path.join(this.checkpointDir, latestFile);
      
      const content = await fs.readFile(filepath, 'utf-8');
      const checkpoint = JSON.parse(content) as Checkpoint;
      
      // Convert date strings back to Date objects
      checkpoint.timestamp = new Date(checkpoint.timestamp);
      if (checkpoint.metrics.startTime) {
        checkpoint.metrics.startTime = new Date(checkpoint.metrics.startTime);
      }
      if (checkpoint.metrics.endTime) {
        checkpoint.metrics.endTime = new Date(checkpoint.metrics.endTime);
      }

      this.logger.info(`Loaded checkpoint: ${latestFile}`);
      return checkpoint;
    } catch (error) {
      this.logger.error('Failed to load checkpoint', error as Error);
      return null;
    }
  }

  /**
   * Load a specific checkpoint by ID
   */
  async loadCheckpoint(checkpointId: string): Promise<Checkpoint | null> {
    try {
      await this.ensureCheckpointDir();
      const files = await fs.readdir(this.checkpointDir);
      
      const checkpointFile = files.find((f: string) => 
        f.startsWith(`checkpoint-${checkpointId}-`) && f.endsWith('.json')
      );

      if (!checkpointFile) {
        this.logger.warn(`Checkpoint not found: ${checkpointId}`);
        return null;
      }

      const filepath = path.join(this.checkpointDir, checkpointFile);
      const content = await fs.readFile(filepath, 'utf-8');
      const checkpoint = JSON.parse(content) as Checkpoint;
      
      // Convert date strings back to Date objects
      checkpoint.timestamp = new Date(checkpoint.timestamp);
      if (checkpoint.metrics.startTime) {
        checkpoint.metrics.startTime = new Date(checkpoint.metrics.startTime);
      }
      if (checkpoint.metrics.endTime) {
        checkpoint.metrics.endTime = new Date(checkpoint.metrics.endTime);
      }

      return checkpoint;
    } catch (error) {
      this.logger.error(`Failed to load checkpoint ${checkpointId}`, error as Error);
      return null;
    }
  }

  /**
   * List all available checkpoints
   */
  async listCheckpoints(): Promise<Array<{ id: string; timestamp: Date; filename: string }>> {
    try {
      await this.ensureCheckpointDir();
      const files = await fs.readdir(this.checkpointDir);
      
      return files
        .filter((f: string) => f.startsWith('checkpoint-') && f.endsWith('.json'))
        .map((filename: string) => {
          const match = filename.match(/checkpoint-(.+)-(\d+)\.json/);
          if (!match) return null;
          
          return {
            id: match[1],
            timestamp: new Date(parseInt(match[2])),
            filename,
          };
        })
        .filter((item): item is NonNullable<typeof item> => item !== null)
        .sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
    } catch (error) {
      this.logger.error('Failed to list checkpoints', error as Error);
      return [];
    }
  }

  /**
   * Delete old checkpoints (keep only the most recent N)
   */
  async cleanupOldCheckpoints(keepCount: number = 5): Promise<void> {
    try {
      const checkpoints = await this.listCheckpoints();
      
      if (checkpoints.length <= keepCount) {
        return;
      }

      const toDelete = checkpoints.slice(keepCount);
      
      for (const checkpoint of toDelete) {
        const filepath = path.join(this.checkpointDir, checkpoint.filename);
        await fs.unlink(filepath);
        this.logger.debug(`Deleted old checkpoint: ${checkpoint.filename}`);
      }

      this.logger.info(`Cleaned up ${toDelete.length} old checkpoints`);
    } catch (error) {
      this.logger.error('Failed to cleanup checkpoints', error as Error);
    }
  }

  /**
   * Create a checkpoint from current state
   */
  createCheckpoint(
    id: string,
    state: GeneratorState,
    progress: GenerationProgress,
    config: CheckpointConfig,
    metrics: Partial<GenerationMetrics>
  ): Checkpoint {
    return {
      id,
      timestamp: new Date(),
      state: this.serializeState(state),
      progress,
      config,
      metrics,
    };
  }

  /**
   * Serialize state for storage
   */
  private serializeState(state: GeneratorState): GeneratorState {
    return {
      organizationIds: [...state.organizationIds],
      ledgerIds: this.mapToObject(state.ledgerIds),
      assetIds: this.mapToObject(state.assetIds),
      assetCodes: this.mapToObject(state.assetCodes),
      portfolioIds: this.mapToObject(state.portfolioIds),
      segmentIds: this.mapToObject(state.segmentIds),
      accountIds: this.mapToObject(state.accountIds),
      accountAliases: this.mapToObject(state.accountAliases),
      transactionIds: this.mapToObject(state.transactionIds),
      accountAssets: this.mapToObject(
        state.accountAssets,
        (innerMap) => this.mapToObject(innerMap as Map<string, string>)
      ),
    } as any;
  }

  /**
   * Convert Map to plain object for JSON serialization
   */
  private mapToObject<V>(
    map: Map<string, V>,
    valueTransformer?: (value: V) => any
  ): Record<string, V> {
    const obj: Record<string, any> = {};
    map.forEach((value, key) => {
      obj[key] = valueTransformer ? valueTransformer(value) : value;
    });
    return obj;
  }

  /**
   * Restore state from checkpoint
   */
  restoreState(checkpoint: Checkpoint): GeneratorState {
    const state = checkpoint.state as any;
    
    return {
      organizationIds: state.organizationIds,
      ledgerIds: this.objectToMap(state.ledgerIds),
      assetIds: this.objectToMap(state.assetIds),
      assetCodes: this.objectToMap(state.assetCodes),
      portfolioIds: this.objectToMap(state.portfolioIds),
      segmentIds: this.objectToMap(state.segmentIds),
      accountIds: this.objectToMap(state.accountIds),
      accountAliases: this.objectToMap(state.accountAliases),
      transactionIds: this.objectToMap(state.transactionIds),
      accountAssets: this.objectToMap(
        state.accountAssets,
        (innerObj) => this.objectToMap(innerObj as Record<string, unknown>)
      ),
    };
  }

  /**
   * Convert plain object to Map
   */
  private objectToMap<V>(
    obj: Record<string, V>,
    valueTransformer?: (value: V) => any
  ): Map<string, any> {
    const map = new Map<string, any>();
    Object.entries(obj).forEach(([key, value]) => {
      map.set(key, valueTransformer ? valueTransformer(value) : value);
    });
    return map;
  }

  /**
   * Generate checkpoint filename
   */
  private generateCheckpointFilename(id: string, timestamp: Date): string {
    return `checkpoint-${id}-${timestamp.getTime()}.json`;
  }

  /**
   * Determine what needs to be resumed based on checkpoint
   */
  determineResumePoint(checkpoint: Checkpoint): {
    skipOrganizations: number;
    skipLedgers: Map<string, number>;
    currentPhase: GenerationProgress['phase'];
  } {
    const { progress, state } = checkpoint;
    
    // Create a map of how many ledgers to skip per organization
    const skipLedgers = new Map<string, number>();
    state.organizationIds.forEach((orgId) => {
      const ledgers = (state.ledgerIds as any)[orgId] || [];
      skipLedgers.set(orgId, ledgers.length);
    });

    return {
      skipOrganizations: progress.currentOrganizationIndex,
      skipLedgers,
      currentPhase: progress.phase,
    };
  }
}