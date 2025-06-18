import { container } from '@/core/infrastructure/container-registry/container-registry'

/**
 * Gets a specific controller method from the dependency injection container.
 *
 * @param controller
 * @param method
 * @returns
 */
export function getController(controller: any, method: (c: any) => any) {
  return async (...args: any) => {
    const controllerInstance: typeof controller =
      await container.getAsync(controller)

    return method(controllerInstance).bind(controllerInstance)(...args)
  }
}
