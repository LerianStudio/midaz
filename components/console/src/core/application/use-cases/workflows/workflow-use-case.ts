import { inject, injectable } from 'inversify'
import { WorkflowRepository } from '@/core/domain/repositories/workflow-repository'
import {
  Workflow,
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  WorkflowValidationResult,
  WorkflowStatus
} from '@/core/domain/entities/workflow'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { MIDAZ_SYMBOLS } from '@/core/infrastructure/container-registry/midaz/midaz-module'
import { WorkflowExecutionRepository } from '@/core/application/repositories/workflow-execution-repository'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'

@injectable()
export class WorkflowUseCase {
  constructor(
    @inject(MIDAZ_SYMBOLS.WorkflowRepository)
    private readonly workflowRepository: WorkflowRepository,
    @inject(MIDAZ_SYMBOLS.OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository,
    @inject(MIDAZ_SYMBOLS.WorkflowExecutionRepository)
    private readonly executionRepository: WorkflowExecutionRepository
  ) {}

  async createWorkflow(
    organizationId: string,
    workflow: CreateWorkflowRequest
  ): Promise<Workflow> {
    // Validate organization exists
    await this.organizationRepository.findById(organizationId)

    // Validate workflow
    const validation = await this.workflowRepository.validate(
      organizationId,
      workflow
    )
    if (!validation.isValid) {
      throw new Error(
        `Workflow validation failed: ${validation.errors.map((e) => e.message).join(', ')}`
      )
    }

    return this.workflowRepository.create(organizationId, workflow)
  }

  async fetchWorkflows(
    organizationId: string,
    limit: number,
    page: number,
    filters?: {
      status?: string
      category?: string
      search?: string
    }
  ): Promise<PaginationEntity<Workflow>> {
    return this.workflowRepository.fetchAll(
      organizationId,
      limit,
      page,
      filters
    )
  }

  async fetchWorkflowById(
    organizationId: string,
    workflowId: string
  ): Promise<Workflow> {
    return this.workflowRepository.fetchById(organizationId, workflowId)
  }

  async updateWorkflow(
    organizationId: string,
    workflowId: string,
    workflow: UpdateWorkflowRequest
  ): Promise<Workflow> {
    // Get existing workflow to validate changes
    const existingWorkflow = await this.workflowRepository.fetchById(
      organizationId,
      workflowId
    )

    // Create a merged workflow for validation
    const mergedWorkflow = { ...existingWorkflow, ...workflow }

    // Validate updated workflow
    const validation = await this.workflowRepository.validate(
      organizationId,
      mergedWorkflow
    )
    if (!validation.isValid) {
      throw new Error(
        `Workflow validation failed: ${validation.errors.map((e) => e.message).join(', ')}`
      )
    }

    return this.workflowRepository.update(organizationId, workflowId, workflow)
  }

  async deleteWorkflow(
    organizationId: string,
    workflowId: string
  ): Promise<void> {
    return this.workflowRepository.delete(organizationId, workflowId)
  }

  async validateWorkflow(
    organizationId: string,
    workflow: Workflow | CreateWorkflowRequest
  ): Promise<WorkflowValidationResult> {
    return this.workflowRepository.validate(organizationId, workflow)
  }

  async duplicateWorkflow(
    organizationId: string,
    workflowId: string,
    name: string
  ): Promise<Workflow> {
    // Validate the new name doesn't exist
    const workflows = await this.workflowRepository.fetchAll(
      organizationId,
      100,
      1
    )
    const nameExists = workflows.items.some((w) => w.name === name)
    if (nameExists) {
      throw new Error(`A workflow with the name "${name}" already exists`)
    }

    return this.workflowRepository.duplicate(organizationId, workflowId, name)
  }

  async updateWorkflowStatus(
    organizationId: string,
    workflowId: string,
    status: WorkflowStatus
  ): Promise<Workflow> {
    // Get workflow to validate status change
    const workflow = await this.workflowRepository.fetchById(
      organizationId,
      workflowId
    )

    // Validate status transitions
    if (workflow.status === 'DEPRECATED' && status !== 'DEPRECATED') {
      throw new Error('Cannot change status of a deprecated workflow')
    }

    if (status === 'ACTIVE' && workflow.status === 'DRAFT') {
      // Validate workflow before activating
      const validation = await this.workflowRepository.validate(
        organizationId,
        workflow
      )
      if (!validation.isValid) {
        throw new Error(
          `Cannot activate workflow: ${validation.errors.map((e) => e.message).join(', ')}`
        )
      }
    }

    return this.workflowRepository.updateStatus(
      organizationId,
      workflowId,
      status
    )
  }

  async startExecution(
    workflowId: string,
    input?: Record<string, any>
  ): Promise<WorkflowExecution> {
    // Get the workflow first to validate it exists and is active
    const workflow = await this.workflowRepository.fetchById(
      'default',
      workflowId
    )

    if (workflow.status !== 'ACTIVE') {
      throw new Error(
        `Cannot start execution: workflow is not active (current status: ${workflow.status})`
      )
    }

    // Create a new execution
    const execution: WorkflowExecution = {
      workflowId,
      workflowName: workflow.name,
      workflowVersion: workflow.version,
      executionId: `exec-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      status: 'RUNNING',
      startTime: Date.now(),
      input: input || {},
      output: {},
      tasks: [],
      createdBy: 'system',
      priority: 0
    }

    return this.executionRepository.create(execution)
  }
}
