'use client'

import { useState } from 'react'
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
import { Separator } from '@/components/ui/separator'
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
  Code,
  FileText,
  User,
  Calendar,
  Timer
} from 'lucide-react'
import {
  WorkflowExecution,
  TaskExecution
} from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'
import { ExecutionTimeline } from './execution-timeline'

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
  const [execution] = useState<WorkflowExecution>(
    mockWorkflowExecutions.find((e) => e.executionId === executionId) ||
      mockWorkflowExecutions[0]
  )

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

  const handlePause = () => {
    console.log('Pausing execution:', execution.executionId)
  }

  const handleResume = () => {
    console.log('Resuming execution:', execution.executionId)
  }

  const handleTerminate = () => {
    if (confirm('Are you sure you want to terminate this execution?')) {
      console.log('Terminating execution:', execution.executionId)
    }
  }

  const handleRetry = () => {
    console.log('Retrying execution:', execution.executionId)
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
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  router.push(
                    `/plugins/workflows/executions/${execution.executionId}/tasks/${task.taskId}`
                  )
                }
              >
                <Eye className="mr-1 h-3 w-3" />
                Details
              </Button>
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
            </div>
            <p className="text-muted-foreground">
              Execution ID: {execution.executionId} • Version:{' '}
              {execution.workflowVersion}
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
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
              <Button variant="destructive" onClick={handleTerminate}>
                <Square className="mr-2 h-4 w-4" />
                Terminate
              </Button>
            </>
          )}

          {canRetry && (
            <Button onClick={handleRetry}>
              <RotateCcw className="mr-2 h-4 w-4" />
              Retry
            </Button>
          )}

          <Button variant="outline">
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

        <TabsContent value="timeline">
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
