/**
 * This file is used to import all the instrumentation modules that are needed for the nextjs application.
 * This is the entry point for the instrumentation modules.
 *
 */

export async function register() {
  if (
    process.env.NEXT_RUNTIME === 'nodejs' &&
    process.env.ENABLE_TELEMETRY === 'true'
  ) {
    await import('./core/infrastructure/observability/instrumentation-config')
  }
}
