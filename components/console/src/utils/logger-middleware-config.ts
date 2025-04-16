import { LoggerAggregator } from '@/core/application/logger/logger-aggregator'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { MidazRequestContext } from '@/core/infrastructure/logger/decorators/midaz-id'
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
const midazRequestContext: MidazRequestContext =
  container.get<MidazRequestContext>(MidazRequestContext)

/**
 * Middleware factory function that creates a logger middleware
 * This middleware handles request logging with context information
 */
export function loggerMiddleware(config: LoggerMiddlewareConfig) {
  return async (req: NextRequest, next: NextHandler) => {
    // Clear any existing Midaz ID from the context
    midazRequestContext.clearMidazId()

    // Extract request body for non-GET and non-DELETE requests
    let body = undefined
    if (config.method !== 'GET' && config.method !== 'DELETE') {
      body = await req.json()
    }

    // Execute the next middleware/handler within a logged context
    return loggerAggregator.runWithContext(
      config.operationName,
      config.method,
      {
        useCase: config.useCase,
        action: config.action || 'execute', // Default action is 'execute'
        midazId: midazRequestContext.getMidazId()
      },
      async () => {
        return await next()
      }
    )
  }
}
