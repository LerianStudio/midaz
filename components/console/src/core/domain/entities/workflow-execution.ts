export interface WorkflowExecution {
  workflowId: string
  workflowName: string
  workflowVersion: number
  executionId: string
  status: ExecutionStatus
  startTime: number
  endTime?: number
  totalExecutionTime?: number
  input: Record<string, any>
  output?: Record<string, any>
  reasonForIncompletion?: string
  failedReferenceTaskNames?: string[]
  tasks: TaskExecution[]
  createdBy: string
  priority: number
  correlationId?: string
  parentWorkflowId?: string
  parentWorkflowTaskId?: string
  externalInputPayloadStoragePath?: string
  externalOutputPayloadStoragePath?: string
  variables?: Record<string, any>
}

export type ExecutionStatus =
  | 'RUNNING'
  | 'COMPLETED'
  | 'FAILED'
  | 'TIMED_OUT'
  | 'TERMINATED'
  | 'PAUSED'

export interface TaskExecution {
  taskId: string
  taskType: string
  taskDefName: string
  referenceTaskName: string
  status: TaskExecutionStatus
  startTime: number
  endTime?: number
  executionTime?: number
  scheduledTime?: number
  updateTime?: number
  retryCount: number
  seq: number
  pollCount?: number
  inputData?: Record<string, any>
  outputData?: Record<string, any>
  reasonForIncompletion?: string
  callbackFromWorker?: boolean
  callbackAfterSeconds?: number
  workerId?: string
  domain?: string
  iteration?: number
  externalInputPayloadStoragePath?: string
  externalOutputPayloadStoragePath?: string
  workflowTask?: any
}

export type TaskExecutionStatus =
  | 'IN_PROGRESS'
  | 'CANCELLED'
  | 'FAILED'
  | 'FAILED_WITH_TERMINAL_ERROR'
  | 'COMPLETED'
  | 'COMPLETED_WITH_ERRORS'
  | 'SCHEDULED'
  | 'TIMED_OUT'
  | 'SKIPPED'

export interface ExecutionSearchResult {
  totalHits: number
  results: WorkflowExecution[]
}

export interface ExecutionSearchRequest {
  query?: string
  start?: number
  size?: number
  sort?: string
  freeText?: string
  workflowNames?: string[]
  statuses?: ExecutionStatus[]
  startTimeFrom?: number
  startTimeTo?: number
  correlationId?: string
}

export interface StartWorkflowRequest {
  name: string
  version?: number
  input: Record<string, any>
  correlationId?: string
  priority?: number
  taskToDomain?: Record<string, string>
  externalInputPayloadStoragePath?: string
}

export interface StartWorkflowResponse {
  workflowId: string
  status: ExecutionStatus
  input: Record<string, any>
  taskToDomain?: Record<string, string>
}

export interface UpdateWorkflowStateRequest {
  workflowId: string
  action: WorkflowAction
  variables?: Record<string, any>
  taskReferenceName?: string
  taskId?: string
}

export type WorkflowAction =
  | 'RESTART'
  | 'RETRY'
  | 'PAUSE'
  | 'RESUME'
  | 'TERMINATE'
  | 'RESET'
  | 'RERUN'

export interface ExecutionMetrics {
  totalExecutions: number
  runningExecutions: number
  completedExecutions: number
  failedExecutions: number
  terminatedExecutions: number
  timedOutExecutions: number
  avgExecutionTime: number
  minExecutionTime: number
  maxExecutionTime: number
  successRate: number
  failureRate: number
  throughput: number
  executionsPerHour: number
}

export interface TaskMetrics {
  taskName: string
  totalExecutions: number
  completedExecutions: number
  failedExecutions: number
  avgExecutionTime: number
  minExecutionTime: number
  maxExecutionTime: number
  successRate: number
  p50ExecutionTime: number
  p95ExecutionTime: number
  p99ExecutionTime: number
}
