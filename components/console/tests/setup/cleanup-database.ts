import { resolve } from 'path'
import { command } from '../utils/cmd'
import { delay } from '../utils/delay'

// Get the correct path to docker-compose.test.yml
const dockerComposePath = resolve(
  __dirname,
  '../../../infra/docker-compose.test.yml'
)

/**
 * Cleanup test database and Docker environment
 * This will:
 * 1. Stop all test containers
 * 2. Remove all test volumes
 * 3. Optionally restart the containers
 */
export async function cleanupDatabase(restart = false) {
  try {
    // eslint-disable-next-line no-console
    console.log('Starting cleanup of test environment...\n')

    // Step 1: Stop and remove containers and volumes
    // eslint-disable-next-line no-console
    console.log('1. Stopping containers and removing volumes...')
    const downCommand = `docker compose -f "${dockerComposePath}" down -v`

    try {
      await command(downCommand)
    } catch (error) {
      console.error('Failed to stop containers:', error)
      throw error
    }

    // eslint-disable-next-line no-console
    console.log('✓ Containers stopped and volumes removed')

    if (restart) {
      // Step 2: Restart containers
      // eslint-disable-next-line no-console
      console.log('\n2. Restarting containers...')
      const upCommand = `docker compose -f "${dockerComposePath}" up -d`

      try {
        await command(upCommand)
      } catch (error) {
        console.error('Failed to restart containers:', error)
        throw error
      }

      // eslint-disable-next-line no-console
      console.log('✓ Containers restarted')

      // Wait a bit for containers to be healthy
      // eslint-disable-next-line no-console
      console.log('\n3. Waiting for database containers to be healthy...')
      await delay(10000)
      // eslint-disable-next-line no-console
      console.log('✓ Database containers are ready')

      // Step 3: Restart Onboarding and Transaction services to run migrations
      // eslint-disable-next-line no-console
      console.log(
        '\n4. Restarting Onboarding and Transaction services to run migrations...'
      )
      const onboardingPath = resolve(__dirname, '../../../onboarding')
      const transactionPath = resolve(__dirname, '../../../transaction')

      try {
        await command([
          `docker compose -f "${onboardingPath}/docker-compose.test.yml" restart`,
          `docker compose -f "${transactionPath}/docker-compose.test.yml" restart`
        ])
      } catch (error) {
        console.error('Failed to restart services:', error)
        throw error
      }

      // eslint-disable-next-line no-console
      console.log('✓ Services restarted')

      // Wait for migrations to complete
      // eslint-disable-next-line no-console
      console.log('\n5. Waiting for migrations to complete...')
      await delay(5000)
      // eslint-disable-next-line no-console
      console.log('✓ Migrations should be complete')
    }

    // eslint-disable-next-line no-console
    console.log('\n✓ Cleanup completed successfully!')

    if (restart) {
      // eslint-disable-next-line no-console
      console.log('\nYou can now run: tsx tests/setup/setup-database.ts')
    } else {
      // eslint-disable-next-line no-console
      console.log(
        '\nTo start containers: docker compose -f components/infra/docker-compose.test.yml up -d'
      )
    }

    return true
  } catch (error) {
    console.error('\n✗ Cleanup failed:', error)
    throw error
  }
}

// Run if executed directly
if (require.main === module) {
  // Check if --restart flag is provided
  const restart = process.argv.includes('--restart')

  cleanupDatabase(restart)
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error)
      process.exit(1)
    })
}
