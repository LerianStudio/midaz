import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'

export abstract class WorkflowExecutionRepository {
  abstract create(execution: WorkflowExecution): Promise<WorkflowExecution>
  abstract findById(id: string): Promise<WorkflowExecution | null>
  abstract findAll(params?: {
    workflowId?: string
    status?: string
    limit?: number
    offset?: number
  }): Promise<{ executions: WorkflowExecution[]; total: number }>
  abstract update(
    id: string,
    updates: Partial<WorkflowExecution>
  ): Promise<WorkflowExecution>
  abstract delete(id: string): Promise<void>
  abstract findByWorkflowId(
    workflowId: string,
    limit?: number
  ): Promise<WorkflowExecution[]>
  abstract findRunningExecutions(): Promise<WorkflowExecution[]>
}
