'use server'

import { revalidatePath } from 'next/cache'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { WorkflowRepository } from '@/core/domain/repositories/workflow-repository'
import { WorkflowExecutionRepository } from '@/core/application/repositories/workflow-execution-repository'
import { WorkflowUseCase } from '@/core/application/use-cases/workflows/workflow-use-case'
import { Workflow } from '@/core/domain/entities/workflow'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { MIDAZ_SYMBOLS } from '@/core/infrastructure/container-registry/midaz/midaz-module'
import { WORKFLOW_SYMBOLS } from '@/core/infrastructure/container-registry/use-cases/workflow-module'

interface ActionResult<T> {
  success: boolean
  data?: T
  error?: string
}

const workflowRepository = container.get<WorkflowRepository>(
  MIDAZ_SYMBOLS.WorkflowRepository
)
const workflowUseCase = container.get<WorkflowUseCase>(
  WORKFLOW_SYMBOLS.WorkflowUseCase
)

// Workflow Management Actions
export async function getWorkflows(params?: {
  organizationId: string
  status?: string
  limit?: number
  page?: number
}): Promise<ActionResult<{ workflows: Workflow[]; total: number }>> {
  try {
    const {
      organizationId = 'default',
      limit = 10,
      page = 1,
      status
    } = params || {}
    const result = await workflowRepository.fetchAll(
      organizationId,
      limit,
      page,
      { status }
    )
    return {
      success: true,
      data: {
        workflows: result.items,
        total: result.total
      }
    }
  } catch (error) {
    console.error('Error fetching workflows:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to fetch workflows'
    }
  }
}

export async function getWorkflowById(
  id: string
): Promise<ActionResult<Workflow>> {
  try {
    const workflow = await workflowRepository.findById(id)
    return {
      success: true,
      data: workflow
    }
  } catch (error) {
    console.error('Error fetching workflow:', error)
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Failed to fetch workflow'
    }
  }
}

export async function createWorkflow(
  workflow: Omit<Workflow, 'id' | 'createdAt' | 'updatedAt'>
): Promise<ActionResult<Workflow>> {
  try {
    const created = await workflowRepository.create(workflow)
    revalidatePath('/plugins/workflows')
    return {
      success: true,
      data: created
    }
  } catch (error) {
    console.error('Error creating workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to create workflow'
    }
  }
}

export async function updateWorkflow(
  id: string,
  updates: Partial<Workflow>
): Promise<ActionResult<Workflow>> {
  try {
    const updated = await workflowRepository.update(id, updates)
    revalidatePath('/plugins/workflows')
    revalidatePath(`/plugins/workflows/${id}`)
    return {
      success: true,
      data: updated
    }
  } catch (error) {
    console.error('Error updating workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to update workflow'
    }
  }
}

export async function deleteWorkflow(id: string): Promise<ActionResult<void>> {
  try {
    await workflowRepository.delete(id)
    revalidatePath('/plugins/workflows')
    return {
      success: true
    }
  } catch (error) {
    console.error('Error deleting workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to delete workflow'
    }
  }
}

export async function updateWorkflowStatus(
  id: string,
  status: 'active' | 'inactive'
): Promise<ActionResult<Workflow>> {
  try {
    const updated = await workflowRepository.update(id, { status })
    revalidatePath('/plugins/workflows')
    revalidatePath(`/plugins/workflows/${id}`)
    return {
      success: true,
      data: updated
    }
  } catch (error) {
    console.error('Error updating workflow status:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to update workflow status'
    }
  }
}

// Workflow Execution Actions
export async function getWorkflowExecutions(params?: {
  workflowId?: string
  status?: string
  limit?: number
  page?: number
}): Promise<ActionResult<{ executions: WorkflowExecution[]; total: number }>> {
  try {
    const { limit = 10, page = 1, status, workflowId } = params || {}
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const result = await executionRepository.findAll({
      limit,
      offset: (page - 1) * limit,
      status,
      workflowId
    })
    return {
      success: true,
      data: {
        executions: result.executions,
        total: result.total
      }
    }
  } catch (error) {
    console.error('Error fetching workflow executions:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to fetch workflow executions'
    }
  }
}

export async function getWorkflowExecutionById(
  id: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const execution = await executionRepository.findById(id)
    return {
      success: true,
      data: execution
    }
  } catch (error) {
    console.error('Error fetching workflow execution:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to fetch workflow execution'
    }
  }
}

export async function startWorkflowExecution(
  workflowId: string,
  input?: Record<string, any>
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const execution = await workflowUseCase.startExecution(workflowId, input)
    revalidatePath('/plugins/workflows/executions')
    return {
      success: true,
      data: execution
    }
  } catch (error) {
    console.error('Error starting workflow execution:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to start workflow execution'
    }
  }
}

export async function pauseWorkflowExecution(
  executionId: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const execution = await executionRepository.update(executionId, {
      status: 'paused'
    })
    revalidatePath('/plugins/workflows/executions')
    revalidatePath(`/plugins/workflows/executions/${executionId}`)
    return {
      success: true,
      data: execution
    }
  } catch (error) {
    console.error('Error pausing workflow execution:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to pause workflow execution'
    }
  }
}

export async function resumeWorkflowExecution(
  executionId: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const execution = await executionRepository.update(executionId, {
      status: 'running'
    })
    revalidatePath('/plugins/workflows/executions')
    revalidatePath(`/plugins/workflows/executions/${executionId}`)
    return {
      success: true,
      data: execution
    }
  } catch (error) {
    console.error('Error resuming workflow execution:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to resume workflow execution'
    }
  }
}

export async function terminateWorkflowExecution(
  executionId: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const execution = await executionRepository.update(executionId, {
      status: 'terminated',
      endTime: new Date()
    })
    revalidatePath('/plugins/workflows/executions')
    revalidatePath(`/plugins/workflows/executions/${executionId}`)
    return {
      success: true,
      data: execution
    }
  } catch (error) {
    console.error('Error terminating workflow execution:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to terminate workflow execution'
    }
  }
}

export async function retryWorkflowExecution(
  executionId: string
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const execution = await executionRepository.findById(executionId)
    const newExecution = await workflowUseCase.startExecution(
      execution.workflowId,
      execution.input
    )
    revalidatePath('/plugins/workflows/executions')
    return {
      success: true,
      data: newExecution
    }
  } catch (error) {
    console.error('Error retrying workflow execution:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to retry workflow execution'
    }
  }
}

export async function updateWorkflowExecutionStatus(
  executionId: string,
  status: WorkflowExecution['status']
): Promise<ActionResult<WorkflowExecution>> {
  try {
    const executionRepository = container.get<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    const updates: Partial<WorkflowExecution> = { status }

    if (
      status === 'completed' ||
      status === 'failed' ||
      status === 'terminated'
    ) {
      updates.endTime = new Date()
    }

    const execution = await executionRepository.update(executionId, updates)
    revalidatePath('/plugins/workflows/executions')
    revalidatePath(`/plugins/workflows/executions/${executionId}`)
    return {
      success: true,
      data: execution
    }
  } catch (error) {
    console.error('Error updating workflow execution status:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to update workflow execution status'
    }
  }
}
