/**
 * Tests for configuration manager
 */

import { ConfigurationManager } from '../../../src/config/configuration-manager';

// Mock environment variables
const originalEnv = process.env;

describe('ConfigurationManager', () => {
  beforeEach(() => {
    // Reset environment
    process.env = { ...originalEnv };
    
    // Reset singleton instance
    (ConfigurationManager as any).instance = undefined;
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  describe('singleton pattern', () => {
    it('should return the same instance', () => {
      const instance1 = ConfigurationManager.getInstance();
      const instance2 = ConfigurationManager.getInstance();

      expect(instance1).toBe(instance2);
    });

    it('should maintain state across calls', () => {
      const instance1 = ConfigurationManager.getInstance();
      instance1.setVolumePreset('medium');

      const instance2 = ConfigurationManager.getInstance();
      const config = instance2.getConfig();

      expect(config.volume).toBeDefined();
      expect(config.volume?.organizations).toBe(3);
    });
  });

  describe('environment configuration', () => {
    it('should load default environment values', () => {
      const manager = ConfigurationManager.getInstance();
      const envConfig = manager.getEnvironmentConfig();

      expect(envConfig.NODE_ENV).toBe('development');
      expect(envConfig.LOG_LEVEL).toBe('info');
      expect(envConfig.API_BASE_URL).toBe('http://localhost:8080');
      expect(envConfig.API_TIMEOUT).toBe(30000);
    });

    it('should override with environment variables', () => {
      process.env.NODE_ENV = 'production';
      process.env.LOG_LEVEL = 'error';
      process.env.API_BASE_URL = 'https://api.example.com';
      process.env.API_TIMEOUT = '60000';

      const manager = ConfigurationManager.getInstance();
      const envConfig = manager.getEnvironmentConfig();

      expect(envConfig.NODE_ENV).toBe('production');
      expect(envConfig.LOG_LEVEL).toBe('error');
      expect(envConfig.API_BASE_URL).toBe('https://api.example.com');
      expect(envConfig.API_TIMEOUT).toBe(60000);
    });

    it('should validate environment variables', () => {
      process.env.API_TIMEOUT = 'invalid';
      
      expect(() => {
        ConfigurationManager.getInstance();
      }).toThrow();
    });
  });

  describe('volume presets', () => {
    let manager: ConfigurationManager;

    beforeEach(() => {
      manager = ConfigurationManager.getInstance();
    });

    it('should get available volume presets', () => {
      const presets = manager.getAvailableVolumePresets();

      expect(presets).toHaveLength(4);
      expect(presets.map(p => p.name)).toEqual(['Small', 'Medium', 'Large', 'Extra Large']);
    });

    it('should get specific volume preset', () => {
      const smallPreset = manager.getVolumePreset('small');

      expect(smallPreset).toBeDefined();
      expect(smallPreset?.name).toBe('Small');
      expect(smallPreset?.config.organizations).toBe(1);
      expect(smallPreset?.config.ledgersPerOrg).toBe(2);
    });

    it('should return undefined for unknown preset', () => {
      const unknownPreset = manager.getVolumePreset('unknown');

      expect(unknownPreset).toBeUndefined();
    });

    it('should set volume preset by name', () => {
      const success = manager.setVolumePreset('medium');

      expect(success).toBe(true);

      const config = manager.getConfig();
      expect(config.volume?.organizations).toBe(3);
      expect(config.volume?.ledgersPerOrg).toBe(5);
    });

    it('should return false for unknown preset name', () => {
      const success = manager.setVolumePreset('unknown');

      expect(success).toBe(false);

      const config = manager.getConfig();
      expect(config.volume).toBeUndefined();
    });

    it('should be case insensitive', () => {
      const preset1 = manager.getVolumePreset('SMALL');
      const preset2 = manager.getVolumePreset('small');
      const preset3 = manager.getVolumePreset('Small');

      expect(preset1).toEqual(preset2);
      expect(preset2).toEqual(preset3);
    });
  });

  describe('custom volume configuration', () => {
    let manager: ConfigurationManager;

    beforeEach(() => {
      manager = ConfigurationManager.getInstance();
    });

    it('should set custom volume configuration', () => {
      const customVolume = {
        organizations: 5,
        ledgersPerOrg: 3,
        assetsPerLedger: 10,
        portfoliosPerLedger: 2,
        segmentsPerLedger: 1,
        accountsPerLedger: 20,
        transactionsPerLedger: 100,
      };

      manager.setVolumeConfig(customVolume);

      const config = manager.getConfig();
      expect(config.volume).toEqual(customVolume);
    });

    it('should validate custom volume configuration', () => {
      const invalidVolume = {
        organizations: 0, // Invalid: must be at least 1
        ledgersPerOrg: 3,
        assetsPerLedger: 10,
        portfoliosPerLedger: 2,
        segmentsPerLedger: 1,
        accountsPerLedger: 20,
        transactionsPerLedger: 100,
      };

      expect(() => {
        manager.setVolumeConfig(invalidVolume as any);
      }).toThrow();
    });
  });

  describe('environment detection', () => {
    let manager: ConfigurationManager;

    beforeEach(() => {
      manager = ConfigurationManager.getInstance();
    });

    it('should detect development environment', () => {
      process.env.NODE_ENV = 'development';
      manager = ConfigurationManager.getInstance();

      expect(manager.isDevelopment()).toBe(true);
      expect(manager.isProduction()).toBe(false);
      expect(manager.isTest()).toBe(false);
    });

    it('should detect production environment', () => {
      process.env.NODE_ENV = 'production';
      manager = ConfigurationManager.getInstance();

      expect(manager.isDevelopment()).toBe(false);
      expect(manager.isProduction()).toBe(true);
      expect(manager.isTest()).toBe(false);
    });

    it('should detect test environment', () => {
      process.env.NODE_ENV = 'test';
      manager = ConfigurationManager.getInstance();

      expect(manager.isDevelopment()).toBe(false);
      expect(manager.isProduction()).toBe(false);
      expect(manager.isTest()).toBe(true);
    });
  });

  describe('API configuration', () => {
    it('should return API configuration', () => {
      process.env.API_BASE_URL = 'https://api.test.com';
      process.env.API_TIMEOUT = '45000';
      process.env.MAX_RETRIES = '5';
      process.env.RETRY_DELAY = '2000';

      const manager = ConfigurationManager.getInstance();
      const apiConfig = manager.getApiConfig();

      expect(apiConfig.baseUrl).toBe('https://api.test.com');
      expect(apiConfig.timeout).toBe(45000);
      expect(apiConfig.maxRetries).toBe(5);
      expect(apiConfig.retryDelay).toBe(2000);
    });
  });

  describe('configuration validation', () => {
    let manager: ConfigurationManager;

    beforeEach(() => {
      manager = ConfigurationManager.getInstance();
    });

    it('should validate correct configuration', () => {
      const result = manager.validateConfig();

      expect(result.isValid).toBe(true);
      expect(result.errors).toBeUndefined();
    });

    it('should detect invalid configuration', () => {
      // Force invalid configuration
      (manager as any).config.generator.batchSize = -1;

      const result = manager.validateConfig();

      expect(result.isValid).toBe(false);
      expect(result.errors).toBeDefined();
      expect(result.errors?.some(err => err.includes('batchSize'))).toBe(true);
    });
  });

  describe('configuration export/import', () => {
    let manager: ConfigurationManager;

    beforeEach(() => {
      manager = ConfigurationManager.getInstance();
    });

    it('should export configuration as JSON', () => {
      manager.setVolumePreset('small');
      
      const configJson = manager.exportConfig();
      const parsed = JSON.parse(configJson);

      expect(parsed.volume.organizations).toBe(1);
      expect(parsed.environment.NODE_ENV).toBe('development');
    });

    it('should import configuration from JSON', () => {
      const config = {
        environment: {
          NODE_ENV: 'test',
          LOG_LEVEL: 'debug',
          API_BASE_URL: 'http://test.com',
          API_TIMEOUT: 15000,
          MAX_RETRIES: 2,
          RETRY_DELAY: 500,
          BATCH_SIZE: 5,
          ENABLE_PROGRESS_REPORTING: false,
          ENABLE_CIRCUIT_BREAKER: false,
          ENABLE_VALIDATION: false,
          MEMORY_OPTIMIZATION: false,
          MAX_ENTITIES_IN_MEMORY: 5000,
        },
        volume: {
          organizations: 2,
          ledgersPerOrg: 1,
          assetsPerLedger: 2,
          portfoliosPerLedger: 1,
          segmentsPerLedger: 0,
          accountsPerLedger: 3,
          transactionsPerLedger: 5,
        },
        circuitBreaker: {
          failureThreshold: 3,
          recoveryTimeout: 30000,
          monitoringPeriod: 120000,
          minimumRequests: 2,
          successThreshold: 0.6,
        },
        progress: {
          updateInterval: 3000,
          showETA: false,
          showThroughput: false,
          showProgressBar: false,
          progressBarWidth: 20,
        },
        state: {
          maxEntitiesInMemory: 5000,
          enableSnapshots: false,
          snapshotInterval: 15000,
          memoryOptimized: false,
        },
        generator: {
          batchSize: 5,
          retryAttempts: 2,
          retryDelayMs: 500,
          timeoutMs: 15000,
          enableValidation: false,
          enableCircuitBreaker: false,
          enableProgressReporting: false,
        },
      };

      manager.importConfig(JSON.stringify(config));

      const importedConfig = manager.getConfig();
      expect(importedConfig.volume?.organizations).toBe(2);
      expect(importedConfig.environment.LOG_LEVEL).toBe('debug');
      expect(importedConfig.generator.batchSize).toBe(5);
    });
  });

  describe('configuration summary', () => {
    it('should generate configuration summary', () => {
      const manager = ConfigurationManager.getInstance();
      manager.setVolumePreset('medium');

      const summary = manager.getConfigSummary();

      expect(summary).toContain('Environment: development');
      expect(summary).toContain('Organizations: 3');
      expect(summary).toContain('Ledgers per Org: 5');
      expect(summary).toContain('Estimated Total Entities:');
    });

    it('should handle missing volume configuration', () => {
      const manager = ConfigurationManager.getInstance();

      const summary = manager.getConfigSummary();

      expect(summary).toBe('No volume configuration set');
    });
  });

  describe('reset functionality', () => {
    it('should reset configuration to defaults', () => {
      const manager = ConfigurationManager.getInstance();
      
      // Modify configuration
      manager.setVolumePreset('large');
      manager.updateConfig({
        generator: {
          batchSize: 50,
          retryAttempts: 10,
          retryDelayMs: 5000,
          timeoutMs: 120000,
          enableValidation: false,
          enableCircuitBreaker: false,
          enableProgressReporting: false,
        },
      });

      // Reset to defaults
      manager.resetToDefaults();

      const config = manager.getConfig();
      expect(config.volume).toBeUndefined();
      expect(config.generator.batchSize).toBe(10); // Default value
    });
  });
});