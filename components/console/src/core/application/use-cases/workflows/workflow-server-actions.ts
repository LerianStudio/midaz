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

export async function createWorkflowAction(params: {
  workflow: CreateWorkflowRequest
}): Promise<{ success: boolean; data?: Workflow; error?: string }> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    const workflow = await workflowUseCase.createWorkflow(
      organizationId,
      params.workflow
    )
    return { success: true, data: workflow }
  } catch (error) {
    console.error('Failed to create workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to create workflow'
    }
  }
}

export async function fetchWorkflowByIdAction(params: {
  workflowId: string
}): Promise<{ success: boolean; data?: Workflow; error?: string }> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    const workflow = await workflowUseCase.fetchWorkflowById(
      organizationId,
      params.workflowId
    )
    return { success: true, data: workflow }
  } catch (error) {
    console.error('Failed to fetch workflow:', error)
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Failed to fetch workflow'
    }
  }
}

export async function updateWorkflowAction(params: {
  workflowId: string
  workflow: UpdateWorkflowRequest
}): Promise<{ success: boolean; data?: Workflow; error?: string }> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    const workflow = await workflowUseCase.updateWorkflow(
      organizationId,
      params.workflowId,
      params.workflow
    )
    return { success: true, data: workflow }
  } catch (error) {
    console.error('Failed to update workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to update workflow'
    }
  }
}

export async function deleteWorkflowAction(params: {
  workflowId: string
}): Promise<{ success: boolean; error?: string }> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    await workflowUseCase.deleteWorkflow(organizationId, params.workflowId)
    return { success: true }
  } catch (error) {
    console.error('Failed to delete workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to delete workflow'
    }
  }
}

export async function validateWorkflowAction(params: {
  workflow: Workflow | CreateWorkflowRequest
}): Promise<{
  success: boolean
  data?: WorkflowValidationResult
  error?: string
}> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    const result = await workflowUseCase.validateWorkflow(
      organizationId,
      params.workflow
    )
    return { success: true, data: result }
  } catch (error) {
    console.error('Failed to validate workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to validate workflow'
    }
  }
}

export async function duplicateWorkflowAction(params: {
  workflowId: string
  name: string
}): Promise<{ success: boolean; data?: Workflow; error?: string }> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    const workflow = await workflowUseCase.duplicateWorkflow(
      organizationId,
      params.workflowId,
      params.name
    )
    return { success: true, data: workflow }
  } catch (error) {
    console.error('Failed to duplicate workflow:', error)
    return {
      success: false,
      error:
        error instanceof Error ? error.message : 'Failed to duplicate workflow'
    }
  }
}

export async function updateWorkflowStatusAction(params: {
  workflowId: string
  status: WorkflowStatus
}): Promise<{ success: boolean; data?: Workflow; error?: string }> {
  try {
    const organizationId = await getOrganizationId()
    const workflowUseCase = container.get(WorkflowUseCase)
    const workflow = await workflowUseCase.updateWorkflowStatus(
      organizationId,
      params.workflowId,
      params.status
    )
    return { success: true, data: workflow }
  } catch (error) {
    console.error('Failed to update workflow status:', error)
    return {
      success: false,
      error:
        error instanceof Error
          ? error.message
          : 'Failed to update workflow status'
    }
  }
}
