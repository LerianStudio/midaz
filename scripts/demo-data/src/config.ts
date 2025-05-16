/**
 * Configuration for Midaz demo data generator
 */

import { VolumeMetrics, VolumeSize } from './types';

/**
 * Default generator options
 */
export const DEFAULT_OPTIONS = {
  volume: VolumeSize.SMALL,
  baseUrl: 'http://localhost',
  onboardingPort: 3000,
  transactionPort: 3001,
  concurrency: 10,
  debug: false,
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
    accountsPerLedger: 10,
    transactionsPerAccount: 5,
  },
  [VolumeSize.MEDIUM]: {
    organizations: 5,
    ledgersPerOrg: 3,
    assetsPerLedger: 5,
    portfoliosPerLedger: 3,
    segmentsPerLedger: 4,
    accountsPerLedger: 50,
    transactionsPerAccount: 10,
  },
  [VolumeSize.LARGE]: {
    organizations: 10,
    ledgersPerOrg: 5,
    assetsPerLedger: 8,
    portfoliosPerLedger: 5,
    segmentsPerLedger: 6,
    accountsPerLedger: 100,
    transactionsPerAccount: 20,
  },
};

/**
 * Transaction amount ranges in BRL
 */
export const TRANSACTION_AMOUNTS = {
  min: 10,
  max: 10000,
  scale: 2,
};

/**
 * Common assets to generate
 */
export const ASSET_TEMPLATES = [
  { code: 'BRL', name: 'Brazilian Real', symbol: 'R$', scale: 2 },
  { code: 'USD', name: 'US Dollar', symbol: '$', scale: 2 },
  { code: 'EUR', name: 'Euro', symbol: '€', scale: 2 },
  { code: 'BTC', name: 'Bitcoin', symbol: '₿', scale: 8 },
  { code: 'ETH', name: 'Ethereum', symbol: 'Ξ', scale: 18 },
  { code: 'GOLD', name: 'Gold', symbol: 'Au', scale: 6 },
  { code: 'SILVER', name: 'Silver', symbol: 'Ag', scale: 6 },
];

/**
 * Retry configuration
 */
export const RETRY_CONFIG = {
  maxRetries: 3,
  initialDelay: 100,
  maxDelay: 1000,
  retryableStatusCodes: [408, 429, 500, 502, 503, 504],
};

/**
 * Person type distribution (70% PF / 30% PJ)
 */
export const PERSON_TYPE_DISTRIBUTION = {
  individualPercentage: 70,
  companyPercentage: 30,
};

/**
 * Maximum concurrent operations
 */
export const MAX_CONCURRENCY = 100;
