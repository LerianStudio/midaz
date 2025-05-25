export interface ReconciliationRuleEntity {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  description?: string
  ruleType: ReconciliationRuleType
  criteria: ReconciliationRuleCriteria
  priority: number
  isActive: boolean
  version: number
  performance?: ReconciliationRulePerformance
  configuration?: ReconciliationRuleConfiguration
  testResults?: ReconciliationRuleTestResult[]
  approvalStatus?: RuleApprovalStatus
  approvedBy?: string
  approvedAt?: string
  createdBy: string
  createdAt: string
  updatedAt: string
  tags?: string[]
}

export type ReconciliationRuleType =
  | 'amount'
  | 'date'
  | 'string'
  | 'regex'
  | 'metadata'
  | 'composite'
  | 'ai_assisted'

export interface ReconciliationRuleCriteria {
  field: string
  operator: ReconciliationRuleOperator
  value?: any
  tolerance?: number
  caseSensitive?: boolean
  additionalFields?: string[]
  similarityThreshold?: number
  dateWindow?: number
  amountWindow?: number
  regexPattern?: string
  metadataConditions?: MetadataCondition[]
  compositeRules?: CompositeRule[]
}

export type ReconciliationRuleOperator =
  | 'equals'
  | 'not_equals'
  | 'greater_than'
  | 'greater_than_or_equal'
  | 'less_than'
  | 'less_than_or_equal'
  | 'contains'
  | 'not_contains'
  | 'starts_with'
  | 'ends_with'
  | 'regex_match'
  | 'fuzzy_match'
  | 'semantic_match'
  | 'within_range'
  | 'within_date_range'
  | 'in_list'
  | 'not_in_list'

export interface MetadataCondition {
  key: string
  operator: ReconciliationRuleOperator
  value: any
  required: boolean
}

export interface CompositeRule {
  condition: 'AND' | 'OR'
  rules: ReconciliationRuleCriteria[]
  weight?: number
}

export interface ReconciliationRuleConfiguration {
  enablePreliminaryFiltering: boolean
  maxCandidates: number
  enableLogging: boolean
  logLevel: 'debug' | 'info' | 'warning' | 'error'
  timeout: number
  retryAttempts: number
  cacheResults: boolean
  parallelExecution: boolean
}

export interface ReconciliationRulePerformance {
  matchCount: number
  successRate: number
  falsePositiveRate: number
  falseNegativeRate: number
  averageConfidence: number
  averageExecutionTime: number
  lastExecutionTime: number
  totalExecutions: number
  lastOptimizationDate?: string
  optimizationSuggestions?: string[]
}

export interface ReconciliationRuleTestResult {
  id: string
  testDate: string
  testDataSize: number
  matches: number
  falsePositives: number
  falseNegatives: number
  executionTime: number
  confidence: number
  passed: boolean
  issues?: string[]
  recommendations?: string[]
}

export type RuleApprovalStatus =
  | 'draft'
  | 'pending_approval'
  | 'approved'
  | 'rejected'
  | 'requires_changes'

export interface CreateReconciliationRuleRequest {
  ledgerId: string
  organizationId: string
  name: string
  description?: string
  ruleType: ReconciliationRuleType
  criteria: ReconciliationRuleCriteria
  priority: number
  configuration?: ReconciliationRuleConfiguration
  tags?: string[]
}

export interface UpdateReconciliationRuleRequest {
  name?: string
  description?: string
  criteria?: ReconciliationRuleCriteria
  priority?: number
  isActive?: boolean
  configuration?: ReconciliationRuleConfiguration
  tags?: string[]
}

export interface TestReconciliationRuleRequest {
  ruleId: string
  testData: ReconciliationRuleTestData
  configuration?: {
    sampleSize?: number
    includePerformanceMetrics: boolean
    includeConfidenceScores: boolean
  }
}

export interface ReconciliationRuleTestData {
  externalTransactions: any[]
  internalTransactions: any[]
  expectedMatches?: ExpectedMatch[]
}

export interface ExpectedMatch {
  externalTransactionId: string
  internalTransactionId: string
  shouldMatch: boolean
  expectedConfidence?: number
}

export interface BulkRuleOperationRequest {
  ruleIds: string[]
  operation: BulkRuleOperation
  priority?: number
  isActive?: boolean
  tags?: string[]
  operatedBy: string
}

export type BulkRuleOperation =
  | 'activate'
  | 'deactivate'
  | 'change_priority'
  | 'add_tags'
  | 'remove_tags'
  | 'delete'

export interface ReconciliationRuleFilters {
  ruleType?: ReconciliationRuleType[]
  isActive?: boolean
  priority?: {
    min: number
    max: number
  }
  approvalStatus?: RuleApprovalStatus[]
  createdBy?: string
  dateRange?: {
    start: string
    end: string
  }
  tags?: string[]
  searchText?: string
  performance?: {
    minSuccessRate?: number
    maxExecutionTime?: number
  }
}

export interface ReconciliationRuleTemplate {
  id: string
  name: string
  description: string
  category: string
  ruleType: ReconciliationRuleType
  template: Partial<ReconciliationRuleEntity>
  parameters: RuleTemplateParameter[]
  examples: RuleTemplateExample[]
  complexity: 'simple' | 'intermediate' | 'advanced'
  popularity: number
  tags: string[]
}

export interface RuleTemplateParameter {
  name: string
  description: string
  type: 'string' | 'number' | 'boolean' | 'select' | 'multi-select'
  required: boolean
  defaultValue?: any
  options?: string[]
  validation?: string
}

export interface RuleTemplateExample {
  name: string
  description: string
  inputData: any
  expectedOutput: any
  explanation: string
}

export interface ReconciliationRuleAnalytics {
  performanceMetrics: {
    ruleId: string
    ruleName: string
    executionCount: number
    averageExecutionTime: number
    successRate: number
    matchCount: number
    lastExecution: string
  }[]
  typeDistribution: {
    type: ReconciliationRuleType
    count: number
    percentage: number
    averagePerformance: number
  }[]
  priorityDistribution: {
    priority: number
    count: number
    averagePerformance: number
  }[]
  usagePatterns: {
    date: string
    totalExecutions: number
    uniqueRulesUsed: number
    averageExecutionTime: number
  }[]
  optimizationOpportunities: {
    ruleId: string
    ruleName: string
    currentPerformance: number
    potentialImprovement: number
    recommendation: string
    effort: 'low' | 'medium' | 'high'
  }[]
}

export interface ReconciliationRuleSummary {
  totalRules: number
  activeRules: number
  inactiveRules: number
  draftRules: number
  pendingApprovalRules: number
  averageSuccessRate: number
  mostUsedRuleType: ReconciliationRuleType
  topPerformingRules: {
    ruleId: string
    name: string
    successRate: number
    matchCount: number
  }[]
  recentlyCreated: ReconciliationRuleEntity[]
}
