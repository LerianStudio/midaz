/**
 * Asset and value structure for amounts
 */
export interface FeeEngineAmount {
  asset: string
  value: string
}

/**
 * Share configuration for percentage-based distribution
 */
export interface FeeEngineShare {
  percentage: number
}

/**
 * Account configuration for source or destination
 * Supports both percentage-based shares and fixed amounts
 */
export interface FeeEngineAccountEntry {
  accountAlias: string
  share?: FeeEngineShare
  amount?: FeeEngineAmount
  metadata?: Record<string, any>
  route?: string
}

/**
 * Source configuration with accounts
 */
export interface FeeEngineSource {
  from: FeeEngineAccountEntry[]
}

/**
 * Distribution configuration with accounts
 */
export interface FeeEngineDistribute {
  to: FeeEngineAccountEntry[]
}

/**
 * Send configuration containing the transaction details
 */
export interface FeeEngineSend {
  asset: string
  value: string
  source: FeeEngineSource
  distribute: FeeEngineDistribute
}

/**
 * Complete transaction structure for Fee Engine
 */
export interface FeeEngineTransaction {
  route?: string
  description: string
  chartOfAccountsGroupName?: string
  send: FeeEngineSend
  metadata?: Record<string, any>
}

/**
 * Fee calculation request structure
 * Note: segmentId is OPTIONAL per RFC
 */
export interface FeeCalculationRequest {
  segmentId?: string // OPTIONAL
  ledgerId: string
  transaction: FeeEngineTransaction
}

/**
 * Fee entry in calculation response
 */
export interface FeeEngineFeeEntry {
  id: string
  name: string
  description: string
  type: 'fixed' | 'percentage' | 'tiered'
  amount: FeeEngineAmount
  appliedTo: 'source' | 'destination' | 'transaction'
  calculatedFrom: string
  metadata?: Record<string, any>
}

/**
 * Successful fee calculation response
 */
export interface FeeCalculationSuccess {
  segmentId?: string
  ledgerId: string
  transaction: FeeEngineTransaction // Transaction with fees applied
  fees?: FeeEngineFeeEntry[]
  totalFees?: FeeEngineAmount
  netAmount?: FeeEngineAmount
  originalAmount?: FeeEngineAmount
  calculatedAt?: string
}

/**
 * No fees applicable response
 */
export interface FeeCalculationNoFees {
  feesApplied: []
  message: string
}

/**
 * Gratuity response (fees waived)
 */
export interface FeeCalculationGratuity {
  feesApplied: []
  message: string
}

/**
 * Error response structure
 */
export interface FeeCalculationError {
  error: {
    code: string
    message: string
    details?: Array<{
      code: string
      field: string
      message: string
      severity: 'error' | 'warning'
    }>
  }
}

/**
 * Union type for all possible fee calculation responses
 */
export type FeeCalculationResponse =
  | FeeCalculationSuccess
  | FeeCalculationNoFees
  | FeeCalculationGratuity
  | FeeCalculationError

/**
 * Type guards for response types
 */
export function isSuccessResponse(
  response: FeeCalculationResponse
): response is FeeCalculationSuccess {
  return 'transaction' in response && !('error' in response)
}

export function isNoFeesResponse(
  response: FeeCalculationResponse
): response is FeeCalculationNoFees {
  return (
    'feesApplied' in response &&
    Array.isArray(response.feesApplied) &&
    response.feesApplied.length === 0 &&
    'message' in response
  )
}

export function isGratuityResponse(
  response: FeeCalculationResponse
): response is FeeCalculationGratuity {
  return (
    'feesApplied' in response &&
    Array.isArray(response.feesApplied) &&
    response.feesApplied.length === 0 &&
    'message' in response &&
    response.message.toLowerCase().includes('gratuity')
  )
}

export function isErrorResponse(
  response: FeeCalculationResponse
): response is FeeCalculationError {
  return 'error' in response
}

/**
 * Console transaction format for frontend
 * Supports both simple and complex transaction modes
 */
export interface ConsoleTransaction {
  description: string
  asset: string
  value: string
  simple?: {
    from: string // Single account alias
    to: string // Single account alias
  }
  complex?: {
    source: Array<{
      accountAlias: string
      value?: string
      percentage?: number
    }>
    destination: Array<{
      accountAlias: string
      value?: string
      percentage?: number
    }>
  }
  metadata?: {
    route?: string
    segmentId?: string
    [key: string]: any
  }
}
