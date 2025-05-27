import { injectable } from 'inversify'
import { WorkflowExecutionRepository } from '@/core/application/repositories/workflow-execution-repository'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'

@injectable()
export class WorkflowExecutionMockRepository extends WorkflowExecutionRepository {
  private executions: Map<string, WorkflowExecution> = new Map()

  constructor() {
    super()
    // Initialize with mock data
    mockWorkflowExecutions.forEach((execution) => {
      this.executions.set(execution.executionId, execution)
    })
  }

  async create(execution: WorkflowExecution): Promise<WorkflowExecution> {
    const newExecution = {
      ...execution,
      executionId:
        execution.executionId ||
        `exec_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
    }

    this.executions.set(newExecution.executionId, newExecution)

    // Simulate async execution progression
    this.simulateExecutionProgress(newExecution.executionId)

    return newExecution
  }

  async findById(id: string): Promise<WorkflowExecution | null> {
    return this.executions.get(id) || null
  }

  async findAll(params?: {
    workflowId?: string
    status?: string
    limit?: number
    offset?: number
  }): Promise<{ executions: WorkflowExecution[]; total: number }> {
    let executions = Array.from(this.executions.values())

    // Apply filters
    if (params?.workflowId) {
      executions = executions.filter((e) => e.workflowId === params.workflowId)
    }

    if (params?.status) {
      executions = executions.filter((e) => e.status === params.status)
    }

    // Sort by start time (newest first)
    executions.sort((a, b) => {
      const timeA = a.startTime || 0
      const timeB = b.startTime || 0
      return timeB - timeA
    })

    const total = executions.length

    // Apply pagination
    const offset = params?.offset || 0
    const limit = params?.limit || 10
    executions = executions.slice(offset, offset + limit)

    return { executions, total }
  }

  async update(
    id: string,
    updates: Partial<WorkflowExecution>
  ): Promise<WorkflowExecution> {
    const execution = this.executions.get(id)

    if (!execution) {
      throw new Error(`Execution ${id} not found`)
    }

    const updatedExecution = {
      ...execution,
      ...updates
    }

    this.executions.set(id, updatedExecution)
    return updatedExecution
  }

  async delete(id: string): Promise<void> {
    this.executions.delete(id)
  }

  async findByWorkflowId(
    workflowId: string,
    limit?: number
  ): Promise<WorkflowExecution[]> {
    const executions = Array.from(this.executions.values())
      .filter((e) => e.workflowId === workflowId)
      .sort((a, b) => {
        const timeA = a.startTime || 0
        const timeB = b.startTime || 0
        return timeB - timeA
      })

    return limit ? executions.slice(0, limit) : executions
  }

  async findRunningExecutions(): Promise<WorkflowExecution[]> {
    return Array.from(this.executions.values()).filter(
      (e) => e.status === 'RUNNING' || e.status === 'PAUSED'
    )
  }

  // Simulate execution progression
  private simulateExecutionProgress(executionId: string) {
    setTimeout(async () => {
      const execution = this.executions.get(executionId)
      if (!execution || execution.status !== 'RUNNING') return

      // Add some task progress
      const tasks = execution.tasks || []
      if (tasks.length === 0) {
        // Create initial tasks based on workflow
        const initialTasks = [
          {
            taskId: `task_${Date.now()}_1`,
            taskType: 'HTTP',
            taskDefName: 'validate_accounts',
            referenceTaskName: 'account_validation',
            status: 'IN_PROGRESS' as const,
            startTime: Date.now(),
            workerId: 'worker-1',
            retryCount: 0,
            seq: 1,
            pollCount: 1,
            inputData: execution.input,
            outputData: {},
            logs: []
          }
        ]

        await this.update(executionId, { tasks: initialTasks })

        // Continue simulation
        setTimeout(() => this.simulateExecutionProgress(executionId), 2000)
      } else {
        // Progress existing tasks
        const lastTask = tasks[tasks.length - 1]

        if (lastTask.status === 'IN_PROGRESS') {
          // Complete current task
          lastTask.status = 'COMPLETED'
          lastTask.endTime = Date.now()
          lastTask.outputData = { result: 'success' }

          // Add next task if not at the end
          if (tasks.length < 3) {
            const nextTask = {
              taskId: `task_${Date.now()}_${tasks.length + 1}`,
              taskType: tasks.length === 1 ? 'HTTP' : 'SIMPLE',
              taskDefName:
                tasks.length === 1 ? 'calculate_fees' : 'create_transaction',
              referenceTaskName:
                tasks.length === 1 ? 'fee_calculation' : 'transaction_creation',
              status: 'IN_PROGRESS' as const,
              startTime: Date.now(),
              workerId: 'worker-1',
              retryCount: 0,
              seq: tasks.length + 1,
              pollCount: 1,
              inputData: lastTask.outputData,
              outputData: {},
              logs: []
            }

            tasks.push(nextTask)
            await this.update(executionId, { tasks })

            // Continue simulation
            setTimeout(() => this.simulateExecutionProgress(executionId), 3000)
          } else {
            // Complete execution
            await this.update(executionId, {
              status: 'COMPLETED',
              endTime: Date.now(),
              output: {
                transactionId: `txn_${Date.now()}`,
                status: 'completed',
                totalAmount: execution.input.amount,
                fees: 15
              },
              tasks
            })
          }
        }
      }
    }, 1000)
  }
}
