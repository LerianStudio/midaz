/**
 * Simple Dependency Injection Container
 */

export type Factory<T = any> = () => T;
export type AsyncFactory<T = any> = () => Promise<T>;

/**
 * Service registration options
 */
export interface ServiceOptions {
  singleton?: boolean;
}

/**
 * Simple dependency injection container
 */
export class Container {
  private services = new Map<string, { factory: Factory | AsyncFactory; singleton: boolean; instance?: any }>();
  private instances = new Map<string, any>();

  /**
   * Register a service with a factory function
   */
  register<T>(token: string, factory: Factory<T>, options: ServiceOptions = {}): void {
    this.services.set(token, {
      factory,
      singleton: options.singleton ?? false
    });
  }

  /**
   * Register an async service with a factory function
   */
  registerAsync<T>(token: string, factory: AsyncFactory<T>, options: ServiceOptions = {}): void {
    this.services.set(token, {
      factory,
      singleton: options.singleton ?? false
    });
  }

  /**
   * Register a singleton service (always returns the same instance)
   */
  registerSingleton<T>(token: string, factory: Factory<T>): void {
    this.register(token, factory, { singleton: true });
  }

  /**
   * Register an async singleton service
   */
  registerAsyncSingleton<T>(token: string, factory: AsyncFactory<T>): void {
    this.registerAsync(token, factory, { singleton: true });
  }

  /**
   * Register an existing instance
   */
  registerInstance<T>(token: string, instance: T): void {
    this.instances.set(token, instance);
  }

  /**
   * Resolve a service by token
   */
  resolve<T>(token: string): T {
    // Check if we have a direct instance
    if (this.instances.has(token)) {
      return this.instances.get(token);
    }

    const service = this.services.get(token);
    if (!service) {
      throw new Error(`Service '${token}' not registered`);
    }

    // For singletons, check if we already have an instance
    if (service.singleton && service.instance !== undefined) {
      return service.instance;
    }

    // Create new instance
    const instance = service.factory();

    // Store singleton instances
    if (service.singleton) {
      service.instance = instance;
    }

    return instance;
  }

  /**
   * Resolve an async service by token
   */
  async resolveAsync<T>(token: string): Promise<T> {
    // Check if we have a direct instance
    if (this.instances.has(token)) {
      return this.instances.get(token);
    }

    const service = this.services.get(token);
    if (!service) {
      throw new Error(`Service '${token}' not registered`);
    }

    // For singletons, check if we already have an instance
    if (service.singleton && service.instance !== undefined) {
      return service.instance;
    }

    // Create new instance
    const instance = await service.factory();

    // Store singleton instances
    if (service.singleton) {
      service.instance = instance;
    }

    return instance;
  }

  /**
   * Check if a service is registered
   */
  has(token: string): boolean {
    return this.services.has(token) || this.instances.has(token);
  }

  /**
   * Unregister a service
   */
  unregister(token: string): void {
    this.services.delete(token);
    this.instances.delete(token);
  }

  /**
   * Clear all registrations
   */
  clear(): void {
    this.services.clear();
    this.instances.clear();
  }

  /**
   * Get all registered service tokens
   */
  getTokens(): string[] {
    return [...new Set([...this.services.keys(), ...this.instances.keys()])];
  }
}