import { LoggerAggregator, RequestIdRepository } from '@lerianstudio/lib-logs'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { NextHandler } from '@/lib/middleware/types'
import { NextRequest } from 'next/server'

interface LoggerMiddlewareConfig {
  operationName: string
  method: string
  useCase?: string
  action?: string
  logLevel?: 'info' | 'error' | 'warn' | 'debug' | 'audit'
}

const loggerAggregator = container.get(LoggerAggregator)
const requestIdRepository: RequestIdRepository =
  container.get<RequestIdRepository>(RequestIdRepository)

export function loggerMiddleware(config: LoggerMiddlewareConfig) {
  return async (req: NextRequest, next: NextHandler) => {
    let _body = undefined
    if (config.method !== 'GET' && config.method !== 'DELETE') {
      _body = await req.json()
    }

    const traceId = requestIdRepository.generate()
    requestIdRepository.set(traceId)

    return loggerAggregator.runWithContext(
      config.operationName,
      config.method,
      {
        useCase: config.useCase,
        action: config.action || 'execute', // Default action is 'execute'
        midazId: traceId
      },
      async () => {
        return await next()
      }
    )
  }
}
