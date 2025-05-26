import { StatusEntity } from './status-entity'

export interface ImportEntity {
  id: string
  ledgerId: string
  organizationId: string
  fileName: string
  filePath: string
  fileSize: number
  status: ImportStatus
  totalRecords: number
  processedRecords: number
  failedRecords: number
  startedAt?: string
  completedAt?: string
  errorDetails?: ImportErrorDetails
  validationResults?: ImportValidationResult[]
  preview?: ImportPreview
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export interface ImportErrorDetails {
  validationErrors: ImportValidationError[]
  processingErrors?: ImportProcessingError[]
  summary?: string
}

export interface ImportValidationError {
  line: number
  field: string
  error: string
  value?: string
  suggestion?: string
}

export interface ImportProcessingError {
  recordIndex: number
  error: string
  details?: string
}

export interface ImportValidationResult {
  field: string
  isValid: boolean
  errors: string[]
  warnings: string[]
  statistics?: ImportFieldStatistics
}

export interface ImportFieldStatistics {
  totalValues: number
  uniqueValues: number
  nullValues: number
  validFormat: number
  invalidFormat: number
  averageLength?: number
  minValue?: number | string
  maxValue?: number | string
}

export interface ImportPreview {
  headers: string[]
  rows: ImportPreviewRow[]
  totalRows: number
  fileType: ImportFileType
  encoding?: string
  delimiter?: string
}

export interface ImportPreviewRow {
  index: number
  data: Record<string, string | number | null>
  isValid: boolean
  errors?: string[]
}

export type ImportStatus =
  | 'pending'
  | 'validating'
  | 'processing'
  | 'completed'
  | 'failed'
  | 'cancelled'

export type ImportFileType = 'csv' | 'json' | 'xlsx' | 'xml'

export interface ImportConfiguration {
  fileType: ImportFileType
  delimiter?: string
  encoding?: string
  hasHeaders: boolean
  skipRows?: number
  fieldMappings: ImportFieldMapping[]
  validation: ImportValidationConfig
}

export interface ImportFieldMapping {
  sourceField: string
  targetField: string
  transformation?: string
  required: boolean
  dataType: 'string' | 'number' | 'date' | 'boolean'
}

export interface ImportValidationConfig {
  strictMode: boolean
  allowPartialImport: boolean
  maxErrorThreshold: number
  requiredFields: string[]
  customValidations?: ImportCustomValidation[]
}

export interface ImportCustomValidation {
  field: string
  rule: string
  message: string
  severity: 'error' | 'warning'
}

export interface CreateImportRequest {
  ledgerId: string
  organizationId: string
  fileName: string
  fileContent: string | File
  configuration: ImportConfiguration
}

export interface ImportProgress {
  importId: string
  status: ImportStatus
  totalRecords: number
  processedRecords: number
  failedRecords: number
  successRate: number
  estimatedTimeRemaining?: number
  currentPhase: ImportPhase
  phaseProgress: number
  errors: ImportValidationError[]
  warnings: string[]
}

export type ImportPhase =
  | 'uploading'
  | 'validating'
  | 'processing'
  | 'indexing'
  | 'finalizing'

export interface ImportSummary {
  totalImports: number
  completedImports: number
  failedImports: number
  averageProcessingTime: number
  totalRecordsProcessed: number
  successRate: number
  recentImports: ImportEntity[]
}
