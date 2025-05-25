'use client'

import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import {
  CheckCircle,
  XCircle,
  Clock,
  Activity,
  AlertTriangle,
  Play,
  Square,
  ChevronRight,
  Eye
} from 'lucide-react'
import {
  WorkflowExecution,
  TaskExecution
} from '@/core/domain/entities/workflow-execution'

interface ExecutionTimelineProps {
  execution: WorkflowExecution
}

const taskStatusIcons = {
  IN_PROGRESS: <Activity className="h-4 w-4 animate-pulse text-blue-500" />,
  COMPLETED: <CheckCircle className="h-4 w-4 text-green-500" />,
  COMPLETED_WITH_ERRORS: <AlertTriangle className="h-4 w-4 text-yellow-500" />,
  FAILED: <XCircle className="h-4 w-4 text-red-500" />,
  FAILED_WITH_TERMINAL_ERROR: <XCircle className="h-4 w-4 text-red-500" />,
  CANCELLED: <Square className="h-4 w-4 text-gray-500" />,
  TIMED_OUT: <Clock className="h-4 w-4 text-orange-500" />,
  SCHEDULED: <Clock className="h-4 w-4 text-purple-500" />,
  SKIPPED: <ChevronRight className="h-4 w-4 text-gray-500" />
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

export function ExecutionTimeline({ execution }: ExecutionTimelineProps) {
  const formatTime = (timestamp: number) => {
    return new Date(timestamp).toLocaleTimeString()
  }

  const formatDuration = (executionTime?: number) => {
    if (!executionTime) return 'N/A'
    return `${(executionTime / 1000).toFixed(2)}s`
  }

  const getTaskProgress = (task: TaskExecution) => {
    if (task.status === 'COMPLETED' || task.status === 'COMPLETED_WITH_ERRORS')
      return 100
    if (
      task.status === 'FAILED' ||
      task.status === 'CANCELLED' ||
      task.status === 'TIMED_OUT'
    )
      return 0
    if (task.status === 'IN_PROGRESS') return 50 // Estimated progress for running tasks
    return 0
  }

  const getOverallProgress = () => {
    const completedTasks = execution.tasks.filter(
      (task) =>
        task.status === 'COMPLETED' || task.status === 'COMPLETED_WITH_ERRORS'
    ).length
    return execution.tasks.length > 0
      ? (completedTasks / execution.tasks.length) * 100
      : 0
  }

  const sortedTasks = [...execution.tasks].sort((a, b) => a.seq - b.seq)

  return (
    <div className="space-y-6">
      {/* Workflow Progress Overview */}
      <Card>
        <CardContent className="p-6">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold">Execution Progress</h3>
              <Badge variant="outline">
                {execution.tasks.filter((t) => t.status === 'COMPLETED').length}{' '}
                of {execution.tasks.length} completed
              </Badge>
            </div>

            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Overall Progress</span>
                <span>{getOverallProgress().toFixed(0)}%</span>
              </div>
              <Progress value={getOverallProgress()} className="h-3" />
            </div>

            <div className="grid grid-cols-3 gap-4 text-center">
              <div>
                <p className="text-2xl font-bold text-green-600">
                  {
                    execution.tasks.filter((t) => t.status === 'COMPLETED')
                      .length
                  }
                </p>
                <p className="text-sm text-muted-foreground">Completed</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-blue-600">
                  {
                    execution.tasks.filter((t) => t.status === 'IN_PROGRESS')
                      .length
                  }
                </p>
                <p className="text-sm text-muted-foreground">Running</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-red-600">
                  {
                    execution.tasks.filter(
                      (t) =>
                        t.status === 'FAILED' ||
                        t.status === 'FAILED_WITH_TERMINAL_ERROR'
                    ).length
                  }
                </p>
                <p className="text-sm text-muted-foreground">Failed</p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Timeline */}
      <div className="space-y-1">
        {/* Start Event */}
        <div className="flex items-center space-x-4 rounded-lg bg-green-50 p-4 dark:bg-green-900/20">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-green-500">
            <Play className="h-4 w-4 text-white" />
          </div>
          <div className="flex-1">
            <h4 className="font-medium">Workflow Started</h4>
            <p className="text-sm text-muted-foreground">
              {formatTime(execution.startTime)} • By {execution.createdBy}
            </p>
          </div>
          <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
            START
          </Badge>
        </div>

        {/* Task Timeline */}
        {sortedTasks.map((task, index) => {
          const isLastTask = index === sortedTasks.length - 1
          const progress = getTaskProgress(task)

          return (
            <div key={task.taskId} className="relative">
              {/* Connection Line */}
              {!isLastTask && (
                <div className="absolute left-4 top-16 z-0 h-8 w-px bg-border"></div>
              )}

              <div className="relative z-10 flex items-start space-x-4 rounded-lg p-4 hover:bg-muted/50">
                <div className="flex h-8 w-8 items-center justify-center rounded-full border-2 border-border bg-background">
                  {taskStatusIcons[task.status]}
                </div>

                <div className="flex-1 space-y-2">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-medium">{task.referenceTaskName}</h4>
                      <p className="text-sm text-muted-foreground">
                        {task.taskType} • Seq: {task.seq}
                        {task.retryCount > 0 && ` • Retry: ${task.retryCount}`}
                      </p>
                    </div>
                    <div className="flex items-center space-x-2">
                      <Badge
                        className={taskStatusColors[task.status]}
                        variant="secondary"
                      >
                        {task.status}
                      </Badge>
                      <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
                        <Eye className="h-3 w-3" />
                      </Button>
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
                    <div>
                      <p className="text-muted-foreground">Started</p>
                      <p className="font-medium">
                        {formatTime(task.startTime)}
                      </p>
                    </div>
                    {task.endTime && (
                      <div>
                        <p className="text-muted-foreground">Completed</p>
                        <p className="font-medium">
                          {formatTime(task.endTime)}
                        </p>
                      </div>
                    )}
                    <div>
                      <p className="text-muted-foreground">Duration</p>
                      <p className="font-medium">
                        {formatDuration(task.executionTime)}
                      </p>
                    </div>
                    {task.workerId && (
                      <div>
                        <p className="text-muted-foreground">Worker</p>
                        <p className="text-xs font-medium">
                          {task.workerId.slice(-8)}
                        </p>
                      </div>
                    )}
                  </div>

                  {/* Task Progress Bar for Running Tasks */}
                  {task.status === 'IN_PROGRESS' && (
                    <div className="space-y-1">
                      <div className="flex justify-between text-xs">
                        <span>Progress</span>
                        <span>{progress}%</span>
                      </div>
                      <Progress value={progress} className="h-1" />
                    </div>
                  )}

                  {/* Error Information */}
                  {task.reasonForIncompletion && (
                    <div className="rounded border border-red-200 bg-red-50 p-3">
                      <div className="flex items-start space-x-2">
                        <AlertTriangle className="mt-0.5 h-4 w-4 text-red-500" />
                        <div>
                          <p className="text-sm font-medium text-red-800">
                            Task Failed
                          </p>
                          <p className="text-sm text-red-700">
                            {task.reasonForIncompletion}
                          </p>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Task Output Preview */}
                  {task.outputData &&
                    Object.keys(task.outputData).length > 0 && (
                      <details className="text-sm">
                        <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                          View Output Data
                        </summary>
                        <pre className="mt-2 max-h-32 overflow-auto rounded bg-muted p-2 text-xs">
                          {JSON.stringify(task.outputData, null, 2)}
                        </pre>
                      </details>
                    )}
                </div>
              </div>
            </div>
          )
        })}

        {/* End Event */}
        {execution.status !== 'RUNNING' && (
          <div
            className={`flex items-center space-x-4 rounded-lg p-4 ${
              execution.status === 'COMPLETED'
                ? 'bg-green-50 dark:bg-green-900/20'
                : 'bg-red-50 dark:bg-red-900/20'
            }`}
          >
            <div
              className={`flex h-8 w-8 items-center justify-center rounded-full ${
                execution.status === 'COMPLETED' ? 'bg-green-500' : 'bg-red-500'
              }`}
            >
              {execution.status === 'COMPLETED' ? (
                <CheckCircle className="h-4 w-4 text-white" />
              ) : (
                <XCircle className="h-4 w-4 text-white" />
              )}
            </div>
            <div className="flex-1">
              <h4 className="font-medium">
                Workflow{' '}
                {execution.status === 'COMPLETED' ? 'Completed' : 'Failed'}
              </h4>
              <p className="text-sm text-muted-foreground">
                {execution.endTime && formatTime(execution.endTime)}
                {execution.totalExecutionTime &&
                  ` • Total duration: ${formatDuration(execution.totalExecutionTime)}`}
              </p>
              {execution.reasonForIncompletion && (
                <p className="mt-1 text-sm text-red-600">
                  {execution.reasonForIncompletion}
                </p>
              )}
            </div>
            <Badge
              className={
                execution.status === 'COMPLETED'
                  ? 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200'
                  : 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
              }
            >
              {execution.status}
            </Badge>
          </div>
        )}
      </div>
    </div>
  )
}
