'use client'

import React, { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Play,
  Pause,
  Square,
  RotateCcw,
  Loader2,
  AlertCircle,
  CheckCircle,
  XCircle,
  Clock,
  Activity,
  ChevronRight
} from 'lucide-react'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { useToast } from '@/hooks/use-toast'

interface ExecutionControlPanelProps {
  execution: WorkflowExecution
  onPause?: () => Promise<void>
  onResume?: () => Promise<void>
  onTerminate?: (reason?: string) => Promise<void>
  onRetry?: (options?: RetryOptions) => Promise<void>
  onRerun?: () => Promise<void>
}

interface RetryOptions {
  fromTaskRef?: string
  retryCount?: number
  skipCompleted?: boolean
}

type ControlAction = 'pause' | 'resume' | 'terminate' | 'retry' | 'rerun'

const actionIcons = {
  pause: Pause,
  resume: Play,
  terminate: Square,
  retry: RotateCcw,
  rerun: Play
}

const actionLabels = {
  pause: 'Pause Execution',
  resume: 'Resume Execution',
  terminate: 'Terminate Execution',
  retry: 'Retry Failed Tasks',
  rerun: 'Rerun Workflow'
}

const actionDescriptions = {
  pause: 'Temporarily pause the workflow execution. You can resume it later.',
  resume: 'Resume the paused workflow execution from where it left off.',
  terminate:
    'Stop the workflow execution immediately. This action cannot be undone.',
  retry:
    'Retry failed tasks in the workflow. You can choose specific tasks to retry.',
  rerun: 'Start a new execution of the workflow with the same input parameters.'
}

export function ExecutionControlPanel({
  execution,
  onPause,
  onResume,
  onTerminate,
  onRetry,
  onRerun
}: ExecutionControlPanelProps) {
  const { toast } = useToast()
  const [isProcessing, setIsProcessing] = useState(false)
  const [activeAction, setActiveAction] = useState<ControlAction | null>(null)
  const [showConfirmDialog, setShowConfirmDialog] = useState(false)
  const [showRetryDialog, setShowRetryDialog] = useState(false)
  const [terminateReason, setTerminateReason] = useState('')
  const [retryOptions, setRetryOptions] = useState<RetryOptions>({
    skipCompleted: true
  })

  // Determine available actions based on execution status
  const getAvailableActions = (): ControlAction[] => {
    switch (execution.status) {
      case 'RUNNING':
        return ['pause', 'terminate']
      case 'PAUSED':
        return ['resume', 'terminate']
      case 'FAILED':
      case 'TIMED_OUT':
        return ['retry', 'rerun']
      case 'COMPLETED':
      case 'TERMINATED':
        return ['rerun']
      default:
        return []
    }
  }

  const availableActions = getAvailableActions()

  const handleAction = async (action: ControlAction) => {
    setActiveAction(action)

    // Show confirmation for destructive actions
    if (action === 'terminate') {
      setShowConfirmDialog(true)
      return
    }

    // Show retry options dialog
    if (action === 'retry') {
      setShowRetryDialog(true)
      return
    }

    // Execute the action
    await executeAction(action)
  }

  const executeAction = async (action: ControlAction, options?: any) => {
    setIsProcessing(true)

    try {
      switch (action) {
        case 'pause':
          if (onPause) {
            await onPause()
            toast({
              title: 'Execution paused',
              description:
                'The workflow execution has been paused successfully.'
            })
          }
          break

        case 'resume':
          if (onResume) {
            await onResume()
            toast({
              title: 'Execution resumed',
              description: 'The workflow execution has been resumed.'
            })
          }
          break

        case 'terminate':
          if (onTerminate) {
            await onTerminate(terminateReason || undefined)
            toast({
              title: 'Execution terminated',
              description: 'The workflow execution has been terminated.',
              variant: 'destructive'
            })
          }
          break

        case 'retry':
          if (onRetry) {
            await onRetry(options || retryOptions)
            toast({
              title: 'Retry initiated',
              description: 'Failed tasks are being retried.'
            })
          }
          break

        case 'rerun':
          if (onRerun) {
            await onRerun()
            toast({
              title: 'Workflow rerun started',
              description:
                'A new execution has been started with the same parameters.'
            })
          }
          break
      }
    } catch (error) {
      toast({
        title: 'Action failed',
        description:
          error instanceof Error ? error.message : 'An error occurred',
        variant: 'destructive'
      })
    } finally {
      setIsProcessing(false)
      setActiveAction(null)
      setShowConfirmDialog(false)
      setShowRetryDialog(false)
    }
  }

  // Get failed tasks for retry options
  const failedTasks = execution.tasks.filter(
    (task) =>
      task.status === 'FAILED' ||
      task.status === 'FAILED_WITH_TERMINAL_ERROR' ||
      task.status === 'TIMED_OUT'
  )

  return (
    <div className="space-y-4">
      {/* Control Actions */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {availableActions.map((action) => {
          const Icon = actionIcons[action]
          const isActive = activeAction === action && isProcessing

          return (
            <Card
              key={action}
              className={`cursor-pointer transition-all hover:shadow-md ${
                isActive ? 'ring-2 ring-primary' : ''
              }`}
              onClick={() => !isProcessing && handleAction(action)}
            >
              <CardContent className="flex items-center justify-between p-4">
                <div className="flex items-center space-x-3">
                  <div
                    className={`rounded-lg p-2 ${
                      action === 'terminate'
                        ? 'bg-destructive/10 text-destructive'
                        : action === 'pause'
                          ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/20'
                          : 'bg-primary/10 text-primary'
                    }`}
                  >
                    {isActive ? (
                      <Loader2 className="h-5 w-5 animate-spin" />
                    ) : (
                      <Icon className="h-5 w-5" />
                    )}
                  </div>
                  <div>
                    <p className="font-medium">{actionLabels[action]}</p>
                    <p className="text-xs text-muted-foreground">
                      {actionDescriptions[action]}
                    </p>
                  </div>
                </div>
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              </CardContent>
            </Card>
          )
        })}
      </div>

      {/* Execution Status Summary */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Execution Status</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Status Badge */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">
              Current Status
            </span>
            <Badge
              variant={
                execution.status === 'COMPLETED'
                  ? 'default'
                  : execution.status === 'FAILED'
                    ? 'destructive'
                    : 'secondary'
              }
            >
              {execution.status}
            </Badge>
          </div>

          {/* Task Summary */}
          <div className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Total Tasks</span>
              <span className="font-medium">{execution.tasks.length}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="flex items-center gap-2 text-muted-foreground">
                <CheckCircle className="h-3 w-3 text-green-500" />
                Completed
              </span>
              <span className="font-medium">
                {execution.tasks.filter((t) => t.status === 'COMPLETED').length}
              </span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="flex items-center gap-2 text-muted-foreground">
                <XCircle className="h-3 w-3 text-red-500" />
                Failed
              </span>
              <span className="font-medium">{failedTasks.length}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="flex items-center gap-2 text-muted-foreground">
                <Activity className="h-3 w-3 text-blue-500" />
                Running
              </span>
              <span className="font-medium">
                {
                  execution.tasks.filter((t) => t.status === 'IN_PROGRESS')
                    .length
                }
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Terminate Confirmation Dialog */}
      <AlertDialog open={showConfirmDialog} onOpenChange={setShowConfirmDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Terminate Execution?</AlertDialogTitle>
            <AlertDialogDescription>
              This will immediately stop the workflow execution. This action
              cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="my-4 space-y-2">
            <Label>Reason for termination (optional)</Label>
            <Textarea
              placeholder="Enter a reason for terminating this execution..."
              value={terminateReason}
              onChange={(e) => setTerminateReason(e.target.value)}
              rows={3}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => executeAction('terminate')}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Terminate Execution
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Retry Options Dialog */}
      <Dialog open={showRetryDialog} onOpenChange={setShowRetryDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Retry Failed Tasks</DialogTitle>
            <DialogDescription>
              Configure how you want to retry the failed tasks in this workflow.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {/* Failed Tasks Summary */}
            {failedTasks.length > 0 && (
              <Alert>
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  {failedTasks.length} task{failedTasks.length > 1 ? 's' : ''}{' '}
                  failed in this execution
                </AlertDescription>
              </Alert>
            )}

            {/* Retry From Task */}
            <div className="space-y-2">
              <Label>Retry from task (optional)</Label>
              <Select
                value={retryOptions.fromTaskRef || ''}
                onValueChange={(value) =>
                  setRetryOptions({
                    ...retryOptions,
                    fromTaskRef: value || undefined
                  })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Retry all failed tasks" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All failed tasks</SelectItem>
                  {failedTasks.map((task) => (
                    <SelectItem
                      key={task.taskId}
                      value={task.referenceTaskName}
                    >
                      {task.referenceTaskName}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Skip Completed Tasks */}
            <div className="flex items-center space-x-2">
              <input
                type="checkbox"
                id="skip-completed"
                checked={retryOptions.skipCompleted}
                onChange={(e) =>
                  setRetryOptions({
                    ...retryOptions,
                    skipCompleted: e.target.checked
                  })
                }
                className="rounded border-gray-300"
              />
              <Label htmlFor="skip-completed" className="text-sm font-normal">
                Skip already completed tasks
              </Label>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRetryDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => executeAction('retry', retryOptions)}
              disabled={isProcessing}
            >
              {isProcessing ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Retrying...
                </>
              ) : (
                <>
                  <RotateCcw className="mr-2 h-4 w-4" />
                  Retry Tasks
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
