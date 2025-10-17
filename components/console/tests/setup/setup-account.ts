import { postRequest } from '../utils/fetcher'
import { ORGANIZATION_ID, LEDGER_ID, MIDAZ_BASE_PATH } from '../fixtures/config'
import { ACCOUNTS } from '../fixtures/accounts'

interface AccountPayload {
  name: string
  assetCode: string
  type: string
  alias: string
  status: {
    code: string
    description?: string
  }
}

/**
 * Create an account via the Onboarding API
 */
async function createAccount(payload: AccountPayload) {
  const url = `${MIDAZ_BASE_PATH}/v1/organizations/${ORGANIZATION_ID}/ledgers/${LEDGER_ID}/accounts`

  try {
    const response = await postRequest(url, payload)
    return response
  } catch (error) {
    console.error(`Failed to create account ${payload.alias}:`, error)
    throw error
  }
}

/**
 * Setup test accounts in the database via API
 * Creates:
 * 1. BRL Accounts - 5 accounts using Brazilian Real asset
 * 2. BTC Accounts - 4 accounts using Bitcoin asset
 */
export async function setupAccounts() {
  if (!ORGANIZATION_ID) {
    throw new Error('ORGANIZATION_ID environment variable is required')
  }

  if (!LEDGER_ID) {
    throw new Error('LEDGER_ID environment variable is required')
  }

  try {
    // eslint-disable-next-line no-console
    console.log('Creating accounts...')

    const createdAccounts: Record<string, any> = {}

    // Create all accounts from the fixtures
    for (const [key, account] of Object.entries(ACCOUNTS)) {
      const result = await createAccount(account)
      createdAccounts[key.toLowerCase()] = result
      // eslint-disable-next-line no-console
      console.log(`✓ Created ${account.name}:`, result.id)
    }

    // eslint-disable-next-line no-console
    console.log('✓ Test accounts created successfully')

    return createdAccounts
  } catch (error) {
    console.error('Failed to setup accounts:', error)
    throw error
  }
}

// Run if executed directly
if (require.main === module) {
  setupAccounts()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error)
      process.exit(1)
    })
}
