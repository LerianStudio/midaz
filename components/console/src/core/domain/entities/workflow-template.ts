export interface WorkflowTemplate {
  id: string
  name: string
  description: string
  category: TemplateCategory
  tags: string[]
  workflow: TemplateWorkflow
  parameters: TemplateParameter[]
  usageCount: number
  rating: number
  createdBy: string
  createdAt: string
  updatedAt: string
  isPublic: boolean
  metadata: TemplateMetadata
}

export type TemplateCategory =
  | 'payments'
  | 'onboarding'
  | 'compliance'
  | 'reconciliation'
  | 'reporting'
  | 'notifications'
  | 'integration'
  | 'custom'

export interface TemplateWorkflow {
  name: string
  description: string
  tasks: TemplateTask[]
  inputParameters?: string[]
  outputParameters?: string[]
  timeoutSeconds?: number
}

export interface TemplateTask {
  name: string
  type: string
  description: string
  configurable?: TemplateTaskConfigurable
  optional?: boolean
  defaultConfiguration?: Record<string, any>
}

export interface TemplateTaskConfigurable {
  endpoint?: boolean
  method?: boolean
  headers?: boolean
  body?: boolean
  timeout?: boolean
  retries?: boolean
}

export interface TemplateParameter {
  name: string
  type: ParameterType
  required: boolean
  description: string
  defaultValue?: any
  validation?: ParameterValidation
  options?: ParameterOption[]
}

export type ParameterType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'object'
  | 'array'
  | 'select'
  | 'multiselect'

export interface ParameterValidation {
  pattern?: string
  min?: number
  max?: number
  minLength?: number
  maxLength?: number
}

export interface ParameterOption {
  label: string
  value: any
  description?: string
}

export interface TemplateMetadata {
  version: string
  schemaVersion: string
  complexity: TemplateComplexity
  estimatedDuration: string
  requiredServices: string[]
  supportedFormats: string[]
  documentation?: string
  examples?: TemplateExample[]
}

export type TemplateComplexity = 'SIMPLE' | 'MEDIUM' | 'COMPLEX' | 'ADVANCED'

export interface TemplateExample {
  name: string
  description: string
  input: Record<string, any>
  expectedOutput: Record<string, any>
  notes?: string
}

export interface CreateTemplateRequest {
  name: string
  description: string
  category: TemplateCategory
  tags: string[]
  workflow: TemplateWorkflow
  parameters: TemplateParameter[]
  isPublic?: boolean
  metadata?: Partial<TemplateMetadata>
}

export interface UseTemplateRequest {
  templateId: string
  workflowName: string
  parameters: Record<string, any>
  customizations?: TemplateCustomization[]
}

export interface TemplateCustomization {
  taskName: string
  field: string
  value: any
}

export interface TemplateSearchRequest {
  query?: string
  category?: TemplateCategory
  tags?: string[]
  complexity?: TemplateComplexity
  rating?: number
  sortBy?: 'name' | 'rating' | 'usage' | 'created' | 'updated'
  sortOrder?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export interface TemplateSearchResult {
  total: number
  templates: WorkflowTemplate[]
}

export interface TemplateUsageAnalytics {
  templateId: string
  totalUsage: number
  usageThisMonth: number
  usageGrowth: number
  avgRating: number
  totalRatings: number
  successRate: number
  avgExecutionTime: number
  popularParameters: ParameterUsage[]
  userFeedback: TemplateFeedback[]
}

export interface ParameterUsage {
  parameter: string
  usageCount: number
  popularValues: { value: any; count: number }[]
}

export interface TemplateFeedback {
  userId: string
  rating: number
  comment?: string
  createdAt: string
  helpful: boolean
}
