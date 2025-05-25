/**
 * Dependency Injection Container
 * Manages service registration and resolution
 */

export type ServiceFactory<T> = () => T;
export type AsyncServiceFactory<T> = () => Promise<T>;
export type Factory<T> = ServiceFactory<T> | AsyncServiceFactory<T>;

export interface ServiceDescriptor<T> {
  factory: Factory<T>;
  singleton: boolean;
  instance?: T;
}

export class Container {
  private services = new Map<string | symbol, ServiceDescriptor<any>>();
  private resolving = new Set<string | symbol>();

  /**
   * Register a service with the container
   * @param token Unique identifier for the service
   * @param factory Function that creates the service
   * @param options Registration options
   */
  register<T>(
    token: string | symbol,
    factory: Factory<T>,
    options: { singleton?: boolean } = {}
  ): Container {
    const { singleton = true } = options;

    this.services.set(token, {
      factory,
      singleton,
    });

    return this;
  }

  /**
   * Register a singleton service (convenience method)
   */
  registerSingleton<T>(token: string | symbol, factory: Factory<T>): Container {
    return this.register(token, factory, { singleton: true });
  }

  /**
   * Register a transient service (convenience method)
   */
  registerTransient<T>(token: string | symbol, factory: Factory<T>): Container {
    return this.register(token, factory, { singleton: false });
  }

  /**
   * Register a value directly (for constants, configs, etc.)
   */
  registerValue<T>(token: string | symbol, value: T): Container {
    this.services.set(token, {
      factory: () => value,
      singleton: true,
      instance: value,
    });

    return this;
  }

  /**
   * Resolve a service from the container
   * @param token Service identifier
   * @throws Error if service is not registered or has circular dependencies
   */
  resolve<T>(token: string | symbol): T {
    const descriptor = this.services.get(token);

    if (!descriptor) {
      throw new Error(`Service '${String(token)}' not registered`);
    }

    // Check for circular dependencies
    if (this.resolving.has(token)) {
      throw new Error(`Circular dependency detected for service '${String(token)}'`);
    }

    // Return existing instance for singletons
    if (descriptor.singleton && descriptor.instance !== undefined) {
      return descriptor.instance;
    }

    try {
      this.resolving.add(token);

      // Create new instance
      const instance = descriptor.factory();

      // Store instance for singletons
      if (descriptor.singleton) {
        descriptor.instance = instance;
      }

      return instance;
    } finally {
      this.resolving.delete(token);
    }
  }

  /**
   * Resolve a service asynchronously
   * Useful when factories return promises
   */
  async resolveAsync<T>(token: string | symbol): Promise<T> {
    const descriptor = this.services.get(token);

    if (!descriptor) {
      throw new Error(`Service '${String(token)}' not registered`);
    }

    // Check for circular dependencies
    if (this.resolving.has(token)) {
      throw new Error(`Circular dependency detected for service '${String(token)}'`);
    }

    // Return existing instance for singletons
    if (descriptor.singleton && descriptor.instance !== undefined) {
      return descriptor.instance;
    }

    try {
      this.resolving.add(token);

      // Create new instance (might be async)
      const instance = await descriptor.factory();

      // Store instance for singletons
      if (descriptor.singleton) {
        descriptor.instance = instance;
      }

      return instance;
    } finally {
      this.resolving.delete(token);
    }
  }

  /**
   * Check if a service is registered
   */
  has(token: string | symbol): boolean {
    return this.services.has(token);
  }

  /**
   * Clear all services from the container
   */
  clear(): void {
    this.services.clear();
    this.resolving.clear();
  }

  /**
   * Get all registered service tokens
   */
  getTokens(): (string | symbol)[] {
    return Array.from(this.services.keys());
  }

  /**
   * Create a child container that inherits from this one
   */
  createChild(): Container {
    const child = new Container();
    
    // Copy all service descriptors to child
    this.services.forEach((descriptor, token) => {
      child.services.set(token, { ...descriptor });
    });

    return child;
  }
}

/**
 * Service tokens for type-safe dependency injection
 */
export const ServiceTokens = {
  // Core services
  Logger: Symbol('Logger'),
  MidazClient: Symbol('MidazClient'),
  StateManager: Symbol('StateManager'),
  Config: Symbol('Config'),

  // Generators
  OrganizationGenerator: Symbol('OrganizationGenerator'),
  LedgerGenerator: Symbol('LedgerGenerator'),
  AssetGenerator: Symbol('AssetGenerator'),
  PortfolioGenerator: Symbol('PortfolioGenerator'),
  SegmentGenerator: Symbol('SegmentGenerator'),
  AccountGenerator: Symbol('AccountGenerator'),
  TransactionGenerator: Symbol('TransactionGenerator'),
  TransactionOrchestrator: Symbol('TransactionOrchestrator'),

  // Strategies
  DepositStrategy: Symbol('DepositStrategy'),
  TransferStrategy: Symbol('TransferStrategy'),

  // Utilities
  CheckpointManager: Symbol('CheckpointManager'),
  PerformanceReporter: Symbol('PerformanceReporter'),
  PluginManager: Symbol('PluginManager'),
  CircuitBreaker: Symbol('CircuitBreaker'),
} as const;

/**
 * Default container instance
 */
export const defaultContainer = new Container();