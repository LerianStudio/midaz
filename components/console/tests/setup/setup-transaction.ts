import { postRequest } from '../utils/fetcher'
import {
  ORGANIZATION_ID,
  LEDGER_ID,
  MIDAZ_TRANSACTION_BASE_PATH
} from '../fixtures/config'
import { TRANSACTIONS } from '../fixtures/transactions'

export interface TransactionPayload {
  chartOfAccountsGroupName: string
  description: string
  send: {
    asset: string
    value: string
    source: {
      from: Array<{
        accountAlias: string
        amount: {
          value: string
          asset: string
        }
        chartOfAccounts?: string
        description?: string
      }>
    }
    distribute: {
      to: Array<{
        accountAlias: string
        amount: {
          value: string
          asset: string
        }
        chartOfAccounts?: string
        description?: string
      }>
    }
  }
}

/**
 * Create a transaction via the Transaction API
 */
export async function createTransaction(payload: TransactionPayload) {
  const url = `${MIDAZ_TRANSACTION_BASE_PATH}/v1/organizations/${ORGANIZATION_ID}/ledgers/${LEDGER_ID}/transactions/json`

  try {
    const response = await postRequest(url, payload)
    return response
  } catch (error) {
    console.error(`Failed to create transaction ${payload.description}:`, error)
    throw error
  }
}

/**
 * Setup test transactions in the database via API
 * Creates initial deposits for all accounts:
 * - 5 BRL accounts: 100 BRL each
 * - 4 BTC accounts: 0.001 BTC each
 */
export async function setupTransactions() {
  if (!ORGANIZATION_ID) {
    throw new Error('ORGANIZATION_ID environment variable is required')
  }

  if (!LEDGER_ID) {
    throw new Error('LEDGER_ID environment variable is required')
  }

  try {
    // eslint-disable-next-line no-console
    console.log('Creating transactions...')

    const createdTransactions: Record<string, any> = {}

    // Create all transactions from the fixtures
    for (const [key, transaction] of Object.entries(TRANSACTIONS)) {
      const result = await createTransaction(
        JSON.parse(JSON.stringify(transaction))
      )
      createdTransactions[key.toLowerCase()] = result
      // eslint-disable-next-line no-console
      console.log(`✓ Created transaction: ${transaction.description}`)
    }

    // eslint-disable-next-line no-console
    console.log('✓ Test transactions created successfully')

    return createdTransactions
  } catch (error) {
    console.error('Failed to setup transactions:', error)
    throw error
  }
}

// Run if executed directly
if (require.main === module) {
  setupTransactions()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error)
      process.exit(1)
    })
}
