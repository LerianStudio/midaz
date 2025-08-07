import {
  ConsoleTransaction,
  FeeEngineTransaction,
  FeeEngineAccountEntry
} from '@/types/fee-engine-transaction.types'

export function convertToFeeEngineFormat(
  transaction: ConsoleTransaction
): FeeEngineTransaction {
  if (transaction.simple) {
    return {
      description: transaction.description,
      chartOfAccountsGroupName: transaction.metadata?.chartOfAccountsGroupName,
      route: transaction.metadata?.route,
      send: {
        asset: transaction.asset,
        value: transaction.value,
        source: {
          from: [
            {
              accountAlias: transaction.simple.from,
              amount: {
                asset: transaction.asset,
                value: transaction.value
              }
            }
          ]
        },
        distribute: {
          to: [
            {
              accountAlias: transaction.simple.to,
              amount: {
                asset: transaction.asset,
                value: transaction.value
              }
            }
          ]
        }
      },
      metadata: transaction.metadata
    }
  }

  if (transaction.complex) {
    const sourceAccounts: FeeEngineAccountEntry[] =
      transaction.complex.source.map((source) => {
        const account: FeeEngineAccountEntry = {
          accountAlias: source.accountAlias
        }

        if (source.percentage !== undefined) {
          account.share = { percentage: source.percentage }
        } else if (source.value !== undefined) {
          account.amount = {
            asset: transaction.asset,
            value: source.value
          }
        }

        return account
      })

    const destinationAccounts: FeeEngineAccountEntry[] =
      transaction.complex.destination.map((dest) => {
        const account: FeeEngineAccountEntry = {
          accountAlias: dest.accountAlias
        }

        if (dest.percentage !== undefined) {
          account.share = { percentage: dest.percentage }
        } else if (dest.value !== undefined) {
          account.amount = {
            asset: transaction.asset,
            value: dest.value
          }
        }

        return account
      })

    return {
      description: transaction.description,
      chartOfAccountsGroupName: transaction.metadata?.chartOfAccountsGroupName,
      route: transaction.metadata?.route,
      send: {
        asset: transaction.asset,
        value: transaction.value,
        source: {
          from: sourceAccounts
        },
        distribute: {
          to: destinationAccounts
        }
      },
      metadata: transaction.metadata
    }
  }

  throw new Error('Transaction must have either simple or complex format')
}

/**
 * Validates if percentages add up to 100
 */
export function validatePercentages(
  accounts: Array<{ percentage?: number }>
): boolean {
  const total = accounts.reduce((sum, account) => {
    return sum + (account.percentage || 0)
  }, 0)

  return Math.abs(total - 100) < 0.01 // Allow for small floating point errors
}

/**
 * Validates if amounts add up to total value
 */
export function validateAmounts(
  accounts: Array<{ value?: string }>,
  totalValue: string
): boolean {
  const total = accounts.reduce((sum, account) => {
    return sum + parseFloat(account.value || '0')
  }, 0)

  const expectedTotal = parseFloat(totalValue)
  return Math.abs(total - expectedTotal) < 0.01 // Allow for small floating point errors
}

/**
 * Helper to determine if transaction uses percentages or amounts
 */
export function isPercentageBased(
  accounts: Array<{ percentage?: number; value?: string }>
): boolean {
  return accounts.some((account) => account.percentage !== undefined)
}

/**
 * Helper to determine if transaction uses amounts
 */
export function isAmountBased(
  accounts: Array<{ percentage?: number; value?: string }>
): boolean {
  return accounts.some((account) => account.value !== undefined)
}
