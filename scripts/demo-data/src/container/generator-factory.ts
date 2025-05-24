/**
 * Factory for creating generators with dependency injection
 */

import { MidazClient } from 'midaz-sdk/src';
import { Generator } from '../generator';
import {
  AccountGenerator,
  AssetGenerator,
  LedgerGenerator,
  OrganizationGenerator,
  PortfolioGenerator,
  SegmentGenerator,
} from '../generators';
import { TransactionGenerator } from '../generators/transactions';
import { initializeClient } from '../services/client';
import { Logger } from '../services/logger';
import { GeneratorOptions } from '../types';
import { StateManager } from '../utils/state';
import { Container } from './container';

/**
 * Service tokens for dependency injection
 */
export const SERVICE_TOKENS = {
  CLIENT: 'client',
  LOGGER: 'logger',
  STATE_MANAGER: 'stateManager',
  ORGANIZATION_GENERATOR: 'organizationGenerator',
  LEDGER_GENERATOR: 'ledgerGenerator',
  ASSET_GENERATOR: 'assetGenerator',
  PORTFOLIO_GENERATOR: 'portfolioGenerator',
  SEGMENT_GENERATOR: 'segmentGenerator',
  ACCOUNT_GENERATOR: 'accountGenerator',
  TRANSACTION_GENERATOR: 'transactionGenerator',
} as const;

/**
 * Factory for creating generators with proper dependency injection
 */
export class GeneratorFactory {
  /**
   * Create a fully configured generator with all dependencies
   */
  static create(options: GeneratorOptions): Generator {
    const container = new Container();

    // Register core services as singletons
    container.registerSingleton(SERVICE_TOKENS.CLIENT, () => initializeClient(options));
    container.registerSingleton(SERVICE_TOKENS.LOGGER, () => new Logger(options));
    container.registerSingleton(SERVICE_TOKENS.STATE_MANAGER, () => StateManager.getInstance());

    // Register generators
    container.register(SERVICE_TOKENS.ORGANIZATION_GENERATOR, () => 
      new OrganizationGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    container.register(SERVICE_TOKENS.LEDGER_GENERATOR, () => 
      new LedgerGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    container.register(SERVICE_TOKENS.ASSET_GENERATOR, () => 
      new AssetGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    container.register(SERVICE_TOKENS.PORTFOLIO_GENERATOR, () => 
      new PortfolioGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    container.register(SERVICE_TOKENS.SEGMENT_GENERATOR, () => 
      new SegmentGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    container.register(SERVICE_TOKENS.ACCOUNT_GENERATOR, () => 
      new AccountGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    container.register(SERVICE_TOKENS.TRANSACTION_GENERATOR, () => 
      new TransactionGenerator(
        container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
        container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
      )
    );

    return new Generator(options, container);
  }

  /**
   * Create a container with just the core services
   */
  static createContainer(options: GeneratorOptions): Container {
    const container = new Container();

    container.registerSingleton(SERVICE_TOKENS.CLIENT, () => initializeClient(options));
    container.registerSingleton(SERVICE_TOKENS.LOGGER, () => new Logger(options));
    container.registerSingleton(SERVICE_TOKENS.STATE_MANAGER, () => StateManager.getInstance());

    return container;
  }

  /**
   * Create a specific generator with dependencies
   */
  static createGenerator<T>(
    GeneratorClass: new (client: MidazClient, logger: Logger) => T,
    options: GeneratorOptions
  ): T {
    const container = GeneratorFactory.createContainer(options);

    return new GeneratorClass(
      container.resolve<MidazClient>(SERVICE_TOKENS.CLIENT),
      container.resolve<Logger>(SERVICE_TOKENS.LOGGER)
    );
  }
}