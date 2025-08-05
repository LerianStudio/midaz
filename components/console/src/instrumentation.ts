export async function register() {
  if (
    process.env.NEXT_RUNTIME === 'nodejs' &&
    process.env.ENABLE_TELEMETRY === 'true'
  ) {
    await import('./core/infrastructure/observability/instrumentation-config')
  }
}
