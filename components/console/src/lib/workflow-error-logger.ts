import {
  WorkflowError,
  WorkflowErrorType
} from '@/components/workflows/error-handling-wrapper'

export interface ErrorLogEntry {
  timestamp: Date
  error: WorkflowError
  context?: Record<string, any>
  userId?: string
  sessionId?: string
  url?: string
}

export interface ErrorLogger {
  log(error: Error | WorkflowError, context?: Record<string, any>): void
  logWarning(message: string, context?: Record<string, any>): void
  logInfo(message: string, context?: Record<string, any>): void
  getRecentErrors(limit?: number): ErrorLogEntry[]
  clearErrors(): void
}

class WorkflowErrorLogger implements ErrorLogger {
  private errors: ErrorLogEntry[] = []
  private maxErrors = 100
  private isDevelopment = process.env.NODE_ENV === 'development'

  constructor(
    private options?: {
      maxErrors?: number
      logToConsole?: boolean
      logToService?: boolean
      serviceEndpoint?: string
    }
  ) {
    this.maxErrors = options?.maxErrors || 100
  }

  log(error: Error | WorkflowError, context?: Record<string, any>): void {
    const workflowError =
      error instanceof WorkflowError ? error : WorkflowError.fromError(error)

    const entry: ErrorLogEntry = {
      timestamp: new Date(),
      error: workflowError,
      context,
      url: typeof window !== 'undefined' ? window.location.href : undefined,
      sessionId: this.getSessionId(),
      userId: this.getUserId()
    }

    // Store in memory (limited)
    this.errors.unshift(entry)
    if (this.errors.length > this.maxErrors) {
      this.errors = this.errors.slice(0, this.maxErrors)
    }

    // Log to console in development
    if (this.isDevelopment || this.options?.logToConsole) {
      console.group(`🚨 Workflow Error: ${workflowError.type}`)
      console.error('Message:', workflowError.message)
      console.error('Details:', workflowError.details)
      if (context) console.error('Context:', context)
      console.error('Stack:', workflowError.stack)
      console.groupEnd()
    }

    // Send to logging service
    if (this.options?.logToService && this.options.serviceEndpoint) {
      this.sendToService(entry)
    }

    // Track error metrics
    this.trackErrorMetrics(workflowError)
  }

  logWarning(message: string, context?: Record<string, any>): void {
    if (this.isDevelopment || this.options?.logToConsole) {
      console.warn(`⚠️ Workflow Warning: ${message}`, context)
    }
  }

  logInfo(message: string, context?: Record<string, any>): void {
    if (this.isDevelopment || this.options?.logToConsole) {
      console.info(`ℹ️ Workflow Info: ${message}`, context)
    }
  }

  getRecentErrors(limit: number = 10): ErrorLogEntry[] {
    return this.errors.slice(0, limit)
  }

  clearErrors(): void {
    this.errors = []
  }

  private async sendToService(entry: ErrorLogEntry): Promise<void> {
    if (!this.options?.serviceEndpoint) return

    try {
      await fetch(this.options.serviceEndpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          ...entry,
          timestamp: entry.timestamp.toISOString(),
          error: {
            type: entry.error.type,
            message: entry.error.message,
            details: entry.error.details,
            stack: entry.error.stack
          }
        })
      })
    } catch (err) {
      // Silently fail to avoid infinite error loops
      if (this.isDevelopment) {
        console.error('Failed to send error to logging service:', err)
      }
    }
  }

  private trackErrorMetrics(error: WorkflowError): void {
    // Track error counts by type
    if (typeof window !== 'undefined' && (window as any).analytics) {
      ;(window as any).analytics.track('Workflow Error', {
        errorType: error.type,
        errorMessage: error.message,
        timestamp: new Date().toISOString()
      })
    }
  }

  private getSessionId(): string {
    if (typeof window === 'undefined') return ''

    // Get or create session ID
    let sessionId = sessionStorage.getItem('workflow-session-id')
    if (!sessionId) {
      sessionId = `session-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
      sessionStorage.setItem('workflow-session-id', sessionId)
    }
    return sessionId
  }

  private getUserId(): string {
    // This would typically come from your auth context
    return 'anonymous'
  }
}

// Create singleton instance
export const workflowErrorLogger = new WorkflowErrorLogger({
  logToConsole: true,
  logToService: process.env.NEXT_PUBLIC_ERROR_LOGGING_ENDPOINT ? true : false,
  serviceEndpoint: process.env.NEXT_PUBLIC_ERROR_LOGGING_ENDPOINT
})

// Utility functions for common error scenarios
export function logWorkflowError(
  error: Error | WorkflowError,
  context?: Record<string, any>
): void {
  workflowErrorLogger.log(error, context)
}

export function logNetworkError(
  endpoint: string,
  error: Error,
  requestData?: any
): void {
  workflowErrorLogger.log(error, {
    type: 'network',
    endpoint,
    requestData,
    timestamp: new Date().toISOString()
  })
}

export function logValidationError(
  field: string,
  value: any,
  validationMessage: string
): void {
  const error = new WorkflowError(
    `Validation failed for ${field}: ${validationMessage}`,
    WorkflowErrorType.VALIDATION,
    { field, value, validationMessage }
  )
  workflowErrorLogger.log(error)
}

export function logPermissionError(
  action: string,
  resource: string,
  userId?: string
): void {
  const error = new WorkflowError(
    `Permission denied: ${action} on ${resource}`,
    WorkflowErrorType.PERMISSION,
    { action, resource, userId }
  )
  workflowErrorLogger.log(error)
}

// React hook for error logging
export function useErrorLogger() {
  return {
    logError: logWorkflowError,
    logNetworkError,
    logValidationError,
    logPermissionError,
    logWarning: workflowErrorLogger.logWarning.bind(workflowErrorLogger),
    logInfo: workflowErrorLogger.logInfo.bind(workflowErrorLogger),
    getRecentErrors:
      workflowErrorLogger.getRecentErrors.bind(workflowErrorLogger),
    clearErrors: workflowErrorLogger.clearErrors.bind(workflowErrorLogger)
  }
}
