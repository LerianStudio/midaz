import { container } from '@/core/infrastructure/container-registry/container-registry'
import { MidazRequestContext } from '@/core/infrastructure/logger/decorators/midaz-id'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'

export function ControllerLogger(): ClassDecorator {
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
        const midazRequestContext: MidazRequestContext =
          container.get<MidazRequestContext>(MidazRequestContext)

        // Clear any existing Midaz ID from the context
        midazRequestContext.clearMidazId()

        // Extract request body for non-GET and non-DELETE requests
        // let body = undefined
        // if (config.method !== 'GET' && config.method !== 'DELETE') {
        //   body = await req.json()
        // }

        // Execute the next middleware/handler within a logged context
        return loggerAggregator.runWithContext(
          `${target.name}.${methodName}`,
          req.method,
          {
            midazId: midazRequestContext.getMidazId()
          },
          async () => {
            return await originalMethod.apply(this, [req, ...args])
          }
        )
      }
    }
  }
}
