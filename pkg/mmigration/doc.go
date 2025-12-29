// Package mmigration provides migration management utilities with auto-recovery support.
//
// This package wraps lib-commons PostgresConnection to add preflight checks,
// dirty state recovery, advisory locks for concurrent protection, and comprehensive
// observability via Prometheus metrics.
//
// # Key Features
//
//   - PreflightCheck: Validates schema_migrations state before running migrations
//   - SafeGetDB: Wrapper with automatic dirty recovery and retry logic
//   - Advisory locks: Prevents concurrent migration runs across pods
//   - Metrics: Exposes migration_duration_seconds, migration_recovery_total, etc.
//   - Health endpoint: Minimal health status for Kubernetes probes
//
// # Prerequisites
//
// All migrations MUST be idempotent. This means using:
//   - CREATE TABLE IF NOT EXISTS
//   - CREATE INDEX IF NOT EXISTS
//   - ALTER TABLE ... ADD COLUMN IF NOT EXISTS
//   - DROP TABLE IF EXISTS
//
// This is required because auto-recovery clears the dirty flag and allows
// golang-migrate to retry the migration from scratch.
//
// # Basic Usage
//
// Create a MigrationWrapper during service bootstrap using DefaultConfig() as a base:
//
//	// DefaultConfig() provides sensible defaults for optional fields.
//	// You MUST set Component and MigrationsPath explicitly.
//	migrationConfig := mmigration.DefaultConfig()
//	migrationConfig.Component = "my-service"
//	migrationConfig.MigrationsPath = "/app/components/my-service/migrations"
//
//	wrapper, err := mmigration.NewMigrationWrapper(postgresConn, migrationConfig, logger)
//	if err != nil {
//	    log.Fatalf("Failed to create migration wrapper: %v", err)
//	}
//
//	// Use SafeGetDBWithRetry for automatic recovery and retry
//	ctx := context.Background()
//	db, err := wrapper.SafeGetDBWithRetry(ctx)
//	if err != nil {
//	    log.Fatalf("Migration failed: %v", err)
//	}
//
// # Health Endpoint Integration
//
// Add migration health to your service's health endpoint:
//
//	// Using Fiber
//	app.Get("/health/migrations", mmigration.FiberHealthHandler(wrapper))
//
//	// Or check readiness
//	if !mmigration.FiberReadinessCheck(wrapper) {
//	    return fiber.NewError(fiber.StatusServiceUnavailable, "migrations unhealthy")
//	}
//
// # Metrics
//
// The following Prometheus metrics are exposed:
//
//   - midaz_migration_duration_seconds: Time spent in migration operations
//   - midaz_migration_recovery_total: Count of recovery attempts (success/failure)
//   - midaz_migration_lock_wait_seconds: Time spent waiting for advisory lock
//   - midaz_migration_status: Current health status (1=healthy, 0=unhealthy)
//   - midaz_migration_version: Current migration version number
//
// # Configuration
//
// Environment variables (when using bootstrap integration):
//
//   - MIGRATION_AUTO_RECOVER: Enable/disable auto-recovery (default: true)
//   - MIGRATION_MAX_RETRIES: Maximum retry attempts (default: 3)
//
// # Error Handling
//
// The package defines several sentinel errors:
//
//   - ErrMigrationDirty: Migration is in dirty state
//   - ErrMigrationLockFailed: Could not acquire advisory lock
//   - ErrMigrationRecoveryFailed: Recovery attempt failed
//   - ErrMigrationFileNotFound: Migration file doesn't exist
//   - ErrMaxRetriesExceeded: Retry limit reached
//   - ErrMaxRecoveryPerVersionExceeded: Per-version recovery limit reached
package mmigration
