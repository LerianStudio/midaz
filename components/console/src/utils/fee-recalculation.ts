import { FeeCalculationState } from '@/types/fee-calculation.types'
import { extractFeeStateFromCalculation } from '@/utils/fee-calculation-state'

export interface TransactionParameters {
  originalAmount: number
  currency: string
  sourceAccount: string
  destinationAccount: string
  packageId: string
  organizationId: string
  ledgerId: string
}

export const extractTransactionParameters = (
  transaction: any
): TransactionParameters | null => {
  try {
    const packageId = transaction.metadata?.packageAppliedID
    if (!packageId) {
      console.warn('No packageAppliedID found in transaction metadata')
      return null
    }

    const sourceOperations = transaction.source || []
    const destinationOperations = transaction.destination || []

    if (sourceOperations.length === 0 || destinationOperations.length === 0) {
      console.warn(
        'Invalid transaction structure - missing source or destination'
      )
      return null
    }

    const mainSources = sourceOperations.filter(
      (src: any) => !src.metadata?.source
    )
    const mainDestinations = destinationOperations.filter(
      (dest: any) => !dest.metadata?.source
    )

    const mainSource = mainSources[0] || sourceOperations[0]
    const mainDestination = mainDestinations[0] || destinationOperations[0]

    const originalAmount =
      mainDestinations.reduce(
        (sum: number, dest: any) => sum + Number(dest.amount),
        0
      ) || Number(mainDestination.amount)

    return {
      originalAmount,
      currency: transaction.asset,
      sourceAccount: mainSource.accountAlias,
      destinationAccount: mainDestination.accountAlias,
      packageId,
      organizationId: '', // Will be provided by caller
      ledgerId: '' // Will be provided by caller
    }
  } catch (error) {
    console.warn('Failed to extract transaction parameters:', error)
    return null
  }
}

export const recalculateFees = async (
  params: TransactionParameters
): Promise<FeeCalculationState | null> => {
  try {
    const calculationRequest = {
      transaction: {
        chartOfAccountsGroupName: 'FEES', // Default group
        description: 'Fee recalculation for summary',
        value: params.originalAmount,
        asset: params.currency,
        source: [
          {
            accountAlias: params.sourceAccount,
            value: params.originalAmount,
            description: 'Source for fee calculation',
            chartOfAccounts: 'DEBIT'
          }
        ],
        destination: [
          {
            accountAlias: params.destinationAccount,
            value: params.originalAmount,
            description: 'Destination for fee calculation',
            chartOfAccounts: 'CREDIT'
          }
        ],
        metadata: {}
      }
    }

    const response = await fetch(
      `/api/organizations/${params.organizationId}/ledgers/${params.ledgerId}/fees/calculate`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(calculationRequest)
      }
    )

    if (!response.ok) {
      throw new Error(`Fee calculation failed: ${response.status}`)
    }

    const calculationResult = await response.json()

    const originalFormValues = {
      value: params.originalAmount.toString(),
      asset: params.currency,
      source: [{ accountAlias: params.sourceAccount }],
      destination: [{ accountAlias: params.destinationAccount }]
    }

    return extractFeeStateFromCalculation(calculationResult, originalFormValues)
  } catch (error) {
    console.error('Fee recalculation failed:', error)
    return null
  }
}

interface CacheEntry {
  value: FeeCalculationState
  timestamp: number
}

class FeeCalculationCache {
  private cache = new Map<string, CacheEntry>()
  private readonly maxSize = 100 // Maximum number of entries
  private readonly ttlMs = 5 * 60 * 1000 // 5 minutes TTL

  get(key: string): FeeCalculationState | null {
    const entry = this.cache.get(key)
    if (!entry) return null

    if (Date.now() - entry.timestamp > this.ttlMs) {
      this.cache.delete(key)
      return null
    }

    return entry.value
  }

  set(key: string, value: FeeCalculationState): void {
    if (this.cache.size >= this.maxSize) {
      let oldestKey: string | null = null
      let oldestTime = Date.now()

      for (const [k, v] of this.cache.entries()) {
        if (v.timestamp < oldestTime) {
          oldestTime = v.timestamp
          oldestKey = k
        }
      }

      if (oldestKey) {
        this.cache.delete(oldestKey)
      }
    }

    this.cache.set(key, {
      value,
      timestamp: Date.now()
    })
  }

  clear(): void {
    this.cache.clear()
  }

  invalidate(transactionId: string): void {
    const keysToDelete: string[] = []
    for (const key of this.cache.keys()) {
      if (key.startsWith(`${transactionId}-`)) {
        keysToDelete.push(key)
      }
    }
    keysToDelete.forEach((key) => this.cache.delete(key))
  }
}

const feeCalculationCache = new FeeCalculationCache()

export const getCachedOrRecalculatedFees = async (
  transaction: any,
  organizationId: string,
  ledgerId: string
): Promise<FeeCalculationState | null> => {
  const cacheKey = `${transaction.id}-${organizationId}-${ledgerId}`

  const cachedValue = feeCalculationCache.get(cacheKey)
  if (cachedValue) {
    return cachedValue
  }

  const params = extractTransactionParameters(transaction)
  if (!params) {
    return null
  }

  params.organizationId = organizationId
  params.ledgerId = ledgerId

  const feeState = await recalculateFees(params)

  if (feeState) {
    feeCalculationCache.set(cacheKey, feeState)
  }

  return feeState
}

export const clearFeeCalculationCache = () => feeCalculationCache.clear()
export const invalidateFeeCalculation = (transactionId: string) =>
  feeCalculationCache.invalidate(transactionId)
