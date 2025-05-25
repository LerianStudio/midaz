/**
 * Factory for creating and configuring all generators with DI
 */

import { MidazClient } from 'midaz-sdk';
import { Container, ServiceTokens } from './container';
import { GENERATOR_CONFIG } from '../config/generator-config';
import { Logger } from '../services/logger';
import { StateManager } from '../utils/state';
import { initializeClient } from '../services/client';

// Import all generators
import { OrganizationGenerator } from '../generators/organizations';
import { LedgerGenerator } from '../generators/ledgers';
import { AssetGenerator } from '../generators/assets';
import { PortfolioGenerator } from '../generators/portfolios';
import { SegmentGenerator } from '../generators/segments';
import { AccountGenerator } from '../generators/accounts';
import { TransactionGenerator } from '../generators/transactions';
import { TransactionOrchestrator } from '../generators/transactions/transaction-orchestrator';
import { DepositGenerator } from '../generators/transactions/deposit-generator';
import { TransferGenerator } from '../generators/transactions/transfer-generator';
import {
  TransactionStrategyFactory,
  type DepositStrategy,
  type TransferStrategy,
} from '../generators/transactions/strategies/transaction-strategies';

// Import utilities
import { CheckpointManager } from '../utils/checkpoint-manager';
import { PerformanceReporter } from '../monitoring/performance-reporter';
import { InternalPluginManager } from '../plugins/internal/plugin-manager';
import { createCircuitBreaker } from '../utils/circuit-breaker';

import { GeneratorOptions } from '../types';

/**
 * Configure the DI container with all services
 */
export function configureContainer(container: Container, options: GeneratorOptions): Container {
  // Register configuration
  container.registerValue(ServiceTokens.Config, options);

  // Register core services
  container.registerSingleton(ServiceTokens.Logger, () => new Logger(options));
  
  container.registerSingleton(ServiceTokens.MidazClient, () => initializeClient(options));
  
  container.registerSingleton(ServiceTokens.StateManager, () => StateManager.getInstance());

  // Register transaction strategies
  container.registerTransient(ServiceTokens.DepositStrategy, () =>
    TransactionStrategyFactory.createDepositStrategy()
  );

  container.registerTransient(ServiceTokens.TransferStrategy, () =>
    TransactionStrategyFactory.createTransferStrategy()
  );

  // Register generators
  container.registerTransient(ServiceTokens.OrganizationGenerator, () =>
    new OrganizationGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.LedgerGenerator, () =>
    new LedgerGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.AssetGenerator, () =>
    new AssetGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.PortfolioGenerator, () =>
    new PortfolioGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.SegmentGenerator, () =>
    new SegmentGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.AccountGenerator, () =>
    new AccountGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.TransactionGenerator, () =>
    new TransactionGenerator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger)
    )
  );

  container.registerTransient(ServiceTokens.TransactionOrchestrator, () =>
    new TransactionOrchestrator(
      container.resolve<MidazClient>(ServiceTokens.MidazClient),
      container.resolve<Logger>(ServiceTokens.Logger),
      container.resolve<StateManager>(ServiceTokens.StateManager)
    )
  );

  // Register utilities
  container.registerSingleton(ServiceTokens.CheckpointManager, () =>
    new CheckpointManager(
      container.resolve<Logger>(ServiceTokens.Logger),
      GENERATOR_CONFIG.filesystem.checkpointDir
    )
  );

  container.registerTransient(ServiceTokens.PerformanceReporter, () =>
    new PerformanceReporter(container.resolve<Logger>(ServiceTokens.Logger))
  );

  container.registerSingleton(ServiceTokens.PluginManager, () =>
    new InternalPluginManager(
      container.resolve<Logger>(ServiceTokens.Logger),
      container.resolve<StateManager>(ServiceTokens.StateManager)
    )
  );

  container.registerTransient(ServiceTokens.CircuitBreaker, () =>
    createCircuitBreaker(GENERATOR_CONFIG.circuitBreaker)
  );

  return container;
}

/**
 * Create a fully configured container
 */
export function createGeneratorContainer(options: GeneratorOptions): Container {
  const container = new Container();
  return configureContainer(container, options);
}