import { getRequest } from './fetcher'
import {
  ORGANIZATION_ID,
  LEDGER_ID,
  MIDAZ_BASE_PATH,
  MIDAZ_TRANSACTION_BASE_PATH
} from '../fixtures/config'
import { createTransaction } from '../setup/setup-transaction'

interface Balance {
  id: string
  assetCode: string
  available: string
  onHold: string
  scale: string
}

interface BalanceResponse {
  items: Balance[]
}

interface Account {
  id: string
  alias: string
  assetCode: string
}

/**
 * Get account balance by account ID
 */
export async function getAccountBalance(
  accountId: string
): Promise<BalanceResponse> {
  const url = `${MIDAZ_TRANSACTION_BASE_PATH}/v1/organizations/${ORGANIZATION_ID}/ledgers/${LEDGER_ID}/accounts/${accountId}/balances`

  try {
    const response = await getRequest(url)
    return response
  } catch (error) {
    console.error(`Failed to get balance for account ${accountId}:`, error)
    throw error
  }
}

/**
 * Get account by alias
 */
export async function getAccountByAlias(alias: string): Promise<Account> {
  const url = `${MIDAZ_BASE_PATH}/v1/organizations/${ORGANIZATION_ID}/ledgers/${LEDGER_ID}/accounts/alias/${alias}`

  try {
    const response = await getRequest(url)
    return response
  } catch (error) {
    console.error(`Failed to get account by alias ${alias}:`, error)
    throw error
  }
}

/**
 * Reset account balance to target amount by draining excess to external
 */
export async function resetAccountBalance(
  alias: string,
  assetCode: string,
  targetBalance: number
): Promise<void> {
  try {
    // Get account details
    const account = await getAccountByAlias(alias)

    // Get current balance
    const balanceResponse = await getAccountBalance(account.id)

    if (!balanceResponse.items || balanceResponse.items.length === 0) {
      console.log(`Account ${alias} has no balance, skipping reset`)
      return
    }

    // Find balance for the specified asset
    const balance = balanceResponse.items.find((b) => b.assetCode === assetCode)

    if (!balance) {
      console.log(
        `Account ${alias} has no ${assetCode} balance, skipping reset`
      )
      return
    }

    const currentBalance = parseFloat(balance.available)
    const difference = currentBalance - targetBalance

    if (difference <= 0) {
      console.log(
        `Account ${alias} has ${currentBalance} ${assetCode}, no need to drain (target: ${targetBalance})`
      )
      return
    }

    // Drain excess to external
    console.log(
      `Draining ${difference} ${assetCode} from ${alias} to reach target of ${targetBalance}`
    )

    await createTransaction({
      chartOfAccountsGroupName: 'FUNDING',
      description: `Reset balance: drain ${difference} ${assetCode} from ${alias}`,
      send: {
        asset: assetCode,
        value: difference.toString(),
        source: {
          from: [
            {
              accountAlias: alias,
              amount: {
                value: difference.toString(),
                asset: assetCode
              },
              chartOfAccounts: 'FUNDING_DEBIT',
              description: `Drain from ${alias}`
            }
          ]
        },
        distribute: {
          to: [
            {
              accountAlias: `@external/${assetCode}`,
              amount: {
                value: difference.toString(),
                asset: assetCode
              },
              chartOfAccounts: 'FUNDING_CREDIT',
              description: `External ${assetCode} drain`
            }
          ]
        }
      }
    })

    console.log(`✓ Reset balance for ${alias} to ${targetBalance} ${assetCode}`)
  } catch (error) {
    console.error(`Failed to reset balance for ${alias}:`, error)
    throw error
  }
}
