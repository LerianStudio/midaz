/**
 * Domain types for fee calculations
 * These are clean domain models independent of external APIs
 */

/**
 * Context for fee calculations
 */
export interface FeeCalculationContext {
  organizationId: string
  ledgerId: string
  segmentId?: string
  userId?: string
  correlationId?: string
}

/**
 * Fee calculation request
 */
export interface FeeCalculationRequest {
  transaction: FeeTransaction
  metadata?: Record<string, any>
}

/**
 * Transaction structure for fee calculation
 */
export interface FeeTransaction {
  description: string
  chartOfAccountsGroupName?: string
  route?: string
  send: {
    asset: string
    value: string
    source: {
      from: AccountOperation[]
    }
    distribute: {
      to: AccountOperation[]
    }
  }
  metadata?: Record<string, any>
}

/**
 * Account operation in a transaction
 */
export interface AccountOperation {
  accountAlias: string
  amount: {
    asset: string
    value: string
  }
  metadata?: Record<string, any>
}

/**
 * Fee calculation response
 */
export interface FeeCalculationResponse {
  success: boolean
  transaction?: FeeTransactionWithFees
  fees?: AppliedFee[]
  totalFees?: MonetaryAmount
  netAmount?: MonetaryAmount
  originalAmount?: MonetaryAmount
  feesApplied: boolean
  message?: string
  packageId?: string
  errors?: FeeError[]
}

/**
 * Transaction with applied fees
 */
export interface FeeTransactionWithFees extends FeeTransaction {
  feeRules?: FeeRule[]
  isDeductibleFrom?: boolean
}

/**
 * Applied fee information
 */
export interface AppliedFee {
  feeId: string
  feeLabel: string
  calculatedAmount: number
  isDeductibleFrom: boolean
  creditAccount: string
  priority: number
  type: 'fixed' | 'percentage' | 'tiered'
  metadata?: Record<string, any>
}

/**
 * Fee rule definition
 */
export interface FeeRule {
  feeId: string
  feeLabel: string
  isDeductibleFrom: boolean
  creditAccount: string
  priority: number
  referenceAmount?: string
  applicationRule?: string
  calculations?: FeeCalculation[]
}

/**
 * Fee calculation details
 */
export interface FeeCalculation {
  type: 'percentage' | 'fixed' | 'tiered'
  value: string | number
  tier?: {
    min: number
    max: number
  }
}

/**
 * Monetary amount
 */
export interface MonetaryAmount {
  value: string
  asset: string
}

/**
 * Fee package details
 */
export interface FeePackageDetails {
  id: string
  name: string
  description: string
  status: 'active' | 'inactive' | 'draft'
  fees: Record<string, FeeRule>
  metadata?: Record<string, any>
  createdAt: Date
  updatedAt: Date
}

/**
 * Fee error information
 */
export interface FeeError {
  code: string
  field?: string
  message: string
  severity: 'error' | 'warning'
}

/**
 * Fee service configuration
 */
export interface FeeServiceConfig {
  enabled: boolean
  baseUrl?: string
  pluginUrl?: string
  timeout?: number
  retryAttempts?: number
  retryDelay?: number
  circuitBreakerThreshold?: number
  cacheEnabled?: boolean
  cacheTTL?: number
}

/**
 * Fee service errors
 */
export class FeeServiceError extends Error {
  constructor(
    message: string,
    public code: string,
    public statusCode?: number,
    public details?: any
  ) {
    super(message)
    this.name = 'FeeServiceError'
  }
}

export class FeeServiceUnavailableError extends FeeServiceError {
  constructor(message?: string) {
    super(
      message || 'Fee service is currently unavailable',
      'FEE_SERVICE_UNAVAILABLE',
      503
    )
    this.name = 'FeeServiceUnavailableError'
  }
}

export class FeeConfigurationError extends FeeServiceError {
  constructor(message: string) {
    super(message, 'FEE_CONFIGURATION_ERROR', 500)
    this.name = 'FeeConfigurationError'
  }
}

export class FeeValidationError extends FeeServiceError {
  constructor(message: string, details?: any) {
    super(message, 'FEE_VALIDATION_ERROR', 400, details)
    this.name = 'FeeValidationError'
  }
}
