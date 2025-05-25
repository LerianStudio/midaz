export interface Template {
  id: string
  name: string
  description?: string
  category: string
  tags: string[]
  status: TemplateStatus
  fileUrl: string
  mappedFields: Record<string, TableFields>
  validated: boolean
  active: boolean
  usageCount: number
  lastUsed?: string
  metadata: Record<string, any>
  createdAt: string
  updatedAt: string
  createdBy: string
  // Additional properties for template details
  format?: string
  engine?: string
  version?: string
  dataSourceIds?: string[]
  content?: string
}

export interface TableFields {
  table: string
  fields: string[]
  queries?: string[]
}

export interface TemplateFieldMapping {
  templateId: string
  mappedFields: Record<string, TableFields>
  validated: boolean
  updatedAt: string
}

export type TemplateStatus = 'active' | 'inactive' | 'draft' | 'archived'

export type TemplateCategory = 'FINANCIAL' | 'OPERATIONAL' | 'COMPLIANCE' | 'MARKETING' | 'CUSTOM'

export interface CreateTemplateInput {
  name: string
  description?: string
  category: string
  tags: string[]
  fileContent: string
  fileName: string
}

export interface UpdateTemplateInput {
  name?: string
  description?: string
  category?: string
  tags?: string[]
  active?: boolean
  metadata?: Record<string, any>
}

export interface TemplateFilters {
  category?: string
  status?: TemplateStatus
  tags?: string[]
  search?: string
  createdBy?: string
}

export interface TemplateValidationResult {
  isValid: boolean
  errors: TemplateValidationError[]
  warnings: TemplateValidationWarning[]
  extractedFields: string[]
}

export interface TemplateValidationError {
  type: 'syntax' | 'field' | 'data_source' | 'security'
  message: string
  line?: number
  column?: number
  field?: string
}

export interface TemplateValidationWarning {
  type: 'performance' | 'best_practice' | 'compatibility'
  message: string
  suggestion?: string
}
