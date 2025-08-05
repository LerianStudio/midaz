/**
 * Share configuration for account distribution
 */
export interface FeeEngineShare {
  percentage: number
}

/**
 * Account reference with share configuration
 */
export interface FeeEngineAccount {
  accountAlias: string
  share: FeeEngineShare
}

/**
 * Source configuration for transaction
 */
export interface FeeEngineSource {
  from: FeeEngineAccount[]
}

/**
 * Distribution configuration for transaction
 */
export interface FeeEngineDistribute {
  to: FeeEngineAccount[]
}

/**
 * Send configuration containing asset, value and distribution
 */
export interface FeeEngineSend {
  asset: string
  value: string
  source: FeeEngineSource
  distribute: FeeEngineDistribute
}

/**
 * Transaction structure for Fee Engine
 */
export interface FeeEngineTransaction {
  route: string
  description: string
  send: FeeEngineSend
}

/**
 * Main request structure for Fee Engine API
 */
export interface FeeEngineCalculateRequest {
  segmentId: string
  ledgerId: string
  transaction: FeeEngineTransaction
}

/**
 * Fee amount structure in responses
 */
export interface FeeEngineAmount {
  value: string
  asset: string
}

/**
 * Individual fee entry in response
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
 * Fee calculation response from Fee Engine
 */
export interface FeeEngineCalculateResponse {
  transactionId?: string
  fees?: FeeEngineFeeEntry[]
  totalFees?: FeeEngineAmount
  netAmount?: FeeEngineAmount
  originalAmount?: FeeEngineAmount
  feesApplied: boolean | any[] // Can be boolean or empty array based on RFC
  message?: string
  calculatedAt?: string
}

/**
 * Error detail structure
 */
export interface FeeEngineErrorDetail {
  code: string
  field: string
  message: string
  severity: 'error' | 'warning'
}

/**
 * Error response structure
 */
export interface FeeEngineErrorResponse {
  error: {
    code: string
    message: string
    details?: FeeEngineErrorDetail[]
  }
}

/**
 * Fee package status
 */
export type FeePackageStatus = 'active' | 'inactive' | 'draft'

/**
 * Fee package structure
 */
export interface FeeEnginePackage {
  id: string
  name: string
  description: string
  status: FeePackageStatus
  rules: FeeEngineRule[]
  createdAt: string
  updatedAt: string
  metadata?: Record<string, any>
}

/**
 * Fee rule structure
 */
export interface FeeEngineRule {
  id: string
  name: string
  description: string
  type: 'fixed' | 'percentage' | 'tiered'
  amount: number
  asset?: string
  appliedTo: 'source' | 'destination' | 'transaction'
  conditions?: FeeEngineCondition[]
  metadata?: Record<string, any>
}

/**
 * Rule condition structure
 */
export interface FeeEngineCondition {
  field: string
  operator: 'eq' | 'ne' | 'gt' | 'gte' | 'lt' | 'lte' | 'in' | 'nin'
  value: any
}

/**
 * Type guards
 */
export function isFeeEngineError(
  response: any
): response is FeeEngineErrorResponse {
  return response?.error !== undefined
}

export function isFeeEngineCalculateResponse(
  response: any
): response is FeeEngineCalculateResponse {
  if (response?.fees !== undefined && Array.isArray(response.fees)) {
    return true
  }

  if (
    response?.feesApplied !== undefined &&
    Array.isArray(response.feesApplied) &&
    response.feesApplied.length === 0
  ) {
    return true
  }

  return false
}
