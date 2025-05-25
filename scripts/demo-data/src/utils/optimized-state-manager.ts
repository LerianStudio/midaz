/**
 * Memory-optimized state manager
 * Extends the base StateManager with memory management capabilities
 */

import { GENERATOR_CONFIG } from '../config/generator-config';
import { StateManager } from './state';
import { GeneratorState } from '../types';

export interface MemoryStats {
  totalEntities: number;
  estimatedMemoryMB: number;
  entitiesByType: Record<string, number>;
  oldestEntityAge: number;
  compactionNeeded: boolean;
}

export class OptimizedStateManager extends StateManager {
  private maxEntitiesInMemory = GENERATOR_CONFIG.memory.maxEntitiesInMemory;
  private compactionThreshold = GENERATOR_CONFIG.memory.compactionThreshold;
  private entityTimestamps = new Map<string, number>();
  private lastCompactionTime = Date.now();

  /**
   * Override parent methods to add memory tracking
   */
  addOrganizationId(id: string): void {
    super.addOrganizationId(id);
    this.trackEntity('organization', id);
    this.checkMemoryUsage();
  }

  addLedgerId(orgId: string, ledgerId: string): void {
    super.addLedgerId(orgId, ledgerId);
    this.trackEntity('ledger', ledgerId);
    this.checkMemoryUsage();
  }

  addAssetId(ledgerId: string, assetId: string, assetCode: string): void {
    super.addAssetId(ledgerId, assetId, assetCode);
    this.trackEntity('asset', assetId);
    this.checkMemoryUsage();
  }

  addPortfolioId(ledgerId: string, portfolioId: string): void {
    super.addPortfolioId(ledgerId, portfolioId);
    this.trackEntity('portfolio', portfolioId);
    this.checkMemoryUsage();
  }

  addSegmentId(ledgerId: string, segmentId: string): void {
    super.addSegmentId(ledgerId, segmentId);
    this.trackEntity('segment', segmentId);
    this.checkMemoryUsage();
  }

  addAccountId(ledgerId: string, accountId: string, accountAlias?: string): void {
    super.addAccountId(ledgerId, accountId, accountAlias);
    this.trackEntity('account', accountId);
    this.checkMemoryUsage();
  }

  addTransactionId(ledgerId: string, transactionId: string): void {
    super.addTransactionId(ledgerId, transactionId);
    this.trackEntity('transaction', transactionId);
    this.checkMemoryUsage();
  }

  /**
   * Track entity creation time for LRU eviction
   */
  private trackEntity(type: string, id: string): void {
    const key = `${type}:${id}`;
    this.entityTimestamps.set(key, Date.now());
  }

  /**
   * Check memory usage and compact if needed
   */
  private checkMemoryUsage(): void {
    const totalEntities = this.getTotalEntityCount();
    
    if (totalEntities > this.maxEntitiesInMemory * this.compactionThreshold) {
      this.compactOldestEntities();
    }
  }

  /**
   * Get total entity count across all types
   */
  getTotalEntityCount(): number {
    const state = this.getState();
    let count = 0;

    count += state.organizationIds.length;
    state.ledgerIds.forEach(ledgers => count += ledgers.length);
    state.assetIds.forEach(assets => count += assets.length);
    state.portfolioIds.forEach(portfolios => count += portfolios.length);
    state.segmentIds.forEach(segments => count += segments.length);
    state.accountIds.forEach(accounts => count += accounts.length);
    state.transactionIds.forEach(transactions => count += transactions.length);

    return count;
  }

  /**
   * Compact oldest entities to free memory
   */
  compactOldestEntities(): void {
    const now = Date.now();
    const timeSinceLastCompaction = now - this.lastCompactionTime;
    
    // Don't compact too frequently
    if (timeSinceLastCompaction < 5000) {
      return;
    }

    const totalEntities = this.getTotalEntityCount();
    const entitiesToRemove = Math.floor(totalEntities * 0.2); // Remove 20% of entities
    
    // Sort entities by age
    const sortedEntities = Array.from(this.entityTimestamps.entries())
      .sort((a, b) => a[1] - b[1])
      .slice(0, entitiesToRemove);

    // Remove oldest entities
    sortedEntities.forEach(([key]) => {
      const [type, id] = key.split(':');
      this.removeEntity(type, id);
      this.entityTimestamps.delete(key);
    });

    this.lastCompactionTime = now;
    
    const logger = (globalThis as any).logger;
    if (logger) {
      logger.debug(
        `Memory compaction: removed ${entitiesToRemove} entities, ` +
        `${this.getTotalEntityCount()} remaining`
      );
    }
  }

  /**
   * Remove an entity from state
   */
  private removeEntity(type: string, id: string): void {
    const state = this.getState();

    switch (type) {
      case 'transaction':
        // Find and remove transaction ID from all ledgers
        state.transactionIds.forEach((transactions, ledgerId) => {
          const index = transactions.indexOf(id);
          if (index !== -1) {
            transactions.splice(index, 1);
          }
        });
        break;

      case 'account':
        // Remove account from all ledgers
        state.accountIds.forEach((accounts, ledgerId) => {
          const index = accounts.indexOf(id);
          if (index !== -1) {
            accounts.splice(index, 1);
            // Also remove from aliases
            const aliases = state.accountAliases.get(ledgerId);
            if (aliases && aliases[index]) {
              aliases.splice(index, 1);
            }
          }
        });
        break;

      // For other entity types, we typically don't remove them as they're
      // needed for relationships. Only remove transaction and account data
      // which takes up the most memory.
    }
  }

  /**
   * Get memory statistics
   */
  getMemoryStats(): MemoryStats {
    const state = this.getState();
    const totalEntities = this.getTotalEntityCount();
    
    // Estimate memory usage (rough approximation)
    // Each ID string ~50 bytes, plus overhead
    const estimatedMemoryMB = (totalEntities * 100) / (1024 * 1024);

    // Count entities by type
    const entitiesByType: Record<string, number> = {
      organizations: state.organizationIds.length,
      ledgers: Array.from(state.ledgerIds.values()).reduce((sum, arr) => sum + arr.length, 0),
      assets: Array.from(state.assetIds.values()).reduce((sum, arr) => sum + arr.length, 0),
      portfolios: Array.from(state.portfolioIds.values()).reduce((sum, arr) => sum + arr.length, 0),
      segments: Array.from(state.segmentIds.values()).reduce((sum, arr) => sum + arr.length, 0),
      accounts: Array.from(state.accountIds.values()).reduce((sum, arr) => sum + arr.length, 0),
      transactions: Array.from(state.transactionIds.values()).reduce((sum, arr) => sum + arr.length, 0),
    };

    // Find oldest entity
    let oldestEntityAge = 0;
    if (this.entityTimestamps.size > 0) {
      const oldestTimestamp = Math.min(...this.entityTimestamps.values());
      oldestEntityAge = Date.now() - oldestTimestamp;
    }

    return {
      totalEntities,
      estimatedMemoryMB,
      entitiesByType,
      oldestEntityAge,
      compactionNeeded: totalEntities > this.maxEntitiesInMemory * this.compactionThreshold,
    };
  }

  /**
   * Create a snapshot of current state
   */
  createSnapshot(): GeneratorState {
    return {
      organizationIds: [...this.getState().organizationIds],
      ledgerIds: new Map(this.getState().ledgerIds),
      assetIds: new Map(this.getState().assetIds),
      assetCodes: new Map(this.getState().assetCodes),
      portfolioIds: new Map(this.getState().portfolioIds),
      segmentIds: new Map(this.getState().segmentIds),
      accountIds: new Map(this.getState().accountIds),
      accountAliases: new Map(this.getState().accountAliases),
      transactionIds: new Map(this.getState().transactionIds),
      accountAssets: new Map(
        Array.from(this.getState().accountAssets.entries()).map(([k, v]) => [k, new Map(v)])
      ),
    };
  }

  /**
   * Restore from a snapshot
   */
  restoreFromSnapshot(snapshot: GeneratorState): void {
    this.reset();
    const state = this.getState();

    // Restore all data
    snapshot.organizationIds.forEach(id => this.addOrganizationId(id));
    
    snapshot.ledgerIds.forEach((ledgers, orgId) => {
      ledgers.forEach(ledgerId => this.addLedgerId(orgId, ledgerId));
    });

    snapshot.assetIds.forEach((assets, ledgerId) => {
      const assetCodes = snapshot.assetCodes.get(ledgerId) || [];
      assets.forEach((assetId, index) => {
        this.addAssetId(ledgerId, assetId, assetCodes[index]);
      });
    });

    snapshot.portfolioIds.forEach((portfolios, ledgerId) => {
      portfolios.forEach(portfolioId => this.addPortfolioId(ledgerId, portfolioId));
    });

    snapshot.segmentIds.forEach((segments, ledgerId) => {
      segments.forEach(segmentId => this.addSegmentId(ledgerId, segmentId));
    });

    snapshot.accountIds.forEach((accounts, ledgerId) => {
      const aliases = snapshot.accountAliases.get(ledgerId) || [];
      accounts.forEach((accountId, index) => {
        this.addAccountId(ledgerId, accountId, aliases[index]);
      });
    });

    snapshot.transactionIds.forEach((transactions, ledgerId) => {
      transactions.forEach(transactionId => this.addTransactionId(ledgerId, transactionId));
    });

    snapshot.accountAssets.forEach((accountAssets, ledgerId) => {
      accountAssets.forEach((assetCode, accountId) => {
        this.setAccountAsset(ledgerId, accountId, assetCode);
      });
    });
  }

  /**
   * Clear old transaction data (transactions are usually the largest dataset)
   */
  clearOldTransactions(keepPercentage: number = 0.1): void {
    const state = this.getState();
    
    state.transactionIds.forEach((transactions, ledgerId) => {
      const keepCount = Math.floor(transactions.length * keepPercentage);
      const toRemove = transactions.length - keepCount;
      
      if (toRemove > 0) {
        // Remove oldest transactions (assuming they're added chronologically)
        const removed = transactions.splice(0, toRemove);
        
        // Also remove from timestamps
        removed.forEach(id => {
          this.entityTimestamps.delete(`transaction:${id}`);
        });
      }
    });
  }
}