'use client'

import { useState, useMemo } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import {
  Activity,
  CheckCircle,
  XCircle,
  Clock,
  Square,
  Pause,
  CircleDot,
  ChevronRight,
  Timer,
  AlertTriangle,
  Loader2,
  Play,
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
  COMPLETED_WITH_ERRORS: <CheckCircle className="h-4 w-4 text-yellow-500" />,
  FAILED: <XCircle className="h-4 w-4 text-red-500" />,
  FAILED_WITH_TERMINAL_ERROR: <XCircle className="h-4 w-4 text-red-700" />,
  CANCELLED: <Square className="h-4 w-4 text-gray-500" />,
  TIMED_OUT: <Clock className="h-4 w-4 text-orange-500" />,
  SCHEDULED: <CircleDot className="h-4 w-4 text-purple-500" />,
  SKIPPED: <ChevronRight className="h-4 w-4 text-gray-400" />
}

const taskStatusColors = {
  IN_PROGRESS: 'bg-blue-500',
  COMPLETED: 'bg-green-500',
  COMPLETED_WITH_ERRORS: 'bg-yellow-500',
  FAILED: 'bg-red-500',
  FAILED_WITH_TERMINAL_ERROR: 'bg-red-700',
  CANCELLED: 'bg-gray-500',
  TIMED_OUT: 'bg-orange-500',
  SCHEDULED: 'bg-purple-500',
  SKIPPED: 'bg-gray-400'
}

export function ExecutionTimeline({ execution }: ExecutionTimelineProps) {
  const [selectedTask, setSelectedTask] = useState<TaskExecution | null>(null)
  const [hoveredTask, setHoveredTask] = useState<string | null>(null)
  const [viewMode, setViewMode] = useState<'gantt' | 'list'>('gantt')

  // Calculate timeline bounds
  const { timelineStart, timelineEnd, totalDuration } = useMemo(() => {
    if (!execution.tasks || execution.tasks.length === 0) {
      const end = execution.endTime || Date.now()
      return {
        timelineStart: execution.startTime,
        timelineEnd: end,
        totalDuration: end - execution.startTime
      }
    }

    const start = Math.min(
      execution.startTime,
      ...execution.tasks.map((t) => t.startTime)
    )
    const end = Math.max(
      execution.endTime || Date.now(),
      ...execution.tasks.map((t) => t.endTime || Date.now())
    )

    return {
      timelineStart: start,
      timelineEnd: end,
      totalDuration: end - start
    }
  }, [execution])

  const getTaskPosition = (task: TaskExecution) => {
    const taskStart = task.startTime - timelineStart
    const taskDuration = (task.endTime || Date.now()) - task.startTime
    const left = (taskStart / totalDuration) * 100
    const width = Math.max((taskDuration / totalDuration) * 100, 1) // Minimum 1% width

    return { left: `${left}%`, width: `${width}%` }
  }

  const formatDate = (timestamp: number) => {
    return new Date(timestamp).toLocaleString()
  }

  const formatShortTime = (timestamp: number) => {
    return new Date(timestamp).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    })
  }

  const formatDuration = (ms: number) => {
    const seconds = Math.floor(ms / 1000)
    const minutes = Math.floor(seconds / 60)
    const hours = Math.floor(minutes / 60)

    if (hours > 0) {
      return `${hours}h ${minutes % 60}m ${seconds % 60}s`
    } else if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`
    } else {
      return `${seconds}s`
    }
  }

  const getTimeMarkers = () => {
    const markers = []
    const markerCount = 5
    const interval = totalDuration / (markerCount - 1)

    for (let i = 0; i < markerCount; i++) {
      const time = timelineStart + interval * i
      markers.push({
        position: (i / (markerCount - 1)) * 100,
        time,
        label: formatShortTime(time)
      })
    }

    return markers
  }

  const sortedTasks = [...execution.tasks].sort((a, b) => a.seq - b.seq)

  const renderGanttView = () => (
    <div className="relative">
      {/* Time markers */}
      <div className="mb-2 flex justify-between text-xs text-muted-foreground">
        {getTimeMarkers().map((marker, index) => (
          <span key={index}>{marker.label}</span>
        ))}
      </div>

      {/* Timeline container */}
      <div
        className="relative rounded-lg border bg-muted/10 p-4"
        style={{ minHeight: '200px' }}
      >
        {/* Background grid */}
        <div className="absolute inset-0">
          {getTimeMarkers().map((marker, index) => (
            <div
              key={index}
              className="absolute top-0 h-full w-px bg-muted/30"
              style={{ left: `${marker.position}%` }}
            />
          ))}
        </div>

        {/* Task bars */}
        <div
          className="relative"
          style={{ height: `${Math.max(execution.tasks.length * 40, 100)}px` }}
        >
          {execution.tasks.map((task, index) => {
            const position = getTaskPosition(task)
            const isHovered = hoveredTask === task.taskId
            const isSelected = selectedTask?.taskId === task.taskId

            return (
              <Tooltip key={task.taskId}>
                <TooltipTrigger asChild>
                  <div
                    className={`absolute flex cursor-pointer items-center rounded transition-all ${
                      isSelected ? 'z-10 ring-2 ring-primary' : ''
                    } ${isHovered ? 'z-20 shadow-lg' : 'shadow-sm'}`}
                    style={{
                      left: position.left,
                      width: position.width,
                      top: `${index * 40}px`,
                      height: '32px'
                    }}
                    onClick={() => setSelectedTask(task)}
                    onMouseEnter={() => setHoveredTask(task.taskId)}
                    onMouseLeave={() => setHoveredTask(null)}
                  >
                    <div
                      className={`h-full w-full rounded ${taskStatusColors[task.status]} flex items-center bg-opacity-80 px-2`}
                    >
                      <div className="flex items-center space-x-2 text-white">
                        {taskStatusIcons[task.status]}
                        <span className="truncate text-xs font-medium">
                          {task.referenceTaskName}
                        </span>
                      </div>
                    </div>
                  </div>
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs">
                  <div className="space-y-1">
                    <p className="font-semibold">{task.referenceTaskName}</p>
                    <p className="text-sm">Type: {task.taskType}</p>
                    <p className="text-sm">Status: {task.status}</p>
                    <p className="text-sm">
                      Started: {formatShortTime(task.startTime)}
                    </p>
                    {task.endTime && (
                      <p className="text-sm">
                        Ended: {formatShortTime(task.endTime)}
                      </p>
                    )}
                    {task.executionTime && (
                      <p className="text-sm">
                        Duration: {formatDuration(task.executionTime)}
                      </p>
                    )}
                    {task.retryCount > 0 && (
                      <p className="text-sm">Retries: {task.retryCount}</p>
                    )}
                  </div>
                </TooltipContent>
              </Tooltip>
            )
          })}
        </div>

        {/* Current time indicator for running executions */}
        {execution.status === 'RUNNING' && (
          <div
            className="absolute top-0 h-full w-0.5 animate-pulse bg-primary"
            style={{
              left: `${((Date.now() - timelineStart) / totalDuration) * 100}%`
            }}
          >
            <div className="absolute -top-1 left-1/2 -translate-x-1/2">
              <div className="h-2 w-2 animate-ping rounded-full bg-primary" />
            </div>
          </div>
        )}
      </div>

      {/* Legend */}
      <div className="mt-4 flex flex-wrap gap-4 text-xs">
        <div className="flex items-center space-x-1">
          <div className="h-3 w-3 rounded bg-blue-500" />
          <span>In Progress</span>
        </div>
        <div className="flex items-center space-x-1">
          <div className="h-3 w-3 rounded bg-green-500" />
          <span>Completed</span>
        </div>
        <div className="flex items-center space-x-1">
          <div className="h-3 w-3 rounded bg-red-500" />
          <span>Failed</span>
        </div>
        <div className="flex items-center space-x-1">
          <div className="h-3 w-3 rounded bg-yellow-500" />
          <span>Warning</span>
        </div>
        <div className="flex items-center space-x-1">
          <div className="h-3 w-3 rounded bg-orange-500" />
          <span>Timed Out</span>
        </div>
      </div>
    </div>
  )

  const renderListView = () => (
    <div className="space-y-1">
      {/* Start Event */}
      <div className="flex items-center space-x-4 rounded-lg bg-green-50 p-4 dark:bg-green-900/20">
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-green-500">
          <Play className="h-4 w-4 text-white" />
        </div>
        <div className="flex-1">
          <h4 className="font-medium">Workflow Started</h4>
          <p className="text-sm text-muted-foreground">
            {formatDate(execution.startTime)} • By {execution.createdBy}
          </p>
        </div>
        <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
          START
        </Badge>
      </div>

      {/* Task Timeline */}
      {sortedTasks.map((task, index) => {
        const isLastTask = index === sortedTasks.length - 1

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
                      className={`${taskStatusColors[task.status]} text-white`}
                      variant="secondary"
                    >
                      {task.status}
                    </Badge>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
                  <div>
                    <p className="text-muted-foreground">Started</p>
                    <p className="font-medium">
                      {formatShortTime(task.startTime)}
                    </p>
                  </div>
                  {task.endTime && (
                    <div>
                      <p className="text-muted-foreground">Completed</p>
                      <p className="font-medium">
                        {formatShortTime(task.endTime)}
                      </p>
                    </div>
                  )}
                  <div>
                    <p className="text-muted-foreground">Duration</p>
                    <p className="font-medium">
                      {task.executionTime
                        ? formatDuration(task.executionTime)
                        : 'Running...'}
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

                {/* Error Information */}
                {task.reasonForIncompletion && (
                  <div className="rounded border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-950">
                    <div className="flex items-start space-x-2">
                      <AlertTriangle className="mt-0.5 h-4 w-4 text-red-500" />
                      <div>
                        <p className="text-sm font-medium text-red-800 dark:text-red-200">
                          Task Failed
                        </p>
                        <p className="text-sm text-red-700 dark:text-red-300">
                          {task.reasonForIncompletion}
                        </p>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )
      })}

      {/* End Event */}
      {execution.status !== 'RUNNING' && execution.status !== 'PAUSED' && (
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
              Workflow {execution.status.toLowerCase()}
            </h4>
            <p className="text-sm text-muted-foreground">
              {execution.endTime && formatDate(execution.endTime)}
              {execution.endTime &&
                ` • Total duration: ${formatDuration(execution.endTime - execution.startTime)}`}
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
  )

  return (
    <TooltipProvider>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Execution Timeline</CardTitle>
              <CardDescription>
                Visual timeline showing task execution flow and timing
              </CardDescription>
            </div>
            <div className="flex items-center space-x-2">
              <Button
                variant={viewMode === 'gantt' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setViewMode('gantt')}
              >
                Gantt View
              </Button>
              <Button
                variant={viewMode === 'list' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setViewMode('list')}
              >
                List View
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {viewMode === 'gantt' ? renderGanttView() : renderListView()}

          {/* Selected Task Details */}
          {selectedTask && (
            <Card className="mt-6">
              <CardHeader>
                <CardTitle className="text-base">
                  Task Details: {selectedTask.referenceTaskName}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-4 md:grid-cols-2">
                  <div>
                    <h5 className="mb-2 font-medium">Properties</h5>
                    <dl className="space-y-1 text-sm">
                      <div className="flex justify-between">
                        <dt className="text-muted-foreground">Task ID:</dt>
                        <dd className="font-mono text-xs">
                          {selectedTask.taskId}
                        </dd>
                      </div>
                      <div className="flex justify-between">
                        <dt className="text-muted-foreground">Type:</dt>
                        <dd>{selectedTask.taskType}</dd>
                      </div>
                      <div className="flex justify-between">
                        <dt className="text-muted-foreground">Status:</dt>
                        <dd>
                          <Badge
                            className={`${taskStatusColors[selectedTask.status]} text-white`}
                            variant="secondary"
                          >
                            {selectedTask.status}
                          </Badge>
                        </dd>
                      </div>
                      <div className="flex justify-between">
                        <dt className="text-muted-foreground">Sequence:</dt>
                        <dd>{selectedTask.seq}</dd>
                      </div>
                    </dl>
                  </div>
                  <div>
                    <h5 className="mb-2 font-medium">Timing</h5>
                    <dl className="space-y-1 text-sm">
                      <div className="flex justify-between">
                        <dt className="text-muted-foreground">Started:</dt>
                        <dd>{formatDate(selectedTask.startTime)}</dd>
                      </div>
                      {selectedTask.endTime && (
                        <div className="flex justify-between">
                          <dt className="text-muted-foreground">Ended:</dt>
                          <dd>{formatDate(selectedTask.endTime)}</dd>
                        </div>
                      )}
                      {selectedTask.executionTime && (
                        <div className="flex justify-between">
                          <dt className="text-muted-foreground">Duration:</dt>
                          <dd>{formatDuration(selectedTask.executionTime)}</dd>
                        </div>
                      )}
                      {selectedTask.retryCount > 0 && (
                        <div className="flex justify-between">
                          <dt className="text-muted-foreground">Retries:</dt>
                          <dd>{selectedTask.retryCount}</dd>
                        </div>
                      )}
                    </dl>
                  </div>
                </div>
                {selectedTask.reasonForIncompletion && (
                  <div className="mt-4 rounded border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-950">
                    <div className="flex items-start space-x-2">
                      <AlertTriangle className="mt-0.5 h-4 w-4 text-red-500" />
                      <div>
                        <p className="text-sm font-medium text-red-800 dark:text-red-200">
                          Error Details
                        </p>
                        <p className="text-sm text-red-700 dark:text-red-300">
                          {selectedTask.reasonForIncompletion}
                        </p>
                      </div>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </CardContent>
      </Card>
    </TooltipProvider>
  )
}
