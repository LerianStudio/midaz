import { injectable } from 'inversify'
import { WorkflowRepository } from '@/core/domain/repositories/workflow-repository'
import {
  Workflow,
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  WorkflowValidationResult,
  WorkflowStatus
} from '@/core/domain/entities/workflow'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { mockWorkflows } from '@/lib/mock-data/workflows'

@injectable()
export class WorkflowMockRepository extends WorkflowRepository {
  private workflows: Map<string, Workflow> = new Map(
    mockWorkflows.map((workflow) => [`${workflow.id}`, workflow])
  )

  async create(
    organizationId: string,
    workflowRequest: CreateWorkflowRequest
  ): Promise<Workflow> {
    const now = new Date().toISOString()
    const workflow: Workflow = {
      id: `01956b69-9102-75b7-8860-${Date.now().toString(16)}`,
      name: workflowRequest.name,
      description: workflowRequest.description,
      version: 1,
      status: 'DRAFT' as WorkflowStatus,
      tasks: workflowRequest.tasks || [],
      inputParameters: workflowRequest.inputParameters || [],
      outputParameters: workflowRequest.outputParameters,
      createdBy: 'current.user@company.com', // In real implementation, get from session
      createdAt: now,
      updatedAt: now,
      executionCount: 0,
      successRate: 0,
      metadata: {
        tags: [],
        ...workflowRequest.metadata
      }
    }

    this.workflows.set(workflow.id, workflow)
    return workflow
  }

  async fetchAll(
    organizationId: string,
    limit: number,
    page: number,
    filters?: {
      status?: string
      category?: string
      search?: string
    }
  ): Promise<PaginationEntity<Workflow>> {
    let filteredWorkflows = Array.from(this.workflows.values())

    if (filters) {
      if (filters.status) {
        filteredWorkflows = filteredWorkflows.filter(
          (w) => w.status === filters.status
        )
      }
      if (filters.category) {
        filteredWorkflows = filteredWorkflows.filter(
          (w) => w.metadata.category === filters.category
        )
      }
      if (filters.search) {
        const searchLower = filters.search.toLowerCase()
        filteredWorkflows = filteredWorkflows.filter(
          (w) =>
            w.name.toLowerCase().includes(searchLower) ||
            w.description?.toLowerCase().includes(searchLower) ||
            w.metadata.tags.some((tag) =>
              tag.toLowerCase().includes(searchLower)
            )
        )
      }
    }

    const startIndex = (page - 1) * limit
    const endIndex = startIndex + limit
    const paginatedWorkflows = filteredWorkflows.slice(startIndex, endIndex)

    return {
      items: paginatedWorkflows,
      page,
      limit,
      total: filteredWorkflows.length,
      hasMore: endIndex < filteredWorkflows.length
    }
  }

  async fetchById(
    organizationId: string,
    workflowId: string
  ): Promise<Workflow> {
    const workflow = this.workflows.get(workflowId)
    if (!workflow) {
      throw new Error(`Workflow with id ${workflowId} not found`)
    }
    return workflow
  }

  async findById(workflowId: string): Promise<Workflow> {
    const workflow = this.workflows.get(workflowId)
    if (!workflow) {
      throw new Error(`Workflow with id ${workflowId} not found`)
    }
    return workflow
  }

  async update(
    organizationId: string,
    workflowId: string,
    updateRequest: UpdateWorkflowRequest
  ): Promise<Workflow> {
    const workflow = await this.fetchById(organizationId, workflowId)

    const updatedWorkflow: Workflow = {
      ...workflow,
      ...updateRequest,
      id: workflow.id, // Ensure ID is not changed
      version: workflow.version + 1,
      updatedAt: new Date().toISOString(),
      metadata: {
        ...workflow.metadata,
        ...updateRequest.metadata
      }
    }

    this.workflows.set(workflowId, updatedWorkflow)
    return updatedWorkflow
  }

  async delete(organizationId: string, workflowId: string): Promise<void> {
    const workflow = await this.fetchById(organizationId, workflowId)
    if (workflow.status === 'ACTIVE') {
      throw new Error(
        'Cannot delete an active workflow. Please deactivate it first.'
      )
    }
    this.workflows.delete(workflowId)
  }

  async validate(
    organizationId: string,
    workflow: Workflow | CreateWorkflowRequest
  ): Promise<WorkflowValidationResult> {
    const errors = []
    const warnings = []

    // Basic validation
    if (!workflow.name || workflow.name.trim() === '') {
      errors.push({
        type: 'INVALID_CONFIGURATION' as const,
        message: 'Workflow name is required'
      })
    }

    if (!workflow.tasks || workflow.tasks.length === 0) {
      errors.push({
        type: 'INVALID_CONFIGURATION' as const,
        message: 'Workflow must have at least one task'
      })
    }

    // Task validation
    workflow.tasks?.forEach((task, index) => {
      if (!task.name || !task.taskReferenceName) {
        errors.push({
          type: 'INVALID_CONFIGURATION' as const,
          message: `Task at position ${index + 1} is missing required fields`,
          taskName: task.name
        })
      }

      // Check for duplicate task reference names
      const duplicates = workflow.tasks?.filter(
        (t) => t.taskReferenceName === task.taskReferenceName
      )
      if (duplicates && duplicates.length > 1) {
        errors.push({
          type: 'INVALID_CONFIGURATION' as const,
          message: `Duplicate task reference name: ${task.taskReferenceName}`,
          taskName: task.name
        })
      }
    })

    // Performance warnings
    if (workflow.tasks && workflow.tasks.length > 20) {
      warnings.push({
        type: 'PERFORMANCE' as const,
        message: 'Workflow has more than 20 tasks which may impact performance',
        recommendation: 'Consider breaking down into sub-workflows'
      })
    }

    return {
      isValid: errors.length === 0,
      errors,
      warnings
    }
  }

  async duplicate(
    organizationId: string,
    workflowId: string,
    name: string
  ): Promise<Workflow> {
    const originalWorkflow = await this.fetchById(organizationId, workflowId)

    const duplicatedWorkflow: Workflow = {
      ...originalWorkflow,
      id: `01956b69-9102-75b7-8860-${Date.now().toString(16)}`,
      name,
      version: 1,
      status: 'DRAFT' as WorkflowStatus,
      executionCount: 0,
      lastExecuted: undefined,
      avgExecutionTime: undefined,
      successRate: 0,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString()
    }

    this.workflows.set(duplicatedWorkflow.id, duplicatedWorkflow)
    return duplicatedWorkflow
  }

  async updateStatus(
    organizationId: string,
    workflowId: string,
    status: WorkflowStatus
  ): Promise<Workflow> {
    const workflow = await this.fetchById(organizationId, workflowId)

    const updatedWorkflow: Workflow = {
      ...workflow,
      status,
      updatedAt: new Date().toISOString()
    }

    this.workflows.set(workflowId, updatedWorkflow)
    return updatedWorkflow
  }
}
