export interface ReconciliationAdjustmentEntity {
  id: string
  processId: string
  exceptionId?: string
  adjustmentType: AdjustmentType
  amount: number
  currency: string
  reason: string
  description?: string
  status: AdjustmentStatus
  approval: AdjustmentApproval
  sourceTransactionId?: string
  targetTransactionId?: string
  accountId?: string
  adjustmentDate: string
  effectiveDate: string
  evidence: AdjustmentEvidence[]
  impactAnalysis?: AdjustmentImpactAnalysis
  reversalId?: string
  metadata: Record<string, any>
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type AdjustmentType =
  | 'timing_difference'
  | 'amount_variance'
  | 'fee_adjustment'
  | 'currency_adjustment'
  | 'rounding_difference'
  | 'write_off'
  | 'reclassification'
  | 'correction'
  | 'reversal'

export type AdjustmentStatus =
  | 'draft'
  | 'pending_approval'
  | 'approved'
  | 'rejected'
  | 'implemented'
  | 'reversed'
  | 'cancelled'

export interface AdjustmentApproval {
  required: boolean
  level: number
  approvers: AdjustmentApprover[]
  currentApprover?: string
  approvalChain: AdjustmentApprovalStep[]
  deadline?: string
  escalation?: AdjustmentEscalation
}

export interface AdjustmentApprover {
  userId: string
  name: string
  role: string
  level: number
  delegatedTo?: string
}

export interface AdjustmentApprovalStep {
  level: number
  approver: string
  status: 'pending' | 'approved' | 'rejected' | 'delegated'
  timestamp?: string
  comments?: string
  conditions?: string[]
}

export interface AdjustmentEscalation {
  triggered: boolean
  level: number
  escalatedTo: string
  escalatedAt: string
  reason: string
}

export interface AdjustmentEvidence {
  type: AdjustmentEvidenceType
  description: string
  source: string
  document?: string
  calculation?: AdjustmentCalculation
  timestamp: string
  verifiedBy?: string
}

export type AdjustmentEvidenceType =
  | 'bank_statement'
  | 'receipt'
  | 'email_confirmation'
  | 'system_report'
  | 'calculation'
  | 'screenshot'
  | 'audit_trail'
  | 'third_party_confirmation'

export interface AdjustmentCalculation {
  formula: string
  inputs: Record<string, number>
  result: number
  explanation: string
  verificationSteps: string[]
}

export interface AdjustmentImpactAnalysis {
  accountImpacts: AccountImpact[]
  financialImpact: FinancialImpact
  complianceImpact?: ComplianceImpact
  businessImpact?: BusinessImpact
  riskAssessment: RiskAssessment
}

export interface AccountImpact {
  accountId: string
  accountName: string
  currentBalance: number
  adjustmentAmount: number
  newBalance: number
  impact: 'positive' | 'negative' | 'neutral'
}

export interface FinancialImpact {
  totalAdjustmentAmount: number
  currencyImpact: Record<string, number>
  netEffect: number
  percentageOfTotal: number
  materiality: 'immaterial' | 'material' | 'significant'
}

export interface ComplianceImpact {
  regulations: string[]
  requiresReporting: boolean
  reportingDeadline?: string
  complianceScore: number
  risks: string[]
}

export interface BusinessImpact {
  customerAffected: boolean
  serviceImpact: 'none' | 'minimal' | 'moderate' | 'significant'
  reputationRisk: 'low' | 'medium' | 'high'
  operationalChanges: string[]
}

export interface RiskAssessment {
  overallRisk: 'low' | 'medium' | 'high' | 'critical'
  riskFactors: RiskFactor[]
  mitigationSteps: string[]
  reviewRequired: boolean
}

export interface RiskFactor {
  factor: string
  probability: number
  impact: number
  score: number
  description: string
}

export interface CreateAdjustmentRequest {
  processId: string
  exceptionId?: string
  adjustmentType: AdjustmentType
  amount: number
  currency: string
  reason: string
  description?: string
  sourceTransactionId?: string
  targetTransactionId?: string
  accountId?: string
  effectiveDate: string
  evidence: Omit<AdjustmentEvidence, 'timestamp'>[]
  metadata?: Record<string, any>
}

export interface UpdateAdjustmentRequest {
  reason?: string
  description?: string
  amount?: number
  effectiveDate?: string
  evidence?: Omit<AdjustmentEvidence, 'timestamp'>[]
  metadata?: Record<string, any>
}

export interface ApproveAdjustmentRequest {
  approver: string
  comments?: string
  conditions?: string[]
  delegateTo?: string
}

export interface RejectAdjustmentRequest {
  approver: string
  reason: string
  comments?: string
  suggestedChanges?: string[]
}

export interface BulkAdjustmentOperationRequest {
  adjustmentIds: string[]
  operation: BulkAdjustmentOperation
  approver?: string
  comments?: string
  operatedBy: string
}

export type BulkAdjustmentOperation =
  | 'approve'
  | 'reject'
  | 'cancel'
  | 'escalate'
  | 'change_priority'

export interface AdjustmentFilters {
  processId?: string
  adjustmentType?: AdjustmentType[]
  status?: AdjustmentStatus[]
  amountRange?: {
    min: number
    max: number
  }
  dateRange?: {
    start: string
    end: string
  }
  currency?: string[]
  createdBy?: string
  approver?: string
  accountId?: string
  searchText?: string
  materialityLevel?: ('immaterial' | 'material' | 'significant')[]
}

export interface AdjustmentWorkflow {
  adjustmentId: string
  currentStep: AdjustmentWorkflowStep
  nextSteps: AdjustmentWorkflowAction[]
  history: AdjustmentWorkflowHistory[]
  blockers?: string[]
  estimatedCompletion?: string
}

export type AdjustmentWorkflowStep =
  | 'draft'
  | 'evidence_collection'
  | 'impact_analysis'
  | 'approval_pending'
  | 'implementation_pending'
  | 'verification'
  | 'completed'

export interface AdjustmentWorkflowAction {
  action: string
  label: string
  description: string
  requiredRole: string[]
  estimatedTime?: number
  prerequisites?: string[]
}

export interface AdjustmentWorkflowHistory {
  step: AdjustmentWorkflowStep
  action: string
  timestamp: string
  user: string
  comments?: string
  duration?: number
  automated: boolean
}

export interface AdjustmentAnalytics {
  typeDistribution: {
    type: AdjustmentType
    count: number
    totalAmount: number
    percentage: number
    averageApprovalTime: number
  }[]
  statusDistribution: {
    status: AdjustmentStatus
    count: number
    percentage: number
  }[]
  approvalMetrics: {
    averageApprovalTime: number
    approvalRate: number
    rejectionRate: number
    escalationRate: number
    levelBreakdown: {
      level: number
      averageTime: number
      approvalRate: number
    }[]
  }
  riskDistribution: {
    risk: string
    count: number
    totalValue: number
    percentage: number
  }[]
  trends: {
    date: string
    totalAdjustments: number
    totalValue: number
    approvedValue: number
    averageAmount: number
  }[]
  impactSummary: {
    totalFinancialImpact: number
    affectedAccounts: number
    materialAdjustments: number
    complianceIssues: number
  }
}

export interface AdjustmentSummary {
  totalAdjustments: number
  pendingApproval: number
  approved: number
  rejected: number
  totalValue: number
  averageAdjustmentAmount: number
  highRiskAdjustments: number
  overdueApprovals: number
  recentActivity: ReconciliationAdjustmentEntity[]
}
