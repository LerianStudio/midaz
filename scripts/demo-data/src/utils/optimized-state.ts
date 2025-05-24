/**
 * Optimized state management with better performance and memory efficiency
 */

import { GenerationMetrics, GeneratorState } from '../types';
import { ErrorTracker, GenerationErrorInfo } from './error-tracker';

export interface StateSnapshot {
  timestamp: Date;
  metrics: GenerationMetrics;
  entityCounts: {
    organizations: number;
    ledgers: number;
    assets: number;
    portfolios: number;
    segments: number;
    accounts: number;
    transactions: number;
  };
  errorCounts: {
    organizations: number;
    ledgers: number;
    assets: number;
    portfolios: number;
    segments: number;
    accounts: number;
    transactions: number;
  };
}

export interface StateManagerOptions {
  /** Maximum number of entities to keep in memory per type */
  maxEntitiesInMemory: number;
  /** Whether to automatically persist state snapshots */
  enableSnapshots: boolean;
  /** Interval for automatic snapshots in milliseconds */
  snapshotInterval: number;
  /** Whether to use memory-optimized mode */
  memoryOptimized: boolean;
}

/**
 * Optimized state manager with memory management and performance improvements
 */
export class OptimizedStateManager {
  private static instance: OptimizedStateManager;
  private errorTracker = new ErrorTracker();
  private state: GeneratorState;
  private snapshots: StateSnapshot[] = [];
  private maxSnapshots = 10;
  private snapshotInterval?: NodeJS.Timeout;

  private metrics: GenerationMetrics = {
    startTime: new Date(),
    totalOrganizations: 0,
    totalLedgers: 0,
    totalAssets: 0,
    totalPortfolios: 0,
    totalSegments: 0,
    totalAccounts: 0,
    totalTransactions: 0,
    organizationErrors: 0,
    ledgerErrors: 0,
    assetErrors: 0,
    portfolioErrors: 0,
    segmentErrors: 0,
    accountErrors: 0,
    transactionErrors: 0,
    errors: 0,
    retries: 0,
    duration: function () {
      const end = this.endTime || new Date();
      return end.getTime() - this.startTime.getTime();
    },
  };

  private readonly options: StateManagerOptions;

  private constructor(options: Partial<StateManagerOptions> = {}) {
    this.options = {
      maxEntitiesInMemory: 10000,
      enableSnapshots: true,
      snapshotInterval: 30000, // 30 seconds
      memoryOptimized: true,
      ...options,
    };

    this.initializeState();
    
    if (this.options.enableSnapshots) {
      this.startSnapshots();
    }
  }

  static getInstance(options?: Partial<StateManagerOptions>): OptimizedStateManager {
    if (!OptimizedStateManager.instance) {
      OptimizedStateManager.instance = new OptimizedStateManager(options);
    }
    return OptimizedStateManager.instance;
  }

  static resetInstance(): void {
    if (OptimizedStateManager.instance?.snapshotInterval) {
      clearInterval(OptimizedStateManager.instance.snapshotInterval);
    }
    OptimizedStateManager.instance = undefined as any;
  }

  private initializeState(): void {
    this.state = {
      organizationIds: [],
      ledgerIds: new Map<string, string[]>(),
      assetIds: new Map<string, string[]>(),
      assetCodes: new Map<string, string[]>(),
      portfolioIds: new Map<string, string[]>(),
      segmentIds: new Map<string, string[]>(),
      accountIds: new Map<string, string[]>(),
      accountAliases: new Map<string, string[]>(),
      transactionIds: new Map<string, string[]>(),
      accountAssets: new Map<string, Map<string, string>>(),
    };
  }

  private startSnapshots(): void {
    if (this.options.snapshotInterval > 0) {
      this.snapshotInterval = setInterval(() => {
        this.createSnapshot();
      }, this.options.snapshotInterval);
    }
  }

  private createSnapshot(): StateSnapshot {
    const snapshot: StateSnapshot = {
      timestamp: new Date(),
      metrics: { ...this.metrics },
      entityCounts: {
        organizations: this.state.organizationIds.length,
        ledgers: Array.from(this.state.ledgerIds.values()).reduce((sum, arr) => sum + arr.length, 0),
        assets: Array.from(this.state.assetIds.values()).reduce((sum, arr) => sum + arr.length, 0),
        portfolios: Array.from(this.state.portfolioIds.values()).reduce((sum, arr) => sum + arr.length, 0),
        segments: Array.from(this.state.segmentIds.values()).reduce((sum, arr) => sum + arr.length, 0),
        accounts: Array.from(this.state.accountIds.values()).reduce((sum, arr) => sum + arr.length, 0),
        transactions: Array.from(this.state.transactionIds.values()).reduce((sum, arr) => sum + arr.length, 0),
      },
      errorCounts: {
        organizations: this.metrics.organizationErrors,
        ledgers: this.metrics.ledgerErrors,
        assets: this.metrics.assetErrors,
        portfolios: this.metrics.portfolioErrors,
        segments: this.metrics.segmentErrors,
        accounts: this.metrics.accountErrors,
        transactions: this.metrics.transactionErrors,
      },
    };

    this.snapshots.push(snapshot);
    
    // Keep only the last maxSnapshots
    if (this.snapshots.length > this.maxSnapshots) {
      this.snapshots.shift();
    }

    return snapshot;
  }

  /**
   * Get historical snapshots
   */
  getSnapshots(): StateSnapshot[] {
    return [...this.snapshots];
  }

  /**
   * Get the latest snapshot
   */
  getLatestSnapshot(): StateSnapshot | undefined {
    return this.snapshots[this.snapshots.length - 1];
  }

  /**
   * Add organization ID
   */
  addOrganizationId(id: string): void {
    if (!this.state.organizationIds.includes(id)) {
      this.state.organizationIds.push(id);
      this.metrics.totalOrganizations++;
    }
  }

  /**
   * Add ledger ID with memory optimization
   */
  addLedgerId(organizationId: string, id: string): void {
    if (!this.state.ledgerIds.has(organizationId)) {
      this.state.ledgerIds.set(organizationId, []);
    }
    
    const ledgers = this.state.ledgerIds.get(organizationId)!;
    if (!ledgers.includes(id)) {
      // Apply memory optimization
      if (this.options.memoryOptimized && ledgers.length >= this.options.maxEntitiesInMemory) {
        // Remove oldest entries to maintain memory limit
        ledgers.shift();
      }
      ledgers.push(id);
      this.metrics.totalLedgers++;
    }
  }

  /**
   * Add asset ID and code with memory optimization
   */
  addAssetId(ledgerId: string, id: string, code: string): void {
    // Add asset ID
    if (!this.state.assetIds.has(ledgerId)) {
      this.state.assetIds.set(ledgerId, []);
    }
    
    const assets = this.state.assetIds.get(ledgerId)!;
    if (!assets.includes(id)) {
      if (this.options.memoryOptimized && assets.length >= this.options.maxEntitiesInMemory) {
        assets.shift();
      }
      assets.push(id);
      this.metrics.totalAssets++;
    }

    // Add asset code
    if (!this.state.assetCodes.has(ledgerId)) {
      this.state.assetCodes.set(ledgerId, []);
    }
    
    const codes = this.state.assetCodes.get(ledgerId)!;
    if (!codes.includes(code)) {
      if (this.options.memoryOptimized && codes.length >= this.options.maxEntitiesInMemory) {
        codes.shift();
      }
      codes.push(code);
    }
  }

  /**
   * Add portfolio ID with memory optimization
   */
  addPortfolioId(ledgerId: string, id: string): void {
    if (!this.state.portfolioIds.has(ledgerId)) {
      this.state.portfolioIds.set(ledgerId, []);
    }
    
    const portfolios = this.state.portfolioIds.get(ledgerId)!;
    if (!portfolios.includes(id)) {
      if (this.options.memoryOptimized && portfolios.length >= this.options.maxEntitiesInMemory) {
        portfolios.shift();
      }
      portfolios.push(id);
      this.metrics.totalPortfolios++;
    }
  }

  /**
   * Add segment ID with memory optimization
   */
  addSegmentId(ledgerId: string, id: string): void {
    if (!this.state.segmentIds.has(ledgerId)) {
      this.state.segmentIds.set(ledgerId, []);
    }
    
    const segments = this.state.segmentIds.get(ledgerId)!;
    if (!segments.includes(id)) {
      if (this.options.memoryOptimized && segments.length >= this.options.maxEntitiesInMemory) {
        segments.shift();
      }
      segments.push(id);
      this.metrics.totalSegments++;
    }
  }

  /**
   * Add account ID and alias with memory optimization
   */
  addAccountId(ledgerId: string, id: string, alias: string): void {
    // Add account ID
    if (!this.state.accountIds.has(ledgerId)) {
      this.state.accountIds.set(ledgerId, []);
    }
    
    const accounts = this.state.accountIds.get(ledgerId)!;
    if (!accounts.includes(id)) {
      if (this.options.memoryOptimized && accounts.length >= this.options.maxEntitiesInMemory) {
        accounts.shift();
      }
      accounts.push(id);
      this.metrics.totalAccounts++;
    }

    // Add account alias
    if (!this.state.accountAliases.has(ledgerId)) {
      this.state.accountAliases.set(ledgerId, []);
    }
    
    const aliases = this.state.accountAliases.get(ledgerId)!;
    if (!aliases.includes(alias)) {
      if (this.options.memoryOptimized && aliases.length >= this.options.maxEntitiesInMemory) {
        aliases.shift();
      }
      aliases.push(alias);
    }
  }

  /**
   * Add transaction ID with memory optimization
   */
  addTransactionId(ledgerId: string, id: string): void {
    if (!this.state.transactionIds.has(ledgerId)) {
      this.state.transactionIds.set(ledgerId, []);
    }
    
    const transactions = this.state.transactionIds.get(ledgerId)!;
    if (!transactions.includes(id)) {
      if (this.options.memoryOptimized && transactions.length >= this.options.maxEntitiesInMemory) {
        transactions.shift();
      }
      transactions.push(id);
      this.metrics.totalTransactions++;
    }
  }

  /**
   * Get organization IDs (optimized access)
   */
  getOrganizationIds(): string[] {
    return [...this.state.organizationIds];
  }

  /**
   * Get ledger IDs for organization
   */
  getLedgerIds(organizationId: string): string[] {
    return [...(this.state.ledgerIds.get(organizationId) || [])];
  }

  /**
   * Get all ledger IDs across all organizations
   */
  getAllLedgerIds(): string[] {
    const allLedgers: string[] = [];
    this.state.ledgerIds.forEach(ledgers => {
      allLedgers.push(...ledgers);
    });
    return allLedgers;
  }

  /**
   * Get asset codes for ledger
   */
  getAssetCodes(ledgerId: string): string[] {
    return [...(this.state.assetCodes.get(ledgerId) || [])];
  }

  /**
   * Get account IDs for ledger
   */
  getAccountIds(ledgerId: string): string[] {
    return [...(this.state.accountIds.get(ledgerId) || [])];
  }

  /**
   * Get account aliases for ledger
   */
  getAccountAliases(ledgerId: string): string[] {
    return [...(this.state.accountAliases.get(ledgerId) || [])];
  }

  /**
   * Get portfolio IDs for ledger
   */
  getPortfolioIds(ledgerId: string): string[] {
    return [...(this.state.portfolioIds.get(ledgerId) || [])];
  }

  /**
   * Track generation error with detailed context
   */
  trackGenerationError(
    entityType: string,
    parentId: string,
    error: Error,
    context?: Record<string, any>
  ): void {
    const errorInfo: GenerationErrorInfo = {
      timestamp: new Date(),
      entityType,
      parentId,
      error: {
        name: error.name,
        message: error.message,
        stack: error.stack,
      },
      context,
    };

    this.errorTracker.trackError(errorInfo);
    this.metrics.errors++;
  }

  /**
   * Increment error count for specific entity type
   */
  incrementErrorCount(entityType: string): void {
    switch (entityType) {
      case 'organization':
        this.metrics.organizationErrors++;
        break;
      case 'ledger':
        this.metrics.ledgerErrors++;
        break;
      case 'asset':
        this.metrics.assetErrors++;
        break;
      case 'portfolio':
        this.metrics.portfolioErrors++;
        break;
      case 'segment':
        this.metrics.segmentErrors++;
        break;
      case 'account':
        this.metrics.accountErrors++;
        break;
      case 'transaction':
        this.metrics.transactionErrors++;
        break;
    }
  }

  /**
   * Get current metrics
   */
  getMetrics(): GenerationMetrics {
    return { ...this.metrics };
  }

  /**
   * Get error tracker instance
   */
  getErrorTracker(): ErrorTracker {
    return this.errorTracker;
  }

  /**
   * Get memory usage statistics
   */
  getMemoryStats(): {
    totalEntities: number;
    memoryMaps: number;
    estimatedMemoryMB: number;
    organizationCount: number;
    ledgerMapSize: number;
    assetMapSize: number;
    accountMapSize: number;
  } {
    const totalEntities = 
      this.state.organizationIds.length +
      Array.from(this.state.ledgerIds.values()).reduce((sum, arr) => sum + arr.length, 0) +
      Array.from(this.state.assetIds.values()).reduce((sum, arr) => sum + arr.length, 0) +
      Array.from(this.state.accountIds.values()).reduce((sum, arr) => sum + arr.length, 0);

    const memoryMaps = 
      this.state.ledgerIds.size +
      this.state.assetIds.size +
      this.state.accountIds.size +
      this.state.portfolioIds.size +
      this.state.segmentIds.size;

    // Rough estimation: each entity ID is ~36 bytes (UUID) + map overhead
    const estimatedMemoryMB = (totalEntities * 50 + memoryMaps * 100) / (1024 * 1024);

    return {
      totalEntities,
      memoryMaps,
      estimatedMemoryMB: Number(estimatedMemoryMB.toFixed(2)),
      organizationCount: this.state.organizationIds.length,
      ledgerMapSize: this.state.ledgerIds.size,
      assetMapSize: this.state.assetIds.size,
      accountMapSize: this.state.accountIds.size,
    };
  }

  /**
   * Clear state to free memory
   */
  clearState(): void {
    this.state.organizationIds = [];
    this.state.ledgerIds.clear();
    this.state.assetIds.clear();
    this.state.assetCodes.clear();
    this.state.portfolioIds.clear();
    this.state.segmentIds.clear();
    this.state.accountIds.clear();
    this.state.accountAliases.clear();
    this.state.transactionIds.clear();
    this.state.accountAssets.clear();
    this.snapshots = [];
    this.errorTracker = new ErrorTracker();
  }

  /**
   * Cleanup and stop snapshots
   */
  cleanup(): void {
    if (this.snapshotInterval) {
      clearInterval(this.snapshotInterval);
      this.snapshotInterval = undefined;
    }
  }
}