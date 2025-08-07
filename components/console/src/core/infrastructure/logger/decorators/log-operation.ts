import { inject } from 'inversify'
import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { snakeCase } from 'lodash'

export function LogOperation(options: {
  layer: 'application' | 'infrastructure' | 'domain'
  operation?: string
}): MethodDecorator {
  if (process.env.NODE_ENV === 'test') {
    return (_target, _propertyKey, descriptor) => descriptor
  }

  const ServiceInjection = inject(LoggerAggregator)

  return function (
    target,
    propertyKey: string | symbol,
    descriptor: PropertyDescriptor
  ) {
    ServiceInjection(target, 'loggerAggregator')

    const originalMethod = descriptor.value

    // Example: FetchAllSegmentsUseCase -> fetch_all_segments
    if (!options.operation) {
      options.operation = snakeCase(
        target.constructor.name.replace('UseCase', '')
      )
    }

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
