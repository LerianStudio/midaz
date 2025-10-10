import { postRequest } from '../utils/fetcher'
import {
  ORGANIZATION_ID,
  LEDGER_ID,
  MIDAZ_TRANSACTION_BASE_PATH
} from '../fixtures/config'
import { TRANSACTIONS } from '../fixtures/transactions'

interface TransactionPayload {
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
async function createTransaction(payload: TransactionPayload) {
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
 * Creates:
 * 1. BRL Deposit - Moves 100 BRL from @external/BRL to brl-account-e2e
 * 2. BTC Deposit - Moves 0.001 BTC from @external/BTC to btc-account-e2e
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

    // Create BRL Deposit Transaction
    const brlTransaction = await createTransaction(
      JSON.parse(JSON.stringify(TRANSACTIONS.BRL_DEPOSIT))
    )
    // eslint-disable-next-line no-console
    console.log('✓ Created BRL deposit transaction:', brlTransaction.id)

    // Create BTC Deposit Transaction
    const btcTransaction = await createTransaction(
      JSON.parse(JSON.stringify(TRANSACTIONS.BTC_DEPOSIT))
    )
    // eslint-disable-next-line no-console
    console.log('✓ Created BTC deposit transaction:', btcTransaction.id)

    // eslint-disable-next-line no-console
    console.log('✓ Test transactions created successfully')

    return {
      brl: brlTransaction,
      btc: btcTransaction
    }
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
