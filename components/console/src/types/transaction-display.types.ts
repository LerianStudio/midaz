/**
 * Represents a single operation in a transaction
 */
export interface TransactionOperation {
  accountAlias: string
  asset: string
  amount: string
  description?: string
  chartOfAccounts?: Record<string, string>
  metadata?: Record<string, any>
}

/**
 * Enhanced operation with display metadata
 */
export interface EnhancedTransactionOperation extends TransactionOperation {
  operationId: string // Unique identifier for the operation
  operationType: 'source' | 'destination'
  originalAmount?: string // Original amount before any modifications
}

/**
 * Represents a source-to-destination flow
 */
export interface TransactionFlow {
  flowId: string
  sourceOperation: EnhancedTransactionOperation
  destinationOperations: EnhancedTransactionOperation[]

  sourceAmount: string
  destinationTotalAmount: string

  isSimpleFlow: boolean // true for 1:1 flows
}

/**
 * Complete transaction display structure
 */
export interface TransactionDisplayData {
  transactionId?: string
  description?: string
  chartOfAccountsGroupName?: string
  asset: string
  originalAmount: string
  metadata: Record<string, any>

  flows: TransactionFlow[]

  summary: {
    totalSourceAmount: string
    totalDestinationAmount: string
    uniqueSourceAccounts: string[]
    uniqueDestinationAccounts: string[]
  }

  displayMode: 'simple' | 'complex' // simple for 1:1, complex for N:N
  hasWarnings: boolean
  warnings: string[]
}

/**
 * Maps raw transaction data to display structure
 */
export interface TransactionDisplayMapper {
  mapFromFormData(formData: any): TransactionDisplayData
  mapFromTransaction(transaction: any): TransactionDisplayData
}

/**
 * Transaction validation result with display context
 */
export interface TransactionDisplayValidation {
  isValid: boolean
  errors: Array<{
    type: 'balance' | 'relationship' | 'data'
    message: string
    affectedAccounts?: string[]
  }>
  warnings: Array<{
    type: 'display' | 'calculation' | 'merge'
    message: string
    suggestion?: string
  }>
  displayData?: TransactionDisplayData
}
