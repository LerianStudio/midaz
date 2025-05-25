/**
 * Tests for centralized generator configuration
 */

import { GENERATOR_CONFIG, VOLUME_METRICS, DEFAULT_OPTIONS } from '../../../src/config/generator-config';
import { VolumeSize } from '../../../src/types';

describe('Generator Configuration', () => {
  describe('GENERATOR_CONFIG', () => {
    it('should have all required configuration sections', () => {
      expect(GENERATOR_CONFIG).toHaveProperty('api');
      expect(GENERATOR_CONFIG).toHaveProperty('delays');
      expect(GENERATOR_CONFIG).toHaveProperty('concurrency');
      expect(GENERATOR_CONFIG).toHaveProperty('batches');
      expect(GENERATOR_CONFIG).toHaveProperty('circuitBreaker');
      expect(GENERATOR_CONFIG).toHaveProperty('memory');
      expect(GENERATOR_CONFIG).toHaveProperty('retry');
      expect(GENERATOR_CONFIG).toHaveProperty('progress');
      expect(GENERATOR_CONFIG).toHaveProperty('transactions');
      expect(GENERATOR_CONFIG).toHaveProperty('assets');
      expect(GENERATOR_CONFIG).toHaveProperty('accounts');
      expect(GENERATOR_CONFIG).toHaveProperty('personTypes');
      expect(GENERATOR_CONFIG).toHaveProperty('validation');
      expect(GENERATOR_CONFIG).toHaveProperty('logging');
      expect(GENERATOR_CONFIG).toHaveProperty('filesystem');
    });

    describe('API Configuration', () => {
      it('should have valid API default values', () => {
        expect(GENERATOR_CONFIG.api.defaultBaseUrl).toBe('http://localhost');
        expect(GENERATOR_CONFIG.api.defaultOnboardingPort).toBe(3000);
        expect(GENERATOR_CONFIG.api.defaultTransactionPort).toBe(3001);
        expect(GENERATOR_CONFIG.api.defaultTimeout).toBe(30000);
      });
    });

    describe('Delays Configuration', () => {
      it('should have positive delay values', () => {
        expect(GENERATOR_CONFIG.delays.depositSettlement).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.delays.betweenDepositsAndTransfers).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.delays.betweenBatches).toBeGreaterThanOrEqual(0);
        expect(GENERATOR_CONFIG.delays.retryBase).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.delays.retryMax).toBeGreaterThan(GENERATOR_CONFIG.delays.retryBase);
      });
    });

    describe('Concurrency Configuration', () => {
      it('should have valid concurrency limits', () => {
        expect(GENERATOR_CONFIG.concurrency.default).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.concurrency.max).toBeGreaterThan(GENERATOR_CONFIG.concurrency.default);
        expect(GENERATOR_CONFIG.concurrency.accountDetailFetching).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.concurrency.transactionBatches).toBeGreaterThan(0);
      });
    });

    describe('Batch Processing Configuration', () => {
      it('should have valid batch configurations', () => {
        expect(GENERATOR_CONFIG.batches.defaultSize).toBeGreaterThan(0);
        
        // Deposits config
        expect(GENERATOR_CONFIG.batches.deposits.maxRetries).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.batches.deposits.useEnhancedRecovery).toBe(true);
        expect(GENERATOR_CONFIG.batches.deposits.stopOnError).toBe(false);
        expect(GENERATOR_CONFIG.batches.deposits.maxConcurrentOperationsPerAsset).toBeGreaterThan(0);
        
        // Transfers config
        expect(GENERATOR_CONFIG.batches.transfers.maxRetries).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.batches.transfers.useEnhancedRecovery).toBe(true);
        expect(GENERATOR_CONFIG.batches.transfers.stopOnError).toBe(false);
        expect(GENERATOR_CONFIG.batches.transfers.maxConcurrentOperationsPerAsset).toBeGreaterThan(0);
      });
    });

    describe('Circuit Breaker Configuration', () => {
      it('should have valid circuit breaker settings', () => {
        expect(GENERATOR_CONFIG.circuitBreaker.failureThreshold).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.circuitBreaker.resetTimeout).toBeGreaterThan(0);
        expect(GENERATOR_CONFIG.circuitBreaker.monitoringPeriod).toBeGreaterThan(0);
      });
    });

    describe('Asset Configuration', () => {
      it('should have valid asset templates', () => {
        expect(GENERATOR_CONFIG.assets.templates).toHaveLength(7);
        
        const brl = GENERATOR_CONFIG.assets.templates.find(a => a.code === 'BRL');
        expect(brl).toBeDefined();
        expect(brl?.scale).toBe(2);
        
        const btc = GENERATOR_CONFIG.assets.templates.find(a => a.code === 'BTC');
        expect(btc).toBeDefined();
        expect(btc?.scale).toBe(8);
      });

      it('should have valid deposit amounts', () => {
        expect(GENERATOR_CONFIG.assets.deposits.CRYPTO).toBe(10000);
        expect(GENERATOR_CONFIG.assets.deposits.COMMODITIES).toBe(500000);
        expect(GENERATOR_CONFIG.assets.deposits.DEFAULT).toBe(1000000);
      });

      it('should have valid transfer amount ranges', () => {
        expect(GENERATOR_CONFIG.assets.transfers.CRYPTO.min).toBeLessThan(GENERATOR_CONFIG.assets.transfers.CRYPTO.max);
        expect(GENERATOR_CONFIG.assets.transfers.COMMODITIES.min).toBeLessThan(GENERATOR_CONFIG.assets.transfers.COMMODITIES.max);
        expect(GENERATOR_CONFIG.assets.transfers.CURRENCIES.min).toBeLessThan(GENERATOR_CONFIG.assets.transfers.CURRENCIES.max);
      });
    });
  });

  describe('VOLUME_METRICS', () => {
    it('should have configurations for all volume sizes', () => {
      expect(VOLUME_METRICS).toHaveProperty(VolumeSize.SMALL);
      expect(VOLUME_METRICS).toHaveProperty(VolumeSize.MEDIUM);
      expect(VOLUME_METRICS).toHaveProperty(VolumeSize.LARGE);
    });

    it('should have increasing values for larger volumes', () => {
      const small = VOLUME_METRICS[VolumeSize.SMALL];
      const medium = VOLUME_METRICS[VolumeSize.MEDIUM];
      const large = VOLUME_METRICS[VolumeSize.LARGE];

      expect(medium.organizations).toBeGreaterThan(small.organizations);
      expect(large.organizations).toBeGreaterThan(medium.organizations);

      expect(medium.ledgersPerOrg).toBeGreaterThanOrEqual(small.ledgersPerOrg);
      expect(large.ledgersPerOrg).toBeGreaterThanOrEqual(medium.ledgersPerOrg);
    });
  });

  describe('DEFAULT_OPTIONS', () => {
    it('should use values from GENERATOR_CONFIG', () => {
      expect(DEFAULT_OPTIONS.baseUrl).toBe(GENERATOR_CONFIG.api.defaultBaseUrl);
      expect(DEFAULT_OPTIONS.onboardingPort).toBe(GENERATOR_CONFIG.api.defaultOnboardingPort);
      expect(DEFAULT_OPTIONS.transactionPort).toBe(GENERATOR_CONFIG.api.defaultTransactionPort);
      expect(DEFAULT_OPTIONS.concurrency).toBe(GENERATOR_CONFIG.concurrency.default);
    });
  });

  describe('Backward Compatibility Exports', () => {
    it('should maintain backward compatibility constants', () => {
      const { 
        TRANSACTION_AMOUNTS,
        ASSET_TEMPLATES,
        RETRY_CONFIG,
        PERSON_TYPE_DISTRIBUTION,
        MAX_CONCURRENCY,
        TRANSACTION_TRANSFER_AMOUNTS,
        DEPOSIT_AMOUNTS,
        PROCESSING_DELAYS,
        BATCH_PROCESSING_CONFIG,
        ACCOUNT_FORMATS,
        TRANSACTION_METADATA
      } = require('../../../src/config/generator-config');

      expect(TRANSACTION_AMOUNTS.scale).toBe(GENERATOR_CONFIG.transactions.scale);
      expect(ASSET_TEMPLATES).toBe(GENERATOR_CONFIG.assets.templates);
      expect(RETRY_CONFIG).toBe(GENERATOR_CONFIG.retry);
      expect(PERSON_TYPE_DISTRIBUTION).toBe(GENERATOR_CONFIG.personTypes);
      expect(MAX_CONCURRENCY).toBe(GENERATOR_CONFIG.concurrency.max);
      expect(DEPOSIT_AMOUNTS).toBe(GENERATOR_CONFIG.assets.deposits);
    });
  });
});