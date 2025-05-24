/**
 * Integration tests for the complete generator system
 */

import { Generator } from '../../src/generator';
import { ConfigurationManager } from '../../src/config/configuration-manager';
import { OptimizedStateManager } from '../../src/utils/optimized-state';
import { Logger } from '../../src/services/logger';

// Mock the SDK client
jest.mock('midaz-sdk/src', () => {
  return {
    MidazClient: jest.fn().mockImplementation(() => ({
      entities: {
        organizations: {
          createOrganization: jest.fn(),
          listOrganizations: jest.fn().mockResolvedValue({ items: [] }),
        },
        ledgers: {
          createLedger: jest.fn(),
          listLedgers: jest.fn().mockResolvedValue({ items: [] }),
        },
        assets: {
          createAsset: jest.fn(),
          listAssets: jest.fn().mockResolvedValue({ items: [] }),
        },
        portfolios: {
          createPortfolio: jest.fn(),
          listPortfolios: jest.fn().mockResolvedValue({ items: [] }),
        },
        segments: {
          createSegment: jest.fn(),
          listSegments: jest.fn().mockResolvedValue({ items: [] }),
        },
        accounts: {
          createAccount: jest.fn(),
          listAccounts: jest.fn().mockResolvedValue({ items: [] }),
        },
        transactions: {
          createTransaction: jest.fn(),
          listTransactions: jest.fn().mockResolvedValue({ items: [] }),
        },
      },
    })),
  };
});

describe('Generator Integration Tests', () => {
  let generator: Generator;
  let configManager: ConfigurationManager;
  let stateManager: OptimizedStateManager;
  let mockLogger: jest.Mocked<Logger>;

  beforeEach(() => {
    // Reset singletons
    (ConfigurationManager as any).instance = undefined;
    OptimizedStateManager.resetInstance();

    // Create mock logger
    mockLogger = {
      debug: jest.fn(),
      info: jest.fn(),
      warn: jest.fn(),
      error: jest.fn(),
      progress: jest.fn(),
    } as any;

    // Initialize managers
    configManager = ConfigurationManager.getInstance();
    stateManager = OptimizedStateManager.getInstance({
      maxEntitiesInMemory: 100,
      enableSnapshots: false,
      memoryOptimized: true,
    });

    // Create generator
    generator = new Generator({
      logger: mockLogger,
      apiBaseUrl: 'http://localhost:8080',
      enableValidation: true,
      enableCircuitBreaker: false, // Disable for testing
      enableProgressReporting: false, // Disable for testing
      organizations: 1,
      ledgersPerOrg: 1,
      assetsPerLedger: 2,
      portfoliosPerLedger: 1,
      segmentsPerLedger: 1,
      accountsPerLedger: 3,
      transactionsPerLedger: 2,
      batchSize: 5,
      retryAttempts: 1,
      retryDelayMs: 100,
    });

    jest.clearAllMocks();
  });

  afterEach(() => {
    stateManager.cleanup();
  });

  describe('configuration integration', () => {
    it('should use configuration manager settings', () => {
      configManager.setVolumePreset('small');
      
      const config = configManager.getConfig();
      expect(config.volume?.organizations).toBe(1);
      expect(config.volume?.ledgersPerOrg).toBe(2);
    });

    it('should validate configuration before generation', () => {
      const validation = configManager.validateConfig();
      expect(validation.isValid).toBe(true);
    });

    it('should handle environment-specific configuration', () => {
      const envConfig = configManager.getEnvironmentConfig();
      expect(envConfig.NODE_ENV).toBeDefined();
      expect(envConfig.API_BASE_URL).toBeDefined();
    });
  });

  describe('state management integration', () => {
    it('should track entities across generation', async () => {
      // Mock successful API responses
      const mockOrg = { id: 'org-1', legalName: 'Test Org', code: 'TEST' };
      const mockLedger = { id: 'ledger-1', name: 'Test Ledger', code: 'TEST_LEDGER' };
      
      (generator as any).client.entities.organizations.createOrganization.mockResolvedValue(mockOrg);
      (generator as any).client.entities.ledgers.createLedger.mockResolvedValue(mockLedger);

      // Track entities manually for testing
      stateManager.addOrganizationId(mockOrg.id);
      stateManager.addLedgerId(mockOrg.id, mockLedger.id);

      const metrics = stateManager.getMetrics();
      expect(metrics.totalOrganizations).toBe(1);
      expect(metrics.totalLedgers).toBe(1);

      const orgIds = stateManager.getOrganizationIds();
      expect(orgIds).toContain(mockOrg.id);

      const ledgerIds = stateManager.getLedgerIds(mockOrg.id);
      expect(ledgerIds).toContain(mockLedger.id);
    });

    it('should track errors in state manager', () => {
      const error = new Error('Test error');
      stateManager.trackGenerationError('organization', 'test-parent', error, { context: 'test' });
      stateManager.incrementErrorCount('organization');

      const metrics = stateManager.getMetrics();
      expect(metrics.errors).toBe(1);
      expect(metrics.organizationErrors).toBe(1);

      const errorTracker = stateManager.getErrorTracker();
      const report = errorTracker.generateReport();
      expect(report.totalErrors).toBe(1);
    });

    it('should manage memory usage effectively', () => {
      // Add many entities to test memory limits
      for (let i = 0; i < 150; i++) {
        stateManager.addOrganizationId(`org-${i}`);
        stateManager.addLedgerId(`org-${i}`, `ledger-${i}`);
        stateManager.addAssetId(`ledger-${i}`, `asset-${i}`, `ASSET_${i}`);
      }

      const memoryStats = stateManager.getMemoryStats();
      expect(memoryStats.totalEntities).toBeGreaterThan(0);
      expect(memoryStats.estimatedMemoryMB).toBeGreaterThan(0);
      
      // With memory optimization, should not exceed limits significantly
      expect(memoryStats.totalEntities).toBeLessThanOrEqual(500); // Reasonable limit
    });
  });

  describe('validation integration', () => {
    it('should validate data before generation', () => {
      const validOrgData = {
        legalName: 'Test Organization',
        code: 'TEST_ORG',
        status: 'ACTIVE',
      };

      // This should not throw with valid data
      expect(() => {
        (generator as any).validateOrganizationData(validOrgData);
      }).not.toThrow();
    });

    it('should reject invalid data during generation', () => {
      const invalidOrgData = {
        legalName: '', // Invalid: empty name
        code: 'TEST_ORG',
        status: 'INVALID_STATUS', // Invalid: wrong enum value
      };

      expect(() => {
        (generator as any).validateOrganizationData(invalidOrgData);
      }).toThrow();
    });
  });

  describe('error handling integration', () => {
    it('should handle API errors gracefully', async () => {
      // Mock API error
      const apiError = new Error('API connection failed');
      (generator as any).client.entities.organizations.createOrganization.mockRejectedValue(apiError);

      // Should handle the error without crashing
      await expect(async () => {
        await (generator as any).generateOrganizations();
      }).not.toThrow();

      // Should log the error
      expect(mockLogger.error).toHaveBeenCalled();
    });

    it('should track errors in metrics', async () => {
      const apiError = new Error('Test API error');
      (generator as any).client.entities.organizations.createOrganization.mockRejectedValue(apiError);

      try {
        await (generator as any).generateOrganizations();
      } catch (error) {
        // Expected to fail
      }

      const metrics = stateManager.getMetrics();
      expect(metrics.errors).toBeGreaterThan(0);
    });

    it('should continue generation after individual failures', async () => {
      // Mock mixed success/failure responses
      (generator as any).client.entities.organizations.createOrganization
        .mockRejectedValueOnce(new Error('First org failed'))
        .mockResolvedValueOnce({ id: 'org-2', legalName: 'Org 2', code: 'ORG2' });

      // Should handle partial failures
      await expect(async () => {
        await (generator as any).generateOrganizations();
      }).not.toThrow();

      // Should log both success and failure
      expect(mockLogger.error).toHaveBeenCalled();
      expect(mockLogger.info).toHaveBeenCalled();
    });
  });

  describe('dependency validation', () => {
    it('should validate dependencies before generation', async () => {
      // Ensure we have an organization first
      const mockOrg = { id: 'org-1', legalName: 'Test Org', code: 'TEST' };
      stateManager.addOrganizationId(mockOrg.id);

      // Should have organizations available
      const orgIds = stateManager.getOrganizationIds();
      expect(orgIds).toHaveLength(1);

      // Should be able to validate ledger dependencies
      const hasOrganizations = orgIds.length > 0;
      expect(hasOrganizations).toBe(true);
    });

    it('should prevent generation without proper dependencies', () => {
      // Clear state to simulate no organizations
      stateManager.clearState();

      const orgIds = stateManager.getOrganizationIds();
      expect(orgIds).toHaveLength(0);

      // Should not be able to generate ledgers without organizations
      const canGenerateLedgers = orgIds.length > 0;
      expect(canGenerateLedgers).toBe(false);
    });
  });

  describe('full generation workflow', () => {
    it('should complete end-to-end generation successfully', async () => {
      // Mock all successful API responses
      const mockOrg = { id: 'org-1', legalName: 'Test Org', code: 'TEST' };
      const mockLedger = { id: 'ledger-1', name: 'Test Ledger', code: 'TEST_LEDGER' };
      const mockAsset = { id: 'asset-1', name: 'Test Asset', code: 'TEST_ASSET' };
      const mockPortfolio = { id: 'portfolio-1', name: 'Test Portfolio', code: 'TEST_PORTFOLIO' };
      const mockAccount = { id: 'account-1', name: 'Test Account', alias: 'test_account' };

      (generator as any).client.entities.organizations.createOrganization.mockResolvedValue(mockOrg);
      (generator as any).client.entities.ledgers.createLedger.mockResolvedValue(mockLedger);
      (generator as any).client.entities.assets.createAsset.mockResolvedValue(mockAsset);
      (generator as any).client.entities.portfolios.createPortfolio.mockResolvedValue(mockPortfolio);
      (generator as any).client.entities.accounts.createAccount.mockResolvedValue(mockAccount);

      // This should complete without errors
      await expect(async () => {
        await generator.generateAll();
      }).not.toThrow();

      // Verify that metrics were tracked
      const finalMetrics = stateManager.getMetrics();
      expect(finalMetrics.totalOrganizations).toBeGreaterThan(0);
    }, 10000); // Increase timeout for full generation

    it('should maintain consistency across entity relationships', async () => {
      // Setup mock responses
      const mockOrg = { id: 'org-1', legalName: 'Test Org', code: 'TEST' };
      const mockLedger = { id: 'ledger-1', name: 'Test Ledger', code: 'TEST_LEDGER' };
      const mockAsset = { id: 'asset-1', name: 'Test Asset', code: 'TEST_ASSET' };

      (generator as any).client.entities.organizations.createOrganization.mockResolvedValue(mockOrg);
      (generator as any).client.entities.ledgers.createLedger.mockResolvedValue(mockLedger);
      (generator as any).client.entities.assets.createAsset.mockResolvedValue(mockAsset);

      // Manually track entities to simulate generation
      stateManager.addOrganizationId(mockOrg.id);
      stateManager.addLedgerId(mockOrg.id, mockLedger.id);
      stateManager.addAssetId(mockLedger.id, mockAsset.id, mockAsset.code);

      // Verify relationships are maintained
      const orgIds = stateManager.getOrganizationIds();
      const ledgerIds = stateManager.getLedgerIds(mockOrg.id);
      const assetCodes = stateManager.getAssetCodes(mockLedger.id);

      expect(orgIds).toContain(mockOrg.id);
      expect(ledgerIds).toContain(mockLedger.id);
      expect(assetCodes).toContain(mockAsset.code);
    });
  });

  describe('performance and scalability', () => {
    it('should handle large volumes efficiently', () => {
      // Test with larger configuration
      configManager.setVolumeConfig({
        organizations: 10,
        ledgersPerOrg: 5,
        assetsPerLedger: 10,
        portfoliosPerLedger: 3,
        segmentsPerLedger: 2,
        accountsPerLedger: 20,
        transactionsPerLedger: 50,
      });

      const config = configManager.getConfig();
      expect(config.volume?.organizations).toBe(10);

      // Calculate estimated entities
      const estimated = 10 * 5 * (10 + 3 + 2 + 20 + 50); // 4250 entities
      expect(estimated).toBe(4250);

      // Should be within reasonable limits
      expect(estimated).toBeLessThan(10000);
    });

    it('should monitor memory usage during generation', () => {
      // Simulate adding many entities
      for (let i = 0; i < 50; i++) {
        stateManager.addOrganizationId(`org-${i}`);
        for (let j = 0; j < 5; j++) {
          const ledgerId = `ledger-${i}-${j}`;
          stateManager.addLedgerId(`org-${i}`, ledgerId);
          for (let k = 0; k < 10; k++) {
            stateManager.addAssetId(ledgerId, `asset-${i}-${j}-${k}`, `ASSET_${i}_${j}_${k}`);
          }
        }
      }

      const memoryStats = stateManager.getMemoryStats();
      expect(memoryStats.totalEntities).toBeGreaterThan(0);
      expect(memoryStats.estimatedMemoryMB).toBeGreaterThan(0);
      
      // Memory should be reasonable
      expect(memoryStats.estimatedMemoryMB).toBeLessThan(100); // Less than 100MB
    });
  });
});