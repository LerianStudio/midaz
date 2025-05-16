/**
 * State management for the generator
 * Keeps track of all generated entities and their relationships
 */

import { GenerationMetrics, GeneratorState } from '../types';

/**
 * Generator state singleton
 * Maintains references to all generated entities for relationship linking
 */
export class StateManager {
  private static instance: StateManager;
  private state: GeneratorState = {
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

  private metrics: GenerationMetrics = {
    startTime: new Date(),
    totalOrganizations: 0,
    totalLedgers: 0,
    totalAssets: 0,
    totalPortfolios: 0,
    totalSegments: 0,
    totalAccounts: 0,
    totalTransactions: 0,
    // Add error counters per entity type
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

  /**
   * Get singleton instance
   */
  public static getInstance(): StateManager {
    if (!StateManager.instance) {
      StateManager.instance = new StateManager();
    }
    return StateManager.instance;
  }

  /**
   * Reset state to start fresh
   */
  public reset(): void {
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

    this.metrics = {
      startTime: new Date(),
      totalOrganizations: 0,
      totalLedgers: 0,
      totalAssets: 0,
      totalPortfolios: 0,
      totalSegments: 0,
      totalAccounts: 0,
      totalTransactions: 0,
      // Add error counters per entity type
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
  }

  /**
   * Get current state
   */
  public getState(): GeneratorState {
    return this.state;
  }

  /**
   * Get current metrics
   */
  public getMetrics(): GenerationMetrics {
    return this.metrics;
  }

  /**
   * Mark generation as complete and record end time
   */
  public completeGeneration(): GenerationMetrics {
    this.metrics.endTime = new Date();
    return this.metrics;
  }

  // Organization state methods
  public addOrganizationId(id: string): void {
    this.state.organizationIds.push(id);
    this.metrics.totalOrganizations++;
  }

  public getOrganizationIds(): string[] {
    return this.state.organizationIds;
  }

  // Ledger state methods
  public addLedgerId(orgId: string, ledgerId: string): void {
    if (!this.state.ledgerIds.has(orgId)) {
      this.state.ledgerIds.set(orgId, []);
    }
    this.state.ledgerIds.get(orgId)?.push(ledgerId);
    this.metrics.totalLedgers++;
  }

  public getLedgerIds(orgId: string): string[] {
    return this.state.ledgerIds.get(orgId) || [];
  }

  public getAllLedgerIds(): string[] {
    const allIds: string[] = [];
    this.state.ledgerIds.forEach((ids) => allIds.push(...ids));
    return allIds;
  }

  // Asset state methods
  public addAssetId(ledgerId: string, assetId: string, assetCode: string): void {
    if (!this.state.assetIds.has(ledgerId)) {
      this.state.assetIds.set(ledgerId, []);
      this.state.assetCodes.set(ledgerId, []);
    }
    this.state.assetIds.get(ledgerId)?.push(assetId);
    this.state.assetCodes.get(ledgerId)?.push(assetCode);
    this.metrics.totalAssets++;
  }

  public getAssetIds(ledgerId: string): string[] {
    return this.state.assetIds.get(ledgerId) || [];
  }

  public getAssetCodes(ledgerId: string): string[] {
    return this.state.assetCodes.get(ledgerId) || [];
  }

  // Portfolio state methods
  public addPortfolioId(ledgerId: string, portfolioId: string): void {
    if (!this.state.portfolioIds.has(ledgerId)) {
      this.state.portfolioIds.set(ledgerId, []);
    }
    this.state.portfolioIds.get(ledgerId)?.push(portfolioId);
    this.metrics.totalPortfolios++;
  }

  public getPortfolioIds(ledgerId: string): string[] {
    return this.state.portfolioIds.get(ledgerId) || [];
  }

  // Segment state methods
  public addSegmentId(ledgerId: string, segmentId: string): void {
    if (!this.state.segmentIds.has(ledgerId)) {
      this.state.segmentIds.set(ledgerId, []);
    }
    this.state.segmentIds.get(ledgerId)?.push(segmentId);
    this.metrics.totalSegments++;
  }

  public getSegmentIds(ledgerId: string): string[] {
    return this.state.segmentIds.get(ledgerId) || [];
  }

  // Account state methods
  public addAccountId(ledgerId: string, accountId: string, accountAlias?: string): void {
    if (!this.state.accountIds.has(ledgerId)) {
      this.state.accountIds.set(ledgerId, []);
      this.state.accountAliases.set(ledgerId, []);
    }
    this.state.accountIds.get(ledgerId)?.push(accountId);
    if (accountAlias) {
      this.state.accountAliases.get(ledgerId)?.push(accountAlias);
    }
    this.metrics.totalAccounts++;
  }

  public getAccountIds(ledgerId: string): string[] {
    return this.state.accountIds.get(ledgerId) || [];
  }

  public getAccountAliases(ledgerId: string): string[] {
    return this.state.accountAliases.get(ledgerId) || [];
  }

  /**
   * Associate an asset code with an account
   * This helps track which asset type is used for each account
   */
  public setAccountAsset(ledgerId: string, accountId: string, assetCode: string): void {
    if (!this.state.accountAssets.has(ledgerId)) {
      this.state.accountAssets.set(ledgerId, new Map<string, string>());
    }
    this.state.accountAssets.get(ledgerId)?.set(accountId, assetCode);
  }

  /**
   * Get the asset code associated with an account
   */
  public getAccountAsset(ledgerId: string, accountId: string): string {
    return this.state.accountAssets.get(ledgerId)?.get(accountId) || 'BRL';
  }

  // Transaction state methods
  public addTransactionId(ledgerId: string, transactionId: string): void {
    if (!this.state.transactionIds.has(ledgerId)) {
      this.state.transactionIds.set(ledgerId, []);
    }
    this.state.transactionIds.get(ledgerId)?.push(transactionId);
    this.metrics.totalTransactions++;
  }

  public getTransactionIds(ledgerId: string): string[] {
    return this.state.transactionIds.get(ledgerId) || [];
  }

  // Error tracking
  public incrementErrorCount(entityType?: string): void {
    this.metrics.errors++;
    
    // Track errors by entity type if specified
    if (entityType) {
      switch (entityType.toLowerCase()) {
        case 'organization':
          this.metrics.organizationErrors = (this.metrics.organizationErrors || 0) + 1;
          break;
        case 'ledger':
          this.metrics.ledgerErrors = (this.metrics.ledgerErrors || 0) + 1;
          break;
        case 'asset':
          this.metrics.assetErrors = (this.metrics.assetErrors || 0) + 1;
          break;
        case 'portfolio':
          this.metrics.portfolioErrors = (this.metrics.portfolioErrors || 0) + 1;
          break;
        case 'segment':
          this.metrics.segmentErrors = (this.metrics.segmentErrors || 0) + 1;
          break;
        case 'account':
          this.metrics.accountErrors = (this.metrics.accountErrors || 0) + 1;
          break;
        case 'transaction':
          this.metrics.transactionErrors = (this.metrics.transactionErrors || 0) + 1;
          break;
      }
    }
  }

  // Retry tracking
  public incrementRetryCount(): void {
    this.metrics.retries++;
  }
}
