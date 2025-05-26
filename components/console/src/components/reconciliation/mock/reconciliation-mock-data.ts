import {
  ImportEntity,
  ImportStatus,
  ImportFileType,
  CreateImportRequest
} from '@/core/domain/entities/import-entity'
import {
  ExternalTransactionEntity,
  ExternalTransactionType,
  ExternalTransactionStatus
} from '@/core/domain/entities/external-transaction-entity'
import {
  ReconciliationProcessEntity,
  ReconciliationProcessStatus,
  MatchType
} from '@/core/domain/entities/reconciliation-process-entity'
import {
  MatchEntity,
  MatchStatus,
  MatchReviewPriority
} from '@/core/domain/entities/match-entity'
import {
  ExceptionEntity,
  ExceptionReason,
  ExceptionCategory,
  ExceptionPriority,
  ExceptionStatus
} from '@/core/domain/entities/exception-entity'
import {
  ReconciliationRuleEntity,
  ReconciliationRuleType,
  RuleApprovalStatus
} from '@/core/domain/entities/reconciliation-rule-entity'
import {
  ReconciliationAdjustmentEntity,
  AdjustmentType,
  AdjustmentStatus
} from '@/core/domain/entities/reconciliation-adjustment-entity'

// Helper functions for generating realistic data
const generateId = () => crypto.randomUUID()

const generateDate = (daysAgo: number = 0, hoursOffset: number = 0) => {
  const date = new Date()
  date.setDate(date.getDate() - daysAgo)
  date.setHours(date.getHours() - hoursOffset)
  return date.toISOString()
}

const generateAmount = (min: number = 10, max: number = 10000) => {
  return Math.round((Math.random() * (max - min) + min) * 100) / 100
}

const generateReferenceNumber = (prefix: string = 'REF') => {
  return `${prefix}${Math.random().toString(36).substr(2, 8).toUpperCase()}`
}

const sampleBankNames = [
  'Chase Bank',
  'Bank of America',
  'Wells Fargo',
  'Citibank',
  'Goldman Sachs',
  'JPMorgan Chase',
  'US Bank',
  'PNC Bank',
  'Capital One',
  'TD Bank'
]

const sampleDescriptions = [
  'Wire transfer payment',
  'ACH deposit',
  'Card payment',
  'Check deposit',
  'Online transfer',
  'ATM withdrawal',
  'Direct deposit',
  'Payroll deposit',
  'Loan payment',
  'Mortgage payment',
  'Credit card payment',
  'Investment transfer',
  'Insurance payment',
  'Utility payment',
  'Vendor payment',
  'Customer refund',
  'Processing fee',
  'Service charge',
  'Interest payment',
  'Dividend payment'
]

const sampleMerchantNames = [
  'Amazon Payments',
  'PayPal',
  'Stripe',
  'Square',
  'Venmo',
  'Apple Pay',
  'Google Pay',
  'Shopify Payments',
  'Adyen',
  'Worldpay'
]

export class ReconciliationMockData {
  // Import Mock Data
  static generateImports(count: number = 10): ImportEntity[] {
    const imports: ImportEntity[] = []
    const statuses: ImportStatus[] = [
      'completed',
      'processing',
      'failed',
      'pending',
      'validating'
    ]
    const fileTypes: ImportFileType[] = ['csv', 'json', 'xlsx']

    for (let i = 0; i < count; i++) {
      const status = statuses[Math.floor(Math.random() * statuses.length)]
      const fileType = fileTypes[Math.floor(Math.random() * fileTypes.length)]
      const totalRecords = Math.floor(Math.random() * 5000) + 500
      const processedRecords =
        status === 'completed'
          ? totalRecords
          : status === 'processing'
            ? Math.floor(totalRecords * 0.6)
            : status === 'failed'
              ? Math.floor(totalRecords * 0.3)
              : 0
      const failedRecords =
        status === 'failed'
          ? Math.floor(totalRecords * 0.1)
          : status === 'processing'
            ? Math.floor(totalRecords * 0.02)
            : 0

      const importEntity: ImportEntity = {
        id: generateId(),
        ledgerId: generateId(),
        organizationId: generateId(),
        fileName: `transactions_${generateDate(i).split('T')[0]}_${i + 1}.${fileType}`,
        filePath: `/uploads/transactions_${generateDate(i).split('T')[0]}_${i + 1}.${fileType}`,
        fileSize: Math.floor(Math.random() * 5000000) + 100000,
        status,
        totalRecords,
        processedRecords,
        failedRecords,
        startedAt:
          status !== 'pending'
            ? generateDate(i, Math.floor(Math.random() * 24))
            : undefined,
        completedAt:
          status === 'completed'
            ? generateDate(i, Math.floor(Math.random() * 12))
            : undefined,
        errorDetails:
          status === 'failed'
            ? {
                validationErrors: [
                  {
                    line: 1245,
                    field: 'amount',
                    error: 'Invalid decimal format',
                    value: '12.34.56'
                  },
                  {
                    line: 2108,
                    field: 'date',
                    error: 'Invalid date format',
                    value: '2024/13/45'
                  }
                ],
                summary: 'Multiple validation errors detected during processing'
              }
            : undefined,
        createdAt: generateDate(i + 1),
        updatedAt: generateDate(i)
      }

      imports.push(importEntity)
    }

    return imports
  }

  // External Transaction Mock Data
  static generateExternalTransactions(
    importId: string,
    count: number = 100
  ): ExternalTransactionEntity[] {
    const transactions: ExternalTransactionEntity[] = []
    const types: ExternalTransactionType[] = [
      'debit',
      'credit',
      'transfer',
      'fee',
      'adjustment'
    ]
    const statuses: ExternalTransactionStatus[] = [
      'imported',
      'validated',
      'processed',
      'matched',
      'exception'
    ]
    const currencies = ['USD', 'EUR', 'GBP', 'CAD', 'AUD']

    for (let i = 0; i < count; i++) {
      const type = types[Math.floor(Math.random() * types.length)]
      const status = statuses[Math.floor(Math.random() * statuses.length)]
      const currency = currencies[Math.floor(Math.random() * currencies.length)]

      const transaction: ExternalTransactionEntity = {
        id: generateId(),
        importId,
        externalId: generateReferenceNumber('EXT'),
        sourceSystem:
          sampleBankNames[Math.floor(Math.random() * sampleBankNames.length)],
        amount: type === 'debit' ? -generateAmount() : generateAmount(),
        currency,
        date: generateDate(Math.floor(Math.random() * 30)),
        description:
          sampleDescriptions[
            Math.floor(Math.random() * sampleDescriptions.length)
          ],
        referenceNumber: generateReferenceNumber(),
        accountNumber: `****${Math.floor(Math.random() * 9999)
          .toString()
          .padStart(4, '0')}`,
        accountName: `Account ${Math.floor(Math.random() * 100) + 1}`,
        transactionType: type,
        status,
        metadata: {
          sourceBank:
            sampleBankNames[Math.floor(Math.random() * sampleBankNames.length)],
          processingTime: Math.floor(Math.random() * 1000) + 100,
          riskScore: Math.random() * 100
        },
        rawData: {
          originalAmount:
            type === 'debit' ? -generateAmount() : generateAmount(),
          originalCurrency: currency,
          exchangeRate: currency !== 'USD' ? Math.random() * 0.2 + 0.9 : 1
        },
        fingerprint: crypto.randomUUID().replace(/-/g, '').substring(0, 16),
        createdAt: generateDate(Math.floor(Math.random() * 30)),
        updatedAt: generateDate(Math.floor(Math.random() * 15))
      }

      transactions.push(transaction)
    }

    return transactions
  }

  // Reconciliation Process Mock Data
  static generateReconciliationProcesses(
    count: number = 5
  ): ReconciliationProcessEntity[] {
    const processes: ReconciliationProcessEntity[] = []
    const statuses: ReconciliationProcessStatus[] = [
      'completed',
      'processing',
      'failed',
      'queued',
      'paused'
    ]

    for (let i = 0; i < count; i++) {
      const status = statuses[Math.floor(Math.random() * statuses.length)]
      const totalTransactions = Math.floor(Math.random() * 5000) + 1000
      const processedTransactions =
        status === 'completed'
          ? totalTransactions
          : status === 'processing'
            ? Math.floor(totalTransactions * 0.7)
            : status === 'failed'
              ? Math.floor(totalTransactions * 0.4)
              : 0
      const matchedTransactions = Math.floor(processedTransactions * 0.9)
      const exceptionCount = processedTransactions - matchedTransactions

      const process: ReconciliationProcessEntity = {
        id: generateId(),
        ledgerId: generateId(),
        organizationId: generateId(),
        importId: generateId(),
        name: `Reconciliation Process ${i + 1} - ${generateDate(i).split('T')[0]}`,
        description: `Automated reconciliation for imported transactions`,
        status,
        progress: {
          totalTransactions,
          processedTransactions,
          matchedTransactions,
          exceptionCount,
          progressPercentage: Math.floor(
            (processedTransactions / totalTransactions) * 100
          ),
          currentPhase:
            status === 'processing' ? 'ai_matching' : 'finalization',
          phaseProgress: Math.floor(Math.random() * 100),
          estimatedTimeRemaining:
            status === 'processing'
              ? Math.floor(Math.random() * 3600)
              : undefined,
          throughput: Math.floor(Math.random() * 100) + 50
        },
        configuration: {
          enableAiMatching: Math.random() > 0.3,
          minConfidenceScore: Math.random() * 0.3 + 0.7,
          maxCandidates: Math.floor(Math.random() * 100) + 50,
          parallelWorkers: Math.floor(Math.random() * 15) + 5,
          batchSize: Math.floor(Math.random() * 200) + 100,
          amountTolerance: Math.random() * 0.1,
          dateTolerance: Math.floor(Math.random() * 7) + 1,
          rules: [
            { ruleId: generateId(), priority: 1, isEnabled: true },
            { ruleId: generateId(), priority: 2, isEnabled: true },
            { ruleId: generateId(), priority: 3, isEnabled: false }
          ]
        },
        summary:
          status === 'completed'
            ? {
                matchTypes: {
                  exact: Math.floor(matchedTransactions * 0.6),
                  fuzzy: Math.floor(matchedTransactions * 0.2),
                  ai_semantic: Math.floor(matchedTransactions * 0.15),
                  manual: Math.floor(matchedTransactions * 0.05),
                  rule_based: Math.floor(matchedTransactions * 0.7)
                },
                averageConfidence: Math.random() * 0.2 + 0.8,
                processingTime: `${Math.floor(Math.random() * 60)}:${Math.floor(
                  Math.random() * 60
                )
                  .toString()
                  .padStart(2, '0')}`,
                throughput: `${Math.floor(Math.random() * 500) + 100} transactions/minute`,
                exceptionBreakdown: {
                  no_match_found: Math.floor(exceptionCount * 0.4),
                  multiple_matches: Math.floor(exceptionCount * 0.2),
                  low_confidence: Math.floor(exceptionCount * 0.3),
                  amount_mismatch: Math.floor(exceptionCount * 0.05),
                  date_mismatch: Math.floor(exceptionCount * 0.03),
                  validation_failed: Math.floor(exceptionCount * 0.01),
                  manual_review_required: Math.floor(exceptionCount * 0.01)
                },
                qualityMetrics: {
                  accuracyScore: Math.random() * 0.1 + 0.9,
                  precisionScore: Math.random() * 0.1 + 0.85,
                  recallScore: Math.random() * 0.1 + 0.88,
                  f1Score: Math.random() * 0.1 + 0.87
                }
              }
            : undefined,
        startedAt:
          status !== 'queued'
            ? generateDate(i, Math.floor(Math.random() * 24))
            : undefined,
        completedAt:
          status === 'completed'
            ? generateDate(i, Math.floor(Math.random() * 12))
            : undefined,
        estimatedCompletionAt:
          status === 'processing'
            ? generateDate(0, -Math.floor(Math.random() * 2))
            : undefined,
        createdAt: generateDate(i + 1),
        updatedAt: generateDate(i),
        createdBy: `user${Math.floor(Math.random() * 10) + 1}@company.com`,
        lastModifiedBy:
          status !== 'queued'
            ? `user${Math.floor(Math.random() * 10) + 1}@company.com`
            : undefined
      }

      processes.push(process)
    }

    return processes
  }

  // Match Mock Data
  static generateMatches(processId: string, count: number = 50): MatchEntity[] {
    const matches: MatchEntity[] = []
    const types: MatchType[] = [
      'exact',
      'fuzzy',
      'ai_semantic',
      'manual',
      'rule_based'
    ]
    const statuses: MatchStatus[] = [
      'pending',
      'confirmed',
      'rejected',
      'under_review',
      'auto_approved'
    ]
    const priorities: MatchReviewPriority[] = [
      'low',
      'medium',
      'high',
      'critical'
    ]

    for (let i = 0; i < count; i++) {
      const matchType = types[Math.floor(Math.random() * types.length)]
      const status = statuses[Math.floor(Math.random() * statuses.length)]
      const confidenceScore =
        matchType === 'exact'
          ? Math.random() * 0.05 + 0.95
          : matchType === 'ai_semantic'
            ? Math.random() * 0.3 + 0.6
            : Math.random() * 0.4 + 0.6

      const match: MatchEntity = {
        id: generateId(),
        processId,
        externalTransactionId: generateId(),
        internalTransactionIds: [generateId()],
        matchType,
        confidenceScore,
        status,
        ruleId: matchType === 'rule_based' ? generateId() : undefined,
        matchedFields: {
          amount: matchType === 'exact' ? true : Math.random() * 0.2 + 0.8,
          date: matchType === 'exact' ? true : Math.random() * 0.3 + 0.7,
          description:
            matchType === 'ai_semantic'
              ? Math.random() * 0.3 + 0.7
              : Math.random() > 0.5,
          reference_number: matchType === 'exact' ? true : Math.random() > 0.3,
          similarity_score:
            matchType === 'ai_semantic' ? confidenceScore : undefined,
          embedding_model:
            matchType === 'ai_semantic'
              ? 'sentence-transformers/all-MiniLM-L6-v2'
              : undefined
        },
        similarities:
          matchType === 'ai_semantic'
            ? {
                overall: confidenceScore,
                amount: Math.random() * 0.2 + 0.8,
                date: Math.random() * 0.3 + 0.7,
                description: Math.random() * 0.4 + 0.6,
                reference: Math.random() * 0.3 + 0.7,
                semantic: confidenceScore,
                structural: Math.random() * 0.2 + 0.8
              }
            : undefined,
        reviewedBy:
          status === 'confirmed' || status === 'rejected'
            ? `analyst${Math.floor(Math.random() * 5) + 1}@company.com`
            : undefined,
        reviewedAt:
          status === 'confirmed' || status === 'rejected'
            ? generateDate(Math.floor(Math.random() * 7))
            : undefined,
        aiInsights:
          matchType === 'ai_semantic'
            ? {
                description_similarity: Math.random() * 0.3 + 0.7,
                amount_similarity: Math.random() * 0.2 + 0.8,
                temporal_proximity: Math.random() * 0.3 + 0.7,
                pattern_confidence: confidenceScore,
                suggested_review_priority:
                  priorities[Math.floor(Math.random() * priorities.length)],
                explanation:
                  'AI detected high semantic similarity in transaction descriptions and temporal patterns',
                confidence_factors: [
                  {
                    factor: 'Amount proximity',
                    impact: 0.3,
                    description: 'Amounts are very close',
                    weight: 0.25
                  },
                  {
                    factor: 'Description similarity',
                    impact: 0.4,
                    description: 'High semantic similarity',
                    weight: 0.35
                  },
                  {
                    factor: 'Temporal proximity',
                    impact: 0.2,
                    description: 'Transactions occurred within time window',
                    weight: 0.2
                  },
                  {
                    factor: 'Pattern matching',
                    impact: 0.1,
                    description: 'Matches historical patterns',
                    weight: 0.2
                  }
                ]
              }
            : undefined,
        createdAt: generateDate(Math.floor(Math.random() * 7)),
        updatedAt: generateDate(Math.floor(Math.random() * 3))
      }

      matches.push(match)
    }

    return matches
  }

  // Exception Mock Data
  static generateExceptions(
    processId: string,
    count: number = 20
  ): ExceptionEntity[] {
    const exceptions: ExceptionEntity[] = []
    const reasons: ExceptionReason[] = [
      'no_match_found',
      'multiple_matches',
      'low_confidence',
      'amount_mismatch',
      'date_mismatch'
    ]
    const categories: ExceptionCategory[] = [
      'unmatched',
      'ambiguous',
      'discrepancy',
      'validation'
    ]
    const priorities: ExceptionPriority[] = [
      'low',
      'medium',
      'high',
      'critical'
    ]
    const statuses: ExceptionStatus[] = [
      'pending',
      'assigned',
      'investigating',
      'resolved',
      'escalated'
    ]

    for (let i = 0; i < count; i++) {
      const reason = reasons[Math.floor(Math.random() * reasons.length)]
      const category = categories[Math.floor(Math.random() * categories.length)]
      const priority = priorities[Math.floor(Math.random() * priorities.length)]
      const status = statuses[Math.floor(Math.random() * statuses.length)]

      const exception: ExceptionEntity = {
        id: generateId(),
        processId,
        externalTransactionId: generateId(),
        internalTransactionId:
          reason === 'multiple_matches' ? generateId() : undefined,
        reason,
        category,
        priority,
        status,
        assignedTo:
          status !== 'pending'
            ? `analyst${Math.floor(Math.random() * 5) + 1}@company.com`
            : undefined,
        assignedAt:
          status !== 'pending'
            ? generateDate(Math.floor(Math.random() * 5))
            : undefined,
        resolvedBy:
          status === 'resolved'
            ? `analyst${Math.floor(Math.random() * 5) + 1}@company.com`
            : undefined,
        resolvedAt:
          status === 'resolved'
            ? generateDate(Math.floor(Math.random() * 2))
            : undefined,
        investigationNotes:
          status === 'investigating' || status === 'resolved'
            ? [
                {
                  id: generateId(),
                  timestamp: generateDate(Math.floor(Math.random() * 3)),
                  author: `analyst${Math.floor(Math.random() * 5) + 1}@company.com`,
                  note: 'Initial investigation shows potential timing differences in transaction processing',
                  type: 'investigation'
                },
                {
                  id: generateId(),
                  timestamp: generateDate(Math.floor(Math.random() * 2)),
                  author: `analyst${Math.floor(Math.random() * 5) + 1}@company.com`,
                  note: 'Found similar transaction with slight amount variance - investigating further',
                  type: 'finding'
                }
              ]
            : [],
        suggestedActions: [
          {
            action: 'manual_match',
            confidence: Math.random() * 0.3 + 0.6,
            description: `Potential match found with ${Math.random() > 0.5 ? 'amount' : 'timing'} variance`,
            candidateTransactionId: generateId(),
            reasons: ['Similar amount', 'Close timing', 'Matching account'],
            aiGenerated: true
          },
          {
            action: 'investigate',
            confidence: Math.random() * 0.4 + 0.5,
            description: 'Requires manual investigation for pattern analysis',
            reasons: ['Unusual transaction pattern', 'No clear match found'],
            aiGenerated: false
          }
        ],
        escalationLevel:
          priority === 'critical' ? Math.floor(Math.random() * 2) + 1 : 0,
        metadata: {
          transactionAmount: generateAmount(),
          sourceSystem:
            sampleBankNames[Math.floor(Math.random() * sampleBankNames.length)],
          customerImpact: Math.random() > 0.7,
          regulatoryRisk: priority === 'critical' || priority === 'high'
        },
        createdAt: generateDate(Math.floor(Math.random() * 10)),
        updatedAt: generateDate(Math.floor(Math.random() * 5))
      }

      exceptions.push(exception)
    }

    return exceptions
  }

  // Reconciliation Rule Mock Data
  static generateReconciliationRules(
    count: number = 15
  ): ReconciliationRuleEntity[] {
    const rules: ReconciliationRuleEntity[] = []
    const statuses: RuleApprovalStatus[] = [
      'approved',
      'draft',
      'pending_approval',
      'rejected'
    ]

    const ruleTemplates = [
      {
        name: 'Exact Amount Match',
        type: 'amount',
        description: 'Matches transactions with identical amounts'
      },
      {
        name: 'Amount Range Match',
        type: 'amount',
        description: 'Matches transactions within amount tolerance'
      },
      {
        name: 'Date Window Match',
        type: 'date',
        description: 'Matches transactions within date range'
      },
      {
        name: 'Reference Number Match',
        type: 'string',
        description: 'Matches based on reference numbers'
      },
      {
        name: 'Description Fuzzy Match',
        type: 'string',
        description: 'Fuzzy matching on transaction descriptions'
      },
      {
        name: 'Account Number Pattern',
        type: 'regex',
        description: 'Regex pattern matching for account numbers'
      },
      {
        name: 'Metadata Field Match',
        type: 'metadata',
        description: 'Matches based on metadata fields'
      },
      {
        name: 'Composite Business Rule',
        type: 'composite',
        description: 'Complex multi-field matching rule'
      }
    ]

    for (let i = 0; i < count; i++) {
      const template =
        ruleTemplates[Math.floor(Math.random() * ruleTemplates.length)]
      const ruleType = template.type as ReconciliationRuleType
      const status = statuses[Math.floor(Math.random() * statuses.length)]

      const rule: ReconciliationRuleEntity = {
        id: generateId(),
        ledgerId: generateId(),
        organizationId: generateId(),
        name: `${template.name} ${i + 1}`,
        description: template.description,
        ruleType,
        criteria: {
          field:
            ruleType === 'amount'
              ? 'amount'
              : ruleType === 'date'
                ? 'transaction_date'
                : ruleType === 'string'
                  ? 'description'
                  : ruleType === 'regex'
                    ? 'reference_number'
                    : 'metadata',
          operator:
            ruleType === 'amount'
              ? 'within_range'
              : ruleType === 'date'
                ? 'within_date_range'
                : ruleType === 'string'
                  ? 'fuzzy_match'
                  : ruleType === 'regex'
                    ? 'regex_match'
                    : 'equals',
          tolerance: ruleType === 'amount' ? Math.random() * 0.1 : undefined,
          similarityThreshold:
            ruleType === 'string' ? Math.random() * 0.3 + 0.7 : undefined,
          dateWindow:
            ruleType === 'date' ? Math.floor(Math.random() * 7) + 1 : undefined,
          regexPattern: ruleType === 'regex' ? '^[A-Z]{3}[0-9]{6}$' : undefined
        },
        priority: Math.floor(Math.random() * 10) + 1,
        isActive: status === 'approved' && Math.random() > 0.2,
        version: Math.floor(Math.random() * 5) + 1,
        performance:
          status === 'approved'
            ? {
                matchCount: Math.floor(Math.random() * 1000) + 100,
                successRate: Math.random() * 0.2 + 0.8,
                falsePositiveRate: Math.random() * 0.05,
                falseNegativeRate: Math.random() * 0.05,
                averageConfidence: Math.random() * 0.2 + 0.8,
                averageExecutionTime: Math.random() * 50 + 10,
                lastExecutionTime: Math.random() * 100 + 20,
                totalExecutions: Math.floor(Math.random() * 500) + 50,
                lastOptimizationDate: generateDate(
                  Math.floor(Math.random() * 30)
                )
              }
            : undefined,
        approvalStatus: status,
        approvedBy:
          status === 'approved'
            ? `manager${Math.floor(Math.random() * 3) + 1}@company.com`
            : undefined,
        approvedAt:
          status === 'approved'
            ? generateDate(Math.floor(Math.random() * 60))
            : undefined,
        createdBy: `analyst${Math.floor(Math.random() * 5) + 1}@company.com`,
        createdAt: generateDate(Math.floor(Math.random() * 90)),
        updatedAt: generateDate(Math.floor(Math.random() * 30)),
        tags: [
          'automated',
          ruleType,
          Math.random() > 0.5 ? 'high-volume' : 'standard'
        ]
      }

      rules.push(rule)
    }

    return rules
  }

  // Adjustment Mock Data
  static generateAdjustments(
    count: number = 8
  ): ReconciliationAdjustmentEntity[] {
    const adjustments: ReconciliationAdjustmentEntity[] = []
    const types: AdjustmentType[] = [
      'timing_difference',
      'amount_variance',
      'fee_adjustment',
      'write_off',
      'correction'
    ]
    const statuses: AdjustmentStatus[] = [
      'draft',
      'pending_approval',
      'approved',
      'implemented',
      'rejected'
    ]

    for (let i = 0; i < count; i++) {
      const adjustmentType = types[Math.floor(Math.random() * types.length)]
      const status = statuses[Math.floor(Math.random() * statuses.length)]
      const amount = generateAmount(10, 5000)

      const adjustment: ReconciliationAdjustmentEntity = {
        id: generateId(),
        processId: generateId(),
        exceptionId: generateId(),
        adjustmentType,
        amount,
        currency: 'USD',
        reason: `${adjustmentType.replace('_', ' ')} detected during reconciliation`,
        description: `Adjustment required to resolve reconciliation discrepancy`,
        status,
        approval: {
          required: amount > 1000,
          level: amount > 5000 ? 2 : 1,
          approvers: [
            {
              userId: generateId(),
              name: 'John Smith',
              role: 'Senior Analyst',
              level: 1
            },
            {
              userId: generateId(),
              name: 'Jane Doe',
              role: 'Manager',
              level: 2
            }
          ].slice(0, amount > 5000 ? 2 : 1),
          approvalChain: [
            {
              level: 1,
              approver: 'John Smith',
              status:
                status === 'approved' || status === 'implemented'
                  ? 'approved'
                  : 'pending',
              timestamp:
                status === 'approved' || status === 'implemented'
                  ? generateDate(1)
                  : undefined,
              comments:
                status === 'approved' || status === 'implemented'
                  ? 'Reviewed and approved'
                  : undefined
            }
          ]
        },
        sourceTransactionId: generateId(),
        targetTransactionId: generateId(),
        accountId: generateId(),
        adjustmentDate: generateDate(0),
        effectiveDate: generateDate(1),
        evidence: [
          {
            type: 'bank_statement',
            description: 'Bank statement showing transaction details',
            source: 'Chase Bank Statement Dec 2024',
            timestamp: generateDate(2)
          },
          {
            type: 'calculation',
            description: 'Reconciliation calculation worksheet',
            source: 'Internal Analysis',
            calculation: {
              formula: 'External Amount - Internal Amount',
              inputs: { external: amount + 10, internal: amount - 10 },
              result: 20,
              explanation: 'Timing difference in processing',
              verificationSteps: [
                'Verified external transaction',
                'Confirmed internal posting',
                'Calculated variance'
              ]
            },
            timestamp: generateDate(1)
          }
        ],
        impactAnalysis: {
          accountImpacts: [
            {
              accountId: generateId(),
              accountName: 'Cash - Operating Account',
              currentBalance: generateAmount(10000, 100000),
              adjustmentAmount: amount,
              newBalance: generateAmount(10000, 100000) + amount,
              impact: amount > 0 ? 'positive' : 'negative'
            }
          ],
          financialImpact: {
            totalAdjustmentAmount: amount,
            currencyImpact: { USD: amount },
            netEffect: amount,
            percentageOfTotal: Math.random() * 0.1,
            materiality: amount > 1000 ? 'material' : 'immaterial'
          },
          riskAssessment: {
            overallRisk:
              amount > 5000 ? 'high' : amount > 1000 ? 'medium' : 'low',
            riskFactors: [
              {
                factor: 'Financial Impact',
                probability: 0.8,
                impact: amount > 1000 ? 8 : 5,
                score: amount > 1000 ? 6.4 : 4,
                description: `Adjustment of $${amount} may impact financial reporting`
              }
            ],
            mitigationSteps: [
              'Document approval chain',
              'Verify with bank records',
              'Update reconciliation procedures'
            ],
            reviewRequired: amount > 1000
          }
        },
        metadata: {
          reconciliationRun: generateId(),
          analystId: `analyst${Math.floor(Math.random() * 5) + 1}@company.com`,
          urgency: amount > 5000 ? 'high' : 'normal',
          customerNotificationRequired: Math.random() > 0.8
        },
        createdBy: `analyst${Math.floor(Math.random() * 5) + 1}@company.com`,
        createdAt: generateDate(Math.floor(Math.random() * 7)),
        updatedAt: generateDate(Math.floor(Math.random() * 3))
      }

      adjustments.push(adjustment)
    }

    return adjustments
  }

  // Dashboard Analytics Mock Data
  static generateDashboardAnalytics() {
    return {
      overview: {
        activeProcesses: 3,
        pendingExceptions: 47,
        matchesReview: 156,
        todayImports: 8,
        reconciliationRate: 94.2,
        avgProcessingTime: '4.2m',
        totalValueProcessed: 2547823.45,
        aiAccuracy: 96.8,
        straightThroughProcessing: 92.1
      },
      trends: {
        reconciliationRates: Array.from({ length: 30 }, (_, i) => ({
          date: generateDate(29 - i).split('T')[0],
          rate: Math.random() * 10 + 90,
          volume: Math.floor(Math.random() * 5000) + 1000,
          exceptions: Math.floor(Math.random() * 200) + 50
        })),
        matchTypeDistribution: [
          { type: 'exact', count: 1856, percentage: 62.1 },
          { type: 'fuzzy', count: 398, percentage: 13.3 },
          { type: 'ai_semantic', count: 456, percentage: 15.2 },
          { type: 'manual', count: 178, percentage: 5.9 },
          { type: 'rule_based', count: 102, percentage: 3.4 }
        ],
        exceptionReasons: [
          { reason: 'no_match_found', count: 18, percentage: 38.3 },
          { reason: 'multiple_matches', count: 12, percentage: 25.5 },
          { reason: 'low_confidence', count: 8, percentage: 17.0 },
          { reason: 'amount_mismatch', count: 6, percentage: 12.8 },
          { reason: 'date_mismatch', count: 3, percentage: 6.4 }
        ]
      },
      performance: {
        aiMatchingAccuracy: 96.8,
        processingThroughput: 2847,
        averageResolutionTime: 4.2,
        customerSatisfactionScore: 4.7,
        costPerTransaction: 0.23,
        timeReduction: 68.5
      }
    }
  }

  // Comprehensive mock data generator
  static generateComprehensiveDataSet() {
    const imports = this.generateImports(15)
    const processes = this.generateReconciliationProcesses(8)
    const rules = this.generateReconciliationRules(20)
    const adjustments = this.generateAdjustments(12)
    const analytics = this.generateDashboardAnalytics()

    // Generate related data
    const externalTransactions = imports.flatMap((imp) =>
      this.generateExternalTransactions(
        imp.id,
        Math.floor(Math.random() * 200) + 50
      )
    )

    const matches = processes.flatMap((proc) =>
      this.generateMatches(proc.id, Math.floor(Math.random() * 100) + 30)
    )

    const exceptions = processes.flatMap((proc) =>
      this.generateExceptions(proc.id, Math.floor(Math.random() * 30) + 10)
    )

    return {
      imports,
      externalTransactions,
      processes,
      matches,
      exceptions,
      rules,
      adjustments,
      analytics
    }
  }
}
