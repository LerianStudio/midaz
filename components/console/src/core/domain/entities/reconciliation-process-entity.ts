export interface ReconciliationProcessEntity {
  id: string
  ledgerId: string
  organizationId: string
  importId?: string
  name: string
  description?: string
  status: ReconciliationProcessStatus
  progress: ReconciliationProgress
  configuration: ReconciliationConfiguration
  summary?: ReconciliationSummary
  metrics?: ReconciliationMetrics
  startedAt?: string
  completedAt?: string
  estimatedCompletionAt?: string
  createdAt: string
  updatedAt: string
  createdBy: string
  lastModifiedBy?: string
}

export type ReconciliationProcessStatus =
  | 'draft'
  | 'queued'
  | 'preparing'
  | 'processing'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'paused'

export interface ReconciliationProgress {
  totalTransactions: number
  processedTransactions: number
  matchedTransactions: number
  exceptionCount: number
  progressPercentage: number
  currentPhase: ReconciliationPhase
  phaseProgress: number
  estimatedTimeRemaining?: number
  throughput?: number
}

export type ReconciliationPhase =
  | 'initialization'
  | 'preprocessing'
  | 'rule_matching'
  | 'ai_matching'
  | 'validation'
  | 'finalization'

export interface ReconciliationConfiguration {
  enableAiMatching: boolean
  minConfidenceScore: number
  maxCandidates: number
  parallelWorkers: number
  batchSize: number
  timeWindow?: {
    start: string
    end: string
  }
  amountTolerance: number
  dateTolerance: number
  rules: ReconciliationRuleConfiguration[]
  aiSettings?: ReconciliationAiSettings
  notifications?: ReconciliationNotificationSettings
}

export interface ReconciliationRuleConfiguration {
  ruleId: string
  priority: number
  isEnabled: boolean
  configuration?: Record<string, any>
}

export interface ReconciliationAiSettings {
  embeddingModel: string
  similarityThreshold: number
  maxSimilaritySearch: number
  useSemanticMatching: boolean
  weightings: {
    amount: number
    date: number
    description: number
    reference: number
  }
}

export interface ReconciliationNotificationSettings {
  enableProgressUpdates: boolean
  enableCompletionNotification: boolean
  enableExceptionAlerts: boolean
  notificationChannels: ('email' | 'webhook' | 'ui')[]
  webhookUrl?: string
  emailRecipients?: string[]
}

export interface ReconciliationSummary {
  matchTypes: Record<MatchType, number>
  averageConfidence: number
  processingTime: string
  throughput: string
  exceptionBreakdown: Record<ExceptionReason, number>
  qualityMetrics: {
    accuracyScore: number
    precisionScore: number
    recallScore: number
    f1Score: number
  }
}

export type MatchType =
  | 'exact'
  | 'fuzzy'
  | 'ai_semantic'
  | 'manual'
  | 'rule_based'

export type ExceptionReason =
  | 'no_match_found'
  | 'multiple_matches'
  | 'low_confidence'
  | 'amount_mismatch'
  | 'date_mismatch'
  | 'validation_failed'
  | 'manual_review_required'

export interface ReconciliationMetrics {
  performance: {
    totalExecutionTime: number
    averageTransactionTime: number
    peakThroughput: number
    resourceUtilization: number
  }
  quality: {
    straightThroughProcessing: number
    manualReviewRate: number
    falsePositiveRate: number
    falseNegativeRate: number
  }
  business: {
    totalValueReconciled: number
    exceptionValue: number
    timeSavings: number
    costPerTransaction: number
  }
}

export interface CreateReconciliationProcessRequest {
  ledgerId: string
  organizationId: string
  importId?: string
  name: string
  description?: string
  configuration: ReconciliationConfiguration
}

export interface UpdateReconciliationProcessRequest {
  name?: string
  description?: string
  configuration?: Partial<ReconciliationConfiguration>
}

export interface ReconciliationProcessFilters {
  status?: ReconciliationProcessStatus[]
  dateRange?: {
    start: string
    end: string
  }
  importId?: string
  createdBy?: string
  searchText?: string
}

export interface ReconciliationProcessAnalytics {
  executionTrends: {
    date: string
    processCount: number
    successRate: number
    averageExecutionTime: number
  }[]
  statusDistribution: {
    status: ReconciliationProcessStatus
    count: number
    percentage: number
  }[]
  performanceMetrics: {
    averageProcessingTime: number
    throughputTrend: {
      date: string
      throughput: number
    }[]
    qualityTrend: {
      date: string
      accuracyScore: number
      stp: number
    }[]
  }
}
