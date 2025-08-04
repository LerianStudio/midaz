import { container } from '@/core/infrastructure/container-registry/container-registry'
import { LoggerAggregator, RequestIdRepository } from '@lerianstudio/lib-logs'

export function LoggerInterceptor(): ClassDecorator {
  if (process.env.NODE_ENV === 'test') {
    return (_target) => _target
  }

  return (target: Function) => {
    const prototype = target.prototype
    const methodNames = Object.getOwnPropertyNames(prototype).filter(
      (name) => typeof prototype[name] === 'function' && name !== 'constructor'
    )

    for (const methodName of methodNames) {
      const originalMethod = prototype[methodName]

      prototype[methodName] = async function (req: Request, ...args: any[]) {
        const loggerAggregator = container.get(LoggerAggregator)
        const requestIdRepository: RequestIdRepository =
          container.get<RequestIdRepository>(RequestIdRepository)

        const traceId = requestIdRepository.generate()
        requestIdRepository.set(traceId)

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
