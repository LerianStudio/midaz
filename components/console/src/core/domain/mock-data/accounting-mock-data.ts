/**
 * Comprehensive mock data for Accounting plugin following PLAN-ACCOUNTING.md
 * Includes account types, transaction routes, operation routes, and analytics data
 */

export interface AccountType {
  id: string
  name: string
  description: string
  keyValue: string
  domain: 'ledger' | 'external'
  usageCount: number
  linkedAccounts: number
  lastUsed: string
  status: 'active' | 'inactive' | 'draft' | 'invalid'
  createdAt: string
  updatedAt: string
}

export interface Account {
  id: string
  alias: string
  type: string[]
}

export interface OperationRoute {
  id: string
  type: 'source' | 'destination'
  account: Partial<Account>
  metadata: {
    description: string
    [key: string]: any
  }
}

export interface TransactionRoute {
  id: string
  title: string
  description: string
  category: string
  metadata: {
    requiresApproval: boolean
    minimumAmount: number
    maximumAmount: number
    autoValidate: boolean
    complianceLevel?: string
    [key: string]: any
  }
  operationRoutes: OperationRoute[]
  usageCount: number
  lastUsed: string
  status: 'active' | 'inactive' | 'draft'
  createdAt: string
  updatedAt: string
}

export interface AccountUsageData {
  keyValue: string
  name: string
  usageCount: number
  percentage: number
}

export interface TransactionRoutePerformance {
  title: string
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

export interface AccountingMockData {
  accountTypes: AccountType[]
  transactionRoutes: TransactionRoute[]
  analytics: AccountingAnalytics
}

// Account Types Mock Data (15+ types with proper domain validation)
export const mockAccountTypes: AccountType[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231c',
    name: 'Checking Account',
    description: 'Standard checking account for daily transactions',
    keyValue: 'CHCK',
    domain: 'ledger',
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
    domain: 'ledger',
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
    domain: 'external',
    usageCount: 78,
    linkedAccounts: 23,
    lastUsed: '2024-12-30T16:45:00Z',
    status: 'active',
    createdAt: '2024-12-01T00:00:00Z',
    updatedAt: '2024-12-25T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    name: 'Business Checking',
    description: 'Business checking account for commercial transactions',
    keyValue: 'BUSINESS',
    domain: 'ledger',
    usageCount: 134,
    linkedAccounts: 45,
    lastUsed: '2024-12-31T14:20:00Z',
    status: 'active',
    createdAt: '2024-10-05T00:00:00Z',
    updatedAt: '2024-12-30T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2320',
    name: 'Credit Card Account',
    description: 'Credit card account for credit transactions',
    keyValue: 'CREDIT',
    domain: 'ledger',
    usageCount: 298,
    linkedAccounts: 112,
    lastUsed: '2025-01-01T09:45:00Z',
    status: 'active',
    createdAt: '2024-09-20T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2321',
    name: 'Loan Account',
    description: 'Loan account for lending operations',
    keyValue: 'LOAN',
    domain: 'ledger',
    usageCount: 67,
    linkedAccounts: 34,
    lastUsed: '2024-12-29T11:30:00Z',
    status: 'active',
    createdAt: '2024-11-01T00:00:00Z',
    updatedAt: '2024-12-22T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2322',
    name: 'Investment Account',
    description: 'Investment account for securities trading',
    keyValue: 'INVEST',
    domain: 'ledger',
    usageCount: 89,
    linkedAccounts: 28,
    lastUsed: '2024-12-30T15:20:00Z',
    status: 'active',
    createdAt: '2024-10-15T00:00:00Z',
    updatedAt: '2024-12-25T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2323',
    name: 'Money Market Account',
    description: 'High-yield money market account',
    keyValue: 'MM',
    domain: 'ledger',
    usageCount: 43,
    linkedAccounts: 19,
    lastUsed: '2024-12-28T13:15:00Z',
    status: 'active',
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-27T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2324',
    name: 'Escrow Account',
    description: 'Escrow account for third-party holdings',
    keyValue: 'ESCROW',
    domain: 'ledger',
    usageCount: 56,
    linkedAccounts: 22,
    lastUsed: '2024-12-29T16:45:00Z',
    status: 'active',
    createdAt: '2024-10-30T00:00:00Z',
    updatedAt: '2024-12-24T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2325',
    name: 'Merchant Account',
    description: 'Merchant account for payment processing',
    keyValue: 'MERCHANT',
    domain: 'external',
    usageCount: 187,
    linkedAccounts: 73,
    lastUsed: '2025-01-01T11:20:00Z',
    status: 'active',
    createdAt: '2024-09-15T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2326',
    name: 'Payroll Account',
    description: 'Dedicated payroll processing account',
    keyValue: 'PAYROLL',
    domain: 'ledger',
    usageCount: 102,
    linkedAccounts: 156,
    lastUsed: '2024-12-31T08:00:00Z',
    status: 'active',
    createdAt: '2024-08-01T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2327',
    name: 'Treasury Account',
    description: 'Corporate treasury management account',
    keyValue: 'TREASURY',
    domain: 'ledger',
    usageCount: 34,
    linkedAccounts: 12,
    lastUsed: '2024-12-30T17:30:00Z',
    status: 'active',
    createdAt: '2024-11-05T00:00:00Z',
    updatedAt: '2024-12-29T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2328',
    name: 'Fee Collection Account',
    description: 'Account for collecting service fees',
    keyValue: 'FEE_COLLECT',
    domain: 'ledger',
    usageCount: 234,
    linkedAccounts: 67,
    lastUsed: '2025-01-01T14:45:00Z',
    status: 'active',
    createdAt: '2024-09-01T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2329',
    name: 'Settlement Account',
    description: 'Account for transaction settlements',
    keyValue: 'SETTLEMENT',
    domain: 'external',
    usageCount: 145,
    linkedAccounts: 34,
    lastUsed: '2024-12-31T16:20:00Z',
    status: 'active',
    createdAt: '2024-10-10T00:00:00Z',
    updatedAt: '2024-12-30T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232a',
    name: 'Suspense Account',
    description: 'Temporary holding account for unclassified transactions',
    keyValue: 'SUSPENSE',
    domain: 'ledger',
    usageCount: 23,
    linkedAccounts: 8,
    lastUsed: '2024-12-28T10:15:00Z',
    status: 'active',
    createdAt: '2024-11-15T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232b',
    name: 'Virtual Wallet',
    description: 'Virtual wallet for digital transactions',
    keyValue: 'VIRTUAL',
    domain: 'ledger',
    usageCount: 312,
    linkedAccounts: 89,
    lastUsed: '2025-01-01T13:30:00Z',
    status: 'active',
    createdAt: '2024-08-15T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232c',
    name: 'Draft Account',
    description: 'Account type in draft status for testing',
    keyValue: 'DRAFT_TEST',
    domain: 'ledger',
    usageCount: 0,
    linkedAccounts: 0,
    lastUsed: '2024-12-15T00:00:00Z',
    status: 'draft',
    createdAt: '2024-12-15T00:00:00Z',
    updatedAt: '2024-12-15T00:00:00Z'
  }
]

// Transaction Routes Mock Data (8+ routes with operation mappings)
export const mockTransactionRoutes: TransactionRoute[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    title: 'Standard Account Transfer',
    description: 'Standard transfer between internal accounts',
    category: 'transfers',
    metadata: {
      requiresApproval: false,
      minimumAmount: 0.01,
      maximumAmount: 10000.0,
      autoValidate: true
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2320',
        type: 'source',
        account: {
          id: '01956b69-9102-75b7-8860-3e75c11d2321',
          alias: 'checking-001',
          type: ['CHCK']
        },
        metadata: {
          description: 'Debit from source checking account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2322',
        type: 'destination',
        account: {
          id: '01956b69-9102-75b7-8860-3e75c11d2323',
          alias: 'savings-001',
          type: ['SVGS']
        },
        metadata: {
          description: 'Credit to destination savings account'
        }
      }
    ],
    usageCount: 1234,
    lastUsed: '2025-01-01T14:20:00Z',
    status: 'active',
    createdAt: '2024-10-15T00:00:00Z',
    updatedAt: '2024-12-22T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2324',
    title: 'External Wire Transfer',
    description: 'Transfer to external bank account via wire',
    category: 'external_transfers',
    metadata: {
      requiresApproval: true,
      minimumAmount: 100.0,
      maximumAmount: 50000.0,
      autoValidate: false,
      complianceLevel: 'high'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2325',
        type: 'source',
        account: {
          alias: 'business-checking',
          type: ['CHCK', 'BUSINESS']
        },
        metadata: {
          description: 'Debit from business checking account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2326',
        type: 'destination',
        account: {
          type: ['EXT_BANK']
        },
        metadata: {
          description: 'Credit to external bank account'
        }
      }
    ],
    usageCount: 89,
    lastUsed: '2024-12-29T11:30:00Z',
    status: 'active',
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2327',
    title: 'Credit Card Payment',
    description: 'Payment processing for credit card transactions',
    category: 'payments',
    metadata: {
      requiresApproval: false,
      minimumAmount: 1.0,
      maximumAmount: 5000.0,
      autoValidate: true,
      complianceLevel: 'medium'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2328',
        type: 'source',
        account: {
          type: ['CREDIT']
        },
        metadata: {
          description: 'Charge to credit card account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2329',
        type: 'destination',
        account: {
          type: ['MERCHANT']
        },
        metadata: {
          description: 'Credit to merchant account'
        }
      }
    ],
    usageCount: 2456,
    lastUsed: '2025-01-01T15:45:00Z',
    status: 'active',
    createdAt: '2024-09-10T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232a',
    title: 'Payroll Distribution',
    description: 'Employee payroll distribution process',
    category: 'payroll',
    metadata: {
      requiresApproval: true,
      minimumAmount: 500.0,
      maximumAmount: 100000.0,
      autoValidate: false,
      complianceLevel: 'high'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d232b',
        type: 'source',
        account: {
          type: ['PAYROLL']
        },
        metadata: {
          description: 'Debit from payroll account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d232c',
        type: 'destination',
        account: {
          type: ['CHCK', 'SVGS']
        },
        metadata: {
          description: 'Credit to employee accounts'
        }
      }
    ],
    usageCount: 156,
    lastUsed: '2024-12-31T08:00:00Z',
    status: 'active',
    createdAt: '2024-08-01T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d232d',
    title: 'Investment Transfer',
    description: 'Transfer funds to investment accounts',
    category: 'investments',
    metadata: {
      requiresApproval: true,
      minimumAmount: 1000.0,
      maximumAmount: 1000000.0,
      autoValidate: false,
      complianceLevel: 'high'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d232e',
        type: 'source',
        account: {
          type: ['CHCK', 'SVGS']
        },
        metadata: {
          description: 'Debit from funding account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d232f',
        type: 'destination',
        account: {
          type: ['INVEST']
        },
        metadata: {
          description: 'Credit to investment account'
        }
      }
    ],
    usageCount: 234,
    lastUsed: '2024-12-30T16:30:00Z',
    status: 'active',
    createdAt: '2024-10-01T00:00:00Z',
    updatedAt: '2024-12-30T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2330',
    title: 'Fee Collection',
    description: 'Automatic fee collection process',
    category: 'fees',
    metadata: {
      requiresApproval: false,
      minimumAmount: 0.01,
      maximumAmount: 1000.0,
      autoValidate: true,
      complianceLevel: 'low'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2331',
        type: 'source',
        account: {
          type: ['CHCK', 'SVGS', 'CREDIT']
        },
        metadata: {
          description: 'Debit fee from customer account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2332',
        type: 'destination',
        account: {
          type: ['FEE_COLLECT']
        },
        metadata: {
          description: 'Credit to fee collection account'
        }
      }
    ],
    usageCount: 3421,
    lastUsed: '2025-01-01T14:45:00Z',
    status: 'active',
    createdAt: '2024-09-01T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2333',
    title: 'Settlement Processing',
    description: 'Daily settlement processing for transactions',
    category: 'settlements',
    metadata: {
      requiresApproval: false,
      minimumAmount: 0.01,
      maximumAmount: 10000000.0,
      autoValidate: true,
      complianceLevel: 'high'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2334',
        type: 'source',
        account: {
          type: ['MERCHANT', 'TREASURY']
        },
        metadata: {
          description: 'Debit from merchant/treasury account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2335',
        type: 'destination',
        account: {
          type: ['SETTLEMENT']
        },
        metadata: {
          description: 'Credit to settlement account'
        }
      }
    ],
    usageCount: 876,
    lastUsed: '2024-12-31T23:59:00Z',
    status: 'active',
    createdAt: '2024-10-10T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2336',
    title: 'Virtual Wallet Top-up',
    description: 'Top-up virtual wallet from bank account',
    category: 'digital_payments',
    metadata: {
      requiresApproval: false,
      minimumAmount: 5.0,
      maximumAmount: 2000.0,
      autoValidate: true,
      complianceLevel: 'medium'
    },
    operationRoutes: [
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2337',
        type: 'source',
        account: {
          type: ['CHCK', 'SVGS']
        },
        metadata: {
          description: 'Debit from bank account'
        }
      },
      {
        id: '01956b69-9102-75b7-8860-3e75c11d2338',
        type: 'destination',
        account: {
          type: ['VIRTUAL']
        },
        metadata: {
          description: 'Credit to virtual wallet'
        }
      }
    ],
    usageCount: 1876,
    lastUsed: '2025-01-01T13:30:00Z',
    status: 'active',
    createdAt: '2024-08-15T00:00:00Z',
    updatedAt: '2024-12-31T00:00:00Z'
  }
]

// Comprehensive Analytics Mock Data
export const mockAnalyticsData: AccountingAnalytics = {
  overview: {
    totalAccountTypes: 17,
    activeAccountTypes: 16,
    totalTransactionRoutes: 8,
    activeTransactionRoutes: 8,
    totalOperationRoutes: 16,
    monthlyUsage: 9876,
    complianceScore: 96.8
  },
  accountTypeUsage: [
    {
      keyValue: 'FEE_COLLECT',
      name: 'Fee Collection Account',
      usageCount: 3421,
      percentage: 34.7
    },
    {
      keyValue: 'CREDIT',
      name: 'Credit Card Account',
      usageCount: 2456,
      percentage: 24.9
    },
    {
      keyValue: 'VIRTUAL',
      name: 'Virtual Wallet',
      usageCount: 1876,
      percentage: 19.0
    },
    {
      keyValue: 'CHCK',
      name: 'Checking Account',
      usageCount: 1234,
      percentage: 12.5
    },
    {
      keyValue: 'SETTLEMENT',
      name: 'Settlement Account',
      usageCount: 876,
      percentage: 8.9
    }
  ],
  transactionRoutePerformance: [
    {
      title: 'Fee Collection',
      usageCount: 3421,
      successRate: 99.9,
      avgProcessingTime: '0.8s'
    },
    {
      title: 'Credit Card Payment',
      usageCount: 2456,
      successRate: 99.6,
      avgProcessingTime: '1.1s'
    },
    {
      title: 'Virtual Wallet Top-up',
      usageCount: 1876,
      successRate: 99.8,
      avgProcessingTime: '1.0s'
    },
    {
      title: 'Standard Account Transfer',
      usageCount: 1234,
      successRate: 99.8,
      avgProcessingTime: '1.2s'
    },
    {
      title: 'Settlement Processing',
      usageCount: 876,
      successRate: 99.5,
      avgProcessingTime: '2.1s'
    },
    {
      title: 'Investment Transfer',
      usageCount: 234,
      successRate: 98.9,
      avgProcessingTime: '3.4s'
    },
    {
      title: 'Payroll Distribution',
      usageCount: 156,
      successRate: 99.2,
      avgProcessingTime: '2.8s'
    },
    {
      title: 'External Wire Transfer',
      usageCount: 89,
      successRate: 98.9,
      avgProcessingTime: '5.2s'
    }
  ],
  complianceTrends: [
    { date: '2024-11-01', score: 92.1, violations: 8 },
    { date: '2024-11-15', score: 94.3, violations: 5 },
    { date: '2024-12-01', score: 95.2, violations: 4 },
    { date: '2024-12-15', score: 96.5, violations: 2 },
    { date: '2025-01-01', score: 96.8, violations: 1 }
  ],
  auditTrail: [
    {
      id: 'audit-001',
      timestamp: '2025-01-01T14:30:00Z',
      user: 'john.doe@midaz.com',
      action: 'UPDATE_ACCOUNT_TYPE',
      resource: 'AccountType',
      resourceId: '01956b69-9102-75b7-8860-3e75c11d231c',
      details: 'Updated description for CHCK account type',
      ipAddress: '192.168.1.100',
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
    },
    {
      id: 'audit-002',
      timestamp: '2025-01-01T13:45:00Z',
      user: 'jane.smith@midaz.com',
      action: 'CREATE_TRANSACTION_ROUTE',
      resource: 'TransactionRoute',
      resourceId: '01956b69-9102-75b7-8860-3e75c11d2336',
      details: 'Created new Virtual Wallet Top-up transaction route',
      ipAddress: '192.168.1.101',
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)'
    },
    {
      id: 'audit-003',
      timestamp: '2025-01-01T12:20:00Z',
      user: 'admin@midaz.com',
      action: 'APPROVE_TRANSACTION_ROUTE',
      resource: 'TransactionRoute',
      resourceId: '01956b69-9102-75b7-8860-3e75c11d232a',
      details: 'Approved payroll distribution route for production use',
      ipAddress: '192.168.1.102',
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
    },
    {
      id: 'audit-004',
      timestamp: '2025-01-01T11:15:00Z',
      user: 'compliance@midaz.com',
      action: 'VALIDATE_OPERATION_ROUTE',
      resource: 'OperationRoute',
      resourceId: '01956b69-9102-75b7-8860-3e75c11d2325',
      details: 'Validated compliance for external wire transfer operation',
      ipAddress: '192.168.1.103',
      userAgent: 'Mozilla/5.0 (Ubuntu; Linux x86_64)'
    },
    {
      id: 'audit-005',
      timestamp: '2025-01-01T10:30:00Z',
      user: 'treasury@midaz.com',
      action: 'UPDATE_OPERATION_ROUTE',
      resource: 'OperationRoute',
      resourceId: '01956b69-9102-75b7-8860-3e75c11d232e',
      details: 'Updated investment transfer operation limits',
      ipAddress: '192.168.1.104',
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
    }
  ]
}

// Main export combining all mock data
export const accountingMockData: AccountingMockData = {
  accountTypes: mockAccountTypes,
  transactionRoutes: mockTransactionRoutes,
  analytics: mockAnalyticsData
}

// Utility functions for mock data manipulation
export const getAccountTypeByKeyValue = (
  keyValue: string
): AccountType | undefined => {
  return mockAccountTypes.find((type) => type.keyValue === keyValue)
}

export const getTransactionRouteById = (
  id: string
): TransactionRoute | undefined => {
  return mockTransactionRoutes.find((route) => route.id === id)
}

export const getActiveAccountTypes = (): AccountType[] => {
  return mockAccountTypes.filter((type) => type.status === 'active')
}

export const getActiveTransactionRoutes = (): TransactionRoute[] => {
  return mockTransactionRoutes.filter((route) => route.status === 'active')
}

export const getAccountTypesByDomain = (
  domain: 'ledger' | 'external'
): AccountType[] => {
  return mockAccountTypes.filter((type) => type.domain === domain)
}

export const getTransactionRoutesByCategory = (
  category: string
): TransactionRoute[] => {
  return mockTransactionRoutes.filter((route) => route.category === category)
}

export const generateMockAuditEntry = (
  user: string,
  action: string,
  resource: string,
  resourceId: string,
  details: string
): AuditTrailEntry => {
  return {
    id: `audit-${Date.now()}`,
    timestamp: new Date().toISOString(),
    user,
    action,
    resource,
    resourceId,
    details,
    ipAddress: '192.168.1.100',
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
  }
}
