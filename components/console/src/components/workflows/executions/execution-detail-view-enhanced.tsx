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
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger
} from '@/components/ui/alert-dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/hooks/use-toast'
import { Skeleton } from '@/components/ui/skeleton'
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
  RefreshCw,
  Loader2,
  ChevronRight,
  ChevronDown
} from 'lucide-react'
import {
  WorkflowExecution,
  TaskExecution
} from '@/core/domain/entities/workflow-execution'
import {
  getWorkflowExecutionById,
  pauseWorkflowExecution,
  resumeWorkflowExecution,
  terminateWorkflowExecution,
  retryWorkflowExecution
} from '@/app/actions/workflows-enhanced'
import { ExecutionTimeline } from './execution-timeline-enhanced'
import { useAutoRefresh } from '@/hooks/use-auto-refresh'

interface ExecutionDetailViewEnhancedProps {
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

export function ExecutionDetailViewEnhanced({
  executionId
}: ExecutionDetailViewEnhancedProps) {
  const router = useRouter()
  const [execution, setExecution] = useState<WorkflowExecution | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [terminationReason, setTerminationReason] = useState('')
  const [expandedTasks, setExpandedTasks] = useState<Set<string>>(new Set())

  const fetchExecution = useCallback(async () => {
    if (isRefreshing) return

    setIsRefreshing(true)
    const result = await getWorkflowExecutionById(executionId)

    if (result.success && result.data) {
      setExecution(result.data)
      setError(null)
    } else {
      setError(result.error || 'Failed to fetch execution')
    }

    setIsRefreshing(false)
    setLoading(false)
  }, [executionId, isRefreshing])

  // Auto-refresh for running executions
  const shouldAutoRefresh =
    execution?.status === 'RUNNING' || execution?.status === 'PAUSED'
  useAutoRefresh({
    enabled: shouldAutoRefresh,
    interval: 3000,
    onRefresh: fetchExecution
  })

  useEffect(() => {
    fetchExecution()
  }, [fetchExecution])

  const formatDuration = (startTime: number, endTime?: number) => {
    if (!endTime) {
      const now = Date.now()
      const duration = Math.floor((now - startTime) / 1000)
      const hours = Math.floor(duration / 3600)
      const minutes = Math.floor((duration % 3600) / 60)
      const seconds = duration % 60

      if (hours > 0) {
        return `${hours}h ${minutes}m ${seconds}s`
      }
      return `${minutes}m ${seconds}s`
    }

    const duration = Math.floor((endTime - startTime) / 1000)
    const hours = Math.floor(duration / 3600)
    const minutes = Math.floor((duration % 3600) / 60)
    const seconds = duration % 60

    if (hours > 0) {
      return `${hours}h ${minutes}m ${seconds}s`
    }
    return `${minutes}m ${seconds}s`
  }

  const formatDate = (timestamp: number) => {
    return new Date(timestamp).toLocaleString()
  }

  const getTaskProgress = () => {
    if (!execution || !execution.tasks || execution.tasks.length === 0) return 0

    const completedTasks = execution.tasks.filter(
      (task) =>
        task.status === 'COMPLETED' || task.status === 'COMPLETED_WITH_ERRORS'
    ).length

    return (completedTasks / execution.tasks.length) * 100
  }

  const handlePause = async () => {
    if (!execution) return

    const result = await pauseWorkflowExecution(execution.executionId)
    if (result.success) {
      toast({
        title: 'Execution paused',
        description: 'The workflow execution has been paused successfully.'
      })
      fetchExecution()
    } else {
      toast({
        title: 'Failed to pause execution',
        description: result.error,
        variant: 'destructive'
      })
    }
  }

  const handleResume = async () => {
    if (!execution) return

    const result = await resumeWorkflowExecution(execution.executionId)
    if (result.success) {
      toast({
        title: 'Execution resumed',
        description: 'The workflow execution has been resumed successfully.'
      })
      fetchExecution()
    } else {
      toast({
        title: 'Failed to resume execution',
        description: result.error,
        variant: 'destructive'
      })
    }
  }

  const handleTerminate = async () => {
    if (!execution || !terminationReason.trim()) return

    const result = await terminateWorkflowExecution(
      execution.executionId,
      terminationReason
    )
    if (result.success) {
      toast({
        title: 'Execution terminated',
        description: 'The workflow execution has been terminated successfully.'
      })
      setTerminationReason('')
      fetchExecution()
    } else {
      toast({
        title: 'Failed to terminate execution',
        description: result.error,
        variant: 'destructive'
      })
    }
  }

  const handleRetry = async () => {
    if (!execution) return

    const result = await retryWorkflowExecution(execution.executionId)
    if (result.success && result.data) {
      toast({
        title: 'Execution retried',
        description: 'A new execution has been started.'
      })
      router.push(`/plugins/workflows/executions/${result.data.executionId}`)
    } else {
      toast({
        title: 'Failed to retry execution',
        description: result.error,
        variant: 'destructive'
      })
    }
  }

  const handleExport = () => {
    if (!execution) return

    const dataStr = JSON.stringify(execution, null, 2)
    const dataUri =
      'data:application/json;charset=utf-8,' + encodeURIComponent(dataStr)

    const exportFileDefaultName = `workflow-execution-${execution.executionId}.json`

    const linkElement = document.createElement('a')
    linkElement.setAttribute('href', dataUri)
    linkElement.setAttribute('download', exportFileDefaultName)
    linkElement.click()
  }

  const toggleTaskExpanded = (taskId: string) => {
    const newExpanded = new Set(expandedTasks)
    if (newExpanded.has(taskId)) {
      newExpanded.delete(taskId)
    } else {
      newExpanded.add(taskId)
    }
    setExpandedTasks(newExpanded)
  }

  if (loading) {
    return <ExecutionDetailSkeleton />
  }

  if (error || !execution) {
    return (
      <Card className="border-red-200">
        <CardContent className="py-8 text-center">
          <XCircle className="mx-auto mb-4 h-12 w-12 text-red-500" />
          <h3 className="mb-2 text-lg font-semibold text-red-700">
            Failed to load execution
          </h3>
          <p className="text-red-600">{error || 'Execution not found'}</p>
          <Button
            variant="outline"
            onClick={() => router.back()}
            className="mt-4"
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Go Back
          </Button>
        </CardContent>
      </Card>
    )
  }

  const canControl =
    execution.status === 'RUNNING' || execution.status === 'PAUSED'
  const canRetry =
    execution.status === 'FAILED' || execution.status === 'TIMED_OUT'
  const progress = getTaskProgress()

  const renderTaskDetails = () => (
    <div className="space-y-4">
      {execution.tasks.map((task) => {
        const isExpanded = expandedTasks.has(task.taskId)

        return (
          <Card key={task.taskId} className="overflow-hidden">
            <CardContent className="p-0">
              <div
                className="cursor-pointer p-4 hover:bg-muted/50"
                onClick={() => toggleTaskExpanded(task.taskId)}
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="mb-2 flex items-center space-x-3">
                      <Button variant="ghost" size="sm" className="h-5 w-5 p-0">
                        {isExpanded ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </Button>
                      <h4 className="font-medium">{task.referenceTaskName}</h4>
                      <Badge
                        className={taskStatusColors[task.status]}
                        variant="secondary"
                      >
                        {task.status}
                      </Badge>
                      <Badge variant="outline">{task.taskType}</Badge>
                    </div>
                    <div className="ml-7 grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
                      <div>
                        <p className="text-muted-foreground">Started</p>
                        <p className="font-medium">
                          {formatDate(task.startTime)}
                        </p>
                      </div>
                      {task.endTime && (
                        <div>
                          <p className="text-muted-foreground">Completed</p>
                          <p className="font-medium">
                            {formatDate(task.endTime)}
                          </p>
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
                  <Dialog>
                    <DialogTrigger asChild>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={(e) => {
                          e.stopPropagation()
                        }}
                      >
                        <Eye className="mr-1 h-3 w-3" />
                        Details
                      </Button>
                    </DialogTrigger>
                    <DialogContent className="max-w-3xl">
                      <DialogHeader>
                        <DialogTitle>
                          Task Details: {task.referenceTaskName}
                        </DialogTitle>
                        <DialogDescription>
                          View input and output data for this task
                        </DialogDescription>
                      </DialogHeader>
                      <div className="space-y-4">
                        <Tabs defaultValue="input">
                          <TabsList className="grid w-full grid-cols-2">
                            <TabsTrigger value="input">Input</TabsTrigger>
                            <TabsTrigger value="output">Output</TabsTrigger>
                          </TabsList>
                          <TabsContent value="input" className="mt-4">
                            <ScrollArea className="h-[400px] w-full rounded-md border p-4">
                              <pre className="text-sm">
                                {JSON.stringify(task.inputData || {}, null, 2)}
                              </pre>
                            </ScrollArea>
                          </TabsContent>
                          <TabsContent value="output" className="mt-4">
                            <ScrollArea className="h-[400px] w-full rounded-md border p-4">
                              <pre className="text-sm">
                                {JSON.stringify(task.outputData || {}, null, 2)}
                              </pre>
                            </ScrollArea>
                          </TabsContent>
                        </Tabs>
                      </div>
                    </DialogContent>
                  </Dialog>
                </div>

                {task.reasonForIncompletion && (
                  <div className="ml-7 mt-3 rounded border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-950">
                    <div className="flex items-center space-x-2 text-red-800 dark:text-red-200">
                      <AlertTriangle className="h-4 w-4" />
                      <span className="font-medium">Error</span>
                    </div>
                    <p className="mt-1 text-sm text-red-700 dark:text-red-300">
                      {task.reasonForIncompletion}
                    </p>
                  </div>
                )}
              </div>

              {isExpanded && (
                <div className="border-t bg-muted/30 p-4">
                  <div className="grid gap-4 md:grid-cols-2">
                    <div>
                      <h5 className="mb-2 font-medium">Task Properties</h5>
                      <dl className="space-y-1 text-sm">
                        <div className="flex justify-between">
                          <dt className="text-muted-foreground">Task ID:</dt>
                          <dd className="font-mono">{task.taskId}</dd>
                        </div>
                        <div className="flex justify-between">
                          <dt className="text-muted-foreground">Sequence:</dt>
                          <dd>{task.seq}</dd>
                        </div>
                        {task.workerId && (
                          <div className="flex justify-between">
                            <dt className="text-muted-foreground">
                              Worker ID:
                            </dt>
                            <dd className="font-mono">{task.workerId}</dd>
                          </div>
                        )}
                        {task.domain && (
                          <div className="flex justify-between">
                            <dt className="text-muted-foreground">Domain:</dt>
                            <dd>{task.domain}</dd>
                          </div>
                        )}
                      </dl>
                    </div>
                    <div>
                      <h5 className="mb-2 font-medium">Timing Information</h5>
                      <dl className="space-y-1 text-sm">
                        {task.scheduledTime && (
                          <div className="flex justify-between">
                            <dt className="text-muted-foreground">
                              Scheduled:
                            </dt>
                            <dd>{formatDate(task.scheduledTime)}</dd>
                          </div>
                        )}
                        {task.updateTime && (
                          <div className="flex justify-between">
                            <dt className="text-muted-foreground">
                              Last Updated:
                            </dt>
                            <dd>{formatDate(task.updateTime)}</dd>
                          </div>
                        )}
                        {task.callbackAfterSeconds && (
                          <div className="flex justify-between">
                            <dt className="text-muted-foreground">
                              Callback After:
                            </dt>
                            <dd>{task.callbackAfterSeconds}s</dd>
                          </div>
                        )}
                      </dl>
                    </div>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        )
      })}
    </div>
  )

  const renderInputOutput = () => (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Input Parameters</CardTitle>
        </CardHeader>
        <CardContent>
          <ScrollArea className="h-[300px] w-full">
            <pre className="overflow-auto rounded-lg bg-muted p-4 text-sm">
              {JSON.stringify(execution.input, null, 2)}
            </pre>
          </ScrollArea>
        </CardContent>
      </Card>

      {execution.output && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Output Data</CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[300px] w-full">
              <pre className="overflow-auto rounded-lg bg-muted p-4 text-sm">
                {JSON.stringify(execution.output, null, 2)}
              </pre>
            </ScrollArea>
          </CardContent>
        </Card>
      )}

      {execution.variables && Object.keys(execution.variables).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Workflow Variables</CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[300px] w-full">
              <pre className="overflow-auto rounded-lg bg-muted p-4 text-sm">
                {JSON.stringify(execution.variables, null, 2)}
              </pre>
            </ScrollArea>
          </CardContent>
        </Card>
      )}
    </div>
  )

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-center space-x-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.back()}
            className="flex items-center space-x-2"
          >
            <ArrowLeft className="h-4 w-4" />
            <span>Back</span>
          </Button>
          <div>
            <div className="mb-2 flex items-center space-x-3">
              <h1 className="text-2xl font-bold">{execution.workflowName}</h1>
              <Badge
                className={statusColors[execution.status]}
                variant="secondary"
              >
                <div className="flex items-center space-x-1">
                  {statusIcons[execution.status]}
                  <span>{execution.status}</span>
                </div>
              </Badge>
              {isRefreshing && (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
            </div>
            <p className="text-muted-foreground">
              Execution ID: {execution.executionId} • Version:{' '}
              {execution.workflowVersion}
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <Button
            variant="outline"
            size="sm"
            onClick={fetchExecution}
            disabled={isRefreshing}
          >
            <RefreshCw
              className={`mr-2 h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`}
            />
            Refresh
          </Button>

          {canControl && (
            <>
              {execution.status === 'RUNNING' && (
                <Button variant="outline" onClick={handlePause}>
                  <Pause className="mr-2 h-4 w-4" />
                  Pause
                </Button>
              )}
              {execution.status === 'PAUSED' && (
                <Button variant="outline" onClick={handleResume}>
                  <Play className="mr-2 h-4 w-4" />
                  Resume
                </Button>
              )}
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button variant="destructive">
                    <Square className="mr-2 h-4 w-4" />
                    Terminate
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Terminate Workflow Execution
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      This action will permanently terminate the workflow
                      execution. This cannot be undone.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <div className="my-4">
                    <Label htmlFor="termination-reason">
                      Reason for termination
                    </Label>
                    <Textarea
                      id="termination-reason"
                      value={terminationReason}
                      onChange={(e) => setTerminationReason(e.target.value)}
                      placeholder="Enter a reason for terminating this execution..."
                      className="mt-2"
                    />
                  </div>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={handleTerminate}
                      disabled={!terminationReason.trim()}
                      className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                    >
                      Terminate Execution
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </>
          )}

          {canRetry && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button>
                  <RotateCcw className="mr-2 h-4 w-4" />
                  Retry
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Retry Workflow Execution</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will create a new execution with the same input
                    parameters. Do you want to continue?
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleRetry}>
                    Start New Execution
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}

          <Button variant="outline" onClick={handleExport}>
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
        </div>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Timer className="h-5 w-5 text-blue-500" />
              <div>
                <p className="text-sm text-muted-foreground">Duration</p>
                <p className="text-xl font-bold">
                  {formatDuration(execution.startTime, execution.endTime)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Activity className="h-5 w-5 text-orange-500" />
              <div>
                <p className="text-sm text-muted-foreground">Tasks Progress</p>
                <div className="mt-1 flex items-center space-x-2">
                  <Progress value={progress} className="h-2 w-16" />
                  <span className="text-sm font-medium">
                    {progress.toFixed(0)}%
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <User className="h-5 w-5 text-purple-500" />
              <div>
                <p className="text-sm text-muted-foreground">Created By</p>
                <p className="text-lg font-bold">{execution.createdBy}</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Calendar className="h-5 w-5 text-green-500" />
              <div>
                <p className="text-sm text-muted-foreground">Started</p>
                <p className="text-sm font-medium">
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
          <TabsTrigger value="data">Input/Output</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="timeline" className="mt-0">
          <ExecutionTimeline execution={execution} />
        </TabsContent>

        <TabsContent value="tasks">{renderTaskDetails()}</TabsContent>

        <TabsContent value="data">{renderInputOutput()}</TabsContent>

        <TabsContent value="logs">
          <Card>
            <CardHeader>
              <CardTitle>Execution Logs</CardTitle>
              <CardDescription>
                Detailed execution logs and debug information
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center text-muted-foreground">
                <FileText className="mx-auto mb-4 h-12 w-12 opacity-50" />
                <p>Execution logs will appear here</p>
                <p className="text-sm">
                  Real-time log streaming integration needed
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function ExecutionDetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div className="flex items-center space-x-4">
          <Skeleton className="h-8 w-20" />
          <div>
            <Skeleton className="mb-2 h-8 w-64" />
            <Skeleton className="h-4 w-96" />
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <Skeleton className="h-10 w-24" />
          <Skeleton className="h-10 w-24" />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        {[1, 2, 3, 4].map((i) => (
          <Card key={i}>
            <CardContent className="p-4">
              <Skeleton className="mb-2 h-4 w-20" />
              <Skeleton className="h-6 w-32" />
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardContent className="p-6">
          <Skeleton className="mb-4 h-8 w-48" />
          <div className="space-y-2">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
