'use client'

import { useParams } from 'next/navigation'
import { WorkflowErrorBoundaryWrapper } from '@/components/workflows/error-boundary'
import { ErrorHandlingWrapper } from '@/components/workflows/error-handling-wrapper'
import { ExecutionDetailSkeleton } from '@/components/workflows/loading-states'
import { ExecutionDetailViewEnhanced } from '@/components/workflows/executions/execution-detail-view-enhanced'
import {
  useWorkflowExecutions,
  useExecutionMonitoring
} from '@/hooks/use-workflow-data'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { WifiOff, RefreshCw } from 'lucide-react'

function ExecutionDetailContent() {
  const params = useParams()
  const executionId = params.id as string

  // Use custom hooks for data fetching
  const { executions, isLoading, error, refetch } =
    useWorkflowExecutions(executionId)
  const { isConnected, connectionError } = useExecutionMonitoring(executionId)

  // Get the specific execution
  const execution = executions.find((e) => e.id === executionId)

  return (
    <ErrorHandlingWrapper
      isLoading={isLoading}
      error={error}
      onRetry={refetch}
      customLoadingComponent={<ExecutionDetailSkeleton />}
    >
      <div className="space-y-4 p-6">
        {/* Connection status alert */}
        {!isConnected && connectionError && (
          <Alert variant="warning">
            <WifiOff className="h-4 w-4" />
            <AlertTitle>Real-time updates unavailable</AlertTitle>
            <AlertDescription className="flex items-center justify-between">
              <span>{connectionError}</span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => window.location.reload()}
              >
                <RefreshCw className="mr-2 h-3 w-3" />
                Reconnect
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {/* Execution detail view */}
        {execution ? (
          <ExecutionDetailViewEnhanced
            execution={execution}
            isRealTimeEnabled={isConnected}
          />
        ) : (
          <div className="py-12 text-center">
            <p className="text-muted-foreground">Execution not found</p>
          </div>
        )}
      </div>
    </ErrorHandlingWrapper>
  )
}

export default function ExecutionDetailPage() {
  return (
    <WorkflowErrorBoundaryWrapper
      onError={(error, errorInfo) => {
        // Log to error tracking service
        console.error('Execution detail page error:', error, errorInfo)

        // You could also send this to a monitoring service like Sentry
        // Sentry.captureException(error, { contexts: { react: errorInfo } })
      }}
    >
      <ExecutionDetailContent />
    </WorkflowErrorBoundaryWrapper>
  )
}
