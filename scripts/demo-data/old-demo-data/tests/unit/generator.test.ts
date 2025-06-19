// Import Jest globals
import { jest, describe, it, expect, beforeEach } from '@jest/globals';
import { Generator } from '../../src/generator';
import { GeneratorOptions, VolumeSize } from '../../src/types';

// Define a type to help with mocking
type MockMidazClientType = {
  onboarding: {
    organizations: any;
    ledgers: any;
    assets: any;
    portfolios: any;
    segments: any;
    accounts: any;
  };
  transaction: {
    transactions: any;
  };
};

// The mocks are now handled in the setup.js file
// This keeps individual test files cleaner

// Create a local reference to a mocked client
const getMockClient = (): MockMidazClientType => ({
  onboarding: {
    organizations: {
      create: jest.fn().mockResolvedValue({ id: 'org-id', legalName: 'Test Org' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
    ledgers: {
      create: jest.fn().mockResolvedValue({ id: 'ledger-id', name: 'Test Ledger' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
    assets: {
      create: jest.fn().mockResolvedValue({ id: 'asset-id', code: 'TST' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
    portfolios: {
      create: jest.fn().mockResolvedValue({ id: 'portfolio-id', name: 'Test Portfolio' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
    segments: {
      create: jest.fn().mockResolvedValue({ id: 'segment-id', name: 'Test Segment' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
    accounts: {
      create: jest.fn().mockResolvedValue({ id: 'account-id', name: 'Test Account' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
  },
  transaction: {
    transactions: {
      create: jest.fn().mockResolvedValue({ id: 'tx-id' }),
      list: jest.fn().mockResolvedValue({ items: [] }),
    },
  },
});

const MockMidazClient = jest.fn().mockImplementation(() => getMockClient());

// Mock the logger
jest.mock('../../src/services/logger', () => ({
  Logger: jest.fn().mockImplementation(() => ({
    info: jest.fn(),
    debug: jest.fn(),
    error: jest.fn(),
    warn: jest.fn(),
    metrics: jest.fn(),
  })),
}));

// Mock the state manager
jest.mock('../../src/utils/state', () => ({
  StateManager: {
    getInstance: jest.fn().mockReturnValue({
      reset: jest.fn(),
      addOrganizationId: jest.fn(),
      addLedgerId: jest.fn(),
      addAssetId: jest.fn(),
      addPortfolioId: jest.fn(),
      addSegmentId: jest.fn(),
      addAccountId: jest.fn(),
      addTransactionId: jest.fn(),
      completeGeneration: jest.fn().mockReturnValue({
        totalOrganizations: 1,
        totalLedgers: 1,
        totalAssets: 1,
        totalPortfolios: 1,
        totalSegments: 1,
        totalAccounts: 1,
        totalTransactions: 1,
        organizationErrors: 0,
        ledgerErrors: 0,
        assetErrors: 0,
        portfolioErrors: 0,
        segmentErrors: 0,
        accountErrors: 0,
        transactionErrors: 0,
        errors: 0,
        retries: 0,
        duration: jest.fn().mockReturnValue(1000),
      }),
    }),
  },
}));

// Mock client initialization function
jest.mock('../../src/services/client', () => ({
  initializeClient: jest.fn().mockReturnValue(getMockClient()),
}));

// Mock all generator classes
jest.mock('../../src/generators', () => ({
  OrganizationGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'org-id', legalName: 'Test Org' }]),
  })),
  LedgerGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'ledger-id', name: 'Test Ledger' }]),
  })),
  AssetGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'asset-id', code: 'TST' }]),
  })),
  PortfolioGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'portfolio-id', name: 'Test Portfolio' }]),
  })),
  SegmentGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'segment-id', name: 'Test Segment' }]),
  })),
  AccountGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'account-id', name: 'Test Account' }]),
  })),
  TransactionGenerator: jest.fn().mockImplementation(() => ({
    generate: jest.fn().mockResolvedValue([{ id: 'tx-id' }]),
  })),
}));

// Wrap the tests in a describe block
describe('Generator', () => {
  let generator: Generator;
  const options: GeneratorOptions = {
    volume: VolumeSize.SMALL,
    baseUrl: 'http://localhost',
    onboardingPort: 3000,
    transactionPort: 3001,
    concurrency: 5,
    debug: false,
  };

  beforeEach(() => {
    jest.clearAllMocks();
    generator = new Generator(options);
  });

  describe('constructor', () => {
    it('should initialize with the provided options', () => {
      expect(generator).toBeDefined();
    });
  });

  describe('run', () => {
    it('should run the generator successfully', async () => {
      await expect(generator.run()).resolves.not.toThrow();
    });

    it('should handle and rethrow errors', async () => {
      const mockError = new Error('Test error');
      
      // Mock the organization generator to throw an error
      const generators = jest.requireMock('../../src/generators');
      generators.OrganizationGenerator.mockImplementation(() => ({
        generate: jest.fn().mockRejectedValue(mockError),
      }));
      
      // Create a new instance with the mocked error
      const errorGenerator = new Generator(options);
      
      await expect(errorGenerator.run()).rejects.toThrow(mockError);
    });
  });
});
