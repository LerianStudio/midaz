import { FeeEngineTransaction } from '@/types/fee-engine-transaction.types'

interface ConsoleSourceDestination {
  accountAlias: string
  value: string
  asset?: string
  description?: string
  chartOfAccounts?: string
  metadata?: Record<string, any>
}

interface CurrentConsoleTransaction {
  description: string
  chartOfAccountsGroupName?: string
  value: string
  asset: string
  source: ConsoleSourceDestination[]
  destination: ConsoleSourceDestination[]
  metadata?: Record<string, any>
}

/**
 * Converts current console transaction format to Fee Engine format
 */
export function convertConsoleToFeeEngine(
  transaction: CurrentConsoleTransaction
): FeeEngineTransaction {
  const {
    route,
    segmentId: _segmentId,
    ...otherMetadata
  } = transaction.metadata || {}

  const feeEngineTransaction: FeeEngineTransaction = {
    description: transaction.description || 'Transaction',
    chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
    route: route as string | undefined,
    send: {
      asset: transaction.asset,
      value: transaction.value,
      source: {
        from: transaction.source.map((src) => ({
          accountAlias: src.accountAlias,
          amount: {
            asset: transaction.asset,
            value: src.value
          },
          metadata: src.metadata
        }))
      },
      distribute: {
        to: transaction.destination.map((dest) => ({
          accountAlias: dest.accountAlias,
          amount: {
            asset: transaction.asset,
            value: dest.value
          },
          metadata: dest.metadata
        }))
      }
    },
    metadata: {
      ...otherMetadata
      // It should be at the request level
    }
  }

  if (!feeEngineTransaction.chartOfAccountsGroupName) {
    delete feeEngineTransaction.chartOfAccountsGroupName
  }
  if (!feeEngineTransaction.route) {
    delete feeEngineTransaction.route
  }
  if (
    !feeEngineTransaction.metadata ||
    Object.keys(feeEngineTransaction.metadata).length === 0
  ) {
    delete feeEngineTransaction.metadata
  }

  return feeEngineTransaction
}
