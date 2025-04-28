import { interfaces, Container as InversifyContainer } from 'inversify'

/**
 * A Wrapper class for the Inversify Container.
 * Allows the container into a N depth hierarchy module system.
 */
export class Container {
  public container: InversifyContainer

  constructor() {
    this.container = new InversifyContainer()
  }

  /**
   * Loads a module into the container.
   * Internally calls the registry method of the module.
   * All child modules are registered in the parent container.
   * @param module ContainerModule
   */
  load(module: ContainerModule) {
    if (!module.hasOwnProperty('registry')) {
      throw new Error(
        `Container: module ${module} does not have a registry method`
      )
    }

    module.registry(this)
  }

  // Inversify Container Wrappers

  bind<T>(
    serviceIdentifier: interfaces.ServiceIdentifier<T>
  ): interfaces.BindingToSyntax<T> {
    return this.container.bind(serviceIdentifier)
  }

  get<T>(serviceIdentifier: interfaces.ServiceIdentifier<T>): T {
    return this.container.get(serviceIdentifier)
  }
}

export type ContainerModuleRegistry = (container: Container) => void

/**
 * Child module container.
 * Receives a registry method to allow child bindings.
 * @param registry ContainerModuleRegistry
 */
export class ContainerModule {
  public registry: ContainerModuleRegistry

  constructor(registry: ContainerModuleRegistry) {
    this.registry = registry
  }
}
