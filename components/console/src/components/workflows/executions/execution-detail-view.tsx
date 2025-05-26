'use client'

import { useState, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Progress } from '@/components/ui/progress'
import {
  ArrowLeft,
  Play,
  Pause,
  Square,
  RotateCcw,
  Download,
  Activity,
  CheckCircle,
  XCircle,
  Clock,
  AlertTriangle,
  Eye,
  FileText,
  User,
  Calendar,
  Timer,
  Wifi,
  WifiOff
} from 'lucide-react'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'
import { ExecutionTimeline } from './execution-timeline'
import { ExecutionControlPanel } from './execution-control-panel'
import { TaskInspector } from './task-inspector'
import { useExecutionUpdates } from '@/hooks/use-websocket'
import { ExecutionUpdateMessage } from '@/core/infrastructure/websocket/websocket-client'
import { toast } from '@/hooks/use-toast'
import { useWebSocket } from '@/hooks/use-websocket'
import { useMediaQuery } from '@/hooks/use-media-query'
import { TaskExecution } from '@/core/domain/entities/workflow-execution'

// Import mock server in development
if (process.env.NODE_ENV === 'development') {
  import('@/core/infrastructure/websocket/mock-websocket-server')
}

interface ExecutionDetailViewProps {
  executionId: string
}

const statusColors = {
  RUNNING: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
  COMPLETED:
    'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
  FAILED: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
  TIMED_OUT:
    'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
  TERMINATED: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
  PAUSED:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200'
}

const taskStatusColors = {
  IN_PROGRESS: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
  COMPLETED:
    'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
  COMPLETED_WITH_ERRORS:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
  FAILED: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
  FAILED_WITH_TERMINAL_ERROR:
    'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
  CANCELLED: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
  TIMED_OUT:
    'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
  SCHEDULED:
    'bg-purple-100 text-purple-800 dark:bg-purple-800 dark:text-purple-200',
  SKIPPED: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
}

const statusIcons = {
  RUNNING: <Activity className="h-4 w-4 animate-pulse" />,
  COMPLETED: <CheckCircle className="h-4 w-4" />,
  FAILED: <XCircle className="h-4 w-4" />,
  TIMED_OUT: <Clock className="h-4 w-4" />,
  TERMINATED: <Square className="h-4 w-4" />,
  PAUSED: <Pause className="h-4 w-4" />
}

export function ExecutionDetailView({ executionId }: ExecutionDetailViewProps) {
  const router = useRouter()
  const [execution, setExecution] = useState<WorkflowExecution>(
    mockWorkflowExecutions.find((e) => e.executionId === executionId) ||
      mockWorkflowExecutions[0]
  )
  const [lastPolled, setLastPolled] = useState<Date>(new Date())
  const [isPolling, setIsPolling] = useState(false)
  const [lastUpdateTime, setLastUpdateTime] = useState<number | null>(null)
  const [selectedTask, setSelectedTask] = useState<TaskExecution | null>(null)
  const [showTaskInspector, setShowTaskInspector] = useState(false)
  const isMobile = useMediaQuery('(max-width: 768px)')
  const isTablet = useMediaQuery('(max-width: 1024px)')

  // Use WebSocket for real-time updates
  const { isConnected } = useExecutionUpdates(
    executionId,
    useCallback(
      (update: ExecutionUpdateMessage) => {
        // Update execution state with WebSocket data
        setExecution((prev) => ({
          ...prev,
          ...update.updates,
          status: update.status
        }))

        // Update last update time
        setLastUpdateTime(Date.now())

        // Show toast for significant status changes
        if (update.status === 'COMPLETED') {
          toast({
            title: 'Execution Completed',
            description: `Workflow execution ${executionId} has completed successfully.`
          })
        } else if (update.status === 'FAILED') {
          toast({
            variant: 'destructive',
            title: 'Execution Failed',
            description: `Workflow execution ${executionId} has failed.`
          })
        }
      },
      [executionId]
    )
  )

  // Subscribe to task updates as well
  const { subscribe } = useWebSocket()
  useEffect(() => {
    if (!isConnected) return

    const unsubscribe = subscribe('task_update', (message) => {
      if (message.data.executionId === executionId) {
        // Update specific task in the execution
        setExecution((prev) => ({
          ...prev,
          tasks: prev.tasks.map((task) =>
            task.taskId === message.data.taskId
              ? { ...task, ...message.data.updates }
              : task
          )
        }))
        // Update last update time
        setLastUpdateTime(message.timestamp)
      }
    })

    return unsubscribe
  }, [isConnected, executionId, subscribe])

  // Fallback to polling if WebSocket is not available
  useEffect(() => {
    if (!isConnected && execution.status === 'RUNNING') {
      const pollInterval = setInterval(() => {
        setIsPolling(true)

        // In a real app, this would fetch from API
        // For now, simulate updates
        console.log('Polling for updates...')
        setLastPolled(new Date())

        // Simulate completion after some time
        const runtime = Date.now() - execution.startTime
        if (runtime > 30000) {
          // 30 seconds
          setExecution((prev) => ({
            ...prev,
            status: 'COMPLETED',
            endTime: Date.now()
          }))
        }

        setIsPolling(false)
      }, 5000) // Poll every 5 seconds

      return () => clearInterval(pollInterval)
    }
  }, [isConnected, execution.status, execution.startTime])

  const formatDuration = (startTime: number, endTime?: number) => {
    if (!endTime) {
      const now = Date.now()
      const duration = Math.floor((now - startTime) / 1000)
      const minutes = Math.floor(duration / 60)
      const seconds = duration % 60
      return `${minutes}m ${seconds}s`
    }

    const duration = Math.floor((endTime - startTime) / 1000)
    const minutes = Math.floor(duration / 60)
    const seconds = duration % 60
    return `${minutes}m ${seconds}s`
  }

  const formatDate = (timestamp: number) => {
    return new Date(timestamp).toLocaleString()
  }

  const getTaskProgress = () => {
    const completedTasks = execution.tasks.filter(
      (task) =>
        task.status === 'COMPLETED' || task.status === 'COMPLETED_WITH_ERRORS'
    ).length

    return execution.tasks.length > 0
      ? (completedTasks / execution.tasks.length) * 100
      : 0
  }

  const handlePause = async () => {
    console.log('Pausing execution:', execution.executionId)
    // In a real implementation, this would call the API
    setExecution((prev) => ({ ...prev, status: 'PAUSED' }))
    toast({
      title: 'Execution paused',
      description: 'The workflow execution has been paused.'
    })
  }

  const handleResume = async () => {
    console.log('Resuming execution:', execution.executionId)
    // In a real implementation, this would call the API
    setExecution((prev) => ({ ...prev, status: 'RUNNING' }))
    toast({
      title: 'Execution resumed',
      description: 'The workflow execution has been resumed.'
    })
  }

  const handleTerminate = async (reason?: string) => {
    console.log(
      'Terminating execution:',
      execution.executionId,
      'Reason:',
      reason
    )
    // In a real implementation, this would call the API
    setExecution((prev) => ({
      ...prev,
      status: 'TERMINATED',
      endTime: Date.now(),
      reasonForIncompletion: reason || 'Manually terminated by user'
    }))
    toast({
      title: 'Execution terminated',
      description: 'The workflow execution has been terminated.',
      variant: 'destructive'
    })
  }

  const handleRetry = async (options?: any) => {
    console.log(
      'Retrying execution:',
      execution.executionId,
      'Options:',
      options
    )
    // In a real implementation, this would call the API
    toast({
      title: 'Retry initiated',
      description: 'Failed tasks are being retried.'
    })
  }

  const handleRerun = async () => {
    console.log('Rerunning workflow with same input:', execution.input)
    // In a real implementation, this would create a new execution
    router.push(
      `/plugins/workflows/executions/start?workflowId=${execution.workflowId}&input=${encodeURIComponent(JSON.stringify(execution.input))}`
    )
  }

  const canControl =
    execution.status === 'RUNNING' || execution.status === 'PAUSED'
  const canRetry =
    execution.status === 'FAILED' || execution.status === 'TIMED_OUT'

  const renderTaskDetails = () => (
    <div className="space-y-4">
      {execution.tasks.map((task) => (
        <Card key={task.taskId}>
          <CardContent className="p-4">
            <div className="mb-3 flex items-start justify-between">
              <div className="flex-1">
                <div className="mb-2 flex items-center space-x-3">
                  <h4 className="font-medium">{task.referenceTaskName}</h4>
                  <Badge
                    className={taskStatusColors[task.status]}
                    variant="secondary"
                  >
                    {task.status}
                  </Badge>
                  <Badge variant="outline">{task.taskType}</Badge>
                </div>
                <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
                  <div>
                    <p className="text-muted-foreground">Started</p>
                    <p className="font-medium">{formatDate(task.startTime)}</p>
                  </div>
                  {task.endTime && (
                    <div>
                      <p className="text-muted-foreground">Completed</p>
                      <p className="font-medium">{formatDate(task.endTime)}</p>
                    </div>
                  )}
                  <div>
                    <p className="text-muted-foreground">Duration</p>
                    <p className="font-medium">
                      {task.executionTime
                        ? `${(task.executionTime / 1000).toFixed(1)}s`
                        : 'Running...'}
                    </p>
                  </div>
                  {task.retryCount > 0 && (
                    <div>
                      <p className="text-muted-foreground">Retries</p>
                      <p className="font-medium">{task.retryCount}</p>
                    </div>
                  )}
                </div>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setSelectedTask(task)
                    setShowTaskInspector(true)
                  }}
                >
                  <Eye className="mr-1 h-3 w-3" />
                  Inspect
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    router.push(
                      `/plugins/workflows/executions/${execution.executionId}/tasks/${task.taskId}`
                    )
                  }
                >
                  Details
                </Button>
              </div>
            </div>

            {task.reasonForIncompletion && (
              <div className="rounded border border-red-200 bg-red-50 p-3">
                <div className="flex items-center space-x-2 text-red-800">
                  <AlertTriangle className="h-4 w-4" />
                  <span className="font-medium">Error</span>
                </div>
                <p className="mt-1 text-sm text-red-700">
                  {task.reasonForIncompletion}
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )

  const renderInputOutput = () => (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Input Parameters</CardTitle>
        </CardHeader>
        <CardContent>
          <pre className="overflow-auto rounded-lg bg-muted p-4 text-sm">
            {JSON.stringify(execution.input, null, 2)}
          </pre>
        </CardContent>
      </Card>

      {execution.output && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Output Data</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="overflow-auto rounded-lg bg-muted p-4 text-sm">
              {JSON.stringify(execution.output, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}

      {execution.variables && Object.keys(execution.variables).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Workflow Variables</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="overflow-auto rounded-lg bg-muted p-4 text-sm">
              {JSON.stringify(execution.variables, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}
    </div>
  )

  const progress = getTaskProgress()

  return (
    <div className="space-y-6">
      {/* Header */}
      <div
        className={`${isMobile ? 'space-y-3' : 'flex items-start justify-between'}`}
      >
        <div
          className={`${isMobile ? 'space-y-3' : 'flex items-center space-x-4'}`}
        >
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.back()}
            className="flex items-center space-x-2"
          >
            <ArrowLeft className="h-4 w-4" />
            <span>{!isMobile && 'Back'}</span>
          </Button>
          <div className="flex-1">
            <div
              className={`flex ${isMobile ? 'flex-col gap-2' : 'items-center space-x-3'} ${!isMobile && 'mb-2'}`}
            >
              <h1 className={`font-bold ${isMobile ? 'text-lg' : 'text-2xl'}`}>
                {execution.workflowName}
              </h1>
              <div className="flex flex-wrap items-center gap-2">
                <Badge
                  className={statusColors[execution.status]}
                  variant="secondary"
                >
                  <div className="flex items-center space-x-1">
                    {statusIcons[execution.status]}
                    <span>{execution.status}</span>
                  </div>
                </Badge>
                {/* Connection Status Indicator */}
                <div className="flex items-center space-x-1">
                  {isConnected ? (
                    <>
                      <Wifi className="h-4 w-4 text-green-500" />
                      <span className="text-xs text-green-600">Live</span>
                    </>
                  ) : execution.status === 'RUNNING' ? (
                    <>
                      <WifiOff className="h-4 w-4 text-yellow-500" />
                      <span className="text-xs text-yellow-600">
                        Polling {isPolling && '...'}
                      </span>
                    </>
                  ) : null}
                </div>
              </div>
            </div>
            <p
              className={`text-muted-foreground ${isMobile ? 'text-xs' : 'text-sm'}`}
            >
              {isMobile ? (
                <>
                  ID: {execution.executionId.slice(-8)} • v
                  {execution.workflowVersion}
                </>
              ) : (
                <>
                  Execution ID: {execution.executionId} • Version:{' '}
                  {execution.workflowVersion}
                </>
              )}
              {!isConnected && execution.status === 'RUNNING' && (
                <span className="ml-2 text-xs">
                  • Last checked: {lastPolled.toLocaleTimeString()}
                </span>
              )}
            </p>
          </div>
        </div>

        <div
          className={`flex items-center ${isMobile ? 'justify-between gap-2' : 'space-x-2'}`}
        >
          {canControl && (
            <>
              {execution.status === 'RUNNING' && (
                <Button
                  variant="outline"
                  onClick={handlePause}
                  size={isMobile ? 'sm' : 'default'}
                >
                  <Pause
                    className={`${isMobile ? 'h-4 w-4' : 'mr-2 h-4 w-4'}`}
                  />
                  {!isMobile && 'Pause'}
                </Button>
              )}
              {execution.status === 'PAUSED' && (
                <Button
                  variant="outline"
                  onClick={handleResume}
                  size={isMobile ? 'sm' : 'default'}
                >
                  <Play
                    className={`${isMobile ? 'h-4 w-4' : 'mr-2 h-4 w-4'}`}
                  />
                  {!isMobile && 'Resume'}
                </Button>
              )}
              <Button
                variant="destructive"
                onClick={() => handleTerminate()}
                size={isMobile ? 'sm' : 'default'}
              >
                <Square
                  className={`${isMobile ? 'h-4 w-4' : 'mr-2 h-4 w-4'}`}
                />
                {!isMobile && 'Terminate'}
              </Button>
            </>
          )}

          {canRetry && (
            <Button
              onClick={() => handleRetry()}
              size={isMobile ? 'sm' : 'default'}
            >
              <RotateCcw
                className={`${isMobile ? 'h-4 w-4' : 'mr-2 h-4 w-4'}`}
              />
              {!isMobile && 'Retry'}
            </Button>
          )}

          <Button variant="outline" size={isMobile ? 'sm' : 'default'}>
            <Download className={`${isMobile ? 'h-4 w-4' : 'mr-2 h-4 w-4'}`} />
            {!isMobile && 'Export'}
          </Button>
        </div>
      </div>

      {/* Quick Stats */}
      <div
        className={`grid gap-4 ${isMobile ? 'grid-cols-2' : 'grid-cols-1 md:grid-cols-4'}`}
      >
        <Card>
          <CardContent className={`${isMobile ? 'p-3' : 'p-4'}`}>
            <div className="flex items-center space-x-2">
              <Timer
                className={`text-blue-500 ${isMobile ? 'h-4 w-4' : 'h-5 w-5'}`}
              />
              <div className="min-w-0">
                <p
                  className={`text-muted-foreground ${isMobile ? 'text-xs' : 'text-sm'}`}
                >
                  Duration
                </p>
                <p
                  className={`font-bold ${isMobile ? 'truncate text-sm' : 'text-xl'}`}
                >
                  {formatDuration(execution.startTime, execution.endTime)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className={`${isMobile ? 'p-3' : 'p-4'}`}>
            <div className="flex items-center space-x-2">
              <Activity
                className={`text-orange-500 ${isMobile ? 'h-4 w-4' : 'h-5 w-5'}`}
              />
              <div className="min-w-0">
                <p
                  className={`text-muted-foreground ${isMobile ? 'text-xs' : 'text-sm'}`}
                >
                  Progress
                </p>
                <div className="mt-1 flex items-center space-x-2">
                  <Progress
                    value={progress}
                    className={`${isMobile ? 'h-1.5 w-12' : 'h-2 w-16'}`}
                  />
                  <span
                    className={`font-medium ${isMobile ? 'text-xs' : 'text-sm'}`}
                  >
                    {progress.toFixed(0)}%
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className={`${isMobile ? 'p-3' : 'p-4'}`}>
            <div className="flex items-center space-x-2">
              <User
                className={`text-purple-500 ${isMobile ? 'h-4 w-4' : 'h-5 w-5'}`}
              />
              <div className="min-w-0">
                <p
                  className={`text-muted-foreground ${isMobile ? 'text-xs' : 'text-sm'}`}
                >
                  Created By
                </p>
                <p
                  className={`truncate font-bold ${isMobile ? 'text-sm' : 'text-lg'}`}
                >
                  {execution.createdBy}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className={`${isMobile ? 'p-3' : 'p-4'}`}>
            <div className="flex items-center space-x-2">
              <Calendar
                className={`text-green-500 ${isMobile ? 'h-4 w-4' : 'h-5 w-5'}`}
              />
              <div className="min-w-0">
                <p
                  className={`text-muted-foreground ${isMobile ? 'text-xs' : 'text-sm'}`}
                >
                  Started
                </p>
                <p
                  className={`truncate font-medium ${isMobile ? 'text-xs' : 'text-sm'}`}
                >
                  {formatDate(execution.startTime)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Error Information */}
      {execution.reasonForIncompletion && (
        <Card className="border-red-200">
          <CardHeader>
            <CardTitle className="flex items-center space-x-2 text-base text-red-800">
              <AlertTriangle className="h-5 w-5" />
              <span>Execution Failed</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-red-700">{execution.reasonForIncompletion}</p>
            {execution.failedReferenceTaskNames &&
              execution.failedReferenceTaskNames.length > 0 && (
                <div className="mt-2">
                  <p className="text-sm text-red-600">Failed tasks:</p>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {execution.failedReferenceTaskNames.map((taskName) => (
                      <Badge
                        key={taskName}
                        variant="destructive"
                        className="text-xs"
                      >
                        {taskName}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
          </CardContent>
        </Card>
      )}

      {/* Main Content */}
      <Tabs defaultValue="timeline" className="space-y-6">
        <TabsList>
          <TabsTrigger value="timeline">Timeline</TabsTrigger>
          <TabsTrigger value="tasks">Tasks</TabsTrigger>
          <TabsTrigger value="control">Control</TabsTrigger>
          <TabsTrigger value="data">Input/Output</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="timeline">
          <ExecutionTimeline execution={execution} />
        </TabsContent>

        <TabsContent value="tasks">{renderTaskDetails()}</TabsContent>

        <TabsContent value="control">
          <ExecutionControlPanel
            execution={execution}
            onPause={handlePause}
            onResume={handleResume}
            onTerminate={handleTerminate}
            onRetry={handleRetry}
            onRerun={handleRerun}
          />
        </TabsContent>

        <TabsContent value="data">{renderInputOutput()}</TabsContent>

        <TabsContent value="logs">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle>Execution Logs</CardTitle>
                  <CardDescription>
                    Detailed execution logs and debug information
                  </CardDescription>
                </div>
                {isConnected && execution.status === 'RUNNING' && (
                  <div className="flex items-center space-x-2">
                    <div className="h-2 w-2 animate-pulse rounded-full bg-green-500" />
                    <span className="text-sm text-green-600">
                      Streaming logs...
                    </span>
                  </div>
                )}
              </div>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center text-muted-foreground">
                <FileText className="mx-auto mb-4 h-12 w-12 opacity-50" />
                <p>Execution logs will appear here</p>
                <p className="text-sm">
                  {isConnected
                    ? 'Real-time log streaming active'
                    : 'Real-time log streaming unavailable - falling back to polling'}
                </p>
                {lastUpdateTime && (
                  <p className="mt-2 text-xs">
                    Last update: {new Date(lastUpdateTime).toLocaleTimeString()}
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Task Inspector Dialog */}
      {selectedTask && (
        <TaskInspector
          task={selectedTask}
          open={showTaskInspector}
          onOpenChange={setShowTaskInspector}
        />
      )}
    </div>
  )
}
