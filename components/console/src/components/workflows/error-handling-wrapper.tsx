'use client'

import React, { ReactNode } from 'react'
import { WorkflowErrorBoundary } from './error-boundary'
import { WorkflowPageLoader } from './loading-states'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { AlertCircle, RefreshCw, WifiOff } from 'lucide-react'

interface ErrorHandlingWrapperProps {
  children: ReactNode
  isLoading?: boolean
  error?: Error | string | null
  onRetry?: () => void
  customErrorComponent?: ReactNode
  customLoadingComponent?: ReactNode
}

export function ErrorHandlingWrapper({
  children,
  isLoading = false,
  error = null,
  onRetry,
  customErrorComponent,
  customLoadingComponent
}: ErrorHandlingWrapperProps) {
  // Show loading state
  if (isLoading) {
    return <>{customLoadingComponent || <WorkflowPageLoader />}</>
  }

  // Show error state
  if (error) {
    if (customErrorComponent) {
      return <>{customErrorComponent}</>
    }

    const errorMessage = typeof error === 'string' ? error : error.message
    const isNetworkError =
      errorMessage.toLowerCase().includes('network') ||
      errorMessage.toLowerCase().includes('fetch')

    return (
      <div className="flex min-h-[400px] items-center justify-center p-4">
        <div className="w-full max-w-md space-y-4">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>
              {isNetworkError ? 'Connection Error' : 'Error'}
            </AlertTitle>
            <AlertDescription className="mt-2">
              {isNetworkError
                ? 'Unable to connect to the server. Please check your internet connection and try again.'
                : errorMessage || 'An unexpected error occurred'}
            </AlertDescription>
          </Alert>

          {isNetworkError && (
            <div className="flex justify-center">
              <WifiOff className="h-16 w-16 text-muted-foreground" />
            </div>
          )}

          {onRetry && (
            <div className="flex justify-center">
              <Button
                onClick={onRetry}
                variant="default"
                className="flex items-center gap-2"
              >
                <RefreshCw className="h-4 w-4" />
                Try Again
              </Button>
            </div>
          )}
        </div>
      </div>
    )
  }

  // Wrap children in error boundary
  return <WorkflowErrorBoundary>{children}</WorkflowErrorBoundary>
}

// Hook for handling async operations with error and loading states
export function useAsyncOperation<T>() {
  const [isLoading, setIsLoading] = React.useState(false)
  const [error, setError] = React.useState<Error | null>(null)
  const [data, setData] = React.useState<T | null>(null)

  const execute = React.useCallback(
    async (
      operation: () => Promise<T>,
      options?: {
        onSuccess?: (data: T) => void
        onError?: (error: Error) => void
        retryCount?: number
        retryDelay?: number
      }
    ) => {
      const {
        onSuccess,
        onError,
        retryCount = 0,
        retryDelay = 1000
      } = options || {}

      const attemptOperation = async (attempt: number): Promise<void> => {
        try {
          setIsLoading(true)
          setError(null)
          const result = await operation()
          setData(result)
          onSuccess?.(result)
        } catch (err) {
          const error =
            err instanceof Error ? err : new Error('Operation failed')

          if (attempt < retryCount) {
            console.log(
              `Retrying operation (attempt ${attempt + 1}/${retryCount})...`
            )
            await new Promise((resolve) => setTimeout(resolve, retryDelay))
            return attemptOperation(attempt + 1)
          }

          setError(error)
          onError?.(error)
        } finally {
          setIsLoading(false)
        }
      }

      return attemptOperation(0)
    },
    []
  )

  const reset = React.useCallback(() => {
    setIsLoading(false)
    setError(null)
    setData(null)
  }, [])

  const retry = React.useCallback(() => {
    if (error) {
      setError(null)
    }
  }, [error])

  return {
    isLoading,
    error,
    data,
    execute,
    reset,
    retry
  }
}

// Enhanced error types for better error handling
export enum WorkflowErrorType {
  NETWORK = 'NETWORK',
  VALIDATION = 'VALIDATION',
  PERMISSION = 'PERMISSION',
  NOT_FOUND = 'NOT_FOUND',
  SERVER = 'SERVER',
  UNKNOWN = 'UNKNOWN'
}

export class WorkflowError extends Error {
  constructor(
    message: string,
    public type: WorkflowErrorType = WorkflowErrorType.UNKNOWN,
    public details?: any
  ) {
    super(message)
    this.name = 'WorkflowError'
  }

  static fromError(error: any): WorkflowError {
    if (error instanceof WorkflowError) {
      return error
    }

    const message = error?.message || 'An unexpected error occurred'

    // Determine error type based on error details
    let type = WorkflowErrorType.UNKNOWN

    if (error?.code === 'NETWORK_ERROR' || message.includes('fetch')) {
      type = WorkflowErrorType.NETWORK
    } else if (error?.code === 'VALIDATION_ERROR' || error?.status === 400) {
      type = WorkflowErrorType.VALIDATION
    } else if (error?.code === 'PERMISSION_ERROR' || error?.status === 403) {
      type = WorkflowErrorType.PERMISSION
    } else if (error?.code === 'NOT_FOUND' || error?.status === 404) {
      type = WorkflowErrorType.NOT_FOUND
    } else if (error?.status >= 500) {
      type = WorkflowErrorType.SERVER
    }

    return new WorkflowError(message, type, error)
  }
}

// Error display component with specific handling for different error types
export function WorkflowErrorDisplay({
  error,
  onRetry
}: {
  error: WorkflowError | Error
  onRetry?: () => void
}) {
  const workflowError =
    error instanceof WorkflowError ? error : WorkflowError.fromError(error)

  const getErrorIcon = () => {
    switch (workflowError.type) {
      case WorkflowErrorType.NETWORK:
        return <WifiOff className="h-5 w-5" />
      default:
        return <AlertCircle className="h-5 w-5" />
    }
  }

  const getErrorTitle = () => {
    switch (workflowError.type) {
      case WorkflowErrorType.NETWORK:
        return 'Connection Error'
      case WorkflowErrorType.VALIDATION:
        return 'Validation Error'
      case WorkflowErrorType.PERMISSION:
        return 'Permission Denied'
      case WorkflowErrorType.NOT_FOUND:
        return 'Not Found'
      case WorkflowErrorType.SERVER:
        return 'Server Error'
      default:
        return 'Error'
    }
  }

  return (
    <Alert variant="destructive">
      <div className="flex items-start gap-3">
        {getErrorIcon()}
        <div className="flex-1">
          <AlertTitle>{getErrorTitle()}</AlertTitle>
          <AlertDescription className="mt-2">
            {workflowError.message}
          </AlertDescription>
          {onRetry && workflowError.type === WorkflowErrorType.NETWORK && (
            <Button
              onClick={onRetry}
              variant="outline"
              size="sm"
              className="mt-3"
            >
              <RefreshCw className="mr-2 h-3 w-3" />
              Retry
            </Button>
          )}
        </div>
      </div>
    </Alert>
  )
}
