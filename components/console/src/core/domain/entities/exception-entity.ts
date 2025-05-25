export interface ExceptionEntity {
  id: string
  processId: string
  externalTransactionId?: string
  internalTransactionId?: string
  reason: ExceptionReason
  category: ExceptionCategory
  priority: ExceptionPriority
  status: ExceptionStatus
  assignedTo?: string
  assignedAt?: string
  resolvedBy?: string
  resolvedAt?: string
  resolution?: ExceptionResolution
  investigationNotes: ExceptionNote[]
  suggestedActions: ExceptionSuggestedAction[]
  relatedExceptions?: string[]
  escalationLevel: number
  escalationHistory?: ExceptionEscalation[]
  metadata: Record<string, any>
  createdAt: string
  updatedAt: string
}

export type ExceptionReason =
  | 'no_match_found'
  | 'multiple_matches'
  | 'low_confidence'
  | 'amount_mismatch'
  | 'date_mismatch'
  | 'duplicate_transaction'
  | 'validation_failed'
  | 'manual_review_required'
  | 'data_quality_issue'
  | 'system_error'

export type ExceptionCategory =
  | 'unmatched'
  | 'ambiguous'
  | 'discrepancy'
  | 'duplicate'
  | 'validation'
  | 'technical'
  | 'business_rule'

export type ExceptionPriority = 'low' | 'medium' | 'high' | 'critical'

export type ExceptionStatus =
  | 'pending'
  | 'assigned'
  | 'investigating'
  | 'resolved'
  | 'escalated'
  | 'closed'
  | 'reopened'

export interface ExceptionNote {
  id: string
  timestamp: string
  author: string
  note: string
  type: ExceptionNoteType
  attachments?: string[]
}

export type ExceptionNoteType =
  | 'investigation'
  | 'finding'
  | 'resolution'
  | 'escalation'
  | 'system'

export interface ExceptionSuggestedAction {
  action: ExceptionActionType
  confidence: number
  description: string
  candidateTransactionId?: string
  adjustmentAmount?: number
  reasons: string[]
  aiGenerated: boolean
}

export type ExceptionActionType =
  | 'manual_match'
  | 'create_adjustment'
  | 'investigate'
  | 'escalate'
  | 'ignore'
  | 'split_transaction'
  | 'merge_transactions'

export interface ExceptionResolution {
  type: ExceptionResolutionType
  description: string
  matchedTransactionId?: string
  adjustmentId?: string
  adjustmentAmount?: number
  evidence?: ExceptionEvidence[]
  approvalRequired: boolean
  approvedBy?: string
  approvedAt?: string
}

export type ExceptionResolutionType =
  | 'matched'
  | 'adjusted'
  | 'written_off'
  | 'transferred'
  | 'ignored'
  | 'pending_information'

export interface ExceptionEvidence {
  type: 'document' | 'screenshot' | 'data' | 'calculation'
  description: string
  source: string
  timestamp: string
  metadata?: Record<string, any>
}

export interface ExceptionEscalation {
  level: number
  escalatedBy: string
  escalatedTo: string
  escalatedAt: string
  reason: string
  deadline?: string
}

export interface CreateExceptionRequest {
  processId: string
  externalTransactionId?: string
  internalTransactionId?: string
  reason: ExceptionReason
  category: ExceptionCategory
  priority: ExceptionPriority
  description?: string
  metadata?: Record<string, any>
}

export interface UpdateExceptionRequest {
  status?: ExceptionStatus
  priority?: ExceptionPriority
  assignedTo?: string
  notes?: string
}

export interface ResolveExceptionRequest {
  resolutionType: ExceptionResolutionType
  description: string
  matchedTransactionId?: string
  adjustmentAmount?: number
  evidence?: ExceptionEvidence[]
  resolvedBy: string
}

export interface BulkExceptionOperationRequest {
  exceptionIds: string[]
  operation: BulkExceptionOperation
  assignedTo?: string
  priority?: ExceptionPriority
  notes?: string
  operatedBy: string
}

export type BulkExceptionOperation =
  | 'assign'
  | 'escalate'
  | 'change_priority'
  | 'add_note'
  | 'resolve_bulk'

export interface ExceptionFilters {
  processId?: string
  reason?: ExceptionReason[]
  category?: ExceptionCategory[]
  priority?: ExceptionPriority[]
  status?: ExceptionStatus[]
  assignedTo?: string
  escalationLevel?: number[]
  dateRange?: {
    start: string
    end: string
  }
  hasResolution?: boolean
  searchText?: string
}

export interface ExceptionWorkflowState {
  exceptionId: string
  currentStep: ExceptionWorkflowStep
  availableActions: ExceptionWorkflowAction[]
  history: ExceptionWorkflowHistory[]
  deadline?: string
  blockers?: string[]
}

export type ExceptionWorkflowStep =
  | 'initial_review'
  | 'investigation'
  | 'resolution_proposal'
  | 'approval_pending'
  | 'implementation'
  | 'verification'
  | 'closed'

export interface ExceptionWorkflowAction {
  action: string
  label: string
  description: string
  requiresApproval: boolean
  estimatedTime?: number
}

export interface ExceptionWorkflowHistory {
  step: ExceptionWorkflowStep
  action: string
  timestamp: string
  user: string
  notes?: string
  duration?: number
}

export interface ExceptionAnalytics {
  reasonDistribution: {
    reason: ExceptionReason
    count: number
    percentage: number
    averageResolutionTime: number
  }[]
  categoryDistribution: {
    category: ExceptionCategory
    count: number
    percentage: number
  }[]
  priorityDistribution: {
    priority: ExceptionPriority
    count: number
    percentage: number
    averageResolutionTime: number
  }[]
  resolutionMetrics: {
    averageResolutionTime: number
    resolutionRate: number
    escalationRate: number
    reopenRate: number
    straightThroughResolution: number
  }
  assigneePerformance: {
    assignee: string
    activeExceptions: number
    resolvedExceptions: number
    averageResolutionTime: number
    qualityScore: number
  }[]
  trends: {
    date: string
    created: number
    resolved: number
    backlog: number
    averageAge: number
  }[]
}

export interface ExceptionSummary {
  totalExceptions: number
  pendingExceptions: number
  resolvedExceptions: number
  criticalExceptions: number
  averageAge: number
  oldestException: number
  resolutionRate: number
  escalationRate: number
  backlogTrend: 'increasing' | 'decreasing' | 'stable'
}
