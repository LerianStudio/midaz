import { inject } from 'inversify'
import { LoggerAggregator } from '../logger/logger-aggregator'
import { snakeCase } from 'lodash'

export function LogOperation(options: {
  layer: 'application' | 'infrastructure' | 'domain'
  operation?: string
}): MethodDecorator {
  // If the environment is test, return the empty descriptor
  if (process.env.NODE_ENV === 'test') {
    return (_target, _propertyKey, descriptor) => descriptor
  }

  // Gets a function for injecting the service
  const ServiceInjection = inject(LoggerAggregator)

  return function (
    target,
    propertyKey: string | symbol,
    descriptor: PropertyDescriptor
  ) {
    // Injects the service into the target
    ServiceInjection(target, 'loggerAggregator')

    // Saves the original method
    const originalMethod = descriptor.value

    // If operation is not provided, use the class name as operation
    // Example: FetchAllSegmentsUseCase -> fetch_all_segments
    if (!options.operation) {
      options.operation = snakeCase(
        target.constructor.name.replace('UseCase', '')
      )
    }

    // Overrides the method
    descriptor.value = async function (...args: any[]) {
      const midazLogger: LoggerAggregator = (this as any).loggerAggregator
      const isDebugEnabled = process.env.MIDAZ_CONSOLE_ENABLE_DEBUG === 'true'

      try {
        midazLogger.info({
          layer: options.layer,
          operation: `${options.operation}_start`,
          level: 'info',
          message: `Starting ${options.operation}`,
          ...(isDebugEnabled && { metadata: { args } })
        })

        const result = await originalMethod.apply(this, args)

        midazLogger.info({
          layer: options.layer,
          operation: `${options.operation}_success`,
          message: `${options.operation} completed successfully`,
          ...(isDebugEnabled && { metadata: { result } })
        })

        return result
      } catch (error) {
        midazLogger.error(`${options.operation} failed`, {
          error: error as Error
        })

        throw error
      }
    }
  }
}
