import { AsyncLocalStorage } from 'async_hooks'
import { inject, injectable } from 'inversify'
import { LoggerRepository } from '@/core/domain/repositories/logger/logger-repository'

/**
 * LoggerAggregator
 * This class provides a sophisticated logging system that aggregates logs per request.
 * It uses AsyncLocalStorage to maintain request-scoped context and collect all log events
 * that occurs during a request's lifecycle.
 *
 * Key features:
 * - Request-scoped logging context
 * - Automatic error handling and logging
 * - Event timeline creation
 * - Log level determination based on severity
 * - Debug mode toggle support
 * - Metadata and duration tracking
 *
 * Usage:
 * Typically wrapped around HTTP request handlers to collect and aggregate
 * all logs generated during request processing into a single timeline.
 */

type Layer = 'api' | 'application' | 'infrastructure' | 'domain'
type Level = 'info' | 'error' | 'warn' | 'debug' | 'audit'

interface LogEvent {
  timestamp: number
  layer?: Layer
  operation?: string
  level?: Level
  message: string
  context?: any
  error?: Error
}

interface RequestContext {
  startTime: number
  path: string
  method: string
  metadata: Record<string, any>
  events: LogEvent[]
}

@injectable()
export class LoggerAggregator {
  // AsyncLocalStorage ensures request-scoped storage that's isolated between different requests
  private storage = new AsyncLocalStorage<RequestContext>()

  constructor(
    @inject(LoggerRepository)
    private readonly loggerRepository: LoggerRepository
  ) {
    this.storage = new AsyncLocalStorage<RequestContext>()
  }

  // Checks if debug logging is enabled via environment variable
  private shouldLogDebug(): boolean {
    return process.env.ENABLE_DEBUG === 'true'
  }

  /**
   * Wraps the execution of a request with a logging context
   * Captures all logs during the request lifecycle and handles errors
   * @param path - Request path
   * @param method - HTTP method
   * @param initialMetadata - Initial request metadata
   * @param fn - Async function to execute within the context
   */
  async runWithContext<T>(
    path: string,
    method: string,
    initialMetadata: Record<string, any>,
    fn: () => Promise<T>
  ): Promise<T> {
    const context: RequestContext = {
      startTime: Date.now(),
      path,
      method,
      metadata: initialMetadata,
      events: []
    }
    return this.storage.run(context, async () => {
      try {
        const result = await fn()
        this.finalizeContext()
        return result
      } catch (error) {
        this.addEvent({
          layer: 'api',
          operation: 'request_error',
          level: 'error',
          message: 'Request failed',
          error: error instanceof Error ? error : new Error(String(error))
        })
        this.finalizeContext()
        throw error
      }
    })
  }

  /**
   * Finalizes the request context by aggregating all collected logs
   * Creates a timeline of events and logs it with appropriate level
   * Includes request duration and metadata
   */
  finalizeContext() {
    // Get the current request context from AsyncLocalStorage
    const context = this.getContext()
    if (context) {
      // Calculate the total request duration in seconds
      const duration = (Date.now() - context.startTime) / 1000
      // Determine the most severe log level from all collected events
      // const logLevel = this.determineLogLevel(context.events)
      // Log the final timeline using the logger repository

      const logEventInfo = `${context.method} ${context.path}`
      const LogEventDetails = {
        events: context.events.map((event) => ({
          timestamp: new Date(event.timestamp).toISOString(),
          level: event.level?.toUpperCase() || 'INFO',
          message: event.message,
          layer: event.layer,
          operation: event.operation,
          ...(event.error && { error: event.error }),
          ...(event.context && { context: event.context })
        }))
      }
      const logEventExecution = {
        duration: `${duration}s`,
        path: context.path,
        method: context.method,
        ...context.metadata
      }

      this.loggerRepository.info(
        `${context.method} ${context.path}`,
        {
          events: context.events.map((event) => ({
            timestamp: new Date(event.timestamp).toISOString(),
            level: event.level?.toUpperCase() || 'INFO',
            message: event.message,
            layer: event.layer,
            operation: event.operation,
            ...(event.error && { error: event.error }),
            ...(event.context && { context: event.context })
          }))
        },
        {
          duration: `${duration}s`,
          path: context.path,
          method: context.method,
          ...context.metadata
        }
      )
    }
  }

  /**
   * Retrieves the current request context from storage
   * @returns The current RequestContext or undefined if not in a request context
   */
  private getContext(): RequestContext | undefined {
    return this.storage.getStore()
  }

  /**
   * Adds a new event to the current request context
   * Skips debug events if debug logging is disabled
   * Automatically adds timestamp to the event
   * @param event - The event to be logged
   */
  addEvent(event: Omit<LogEvent, 'timestamp'>) {
    const context = this.getContext()
    if (context) {
      if (event.level === 'debug' && !this.shouldLogDebug()) {
        return
      }
      const fullEvent: LogEvent = {
        timestamp: Date.now(),
        ...event
      }
      context.events.push(fullEvent)
    }
  }

  /**
   * Helper method to create a standardized log event
   */
  private createLogEvent(
    level: Level,
    message: any,
    context?: any,
    defaultLayer?: Layer
  ): Omit<LogEvent, 'timestamp'> {
    if (typeof message === 'string') {
      return {
        level,
        message,
        context,
        layer: defaultLayer
      }
    }

    return {
      message: message.message || '',
      ...message,
      level,
      layer: message.layer || defaultLayer,
      context: {
        ...message.context
      }
    }
  }

  debug(message: any, context?: any) {
    if (!this.shouldLogDebug()) {
      return
    }
    const event = this.createLogEvent('debug', message, context)
    this.addEvent(event)
  }

  info(message: any, context?: any) {
    const event = this.createLogEvent('info', message, context)
    this.addEvent(event)
  }

  error(message: any, context?: any) {
    const event = this.createLogEvent('error', message, context)
    this.addEvent(event)
  }

  warn(message: any, context?: any) {
    const event = this.createLogEvent('warn', message, context)
    this.addEvent(event)
  }

  audit(message: any, context?: any) {
    const event = this.createLogEvent('audit', message, context)
    this.addEvent(event)
  }
}
