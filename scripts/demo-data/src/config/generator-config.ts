/**
 * Centralized configuration for the Midaz Demo Data Generator
 * All magic numbers and configuration values are consolidated here
 */

import { VolumeMetrics, VolumeSize } from '../types';

/**
 * Main generator configuration object
 */
export const GENERATOR_CONFIG = {
  // API Configuration
  api: {
    defaultBaseUrl: 'http://localhost',
    defaultOnboardingPort: 3000,
    defaultTransactionPort: 3001,
    defaultTimeout: 30000,
  },

  // Timing configuration
  delays: {
    depositSettlement: 3000, // ms to wait for deposits to settle
    betweenDepositsAndTransfers: 1000, // ms between deposit and transfer phases
    betweenBatches: 100, // ms between batch operations
    retryBase: 100, // base retry delay in ms
    retryMax: 2000, // max retry delay in ms
  },

  // Concurrency configuration
  concurrency: {
    default: 10,
    max: 100,
    accountDetailFetching: 10,
    transactionBatches: 5,
  },

  // Batch processing
  batches: {
    defaultSize: 100,
    deposits: {
      maxRetries: 3,
      useEnhancedRecovery: true,
      stopOnError: false,
      delayBetweenTransactions: 0,
      maxConcurrentOperationsPerAsset: 100,
    },
    transfers: {
      maxRetries: 2,
      useEnhancedRecovery: true,
      stopOnError: false,
      delayBetweenTransactions: 150,
      maxConcurrentOperationsPerAsset: 100,
    },
  },

  // Circuit breaker
  circuitBreaker: {
    failureThreshold: 5,
    resetTimeout: 30000,
    monitoringPeriod: 10000,
    recoveryTimeout: 30000,
    successThreshold: 3,
  },

  // Memory management
  memory: {
    maxEntitiesInMemory: 10000,
    compactionThreshold: 0.8,
    snapshotInterval: 5000,
  },

  // Retry configuration
  retry: {
    maxRetries: 3,
    initialDelay: 100,
    maxDelay: 1000,
    retryableStatusCodes: [408, 429, 500, 502, 503, 504],
  },

  // Progress reporting
  progress: {
    updateInterval: 10, // Update progress every N items
    largeUpdateInterval: 50, // Update progress every N items for large batches
    minItemsForProgress: 10,
  },

  // Transaction configuration
  transactions: {
    scale: 2, // Default scale for currency amounts
    metadata: {
      deposit: {
        type: 'deposit',
        generator: 'bulk-initial-deposits',
      },
      transfer: {
        type: 'transfer',
        batchProcessed: true,
      },
    },
  },

  // Asset-specific configuration
  assets: {
    // Templates for common assets
    templates: [
      { code: 'BRL', name: 'Brazilian Real', symbol: 'R$', scale: 2 },
      { code: 'USD', name: 'US Dollar', symbol: '$', scale: 2 },
      { code: 'EUR', name: 'Euro', symbol: '€', scale: 2 },
      { code: 'BTC', name: 'Bitcoin', symbol: '₿', scale: 8 },
      { code: 'ETH', name: 'Ethereum', symbol: 'Ξ', scale: 18 },
      { code: 'XAU', name: 'Gold', symbol: 'Au', scale: 6 },
      { code: 'XAG', name: 'Silver', symbol: 'Ag', scale: 6 },
    ],
    // Deposit amounts by asset type
    deposits: {
      CRYPTO: 10000, // 100.00 units (BTC, ETH)
      COMMODITIES: 500000, // 5000.00 units (Gold, Silver)
      DEFAULT: 1000000, // 10000.00 units (Currencies)
    },
    // Transfer amount ranges by asset type
    transfers: {
      CRYPTO: { min: 0.1, max: 1 },
      COMMODITIES: { min: 1, max: 10 },
      CURRENCIES: { min: 100, max: 500 },
      // Small amounts for testing to avoid insufficient funds
      CRYPTO_SMALL: { min: 0.01, max: 0.1 },
      COMMODITIES_SMALL: { min: 0.1, max: 1 },
      CURRENCIES_SMALL: { min: 10, max: 50 },
    },
  },

  // Account configuration
  accounts: {
    externalAccountFormat: '@external/{assetCode}',
    defaultAssetCode: 'BRL',
  },

  // Person type distribution
  personTypes: {
    individualPercentage: 70,
    companyPercentage: 30,
  },

  // Validation
  validation: {
    enabled: true,
    throwOnWarning: false,
  },

  // Logging
  logging: {
    levels: {
      development: 'debug',
      production: 'info',
      test: 'error',
    },
    emoji: {
      enabled: true,
      success: '✅',
      error: '❌',
      warning: '⚠️',
      info: 'ℹ️',
      debug: '🐛',
      progress: '📊',
    },
  },

  // File system
  filesystem: {
    checkpointDir: './checkpoints',
    checkpointFilePrefix: 'checkpoint-',
  },
};

/**
 * Volume metrics configuration
 */
export const VOLUME_METRICS: Record<VolumeSize, VolumeMetrics> = {
  [VolumeSize.SMALL]: {
    organizations: 2,
    ledgersPerOrg: 2,
    assetsPerLedger: 3,
    portfoliosPerLedger: 2,
    segmentsPerLedger: 2,
    accountsPerLedger: 100,
    transactionsPerAccount: 20,
  },
  [VolumeSize.MEDIUM]: {
    organizations: 5,
    ledgersPerOrg: 3,
    assetsPerLedger: 5,
    portfoliosPerLedger: 3,
    segmentsPerLedger: 4,
    accountsPerLedger: 50,
    transactionsPerAccount: 20,
  },
  [VolumeSize.LARGE]: {
    organizations: 10,
    ledgersPerOrg: 5,
    assetsPerLedger: 8,
    portfoliosPerLedger: 5,
    segmentsPerLedger: 6,
    accountsPerLedger: 1000,
    transactionsPerAccount: 20,
  },
};

/**
 * Default generator options
 */
export const DEFAULT_OPTIONS = {
  volume: VolumeSize.SMALL,
  baseUrl: GENERATOR_CONFIG.api.defaultBaseUrl,
  onboardingPort: GENERATOR_CONFIG.api.defaultOnboardingPort,
  transactionPort: GENERATOR_CONFIG.api.defaultTransactionPort,
  concurrency: GENERATOR_CONFIG.concurrency.default,
  debug: false,
};

// Re-export old constants for backward compatibility (to be removed later)
export const TRANSACTION_AMOUNTS = {
  min: GENERATOR_CONFIG.assets.transfers.CURRENCIES.min,
  max: GENERATOR_CONFIG.assets.transfers.CURRENCIES.max,
  scale: GENERATOR_CONFIG.transactions.scale,
};

export const ASSET_TEMPLATES = GENERATOR_CONFIG.assets.templates;

export const RETRY_CONFIG = GENERATOR_CONFIG.retry;

export const PERSON_TYPE_DISTRIBUTION = GENERATOR_CONFIG.personTypes;

export const MAX_CONCURRENCY = GENERATOR_CONFIG.concurrency.max;

export const TRANSACTION_TRANSFER_AMOUNTS = GENERATOR_CONFIG.assets.transfers;

export const DEPOSIT_AMOUNTS = GENERATOR_CONFIG.assets.deposits;

export const PROCESSING_DELAYS = {
  BETWEEN_DEPOSIT_AND_TRANSFER: GENERATOR_CONFIG.delays.betweenDepositsAndTransfers,
};

export const BATCH_PROCESSING_CONFIG = GENERATOR_CONFIG.batches;

export const ACCOUNT_FORMATS = {
  EXTERNAL_SOURCE: GENERATOR_CONFIG.accounts.externalAccountFormat,
};

export const TRANSACTION_METADATA = GENERATOR_CONFIG.transactions.metadata;