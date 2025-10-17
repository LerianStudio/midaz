import { exec } from 'child_process'
import { promisify } from 'util'
import {
  DB_HOST,
  DB_PORT,
  DB_USER,
  DB_PASSWORD,
  DB_NAME
} from '../fixtures/config'

const execAsync = promisify(exec)

/**
 * Execute a SQL file against the test database
 * @param sqlFilePath - Absolute path to the SQL file to execute
 * @param variables - Optional key-value pairs for SQL variable substitution (e.g., { org_id: '123' })
 * @returns Promise that resolves when execution is complete
 */
export async function executeSqlFile(
  sqlFilePath: string,
  variables?: Record<string, string>
): Promise<void> {
  // Database connection details from environment variables
  const dbHost = DB_HOST || 'localhost'
  const dbPort = DB_PORT || '5701'
  const dbUser = DB_USER || 'midaz'
  const dbPassword = DB_PASSWORD || 'lerian'
  const dbName = DB_NAME || 'onboarding'

  // Set password in environment for psql
  const env = {
    ...process.env,
    PGPASSWORD: dbPassword
  }

  try {
    // Try to use docker exec if psql is not available on Windows
    let command: string

    // Check if we're on Windows and use docker exec as fallback
    try {
      await execAsync('psql --version')
      // psql is available, use it directly
      command = `psql -h ${dbHost} -p ${dbPort} -U ${dbUser} -d ${dbName}`

      // Add variable definitions if provided
      if (variables) {
        for (const [key, value] of Object.entries(variables)) {
          command += ` -v ${key}="${value}"`
        }
      }

      command += ` -f "${sqlFilePath}"`
    } catch {
      // psql not found, use docker exec instead
      // eslint-disable-next-line no-console
      console.log('psql not found, using docker exec...')

      const containerName = 'midaz-postgres-primary-test'
      const sqlFileName = sqlFilePath.split(/[\\/]/).pop()

      // Build docker command with variable definitions
      let psqlCmd = `psql -U ${dbUser} -d ${dbName}`
      if (variables) {
        for (const [key, value] of Object.entries(variables)) {
          psqlCmd += ` -v ${key}="${value}"`
        }
      }
      psqlCmd += ` -f /tmp/${sqlFileName}`

      command = `docker cp "${sqlFilePath}" ${containerName}:/tmp/${sqlFileName} && docker exec -e PGPASSWORD=${dbPassword} ${containerName} ${psqlCmd}`
    }

    const { stdout, stderr } = await execAsync(command, { env })

    // PostgreSQL outputs some informational messages to stderr, filter those out
    if (
      stderr &&
      !stderr.includes('NOTICE') &&
      !stderr.includes('INSERT 0 1')
    ) {
      console.warn('PostgreSQL stderr:', stderr)
    }

    if (stdout) {
      // eslint-disable-next-line no-console
      console.log('SQL execution output:', stdout)
    }
  } catch (error) {
    console.error('Failed to execute SQL file:', sqlFilePath, error)
    throw error
  }
}

/**
 * Execute raw SQL against the test database
 * @param sql - SQL query string to execute
 * @returns Promise that resolves when execution is complete
 */
export async function executeSql(sql: string): Promise<void> {
  // Database connection details from environment variables
  const dbHost = DB_HOST || 'localhost'
  const dbPort = DB_PORT || '5701'
  const dbUser = DB_USER || 'midaz'
  const dbPassword = DB_PASSWORD || 'lerian'
  const dbName = DB_NAME || 'onboarding'

  // Set password in environment for psql
  const env = {
    ...process.env,
    PGPASSWORD: dbPassword
  }

  try {
    // Execute the SQL using psql with -c flag
    const command = `psql -h ${dbHost} -p ${dbPort} -U ${dbUser} -d ${dbName} -c "${sql.replace(/"/g, '\\"')}"`

    const { stdout, stderr } = await execAsync(command, { env })

    // PostgreSQL outputs some informational messages to stderr, filter those out
    if (
      stderr &&
      !stderr.includes('NOTICE') &&
      !stderr.includes('INSERT 0 1')
    ) {
      console.warn('PostgreSQL stderr:', stderr)
    }

    if (stdout) {
      // eslint-disable-next-line no-console
      console.log('SQL execution output:', stdout)
    }
  } catch (error) {
    console.error('Failed to execute SQL:', sql, error)
    throw error
  }
}
