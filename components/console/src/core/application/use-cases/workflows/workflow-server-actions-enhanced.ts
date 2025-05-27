'use server'

import { container } from '@/core/infrastructure/container-registry/container-registry'
import { WorkflowUseCase } from './workflow-use-case'
import {
  Workflow,
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  WorkflowValidationResult,
  WorkflowStatus
} from '@/core/domain/entities/workflow'
import { getOrganizationId } from '@/lib/actions'

// Enhanced error types for better error handling
export type ActionErrorType =
  | 'NETWORK_ERROR'
  | 'VALIDATION_ERROR'
  | 'PERMISSION_ERROR'
  | 'NOT_FOUND'
  | 'SERVER_ERROR'
  | 'UNKNOWN_ERROR'

export interface ActionError {
  type: ActionErrorType
  message: string
  details?: any
}

export interface ActionResult<T> {
  success: boolean
  data?: T
  error?: ActionError
}

// Helper function to classify errors
function classifyError(error: any): ActionError {
  // Network errors
  if (error?.code === 'ECONNREFUSED' || error?.message?.includes('fetch')) {
    return {
      type: 'NETWORK_ERROR',
      message:
        'Unable to connect to the server. Please check your connection and try again.',
      details: error
    }
  }

  // Validation errors
  if (error?.status === 400 || error?.name === 'ValidationError') {
    return {
      type: 'VALIDATION_ERROR',
      message: error?.message || 'The provided data is invalid',
      details: error?.validationErrors || error
    }
  }

  // Permission errors
  if (error?.status === 403 || error?.code === 'FORBIDDEN') {
    return {
      type: 'PERMISSION_ERROR',
      message: 'You do not have permission to perform this action',
      details: error
    }
  }

  // Not found errors
  if (error?.status === 404 || error?.code === 'NOT_FOUND') {
    return {
      type: 'NOT_FOUND',
      message: error?.message || 'The requested resource was not found',
      details: error
    }
  }

  // Server errors
  if (error?.status >= 500) {
    return {
      type: 'SERVER_ERROR',
      message: 'A server error occurred. Please try again later.',
      details: error
    }
  }

  // Default unknown error
  return {
    type: 'UNKNOWN_ERROR',
    message: error?.message || 'An unexpected error occurred',
    details: error
  }
}

// Enhanced action with retry logic
async function executeWithRetry<T>(
  operation: () => Promise<T>,
  maxRetries: number = 3,
  delayMs: number = 1000
): Promise<T> {
  let lastError: any

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await operation()
    } catch (error) {
      lastError = error

      // Don't retry for client errors (4xx)
      if (error && typeof error === 'object' && 'status' in error) {
        const status = (error as any).status
        if (status >= 400 && status < 500) {
          throw error
        }
      }

      // Don't retry on the last attempt
      if (attempt === maxRetries) {
        throw error
      }

      // Log retry attempt
      console.log(`Retry attempt ${attempt}/${maxRetries} after ${delayMs}ms`)

      // Wait before retrying
      await new Promise((resolve) => setTimeout(resolve, delayMs))

      // Increase delay for next attempt (exponential backoff)
      delayMs *= 2
    }
  }

  throw lastError
}

export async function createWorkflowActionEnhanced(params: {
  workflow: CreateWorkflowRequest
}): Promise<ActionResult<Workflow>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    const workflow = await executeWithRetry(
      () => workflowUseCase.createWorkflow(organizationId, params.workflow),
      2, // Only retry once for create operations
      500
    )

    return { success: true, data: workflow }
  } catch (error) {
    console.error('[createWorkflowAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

export async function fetchWorkflowByIdActionEnhanced(params: {
  workflowId: string
}): Promise<ActionResult<Workflow>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    const workflow = await executeWithRetry(() =>
      workflowUseCase.fetchWorkflowById(organizationId, params.workflowId)
    )

    return { success: true, data: workflow }
  } catch (error) {
    console.error('[fetchWorkflowByIdAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

export async function updateWorkflowActionEnhanced(params: {
  workflowId: string
  workflow: UpdateWorkflowRequest
}): Promise<ActionResult<Workflow>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    const workflow = await executeWithRetry(
      () =>
        workflowUseCase.updateWorkflow(
          organizationId,
          params.workflowId,
          params.workflow
        ),
      2,
      500
    )

    return { success: true, data: workflow }
  } catch (error) {
    console.error('[updateWorkflowAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

export async function deleteWorkflowActionEnhanced(params: {
  workflowId: string
}): Promise<ActionResult<void>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    await executeWithRetry(
      () => workflowUseCase.deleteWorkflow(organizationId, params.workflowId),
      2,
      500
    )

    return { success: true }
  } catch (error) {
    console.error('[deleteWorkflowAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

export async function validateWorkflowActionEnhanced(params: {
  workflow: Workflow | CreateWorkflowRequest
}): Promise<ActionResult<WorkflowValidationResult>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    // Validation doesn't need retry as it's a local operation
    const result = await workflowUseCase.validateWorkflow(
      organizationId,
      params.workflow
    )

    return { success: true, data: result }
  } catch (error) {
    console.error('[validateWorkflowAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

export async function duplicateWorkflowActionEnhanced(params: {
  workflowId: string
  name: string
}): Promise<ActionResult<Workflow>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    const workflow = await executeWithRetry(
      () =>
        workflowUseCase.duplicateWorkflow(
          organizationId,
          params.workflowId,
          params.name
        ),
      2,
      500
    )

    return { success: true, data: workflow }
  } catch (error) {
    console.error('[duplicateWorkflowAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

export async function updateWorkflowStatusActionEnhanced(params: {
  workflowId: string
  status: WorkflowStatus
}): Promise<ActionResult<Workflow>> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)

    const workflow = await executeWithRetry(
      () =>
        workflowUseCase.updateWorkflowStatus(
          organizationId,
          params.workflowId,
          params.status
        ),
      2,
      500
    )

    return { success: true, data: workflow }
  } catch (error) {
    console.error('[updateWorkflowStatusAction] Error:', error)
    return {
      success: false,
      error: classifyError(error)
    }
  }
}

// Batch operations with enhanced error handling
export async function batchDeleteWorkflowsActionEnhanced(params: {
  workflowIds: string[]
}): Promise<
  ActionResult<{
    succeeded: string[]
    failed: Array<{ id: string; error: ActionError }>
  }>
> {
  const organizationId = await getOrganizationId()
  const workflowUseCase = container.get(WorkflowUseCase)

  const succeeded: string[] = []
  const failed: Array<{ id: string; error: ActionError }> = []

  await Promise.all(
    params.workflowIds.map(async (id) => {
      try {
        await executeWithRetry(
          () => workflowUseCase.deleteWorkflow(organizationId, id),
          2,
          500
        )
        succeeded.push(id)
      } catch (error) {
        console.error(
          `[batchDeleteWorkflows] Failed to delete workflow ${id}:`,
          error
        )
        failed.push({
          id,
          error: classifyError(error)
        })
      }
    })
  )

  return {
    success: failed.length === 0,
    data: { succeeded, failed },
    error:
      failed.length > 0
        ? {
            type: 'VALIDATION_ERROR',
            message: `Failed to delete ${failed.length} workflow(s)`,
            details: failed
          }
        : undefined
  }
}
