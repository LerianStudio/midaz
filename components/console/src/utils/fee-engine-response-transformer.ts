import {
  FeeCalculationResponse,
  isSuccessResponse,
  isNoFeesResponse,
  isGratuityResponse,
  FeeCalculationSuccess
} from '@/types/fee-engine-transaction.types'

interface ConsoleTransactionResponse {
  transaction: {
    description: string
    chartOfAccountsGroupName?: string
    send: {
      asset: string
      value: string
      source: {
        from: any[]
      }
      distribute: {
        to: any[]
      }
    }
    metadata?: any
    feeRules?: any[]
    isDeductibleFrom?: boolean
  }
  feesApplied?: boolean | any[]
  message?: string
  hasNoFees?: boolean
  hasGratuity?: boolean
}

/**
 * Transforms Fee Engine response to console format
 * This maintains compatibility with the existing frontend while using the new Fee Engine format
 */
export function transformFeeEngineResponse(
  feeEngineResponse: FeeCalculationResponse,
  originalTransaction: any
): ConsoleTransactionResponse | FeeCalculationResponse {
  if (isNoFeesResponse(feeEngineResponse)) {
    const feeEngineFormat = convertOriginalToFeeEngine(originalTransaction)
    return {
      feesApplied: false,
      message: feeEngineResponse.message,
      hasNoFees: true,
      transaction: {
        ...feeEngineFormat,
        feeRules: [],
        isDeductibleFrom: false
      }
    }
  }

  if (isGratuityResponse(feeEngineResponse)) {
    const feeEngineFormat = convertOriginalToFeeEngine(originalTransaction)
    return {
      feesApplied: false,
      message: feeEngineResponse.message,
      hasGratuity: true,
      transaction: {
        ...feeEngineFormat,
        feeRules: [],
        isDeductibleFrom: false
      }
    }
  }

  if (isSuccessResponse(feeEngineResponse)) {
    const successResponse = feeEngineResponse as FeeCalculationSuccess

    const feeOperations = extractFeeOperations(
      successResponse.transaction,
      originalTransaction
    )

    const enhancedTransaction = {
      ...successResponse.transaction,
      feeRules: createFeeRulesFromOperations(feeOperations, successResponse),
      isDeductibleFrom: feeOperations.some(
        (op: any) => op.metadata?.isDeductible
      )
    }

    return {
      transaction: enhancedTransaction,
      feesApplied: true
    }
  }

  return feeEngineResponse
}

/**
 * Extract fee operations from the transaction
 * Fee operations are destinations that are not in the original transaction
 */
function extractFeeOperations(
  feeTransaction: any,
  originalTransaction?: any
): any[] {
  const originalDestinations =
    originalTransaction?.destination?.map((d: any) => d.accountAlias) || []

  // or have fee-related metadata
  return feeTransaction.send.distribute.to.filter((dest: any) => {
    if (
      dest.metadata?.isFee ||
      dest.metadata?.feeType ||
      dest.metadata?.addedByFeeEngine
    ) {
      return true
    }

    if (!originalDestinations.includes(dest.accountAlias)) {
      return true
    }

    const originalDest = originalTransaction?.destination?.find(
      (d: any) => d.accountAlias === dest.accountAlias
    )
    if (originalDest && dest.amount?.value !== originalDest.value) {
      return true
    }

    return false
  })
}

/**
 * Create fee rules from fee operations
 * This creates a structure compatible with what the frontend expects
 */
function createFeeRulesFromOperations(
  feeOperations: any[],
  _response: FeeCalculationSuccess
): any[] {
  return feeOperations.map((op, index) => ({
    feeId: `fee-${index}`,
    feeLabel: op.metadata?.feeLabel || `Fee ${index + 1}`,
    isDeductibleFrom: op.metadata?.isDeductible || false,
    creditAccount: op.accountAlias,
    priority: op.metadata?.priority || index,
    referenceAmount: 'originalAmount',
    applicationRule: op.share ? 'percentual' : 'fixed',
    calculations: op.share
      ? [{ type: 'percentage', value: op.share.percentage }]
      : [{ type: 'fixed', value: op.amount?.value }]
  }))
}

/**
 * Calculate value from percentage share
 */
function _calculateShareValue(totalValue: string, percentage?: number): string {
  if (!percentage) return '0'
  const total = parseFloat(totalValue)
  const value = (total * percentage) / 100
  return value.toFixed(2)
}

/**
 * Convert original console transaction to Fee Engine format
 */
function convertOriginalToFeeEngine(originalTransaction: any): any {
  return {
    description: originalTransaction.description || 'Transaction',
    chartOfAccountsGroupName: originalTransaction.chartOfAccountsGroupName,
    send: {
      asset: originalTransaction.asset,
      value: originalTransaction.value,
      source: {
        from: originalTransaction.source.map((src: any) => ({
          accountAlias: src.accountAlias,
          amount: {
            asset: originalTransaction.asset,
            value: src.value
          },
          metadata: src.metadata || {}
        }))
      },
      distribute: {
        to: originalTransaction.destination.map((dest: any) => ({
          accountAlias: dest.accountAlias,
          amount: {
            asset: originalTransaction.asset,
            value: dest.value
          },
          metadata: dest.metadata || {}
        }))
      }
    },
    metadata: originalTransaction.metadata
  }
}
