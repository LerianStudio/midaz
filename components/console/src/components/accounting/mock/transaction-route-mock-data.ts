// Mock data for transaction routes and operation routes

export interface AccountType {
  id: string
  name: string
  code: string
  nature: 'debit' | 'credit'
  category: 'asset' | 'liability' | 'equity' | 'revenue' | 'expense'
  domain: 'customer' | 'provider' | 'system'
  description: string
}

export interface TransactionRoute {
  id: string
  name: string
  description: string
  templateType:
    | 'transfer'
    | 'payment'
    | 'adjustment'
    | 'fee'
    | 'refund'
    | 'custom'
  status: 'active' | 'draft' | 'deprecated'
  createdAt: string
  updatedAt: string
  operationRoutes: OperationRoute[]
  metadata: Record<string, any>
  version: string
  tags: string[]
}

export interface OperationRoute {
  id: string
  transactionRouteId: string
  operationType: 'debit' | 'credit'
  sourceAccountTypeId: string
  destinationAccountTypeId: string
  sourceAccountType?: AccountType
  destinationAccountType?: AccountType
  amount: {
    expression: string // e.g., "{{amount}}", "{{amount}} * 0.03"
    description: string
  }
  description: string
  order: number
  conditions?: {
    field: string
    operator: 'equals' | 'greater_than' | 'less_than' | 'contains'
    value: string
  }[]
}

export interface RouteTemplate {
  id: string
  name: string
  description: string
  category: 'transfer' | 'payment' | 'adjustment' | 'fee' | 'refund'
  icon: string
  operationRoutes: Omit<OperationRoute, 'id' | 'transactionRouteId'>[]
  metadata: Record<string, any>
  tags: string[]
}

// Sample account types
export const mockAccountTypes: AccountType[] = [
  {
    id: 'at-001',
    name: 'Customer Checking Account',
    code: 'CUST_CHECKING',
    nature: 'debit',
    category: 'asset',
    domain: 'customer',
    description: 'Primary checking accounts for customers'
  },
  {
    id: 'at-002',
    name: 'Customer Savings Account',
    code: 'CUST_SAVINGS',
    nature: 'debit',
    category: 'asset',
    domain: 'customer',
    description: 'Savings accounts for customers'
  },
  {
    id: 'at-003',
    name: 'Merchant Settlement Account',
    code: 'MERCHANT_SETTLEMENT',
    nature: 'credit',
    category: 'liability',
    domain: 'provider',
    description: 'Accounts for merchant settlement funds'
  },
  {
    id: 'at-004',
    name: 'Fee Revenue Account',
    code: 'FEE_REVENUE',
    nature: 'credit',
    category: 'revenue',
    domain: 'system',
    description: 'Account to track fee revenue'
  },
  {
    id: 'at-005',
    name: 'Processing Fee Account',
    code: 'PROCESSING_FEE',
    nature: 'debit',
    category: 'expense',
    domain: 'system',
    description: 'Account for processing fees'
  },
  {
    id: 'at-006',
    name: 'Suspense Account',
    code: 'SUSPENSE',
    nature: 'debit',
    category: 'asset',
    domain: 'system',
    description: 'Temporary holding account for unmatched transactions'
  }
]

// Sample route templates
export const mockRouteTemplates: RouteTemplate[] = [
  {
    id: 'template-001',
    name: 'Simple Transfer',
    description: 'Basic transfer between two customer accounts',
    category: 'transfer',
    icon: 'ArrowRightLeft',
    operationRoutes: [
      {
        operationType: 'debit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-001',
        amount: {
          expression: '{{amount}}',
          description: 'Full transaction amount'
        },
        description: 'Debit source account',
        order: 1
      },
      {
        operationType: 'credit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-001',
        amount: {
          expression: '{{amount}}',
          description: 'Full transaction amount'
        },
        description: 'Credit destination account',
        order: 2
      }
    ],
    metadata: {
      requiresKyc: false,
      dailyLimit: 10000,
      monthlyLimit: 100000
    },
    tags: ['basic', 'transfer', 'p2p']
  },
  {
    id: 'template-002',
    name: 'Payment with Fee',
    description: 'Payment transaction with processing fee',
    category: 'payment',
    icon: 'CreditCard',
    operationRoutes: [
      {
        operationType: 'debit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-003',
        amount: {
          expression: '{{amount}}',
          description: 'Payment amount'
        },
        description: 'Debit customer account',
        order: 1
      },
      {
        operationType: 'credit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-003',
        amount: {
          expression: '{{amount}}',
          description: 'Payment amount'
        },
        description: 'Credit merchant account',
        order: 2
      },
      {
        operationType: 'debit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-004',
        amount: {
          expression: '{{amount}} * 0.03',
          description: '3% processing fee'
        },
        description: 'Debit processing fee from customer',
        order: 3
      },
      {
        operationType: 'credit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-004',
        amount: {
          expression: '{{amount}} * 0.03',
          description: '3% processing fee'
        },
        description: 'Credit fee revenue account',
        order: 4
      }
    ],
    metadata: {
      requiresKyc: true,
      feePercentage: 3,
      maxFeeAmount: 50
    },
    tags: ['payment', 'fee', 'merchant']
  },
  {
    id: 'template-003',
    name: 'Balance Adjustment',
    description: 'Manual balance adjustment for corrections',
    category: 'adjustment',
    icon: 'Settings',
    operationRoutes: [
      {
        operationType: 'debit',
        sourceAccountTypeId: 'at-006',
        destinationAccountTypeId: 'at-001',
        amount: {
          expression: '{{amount}}',
          description: 'Adjustment amount'
        },
        description: 'Debit suspense account',
        order: 1
      },
      {
        operationType: 'credit',
        sourceAccountTypeId: 'at-006',
        destinationAccountTypeId: 'at-001',
        amount: {
          expression: '{{amount}}',
          description: 'Adjustment amount'
        },
        description: 'Credit customer account',
        order: 2
      }
    ],
    metadata: {
      requiresApproval: true,
      maxAdjustmentAmount: 1000,
      auditRequired: true
    },
    tags: ['adjustment', 'manual', 'correction']
  }
]

// Sample transaction routes
export const mockTransactionRoutes: TransactionRoute[] = [
  {
    id: 'tr-001',
    name: 'Customer Transfer Route',
    description: 'Standard route for customer-to-customer transfers',
    templateType: 'transfer',
    status: 'active',
    createdAt: '2024-01-15T08:00:00Z',
    updatedAt: '2024-01-20T10:30:00Z',
    version: '1.2.0',
    tags: ['customer', 'transfer', 'p2p'],
    metadata: {
      requiresKyc: false,
      dailyLimit: 10000,
      monthlyLimit: 100000,
      supportedCurrencies: ['USD', 'BRL', 'EUR']
    },
    operationRoutes: [
      {
        id: 'or-001',
        transactionRouteId: 'tr-001',
        operationType: 'debit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-001',
        sourceAccountType: mockAccountTypes[0],
        destinationAccountType: mockAccountTypes[0],
        amount: {
          expression: '{{amount}}',
          description: 'Full transaction amount'
        },
        description: 'Debit source customer account',
        order: 1
      },
      {
        id: 'or-002',
        transactionRouteId: 'tr-001',
        operationType: 'credit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-001',
        sourceAccountType: mockAccountTypes[0],
        destinationAccountType: mockAccountTypes[0],
        amount: {
          expression: '{{amount}}',
          description: 'Full transaction amount'
        },
        description: 'Credit destination customer account',
        order: 2
      }
    ]
  },
  {
    id: 'tr-002',
    name: 'Merchant Payment Route',
    description: 'Payment processing route with fees for merchant transactions',
    templateType: 'payment',
    status: 'active',
    createdAt: '2024-01-10T09:15:00Z',
    updatedAt: '2024-01-25T14:20:00Z',
    version: '2.1.0',
    tags: ['merchant', 'payment', 'fee'],
    metadata: {
      requiresKyc: true,
      feePercentage: 2.9,
      maxFeeAmount: 50,
      supportedCurrencies: ['USD', 'BRL']
    },
    operationRoutes: [
      {
        id: 'or-003',
        transactionRouteId: 'tr-002',
        operationType: 'debit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-003',
        sourceAccountType: mockAccountTypes[0],
        destinationAccountType: mockAccountTypes[2],
        amount: {
          expression: '{{amount}}',
          description: 'Payment amount'
        },
        description: 'Debit customer account for payment',
        order: 1
      },
      {
        id: 'or-004',
        transactionRouteId: 'tr-002',
        operationType: 'credit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-003',
        sourceAccountType: mockAccountTypes[0],
        destinationAccountType: mockAccountTypes[2],
        amount: {
          expression: '{{amount}}',
          description: 'Payment amount'
        },
        description: 'Credit merchant settlement account',
        order: 2
      },
      {
        id: 'or-005',
        transactionRouteId: 'tr-002',
        operationType: 'debit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-004',
        sourceAccountType: mockAccountTypes[0],
        destinationAccountType: mockAccountTypes[3],
        amount: {
          expression: '{{amount}} * 0.029',
          description: '2.9% processing fee'
        },
        description: 'Debit processing fee from customer',
        order: 3,
        conditions: [
          {
            field: 'amount',
            operator: 'greater_than',
            value: '10'
          }
        ]
      },
      {
        id: 'or-006',
        transactionRouteId: 'tr-002',
        operationType: 'credit',
        sourceAccountTypeId: 'at-001',
        destinationAccountTypeId: 'at-004',
        sourceAccountType: mockAccountTypes[0],
        destinationAccountType: mockAccountTypes[3],
        amount: {
          expression: '{{amount}} * 0.029',
          description: '2.9% processing fee'
        },
        description: 'Credit fee revenue account',
        order: 4,
        conditions: [
          {
            field: 'amount',
            operator: 'greater_than',
            value: '10'
          }
        ]
      }
    ]
  },
  {
    id: 'tr-003',
    name: 'Balance Adjustment Route',
    description: 'Manual balance adjustments and corrections',
    templateType: 'adjustment',
    status: 'draft',
    createdAt: '2024-01-25T11:00:00Z',
    updatedAt: '2024-01-25T11:00:00Z',
    version: '1.0.0',
    tags: ['adjustment', 'manual', 'admin'],
    metadata: {
      requiresApproval: true,
      maxAdjustmentAmount: 1000,
      approvalLevels: 2,
      auditRequired: true
    },
    operationRoutes: [
      {
        id: 'or-007',
        transactionRouteId: 'tr-003',
        operationType: 'debit',
        sourceAccountTypeId: 'at-006',
        destinationAccountTypeId: 'at-001',
        sourceAccountType: mockAccountTypes[5],
        destinationAccountType: mockAccountTypes[0],
        amount: {
          expression: '{{adjustment_amount}}',
          description: 'Adjustment amount'
        },
        description: 'Debit suspense account',
        order: 1
      },
      {
        id: 'or-008',
        transactionRouteId: 'tr-003',
        operationType: 'credit',
        sourceAccountTypeId: 'at-006',
        destinationAccountTypeId: 'at-001',
        sourceAccountType: mockAccountTypes[5],
        destinationAccountType: mockAccountTypes[0],
        amount: {
          expression: '{{adjustment_amount}}',
          description: 'Adjustment amount'
        },
        description: 'Credit customer account',
        order: 2
      }
    ]
  }
]

// Sample operation routes for standalone management
export const mockOperationRoutes: OperationRoute[] = [
  ...mockTransactionRoutes.flatMap((tr) => tr.operationRoutes)
]

// Helper functions
export const getTransactionRouteById = (
  id: string
): TransactionRoute | undefined => {
  return mockTransactionRoutes.find((route) => route.id === id)
}

export const getOperationRouteById = (
  id: string
): OperationRoute | undefined => {
  return mockOperationRoutes.find((route) => route.id === id)
}

export const getAccountTypeById = (id: string): AccountType | undefined => {
  return mockAccountTypes.find((accountType) => accountType.id === id)
}

export const getRouteTemplateById = (id: string): RouteTemplate | undefined => {
  return mockRouteTemplates.find((template) => template.id === id)
}

export const getRouteTemplatesByCategory = (
  category: string
): RouteTemplate[] => {
  return mockRouteTemplates.filter((template) => template.category === category)
}
