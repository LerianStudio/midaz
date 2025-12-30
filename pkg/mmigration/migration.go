package mmigration

//go:generate mockgen -destination=logger_mock.go -package=mmigration github.com/LerianStudio/lib-commons/v2/commons/log Logger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/lib/pq" // PostgreSQL driver for raw connections
)

// Sentinel errors for migration operations.
var (
	// ErrMigrationDirty indicates the schema_migrations table has dirty=true.
	ErrMigrationDirty = errors.New("migration is in dirty state")

	// ErrMigrationLockFailed indicates advisory lock acquisition failed.
	ErrMigrationLockFailed = errors.New("failed to acquire migration advisory lock")

	// ErrMigrationRecoveryFailed indicates dirty recovery failed.
	ErrMigrationRecoveryFailed = errors.New("migration recovery failed")

	// ErrMigrationFileNotFound indicates the migration file for recovery doesn't exist.
	ErrMigrationFileNotFound = errors.New("migration file not found for recovery")

	// ErrMaxRetriesExceeded indicates retry limit was reached.
	ErrMaxRetriesExceeded = errors.New("maximum migration retries exceeded")

	// ErrContextCanceled wraps context cancellation errors for proper error handling.
	ErrContextCanceled = errors.New("operation canceled")

	// ErrMissingMigrationsPath indicates MigrationsPath config is not set.
	ErrMissingMigrationsPath = errors.New("migrationsPath is required: use DefaultConfig() as a base and set cfg.MigrationsPath")

	// ErrMissingComponent indicates Component config is not set.
	ErrMissingComponent = errors.New("component is required: use DefaultConfig() as a base and set cfg.Component")

	// ErrNoConnectionString indicates no connection string is available.
	ErrNoConnectionString = errors.New("no connection string available")

	// ErrPostRecoveryVerification indicates verification after recovery failed.
	ErrPostRecoveryVerification = errors.New("post-recovery verification failed")

	// ErrQuerySchemaMigrations indicates a failure querying the schema_migrations table.
	ErrQuerySchemaMigrations = errors.New("failed to query schema_migrations")

	// ErrAdvisoryLockRelease indicates a failure releasing an advisory lock.
	ErrAdvisoryLockRelease = errors.New("failed to release advisory lock")

	// ErrClearDirtyFlag indicates a failure clearing the dirty flag.
	ErrClearDirtyFlag = errors.New("failed to clear dirty flag")

	// ErrSearchMigrationFile indicates a failure searching for migration files.
	ErrSearchMigrationFile = errors.New("failed to search for migration file")

	// ErrRawConnection indicates a failure with raw database connection.
	ErrRawConnection = errors.New("raw connection failed")

	// ErrPreflightCheck indicates a preflight check failure.
	ErrPreflightCheck = errors.New("preflight check failed")

	// ErrGetDB indicates GetDB operation failed.
	ErrGetDB = errors.New("GetDB failed")
)

// Default configuration values.
const (
	// defaultMaxRetries is the default number of retry attempts for migrations.
	defaultMaxRetries = 3

	// defaultMaxRecoveryPerVersion is the default maximum recovery attempts per version.
	defaultMaxRecoveryPerVersion = 3

	// defaultRetryBackoffSeconds is the default initial backoff in seconds.
	defaultRetryBackoffSeconds = 1

	// defaultMaxBackoffSeconds is the default maximum backoff in seconds.
	defaultMaxBackoffSeconds = 30

	// defaultLockTimeoutSeconds is the default lock timeout in seconds.
	defaultLockTimeoutSeconds = 30
)

// MigrationConfig configures migration behavior including auto-recovery.
type MigrationConfig struct {
	// AutoRecoverDirty enables automatic clearing of dirty flag on startup.
	// When true, if schema_migrations.dirty=true, the system will attempt
	// to clear the flag and retry migrations.
	// Default: true
	AutoRecoverDirty bool

	// MaxRetries is the maximum number of retry attempts for migrations.
	// Each retry uses exponential backoff.
	// Default: 3
	MaxRetries int

	// MaxRecoveryPerVersion is the maximum recovery attempts allowed per migration version.
	// After this limit, service refuses to start and requires manual intervention.
	// This prevents infinite boot loops when a migration has a permanent bug.
	// Default: 3
	MaxRecoveryPerVersion int

	// RetryBackoff is the initial backoff duration between retries.
	// Subsequent retries use exponential backoff (2x each time), capped at MaxBackoff.
	// Default: 1 second
	RetryBackoff time.Duration

	// MaxBackoff is the maximum backoff duration (cap for exponential growth).
	// Default: 30 seconds
	MaxBackoff time.Duration

	// LockTimeout is the maximum time to wait for advisory lock.
	// Default: 30 seconds
	LockTimeout time.Duration

	// Component identifies the service (e.g., "transaction", "onboarding").
	// Used for advisory lock namespacing and logging.
	Component string

	// MigrationsPath is the filesystem path to migration files.
	// Used to validate migration files exist before recovery.
	// REQUIRED: Must be explicitly configured (no default).
	MigrationsPath string
}

// DefaultConfig returns a MigrationConfig with sensible defaults for optional fields.
//
// REQUIRED fields that must be set explicitly by the caller:
//   - Component: identifies the service (e.g., "transaction", "onboarding")
//   - MigrationsPath: filesystem path to migration files (e.g., "/app/migrations")
//
// Example usage:
//
//	cfg := mmigration.DefaultConfig()
//	cfg.Component = "my-service"
//	cfg.MigrationsPath = "/app/components/my-service/migrations"
//	wrapper, err := mmigration.NewMigrationWrapper(conn, cfg, logger)
func DefaultConfig() MigrationConfig {
	return MigrationConfig{
		AutoRecoverDirty:      true,
		MaxRetries:            defaultMaxRetries,
		MaxRecoveryPerVersion: defaultMaxRecoveryPerVersion,
		RetryBackoff:          defaultRetryBackoffSeconds * time.Second,
		MaxBackoff:            defaultMaxBackoffSeconds * time.Second,
		LockTimeout:           defaultLockTimeoutSeconds * time.Second,
		// Component and MigrationsPath are intentionally left as zero values.
		// These are REQUIRED fields that must be set explicitly by the caller.
	}
}

// MigrationStatus represents the current state of migrations.
type MigrationStatus struct {
	// Version is the current migration version number.
	Version int

	// Dirty indicates if the migration is in a dirty state.
	Dirty bool

	// LastChecked is when the status was last verified.
	LastChecked time.Time

	// RecoveryAttempts is the number of recovery attempts made.
	RecoveryAttempts int

	// LastError is the most recent migration error, if any.
	LastError error
}

// IsHealthy returns true if migrations are in a healthy state.
func (s MigrationStatus) IsHealthy() bool {
	return !s.Dirty && s.LastError == nil
}

// MigrationWrapper wraps PostgresConnection with migration safety features.
type MigrationWrapper struct {
	conn   *libPostgres.PostgresConnection
	config MigrationConfig
	logger libLog.Logger

	// status holds the current migration status (thread-safe access via mu)
	status MigrationStatus
	mu     sync.RWMutex

	// recoveryAttemptsPerVersion tracks recovery attempts per migration version.
	// This prevents infinite boot loops when a migration has a permanent bug.
	recoveryAttemptsPerVersion map[int]int

	// metrics for observability
	recoveryCount    int64
	lastRecoveryTime time.Time
}

// NewMigrationWrapper creates a new MigrationWrapper with the given configuration.
//
// Required fields (will return error if not set):
//   - Component: identifies the service (used for advisory lock namespacing and logging)
//   - MigrationsPath: filesystem path to migration files (used for file validation before recovery)
//
// Recommended usage pattern:
//
//	cfg := mmigration.DefaultConfig()
//	cfg.Component = "my-service"
//	cfg.MigrationsPath = "/app/components/my-service/migrations"
//	wrapper, err := mmigration.NewMigrationWrapper(conn, cfg, logger)
func NewMigrationWrapper(conn *libPostgres.PostgresConnection, config MigrationConfig, logger libLog.Logger) (*MigrationWrapper, error) {
	// Validate required configuration
	if config.MigrationsPath == "" {
		return nil, ErrMissingMigrationsPath
	}

	if config.Component == "" {
		return nil, ErrMissingComponent
	}

	if config.MaxRetries <= 0 {
		config.MaxRetries = defaultMaxRetries
	}

	if config.MaxRecoveryPerVersion <= 0 {
		config.MaxRecoveryPerVersion = defaultMaxRecoveryPerVersion
	}

	if config.RetryBackoff <= 0 {
		config.RetryBackoff = defaultRetryBackoffSeconds * time.Second
	}

	if config.MaxBackoff <= 0 {
		config.MaxBackoff = defaultMaxBackoffSeconds * time.Second
	}

	if config.LockTimeout <= 0 {
		config.LockTimeout = defaultLockTimeoutSeconds * time.Second
	}

	return &MigrationWrapper{
		conn:                       conn,
		config:                     config,
		logger:                     logger,
		recoveryAttemptsPerVersion: make(map[int]int),
		status: MigrationStatus{
			LastChecked: time.Now(),
		},
	}, nil
}

// GetStatus returns the current migration status (thread-safe).
func (w *MigrationWrapper) GetStatus() MigrationStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.status
}

// updateStatus updates the migration status (thread-safe).
func (w *MigrationWrapper) updateStatus(fn func(*MigrationStatus)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	fn(&w.status)
	w.status.LastChecked = time.Now()
}

// preflightCheckQuery is the SQL to check migration status.
// ORDER BY ensures we get the latest version if multiple rows exist (corruption scenario).
const preflightCheckQuery = `SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`

// PreflightCheck queries the schema_migrations table to determine current state.
// Returns MigrationStatus with version and dirty flag.
// Returns ErrMigrationDirty if dirty=true (caller should handle recovery).
// Returns nil error with Version=0 if schema_migrations table doesn't exist (fresh DB).
func (w *MigrationWrapper) PreflightCheck(ctx context.Context, db *sql.DB) (MigrationStatus, error) {
	status := MigrationStatus{
		LastChecked: time.Now(),
	}

	row := db.QueryRowContext(ctx, preflightCheckQuery)

	err := row.Scan(&status.Version, &status.Dirty)
	if err != nil {
		// Check for context cancellation first
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return status, fmt.Errorf("%w: %w", ErrContextCanceled, err)
		}

		// No rows means fresh database or no migrations run yet
		if errors.Is(err, sql.ErrNoRows) {
			w.logger.Infof("No schema_migrations found for %s - fresh database", w.config.Component)
			return status, nil
		}

		// Check if table doesn't exist using driver-aware error detection.
		// PostgreSQL error code 42P01 = undefined_table (relation does not exist).
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "42P01" {
			w.logger.Infof("schema_migrations table doesn't exist for %s (code=%s) - fresh database",
				w.config.Component, pqErr.Code)

			return status, nil
		}

		// All other errors should propagate - connection issues, auth failures, non-pq errors, etc.
		return status, fmt.Errorf("%w: %w", ErrQuerySchemaMigrations, err)
	}

	w.logger.Infof("Migration preflight check for %s: version=%d, dirty=%v",
		w.config.Component, status.Version, status.Dirty)

	// Update internal status
	w.updateStatus(func(s *MigrationStatus) {
		s.Version = status.Version
		s.Dirty = status.Dirty
	})

	if status.Dirty {
		status.LastError = ErrMigrationDirty
		return status, ErrMigrationDirty
	}

	return status, nil
}

// Advisory lock constants.
const (
	// migrationLockNamespace is the base namespace for advisory locks.
	// This ensures migration locks don't conflict with other application locks.
	migrationLockNamespace uint64 = 0x4D494752 // "MIGR" in hex

	// FNV-1a 64-bit constants for robust, non-overflowing hash computation.
	// See: https://en.wikipedia.org/wiki/Fowler–Noll–Vo_hash_function
	fnvOffsetBasis uint64 = 14695981039346656037
	fnvPrime       uint64 = 1099511628211
)

// advisoryLockKey generates a unique lock key for this component's migrations.
// The key combines a namespace with a hash of the component name to ensure
// different services don't interfere with each other's migration locks.
// Uses FNV-1a hash for deterministic, overflow-safe computation.
func (w *MigrationWrapper) advisoryLockKey() int64 {
	// Compute FNV-1a hash over the component name bytes.
	// uint64 arithmetic naturally wraps on overflow, which is safe for hashing.
	hash := fnvOffsetBasis
	for i := 0; i < len(w.config.Component); i++ {
		hash ^= uint64(w.config.Component[i])
		hash *= fnvPrime
	}

	// Combine with namespace using XOR and cast to int64 for PostgreSQL advisory lock.
	// Mask to MaxInt64 to preserve entropy while ensuring safe uint64 to int64 conversion.
	combined := migrationLockNamespace ^ hash
	combined &= uint64(math.MaxInt64)

	return int64(combined) //nolint:gosec // G115: combined masked to MaxInt64, safe conversion
}

// staleLockQuery queries pg_stat_activity to find who holds an advisory lock.
const staleLockQuery = `
SELECT pid, usename, application_name, backend_start
FROM pg_stat_activity
WHERE pid IN (
    SELECT pid FROM pg_locks WHERE locktype = 'advisory' AND objid = $1
)
LIMIT 1
`

// lockRetryInterval is the interval between lock acquisition attempts.
const lockRetryInterval = 500 * time.Millisecond

// AcquireAdvisoryLock attempts to acquire a PostgreSQL advisory lock for migrations.
// This prevents multiple pods from running migrations simultaneously.
// Uses pg_try_advisory_lock in a retry loop that respects LockTimeout.
func (w *MigrationWrapper) AcquireAdvisoryLock(ctx context.Context, db *sql.DB) error {
	lockKey := w.advisoryLockKey()

	w.logger.Infof("Attempting to acquire migration advisory lock for %s (key=%d, timeout=%v)",
		w.config.Component, lockKey, w.config.LockTimeout)

	lockCtx, cancel := context.WithTimeout(ctx, w.config.LockTimeout)
	defer cancel()

	ticker := time.NewTicker(lockRetryInterval)
	defer ticker.Stop()

	attempt := 0
	for {
		attempt++

		var acquired bool

		err := db.QueryRowContext(lockCtx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
		if err != nil {
			// Check if the error is due to context timeout/cancellation
			if lockCtx.Err() != nil {
				w.logger.Warnf("Timed out waiting for migration advisory lock for %s after %d attempts: %v",
					w.config.Component, attempt, lockCtx.Err())
				w.logStaleLockHolder(ctx, db, lockKey)

				return fmt.Errorf("%w: timeout after %v (%d attempts)", ErrMigrationLockFailed, w.config.LockTimeout, attempt)
			}

			w.logger.Errorf("Failed to query advisory lock for %s (attempt %d): %v", w.config.Component, attempt, err)

			return fmt.Errorf("%w: %w", ErrMigrationLockFailed, err)
		}

		if acquired {
			w.logger.Infof("Successfully acquired migration advisory lock for %s (attempt %d)", w.config.Component, attempt)
			return nil
		}

		// Log on first failed attempt to show who holds the lock
		if attempt == 1 {
			w.logger.Infof("Migration advisory lock for %s is held by another process, waiting...", w.config.Component)
			w.logStaleLockHolder(ctx, db, lockKey)
		}

		// Wait for retry interval or context expiration
		select {
		case <-lockCtx.Done():
			w.logger.Warnf("Timed out waiting for migration advisory lock for %s after %d attempts: %v",
				w.config.Component, attempt, lockCtx.Err())

			return fmt.Errorf("%w: timeout after %v (%d attempts)", ErrMigrationLockFailed, w.config.LockTimeout, attempt)
		case <-ticker.C:
			// Continue to next attempt
		}
	}
}

// logStaleLockHolder queries pg_stat_activity to log information about who holds the lock.
// TODO(review): Consider reducing sensitive info in logs (PID, username, app_name)
// to avoid information disclosure. See security review.
func (w *MigrationWrapper) logStaleLockHolder(ctx context.Context, db *sql.DB, lockKey int64) {
	var (
		pid               int
		username, appName sql.NullString
		backendStart      sql.NullTime
	)

	err := db.QueryRowContext(ctx, staleLockQuery, lockKey).Scan(&pid, &username, &appName, &backendStart)
	if err != nil {
		w.logger.Warnf("Migration advisory lock for %s is held by another process (could not identify holder: %v)",
			w.config.Component, err)

		return
	}

	w.logger.Warnf("Migration advisory lock for %s is held by: PID=%d, user=%s, app=%s, since=%v",
		w.config.Component, pid, username.String, appName.String, backendStart.Time)
}

// ReleaseAdvisoryLock releases the PostgreSQL advisory lock for migrations.
func (w *MigrationWrapper) ReleaseAdvisoryLock(ctx context.Context, db *sql.DB) error {
	lockKey := w.advisoryLockKey()

	w.logger.Infof("Releasing migration advisory lock for %s (key=%d)",
		w.config.Component, lockKey)

	var released bool

	err := db.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey).Scan(&released)
	if err != nil {
		w.logger.Errorf("Failed to release advisory lock for %s: %v", w.config.Component, err)
		return fmt.Errorf("%w: %w", ErrAdvisoryLockRelease, err)
	}

	if !released {
		w.logger.Warnf("Advisory lock for %s was not held (already released or never acquired)",
			w.config.Component)
	} else {
		w.logger.Infof("Successfully released migration advisory lock for %s", w.config.Component)
	}

	return nil
}

// clearDirtyFlagQuery is the SQL to clear the dirty flag for a specific version.
// SECURITY: This query ONLY clears dirty flag, it NEVER modifies the version.
const clearDirtyFlagQuery = `UPDATE schema_migrations SET dirty = false WHERE version = $1 AND dirty = true`

// ErrMaxRecoveryPerVersionExceeded indicates the per-version recovery limit was reached.
var ErrMaxRecoveryPerVersionExceeded = errors.New("maximum recovery attempts exceeded for this migration version")

// recoverDirtyMigration attempts to recover from a dirty migration state.
// This ONLY clears the dirty flag - it NEVER modifies the migration version.
//
// PREREQUISITES:
//   - Migrations MUST be idempotent (use IF NOT EXISTS, IF EXISTS, etc.)
//
// SECURITY CONSTRAINTS:
//   - Only clears dirty flag for specific version, never modifies version
//   - Only executes if AutoRecoverDirty is enabled
//   - Validates migration file exists before recovery
//   - Enforces per-version recovery limit to prevent infinite loops
func (w *MigrationWrapper) recoverDirtyMigration(ctx context.Context, db *sql.DB, version int) error {
	// Check if auto-recovery is enabled
	if !w.config.AutoRecoverDirty {
		w.logger.Errorf("Migration for %s is dirty at version %d but auto-recovery is DISABLED. "+
			"Manual intervention required.",
			w.config.Component, version)

		return fmt.Errorf("%w: auto-recovery disabled for %s at version %d",
			ErrMigrationRecoveryFailed, w.config.Component, version)
	}

	// Check per-version recovery limit
	w.mu.Lock()

	attempts := w.recoveryAttemptsPerVersion[version]
	if attempts >= w.config.MaxRecoveryPerVersion {
		w.mu.Unlock()
		w.logger.Errorf("CRITICAL: Migration version %d for %s has failed recovery %d times. "+
			"Maximum attempts (%d) exceeded. Manual intervention required.",
			version, w.config.Component, attempts, w.config.MaxRecoveryPerVersion)

		w.logger.WithFields(
			"event", "migration_recovery_limit_exceeded",
			"component", w.config.Component,
			"version", version,
			"attempts", attempts,
			"max_attempts", w.config.MaxRecoveryPerVersion,
		).Error("Migration recovery limit exceeded - service cannot start")

		return fmt.Errorf("%w: version %d has failed %d recovery attempts for %s",
			ErrMaxRecoveryPerVersionExceeded, version, attempts, w.config.Component)
	}

	w.recoveryAttemptsPerVersion[version] = attempts + 1
	w.mu.Unlock()

	// Validate migration file exists before attempting recovery
	if err := w.validateMigrationFileExists(version); err != nil {
		w.logger.Errorf("Cannot recover migration %d for %s: %v", version, w.config.Component, err)
		return err
	}

	w.logger.WithFields(
		"event", "migration_recovery_attempt",
		"component", w.config.Component,
		"version", version,
		"attempt", attempts+1,
		"max_attempts", w.config.MaxRecoveryPerVersion,
	).Warn("Attempting automatic recovery of dirty migration")

	w.logger.Warnf("ALERT: Attempting automatic recovery of dirty migration for %s at version %d (attempt %d/%d).",
		w.config.Component, version, attempts+1, w.config.MaxRecoveryPerVersion)

	// Execute the recovery - ONLY clears dirty flag for specific version
	result, err := db.ExecContext(ctx, clearDirtyFlagQuery, version)
	if err != nil {
		w.logger.Errorf("Failed to clear dirty flag for %s at version %d: %v", w.config.Component, version, err)
		return fmt.Errorf("%w: %w", ErrClearDirtyFlag, err)
	}

	rowsAffected, err := result.RowsAffected()

	switch {
	case err != nil:
		w.logger.Warnf("Could not determine rows affected for %s dirty recovery: %v",
			w.config.Component, err)
	case rowsAffected == 0:
		w.logger.Warnf("No rows affected when clearing dirty flag for %s at version %d - "+
			"migration may have already been recovered", w.config.Component, version)
	default:
		w.logger.WithFields(
			"event", "migration_recovery_success",
			"component", w.config.Component,
			"version", version,
			"attempt", attempts+1,
		).Info("Successfully cleared dirty flag")

		w.logger.Infof("Successfully cleared dirty flag for %s migration at version %d",
			w.config.Component, version)
	}

	// Update metrics
	w.mu.Lock()
	w.recoveryCount++
	w.lastRecoveryTime = time.Now()
	w.status.RecoveryAttempts++
	w.mu.Unlock()

	return nil
}

// validateMigrationFileExists checks that the migration file for the given version exists.
func (w *MigrationWrapper) validateMigrationFileExists(version int) error {
	pattern := filepath.Join(w.config.MigrationsPath, fmt.Sprintf("%06d_*.up.sql", version))

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSearchMigrationFile, err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("%w: no migration file found matching %s - cannot auto-recover",
			ErrMigrationFileNotFound, pattern)
	}

	w.logger.Infof("Found migration file for version %d: %s", version, filepath.Base(matches[0]))

	return nil
}

// handleDirtyRecovery handles the recovery process for dirty migrations.
// It attempts recovery, then verifies the dirty flag was cleared.
// Returns nil on success, or an error if recovery or verification failed.
func (w *MigrationWrapper) handleDirtyRecovery(ctx context.Context, rawDB *sql.DB, version int) error {
	// Attempt recovery
	if recoveryErr := w.recoverDirtyMigration(ctx, rawDB, version); recoveryErr != nil {
		return fmt.Errorf("%w: %w", ErrMigrationRecoveryFailed, recoveryErr)
	}

	// Re-verify migration state after recovery to confirm dirty flag is cleared
	w.logger.Infof("Re-verifying migration state after recovery for %s", w.config.Component)

	verifyStatus, verifyErr := w.PreflightCheck(ctx, rawDB)
	if verifyErr != nil {
		// If verifyErr is ErrMigrationDirty, dirty flag is still set
		if errors.Is(verifyErr, ErrMigrationDirty) {
			return fmt.Errorf("%w: dirty flag still set at version %d for %s after recovery attempt",
				ErrPostRecoveryVerification, verifyStatus.Version, w.config.Component)
		}

		return fmt.Errorf("%w for %s: %w", ErrPostRecoveryVerification, w.config.Component, verifyErr)
	}

	// Double-check dirty flag even if no error (defensive check)
	if verifyStatus.Dirty {
		return fmt.Errorf("%w: dirty flag unexpectedly still set at version %d for %s",
			ErrPostRecoveryVerification, verifyStatus.Version, w.config.Component)
	}

	w.logger.Infof("Post-recovery verification successful for %s: dirty flag cleared, proceeding with GetDB()",
		w.config.Component)

	return nil
}

// cleanupRawConnection releases the advisory lock and closes the raw connection.
// Errors are logged but not returned since these are cleanup operations.
func (w *MigrationWrapper) cleanupRawConnection(ctx context.Context, rawDB *sql.DB) {
	// Note: Advisory locks are session-scoped and auto-released when rawDB closes.
	// We log failures for debugging but don't fail the operation since connection closure
	// will release the lock anyway.
	if releaseErr := w.ReleaseAdvisoryLock(ctx, rawDB); releaseErr != nil {
		w.logger.Warnf("Failed to release migration lock for %s (will auto-release on close): %v",
			w.config.Component, releaseErr)
	}

	if closeErr := rawDB.Close(); closeErr != nil {
		w.logger.Warnf("Failed to close raw connection for %s: %v",
			w.config.Component, closeErr)
	}
}

// SafeGetDB wraps PostgresConnection.GetDB() with migration safety features.
// This is the primary entry point for components to get a database connection
// with migration protection.
//
// Workflow:
//  1. Acquire advisory lock (prevents concurrent migrations)
//  2. Run preflight check (detect dirty state)
//  3. If dirty and AutoRecoverDirty=true: recover and retry
//  4. Call underlying GetDB() which runs migrations
//  5. Release advisory lock
//  6. Return database connection
func (w *MigrationWrapper) SafeGetDB(ctx context.Context) (dbresolver.DB, error) {
	w.logger.Infof("SafeGetDB starting for %s", w.config.Component)

	// Get a raw connection for preflight checks (bypasses migration)
	rawDB, err := w.getRawConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: preflight connection: %w", ErrRawConnection, err)
	}

	// Acquire advisory lock
	if err := w.AcquireAdvisoryLock(ctx, rawDB); err != nil {
		w.cleanupRawConnection(ctx, rawDB)
		return nil, fmt.Errorf("%w: %w", ErrMigrationLockFailed, err)
	}

	// Preflight check and handle dirty state if needed
	status, err := w.PreflightCheck(ctx, rawDB)
	if err != nil {
		// Handle non-dirty errors first (early return reduces nesting)
		if !errors.Is(err, ErrMigrationDirty) {
			w.cleanupRawConnection(ctx, rawDB)
			return nil, fmt.Errorf("%w: %w", ErrPreflightCheck, err)
		}

		// Handle dirty migration recovery
		if recoveryErr := w.handleDirtyRecovery(ctx, rawDB, status.Version); recoveryErr != nil {
			w.cleanupRawConnection(ctx, rawDB)
			return nil, recoveryErr
		}
	}

	// Call the underlying GetDB which runs migrations
	w.logger.Infof("Calling underlying GetDB() for %s", w.config.Component)

	db, err := w.conn.GetDB()

	w.cleanupRawConnection(ctx, rawDB)

	if err != nil {
		w.updateStatus(func(s *MigrationStatus) {
			s.LastError = err
		})

		return nil, fmt.Errorf("%w for %s: %w", ErrGetDB, w.config.Component, err)
	}

	// Update status to healthy
	w.updateStatus(func(s *MigrationStatus) {
		s.Dirty = false
		s.LastError = nil
	})

	w.logger.Infof("SafeGetDB completed successfully for %s", w.config.Component)

	return db, nil
}

// getRawConnection gets a raw database connection for preflight checks.
// This connects directly without triggering migrations.
func (w *MigrationWrapper) getRawConnection(ctx context.Context) (*sql.DB, error) {
	connStr := w.conn.ConnectionStringPrimary
	if connStr == "" {
		return nil, ErrNoConnectionString
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("%w: open: %w", ErrRawConnection, err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%w: ping: %w", ErrRawConnection, err)
	}

	return db, nil
}

// GetConnection returns the underlying PostgresConnection.
func (w *MigrationWrapper) GetConnection() *libPostgres.PostgresConnection {
	return w.conn
}

// calculateBackoff calculates the backoff duration for a given attempt.
// Uses exponential backoff: baseBackoff * 2^attempt, capped at MaxBackoff.
func (w *MigrationWrapper) calculateBackoff(attempt int) time.Duration {
	multiplier := 1 << attempt // 2^attempt
	backoff := w.config.RetryBackoff * time.Duration(multiplier)

	if backoff > w.config.MaxBackoff {
		return w.config.MaxBackoff
	}

	return backoff
}

// shouldRetry returns true if another retry attempt should be made.
func (w *MigrationWrapper) shouldRetry(attempt int) bool {
	return attempt < w.config.MaxRetries
}

// isRetryableError returns true if the error warrants a retry.
func (w *MigrationWrapper) isRetryableError(err error) bool {
	// Never retry context errors
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Never retry if we've exceeded retries
	if errors.Is(err, ErrMaxRetriesExceeded) {
		return false
	}

	// Retry dirty migrations and lock failures
	if errors.Is(err, ErrMigrationDirty) || errors.Is(err, ErrMigrationLockFailed) {
		return true
	}

	return false
}

// SafeGetDBWithRetry wraps SafeGetDB with retry logic and exponential backoff.
func (w *MigrationWrapper) SafeGetDBWithRetry(ctx context.Context) (dbresolver.DB, error) {
	var lastErr error

	for attempt := 0; w.shouldRetry(attempt); attempt++ {
		if attempt > 0 {
			backoff := w.calculateBackoff(attempt - 1)
			w.logger.Warnf("Retry attempt %d/%d for %s after %v backoff",
				attempt, w.config.MaxRetries, w.config.Component, backoff)

			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %w", ErrContextCanceled, ctx.Err())
			case <-time.After(backoff):
			}
		}

		db, err := w.SafeGetDB(ctx)
		if err == nil {
			return db, nil
		}

		lastErr = err

		if !w.isRetryableError(err) {
			w.logger.Errorf("Non-retryable error for %s: %v", w.config.Component, err)
			return nil, err
		}

		w.logger.Warnf("Retryable error for %s (attempt %d/%d): %v",
			w.config.Component, attempt+1, w.config.MaxRetries, err)
	}

	return nil, fmt.Errorf("%w: %w", ErrMaxRetriesExceeded, lastErr)
}
