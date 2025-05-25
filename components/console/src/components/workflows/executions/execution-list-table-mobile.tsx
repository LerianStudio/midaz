'use client'

import React, { memo } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import {
  Activity,
  CheckCircle,
  XCircle,
  Clock,
  Play,
  MoreVertical,
  AlertTriangle
} from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { useRouter } from 'next/navigation'

interface ExecutionListMobileCardProps {
  execution: WorkflowExecution
  onAction?: (action: string, execution: WorkflowExecution) => void
}

const statusConfig = {
  RUNNING: {
    color: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
    icon: <Activity className="h-3 w-3 animate-pulse" />
  },
  COMPLETED: {
    color: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    icon: <CheckCircle className="h-3 w-3" />
  },
  FAILED: {
    color: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
    icon: <XCircle className="h-3 w-3" />
  },
  TIMED_OUT: {
    color:
      'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
    icon: <Clock className="h-3 w-3" />
  },
  TERMINATED: {
    color: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
    icon: <XCircle className="h-3 w-3" />
  },
  PAUSED: {
    color:
      'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
    icon: <AlertTriangle className="h-3 w-3" />
  }
}

export const ExecutionListMobileCard = memo<ExecutionListMobileCardProps>(
  ({ execution, onAction }) => {
    const router = useRouter()
    const status = statusConfig[execution.status] || statusConfig.TERMINATED

    const completedTasks = execution.tasks.filter(
      (t) => t.status === 'COMPLETED' || t.status === 'COMPLETED_WITH_ERRORS'
    ).length
    const progress =
      execution.tasks.length > 0
        ? (completedTasks / execution.tasks.length) * 100
        : 0

    const formatTime = (timestamp: number) => {
      const date = new Date(timestamp)
      const today = new Date()
      const isToday = date.toDateString() === today.toDateString()

      if (isToday) {
        return date.toLocaleTimeString([], {
          hour: '2-digit',
          minute: '2-digit'
        })
      }
      return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
    }

    const handleClick = () => {
      router.push(`/plugins/workflows/executions/${execution.executionId}`)
    }

    return (
      <Card
        className="cursor-pointer transition-shadow hover:shadow-md"
        onClick={handleClick}
      >
        <CardContent className="p-4">
          <div className="space-y-3">
            {/* Header */}
            <div className="flex items-start justify-between">
              <div className="min-w-0 flex-1">
                <h3 className="truncate text-sm font-medium">
                  {execution.workflowName}
                </h3>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  #{execution.executionId.slice(-8)}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Badge
                  className={`${status.color} text-xs`}
                  variant="secondary"
                >
                  <span className="flex items-center gap-1">
                    {status.icon}
                    {execution.status}
                  </span>
                </Badge>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    asChild
                    onClick={(e) => e.stopPropagation()}
                  >
                    <Button variant="ghost" size="sm" className="h-7 w-7 p-0">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onClick={(e) => {
                        e.stopPropagation()
                        handleClick()
                      }}
                    >
                      View Details
                    </DropdownMenuItem>
                    {execution.status === 'RUNNING' && (
                      <>
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation()
                            onAction?.('pause', execution)
                          }}
                        >
                          Pause Execution
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation()
                            onAction?.('terminate', execution)
                          }}
                          className="text-red-600"
                        >
                          Terminate
                        </DropdownMenuItem>
                      </>
                    )}
                    {(execution.status === 'FAILED' ||
                      execution.status === 'TIMED_OUT') && (
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation()
                          onAction?.('retry', execution)
                        }}
                      >
                        Retry Execution
                      </DropdownMenuItem>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>

            {/* Progress */}
            {execution.status === 'RUNNING' && (
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">
                    Progress
                  </span>
                  <span className="text-xs font-medium">
                    {progress.toFixed(0)}%
                  </span>
                </div>
                <Progress value={progress} className="h-1.5" />
              </div>
            )}

            {/* Stats */}
            <div className="grid grid-cols-3 gap-2 border-t pt-2">
              <div className="text-center">
                <div className="text-xs font-medium">
                  {completedTasks}/{execution.tasks.length}
                </div>
                <p className="text-[10px] text-muted-foreground">Tasks</p>
              </div>
              <div className="text-center">
                <div className="text-xs font-medium">
                  {formatTime(execution.startTime)}
                </div>
                <p className="text-[10px] text-muted-foreground">Started</p>
              </div>
              <div className="text-center">
                <div className="text-xs font-medium">
                  {execution.totalExecutionTime
                    ? `${(execution.totalExecutionTime / 1000).toFixed(0)}s`
                    : 'Running'}
                </div>
                <p className="text-[10px] text-muted-foreground">Duration</p>
              </div>
            </div>

            {/* Error info */}
            {execution.reasonForIncompletion && (
              <div className="border-t pt-2">
                <p className="line-clamp-2 text-xs text-red-600">
                  {execution.reasonForIncompletion}
                </p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    )
  }
)

ExecutionListMobileCard.displayName = 'ExecutionListMobileCard'
