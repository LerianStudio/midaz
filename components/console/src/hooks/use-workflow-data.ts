'use client'

import { useState, useEffect, useCallback } from 'react'
import { useToast } from '@/hooks/use-toast'
import { Workflow } from '@/core/domain/entities/workflow'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import {
  fetchWorkflowByIdActionEnhanced,
  updateWorkflowActionEnhanced,
  createWorkflowActionEnhanced,
  ActionError
} from '@/core/application/use-cases/workflows/workflow-server-actions-enhanced'

export interface UseWorkflowDataOptions {
  workflowId?: string
  autoFetch?: boolean
  onSuccess?: (workflow: Workflow) => void
  onError?: (error: ActionError) => void
  retryOnError?: boolean
}

export interface UseWorkflowDataReturn {
  workflow: Workflow | null
  isLoading: boolean
  isSaving: boolean
  error: ActionError | null
  refetch: () => Promise<void>
  update: (updates: Partial<Workflow>) => Promise<void>
  create: (workflow: Partial<Workflow>) => Promise<Workflow | null>
  clearError: () => void
}

export function useWorkflowData({
  workflowId,
  autoFetch = true,
  onSuccess,
  onError,
  retryOnError = true
}: UseWorkflowDataOptions = {}): UseWorkflowDataReturn {
  const { toast } = useToast()
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState<ActionError | null>(null)
  const [retryCount, setRetryCount] = useState(0)

  // Fetch workflow data
  const fetchWorkflow = useCallback(async () => {
    if (!workflowId) return

    try {
      setIsLoading(true)
      setError(null)

      const result = await fetchWorkflowByIdActionEnhanced({ workflowId })

      if (result.success && result.data) {
        setWorkflow(result.data)
        onSuccess?.(result.data)
        setRetryCount(0)
      } else if (result.error) {
        setError(result.error)
        onError?.(result.error)

        // Auto-retry for network errors
        if (
          retryOnError &&
          result.error.type === 'NETWORK_ERROR' &&
          retryCount < 3
        ) {
          setTimeout(
            () => {
              setRetryCount((prev) => prev + 1)
              fetchWorkflow()
            },
            1000 * Math.pow(2, retryCount)
          ) // Exponential backoff
        }
      }
    } catch (err) {
      const error: ActionError = {
        type: 'UNKNOWN_ERROR',
        message: err instanceof Error ? err.message : 'Failed to fetch workflow'
      }
      setError(error)
      onError?.(error)
    } finally {
      setIsLoading(false)
    }
  }, [workflowId, onSuccess, onError, retryOnError, retryCount])

  // Update workflow
  const updateWorkflow = useCallback(
    async (updates: Partial<Workflow>) => {
      if (!workflowId || !workflow) return

      try {
        setIsSaving(true)
        setError(null)

        const result = await updateWorkflowActionEnhanced({
          workflowId,
          workflow: { ...workflow, ...updates }
        })

        if (result.success && result.data) {
          setWorkflow(result.data)
          toast({
            title: 'Workflow updated',
            description: 'Your changes have been saved successfully'
          })
        } else if (result.error) {
          setError(result.error)

          // Show appropriate error message based on error type
          let errorTitle = 'Update failed'
          let errorDescription = result.error.message

          switch (result.error.type) {
            case 'VALIDATION_ERROR':
              errorTitle = 'Validation failed'
              break
            case 'PERMISSION_ERROR':
              errorTitle = 'Permission denied'
              break
            case 'NETWORK_ERROR':
              errorTitle = 'Connection error'
              errorDescription =
                'Please check your internet connection and try again'
              break
          }

          toast({
            title: errorTitle,
            description: errorDescription,
            variant: 'destructive'
          })
        }
      } catch (err) {
        const error: ActionError = {
          type: 'UNKNOWN_ERROR',
          message:
            err instanceof Error ? err.message : 'Failed to update workflow'
        }
        setError(error)
        toast({
          title: 'Update failed',
          description: error.message,
          variant: 'destructive'
        })
      } finally {
        setIsSaving(false)
      }
    },
    [workflowId, workflow, toast]
  )

  // Create new workflow
  const createWorkflow = useCallback(
    async (newWorkflow: Partial<Workflow>): Promise<Workflow | null> => {
      try {
        setIsSaving(true)
        setError(null)

        const result = await createWorkflowActionEnhanced({
          workflow: newWorkflow as any // Type assertion for partial workflow
        })

        if (result.success && result.data) {
          setWorkflow(result.data)
          toast({
            title: 'Workflow created',
            description: 'Your new workflow has been created successfully'
          })
          return result.data
        } else if (result.error) {
          setError(result.error)
          toast({
            title: 'Creation failed',
            description: result.error.message,
            variant: 'destructive'
          })
          return null
        }

        return null
      } catch (err) {
        const error: ActionError = {
          type: 'UNKNOWN_ERROR',
          message:
            err instanceof Error ? err.message : 'Failed to create workflow'
        }
        setError(error)
        toast({
          title: 'Creation failed',
          description: error.message,
          variant: 'destructive'
        })
        return null
      } finally {
        setIsSaving(false)
      }
    },
    [toast]
  )

  // Clear error
  const clearError = useCallback(() => {
    setError(null)
    setRetryCount(0)
  }, [])

  // Auto-fetch on mount or workflowId change
  useEffect(() => {
    if (autoFetch && workflowId) {
      fetchWorkflow()
    }
  }, [workflowId, autoFetch])

  return {
    workflow,
    isLoading,
    isSaving,
    error,
    refetch: fetchWorkflow,
    update: updateWorkflow,
    create: createWorkflow,
    clearError
  }
}

// Hook for managing workflow executions
export function useWorkflowExecutions(workflowId?: string) {
  const [executions, setExecutions] = useState<WorkflowExecution[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<ActionError | null>(null)

  const fetchExecutions = useCallback(async () => {
    if (!workflowId) return

    try {
      setIsLoading(true)
      setError(null)

      // This would call a server action to fetch executions
      // const result = await fetchWorkflowExecutionsAction({ workflowId })

      // For now, simulate the response
      await new Promise((resolve) => setTimeout(resolve, 1000))
      setExecutions([])
    } catch (err) {
      const error: ActionError = {
        type: 'UNKNOWN_ERROR',
        message:
          err instanceof Error ? err.message : 'Failed to fetch executions'
      }
      setError(error)
    } finally {
      setIsLoading(false)
    }
  }, [workflowId])

  useEffect(() => {
    if (workflowId) {
      fetchExecutions()
    }
  }, [workflowId, fetchExecutions])

  return {
    executions,
    isLoading,
    error,
    refetch: fetchExecutions
  }
}

// Hook for real-time execution monitoring
export function useExecutionMonitoring(executionId?: string) {
  const [execution, setExecution] = useState<WorkflowExecution | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const [connectionError, setConnectionError] = useState<string | null>(null)

  useEffect(() => {
    if (!executionId) return

    // In a real implementation, this would establish a WebSocket connection
    // or use Server-Sent Events for real-time updates

    const connect = () => {
      try {
        setIsConnected(true)
        setConnectionError(null)

        // Simulate real-time updates
        const interval = setInterval(() => {
          // Update execution state
        }, 1000)

        return () => {
          clearInterval(interval)
          setIsConnected(false)
        }
      } catch (error) {
        setConnectionError('Failed to establish real-time connection')
        setIsConnected(false)
      }
    }

    const cleanup = connect()

    return () => {
      cleanup?.()
    }
  }, [executionId])

  return {
    execution,
    isConnected,
    connectionError
  }
}
