export interface FeeApiAmount {
  asset: string
  value: string
}

export interface FeeApiShare {
  percentage?: number
  percentageOfPercentage?: number
}

export interface FeeApiSourceAccount {
  account?: string
  accountAlias?: string
  amount: FeeApiAmount
  share?: FeeApiShare
  remaining?: string
  chartOfAccounts: string
  description: string
  route: string
  metadata: Record<string, any>
}

export interface FeeApiDestinationAccount {
  account?: string
  accountAlias?: string
  amount: FeeApiAmount
  share?: FeeApiShare
  chartOfAccounts: string
  description: string
  route: string
  metadata: Record<string, any>
}

export interface FeeApiTransactionSend {
  asset: string
  value: string
  source?: {
    from: FeeApiSourceAccount[]
  }
  distribuite?: {
    // Note: API spec uses 'distribuite' not 'distribute'
    to: FeeApiDestinationAccount[]
    metadata?: Record<string, any>
  }
}

export interface FeeApiTransaction {
  chartOfAccountsGroupName: string
  route?: string // Optional at transaction level
  pending?: string // Can be in response
  description: string
  send: FeeApiTransactionSend
  metadata: Record<string, any>
}

export interface FeeApiCalculateRequest {
  segmentId: string
  ledgerId: string
  transaction: FeeApiTransaction
}

export interface FeeApiCalculateSuccessResponse {
  segmentId: string
  ledgerId: string
  transaction: FeeApiTransaction & {
    metadata: Record<string, any> & {
      packageAppliedID?: string
    }
    feeRules?: Array<{
      feeId: string
      feeLabel: string
      isDeductibleFrom: boolean
      creditAccount: string
      priority: number
    }>
    isDeductibleFrom?: boolean // Legacy field for backward compatibility
  }
}

export interface FeeApiNoFeesResponse {
  feesApplied: string[]
  message: string
}

export interface FeeApiGratuityResponse {
  feesApplied: string[]
  message: string
}

export type FeeApiCalculateResponse =
  | FeeApiCalculateSuccessResponse
  | FeeApiNoFeesResponse
  | FeeApiGratuityResponse

export function isFeeApiSuccessResponse(
  response: FeeApiCalculateResponse
): response is FeeApiCalculateSuccessResponse {
  return 'transaction' in response && 'segmentId' in response
}

export function isFeeApiNoFeesResponse(
  response: FeeApiCalculateResponse
): response is FeeApiNoFeesResponse {
  return (
    'feesApplied' in response &&
    'message' in response &&
    response.message.includes('No fee')
  )
}

export function isFeeApiGratuityResponse(
  response: FeeApiCalculateResponse
): response is FeeApiGratuityResponse {
  return (
    'feesApplied' in response &&
    'message' in response &&
    response.message.includes('gratuity')
  )
}

export interface FeeApiEstimateRequest {
  packageId: string
  ledgerId: string
  transaction: {
    chartOfAccountsGroupName: string
    description: string
    send: {
      asset: string
      value: string
      source: {
        from: Array<{
          amount: FeeApiAmount
          account: string
          description: string
          chartOfAccounts: string
        }>
      }
      distribute: {
        // Note: estimate uses 'distribute' not 'distribuite'
        to: Array<{
          amount: FeeApiAmount
          account: string
          chartOfAccounts: string
          description: string
        }>
      }
    }
  }
}

export interface ConsoleTransactionRequest {
  transaction: {
    description: string
    chartOfAccountsGroupName: string
    value: string
    asset: string
    source: Array<{
      accountAlias: string
      value: string
      asset: string
      description: string
      chartOfAccounts: string
      metadata: Record<string, any>
    }>
    destination: Array<{
      accountAlias: string
      value: string
      asset: string
      description: string
      chartOfAccounts: string
      metadata: Record<string, any>
    }>
    metadata: {
      route?: string
      segmentId?: string
      [key: string]: any
    }
  }
}

export interface FeeTransformationMapper {
  /**
   * Transform Console transaction format to Fee Engine API format
   */
  transformConsoleToFeeEngine(
    consoleRequest: ConsoleTransactionRequest,
    ledgerId: string,
    segmentId?: string
  ): import('./fee-engine-api.types').FeeEngineCalculateRequest

  /**
   * Transform Fee Engine response back to Console format
   */
  transformFeeEngineToConsole(
    feeEngineResponse: import('./fee-engine-api.types').FeeEngineCalculateResponse,
    originalRequest: ConsoleTransactionRequest
  ): FeeApiCalculateResponse

  /**
   * Calculate percentage shares from explicit values
   */
  calculatePercentageShares(
    accounts: Array<{ value: string }>
  ): Array<{ percentage: number }>

  /**
   * Convert percentage shares back to explicit values
   */
  calculateExplicitValues(
    accounts: Array<{ percentage: number }>,
    totalValue: string,
    asset: string
  ): Array<{ value: string; asset: string }>
}
