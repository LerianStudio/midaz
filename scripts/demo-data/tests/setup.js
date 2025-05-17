// Jest setup file for global test configuration

// Create a mock MidazClient to be used consistently throughout tests
const createMockClient = () => ({
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

// Mock SDK in various import paths that might be used
// This ensures all imports resolve to the same mock
try {
  jest.mock('../midaz-sdk-typescript/src', () => ({
    MidazClient: jest.fn().mockImplementation(createMockClient),
  }));
} catch (e) {
  // Module might already be mocked or doesn't exist
}

try {
  jest.mock('../../midaz-sdk-typescript/src', () => ({
    MidazClient: jest.fn().mockImplementation(createMockClient),
  }));
} catch (e) {
  // Module might already be mocked or doesn't exist
}

try {
  jest.mock('../../../midaz-sdk-typescript/src', () => ({
    MidazClient: jest.fn().mockImplementation(createMockClient),
  }));
} catch (e) {
  // Module might already be mocked or doesn't exist
}

// Define global variables
global.mockMidazClient = createMockClient;

// Mock other utility functions/modules as needed

// Silence console during tests to clean output
global.consoleSilenced = false;

if (global.consoleSilenced) {
  global.originalConsole = {
    log: console.log,
    error: console.error,
    warn: console.warn,
    info: console.info,
  };

  console.log = jest.fn();
  console.error = jest.fn();
  console.warn = jest.fn();
  console.info = jest.fn();
}

// Reset mocks between tests
beforeEach(() => {
  jest.clearAllMocks();
});

