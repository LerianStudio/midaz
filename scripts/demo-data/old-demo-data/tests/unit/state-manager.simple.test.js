// Simple JS test file for the state manager to avoid TypeScript issues
const { StateManager } = require('../../src/utils/state');

describe('StateManager', () => {
  let stateManager;

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

  it('should track error counts', () => {
    stateManager.incrementErrorCount('organization');
    stateManager.incrementErrorCount('ledger');
    
    const stats = stateManager.completeGeneration();
    expect(stats.organizationErrors).toBe(1);
    expect(stats.ledgerErrors).toBe(1);
    expect(stats.errors).toBe(2);
  });

  it('should track retry counts', () => {
    stateManager.incrementRetryCount();
    stateManager.incrementRetryCount();
    
    const stats = stateManager.completeGeneration();
    expect(stats.retries).toBe(2);
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
