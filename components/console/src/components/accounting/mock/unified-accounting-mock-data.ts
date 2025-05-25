/**
 * Unified mock data structure for Accounting plugin
 * Consolidates data models from both accounting-mock-data.ts and transaction-route-mock-data.ts
 */

// Core interfaces - unified structure
export interface AccountType {
  id: string
  name: string
  description: string
  keyValue: string // Primary identifier for accounting
  code?: string // Alternative code for compatibility
  domain: 'ledger' | 'external'
  nature?: 'debit' | 'credit' // Optional for accounting classification
  category?: 'asset' | 'liability' | 'equity' | 'revenue' | 'expense' // Optional for financial statements
  usageCount: number
  linkedAccounts: number
  lastUsed: string
  status: 'active' | 'inactive' | 'draft' | 'invalid'
  createdAt: string
  updatedAt: string
}

export interface OperationRoute {
  id: string
  transactionRouteId?: string // Optional for standalone operations
  operationType: 'debit' | 'credit'
  sourceAccountTypeId: string
  destinationAccountTypeId: string
  amount: {
    expression: string // e.g., "{{amount}}", "{{amount}} * 0.03"
    description: string
    scale?: number // Decimal places
  }
  description: string
  order: number
  conditions?: {
    field: string
    operator: 'equals' | 'greater_than' | 'less_than' | 'contains'
    value: string
  }[]
  metadata?: {
    [key: string]: any
  }
}

export interface TransactionRoute {
  id: string
  name: string // Use 'name' as primary field
  title?: string // Alternative for compatibility
  description: string
  templateType:
    | 'transfer'
    | 'payment'
    | 'adjustment'
    | 'fee'
    | 'refund'
    | 'custom'
  category?: string // For additional categorization
  status: 'active' | 'draft' | 'deprecated' | 'inactive'
  operationRoutes: OperationRoute[]
  metadata: {
    requiresApproval?: boolean
    minimumAmount?: number
    maximumAmount?: number
    autoValidate?: boolean
    complianceLevel?: string
    [key: string]: any
  }
  version: string
  tags: string[]
  usageCount: number
  lastUsed: string
  createdAt: string
  updatedAt: string
}

// Analytics and reporting interfaces
export interface AccountUsageData {
  keyValue: string
  name: string
  usageCount: number
  percentage: number
}

export interface TransactionRoutePerformance {
  name: string // Use 'name' consistently
  usageCount: number
  successRate: number
  avgProcessingTime: string
}

export interface ComplianceTrend {
  date: string
  score: number
  violations: number
}

export interface AuditTrailEntry {
  id: string
  timestamp: string
  user: string
  action: string
  resource: string
  resourceId: string
  details: string
  ipAddress: string
  userAgent: string
}

export interface AnalyticsOverview {
  totalAccountTypes: number
  activeAccountTypes: number
  totalTransactionRoutes: number
  activeTransactionRoutes: number
  totalOperationRoutes: number
  monthlyUsage: number
  complianceScore: number
}

export interface AccountingAnalytics {
  overview: AnalyticsOverview
  accountTypeUsage: AccountUsageData[]
  transactionRoutePerformance: TransactionRoutePerformance[]
  complianceTrends: ComplianceTrend[]
  auditTrail: AuditTrailEntry[]
}

// Mock data
export const mockAccountTypes: AccountType[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231c',
    name: 'Checking Account',
    description: 'Standard checking account for daily transactions',
    keyValue: 'CHCK',
    code: 'CUST_CHECKING',
    domain: 'ledger',
    nature: 'debit',
    category: 'asset',
    usageCount: 245,
    linkedAccounts: 89,
    lastUsed: '2025-01-01T12:30:00Z',
    status: 'active',
    createdAt: '2024-11-15T00:00:00Z',
    updatedAt: '2024-12-20T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231d',
    name: 'Savings Account',
    description: 'Interest-bearing savings account',
    keyValue: 'SVGS',
    code: 'CUST_SAVINGS',
    domain: 'ledger',
    nature: 'debit',
    category: 'asset',
    usageCount: 156,
    linkedAccounts: 67,
    lastUsed: '2025-01-01T10:15:00Z',
    status: 'active',
    createdAt: '2024-11-10T00:00:00Z',
    updatedAt: '2024-12-18T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231e',
    name: 'External Bank Account',
    description: 'External bank account for wire transfers',
    keyValue: 'EXT_BANK',
    code: 'EXT_BANK',
    domain: 'external',
    nature: 'debit',
    category: 'asset',
    usageCount: 78,
    linkedAccounts: 23,
    lastUsed: '2024-12-30T16:45:00Z',
    status: 'active',
    createdAt: '2024-12-01T00:00:00Z',
    updatedAt: '2024-12-25T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    name: 'Merchant Settlement Account',
    description: 'Account for merchant settlement funds',
    keyValue: 'MRCH_SETTLE',
    code: 'MERCHANT_SETTLEMENT',
    domain: 'external',
    nature: 'credit',
    category: 'liability',
    usageCount: 134,
    linkedAccounts: 45,
    lastUsed: '2024-12-31T09:20:00Z',
    status: 'active',
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2320',
    name: 'Fee Revenue Account',
    description: 'Account to track fee revenue',
    keyValue: 'FEE_REV',
    code: 'FEE_REVENUE',
    domain: 'ledger',
    nature: 'credit',
    category: 'revenue',
    usageCount: 89,
    linkedAccounts: 12,
    lastUsed: '2024-12-31T23:59:00Z',
    status: 'active',
    createdAt: '2024-11-01T00:00:00Z',
    updatedAt: '2024-12-30T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2321',
    name: 'Suspense Account',
    description: 'Temporary holding account for unmatched transactions',
    keyValue: 'SUSPENSE',
    code: 'SUSPENSE',
    domain: 'ledger',
    nature: 'debit',
    category: 'asset',
    usageCount: 23,
    linkedAccounts: 5,
    lastUsed: '2024-12-29T14:30:00Z',
    status: 'active',
    createdAt: '2024-12-15T00:00:00Z',
    updatedAt: '2024-12-29T00:00:00Z'
  }
]

export const mockOperationRoutes: OperationRoute[] = [
  {
    id: 'op-001',
    transactionRouteId: 'tr-001',
    operationType: 'debit',
    sourceAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231c',
    destinationAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231d',
    amount: {
      expression: '{{amount}}',
      description: 'Full transaction amount',
      scale: 2
    },
    description: 'Debit source checking account',
    order: 1
  },
  {
    id: 'op-002',
    transactionRouteId: 'tr-001',
    operationType: 'credit',
    sourceAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231c',
    destinationAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231d',
    amount: {
      expression: '{{amount}}',
      description: 'Full transaction amount',
      scale: 2
    },
    description: 'Credit destination savings account',
    order: 2
  },
  {
    id: 'op-003',
    transactionRouteId: 'tr-002',
    operationType: 'debit',
    sourceAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231c',
    destinationAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231f',
    amount: {
      expression: '{{amount}}',
      description: 'Payment amount',
      scale: 2
    },
    description: 'Debit customer account for payment',
    order: 1
  },
  {
    id: 'op-004',
    transactionRouteId: 'tr-002',
    operationType: 'credit',
    sourceAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231c',
    destinationAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231f',
    amount: {
      expression: '{{amount}}',
      description: 'Payment amount',
      scale: 2
    },
    description: 'Credit merchant settlement account',
    order: 2
  },
  {
    id: 'op-005',
    transactionRouteId: 'tr-002',
    operationType: 'debit',
    sourceAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231c',
    destinationAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d2320',
    amount: {
      expression: '{{amount}} * 0.03',
      description: '3% processing fee',
      scale: 2
    },
    description: 'Charge processing fee',
    order: 3,
    conditions: [
      {
        field: 'amount',
        operator: 'greater_than',
        value: '100'
      }
    ]
  },
  {
    id: 'op-006',
    transactionRouteId: 'tr-002',
    operationType: 'credit',
    sourceAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d231c',
    destinationAccountTypeId: '01956b69-9102-75b7-8860-3e75c11d2320',
    amount: {
      expression: '{{amount}} * 0.03',
      description: '3% processing fee revenue',
      scale: 2
    },
    description: 'Record fee revenue',
    order: 4,
    conditions: [
      {
        field: 'amount',
        operator: 'greater_than',
        value: '100'
      }
    ]
  }
]

export const mockTransactionRoutes: TransactionRoute[] = [
  {
    id: 'tr-001',
    name: 'Standard Account Transfer',
    description: 'Standard transfer between internal accounts',
    templateType: 'transfer',
    status: 'active',
    operationRoutes: mockOperationRoutes.filter(
      (op) => op.transactionRouteId === 'tr-001'
    ),
    metadata: {
      requiresApproval: false,
      minimumAmount: 0.01,
      maximumAmount: 10000.0,
      autoValidate: true
    },
    version: '1.0.0',
    tags: ['transfer', 'internal', 'standard'],
    usageCount: 1234,
    lastUsed: '2025-01-01T14:20:00Z',
    createdAt: '2024-10-15T00:00:00Z',
    updatedAt: '2024-12-22T00:00:00Z'
  },
  {
    id: 'tr-002',
    name: 'Payment with Fee',
    description: 'Payment transaction with processing fee',
    templateType: 'payment',
    status: 'active',
    operationRoutes: mockOperationRoutes.filter(
      (op) => op.transactionRouteId === 'tr-002'
    ),
    metadata: {
      requiresApproval: true,
      minimumAmount: 100.0,
      maximumAmount: 50000.0,
      autoValidate: false,
      complianceLevel: 'high'
    },
    version: '1.2.0',
    tags: ['payment', 'fee', 'merchant'],
    usageCount: 456,
    lastUsed: '2024-12-29T11:30:00Z',
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z'
  }
]

export const mockAnalyticsData: AccountingAnalytics = {
  overview: {
    totalAccountTypes: 15,
    activeAccountTypes: 12,
    totalTransactionRoutes: 8,
    activeTransactionRoutes: 6,
    totalOperationRoutes: 24,
    monthlyUsage: 4567,
    complianceScore: 96.5
  },
  accountTypeUsage: [
    {
      keyValue: 'CHCK',
      name: 'Checking Account',
      usageCount: 245,
      percentage: 45.2
    },
    {
      keyValue: 'SVGS',
      name: 'Savings Account',
      usageCount: 156,
      percentage: 28.8
    },
    {
      keyValue: 'EXT_BANK',
      name: 'External Bank Account',
      usageCount: 78,
      percentage: 14.4
    },
    {
      keyValue: 'MRCH_SETTLE',
      name: 'Merchant Settlement Account',
      usageCount: 134,
      percentage: 11.6
    }
  ],
  transactionRoutePerformance: [
    {
      name: 'Standard Account Transfer',
      usageCount: 1234,
      successRate: 99.8,
      avgProcessingTime: '1.2s'
    },
    {
      name: 'Payment with Fee',
      usageCount: 456,
      successRate: 98.9,
      avgProcessingTime: '3.4s'
    }
  ],
  complianceTrends: [
    {
      date: '2024-12-01',
      score: 94.2,
      violations: 3
    },
    {
      date: '2024-12-15',
      score: 96.5,
      violations: 1
    },
    {
      date: '2025-01-01',
      score: 97.1,
      violations: 0
    }
  ],
  auditTrail: [
    {
      id: 'audit-001',
      timestamp: '2024-12-31T15:30:00Z',
      user: 'john.doe@company.com',
      action: 'account_type_created',
      resource: 'account-type',
      resourceId: '01956b69-9102-75b7-8860-3e75c11d231c',
      details: 'Created new account type: Checking Account (CHCK)',
      ipAddress: '192.168.1.100',
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
    },
    {
      id: 'audit-002',
      timestamp: '2024-12-30T09:15:00Z',
      user: 'jane.smith@company.com',
      action: 'transaction_route_updated',
      resource: 'transaction-route',
      resourceId: 'tr-001',
      details: 'Updated transaction route: Standard Account Transfer',
      ipAddress: '192.168.1.101',
      userAgent:
        'Mozilla/5.0 (macOS; Intel Mac OS X 10_15_7) AppleWebKit/537.36'
    }
  ]
}

// Helper functions
export function getTransactionRouteById(
  id: string
): TransactionRoute | undefined {
  return mockTransactionRoutes.find((route) => route.id === id)
}

export function getAccountTypeById(id: string): AccountType | undefined {
  return mockAccountTypes.find((accountType) => accountType.id === id)
}

export function getOperationRouteById(id: string): OperationRoute | undefined {
  return mockOperationRoutes.find((operation) => operation.id === id)
}

export function getAccountTypeByKeyValue(
  keyValue: string
): AccountType | undefined {
  return mockAccountTypes.find(
    (accountType) => accountType.keyValue === keyValue
  )
}

export function getOperationsByTransactionRoute(
  transactionRouteId: string
): OperationRoute[] {
  return mockOperationRoutes.filter(
    (operation) => operation.transactionRouteId === transactionRouteId
  )
}
