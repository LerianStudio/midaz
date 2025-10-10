import { setupOrganization } from './setup-organization'
import { setupLedger } from './setup-ledger'
import { setupAssets } from './setup-asset'
import { setupAccounts } from './setup-account'
import { setupTransactions } from './setup-transaction'

/**
 * Setup all database fixtures for E2E tests
 * This will insert:
 * 1. Test organization
 * 2. Test ledger
 * 3. Test assets (BRL and BTC)
 * 4. Test accounts (BRL and BTC accounts)
 * 5. Test transactions (BRL and BTC deposits)
 */
export async function setupDatabase() {
  try {
    // eslint-disable-next-line no-console
    console.log('Starting database setup for E2E tests...\n')

    // Step 1: Setup organization
    // eslint-disable-next-line no-console
    console.log('1. Setting up organization...')
    await setupOrganization()

    // Step 2: Setup ledger
    // eslint-disable-next-line no-console
    console.log('\n2. Setting up ledger...')
    await setupLedger()

    // Step 3: Setup assets
    // eslint-disable-next-line no-console
    console.log('\n3. Setting up assets...')
    await setupAssets()

    // Step 4: Setup accounts
    // eslint-disable-next-line no-console
    console.log('\n4. Setting up accounts...')
    await setupAccounts()

    // Step 5: Setup transactions
    // eslint-disable-next-line no-console
    console.log('\n5. Setting up transactions...')
    await setupTransactions()

    // eslint-disable-next-line no-console
    console.log('\n✓ Database setup completed successfully!')
    return true
  } catch (error) {
    console.error('\n✗ Database setup failed:', error)
    throw error
  }
}

// Run if executed directly
if (require.main === module) {
  setupDatabase()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error)
      process.exit(1)
    })
}
