/**
 * Configuration management with validation and environment support
 */


import { z } from 'zod';

// Environment configuration schema
const environmentConfigSchema = z.object({
  NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),
  LOG_LEVEL: z.enum(['debug', 'info', 'warn', 'error']).default('info'),
  API_BASE_URL: z.string().url().default('http://localhost:8080'),
  API_TIMEOUT: z.coerce.number().min(1000).max(300000).default(30000),
  MAX_RETRIES: z.coerce.number().min(0).max(10).default(3),
  RETRY_DELAY: z.coerce.number().min(100).max(10000).default(1000),
  BATCH_SIZE: z.coerce.number().min(1).max(100).default(10),
  ENABLE_PROGRESS_REPORTING: z.coerce.boolean().default(true),
  ENABLE_CIRCUIT_BREAKER: z.coerce.boolean().default(true),
  ENABLE_VALIDATION: z.coerce.boolean().default(true),
  MEMORY_OPTIMIZATION: z.coerce.boolean().default(true),
  MAX_ENTITIES_IN_MEMORY: z.coerce.number().min(100).max(100000).default(10000),
});

// Volume configuration schema
const volumeConfigSchema = z.object({
  organizations: z.number().min(1).max(100),
  ledgersPerOrg: z.number().min(1).max(50),
  assetsPerLedger: z.number().min(1).max(20),
  portfoliosPerLedger: z.number().min(1).max(10),
  segmentsPerLedger: z.number().min(0).max(10),
  accountsPerLedger: z.number().min(2).max(100),
  transactionsPerLedger: z.number().min(0).max(1000),
});

// Circuit breaker configuration schema
const circuitBreakerConfigSchema = z.object({
  failureThreshold: z.number().min(1).max(20).default(5),
  recoveryTimeout: z.number().min(1000).max(300000).default(60000),
  monitoringPeriod: z.number().min(10000).max(3600000).default(300000),
  minimumRequests: z.number().min(1).max(50).default(3),
  successThreshold: z.number().min(0.1).max(1).default(0.5),
});

// Progress reporting configuration schema
const progressConfigSchema = z.object({
  updateInterval: z.number().min(1000).max(60000).default(2000),
  showETA: z.boolean().default(true),
  showThroughput: z.boolean().default(true),
  showProgressBar: z.boolean().default(true),
  progressBarWidth: z.number().min(10).max(100).default(30),
});

// State management configuration schema
const stateConfigSchema = z.object({
  maxEntitiesInMemory: z.number().min(100).max(100000).default(10000),
  enableSnapshots: z.boolean().default(true),
  snapshotInterval: z.number().min(5000).max(300000).default(30000),
  memoryOptimized: z.boolean().default(true),
});

// Main configuration schema
const configSchema = z.object({
  environment: environmentConfigSchema,
  volume: volumeConfigSchema.optional(),
  circuitBreaker: circuitBreakerConfigSchema,
  progress: progressConfigSchema,
  state: stateConfigSchema,
  generator: z.object({
    batchSize: z.number().min(1).max(100).default(10),
    retryAttempts: z.number().min(0).max(10).default(3),
    retryDelayMs: z.number().min(100).max(30000).default(1000),
    timeoutMs: z.number().min(1000).max(300000).default(30000),
    enableValidation: z.boolean().default(true),
    enableCircuitBreaker: z.boolean().default(true),
    enableProgressReporting: z.boolean().default(true),
  }),
});

export type Configuration = z.infer<typeof configSchema>;
export type VolumeConfig = z.infer<typeof volumeConfigSchema>;
export type EnvironmentConfig = z.infer<typeof environmentConfigSchema>;

export interface VolumePreset {
  name: string;
  description: string;
  config: VolumeConfig;
}

export class ConfigurationManager {
  private static instance: ConfigurationManager;
  private config: Configuration;

  // Predefined volume presets
  private static readonly VOLUME_PRESETS: Record<string, VolumePreset> = {
    small: {
      name: 'Small',
      description: 'Small volume for testing and development',
      config: {
        organizations: 1,
        ledgersPerOrg: 2,
        assetsPerLedger: 3,
        portfoliosPerLedger: 2,
        segmentsPerLedger: 1,
        accountsPerLedger: 5,
        transactionsPerLedger: 10,
      },
    },
    medium: {
      name: 'Medium',
      description: 'Medium volume for integration testing',
      config: {
        organizations: 3,
        ledgersPerOrg: 5,
        assetsPerLedger: 8,
        portfoliosPerLedger: 4,
        segmentsPerLedger: 3,
        accountsPerLedger: 15,
        transactionsPerLedger: 50,
      },
    },
    large: {
      name: 'Large',
      description: 'Large volume for performance testing',
      config: {
        organizations: 10,
        ledgersPerOrg: 10,
        assetsPerLedger: 15,
        portfoliosPerLedger: 8,
        segmentsPerLedger: 5,
        accountsPerLedger: 30,
        transactionsPerLedger: 200,
      },
    },
    xlarge: {
      name: 'Extra Large',
      description: 'Extra large volume for stress testing',
      config: {
        organizations: 25,
        ledgersPerOrg: 20,
        assetsPerLedger: 20,
        portfoliosPerLedger: 10,
        segmentsPerLedger: 8,
        accountsPerLedger: 50,
        transactionsPerLedger: 500,
      },
    },
  };

  private constructor() {
    this.config = this.loadConfiguration();
  }

  static getInstance(): ConfigurationManager {
    if (!ConfigurationManager.instance) {
      ConfigurationManager.instance = new ConfigurationManager();
    }
    return ConfigurationManager.instance;
  }

  /**
   * Load configuration from environment and defaults
   */
  private loadConfiguration(): Configuration {
    // Load environment variables
    const envConfig = environmentConfigSchema.parse(process.env);

    // Default configuration
    const defaultConfig: Configuration = {
      environment: envConfig,
      circuitBreaker: circuitBreakerConfigSchema.parse({}),
      progress: progressConfigSchema.parse({}),
      state: stateConfigSchema.parse({
        maxEntitiesInMemory: envConfig.MAX_ENTITIES_IN_MEMORY,
        memoryOptimized: envConfig.MEMORY_OPTIMIZATION,
      }),
      generator: {
        batchSize: envConfig.BATCH_SIZE,
        retryAttempts: envConfig.MAX_RETRIES,
        retryDelayMs: envConfig.RETRY_DELAY,
        timeoutMs: envConfig.API_TIMEOUT,
        enableValidation: envConfig.ENABLE_VALIDATION,
        enableCircuitBreaker: envConfig.ENABLE_CIRCUIT_BREAKER,
        enableProgressReporting: envConfig.ENABLE_PROGRESS_REPORTING,
      },
    };

    return configSchema.parse(defaultConfig);
  }

  /**
   * Get current configuration
   */
  getConfig(): Readonly<Configuration> {
    return this.config;
  }

  /**
   * Update configuration with validation
   */
  updateConfig(partialConfig: Partial<Configuration>): void {
    const newConfig = {
      ...this.config,
      ...partialConfig,
    };

    this.config = configSchema.parse(newConfig);
  }

  /**
   * Get volume configuration by preset name
   */
  getVolumePreset(presetName: string): VolumePreset | undefined {
    return ConfigurationManager.VOLUME_PRESETS[presetName.toLowerCase()];
  }

  /**
   * Get all available volume presets
   */
  getAvailableVolumePresets(): VolumePreset[] {
    return Object.values(ConfigurationManager.VOLUME_PRESETS);
  }

  /**
   * Set volume configuration by preset name
   */
  setVolumePreset(presetName: string): boolean {
    const preset = this.getVolumePreset(presetName);
    if (!preset) {
      return false;
    }

    this.updateConfig({
      volume: preset.config,
    });

    return true;
  }

  /**
   * Set custom volume configuration
   */
  setVolumeConfig(volumeConfig: VolumeConfig): void {
    const validatedConfig = volumeConfigSchema.parse(volumeConfig);
    this.updateConfig({
      volume: validatedConfig,
    });
  }

  /**
   * Get environment-specific configuration
   */
  getEnvironmentConfig(): EnvironmentConfig {
    return this.config.environment;
  }

  /**
   * Check if running in development mode
   */
  isDevelopment(): boolean {
    return this.config.environment.NODE_ENV === 'development';
  }

  /**
   * Check if running in production mode
   */
  isProduction(): boolean {
    return this.config.environment.NODE_ENV === 'production';
  }

  /**
   * Check if running in test mode
   */
  isTest(): boolean {
    return this.config.environment.NODE_ENV === 'test';
  }

  /**
   * Get log level
   */
  getLogLevel(): string {
    return this.config.environment.LOG_LEVEL;
  }

  /**
   * Get API configuration
   */
  getApiConfig(): {
    baseUrl: string;
    timeout: number;
    maxRetries: number;
    retryDelay: number;
  } {
    return {
      baseUrl: this.config.environment.API_BASE_URL,
      timeout: this.config.environment.API_TIMEOUT,
      maxRetries: this.config.environment.MAX_RETRIES,
      retryDelay: this.config.environment.RETRY_DELAY,
    };
  }

  /**
   * Validate configuration
   */
  validateConfig(): { isValid: boolean; errors?: string[] } {
    try {
      configSchema.parse(this.config);
      return { isValid: true };
    } catch (error) {
      if (error instanceof z.ZodError) {
        const errors = error.errors.map(err => `${err.path.join('.')}: ${err.message}`);
        return { isValid: false, errors };
      }
      return { isValid: false, errors: ['Unknown validation error'] };
    }
  }

  /**
   * Export configuration as JSON
   */
  exportConfig(): string {
    return JSON.stringify(this.config, null, 2);
  }

  /**
   * Import configuration from JSON
   */
  importConfig(configJson: string): void {
    const parsedConfig = JSON.parse(configJson);
    this.config = configSchema.parse(parsedConfig);
  }

  /**
   * Reset configuration to defaults
   */
  resetToDefaults(): void {
    this.config = this.loadConfiguration();
  }

  /**
   * Get configuration summary for logging
   */
  getConfigSummary(): string {
    const volume = this.config.volume;
    if (!volume) {
      return 'No volume configuration set';
    }

    const totalEstimated = 
      volume.organizations * (
        volume.ledgersPerOrg * (
          volume.assetsPerLedger + 
          volume.portfoliosPerLedger + 
          volume.segmentsPerLedger + 
          volume.accountsPerLedger + 
          volume.transactionsPerLedger
        )
      );

    return [
      `Environment: ${this.config.environment.NODE_ENV}`,
      `Organizations: ${volume.organizations}`,
      `Ledgers per Org: ${volume.ledgersPerOrg}`,
      `Assets per Ledger: ${volume.assetsPerLedger}`,
      `Accounts per Ledger: ${volume.accountsPerLedger}`,
      `Transactions per Ledger: ${volume.transactionsPerLedger}`,
      `Estimated Total Entities: ~${totalEstimated.toLocaleString()}`,
      `Batch Size: ${this.config.generator.batchSize}`,
      `Circuit Breaker: ${this.config.generator.enableCircuitBreaker ? 'Enabled' : 'Disabled'}`,
      `Memory Optimized: ${this.config.state.memoryOptimized ? 'Enabled' : 'Disabled'}`,
    ].join(' | ');
  }
}