import { container } from '@/core/infrastructure/container-registry/container-registry'
import { LoggerAggregator, RequestIdRepository } from 'lib-logs'

export function LoggerInterceptor(): ClassDecorator {
  // If the environment is test, return the empty descriptor
  if (process.env.NODE_ENV === 'test') {
    return (_target) => _target
  }

  return (target: Function) => {
    // Get all method names from the prototype
    const prototype = target.prototype
    const methodNames = Object.getOwnPropertyNames(prototype).filter(
      (name) => typeof prototype[name] === 'function' && name !== 'constructor'
    )

    // Replace each method with a wrapped version
    for (const methodName of methodNames) {
      const originalMethod = prototype[methodName]

      // Replace with wrapped method
      prototype[methodName] = async function (req: Request, ...args: any[]) {
        const loggerAggregator = container.get(LoggerAggregator)
        const requestIdRepository: RequestIdRepository =
          container.get<RequestIdRepository>(RequestIdRepository)

        const traceId = requestIdRepository.generate()
        requestIdRepository.set(traceId)

        // Execute the next middleware/handler within a logged context
        return loggerAggregator.runWithContext(
          `${target.name}.${methodName}`,
          req.method,
          {
            midazId: traceId
          },
          async () => {
            return await originalMethod.apply(this, [req, ...args])
          }
        )
      }
    }
  }
}
