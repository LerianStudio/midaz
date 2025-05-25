export interface MatchEntity {
  id: string
  processId: string
  externalTransactionId: string
  internalTransactionIds: string[]
  matchType: MatchType
  confidenceScore: number
  status: MatchStatus
  ruleId?: string
  matchedFields: MatchedFields
  similarities?: MatchSimilarities
  reviewedBy?: string
  reviewedAt?: string
  reviewNotes?: string
  aiInsights?: MatchAiInsights
  validationResults?: MatchValidationResult[]
  createdAt: string
  updatedAt: string
}

export type MatchType =
  | 'exact'
  | 'fuzzy'
  | 'ai_semantic'
  | 'manual'
  | 'rule_based'
  | 'partial'

export type MatchStatus =
  | 'pending'
  | 'confirmed'
  | 'rejected'
  | 'under_review'
  | 'auto_approved'
  | 'escalated'

export interface MatchedFields {
  amount?: boolean | number
  date?: boolean | number
  description?: boolean | number
  reference_number?: boolean | number
  account_number?: boolean | number
  account_name?: boolean | number
  similarity_score?: number
  embedding_model?: string
  matched_features?: string[]
  custom_fields?: Record<string, boolean | number>
}

export interface MatchSimilarities {
  overall: number
  amount: number
  date: number
  description: number
  reference: number
  semantic: number
  structural: number
}

export interface MatchAiInsights {
  description_similarity: number
  amount_similarity: number
  temporal_proximity: number
  pattern_confidence: number
  suggested_review_priority: MatchReviewPriority
  explanation?: string
  alternative_candidates?: MatchCandidate[]
  confidence_factors: MatchConfidenceFactor[]
}

export type MatchReviewPriority = 'low' | 'medium' | 'high' | 'critical'

export interface MatchCandidate {
  transactionId: string
  confidenceScore: number
  reasons: string[]
  similarities: MatchSimilarities
}

export interface MatchConfidenceFactor {
  factor: string
  impact: number
  description: string
  weight: number
}

export interface MatchValidationResult {
  validation: string
  passed: boolean
  score?: number
  message?: string
  warnings?: string[]
}

export interface CreateMatchRequest {
  processId: string
  externalTransactionId: string
  internalTransactionIds: string[]
  matchType: MatchType
  confidenceScore: number
  ruleId?: string
  matchedFields: MatchedFields
  similarities?: MatchSimilarities
  aiInsights?: MatchAiInsights
}

export interface UpdateMatchRequest {
  status?: MatchStatus
  reviewNotes?: string
  confidenceScore?: number
  matchedFields?: Partial<MatchedFields>
  reviewedBy?: string
}

export interface BulkMatchOperationRequest {
  matchIds: string[]
  operation: BulkMatchOperation
  reviewNotes?: string
  reviewedBy: string
}

export type BulkMatchOperation =
  | 'approve'
  | 'reject'
  | 'escalate'
  | 'assign_reviewer'

export interface MatchFilters {
  processId?: string
  externalTransactionId?: string
  matchType?: MatchType[]
  status?: MatchStatus[]
  confidenceRange?: {
    min: number
    max: number
  }
  reviewPriority?: MatchReviewPriority[]
  reviewedBy?: string
  dateRange?: {
    start: string
    end: string
  }
  hasAiInsights?: boolean
  ruleId?: string
  searchText?: string
}

export interface MatchAnalytics {
  confidenceDistribution: {
    range: string
    count: number
    percentage: number
    averageReviewTime?: number
  }[]
  typeDistribution: {
    type: MatchType
    count: number
    percentage: number
    averageConfidence: number
  }[]
  statusDistribution: {
    status: MatchStatus
    count: number
    percentage: number
  }[]
  reviewerPerformance: {
    reviewer: string
    matchesReviewed: number
    averageReviewTime: number
    accuracyScore: number
  }[]
  qualityMetrics: {
    straightThroughProcessing: number
    falsePositiveRate: number
    falseNegativeRate: number
    averageConfidence: number
    aiAccuracy: number
  }
  trends: {
    date: string
    totalMatches: number
    approvedMatches: number
    rejectedMatches: number
    averageConfidence: number
  }[]
}

export interface MatchSummary {
  totalMatches: number
  pendingReview: number
  approved: number
  rejected: number
  averageConfidence: number
  highConfidenceMatches: number
  lowConfidenceMatches: number
  aiMatchCount: number
  manualMatchCount: number
  reviewBacklog: number
}
