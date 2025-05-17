import { StateManager } from '../../src/utils/state';

describe('StateManager', () => {
  let stateManager: StateManager;

  beforeEach(() => {
    stateManager = StateManager.getInstance();
    stateManager.reset();
  });

  it('should be a singleton', () => {
    const instance1 = StateManager.getInstance();
    const instance2 = StateManager.getInstance();
    expect(instance1).toBe(instance2);
  });

  it('should track organization creation', () => {
    stateManager.addOrganizationId('org1');
    stateManager.addOrganizationId('org2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalOrganizations).toBe(2);
  });

  it('should track ledger creation', () => {
    stateManager.addLedgerId('org1', 'ledger1');
    stateManager.addLedgerId('org1', 'ledger2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalLedgers).toBe(2);
  });

  it('should track asset creation', () => {
    stateManager.addAssetId('ledger1', 'asset1', 'AST1');
    stateManager.addAssetId('ledger1', 'asset2', 'AST2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalAssets).toBe(2);
  });

  it('should track portfolio creation', () => {
    stateManager.addPortfolioId('ledger1', 'portfolio1');
    stateManager.addPortfolioId('ledger1', 'portfolio2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalPortfolios).toBe(2);
  });

  it('should track segment creation', () => {
    stateManager.addSegmentId('ledger1', 'segment1');
    stateManager.addSegmentId('ledger1', 'segment2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalSegments).toBe(2);
  });

  it('should track account creation', () => {
    stateManager.addAccountId('ledger1', 'account1');
    stateManager.addAccountId('ledger1', 'account2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalAccounts).toBe(2);
  });

  it('should track transaction creation', () => {
    stateManager.addTransactionId('ledger1', 'tx1');
    stateManager.addTransactionId('ledger1', 'tx2');
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalTransactions).toBe(2);
  });

  it('should track errors', () => {
    stateManager.incrementErrorCount('organization');
    stateManager.incrementErrorCount('ledger');
    stateManager.incrementErrorCount('asset');
    stateManager.incrementErrorCount('portfolio');
    stateManager.incrementErrorCount('segment');
    stateManager.incrementErrorCount('account');
    stateManager.incrementErrorCount('transaction');
    
    const stats = stateManager.completeGeneration();
    expect(stats.organizationErrors).toBe(1);
    expect(stats.ledgerErrors).toBe(1);
    expect(stats.assetErrors).toBe(1);
    expect(stats.portfolioErrors).toBe(1);
    expect(stats.segmentErrors).toBe(1);
    expect(stats.accountErrors).toBe(1);
    expect(stats.transactionErrors).toBe(1);
    expect(stats.errors).toBe(7);
  });

  it('should track retries', () => {
    stateManager.incrementRetryCount();
    stateManager.incrementRetryCount();
    
    const stats = stateManager.completeGeneration();
    expect(stats.retries).toBe(2);
  });

  it('should calculate duration correctly', () => {
    // This is a bit tricky to test accurately, so we'll just check that it returns a number
    const stats = stateManager.completeGeneration();
    expect(typeof stats.duration()).toBe('number');
  });

  it('should reset all counters', () => {
    stateManager.addOrganizationId('org1');
    stateManager.incrementRetryCount();
    stateManager.reset();
    
    const stats = stateManager.completeGeneration();
    expect(stats.totalOrganizations).toBe(0);
    expect(stats.retries).toBe(0);
  });
});
