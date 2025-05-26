export interface Workflow {
  id: string
  name: string
  description?: string
  version: number
  status: WorkflowStatus
  tasks: WorkflowTask[]
  inputParameters: string[]
  outputParameters?: string[]
  createdBy: string
  createdAt: string
  updatedAt: string
  executionCount: number
  lastExecuted?: string
  avgExecutionTime?: string
  successRate: number
  metadata: WorkflowMetadata
}

export type WorkflowStatus = 'ACTIVE' | 'INACTIVE' | 'DRAFT' | 'DEPRECATED'

export interface WorkflowTask {
  name: string
  taskReferenceName: string
  type: TaskType
  description?: string
  inputParameters: Record<string, any>
  outputParameters?: Record<string, any>
  optional?: boolean
  asyncComplete?: boolean
  retryCount?: number
  timeoutSeconds?: number
  retryLogic?: RetryLogic
  startDelay?: number
}

export type TaskType =
  | 'HTTP'
  | 'SWITCH'
  | 'DECISION'
  | 'FORK_JOIN'
  | 'FORK_JOIN_DYNAMIC'
  | 'JOIN'
  | 'SUB_WORKFLOW'
  | 'EVENT'
  | 'WAIT'
  | 'HUMAN'
  | 'TERMINATE'
  | 'LAMBDA'
  | 'KAFKA_PUBLISH'
  | 'JSON_JQ_TRANSFORM'
  | 'SET_VARIABLE'
  | 'CUSTOM'

export interface RetryLogic {
  retryPolicy: 'FIXED' | 'EXPONENTIAL_BACKOFF'
  maxRetries: number
  retryDelaySeconds?: number
  backoffScaleMultiplier?: number
}

export interface WorkflowMetadata {
  category?: string
  tags: string[]
  author?: string
  schemaVersion?: string
  timeoutPolicy?: TimeoutPolicy
  failureWorkflow?: string
  restartable?: boolean
  workflowStatusListenerEnabled?: boolean
  ownerEmail?: string
}

export interface TimeoutPolicy {
  timeoutSeconds: number
  alertAfterTimeoutSeconds?: number
  alertAfterRetryCount?: number
}

export interface CreateWorkflowRequest {
  name: string
  description?: string
  tasks: WorkflowTask[]
  inputParameters?: string[]
  outputParameters?: string[]
  metadata?: Partial<WorkflowMetadata>
}

export interface UpdateWorkflowRequest {
  id: string
  name?: string
  description?: string
  tasks?: WorkflowTask[]
  inputParameters?: string[]
  outputParameters?: string[]
  metadata?: Partial<WorkflowMetadata>
}

export interface WorkflowValidationResult {
  isValid: boolean
  errors: WorkflowValidationError[]
  warnings: WorkflowValidationWarning[]
}

export interface WorkflowValidationError {
  type:
    | 'TASK_NOT_FOUND'
    | 'INVALID_CONNECTION'
    | 'MISSING_PARAMETER'
    | 'CIRCULAR_DEPENDENCY'
    | 'INVALID_CONFIGURATION'
  message: string
  taskName?: string
  field?: string
}

export interface WorkflowValidationWarning {
  type: 'PERFORMANCE' | 'BEST_PRACTICE' | 'DEPRECATED'
  message: string
  taskName?: string
  recommendation?: string
}
