import { LoggerAggregator, RequestIdRepository } from 'lib-logs'
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

// Get instances from the dependency injection container
const loggerAggregator = container.get(LoggerAggregator)
const requestIdRepository: RequestIdRepository =
  container.get<RequestIdRepository>(RequestIdRepository)

/**
 * Middleware factory function that creates a logger middleware
 * This middleware handles request logging with context information
 */
export function loggerMiddleware(config: LoggerMiddlewareConfig) {
  return async (req: NextRequest, next: NextHandler) => {
    // Extract request body for non-GET and non-DELETE requests
    let body = undefined
    if (config.method !== 'GET' && config.method !== 'DELETE') {
      body = await req.json()
    }

    const traceId = requestIdRepository.generate()
    requestIdRepository.set(traceId)

    // Execute the next middleware/handler within a logged context
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
