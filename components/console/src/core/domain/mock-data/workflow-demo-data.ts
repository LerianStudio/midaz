import {
  Workflow,
  WorkflowTask,
  WorkflowStatus,
  TaskType
} from '../entities/workflow'
import {
  WorkflowExecution,
  ExecutionStatus
} from '../entities/workflow-execution'
import { WorkflowTemplate } from '../entities/workflow-template'
import { mockWorkflowTemplates } from './workflow-templates'

// Generate demo workflows from templates
export const generateDemoWorkflows = (): Workflow[] => {
  const demoWorkflows: Workflow[] = []

  // Create variations of template-based workflows
  mockWorkflowTemplates.forEach((template, index) => {
    // Create 2-3 workflow instances per template
    for (let i = 0; i < Math.min(3, Math.ceil(Math.random() * 3) + 1); i++) {
      const workflow: Workflow = {
        id: `workflow-${template.id}-${i + 1}`,
        name: `${template.name} v${i + 1}`,
        description: `${template.description} (Instance ${i + 1})`,
        version: i + 1,
        status: ['ACTIVE', 'INACTIVE', 'DRAFT', 'DEPRECATED'][
          Math.floor(Math.random() * 4)
        ] as WorkflowStatus,
        tasks: template.workflow.tasks.map(
          (task, taskIndex): WorkflowTask => ({
            name: task.name,
            taskReferenceName: `${task.name.toLowerCase().replace(/\s+/g, '_')}_${taskIndex}`,
            type: task.type as TaskType,
            description: task.description || '',
            optional: task.optional || false,
            inputParameters: task.defaultConfiguration || {},
            taskDefinition: {
              name: task.name,
              description: task.description || '',
              inputKeys: Object.keys(task.defaultConfiguration || {}),
              outputKeys: ['result', 'status'],
              timeoutSeconds: 300,
              retryCount: 2
            }
          })
        ),
        inputParameters:
          template.workflow.inputParameters
            ?.map((param) => ({ [param]: 'string' }))
            .reduce((acc, curr) => ({ ...acc, ...curr }), {}) || {},
        outputParameters:
          template.workflow.outputParameters
            ?.map((param) => ({ [param]: 'string' }))
            .reduce((acc, curr) => ({ ...acc, ...curr }), {}) || {},
        timeoutSeconds: template.workflow.timeoutSeconds || 3600,
        schemaVersion: 2,
        ownerEmail: 'system@midaz.io',
        createdAt: new Date(
          Date.now() - Math.random() * 30 * 24 * 60 * 60 * 1000
        ).toISOString(),
        updatedAt: new Date(
          Date.now() - Math.random() * 7 * 24 * 60 * 60 * 1000
        ).toISOString(),
        executionCount: Math.floor(Math.random() * 100),
        successRate: Math.floor(Math.random() * 100),
        metadata: {
          category: template.category,
          tags: template.metadata?.tags || [],
          author: template.metadata?.author || 'system'
        },
        createdBy: ['admin', 'system', 'user@midaz.io'][
          Math.floor(Math.random() * 3)
        ],
        updatedBy: ['admin', 'system', 'user@midaz.io'][
          Math.floor(Math.random() * 3)
        ]
      }

      demoWorkflows.push(workflow)
    }
  })

  return demoWorkflows
}

// Generate demo workflow executions
export const generateDemoExecutions = (
  workflows: Workflow[]
): WorkflowExecution[] => {
  const executions: WorkflowExecution[] = []
  const statuses: ExecutionStatus[] = [
    'RUNNING',
    'COMPLETED',
    'FAILED',
    'PAUSED',
    'TERMINATED'
  ]

  // Generate 50-100 executions across all workflows
  const executionCount = 75 + Math.floor(Math.random() * 25)

  for (let i = 0; i < executionCount; i++) {
    const workflow = workflows[Math.floor(Math.random() * workflows.length)]
    const status = statuses[Math.floor(Math.random() * statuses.length)]
    const startTime = new Date(
      Date.now() - Math.random() * 7 * 24 * 60 * 60 * 1000
    )
    const endTime =
      status === 'COMPLETED' || status === 'FAILED' || status === 'TERMINATED'
        ? new Date(startTime.getTime() + Math.random() * 60 * 60 * 1000)
        : null

    const execution: WorkflowExecution = {
      workflowId: workflow.id,
      workflowName: workflow.name,
      workflowVersion: workflow.version,
      executionId: `exec-${Date.now()}-${i}`,
      status,
      startTime: startTime.getTime(),
      endTime: endTime?.getTime() || undefined,
      totalExecutionTime: endTime
        ? endTime.getTime() - startTime.getTime()
        : undefined,
      input: generateSampleInput(workflow),
      output: endTime ? generateSampleOutput(workflow, status) : undefined,
      reasonForIncompletion:
        status === 'FAILED' ? generateFailureReason() : undefined,
      failedReferenceTaskNames:
        status === 'FAILED'
          ? [
              workflow.tasks[Math.floor(Math.random() * workflow.tasks.length)]
                .taskReferenceName
            ]
          : [],
      tasks: generateExecutionTasks(workflow, status),
      createdBy: ['system', 'scheduler', 'api', 'user@midaz.io'][
        Math.floor(Math.random() * 4)
      ],
      priority: Math.floor(Math.random() * 100),
      correlationId: `corr-${Date.now()}-${i}`,
      parentWorkflowId: undefined,
      parentWorkflowTaskId: undefined,
      variables: {}
    }

    executions.push(execution)
  }

  return executions.sort((a, b) => b.startTime - a.startTime)
}

const generateSampleInput = (workflow: Workflow): Record<string, any> => {
  const input: Record<string, any> = {}

  Object.keys(workflow.inputParameters || {}).forEach((key) => {
    switch (key) {
      case 'payment_amount':
        input[key] = Math.floor(Math.random() * 10000) / 100
        break
      case 'payment_type':
        input[key] = ['credit_card', 'bank_transfer', 'digital_wallet'][
          Math.floor(Math.random() * 3)
        ]
        break
      case 'account_id':
        input[key] =
          `ACC${String(Math.floor(Math.random() * 100000000)).padStart(8, '0')}`
        break
      case 'customer_id':
        input[key] =
          `CUST${String(Math.floor(Math.random() * 100000000)).padStart(8, '0')}`
        break
      case 'reconciliation_date':
        input[key] = new Date(
          Date.now() - Math.random() * 30 * 24 * 60 * 60 * 1000
        )
          .toISOString()
          .split('T')[0]
        break
      default:
        input[key] = `sample_${key}_value`
    }
  })

  return input
}

const generateSampleOutput = (
  workflow: Workflow,
  status: string
): Record<string, any> => {
  const output: Record<string, any> = {}

  if (status === 'COMPLETED') {
    Object.keys(workflow.outputParameters || {}).forEach((key) => {
      switch (key) {
        case 'transaction_id':
          output[key] =
            `TXN${String(Math.floor(Math.random() * 100000000)).padStart(8, '0')}`
          break
        case 'status':
          output[key] = 'SUCCESS'
          break
        case 'authorization_code':
          output[key] =
            `AUTH${String(Math.floor(Math.random() * 1000000)).padStart(6, '0')}`
          break
        case 'report_id':
          output[key] =
            `RPT${String(Math.floor(Math.random() * 100000000)).padStart(8, '0')}`
          break
        default:
          output[key] = `result_${key}_value`
      }
    })
  }

  return output
}

const generateFailureReason = (): string => {
  const reasons = [
    'HTTP request timeout',
    'Invalid input parameters',
    'External service unavailable',
    'Authentication failed',
    'Rate limit exceeded',
    'Database connection error',
    'Validation error',
    'Insufficient privileges'
  ]

  return reasons[Math.floor(Math.random() * reasons.length)]
}

const generateExecutionTasks = (
  workflow: Workflow,
  executionStatus: ExecutionStatus
): any[] => {
  return workflow.tasks.map((task, index) => {
    let taskStatus = 'COMPLETED'

    if (executionStatus === 'RUNNING') {
      if (index === Math.floor(workflow.tasks.length / 2)) {
        taskStatus = 'IN_PROGRESS'
      } else if (index > Math.floor(workflow.tasks.length / 2)) {
        taskStatus = 'SCHEDULED'
      }
    } else if (
      executionStatus === 'FAILED' &&
      index === workflow.tasks.length - 1
    ) {
      taskStatus = 'FAILED'
    } else if (
      executionStatus === 'PAUSED' &&
      index === Math.floor(workflow.tasks.length * 0.7)
    ) {
      taskStatus = 'PAUSED'
    }

    return {
      taskType: task.type,
      status: taskStatus,
      referenceTaskName: task.taskReferenceName,
      retryCount: 0,
      seq: index + 1,
      correlationId: null,
      pollCount:
        taskStatus === 'IN_PROGRESS' ? Math.floor(Math.random() * 5) + 1 : 0,
      taskDefName: task.name,
      scheduledTime: Date.now() - (workflow.tasks.length - index) * 30000,
      startTime:
        taskStatus !== 'SCHEDULED'
          ? Date.now() - (workflow.tasks.length - index) * 30000 + 5000
          : 0,
      endTime:
        taskStatus === 'COMPLETED' || taskStatus === 'FAILED'
          ? Date.now() - (workflow.tasks.length - index - 1) * 30000
          : 0,
      updateTime: Date.now(),
      startDelayInSeconds: 0,
      retriedTaskId: null,
      executed: taskStatus !== 'SCHEDULED',
      callbackFromWorker: true,
      responseTimeoutSeconds: 300,
      workflowInstanceId: `exec-${Date.now()}-${Math.floor(Math.random() * 1000)}`,
      workflowType: workflow.name,
      taskId: `task-${Date.now()}-${index}`,
      reasonForIncompletion:
        taskStatus === 'FAILED' ? generateFailureReason() : null,
      callbackAfterSeconds: 0,
      workerId: `worker-${Math.floor(Math.random() * 10) + 1}`,
      inputData: task.inputParameters || {},
      outputData:
        taskStatus === 'COMPLETED'
          ? { result: 'success', timestamp: new Date().toISOString() }
          : {},
      domain: null,
      queueWaitTime: Math.floor(Math.random() * 1000),
      logs:
        taskStatus === 'FAILED'
          ? [{ log: 'Task execution failed', createdTime: Date.now() }]
          : []
    }
  })
}

// Export demo data
export const demoWorkflows = generateDemoWorkflows()
export const demoExecutions = generateDemoExecutions(demoWorkflows)

// Helper functions for demo
export const getDemoWorkflowById = (id: string): Workflow | undefined => {
  return demoWorkflows.find((workflow) => workflow.id === id)
}

export const getDemoExecutionById = (
  id: string
): WorkflowExecution | undefined => {
  return demoExecutions.find((execution) => execution.executionId === id)
}

export const getDemoExecutionsByWorkflow = (
  workflowName: string
): WorkflowExecution[] => {
  return demoExecutions.filter(
    (execution) => execution.workflowName === workflowName
  )
}

export const getDemoExecutionsByStatus = (
  status: ExecutionStatus
): WorkflowExecution[] => {
  return demoExecutions.filter((execution) => execution.status === status)
}
