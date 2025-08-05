import {
  ConsoleTransaction,
  FeeCalculationRequest,
  FeeCalculationResponse,
  FeeEngineTransaction,
  isSuccessResponse,
  isNoFeesResponse,
  isGratuityResponse,
  isErrorResponse
} from '@/types/fee-engine-transaction.types'
import { convertToFeeEngineFormat } from './transaction-format-converter'

interface CreateTransactionWithFeesParams {
  transaction: ConsoleTransaction
  ledgerId: string
  organizationId: string
}

interface TransactionService {
  calculateFees: (
    request: FeeCalculationRequest
  ) => Promise<FeeCalculationResponse>
  createTransaction: (transaction: FeeEngineTransaction) => Promise<any>
}

/**
 * Creates a transaction with fee calculation if enabled
 * This is the main integration point between console and Fee Engine
 */
export async function createTransactionWithFees(
  params: CreateTransactionWithFeesParams,
  services: TransactionService
) {
  const { transaction, ledgerId, organizationId: _organizationId } = params

  const feeEngineTransaction = convertToFeeEngineFormat(transaction)

  const feesEnabled = process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED === 'true'

  if (!feesEnabled) {
    return services.createTransaction(feeEngineTransaction)
  }

  const feeRequest: FeeCalculationRequest = {
    ledgerId,
    transaction: feeEngineTransaction,
    ...(transaction.metadata?.segmentId && {
      segmentId: transaction.metadata.segmentId
    })
  }

  try {
    const feeResponse = await services.calculateFees(feeRequest)

    if (isSuccessResponse(feeResponse)) {
      return services.createTransaction(feeResponse.transaction)
    } else if (isNoFeesResponse(feeResponse)) {
      return services.createTransaction(feeEngineTransaction)
    } else if (isGratuityResponse(feeResponse)) {
      return services.createTransaction(feeEngineTransaction)
    } else if (isErrorResponse(feeResponse)) {
      throw new Error(`Fee calculation failed: ${feeResponse.error.message}`)
    }
  } catch (error) {
    console.error('Fee calculation error:', error)

    // For now, we'll proceed without fees
    return services.createTransaction(feeEngineTransaction)
  }
}

/**
 * Helper to build fee calculation URL
 */
export function buildFeeCalculationUrl(
  organizationId: string,
  ledgerId: string
): string {
  return `/api/organizations/${organizationId}/ledgers/${ledgerId}/fees/calculate`
}

/**
 * Helper to check if fee calculation is available
 */
export function isFeeCalculationAvailable(): boolean {
  return (
    process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED === 'true' &&
    !!process.env.PLUGIN_FEES_PATH
  )
}

/**
 * Helper to extract fee information from response
 */
export function extractFeeInfo(response: FeeCalculationResponse) {
  if (isSuccessResponse(response)) {
    return {
      hasFees: true,
      fees: response.fees || [],
      totalFees: response.totalFees,
      netAmount: response.netAmount,
      originalAmount: response.originalAmount
    }
  }

  return {
    hasFees: false,
    fees: [],
    totalFees: null,
    netAmount: null,
    originalAmount: null
  }
}
