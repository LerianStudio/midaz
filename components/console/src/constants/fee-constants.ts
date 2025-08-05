export const FEE_ERROR_CODES = {
  PRIORITY_1_REFERENCE: '0148',
  PRIORITY_GREATER_1_REFERENCE: '0149',
  PRIORITY_OPERATION_ERROR: '0150',
  INVALID_FEE_STRUCTURE: '0151',
  CALCULATION_ERROR: '0152',
  VALIDATION_ERROR: '0153'
} as const

export const FEE_FIELD_NAMES = {
  ORIGINAL_AMOUNT: 'originalAmount',
  AFTER_FEES_AMOUNT: 'afterFeesAmount',
  TOTAL_FEES: 'totalFees',
  APPLIED_FEES: 'appliedFees',
  FEE_OPERATIONS: 'feeOperations',
  DEDUCTIBLE_FEES: 'deductibleFees',
  NON_DEDUCTIBLE_FEES: 'nonDeductibleFees'
} as const

export const FEE_OPERATION_TYPES = {
  FEE: 'fee',
  TRANSACTION: 'transaction',
  DEDUCTIBLE: 'deductible',
  NON_DEDUCTIBLE: 'non_deductible'
} as const

export const FEE_CALCULATION_LIMITS = {
  MAX_FEE_PERCENTAGE: 100,
  MIN_FEE_AMOUNT: 0,
  MAX_NESTED_FEES: 10,
  DEFAULT_PRECISION: 2
} as const

export const FEE_STATUS = {
  PENDING: 'pending',
  CALCULATED: 'calculated',
  APPLIED: 'applied',
  FAILED: 'failed',
  INVALID: 'invalid'
} as const

export const LOG_PREFIXES = {
  FEE_BREAKDOWN: '[FeeBreakdown]',
  FEE_CALCULATION: '[FeeCalculation]',
  FEE_MIDDLEWARE: '[FeeMiddleware]',
  FEE_VALIDATION: '[FeeValidation]',
  FEE_TRANSFORMER: '[FeeTransformer]'
} as const

export type FeeErrorCode =
  (typeof FEE_ERROR_CODES)[keyof typeof FEE_ERROR_CODES]
export type FeeFieldName =
  (typeof FEE_FIELD_NAMES)[keyof typeof FEE_FIELD_NAMES]
export type FeeOperationType =
  (typeof FEE_OPERATION_TYPES)[keyof typeof FEE_OPERATION_TYPES]
export type FeeStatus = (typeof FEE_STATUS)[keyof typeof FEE_STATUS]
