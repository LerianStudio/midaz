import { exec } from 'child_process'
import { promisify } from 'util'

const execAsync = promisify(exec)

interface CmdOptions {
  cwd?: string
  env?: NodeJS.ProcessEnv
  timeout?: number
}

interface CmdResult {
  stdout: string
  stderr: string
}

/**
 * Execute a command in the shell (internal helper)
 */
async function runCommand(
  cmd: string,
  options?: CmdOptions
): Promise<CmdResult> {
  try {
    const { stdout, stderr } = await execAsync(cmd, {
      cwd: options?.cwd,
      env: options?.env || process.env,
      timeout: options?.timeout
    })

    return { stdout, stderr }
  } catch (error) {
    throw error
  }
}

/**
 * Execute a single command and log output
 */
async function executeSingleCommand(
  cmd: string,
  description?: string,
  options?: CmdOptions
): Promise<CmdResult> {
  if (description) {
    // eslint-disable-next-line no-console
    console.log(`Running: ${description}`)
  }

  try {
    const { stdout, stderr } = await runCommand(cmd, options)

    if (stdout) {
      // eslint-disable-next-line no-console
      console.log(stdout)
    }

    if (stderr && !stderr.includes('warn')) {
      console.warn('stderr:', stderr)
    }

    return { stdout, stderr }
  } catch (error) {
    console.error('Command failed:', cmd)
    throw error
  }
}

/**
 * Execute multiple commands sequentially and log output
 */
async function executeMultipleCommands(
  commands: string[],
  options?: CmdOptions
): Promise<CmdResult[]> {
  const results: CmdResult[] = []

  for (const cmd of commands) {
    const result = await executeSingleCommand(cmd, undefined, options)
    results.push(result)
  }

  return results
}

// Overload signatures
export function command(
  cmd: string,
  description?: string,
  options?: CmdOptions
): Promise<CmdResult>
export function command(
  commands: string[],
  options?: CmdOptions
): Promise<CmdResult[]>

/**
 * Execute command(s) and log output
 *
 * @example
 * // Single command
 * await command('docker ps')
 *
 * @example
 * // Single command with description
 * await command('npm install', 'Installing dependencies')
 *
 * @example
 * // Multiple commands
 * await command(['npm install', 'npm build'])
 *
 * @param cmdOrCommands - Single command string or array of commands
 * @param descriptionOrOptions - Description for single command, or options for array
 * @param options - Optional execution options (only for single command)
 */
export function command(
  cmdOrCommands: string | string[],
  descriptionOrOptions?: string | CmdOptions,
  options?: CmdOptions
): Promise<CmdResult | CmdResult[]> {
  if (Array.isArray(cmdOrCommands)) {
    // Array of commands
    const opts = descriptionOrOptions as CmdOptions | undefined
    return executeMultipleCommands(cmdOrCommands, opts)
  } else {
    // Single command
    const description = descriptionOrOptions as string | undefined
    return executeSingleCommand(cmdOrCommands, description, options)
  }
}
