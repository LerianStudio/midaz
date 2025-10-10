import { join } from 'path'
import { executeSqlFile } from '../utils/database'
import { ORGANIZATION_ID } from '../fixtures/config'

/**
 * Setup test organization in the database
 * Runs the insert-organization.sql script against the test database
 * Uses ORGANIZATION_ID from environment variable
 */
export async function setupOrganization() {
  const sqlFilePath = join(__dirname, 'insert-organization.sql')

  if (!ORGANIZATION_ID) {
    throw new Error('ORGANIZATION_ID environment variable is required')
  }

  try {
    // Execute SQL file with organization_id parameter
    await executeSqlFile(sqlFilePath, { organization_id: ORGANIZATION_ID })

    // eslint-disable-next-line no-console
    console.log('✓ Test organization inserted successfully')
    return true
  } catch (error) {
    console.error('Failed to setup organization:', error)
    throw error
  }
}

// Run if executed directly
if (require.main === module) {
  setupOrganization()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error)
      process.exit(1)
    })
}
