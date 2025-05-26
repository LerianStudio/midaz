import {
  Workflow,
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  WorkflowValidationResult
} from '../entities/workflow'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class WorkflowRepository {
  abstract create: (
    organizationId: string,
    workflow: CreateWorkflowRequest
  ) => Promise<Workflow>

  abstract fetchAll: (
    organizationId: string,
    limit: number,
    page: number,
    filters?: {
      status?: string
      category?: string
      search?: string
    }
  ) => Promise<PaginationEntity<Workflow>>

  abstract fetchById: (
    organizationId: string,
    workflowId: string
  ) => Promise<Workflow>

  abstract update: (
    organizationId: string,
    workflowId: string,
    workflow: UpdateWorkflowRequest
  ) => Promise<Workflow>

  abstract delete: (organizationId: string, workflowId: string) => Promise<void>

  abstract validate: (
    organizationId: string,
    workflow: Workflow | CreateWorkflowRequest
  ) => Promise<WorkflowValidationResult>

  abstract duplicate: (
    organizationId: string,
    workflowId: string,
    name: string
  ) => Promise<Workflow>

  abstract updateStatus: (
    organizationId: string,
    workflowId: string,
    status: 'ACTIVE' | 'INACTIVE' | 'DRAFT' | 'DEPRECATED'
  ) => Promise<Workflow>
}
