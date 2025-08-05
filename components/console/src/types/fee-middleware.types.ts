import {
  FeeApiCalculateRequest,
  FeeApiCalculateResponse
} from './fee-api.types'
import { AppliedFee } from './fee-calculation.types'

export interface FeeValidationResult {
  isValid: boolean
  errors: string[]
  warnings: string[]
  validatedResponse: FeeApiCalculateResponse
}

export interface FeeValidationContext {
  originalRequest: FeeApiCalculateRequest
  feeResponse: FeeApiCalculateResponse
  validationRules?: FeeValidationRules
}

export interface FeeValidationRules {
  allowNegativeFees: boolean
  maxFeePercentage: number
  requireFeeMetadata: boolean
  validateAccountExistence: boolean
}

export interface FeeOperation {
  operationId: string
  feeId: string
  feeLabel: string
  amount: {
    asset: string
    value: string
  }
  sourceAccount: string
  destinationAccount: string
  priority: number
  isDeductibleFrom: boolean
  metadata?: Record<string, unknown>
}

export interface TransactionAccount {
  accountId: string
  accountAlias?: string
  chartOfAccounts: string
  balance?: {
    asset: string
    value: string
  }
  metadata?: Record<string, unknown>
}

export interface FeeStateExtraction {
  originalAmount: string
  totalFees: string
  appliedFees: AppliedFee[]
  deductibleFees: AppliedFee[]
  nonDeductibleFees: AppliedFee[]
  afterFeesAmount: string
  feeOperations: FeeOperation[]
  warnings: string[]
  errors: string[]
}

export interface FeeCalculationContext {
  transactionAmount: string
  asset: string
  sourceAccounts: TransactionAccount[]
  destinationAccounts: TransactionAccount[]
  appliedFeeRules: any[]
  metadata: Record<string, unknown>
}

export interface FeeMiddlewareConfig {
  enableRefactoredValidation: boolean
  enableStrictValidation: boolean
  logLevel: 'debug' | 'info' | 'warn' | 'error'
  validationRules: FeeValidationRules
}

export interface FeeTransformationResult {
  success: boolean
  data?: FeeApiCalculateResponse
  error?: {
    code: string
    message: string
    details?: Record<string, unknown>
  }
}

export function isFeeOperation(obj: unknown): obj is FeeOperation {
  return (
    typeof obj === 'object' &&
    obj !== null &&
    'operationId' in obj &&
    'feeId' in obj &&
    'amount' in obj
  )
}

export function isTransactionAccount(obj: unknown): obj is TransactionAccount {
  return (
    typeof obj === 'object' &&
    obj !== null &&
    'accountId' in obj &&
    'chartOfAccounts' in obj
  )
}

export function isFeeValidationResult(
  obj: unknown
): obj is FeeValidationResult {
  return (
    typeof obj === 'object' &&
    obj !== null &&
    'isValid' in obj &&
    'errors' in obj &&
    Array.isArray((obj as FeeValidationResult).errors)
  )
}
