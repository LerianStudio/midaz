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
  operationType: 'source' | 'destination' | 'fee'
  sourceAccountAlias?: string // For fee operations, tracks which source account paid the fee
  isFee: boolean
  feeType?: 'deductible' | 'non-deductible'
  originalAmount?: string // Original amount before any modifications
}

/**
 * Represents a source-to-destination flow
 */
export interface TransactionFlow {
  flowId: string
  sourceOperation: EnhancedTransactionOperation
  destinationOperations: EnhancedTransactionOperation[]
  feeOperations: EnhancedTransactionOperation[]

  sourceAmount: string
  destinationTotalAmount: string
  feeTotalAmount: string

  isSimpleFlow: boolean // true for 1:1 flows
  hasDeductibleFees: boolean
  hasNonDeductibleFees: boolean
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
    totalFeeAmount: string
    totalDeductibleFees: string
    totalNonDeductibleFees: string
    uniqueSourceAccounts: string[]
    uniqueDestinationAccounts: string[]
    uniqueFeeAccounts: string[]
  }

  feeCalculation?: {
    packageId?: string
    packageLabel?: string
    isDeductibleFrom: boolean
    appliedFees: Array<{
      feeId: string
      feeLabel: string
      amount: string
      creditAccount: string
      isDeductibleFrom: boolean
      sourceAccount?: string // Which source account this fee applies to
    }>
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
  mapFromFeeCalculation(
    feeCalculation: any,
    originalFormData: any
  ): TransactionDisplayData
  mapFromTransaction(transaction: any): TransactionDisplayData
}

/**
 * Transaction validation result with display context
 */
export interface TransactionDisplayValidation {
  isValid: boolean
  errors: Array<{
    type: 'balance' | 'fee' | 'relationship' | 'data'
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
