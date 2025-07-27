import type {
  ConsoleTransactionRequest,
  FeeTransformationMapper,
  FeeApiCalculateResponse,
  FeeApiCalculateSuccessResponse
} from '@/types/fee-api.types'
import type {
  FeeEngineCalculateRequest,
  FeeEngineCalculateResponse,
  FeeEngineFeeEntry
} from '@/types/fee-engine-api.types'

export class FeeEngineTransformer implements FeeTransformationMapper {
  /**
   * Transform Console transaction format to Fee Engine API format
   */
  transformConsoleToFeeEngine(
    consoleRequest: ConsoleTransactionRequest,
    ledgerId: string,
    segmentId?: string
  ): FeeEngineCalculateRequest {
    const { transaction } = consoleRequest

    const finalSegmentId = segmentId || transaction.metadata?.segmentId || ''
    if (!finalSegmentId) {
      throw new Error('Segment ID is required for fee calculation')
    }

    const sourceShares = this.calculatePercentageShares(
      transaction.source.map((s) => ({ value: s.value }))
    )

    const destinationShares = this.calculatePercentageShares(
      transaction.destination.map((d) => ({ value: d.value }))
    )

    const feeEngineRequest: FeeEngineCalculateRequest = {
      segmentId: finalSegmentId,
      ledgerId,
      transaction: {
        route: transaction.metadata?.route || 'default',
        description: transaction.description || 'Transaction',
        send: {
          asset: transaction.asset,
          value: transaction.value,
          source: {
            from: transaction.source.map((source, index) => ({
              accountAlias: source.accountAlias,
              share: {
                percentage: sourceShares[index].percentage
              }
            }))
          },
          distribute: {
            to: transaction.destination.map((dest, index) => ({
              accountAlias: dest.accountAlias,
              share: {
                percentage: destinationShares[index].percentage
              }
            }))
          }
        }
      }
    }

    if (finalSegmentId) {
      feeEngineRequest.segmentId = finalSegmentId
    }

    return feeEngineRequest
  }

  /**
   * Transform Fee Engine response back to Console format
   */
  transformFeeEngineToConsole(
    feeEngineResponse: FeeEngineCalculateResponse,
    originalRequest: ConsoleTransactionRequest
  ): FeeApiCalculateResponse {
    const { transaction } = originalRequest

    if (
      feeEngineResponse.feesApplied !== undefined &&
      Array.isArray(feeEngineResponse.feesApplied) &&
      feeEngineResponse.feesApplied.length === 0
    ) {
      return {
        feesApplied: [],
        message: feeEngineResponse.message || 'No fees applied'
      }
    }

    if (!feeEngineResponse.fees || feeEngineResponse.fees.length === 0) {
      return {
        feesApplied: [],
        message: feeEngineResponse.message || 'No fees applied'
      }
    }

    const feeRules = feeEngineResponse.fees.map((fee: FeeEngineFeeEntry) => ({
      feeId: fee.id,
      feeLabel: fee.name,
      isDeductibleFrom: fee.appliedTo === 'source',
      creditAccount: fee.metadata?.creditAccount || '',
      priority: fee.metadata?.priority || 0
    }))

    const sourceValues = this.calculateExplicitValues(
      transaction.source.map((_, index) => ({
        percentage:
          (parseFloat(transaction.source[index].value) /
            parseFloat(transaction.value)) *
          100
      })),
      feeEngineResponse.originalAmount?.value || '0',
      feeEngineResponse.originalAmount?.asset || 'USD'
    )

    const netValue = feeEngineResponse.netAmount?.value || '0'
    const destinationValues = this.calculateExplicitValues(
      transaction.destination.map((_, index) => ({
        percentage:
          (parseFloat(transaction.destination[index].value) /
            parseFloat(transaction.value)) *
          100
      })),
      netValue,
      feeEngineResponse.netAmount?.asset || 'USD'
    )

    const response: FeeApiCalculateSuccessResponse = {
      segmentId: transaction.metadata?.segmentId || '',
      ledgerId: '', // Will be filled by the route handler
      transaction: {
        chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
        description: transaction.description,
        route: transaction.metadata?.route,
        send: {
          asset: feeEngineResponse.originalAmount?.asset || 'USD',
          value: feeEngineResponse.originalAmount?.value || '0',
          source: {
            from: transaction.source.map((source, index) => ({
              accountAlias: source.accountAlias,
              amount: {
                asset: sourceValues[index].asset,
                value: sourceValues[index].value
              },
              chartOfAccounts: source.chartOfAccounts,
              description: source.description,
              route: transaction.metadata?.route || '',
              metadata: source.metadata
            }))
          },
          distribuite: {
            to: transaction.destination.map((dest, index) => ({
              accountAlias: dest.accountAlias,
              amount: {
                asset: destinationValues[index].asset,
                value: destinationValues[index].value
              },
              chartOfAccounts: dest.chartOfAccounts,
              description: dest.description,
              route: transaction.metadata?.route || '',
              metadata: dest.metadata
            })),
            metadata: {}
          }
        },
        metadata: {
          ...transaction.metadata,
          packageAppliedID: feeEngineResponse.fees[0]?.metadata?.packageId,
          feeRules,
          totalFees: feeEngineResponse.totalFees,
          netAmount: feeEngineResponse.netAmount,
          calculatedAt: feeEngineResponse.calculatedAt
        },
        feeRules,
        isDeductibleFrom: feeRules.some((rule) => rule.isDeductibleFrom)
      }
    }

    return response
  }

  /**
   * Calculate percentage shares from explicit values
   */
  calculatePercentageShares(
    accounts: Array<{ value: string }>
  ): Array<{ percentage: number }> {
    const totalValue = accounts.reduce((sum, account) => {
      return sum + parseFloat(account.value || '0')
    }, 0)

    if (totalValue === 0) {
      const equalShare = 100 / accounts.length
      return accounts.map(() => ({ percentage: equalShare }))
    }

    return accounts.map((account) => ({
      percentage: (parseFloat(account.value || '0') / totalValue) * 100
    }))
  }

  /**
   * Convert percentage shares back to explicit values
   */
  calculateExplicitValues(
    accounts: Array<{ percentage: number }>,
    totalValue: string,
    asset: string
  ): Array<{ value: string; asset: string }> {
    const total = parseFloat(totalValue)

    return accounts.map((account) => ({
      value: ((account.percentage / 100) * total).toFixed(2),
      asset
    }))
  }

  /**
   * Validate Fee Engine response
   */
  validateFeeEngineResponse(
    response: any
  ): response is FeeEngineCalculateResponse {
    if (!response || typeof response !== 'object') {
      return false
    }

    return (
      'fees' in response &&
      Array.isArray(response.fees) &&
      'totalFees' in response &&
      'netAmount' in response &&
      'originalAmount' in response &&
      'feesApplied' in response &&
      'calculatedAt' in response
    )
  }

  /**
   * Handle Fee Engine errors
   */
  handleFeeEngineError(error: any): FeeApiCalculateResponse {
    return {
      feesApplied: [],
      message: error?.message || 'Fee calculation failed - no fees applied'
    }
  }
}

export const feeEngineTransformer = new FeeEngineTransformer()
