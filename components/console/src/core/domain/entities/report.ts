export interface Report {
  id: string
  templateId: string
  templateName: string
  status: ReportStatus
  format: ReportFormat
  parameters: Record<string, any>
  fileUrl?: string
  fileSize?: number
  generatedAt?: string
  downloadCount: number
  expiresAt?: string
  processingTime?: string
  queuePosition?: number
  estimatedCompletion?: string
  startedAt?: string
  metadata: Record<string, any>
  createdAt: string
  createdBy: string
}

export type ReportStatus =
  | 'queued'
  | 'processing'
  | 'completed'
  | 'failed'
  | 'expired'
export type ReportFormat = 'html' | 'pdf' | 'csv' | 'json' | 'xlsx'

export interface CreateReportInput {
  templateId: string
  format: ReportFormat
  parameters: Record<string, any>
  options?: ReportGenerationOptions
}

export interface ReportGenerationOptions {
  locale?: string
  timezone?: string
  compression?: boolean
  watermark?: boolean
  expiresIn?: number // hours
  priority?: 'low' | 'normal' | 'high'
}

export interface ReportFilters {
  templateId?: string
  status?: ReportStatus
  format?: ReportFormat
  createdBy?: string
  dateRange?: {
    from: string
    to: string
  }
}

export interface ReportProgress {
  reportId: string
  status: ReportStatus
  progress: number
  currentStep: string
  estimatedTimeRemaining?: number
  error?: ReportError
}

export interface ReportError {
  code: string
  message: string
  details?: Record<string, any>
  timestamp: string
}

export interface ReportAnalytics {
  totalReports: number
  reportsByStatus: Record<ReportStatus, number>
  reportsByFormat: Record<ReportFormat, number>
  avgGenerationTime: number
  popularTemplates: {
    templateId: string
    templateName: string
    count: number
  }[]
}
