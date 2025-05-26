'use server'

import { revalidatePath } from 'next/cache'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'

interface ActionResult<T> {
  success: boolean
  data?: T
  error?: string
}

// Mock data store (in a real app, this would be a database)
let executionsStore = [...mockWorkflowExecutions]

// Workflow Execution Actions
export async function getWorkflowExecutionById(
  id: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const execution = executionsStore.find((e) => e.executionId === id)

    if (!execution) {
      return { success: false, error: 'Execution not found' }
    }

    // Simulate real-time updates for running executions
    if (execution.status === 'RUNNING') {
      // Update task progress randomly
      execution.tasks = execution.tasks.map((task) => {
        if (task.status === 'IN_PROGRESS' && Math.random() > 0.7) {
          return {
            ...task,
            status: 'COMPLETED' as const,
            endTime: Date.now(),
            executionTime: Date.now() - task.startTime
          }
        }
        if (task.status === 'SCHEDULED' && Math.random() > 0.8) {
          return {
            ...task,
            status: 'IN_PROGRESS' as const,
            startTime: Date.now()
          }
        }
        return task
      })

      // Check if all tasks are completed
      const allTasksCompleted = execution.tasks.every((task) =>
        ['COMPLETED', 'FAILED', 'SKIPPED'].includes(task.status)
      )

      if (allTasksCompleted) {
        execution.status = 'COMPLETED'
        execution.endTime = Date.now()
      }
    }

    return { success: true, data: execution }
  } catch (error) {
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to fetch execution'
    }
  }
}

export async function pauseWorkflowExecution(
  id: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const execution = executionsStore.find((e) => e.executionId === id)

    if (!execution) {
      return { success: false, error: 'Execution not found' }
    }

    if (execution.status !== 'RUNNING') {
      return { success: false, error: 'Can only pause running executions' }
    }

    execution.status = 'PAUSED'

    revalidatePath(`/plugins/workflows/executions/${id}`)

    return { success: true, data: execution }
  } catch (error) {
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to pause execution'
    }
  }
}

export async function resumeWorkflowExecution(
  id: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const execution = executionsStore.find((e) => e.executionId === id)

    if (!execution) {
      return { success: false, error: 'Execution not found' }
    }

    if (execution.status !== 'PAUSED') {
      return { success: false, error: 'Can only resume paused executions' }
    }

    execution.status = 'RUNNING'

    revalidatePath(`/plugins/workflows/executions/${id}`)

    return { success: true, data: execution }
  } catch (error) {
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to resume execution'
    }
  }
}

export async function terminateWorkflowExecution(
  id: string,
  reason: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const execution = executionsStore.find((e) => e.executionId === id)

    if (!execution) {
      return { success: false, error: 'Execution not found' }
    }

    if (!['RUNNING', 'PAUSED'].includes(execution.status)) {
      return {
        success: false,
        error: 'Can only terminate running or paused executions'
      }
    }

    execution.status = 'TERMINATED'
    execution.endTime = Date.now()
    execution.reasonForIncompletion = reason

    revalidatePath(`/plugins/workflows/executions/${id}`)

    return { success: true, data: execution }
  } catch (error) {
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to terminate execution'
    }
  }
}

export async function retryWorkflowExecution(
  id: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const execution = executionsStore.find((e) => e.executionId === id)

    if (!execution) {
      return { success: false, error: 'Execution not found' }
    }

    if (!['FAILED', 'TIMED_OUT'].includes(execution.status)) {
      return {
        success: false,
        error: 'Can only retry failed or timed out executions'
      }
    }

    // Create a new execution based on the failed one
    const newExecution: WorkflowExecution = {
      ...execution,
      executionId: `exec_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      status: 'RUNNING',
      startTime: Date.now(),
      endTime: undefined,
      reasonForIncompletion: undefined,
      failedReferenceTaskNames: [],
      tasks: execution.tasks.map((task) => ({
        ...task,
        status: 'SCHEDULED' as const,
        startTime: 0,
        endTime: undefined,
        executionTime: undefined,
        reasonForIncompletion: undefined,
        outputData: undefined
      })),
      output: undefined
    }

    executionsStore.push(newExecution)

    revalidatePath('/plugins/workflows/executions')

    return { success: true, data: newExecution }
  } catch (error) {
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to retry execution'
    }
  }
}
