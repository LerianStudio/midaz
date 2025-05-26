// Comprehensive unified mock data for Reconciliation plugin
// This file consolidates all reconciliation-related mock data structures

export interface ExternalTransaction {
  id: string
  importId: string
  externalId: string
  amount: number
  currency: string
  description: string
  date: string
  reference: string
  accountNumber?: string
  metadata: Record<string, any>
  status: 'pending' | 'matched' | 'exception'
  createdAt: string
  updatedAt: string
}

export interface InternalTransaction {
  id: string
  amount: number
  currency: string
  description: string
  date: string
  reference: string
  accountId: string
  ledgerId: string
  operationType: 'debit' | 'credit'
  status: 'completed' | 'pending' | 'failed'
  metadata: Record<string, any>
  createdAt: string
  updatedAt: string
}

export interface ReconciliationImport {
  id: string
  ledgerId: string
  organizationId: string
  fileName: string
  filePath: string
  fileSize: number
  fileType: 'csv' | 'json' | 'xlsx'
  status: 'uploading' | 'validating' | 'processing' | 'completed' | 'failed'
  totalRecords: number
  processedRecords: number
  failedRecords: number
  validationErrors: ValidationError[]
  metadata: {
    encoding?: string
    delimiter?: string
    hasHeader?: boolean
    sourceSystem?: string
  }
  startedAt?: string
  completedAt?: string
  createdAt: string
  updatedAt: string
}

export interface ValidationError {
  line: number
  field: string
  value: any
  error: string
  severity: 'error' | 'warning'
}

export interface ReconciliationProcess {
  id: string
  ledgerId: string
  organizationId: string
  importId: string
  name: string
  description?: string
  status: 'queued' | 'processing' | 'completed' | 'failed' | 'cancelled'
  progress: {
    totalTransactions: number
    processedTransactions: number
    matchedTransactions: number
    exceptionCount: number
    progressPercentage: number
    currentStage: string
    estimatedCompletion?: string
  }
  configuration: {
    enableAiMatching: boolean
    minConfidenceScore: number
    maxCandidates: number
    parallelWorkers: number
    batchSize: number
    dateToleranceDays: number
    amountTolerancePercent: number
    enableFuzzyMatching: boolean
  }
  summary?: {
    matchTypes: {
      exact: number
      fuzzy: number
      ai_semantic: number
      manual: number
    }
    averageConfidence: number
    processingTime: string
    throughput: string
    topMatchingRules: Array<{
      ruleId: string
      ruleName: string
      matchCount: number
    }>
  }
  startedAt?: string
  completedAt?: string
  createdAt: string
  updatedAt: string
}

export interface ReconciliationMatch {
  id: string
  processId: string
  externalTransactionId: string
  internalTransactionIds: string[]
  matchType: 'exact' | 'fuzzy' | 'ai_semantic' | 'manual' | 'rule_based'
  confidenceScore: number
  ruleId?: string
  matchedFields: Record<string, boolean | number>
  status: 'pending' | 'confirmed' | 'rejected' | 'under_review'
  aiInsights?: {
    description_similarity?: number
    amount_similarity?: number
    temporal_proximity?: number
    suggested_review_priority?: 'low' | 'medium' | 'high'
    explanation?: string
  }
  reviewedBy?: string
  reviewedAt?: string
  reviewNotes?: string
  createdAt: string
  updatedAt: string
}

export interface ReconciliationException {
  id: string
  processId: string
  externalTransactionId?: string
  internalTransactionId?: string
  reason: string
  category:
    | 'unmatched'
    | 'duplicate'
    | 'amount_mismatch'
    | 'date_mismatch'
    | 'validation_error'
    | 'system_error'
  priority: 'low' | 'medium' | 'high' | 'critical'
  status: 'pending' | 'assigned' | 'investigating' | 'resolved' | 'escalated'
  assignedTo?: string
  resolutionType?:
    | 'manual_match'
    | 'adjustment'
    | 'ignore'
    | 'investigate'
    | 'escalate'
  resolutionDetails?: Record<string, any>
  suggestedActions: Array<{
    action: string
    confidence: number
    description: string
    candidateTransactionId?: string
    metadata?: Record<string, any>
  }>
  investigationNotes: Array<{
    id: string
    timestamp: string
    author: string
    note: string
    attachments?: string[]
  }>
  escalationLevel: number
  dueDate?: string
  resolvedAt?: string
  resolvedBy?: string
  createdAt: string
  updatedAt: string
}

export interface ReconciliationRule {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  description: string
  ruleType: 'amount' | 'date' | 'string' | 'regex' | 'metadata' | 'composite'
  criteria: {
    field: string
    operator:
      | 'equals'
      | 'contains'
      | 'starts_with'
      | 'ends_with'
      | 'regex'
      | 'fuzzy_match'
      | 'range'
    value?: any
    tolerance?: number
    similarity_threshold?: number
    case_sensitive?: boolean
    date_tolerance_days?: number
    amount_tolerance_percent?: number
    additionalFields?: string[]
  }
  priority: number
  isActive: boolean
  performance: {
    matchCount: number
    successRate: number
    averageConfidence: number
    executionTime: string
    lastExecuted?: string
  }
  createdBy: string
  updatedBy?: string
  createdAt: string
  updatedAt: string
}

export interface ReconciliationSource {
  id: string
  name: string
  type: 'database' | 'api' | 'file' | 'webhook'
  description: string
  status: 'connected' | 'disconnected' | 'error' | 'maintenance'
  configuration: {
    connectionString?: string
    apiEndpoint?: string
    authMethod?: string
    refreshRate?: number
    batchSize?: number
  }
  healthMetrics: {
    uptime: number
    lastSync: string
    errorCount: number
    averageResponseTime: string
    recordCount: number
  }
  mapping: {
    fields: Record<string, string>
    transformations?: Array<{
      field: string
      transformation: string
      parameters?: Record<string, any>
    }>
  }
  createdAt: string
  updatedAt: string
}

export interface ReconciliationChain {
  id: string
  name: string
  description: string
  sources: string[]
  workflow: Array<{
    stepId: string
    stepType: 'import' | 'reconcile' | 'validate' | 'notify'
    sourceId?: string
    configuration: Record<string, any>
    dependencies?: string[]
  }>
  schedule?: {
    frequency: 'manual' | 'hourly' | 'daily' | 'weekly' | 'monthly'
    time?: string
    timezone?: string
  }
  status: 'active' | 'inactive' | 'error'
  lastExecuted?: string
  nextExecution?: string
  performance: {
    successRate: number
    averageExecutionTime: string
    totalExecutions: number
  }
  createdAt: string
  updatedAt: string
}

export interface ReconciliationAnalytics {
  period: {
    start: string
    end: string
  }
  overview: {
    totalTransactions: number
    matchedTransactions: number
    exceptionsCount: number
    matchRate: number
    averageProcessingTime: string
    throughput: number
  }
  trends: {
    daily: Array<{
      date: string
      transactions: number
      matches: number
      exceptions: number
      matchRate: number
    }>
    weekly: Array<{
      week: string
      transactions: number
      matches: number
      exceptions: number
      matchRate: number
    }>
  }
  performance: {
    ruleEffectiveness: Array<{
      ruleId: string
      ruleName: string
      matchCount: number
      successRate: number
      averageConfidence: number
    }>
    aiPerformance: {
      totalAiMatches: number
      averageConfidence: number
      confidenceDistribution: Record<string, number>
      modelAccuracy: number
    }
    processingSpeed: {
      averageTransactionsPerMinute: number
      peakThroughput: number
      slowestStep: string
    }
  }
  exceptions: {
    categoryBreakdown: Record<string, number>
    priorityDistribution: Record<string, number>
    resolutionTimes: {
      average: string
      median: string
      percentile95: string
    }
    escalationRate: number
  }
}

// Mock Data Generation
export const mockExternalTransactions: ExternalTransaction[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2322',
    importId: '01956b69-9102-75b7-8860-3e75c11d231c',
    externalId: 'BNK_TXN_2024120115001',
    amount: 1250.75,
    currency: 'USD',
    description: 'Wire Transfer - Acme Corp Payment',
    date: '2024-12-01T15:00:00Z',
    reference: 'ACM240001',
    accountNumber: '1234567890',
    metadata: {
      sourceSystem: 'core_banking',
      batchId: 'BATCH_20241201_01',
      channel: 'wire_transfer'
    },
    status: 'matched',
    createdAt: '2024-12-01T15:01:00Z',
    updatedAt: '2024-12-01T15:05:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2326',
    importId: '01956b69-9102-75b7-8860-3e75c11d231c',
    externalId: 'BNK_TXN_2024120115002',
    amount: 750.0,
    currency: 'USD',
    description: 'Electronic Payment - Vendor Services',
    date: '2024-12-01T14:30:00Z',
    reference: 'VND240002',
    accountNumber: '1234567890',
    metadata: {
      sourceSystem: 'core_banking',
      batchId: 'BATCH_20241201_01',
      channel: 'ach'
    },
    status: 'pending',
    createdAt: '2024-12-01T14:31:00Z',
    updatedAt: '2024-12-01T14:31:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2329',
    importId: '01956b69-9102-75b7-8860-3e75c11d231c',
    externalId: 'BNK_TXN_2024120115003',
    amount: 2500.0,
    currency: 'USD',
    description: 'Customer Deposit - Business Account',
    date: '2024-12-01T13:15:00Z',
    reference: 'DEP240003',
    accountNumber: '1234567890',
    metadata: {
      sourceSystem: 'core_banking',
      batchId: 'BATCH_20241201_01',
      channel: 'deposit'
    },
    status: 'exception',
    createdAt: '2024-12-01T13:16:00Z',
    updatedAt: '2024-12-01T16:00:00Z'
  }
]

export const mockInternalTransactions: InternalTransaction[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2323',
    amount: 1250.75,
    currency: 'USD',
    description: 'Acme Corp - Wire Payment Received',
    date: '2024-12-01T15:02:00Z',
    reference: 'ACM240001',
    accountId: '01956b69-9102-75b7-8860-acc-001',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    operationType: 'credit',
    status: 'completed',
    metadata: {
      transactionId: 'TXN_240001',
      sourceAccount: 'external_wire',
      processingTime: '2.3s'
    },
    createdAt: '2024-12-01T15:02:00Z',
    updatedAt: '2024-12-01T15:02:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2327',
    amount: 745.5,
    currency: 'USD',
    description: 'Vendor Services - ACH Payment',
    date: '2024-12-01T14:32:00Z',
    reference: 'VND240002X',
    accountId: '01956b69-9102-75b7-8860-acc-002',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    operationType: 'debit',
    status: 'completed',
    metadata: {
      transactionId: 'TXN_240002',
      sourceAccount: 'business_checking',
      processingTime: '1.8s'
    },
    createdAt: '2024-12-01T14:32:00Z',
    updatedAt: '2024-12-01T14:32:00Z'
  }
]

export const mockReconciliationImports: ReconciliationImport[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231c',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    fileName: 'bank_transactions_2024_12.csv',
    filePath: '/uploads/reconciliation/bank_transactions_2024_12.csv',
    fileSize: 1048576,
    fileType: 'csv',
    status: 'completed',
    totalRecords: 2500,
    processedRecords: 2500,
    failedRecords: 0,
    validationErrors: [],
    metadata: {
      encoding: 'UTF-8',
      delimiter: ',',
      hasHeader: true,
      sourceSystem: 'core_banking_system'
    },
    startedAt: '2025-01-01T10:00:00Z',
    completedAt: '2025-01-01T10:05:30Z',
    createdAt: '2025-01-01T09:59:45Z',
    updatedAt: '2025-01-01T10:05:30Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    fileName: 'payment_processor_data.json',
    filePath: '/uploads/reconciliation/payment_processor_data.json',
    fileSize: 2097152,
    fileType: 'json',
    status: 'processing',
    totalRecords: 5000,
    processedRecords: 3200,
    failedRecords: 12,
    validationErrors: [
      {
        line: 1245,
        field: 'amount',
        value: 'invalid_decimal',
        error: 'Invalid decimal format',
        severity: 'error'
      },
      {
        line: 2890,
        field: 'date',
        value: '2024-13-01',
        error: 'Invalid date format',
        severity: 'error'
      }
    ],
    metadata: {
      encoding: 'UTF-8',
      sourceSystem: 'payment_processor_api'
    },
    startedAt: '2025-01-01T11:30:00Z',
    createdAt: '2025-01-01T11:29:30Z',
    updatedAt: '2025-01-01T11:35:15Z'
  }
]

export const mockReconciliationProcesses: ReconciliationProcess[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2320',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    importId: '01956b69-9102-75b7-8860-3e75c11d231c',
    name: 'Bank Statement Reconciliation - December 2024',
    description: 'Monthly reconciliation of bank statement transactions',
    status: 'completed',
    progress: {
      totalTransactions: 2500,
      processedTransactions: 2500,
      matchedTransactions: 2387,
      exceptionCount: 113,
      progressPercentage: 100,
      currentStage: 'completed',
      estimatedCompletion: '2025-01-01T10:10:32Z'
    },
    configuration: {
      enableAiMatching: true,
      minConfidenceScore: 0.8,
      maxCandidates: 100,
      parallelWorkers: 10,
      batchSize: 100,
      dateToleranceDays: 2,
      amountTolerancePercent: 0.01,
      enableFuzzyMatching: true
    },
    summary: {
      matchTypes: {
        exact: 1856,
        fuzzy: 398,
        ai_semantic: 133,
        manual: 0
      },
      averageConfidence: 0.923,
      processingTime: '00:04:32',
      throughput: '551 transactions/minute',
      topMatchingRules: [
        {
          ruleId: '01956b69-9102-75b7-8860-3e75c11d232b',
          ruleName: 'Exact Amount and Reference Match',
          matchCount: 1856
        },
        {
          ruleId: '01956b69-9102-75b7-8860-3e75c11d232c',
          ruleName: 'Fuzzy Description Match',
          matchCount: 398
        }
      ]
    },
    startedAt: '2025-01-01T10:06:00Z',
    completedAt: '2025-01-01T10:10:32Z',
    createdAt: '2025-01-01T10:05:45Z',
    updatedAt: '2025-01-01T10:10:32Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2330',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    importId: '01956b69-9102-75b7-8860-3e75c11d231f',
    name: 'Payment Processor Reconciliation - In Progress',
    description: 'Real-time reconciliation of payment processor transactions',
    status: 'processing',
    progress: {
      totalTransactions: 5000,
      processedTransactions: 3200,
      matchedTransactions: 2985,
      exceptionCount: 215,
      progressPercentage: 64,
      currentStage: 'ai_matching',
      estimatedCompletion: '2025-01-01T12:15:00Z'
    },
    configuration: {
      enableAiMatching: true,
      minConfidenceScore: 0.75,
      maxCandidates: 50,
      parallelWorkers: 8,
      batchSize: 200,
      dateToleranceDays: 1,
      amountTolerancePercent: 0.005,
      enableFuzzyMatching: true
    },
    startedAt: '2025-01-01T11:40:00Z',
    createdAt: '2025-01-01T11:35:00Z',
    updatedAt: '2025-01-01T11:50:00Z'
  }
]

export const mockReconciliationMatches: ReconciliationMatch[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2321',
    processId: '01956b69-9102-75b7-8860-3e75c11d2320',
    externalTransactionId: '01956b69-9102-75b7-8860-3e75c11d2322',
    internalTransactionIds: ['01956b69-9102-75b7-8860-3e75c11d2323'],
    matchType: 'exact',
    confidenceScore: 1.0,
    ruleId: '01956b69-9102-75b7-8860-3e75c11d232b',
    matchedFields: {
      amount: true,
      date: true,
      reference_number: true,
      account_number: true
    },
    status: 'confirmed',
    reviewedBy: 'analyst@company.com',
    reviewedAt: '2025-01-01T10:15:00Z',
    reviewNotes: 'Perfect match on all key fields',
    createdAt: '2025-01-01T10:08:15Z',
    updatedAt: '2025-01-01T10:15:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2325',
    processId: '01956b69-9102-75b7-8860-3e75c11d2320',
    externalTransactionId: '01956b69-9102-75b7-8860-3e75c11d2326',
    internalTransactionIds: ['01956b69-9102-75b7-8860-3e75c11d2327'],
    matchType: 'ai_semantic',
    confidenceScore: 0.87,
    matchedFields: {
      similarity_score: 0.87,
      embedding_model: 'sentence-transformers/all-MiniLM-L6-v2',
      matched_features: ['description', 'amount_pattern', 'date_proximity']
    },
    status: 'pending',
    aiInsights: {
      description_similarity: 0.92,
      amount_similarity: 0.85,
      temporal_proximity: 0.94,
      suggested_review_priority: 'medium',
      explanation:
        'High semantic similarity in description with slight amount variance (0.5%). Date proximity within acceptable range.'
    },
    createdAt: '2025-01-01T10:09:22Z',
    updatedAt: '2025-01-01T10:09:22Z'
  }
]

export const mockReconciliationExceptions: ReconciliationException[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2328',
    processId: '01956b69-9102-75b7-8860-3e75c11d2320',
    externalTransactionId: '01956b69-9102-75b7-8860-3e75c11d2329',
    reason: 'No matching internal transaction found',
    category: 'unmatched',
    priority: 'high',
    status: 'assigned',
    assignedTo: 'analyst@company.com',
    suggestedActions: [
      {
        action: 'manual_match',
        confidence: 0.75,
        description: 'Potential match found with transaction ID 01956b69...',
        candidateTransactionId: '01956b69-9102-75b7-8860-3e75c11d232a',
        metadata: {
          similarity_score: 0.75,
          matched_fields: ['amount', 'date_range']
        }
      },
      {
        action: 'investigate',
        confidence: 0.6,
        description:
          'Pattern suggests possible timing difference or processing delay'
      }
    ],
    investigationNotes: [
      {
        id: 'note-001',
        timestamp: '2025-01-01T11:00:00Z',
        author: 'analyst@company.com',
        note: 'Reviewing transaction details. Amount and date match pattern but no exact reference.'
      },
      {
        id: 'note-002',
        timestamp: '2025-01-01T11:15:00Z',
        author: 'analyst@company.com',
        note: 'Found potential candidate with 2-day delay. Checking with source system for processing lag.'
      }
    ],
    escalationLevel: 1,
    dueDate: '2025-01-02T17:00:00Z',
    createdAt: '2025-01-01T10:08:45Z',
    updatedAt: '2025-01-01T11:15:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2331',
    processId: '01956b69-9102-75b7-8860-3e75c11d2320',
    externalTransactionId: '01956b69-9102-75b7-8860-3e75c11d2332',
    internalTransactionId: '01956b69-9102-75b7-8860-3e75c11d2333',
    reason: 'Amount mismatch: External $1000.00 vs Internal $1005.50',
    category: 'amount_mismatch',
    priority: 'medium',
    status: 'investigating',
    assignedTo: 'senior-analyst@company.com',
    suggestedActions: [
      {
        action: 'adjustment',
        confidence: 0.85,
        description: 'Create adjustment entry for $5.50 fee difference',
        metadata: {
          adjustment_amount: 5.5,
          adjustment_type: 'processing_fee'
        }
      }
    ],
    investigationNotes: [
      {
        id: 'note-003',
        timestamp: '2025-01-01T11:30:00Z',
        author: 'senior-analyst@company.com',
        note: 'Amount variance appears to be standard processing fee. Verifying with fee schedule.'
      }
    ],
    escalationLevel: 0,
    dueDate: '2025-01-03T17:00:00Z',
    createdAt: '2025-01-01T10:12:30Z',
    updatedAt: '2025-01-01T11:30:00Z'
  }
]

export const mockReconciliationRules: ReconciliationRule[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232b',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    name: 'Exact Amount and Reference Match',
    description:
      'Matches transactions with identical amounts and reference numbers',
    ruleType: 'amount',
    criteria: {
      field: 'amount',
      operator: 'equals',
      tolerance: 0.01,
      additionalFields: ['reference_number']
    },
    priority: 1,
    isActive: true,
    performance: {
      matchCount: 1856,
      successRate: 0.985,
      averageConfidence: 0.967,
      executionTime: '12ms',
      lastExecuted: '2025-01-01T10:08:00Z'
    },
    createdBy: 'system@company.com',
    updatedBy: 'admin@company.com',
    createdAt: '2024-11-15T00:00:00Z',
    updatedAt: '2024-12-20T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232c',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    name: 'Fuzzy Description Match',
    description:
      'Matches transactions based on similar descriptions using fuzzy matching',
    ruleType: 'string',
    criteria: {
      field: 'description',
      operator: 'fuzzy_match',
      similarity_threshold: 0.8,
      case_sensitive: false
    },
    priority: 3,
    isActive: true,
    performance: {
      matchCount: 398,
      successRate: 0.892,
      averageConfidence: 0.831,
      executionTime: '45ms',
      lastExecuted: '2025-01-01T10:08:30Z'
    },
    createdBy: 'analyst@company.com',
    updatedBy: 'analyst@company.com',
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-18T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232d',
    ledgerId: '01956b69-9102-75b7-8860-ldg-001',
    organizationId: '01956b69-9102-75b7-8860-org-001',
    name: 'Date Range with Amount Tolerance',
    description: 'Matches transactions within date range and amount tolerance',
    ruleType: 'composite',
    criteria: {
      field: 'date',
      operator: 'range',
      date_tolerance_days: 2,
      amount_tolerance_percent: 0.02,
      additionalFields: ['amount']
    },
    priority: 2,
    isActive: true,
    performance: {
      matchCount: 133,
      successRate: 0.756,
      averageConfidence: 0.723,
      executionTime: '78ms',
      lastExecuted: '2025-01-01T10:09:00Z'
    },
    createdBy: 'admin@company.com',
    createdAt: '2024-12-01T00:00:00Z',
    updatedAt: '2024-12-01T00:00:00Z'
  }
]

export const mockReconciliationSources: ReconciliationSource[] = [
  {
    id: 'source-core-banking',
    name: 'Core Banking System',
    type: 'database',
    description: 'Primary banking system with transaction records',
    status: 'connected',
    configuration: {
      connectionString: 'postgresql://banking-db:5432/core_banking',
      refreshRate: 300,
      batchSize: 1000
    },
    healthMetrics: {
      uptime: 99.8,
      lastSync: '2025-01-01T11:45:00Z',
      errorCount: 2,
      averageResponseTime: '125ms',
      recordCount: 2500000
    },
    mapping: {
      fields: {
        transaction_id: 'external_id',
        amount: 'amount',
        transaction_date: 'date',
        description: 'description',
        reference_number: 'reference'
      }
    },
    createdAt: '2024-10-01T00:00:00Z',
    updatedAt: '2025-01-01T11:45:00Z'
  },
  {
    id: 'source-payment-processor',
    name: 'Payment Processor API',
    type: 'api',
    description: 'External payment processor with real-time transaction feed',
    status: 'connected',
    configuration: {
      apiEndpoint: 'https://api.payment-processor.com/v2/transactions',
      authMethod: 'oauth2',
      refreshRate: 60,
      batchSize: 500
    },
    healthMetrics: {
      uptime: 97.5,
      lastSync: '2025-01-01T11:50:00Z',
      errorCount: 15,
      averageResponseTime: '250ms',
      recordCount: 150000
    },
    mapping: {
      fields: {
        txn_id: 'external_id',
        txn_amount: 'amount',
        txn_timestamp: 'date',
        txn_description: 'description',
        merchant_ref: 'reference'
      },
      transformations: [
        {
          field: 'txn_amount',
          transformation: 'divide',
          parameters: { divisor: 100 }
        }
      ]
    },
    createdAt: '2024-11-01T00:00:00Z',
    updatedAt: '2025-01-01T11:50:00Z'
  }
]

export const mockReconciliationChains: ReconciliationChain[] = [
  {
    id: 'chain-daily-reconciliation',
    name: 'Daily Bank Reconciliation',
    description: 'Automated daily reconciliation of all bank sources',
    sources: ['source-core-banking', 'source-payment-processor'],
    workflow: [
      {
        stepId: 'step-1',
        stepType: 'import',
        sourceId: 'source-core-banking',
        configuration: {
          timeframe: 'last_24_hours',
          filters: { status: 'completed' }
        }
      },
      {
        stepId: 'step-2',
        stepType: 'import',
        sourceId: 'source-payment-processor',
        configuration: {
          timeframe: 'last_24_hours'
        },
        dependencies: ['step-1']
      },
      {
        stepId: 'step-3',
        stepType: 'reconcile',
        configuration: {
          enableAi: true,
          minConfidence: 0.8
        },
        dependencies: ['step-2']
      },
      {
        stepId: 'step-4',
        stepType: 'notify',
        configuration: {
          recipients: ['reconciliation-team@company.com'],
          includeExceptions: true
        },
        dependencies: ['step-3']
      }
    ],
    schedule: {
      frequency: 'daily',
      time: '06:00',
      timezone: 'UTC'
    },
    status: 'active',
    lastExecuted: '2025-01-01T06:00:00Z',
    nextExecution: '2025-01-02T06:00:00Z',
    performance: {
      successRate: 0.96,
      averageExecutionTime: '12m 34s',
      totalExecutions: 90
    },
    createdAt: '2024-10-01T00:00:00Z',
    updatedAt: '2025-01-01T06:15:00Z'
  }
]

export const mockReconciliationAnalytics: ReconciliationAnalytics = {
  period: {
    start: '2024-12-01T00:00:00Z',
    end: '2024-12-31T23:59:59Z'
  },
  overview: {
    totalTransactions: 125000,
    matchedTransactions: 119500,
    exceptionsCount: 5500,
    matchRate: 0.956,
    averageProcessingTime: '2m 15s',
    throughput: 925
  },
  trends: {
    daily: [
      {
        date: '2024-12-01',
        transactions: 4200,
        matches: 4015,
        exceptions: 185,
        matchRate: 0.956
      },
      {
        date: '2024-12-02',
        transactions: 3800,
        matches: 3648,
        exceptions: 152,
        matchRate: 0.96
      }
    ],
    weekly: [
      {
        week: '2024-W48',
        transactions: 28500,
        matches: 27360,
        exceptions: 1140,
        matchRate: 0.96
      },
      {
        week: '2024-W49',
        transactions: 31200,
        matches: 29856,
        exceptions: 1344,
        matchRate: 0.957
      }
    ]
  },
  performance: {
    ruleEffectiveness: [
      {
        ruleId: '01956b69-9102-75b7-8860-3e75c11d232b',
        ruleName: 'Exact Amount and Reference Match',
        matchCount: 85600,
        successRate: 0.985,
        averageConfidence: 0.967
      },
      {
        ruleId: '01956b69-9102-75b7-8860-3e75c11d232c',
        ruleName: 'Fuzzy Description Match',
        matchCount: 22400,
        successRate: 0.892,
        averageConfidence: 0.831
      }
    ],
    aiPerformance: {
      totalAiMatches: 11500,
      averageConfidence: 0.863,
      confidenceDistribution: {
        '0.9-1.0': 4200,
        '0.8-0.9': 5100,
        '0.7-0.8': 1800,
        '0.6-0.7': 400
      },
      modelAccuracy: 0.924
    },
    processingSpeed: {
      averageTransactionsPerMinute: 925,
      peakThroughput: 1850,
      slowestStep: 'ai_semantic_matching'
    }
  },
  exceptions: {
    categoryBreakdown: {
      unmatched: 3200,
      amount_mismatch: 1100,
      date_mismatch: 800,
      duplicate: 300,
      validation_error: 100
    },
    priorityDistribution: {
      critical: 150,
      high: 850,
      medium: 2100,
      low: 2400
    },
    resolutionTimes: {
      average: '4h 32m',
      median: '2h 15m',
      percentile95: '18h 45m'
    },
    escalationRate: 0.12
  }
}

// Helper functions for mock data operations
export const getReconciliationImportById = (
  id: string
): ReconciliationImport | undefined => {
  return mockReconciliationImports.find((imp) => imp.id === id)
}

export const getReconciliationProcessById = (
  id: string
): ReconciliationProcess | undefined => {
  return mockReconciliationProcesses.find((proc) => proc.id === id)
}

export const getReconciliationMatchesByProcessId = (
  processId: string
): ReconciliationMatch[] => {
  return mockReconciliationMatches.filter(
    (match) => match.processId === processId
  )
}

export const getReconciliationExceptionsByProcessId = (
  processId: string
): ReconciliationException[] => {
  return mockReconciliationExceptions.filter(
    (exception) => exception.processId === processId
  )
}

export const getReconciliationRuleById = (
  id: string
): ReconciliationRule | undefined => {
  return mockReconciliationRules.find((rule) => rule.id === id)
}

export const getReconciliationSourceById = (
  id: string
): ReconciliationSource | undefined => {
  return mockReconciliationSources.find((source) => source.id === id)
}

export const getReconciliationChainById = (
  id: string
): ReconciliationChain | undefined => {
  return mockReconciliationChains.find((chain) => chain.id === id)
}

// Real-time simulation helpers
export const simulateProcessProgress = (
  processId: string
): ReconciliationProcess | undefined => {
  const process = getReconciliationProcessById(processId)
  if (!process || process.status !== 'processing') return process

  // Simulate progress increment
  const currentProgress = process.progress.progressPercentage
  const newProgress = Math.min(currentProgress + Math.random() * 5, 100)

  return {
    ...process,
    progress: {
      ...process.progress,
      progressPercentage: Math.round(newProgress),
      processedTransactions: Math.round(
        (newProgress / 100) * process.progress.totalTransactions
      ),
      matchedTransactions: Math.round(
        (newProgress / 100) * process.progress.totalTransactions * 0.95
      ),
      exceptionCount: Math.round(
        (newProgress / 100) * process.progress.totalTransactions * 0.05
      )
    },
    updatedAt: new Date().toISOString()
  }
}

export default {
  mockExternalTransactions,
  mockInternalTransactions,
  mockReconciliationImports,
  mockReconciliationProcesses,
  mockReconciliationMatches,
  mockReconciliationExceptions,
  mockReconciliationRules,
  mockReconciliationSources,
  mockReconciliationChains,
  mockReconciliationAnalytics,
  // Helper functions
  getReconciliationImportById,
  getReconciliationProcessById,
  getReconciliationMatchesByProcessId,
  getReconciliationExceptionsByProcessId,
  getReconciliationRuleById,
  getReconciliationSourceById,
  getReconciliationChainById,
  simulateProcessProgress
}
