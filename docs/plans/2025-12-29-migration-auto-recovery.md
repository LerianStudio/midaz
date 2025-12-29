# Migration Auto-Recovery System Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Implement a migration auto-recovery system that prevents service crashes when `schema_migrations.dirty=true` by providing safe dirty flag recovery, advisory locks for concurrent migration protection, and comprehensive observability.

**Architecture:** Create a new `pkg/mmigration` package that wraps lib-commons PostgresConnection with preflight checks and safe recovery. The wrapper intercepts GetDB() calls, checks schema_migrations status, acquires advisory locks, and either proceeds normally or recovers from dirty state. Both transaction and onboarding components will use this wrapper through their bootstrap configurations.

**Key Design Decisions (from Code Review):**
- **Idempotent Migrations Required:** All migrations MUST use idempotent patterns (`IF NOT EXISTS`, `IF EXISTS`, etc.). This is a prerequisite for safe auto-recovery.
- **Per-Version Recovery Limit:** Maximum 3 recovery attempts per migration version per pod. After 3 failures, service refuses to start (requires manual intervention).
- **In-Memory Recovery Counter:** The per-version recovery counter resets on pod restart. This is intentional - each pod instance gets 3 attempts. Kubernetes pod restart limits handle runaway pods. This avoids database complexity while still preventing immediate infinite loops.
- **File Validation:** Migration file must exist at configured path before recovery proceeds.
- **Connection Cleanup:** Raw preflight connections are explicitly closed after `GetDB()` completes to prevent connection pool exhaustion. Lock must be held during migration execution.
- **Stale Lock Detection:** On lock acquisition timeout, query `pg_stat_activity` to log which process holds the lock (internal operational info only).
- **Backoff Cap:** Exponential backoff capped at 30 seconds maximum.
- **Health Endpoint:** Minimal response (healthy/unhealthy boolean only).
- **Audit Logging:** Recovery operations logged as structured JSON via mlog.
- **Absolute Migration Paths:** Use `/app/components/{service}/migrations` for container environments.

**Tech Stack:**
- Go 1.21+
- golang-migrate v4.19.1 (via lib-commons v2.6.0-beta)
- PostgreSQL 14+ (advisory locks, schema_migrations table)
- Fiber (health endpoint enhancements)
- OpenTelemetry (metrics and tracing)

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: Verify with commands below
- Access: PostgreSQL database connection, lib-commons v2.6.0-beta
- State: Working from branch `fix/fred-several-ones-dec-13-2025`, clean working tree

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version           # Expected: go version go1.21+
psql --version       # Expected: psql (PostgreSQL) 14+
git status           # Expected: clean working tree on fix/fred-several-ones-dec-13-2025
ls pkg/              # Expected: assert, dbtx, mlog, etc.
```

## Historical Precedent

**Query:** "migration postgres recovery retry"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task Overview

| Task | Description | Duration | Dependencies |
|------|-------------|----------|--------------|
| 1 | Create mmigration package directory structure | 2 min | None |
| 2 | Write MigrationConfig struct and types | 3 min | Task 1 |
| 3 | Write failing test for PreflightCheck | 3 min | Task 2 |
| 4 | Implement PreflightCheck function | 4 min | Task 3 |
| 5 | Write failing test for advisory lock acquisition | 3 min | Task 4 |
| 6 | Implement advisory lock functions | 4 min | Task 5 |
| 7 | Write failing test for dirty migration recovery | 3 min | Task 6 |
| 8 | Implement recoverDirtyMigration function | 4 min | Task 7 |
| 9 | Write failing test for SafeGetDB | 3 min | Task 8 |
| 10 | Implement SafeGetDB wrapper | 5 min | Task 9 |
| 11 | Write failing test for retry logic with backoff | 3 min | Task 10 |
| 12 | Implement retry logic with exponential backoff | 4 min | Task 11 |
| 13 | Run Code Review Checkpoint 1 | 5 min | Task 12 |
| 14 | Add MigrationStatus type for health reporting | 3 min | Task 13 |
| 15 | Write MigrationHealthHandler for health endpoint | 4 min | Task 14 |
| 16 | Add metrics registration (prometheus) | 4 min | Task 15 |
| 17 | Update transaction bootstrap Config struct | 3 min | Task 16 |
| 18 | Update transaction bootstrap InitServers | 5 min | Task 17 |
| 19 | Update onboarding bootstrap Config struct | 3 min | Task 18 |
| 20 | Update onboarding bootstrap InitServers | 5 min | Task 19 |
| 21 | Run Code Review Checkpoint 2 | 5 min | Task 20 |
| 22 | Create integration test for dirty recovery scenario | 5 min | Task 21 |
| 23 | Add documentation comments and examples | 3 min | Task 22 |
| 24 | Run full test suite and lint | 5 min | Task 23 |
| 25 | Final Code Review Checkpoint | 5 min | Task 24 |

---

## Task 1: Create mmigration package directory structure

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Tools: Go 1.21+
- Directory must exist: `pkg/`

**Step 1: Create the package directory**

Run: `mkdir -p /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration`

**Expected output:**
```
(no output - directory created silently)
```

**Step 2: Create the initial migration.go file**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go
// Package mmigration provides migration management utilities with auto-recovery support.
// It wraps lib-commons PostgresConnection to add preflight checks, dirty state recovery,
// advisory locks for concurrent protection, and comprehensive observability.
//
// Key features:
//   - PreflightCheck: Validates schema_migrations state before running migrations
//   - SafeGetDB: Wrapper with automatic dirty recovery and retry logic
//   - Advisory locks: Prevents concurrent migration runs across pods
//   - Metrics: Exposes migration_duration_seconds and migration_recovery_total
//
// Usage:
//
//	migrationCfg := mmigration.DefaultConfig()
//	wrapper := mmigration.NewMigrationWrapper(postgresConnection, migrationCfg, logger)
//	db, err := wrapper.SafeGetDB(ctx)
package mmigration
```

**Step 3: Verify the file was created**

Run: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/`

**Expected output:**
```
total 8
drwxr-xr-x  3 user  staff    96 Dec 29 XX:XX .
drwxr-xr-x  X user  staff   XXX Dec 29 XX:XX ..
-rw-r--r--  1 user  staff   XXX Dec 29 XX:XX migration.go
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/
git commit -m "$(cat <<'EOF'
feat(mmigration): create package structure for migration auto-recovery

Initialize the mmigration package that will provide migration management
utilities with auto-recovery support for dirty migrations.
EOF
)"
```

**If Task Fails:**

1. **Directory already exists:**
   - Check: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/`
   - Fix: Continue to next step if directory exists
   - Rollback: Not needed

2. **Permission denied:**
   - Check: Verify write permissions on pkg/
   - Fix: `chmod u+w /Users/fredamaral/repos/lerianstudio/midaz/pkg/`
   - Rollback: N/A

---

## Task 2: Write MigrationConfig struct and types

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 1 completed
- File exists: `pkg/mmigration/migration.go`

**Step 1: Add imports and types to migration.go**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/bxcodec/dbresolver/v2"
	_ "github.com/lib/pq" // PostgreSQL driver for raw connections
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

// DefaultConfig returns a MigrationConfig with sensible defaults.
// Note: MigrationsPath must be set explicitly by the caller.
func DefaultConfig() MigrationConfig {
	return MigrationConfig{
		AutoRecoverDirty:      true,
		MaxRetries:            3,
		MaxRecoveryPerVersion: 3,
		RetryBackoff:          1 * time.Second,
		MaxBackoff:            30 * time.Second,
		LockTimeout:           30 * time.Second,
		Component:             "unknown",
		MigrationsPath:        "", // Must be set explicitly
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
// Returns error if MigrationsPath is not configured (required for file validation).
func NewMigrationWrapper(conn *libPostgres.PostgresConnection, config MigrationConfig, logger libLog.Logger) (*MigrationWrapper, error) {
	// Validate required configuration
	if config.MigrationsPath == "" {
		return nil, errors.New("MigrationsPath is required but not configured")
	}

	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}

	if config.MaxRecoveryPerVersion <= 0 {
		config.MaxRecoveryPerVersion = 3
	}

	if config.RetryBackoff <= 0 {
		config.RetryBackoff = 1 * time.Second
	}

	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 30 * time.Second
	}

	if config.LockTimeout <= 0 {
		config.LockTimeout = 30 * time.Second
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
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmigration/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run go vet**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go vet ./pkg/mmigration/...`

**Expected output:**
```
(no output - no issues found)
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
feat(mmigration): add MigrationConfig and MigrationWrapper types

Add configuration struct with sensible defaults for auto-recovery,
retry logic with exponential backoff, advisory lock support, and
MigrationWrapper that will provide safe database access.
EOF
)"
```

**If Task Fails:**

1. **Import errors:**
   - Check: Ensure lib-commons v2.6.0-beta is in go.mod
   - Fix: `go get github.com/LerianStudio/lib-commons/v2@v2.6.0-beta`
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

2. **Compilation errors:**
   - Check: Run `go build ./pkg/mmigration/... 2>&1` for details
   - Fix: Address syntax errors shown
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

---

## Task 3: Write failing test for PreflightCheck

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`

**Prerequisites:**
- Task 2 completed
- File exists: `pkg/mmigration/migration.go`

**Step 1: Create the test file with PreflightCheck tests**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go
package mmigration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger implements libLog.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) Info(args ...any)                        {}
func (m *mockLogger) Infof(format string, args ...any)        {}
func (m *mockLogger) Warn(args ...any)                        {}
func (m *mockLogger) Warnf(format string, args ...any)        {}
func (m *mockLogger) Error(args ...any)                       {}
func (m *mockLogger) Errorf(format string, args ...any)       {}
func (m *mockLogger) Debug(args ...any)                       {}
func (m *mockLogger) Debugf(format string, args ...any)       {}
func (m *mockLogger) Fatal(args ...any)                       {}
func (m *mockLogger) Fatalf(format string, args ...any)       {}
func (m *mockLogger) WithFields(fields ...any) interface {
	Info(args ...any)
	Infof(format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Debug(args ...any)
	Debugf(format string, args ...any)
} {
	return m
}

// newTestWrapper creates a MigrationWrapper with all required fields initialized for testing.
// This helper ensures tests don't bypass constructor validation and avoids nil map panics.
// Use this instead of creating &MigrationWrapper{} directly in tests.
func newTestWrapper(t *testing.T, config MigrationConfig) *MigrationWrapper {
	t.Helper()

	// Apply defaults if not set
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.MaxRecoveryPerVersion <= 0 {
		config.MaxRecoveryPerVersion = 3
	}
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = 1 * time.Second
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 30 * time.Second
	}
	if config.LockTimeout <= 0 {
		config.LockTimeout = 5 * time.Second
	}
	if config.MigrationsPath == "" {
		// Create temp directory for test migrations
		tmpDir := t.TempDir()
		config.MigrationsPath = tmpDir
	}

	return &MigrationWrapper{
		config:                     config,
		logger:                     &mockLogger{},
		recoveryAttemptsPerVersion: make(map[int]int), // CRITICAL: Initialize map
		status: MigrationStatus{
			LastChecked: time.Now(),
		},
	}
}

func TestPreflightCheck_CleanMigration(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect query for schema_migrations
	rows := sqlmock.NewRows([]string{"version", "dirty"}).
		AddRow(18, false)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnRows(rows)

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: &mockLogger{},
	}

	// Execute preflight check
	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, 18, status.Version)
	assert.False(t, status.Dirty)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCheck_DirtyMigration(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect query for schema_migrations with dirty=true
	rows := sqlmock.NewRows([]string{"version", "dirty"}).
		AddRow(15, true)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnRows(rows)

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: &mockLogger{},
	}

	// Execute preflight check
	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	// Verify - should return status with dirty=true and ErrMigrationDirty
	assert.ErrorIs(t, err, ErrMigrationDirty)
	assert.Equal(t, 15, status.Version)
	assert.True(t, status.Dirty)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCheck_NoMigrationsTable(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect query to return no rows (table doesn't exist or empty)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnError(sql.ErrNoRows)

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: &mockLogger{},
	}

	// Execute preflight check
	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	// Verify - no error, version 0, not dirty (fresh database)
	assert.NoError(t, err)
	assert.Equal(t, 0, status.Version)
	assert.False(t, status.Dirty)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCheck_ContextCanceled(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Expect query to be canceled
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnError(context.Canceled)

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: &mockLogger{},
	}

	// Execute preflight check
	_, err = wrapper.PreflightCheck(ctx, db)

	// Verify - should return context.Canceled
	assert.ErrorIs(t, err, context.Canceled)
}
```

**Step 2: Run the test (expect failure)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestPreflightCheck`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmigration [github.com/LerianStudio/midaz/v3/pkg/mmigration.test]
pkg/mmigration/migration_test.go:XX:XX: wrapper.PreflightCheck undefined (type *MigrationWrapper has no field or method PreflightCheck)
FAIL    github.com/LerianStudio/midaz/v3/pkg/mmigration [build failed]
```

**If you see different error:** The test is correctly failing because PreflightCheck doesn't exist yet.

**Step 3: Commit the failing test**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go
git commit -m "$(cat <<'EOF'
test(mmigration): add failing tests for PreflightCheck

TDD: Write tests first for PreflightCheck function covering:
- Clean migration state (version N, dirty=false)
- Dirty migration state (version N, dirty=true)
- No migrations table (fresh database)
- Context cancellation handling
EOF
)"
```

**If Task Fails:**

1. **Import errors for sqlmock:**
   - Fix: `go get github.com/DATA-DOG/go-sqlmock`
   - Rollback: `git checkout -- pkg/mmigration/migration_test.go`

2. **Import errors for testify:**
   - Fix: `go get github.com/stretchr/testify`
   - Rollback: `git checkout -- pkg/mmigration/migration_test.go`

---

## Task 4: Implement PreflightCheck function

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 3 completed (failing tests exist)

**Step 1: Add PreflightCheck method to MigrationWrapper**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go

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
			return status, err
		}

		// No rows means fresh database or no migrations run yet
		if errors.Is(err, sql.ErrNoRows) {
			w.logger.Infof("No schema_migrations found for %s - fresh database", w.config.Component)
			return status, nil
		}

		// Other errors might be table doesn't exist - treat as fresh
		w.logger.Warnf("Could not query schema_migrations for %s: %v - treating as fresh database",
			w.config.Component, err)
		return status, nil
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
```

**Step 2: Run the tests (expect pass)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestPreflightCheck`

**Expected output:**
```
=== RUN   TestPreflightCheck_CleanMigration
--- PASS: TestPreflightCheck_CleanMigration (0.00s)
=== RUN   TestPreflightCheck_DirtyMigration
--- PASS: TestPreflightCheck_DirtyMigration (0.00s)
=== RUN   TestPreflightCheck_NoMigrationsTable
--- PASS: TestPreflightCheck_NoMigrationsTable (0.00s)
=== RUN   TestPreflightCheck_ContextCanceled
--- PASS: TestPreflightCheck_ContextCanceled (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 3: Run go vet**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go vet ./pkg/mmigration/...`

**Expected output:**
```
(no output - no issues)
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
feat(mmigration): implement PreflightCheck for migration status

PreflightCheck queries schema_migrations to determine current state:
- Returns version and dirty flag
- Returns ErrMigrationDirty when dirty=true
- Handles fresh database (no schema_migrations) gracefully
- Respects context cancellation
EOF
)"
```

**If Task Fails:**

1. **Tests still failing:**
   - Check: Run `go test ./pkg/mmigration/... -v 2>&1` for error details
   - Fix: Address any compilation or logic errors
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

2. **go vet errors:**
   - Check: Run `go vet ./pkg/mmigration/... 2>&1`
   - Fix: Address reported issues
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

---

## Task 5: Write failing test for advisory lock acquisition

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`

**Prerequisites:**
- Task 4 completed

**Step 1: Add advisory lock tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go

func TestAcquireAdvisoryLock_Success(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect advisory lock query - PostgreSQL pg_try_advisory_lock returns true on success
	rows := sqlmock.NewRows([]string{"pg_try_advisory_lock"}).
		AddRow(true)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component:   "transaction",
			LockTimeout: 5 * time.Second,
		},
		logger: &mockLogger{},
	}

	// Execute lock acquisition
	ctx := context.Background()
	err = wrapper.AcquireAdvisoryLock(ctx, db)

	// Verify
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcquireAdvisoryLock_AlreadyLocked(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect advisory lock query - returns false when lock is held by another process
	rows := sqlmock.NewRows([]string{"pg_try_advisory_lock"}).
		AddRow(false)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component:   "transaction",
			LockTimeout: 100 * time.Millisecond, // Short timeout for test
		},
		logger: &mockLogger{},
	}

	// Execute lock acquisition
	ctx := context.Background()
	err = wrapper.AcquireAdvisoryLock(ctx, db)

	// Verify - should fail with lock error
	assert.ErrorIs(t, err, ErrMigrationLockFailed)
}

func TestReleaseAdvisoryLock_Success(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect advisory unlock query
	rows := sqlmock.NewRows([]string{"pg_advisory_unlock"}).
		AddRow(true)
	mock.ExpectQuery("SELECT pg_advisory_unlock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component: "transaction",
		},
		logger: &mockLogger{},
	}

	// Execute lock release
	ctx := context.Background()
	err = wrapper.ReleaseAdvisoryLock(ctx, db)

	// Verify
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAdvisoryLockKey_DifferentComponents(t *testing.T) {
	// Test that different components get different lock keys
	wrapperTx := &MigrationWrapper{
		config: MigrationConfig{Component: "transaction"},
		logger: &mockLogger{},
	}
	wrapperOnb := &MigrationWrapper{
		config: MigrationConfig{Component: "onboarding"},
		logger: &mockLogger{},
	}

	keyTx := wrapperTx.advisoryLockKey()
	keyOnb := wrapperOnb.advisoryLockKey()

	// Keys should be different for different components
	assert.NotEqual(t, keyTx, keyOnb)
	// Keys should be consistent for same component
	assert.Equal(t, keyTx, wrapperTx.advisoryLockKey())
}
```

**Step 2: Run the test (expect failure)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestAdvisory`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmigration [github.com/LerianStudio/midaz/v3/pkg/mmigration.test]
pkg/mmigration/migration_test.go:XX: wrapper.AcquireAdvisoryLock undefined
pkg/mmigration/migration_test.go:XX: wrapper.ReleaseAdvisoryLock undefined
pkg/mmigration/migration_test.go:XX: wrapperTx.advisoryLockKey undefined
FAIL    github.com/LerianStudio/midaz/v3/pkg/mmigration [build failed]
```

**Step 3: Commit the failing test**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go
git commit -m "$(cat <<'EOF'
test(mmigration): add failing tests for advisory lock functions

TDD: Write tests for advisory lock acquisition and release:
- Successful lock acquisition
- Failed lock (already locked by another process)
- Lock release
- Different components get different lock keys
EOF
)"
```

**If Task Fails:**

1. **Unexpected compilation success:**
   - Check: Methods may already exist - verify with `grep -n "AcquireAdvisoryLock" pkg/mmigration/migration.go`
   - Fix: If methods exist, skip to Task 6 implementation
   - Rollback: N/A

---

## Task 6: Implement advisory lock functions

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 5 completed (failing tests exist)

**Step 1: Add advisory lock methods**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go

// Advisory lock constants.
const (
	// migrationLockNamespace is the base namespace for advisory locks.
	// This ensures migration locks don't conflict with other application locks.
	migrationLockNamespace int64 = 0x4D494752 // "MIGR" in hex
)

// advisoryLockKey generates a unique lock key for this component's migrations.
// The key combines a namespace with a hash of the component name to ensure
// different services don't interfere with each other's migration locks.
func (w *MigrationWrapper) advisoryLockKey() int64 {
	// Simple hash of component name combined with namespace
	var hash int64
	for _, c := range w.config.Component {
		hash = hash*31 + int64(c)
	}

	return migrationLockNamespace ^ hash
}

// staleLockQuery queries pg_stat_activity to find who holds an advisory lock.
// Used for debugging when lock acquisition fails.
const staleLockQuery = `
SELECT pid, usename, application_name, backend_start
FROM pg_stat_activity
WHERE pid IN (
    SELECT pid FROM pg_locks WHERE locktype = 'advisory' AND objid = $1
)
LIMIT 1
`

// AcquireAdvisoryLock attempts to acquire a PostgreSQL advisory lock for migrations.
// This prevents multiple pods from running migrations simultaneously.
// Uses pg_try_advisory_lock which is non-blocking and session-scoped.
// On failure, queries pg_stat_activity to log information about the lock holder.
func (w *MigrationWrapper) AcquireAdvisoryLock(ctx context.Context, db *sql.DB) error {
	lockKey := w.advisoryLockKey()

	w.logger.Infof("Attempting to acquire migration advisory lock for %s (key=%d)",
		w.config.Component, lockKey)

	// Create context with lock timeout
	lockCtx, cancel := context.WithTimeout(ctx, w.config.LockTimeout)
	defer cancel()

	var acquired bool
	err := db.QueryRowContext(lockCtx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)

	if err != nil {
		w.logger.Errorf("Failed to query advisory lock for %s: %v", w.config.Component, err)
		return fmt.Errorf("%w: %v", ErrMigrationLockFailed, err)
	}

	if !acquired {
		// Lock held by another process - query who holds it for debugging
		w.logStaleLockHolder(ctx, db, lockKey)
		return ErrMigrationLockFailed
	}

	w.logger.Infof("Successfully acquired migration advisory lock for %s", w.config.Component)

	return nil
}

// logStaleLockHolder queries pg_stat_activity to log information about who holds the lock.
// This helps operators debug lock contention issues.
func (w *MigrationWrapper) logStaleLockHolder(ctx context.Context, db *sql.DB, lockKey int64) {
	var pid int
	var username, appName sql.NullString
	var backendStart sql.NullTime

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
		return fmt.Errorf("failed to release advisory lock: %w", err)
	}

	if !released {
		w.logger.Warnf("Advisory lock for %s was not held (already released or never acquired)",
			w.config.Component)
	} else {
		w.logger.Infof("Successfully released migration advisory lock for %s", w.config.Component)
	}

	return nil
}
```

**Step 2: Run the tests (expect pass)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestAdvisory`

**Expected output:**
```
=== RUN   TestAcquireAdvisoryLock_Success
--- PASS: TestAcquireAdvisoryLock_Success (0.00s)
=== RUN   TestAcquireAdvisoryLock_AlreadyLocked
--- PASS: TestAcquireAdvisoryLock_AlreadyLocked (0.00s)
=== RUN   TestReleaseAdvisoryLock_Success
--- PASS: TestReleaseAdvisoryLock_Success (0.00s)
=== RUN   TestAdvisoryLockKey_DifferentComponents
--- PASS: TestAdvisoryLockKey_DifferentComponents (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 3: Run all tests so far**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v`

**Expected output:**
```
=== RUN   TestPreflightCheck_CleanMigration
--- PASS: TestPreflightCheck_CleanMigration (0.00s)
... (all tests pass)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
feat(mmigration): implement advisory lock for concurrent migration protection

Add PostgreSQL advisory lock support to prevent concurrent migrations:
- AcquireAdvisoryLock: Non-blocking lock acquisition with timeout
- ReleaseAdvisoryLock: Clean lock release
- Component-specific lock keys prevent cross-service interference
- Session-scoped locks auto-release on disconnect
EOF
)"
```

**If Task Fails:**

1. **Tests failing:**
   - Check: `go test ./pkg/mmigration/... -v 2>&1`
   - Fix: Address logic or SQL query issues
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

---

## Task 7: Write failing test for dirty migration recovery

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Add dirty recovery tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go

func TestRecoverDirtyMigration_Success(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect UPDATE to clear dirty flag (only clears dirty, NEVER changes version)
	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Use newTestWrapper to ensure all required fields are initialized
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
	})

	// Execute recovery
	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	// Verify
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverDirtyMigration_AutoRecoverDisabled(t *testing.T) {
	// Setup mock database
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Use newTestWrapper with AutoRecoverDirty explicitly disabled
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: false, // Disabled
	})

	// Execute recovery
	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	// Verify - should fail because auto-recover is disabled
	assert.ErrorIs(t, err, ErrMigrationRecoveryFailed)
}

func TestRecoverDirtyMigration_UpdateFails(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect UPDATE to fail
	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnError(errors.New("database connection lost"))

	// Use newTestWrapper to ensure all required fields are initialized
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
	})

	// Execute recovery
	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	// Verify - should return error
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverDirtyMigration_NoRowsAffected(t *testing.T) {
	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect UPDATE that affects no rows (table structure changed?)
	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Use newTestWrapper to ensure all required fields are initialized
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
	})

	// Execute recovery
	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	// Verify - should warn but not fail (idempotent operation)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

**Step 2: Run the test (expect failure)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestRecoverDirty`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmigration [github.com/LerianStudio/midaz/v3/pkg/mmigration.test]
pkg/mmigration/migration_test.go:XX: wrapper.recoverDirtyMigration undefined
FAIL    github.com/LerianStudio/midaz/v3/pkg/mmigration [build failed]
```

**Step 3: Commit the failing test**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go
git commit -m "$(cat <<'EOF'
test(mmigration): add failing tests for dirty migration recovery

TDD: Write tests for recoverDirtyMigration function:
- Successful recovery (clears dirty flag only, NEVER modifies version)
- Auto-recover disabled (should fail gracefully)
- Database error during recovery
- No rows affected (idempotent behavior)
EOF
)"
```

**If Task Fails:**

1. **Import errors:**
   - Fix: Add `"errors"` to imports if not present
   - Rollback: `git checkout -- pkg/mmigration/migration_test.go`

---

## Task 8: Implement recoverDirtyMigration function

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 7 completed (failing tests exist)

**Step 1: Add recoverDirtyMigration method**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go

// clearDirtyFlagQuery is the SQL to clear the dirty flag for a specific version.
// SECURITY: This query ONLY clears dirty flag, it NEVER modifies the version.
// The version represents the last successfully applied migration and must not be changed.
// Uses parameterized query with version to ensure we only clear the specific dirty migration.
const clearDirtyFlagQuery = `UPDATE schema_migrations SET dirty = false WHERE version = $1 AND dirty = true`

// ErrMaxRecoveryPerVersionExceeded indicates the per-version recovery limit was reached.
var ErrMaxRecoveryPerVersionExceeded = errors.New("maximum recovery attempts exceeded for this migration version")

// recoverDirtyMigration attempts to recover from a dirty migration state.
// This ONLY clears the dirty flag - it NEVER modifies the migration version.
//
// The dirty flag indicates a migration was interrupted mid-execution.
// Clearing it allows golang-migrate to retry the migration from scratch.
//
// PREREQUISITES:
//   - Migrations MUST be idempotent (use IF NOT EXISTS, IF EXISTS, etc.)
//   - This is required because partial migrations may have been applied
//
// SECURITY CONSTRAINTS:
//   - Only clears dirty flag for specific version, never modifies version
//   - Only executes if AutoRecoverDirty is enabled
//   - Validates migration file exists before recovery
//   - Enforces per-version recovery limit to prevent infinite loops
//   - Logs all recovery attempts as structured JSON for audit trail
//
// Parameters:
//   - ctx: Context for cancellation
//   - db: Database connection
//   - version: The version number where dirty state was detected
func (w *MigrationWrapper) recoverDirtyMigration(ctx context.Context, db *sql.DB, version int) error {
	// Check if auto-recovery is enabled
	if !w.config.AutoRecoverDirty {
		w.logger.Errorf("Migration for %s is dirty at version %d but auto-recovery is DISABLED. "+
			"Manual intervention required: check migration %06d and either fix the issue or "+
			"manually set dirty=false in schema_migrations table.",
			w.config.Component, version, version)

		return fmt.Errorf("%w: auto-recovery disabled for %s at version %d",
			ErrMigrationRecoveryFailed, w.config.Component, version)
	}

	// Check per-version recovery limit to prevent infinite boot loops
	w.mu.Lock()
	attempts := w.recoveryAttemptsPerVersion[version]
	if attempts >= w.config.MaxRecoveryPerVersion {
		w.mu.Unlock()
		w.logger.Errorf("CRITICAL: Migration version %d for %s has failed recovery %d times. "+
			"Maximum attempts (%d) exceeded. Manual intervention required. "+
			"Check migration file %06d_*.sql for errors.",
			version, w.config.Component, attempts, w.config.MaxRecoveryPerVersion, version)

		// Emit structured audit log for monitoring/alerting
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

	// Emit structured audit log for recovery attempt
	w.logger.WithFields(
		"event", "migration_recovery_attempt",
		"component", w.config.Component,
		"version", version,
		"attempt", attempts+1,
		"max_attempts", w.config.MaxRecoveryPerVersion,
	).Warn("Attempting automatic recovery of dirty migration")

	w.logger.Warnf("ALERT: Attempting automatic recovery of dirty migration for %s at version %d (attempt %d/%d). "+
		"This will clear the dirty flag and allow migrations to retry.",
		w.config.Component, version, attempts+1, w.config.MaxRecoveryPerVersion)

	// Execute the recovery - ONLY clears dirty flag for specific version
	result, err := db.ExecContext(ctx, clearDirtyFlagQuery, version)
	if err != nil {
		w.logger.Errorf("Failed to clear dirty flag for %s at version %d: %v", w.config.Component, version, err)
		return fmt.Errorf("failed to clear dirty flag: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		w.logger.Warnf("Could not determine rows affected for %s dirty recovery: %v",
			w.config.Component, err)
	} else if rowsAffected == 0 {
		w.logger.Warnf("No rows affected when clearing dirty flag for %s at version %d - "+
			"migration may have already been recovered", w.config.Component, version)
	} else {
		// Emit structured audit log for successful recovery
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
// This prevents recovery from proceeding when the migration file is missing or deleted.
func (w *MigrationWrapper) validateMigrationFileExists(version int) error {
	// Look for migration file matching pattern: {version}_*.up.sql
	pattern := filepath.Join(w.config.MigrationsPath, fmt.Sprintf("%06d_*.up.sql", version))
	matches, err := filepath.Glob(pattern)

	if err != nil {
		return fmt.Errorf("failed to search for migration file: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("%w: no migration file found matching %s - cannot auto-recover",
			ErrMigrationFileNotFound, pattern)
	}

	w.logger.Infof("Found migration file for version %d: %s", version, matches[0])

	return nil
}
```

**Step 2: Run the tests (expect pass)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestRecoverDirty`

**Expected output:**
```
=== RUN   TestRecoverDirtyMigration_Success
--- PASS: TestRecoverDirtyMigration_Success (0.00s)
=== RUN   TestRecoverDirtyMigration_AutoRecoverDisabled
--- PASS: TestRecoverDirtyMigration_AutoRecoverDisabled (0.00s)
=== RUN   TestRecoverDirtyMigration_UpdateFails
--- PASS: TestRecoverDirtyMigration_UpdateFails (0.00s)
=== RUN   TestRecoverDirtyMigration_NoRowsAffected
--- PASS: TestRecoverDirtyMigration_NoRowsAffected (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
feat(mmigration): implement recoverDirtyMigration for safe dirty recovery

Add dirty migration recovery with strict security constraints:
- ONLY clears dirty flag, NEVER modifies migration version
- Requires AutoRecoverDirty config to be enabled
- Logs all recovery attempts for audit trail
- Tracks recovery metrics for observability
EOF
)"
```

**If Task Fails:**

1. **Tests failing:**
   - Check: `go test ./pkg/mmigration/... -v 2>&1`
   - Fix: Verify SQL query matches test expectation
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

---

## Task 9: Write failing test for SafeGetDB

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`

**Prerequisites:**
- Task 8 completed

**Step 1: Add SafeGetDB tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go

// Note: SafeGetDB is harder to test with pure unit tests because it wraps
// lib-commons PostgresConnection. These tests focus on the internal logic
// that can be isolated.

func TestSafeGetDB_MockWorkflow(t *testing.T) {
	// This test validates the workflow logic of SafeGetDB
	// by testing its component parts in sequence

	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Step 1: Advisory lock acquisition
	lockRows := sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(lockRows)

	// Step 2: Preflight check (clean)
	statusRows := sqlmock.NewRows([]string{"version", "dirty"}).AddRow(18, false)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnRows(statusRows)

	// Step 3: Advisory lock release
	unlockRows := sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true)
	mock.ExpectQuery("SELECT pg_advisory_unlock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(unlockRows)

	// Use newTestWrapper for consistent test setup
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		LockTimeout:      5 * time.Second,
	})

	ctx := context.Background()

	// Execute workflow manually (since we can't easily mock PostgresConnection)
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	require.NoError(t, err)

	status, err := wrapper.PreflightCheck(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, 18, status.Version)
	assert.False(t, status.Dirty)

	err = wrapper.ReleaseAdvisoryLock(ctx, db)
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSafeGetDB_DirtyRecoveryWorkflow(t *testing.T) {
	// Test the dirty recovery workflow

	// Setup mock database
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Step 1: Advisory lock acquisition
	lockRows := sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(lockRows)

	// Step 2: Preflight check (dirty!)
	statusRows := sqlmock.NewRows([]string{"version", "dirty"}).AddRow(15, true)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnRows(statusRows)

	// Step 3: Recovery - clear dirty flag
	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Step 4: Advisory lock release
	unlockRows := sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true)
	mock.ExpectQuery("SELECT pg_advisory_unlock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(unlockRows)

	// Use newTestWrapper to ensure all required fields are initialized
	// (especially recoveryAttemptsPerVersion map to prevent nil panic)
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		LockTimeout:      5 * time.Second,
	})

	ctx := context.Background()

	// Execute workflow
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	require.NoError(t, err)

	status, err := wrapper.PreflightCheck(ctx, db)
	assert.ErrorIs(t, err, ErrMigrationDirty)
	assert.Equal(t, 15, status.Version)
	assert.True(t, status.Dirty)

	// Recovery
	err = wrapper.recoverDirtyMigration(ctx, db, status.Version)
	require.NoError(t, err)

	err = wrapper.ReleaseAdvisoryLock(ctx, db)
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}
```

**Step 2: Run the tests (expect pass - these test workflow logic)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestSafeGetDB`

**Expected output:**
```
=== RUN   TestSafeGetDB_MockWorkflow
--- PASS: TestSafeGetDB_MockWorkflow (0.00s)
=== RUN   TestSafeGetDB_DirtyRecoveryWorkflow
--- PASS: TestSafeGetDB_DirtyRecoveryWorkflow (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go
git commit -m "$(cat <<'EOF'
test(mmigration): add workflow tests for SafeGetDB logic

Test the complete workflow that SafeGetDB will orchestrate:
- Clean migration path (lock -> preflight -> release)
- Dirty recovery path (lock -> preflight -> recovery -> release)
EOF
)"
```

**If Task Fails:**

1. **Tests failing unexpectedly:**
   - Check: Ensure all previous tasks completed successfully
   - Fix: Review mock expectations match implementation
   - Rollback: `git checkout -- pkg/mmigration/migration_test.go`

---

## Task 10: Implement SafeGetDB wrapper

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 9 completed

**Step 1: Add SafeGetDB method**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go

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
//
// The advisory lock is released after GetDB() returns, not held for connection lifetime.
// This allows the migration to complete atomically while not blocking other operations.
func (w *MigrationWrapper) SafeGetDB(ctx context.Context) (dbresolver.DB, error) {
	w.logger.Infof("SafeGetDB starting for %s", w.config.Component)

	// Get a raw connection for preflight checks (bypasses migration)
	rawDB, err := w.getRawConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw connection for preflight: %w", err)
	}

	// CRITICAL: We do NOT use defer for lock release and connection close here.
	// PostgreSQL session-scoped advisory locks are released when the connection closes.
	// We must keep the connection open until GetDB() completes to maintain lock protection.

	// Acquire advisory lock
	if err := w.AcquireAdvisoryLock(ctx, rawDB); err != nil {
		rawDB.Close() // Clean up on failure
		return nil, fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	// Preflight check
	status, err := w.PreflightCheck(ctx, rawDB)
	if err != nil {
		if errors.Is(err, ErrMigrationDirty) {
			// Attempt recovery
			if recoveryErr := w.recoverDirtyMigration(ctx, rawDB, status.Version); recoveryErr != nil {
				w.ReleaseAdvisoryLock(ctx, rawDB)
				rawDB.Close()
				return nil, fmt.Errorf("migration recovery failed: %w", recoveryErr)
			}

			w.logger.Infof("Migration recovery successful for %s, proceeding with GetDB()",
				w.config.Component)
		} else {
			w.ReleaseAdvisoryLock(ctx, rawDB)
			rawDB.Close()
			return nil, fmt.Errorf("preflight check failed: %w", err)
		}
	}

	// Call the underlying GetDB which runs migrations
	// IMPORTANT: Lock is still held here, protecting concurrent migration attempts
	w.logger.Infof("Calling underlying GetDB() for %s", w.config.Component)

	db, err := w.conn.GetDB()

	// NOW release the lock and close connection - after GetDB() completes
	if releaseErr := w.ReleaseAdvisoryLock(ctx, rawDB); releaseErr != nil {
		w.logger.Warnf("Failed to release migration lock for %s: %v",
			w.config.Component, releaseErr)
	}
	if closeErr := rawDB.Close(); closeErr != nil {
		w.logger.Warnf("Failed to close raw connection for %s: %v",
			w.config.Component, closeErr)
	}

	if err != nil {
		w.updateStatus(func(s *MigrationStatus) {
			s.LastError = err
		})

		return nil, fmt.Errorf("GetDB failed for %s: %w", w.config.Component, err)
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
	// Use the connection string from PostgresConnection to open a raw connection
	// This is a workaround since lib-commons doesn't expose a non-migrating connection method

	// For now, we'll use the primary connection string
	connStr := w.conn.ConnectionStringPrimary
	if connStr == "" {
		return nil, errors.New("no connection string available")
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open raw connection: %w", err)
	}

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping raw connection: %w", err)
	}

	return db, nil
}

// GetConnection returns the underlying PostgresConnection.
// Use this when you need access to the connection object itself.
func (w *MigrationWrapper) GetConnection() *libPostgres.PostgresConnection {
	return w.conn
}
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmigration/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run all tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v`

**Expected output:**
```
=== RUN   TestPreflightCheck_CleanMigration
--- PASS: TestPreflightCheck_CleanMigration (0.00s)
... (all tests pass)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
feat(mmigration): implement SafeGetDB wrapper with full migration protection

SafeGetDB provides the main entry point for safe database access:
- Acquires advisory lock before migration
- Runs preflight check to detect dirty state
- Automatically recovers from dirty state if enabled
- Calls underlying GetDB() which runs migrations
- Releases lock after migration completes
- Updates internal status for health reporting
EOF
)"
```

**If Task Fails:**

1. **Compilation errors:**
   - Check: `go build ./pkg/mmigration/... 2>&1`
   - Fix: Address missing imports or type mismatches
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

2. **Import error for dbresolver:**
   - Fix: Ensure `github.com/bxcodec/dbresolver/v2` is imported
   - Rollback: N/A

---

## Task 11: Write failing test for retry logic with backoff

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Add retry logic tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go

func TestCalculateBackoff(t *testing.T) {
	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			RetryBackoff: 1 * time.Second,
			MaxRetries:   3,
		},
		logger: &mockLogger{},
	}

	// Test exponential backoff
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},  // 1s * 2^0 = 1s
		{1, 2 * time.Second},  // 1s * 2^1 = 2s
		{2, 4 * time.Second},  // 1s * 2^2 = 4s
		{3, 8 * time.Second},  // 1s * 2^3 = 8s
		{4, 16 * time.Second}, // 1s * 2^4 = 16s
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			backoff := wrapper.calculateBackoff(tt.attempt)
			assert.Equal(t, tt.expected, backoff)
		})
	}
}

func TestShouldRetry(t *testing.T) {
	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			MaxRetries: 3,
		},
		logger: &mockLogger{},
	}

	// Should retry for attempts < MaxRetries
	assert.True(t, wrapper.shouldRetry(0))
	assert.True(t, wrapper.shouldRetry(1))
	assert.True(t, wrapper.shouldRetry(2))

	// Should not retry at or above MaxRetries
	assert.False(t, wrapper.shouldRetry(3))
	assert.False(t, wrapper.shouldRetry(4))
}

func TestIsRetryableError(t *testing.T) {
	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: &mockLogger{},
	}

	// Retryable errors
	assert.True(t, wrapper.isRetryableError(ErrMigrationDirty))
	assert.True(t, wrapper.isRetryableError(ErrMigrationLockFailed))

	// Non-retryable errors
	assert.False(t, wrapper.isRetryableError(context.Canceled))
	assert.False(t, wrapper.isRetryableError(context.DeadlineExceeded))
	assert.False(t, wrapper.isRetryableError(ErrMaxRetriesExceeded))
	assert.False(t, wrapper.isRetryableError(errors.New("random error")))
}
```

**Step 2: Run the test (expect failure)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run "TestCalculateBackoff|TestShouldRetry|TestIsRetryable"`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmigration [github.com/LerianStudio/midaz/v3/pkg/mmigration.test]
pkg/mmigration/migration_test.go:XX: wrapper.calculateBackoff undefined
pkg/mmigration/migration_test.go:XX: wrapper.shouldRetry undefined
pkg/mmigration/migration_test.go:XX: wrapper.isRetryableError undefined
FAIL    github.com/LerianStudio/midaz/v3/pkg/mmigration [build failed]
```

**Step 3: Add fmt import if needed**

Check if `"fmt"` is imported in the test file and add if missing.

**Step 4: Commit the failing test**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go
git commit -m "$(cat <<'EOF'
test(mmigration): add failing tests for retry logic

TDD: Write tests for retry helper functions:
- Exponential backoff calculation
- Max retry limit enforcement
- Retryable vs non-retryable error classification
EOF
)"
```

**If Task Fails:**

1. **Import errors:**
   - Fix: Add `"fmt"` to imports
   - Rollback: `git checkout -- pkg/mmigration/migration_test.go`

---

## Task 12: Implement retry logic with exponential backoff

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 11 completed (failing tests exist)

**Step 1: Add retry helper methods**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`:

```go

// calculateBackoff calculates the backoff duration for a given attempt.
// Uses exponential backoff: baseBackoff * 2^attempt
func (w *MigrationWrapper) calculateBackoff(attempt int) time.Duration {
	multiplier := 1 << attempt // 2^attempt
	backoff := w.config.RetryBackoff * time.Duration(multiplier)

	// Cap at MaxBackoff to prevent excessive wait times
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
				return nil, ctx.Err()
			case <-time.After(backoff):
				// Continue after backoff
			}
		}

		db, err := w.SafeGetDB(ctx)
		if err == nil {
			if attempt > 0 {
				w.logger.Infof("Successfully connected after %d retries for %s",
					attempt, w.config.Component)
			}

			return db, nil
		}

		lastErr = err

		if !w.isRetryableError(err) {
			w.logger.Errorf("Non-retryable error for %s: %v", w.config.Component, err)
			return nil, err
		}

		w.logger.Warnf("Retryable error on attempt %d for %s: %v",
			attempt+1, w.config.Component, err)
	}

	return nil, fmt.Errorf("%w: after %d attempts for %s, last error: %v",
		ErrMaxRetriesExceeded, w.config.MaxRetries, w.config.Component, lastErr)
}
```

**Step 2: Run the tests (expect pass)**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run "TestCalculateBackoff|TestShouldRetry|TestIsRetryable"`

**Expected output:**
```
=== RUN   TestCalculateBackoff
=== RUN   TestCalculateBackoff/attempt_0
=== RUN   TestCalculateBackoff/attempt_1
=== RUN   TestCalculateBackoff/attempt_2
=== RUN   TestCalculateBackoff/attempt_3
=== RUN   TestCalculateBackoff/attempt_4
--- PASS: TestCalculateBackoff (0.00s)
=== RUN   TestShouldRetry
--- PASS: TestShouldRetry (0.00s)
=== RUN   TestIsRetryableError
--- PASS: TestIsRetryableError (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 3: Run all tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v`

**Expected output:**
```
... (all tests pass)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
feat(mmigration): implement retry logic with exponential backoff

Add retry support for transient migration failures:
- Exponential backoff: baseBackoff * 2^attempt
- Configurable MaxRetries (default 3)
- Retryable error classification (dirty, lock failed)
- Non-retryable errors fail immediately (context canceled, etc.)
- SafeGetDBWithRetry for automatic retry handling
EOF
)"
```

**If Task Fails:**

1. **Tests failing:**
   - Check: `go test ./pkg/mmigration/... -v 2>&1`
   - Fix: Verify backoff calculation logic
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

---

## Task 13: Run Code Review Checkpoint 1

**Prerequisites:**
- Tasks 1-12 completed

**Step 1: Dispatch all 3 reviewers in parallel**

> **REQUIRED SUB-SKILL:** Use requesting-code-review

Run code review with all 3 reviewers (code-reviewer, business-logic-reviewer, security-reviewer) simultaneously for `pkg/mmigration/`.

**Step 2: Handle findings by severity**

| Severity | Action |
|----------|--------|
| Critical/High/Medium | Fix immediately, then re-run reviewers |
| Low | Add `TODO(review):` comment at relevant location |
| Cosmetic/Nitpick | Add `FIXME(nitpick):` comment at relevant location |

**Step 3: Verify and commit**

After fixing all Critical/High/Medium issues:

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v && go vet ./pkg/mmigration/...`

**Expected output:**
```
... (all tests pass)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
(no vet output)
```

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/
git commit -m "$(cat <<'EOF'
refactor(mmigration): address code review findings checkpoint 1

Apply fixes from code review:
- [List specific fixes here]
EOF
)"
```

**If Task Fails:**

1. **Critical issues found:**
   - Fix: Address all Critical/High/Medium issues
   - Re-run: Code review until clean

---

## Task 14: Add minimal health status for health reporting

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/health.go`

**Prerequisites:**
- Task 13 completed

**Design Decision:** Health endpoint returns minimal information (healthy/unhealthy boolean only) to avoid exposing internal architecture details. Detailed status is available via logs and metrics.

**Step 1: Create health.go with minimal health reporting**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/health.go`:

```go
package mmigration

// HealthStatus represents minimal migration health in health check responses.
// Intentionally minimal to avoid exposing internal architecture details.
type HealthStatus struct {
	// Healthy indicates if migrations are in a good state.
	Healthy bool `json:"healthy"`
}

// GetHealthStatus returns the current health status for health endpoints.
// Returns a minimal response with only healthy/unhealthy status.
// Detailed status information is available via logs and metrics.
func (w *MigrationWrapper) GetHealthStatus() HealthStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return HealthStatus{
		Healthy: w.status.IsHealthy(),
	}
}

// IsHealthy returns true if migrations are in a healthy state.
// Convenience method for simple health checks.
func (w *MigrationWrapper) IsHealthy() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.status.IsHealthy()
}

// HealthChecker is an interface for migration health checking.
// Components can use this to integrate with their health endpoints.
type HealthChecker interface {
	GetHealthStatus() HealthStatus
	IsHealthy() bool
}

// Ensure MigrationWrapper implements HealthChecker.
var _ HealthChecker = (*MigrationWrapper)(nil)
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmigration/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Add test for health status**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go

func TestGetHealthStatus_Healthy(t *testing.T) {
	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component: "transaction",
		},
		logger: &mockLogger{},
		status: MigrationStatus{
			Version:     18,
			Dirty:       false,
			LastChecked: time.Now(),
			LastError:   nil,
		},
	}

	hs := wrapper.GetHealthStatus()

	assert.True(t, hs.Healthy)
}

func TestGetHealthStatus_Unhealthy(t *testing.T) {
	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component: "onboarding",
		},
		logger: &mockLogger{},
		status: MigrationStatus{
			Version:          15,
			Dirty:            true,
			LastChecked:      time.Now(),
			RecoveryAttempts: 2,
			LastError:        ErrMigrationDirty,
		},
	}

	hs := wrapper.GetHealthStatus()

	assert.False(t, hs.Healthy)
}

func TestIsHealthy(t *testing.T) {
	// Test healthy state
	healthyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: &mockLogger{},
		status: MigrationStatus{Dirty: false, LastError: nil},
	}
	assert.True(t, healthyWrapper.IsHealthy())

	// Test unhealthy (dirty)
	dirtyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: &mockLogger{},
		status: MigrationStatus{Dirty: true, LastError: nil},
	}
	assert.False(t, dirtyWrapper.IsHealthy())

	// Test unhealthy (error)
	errorWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: &mockLogger{},
		status: MigrationStatus{Dirty: false, LastError: errors.New("test error")},
	}
	assert.False(t, errorWrapper.IsHealthy())
}

func TestHealthStatus_JSON(t *testing.T) {
	hs := HealthStatus{
		Healthy: true,
	}

	data, err := json.Marshal(hs)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, true, result["healthy"])
	// Verify only healthy field is present (minimal response)
	assert.Len(t, result, 1)
}
```

**Step 4: Run tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -run TestGetHealth`

**Expected output:**
```
=== RUN   TestGetHealthStatus_Healthy
--- PASS: TestGetHealthStatus_Healthy (0.00s)
=== RUN   TestGetHealthStatus_Unhealthy
--- PASS: TestGetHealthStatus_Unhealthy (0.00s)
=== RUN   TestHealthStatus_JSON
--- PASS: TestHealthStatus_JSON (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 5: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/
git commit -m "$(cat <<'EOF'
feat(mmigration): add minimal HealthStatus for health endpoint integration

Add minimal health reporting (healthy/unhealthy only):
- HealthStatus struct with single boolean field
- GetHealthStatus and IsHealthy methods
- HealthChecker interface for component integration
- Detailed status available via logs and metrics
EOF
)"
```

**If Task Fails:**

1. **Compilation errors:**
   - Check: `go build ./pkg/mmigration/... 2>&1`
   - Fix: Address import or type issues
   - Rollback: `git checkout -- pkg/mmigration/health.go`

---

## Task 15: Write MigrationHealthHandler for health endpoint

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/handler.go`

**Prerequisites:**
- Task 14 completed

**Step 1: Create handler.go with HTTP handler**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/handler.go`:

```go
package mmigration

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// FiberHealthHandler returns a Fiber handler for migration health checks.
// Use this to add migration status to your service's health endpoint.
//
// Example usage in routes.go:
//
//	f.Get("/health/migrations", mmigration.FiberHealthHandler(migrationWrapper))
func FiberHealthHandler(checker HealthChecker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		status := checker.GetHealthStatus()

		statusCode := http.StatusOK
		if !status.Healthy {
			statusCode = http.StatusServiceUnavailable
		}

		return c.Status(statusCode).JSON(status)
	}
}

// FiberReadinessCheck returns true if migrations are healthy.
// Use this in readiness probe handlers.
//
// Example:
//
//	f.Get("/ready", func(c *fiber.Ctx) error {
//	    if !mmigration.FiberReadinessCheck(migrationWrapper) {
//	        return c.SendStatus(http.StatusServiceUnavailable)
//	    }
//	    return c.SendStatus(http.StatusOK)
//	})
func FiberReadinessCheck(checker HealthChecker) bool {
	return checker.GetHealthStatus().Healthy
}

// MigrationHealthResponse is the response structure for migration health.
// This matches the HealthStatus JSON output.
//
// swagger:response MigrationHealthResponse
// @Description Migration health status response
type MigrationHealthResponse struct {
	// in: body
	Body HealthStatus
}
```

**Step 2: Add test for handler**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_test.go`:

```go

func TestFiberReadinessCheck(t *testing.T) {
	// Test healthy state
	healthyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: &mockLogger{},
		status: MigrationStatus{
			Version: 18,
			Dirty:   false,
		},
	}

	assert.True(t, FiberReadinessCheck(healthyWrapper))

	// Test unhealthy state
	unhealthyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: &mockLogger{},
		status: MigrationStatus{
			Version:   15,
			Dirty:     true,
			LastError: ErrMigrationDirty,
		},
	}

	assert.False(t, FiberReadinessCheck(unhealthyWrapper))
}
```

**Step 3: Verify compilation and run tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmigration/... && go test ./pkg/mmigration/... -v -run TestFiberReadiness`

**Expected output:**
```
=== RUN   TestFiberReadinessCheck
--- PASS: TestFiberReadinessCheck (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/
git commit -m "$(cat <<'EOF'
feat(mmigration): add Fiber HTTP handlers for health integration

Add HTTP handlers for Fiber framework:
- FiberHealthHandler: Returns JSON health status with appropriate HTTP status
- FiberReadinessCheck: Boolean helper for readiness probes
- Swagger documentation for response type
EOF
)"
```

**If Task Fails:**

1. **Import error for Fiber:**
   - Check: Fiber should be in go.mod already
   - Fix: `go get github.com/gofiber/fiber/v2`
   - Rollback: `git checkout -- pkg/mmigration/handler.go`

---

## Task 16: Add metrics registration (prometheus)

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/metrics.go`

**Prerequisites:**
- Task 15 completed

**Step 1: Create metrics.go with Prometheus metrics**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/metrics.go`:

```go
package mmigration

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for migration operations.
var (
	// MigrationDurationSeconds tracks time spent in migration operations.
	MigrationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "duration_seconds",
			Help:      "Time spent in migration operations",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"component", "operation", "status"},
	)

	// MigrationRecoveryTotal counts migration recovery attempts.
	MigrationRecoveryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "recovery_total",
			Help:      "Total count of migration recovery attempts",
		},
		[]string{"component", "status"},
	)

	// MigrationLockWaitSeconds tracks time spent waiting for advisory locks.
	MigrationLockWaitSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "lock_wait_seconds",
			Help:      "Time spent waiting for migration advisory lock",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"component", "acquired"},
	)

	// MigrationStatusGauge indicates current migration status.
	MigrationStatusGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "status",
			Help:      "Current migration status (1=healthy, 0=unhealthy)",
		},
		[]string{"component"},
	)

	// MigrationVersionGauge tracks current migration version.
	MigrationVersionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "version",
			Help:      "Current migration version number",
		},
		[]string{"component"},
	)
)

// RecordMigrationDuration records the duration of a migration operation.
func (w *MigrationWrapper) RecordMigrationDuration(operation, status string, durationSeconds float64) {
	MigrationDurationSeconds.WithLabelValues(w.config.Component, operation, status).Observe(durationSeconds)
}

// RecordRecoveryAttempt records a migration recovery attempt.
func (w *MigrationWrapper) RecordRecoveryAttempt(successful bool) {
	status := "success"
	if !successful {
		status = "failure"
	}

	MigrationRecoveryTotal.WithLabelValues(w.config.Component, status).Inc()
}

// RecordLockWait records time spent waiting for advisory lock.
func (w *MigrationWrapper) RecordLockWait(acquired bool, durationSeconds float64) {
	acquiredStr := "true"
	if !acquired {
		acquiredStr = "false"
	}

	MigrationLockWaitSeconds.WithLabelValues(w.config.Component, acquiredStr).Observe(durationSeconds)
}

// UpdateStatusMetrics updates the status gauge metrics.
func (w *MigrationWrapper) UpdateStatusMetrics() {
	status := w.GetStatus()

	healthy := float64(0)
	if status.IsHealthy() {
		healthy = 1
	}

	MigrationStatusGauge.WithLabelValues(w.config.Component).Set(healthy)
	MigrationVersionGauge.WithLabelValues(w.config.Component).Set(float64(status.Version))
}
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmigration/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/metrics.go
git commit -m "$(cat <<'EOF'
feat(mmigration): add Prometheus metrics for observability

Add metrics for migration monitoring:
- migration_duration_seconds: Operation timing histogram
- migration_recovery_total: Recovery attempt counter
- migration_lock_wait_seconds: Lock acquisition timing
- migration_status: Current health gauge (1=healthy, 0=unhealthy)
- migration_version: Current version gauge
EOF
)"
```

**If Task Fails:**

1. **Import error for prometheus:**
   - Fix: `go get github.com/prometheus/client_golang/prometheus`
   - Rollback: `git checkout -- pkg/mmigration/metrics.go`

---

## Task 17: Update transaction bootstrap Config struct

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go`

**Prerequisites:**
- Task 16 completed

**Step 1: Add migration config fields to Config struct**

Add the following fields to the Config struct in `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go` (around line 156, after the existing config fields):

```go
	// Migration configuration
	MigrationAutoRecover bool `env:"MIGRATION_AUTO_RECOVER" default:"true"`
	MigrationMaxRetries  int  `env:"MIGRATION_MAX_RETRIES" default:"3"`
```

**Step 2: Add import for mmigration package**

Add to imports in the file:

```go
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
```

**Step 3: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go
git commit -m "$(cat <<'EOF'
feat(transaction): add migration config fields to bootstrap Config

Add environment variables for migration auto-recovery:
- MIGRATION_AUTO_RECOVER: Enable/disable dirty recovery (default: true)
- MIGRATION_MAX_RETRIES: Maximum retry attempts (default: 3)
EOF
)"
```

**If Task Fails:**

1. **Compilation errors:**
   - Check: Ensure import path is correct
   - Fix: Verify module path matches go.mod
   - Rollback: `git checkout -- components/transaction/internal/bootstrap/config.go`

---

## Task 18: Update transaction bootstrap InitServers

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go`

**Prerequisites:**
- Task 17 completed

**Step 1: Update InitServers to use MigrationWrapper**

Find the section where `postgresConnection` is created (around lines 191-200) and modify the code to use the migration wrapper. After the `postgresConnection` creation, add:

```go
	// Create migration wrapper for safe database access with auto-recovery
	migrationConfig := mmigration.MigrationConfig{
		AutoRecoverDirty:      cfg.MigrationAutoRecover,
		MaxRetries:            cfg.MigrationMaxRetries,
		MaxRecoveryPerVersion: 3, // Per-version limit prevents infinite boot loops
		RetryBackoff:          1 * time.Second,
		MaxBackoff:            30 * time.Second,
		LockTimeout:           30 * time.Second,
		Component:             ApplicationName,
		MigrationsPath:        "/app/components/transaction/migrations", // Absolute path for container
	}

	migrationWrapper, err := mmigration.NewMigrationWrapper(postgresConnection, migrationConfig, logger)
	if err != nil {
		logger.Fatalf("Failed to create migration wrapper for %s: %v", ApplicationName, err)
		return nil, fmt.Errorf("migration wrapper initialization failed: %w", err)
	}

	// Use SafeGetDBWithRetry for the first database access
	// This performs preflight check and handles dirty recovery
	ctx := context.Background()
	_, err = migrationWrapper.SafeGetDBWithRetry(ctx)
	if err != nil {
		logger.Errorf("Migration preflight failed for %s: %v", ApplicationName, err)
		// Fall through to let the existing GetDB() handle it
		// This maintains backward compatibility
	} else {
		// Update metrics after successful migration
		migrationWrapper.UpdateStatusMetrics()
		logger.Infof("Migration preflight successful for %s", ApplicationName)
	}
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run existing tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/bootstrap/... -v -short`

**Expected output:**
```
... (tests pass or skip)
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap X.XXXs
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go
git commit -m "$(cat <<'EOF'
feat(transaction): integrate MigrationWrapper in InitServers

Add migration auto-recovery to transaction service startup:
- Create MigrationWrapper with component-specific config
- Run SafeGetDBWithRetry for preflight check and recovery
- Update metrics after successful migration
- Maintain backward compatibility on failure
EOF
)"
```

**If Task Fails:**

1. **Compilation errors:**
   - Check: `go build ./components/transaction/... 2>&1`
   - Fix: Address import or variable scope issues
   - Rollback: `git checkout -- components/transaction/internal/bootstrap/config.go`

2. **Tests failing:**
   - Check: Run tests with verbose output
   - Fix: May need to mock the migration wrapper in tests
   - Rollback: `git checkout -- components/transaction/internal/bootstrap/config.go`

---

## Task 19: Update onboarding bootstrap Config struct

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go`

**Prerequisites:**
- Task 18 completed

**Step 1: Add migration config fields to onboarding Config struct**

Add the following fields to the Config struct in `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go` (around line 117, after existing config fields):

```go
	// Migration configuration
	MigrationAutoRecover bool `env:"MIGRATION_AUTO_RECOVER" default:"true"`
	MigrationMaxRetries  int  `env:"MIGRATION_MAX_RETRIES" default:"3"`
```

**Step 2: Add import for mmigration package**

Add to imports:

```go
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
```

**Step 3: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/onboarding/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add migration config fields to bootstrap Config

Add environment variables for migration auto-recovery:
- MIGRATION_AUTO_RECOVER: Enable/disable dirty recovery (default: true)
- MIGRATION_MAX_RETRIES: Maximum retry attempts (default: 3)
EOF
)"
```

**If Task Fails:**

1. **Compilation errors:**
   - Check: Same fixes as Task 17
   - Rollback: `git checkout -- components/onboarding/internal/bootstrap/config.go`

---

## Task 20: Update onboarding bootstrap InitServers

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go`

**Prerequisites:**
- Task 19 completed

**Step 1: Update InitServers to use MigrationWrapper**

Find the section where `postgresConnection` is created (around lines 148-157) and add after it:

```go
	// Create migration wrapper for safe database access with auto-recovery
	migrationConfig := mmigration.MigrationConfig{
		AutoRecoverDirty:      cfg.MigrationAutoRecover,
		MaxRetries:            cfg.MigrationMaxRetries,
		MaxRecoveryPerVersion: 3, // Per-version limit prevents infinite boot loops
		RetryBackoff:          1 * time.Second,
		MaxBackoff:            30 * time.Second,
		LockTimeout:           30 * time.Second,
		Component:             ApplicationName,
		MigrationsPath:        "/app/components/onboarding/migrations", // Absolute path for container
	}

	migrationWrapper, err := mmigration.NewMigrationWrapper(postgresConnection, migrationConfig, logger)
	if err != nil {
		logger.Fatalf("Failed to create migration wrapper for %s: %v", ApplicationName, err)
		return nil, fmt.Errorf("migration wrapper initialization failed: %w", err)
	}

	// Use SafeGetDBWithRetry for the first database access
	ctx := context.Background()
	_, err = migrationWrapper.SafeGetDBWithRetry(ctx)
	if err != nil {
		logger.Errorf("Migration preflight failed for %s: %v", ApplicationName, err)
	} else {
		migrationWrapper.UpdateStatusMetrics()
		logger.Infof("Migration preflight successful for %s", ApplicationName)
	}
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/onboarding/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Run existing tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/onboarding/internal/bootstrap/... -v -short`

**Expected output:**
```
... (tests pass or skip)
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go
git commit -m "$(cat <<'EOF'
feat(onboarding): integrate MigrationWrapper in InitServers

Add migration auto-recovery to onboarding service startup:
- Create MigrationWrapper with component-specific config
- Run SafeGetDBWithRetry for preflight check and recovery
- Update metrics after successful migration
EOF
)"
```

**If Task Fails:**

1. **Same fixes as Task 18**

---

## Task 21: Run Code Review Checkpoint 2

**Prerequisites:**
- Tasks 14-20 completed

**Step 1: Dispatch all 3 reviewers**

> **REQUIRED SUB-SKILL:** Use requesting-code-review

Review changes in:
- `pkg/mmigration/`
- `components/transaction/internal/bootstrap/config.go`
- `components/onboarding/internal/bootstrap/config.go`

**Step 2: Handle findings by severity**

Fix Critical/High/Medium immediately. Add TODO/FIXME comments for Low/Cosmetic.

**Step 3: Verify and commit fixes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && make lint && go test ./pkg/mmigration/... ./components/transaction/internal/bootstrap/... ./components/onboarding/internal/bootstrap/... -v -short`

```bash
git add .
git commit -m "$(cat <<'EOF'
refactor: address code review findings checkpoint 2

Apply fixes from code review for:
- mmigration package
- transaction bootstrap integration
- onboarding bootstrap integration
EOF
)"
```

---

## Task 22: Create integration test for dirty recovery scenario

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_integration_test.go`

**Prerequisites:**
- Task 21 completed

**Step 1: Create integration test file**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_integration_test.go`:

```go
//go:build integration
// +build integration

package mmigration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_DirtyRecovery tests the full dirty recovery workflow against a real database.
// Run with: go test -tags=integration -v ./pkg/mmigration/...
//
// Requires environment variable:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost:5432/testdb?sslmode=disable
func TestIntegration_DirtyRecovery(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	// Connect to test database
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Ensure we have a clean state
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS schema_migrations")

	// Create schema_migrations table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint NOT NULL PRIMARY KEY,
			dirty boolean NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert a dirty migration state
	_, err = db.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, dirty) VALUES (15, true)
		ON CONFLICT (version) DO UPDATE SET dirty = true
	`)
	require.NoError(t, err)

	// Create wrapper using newTestWrapper for proper initialization
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:        "integration-test",
		AutoRecoverDirty: true,
		MaxRetries:       3,
		RetryBackoff:     100 * time.Millisecond,
		LockTimeout:      5 * time.Second,
	})

	// Test 1: Preflight should detect dirty state
	status, err := wrapper.PreflightCheck(ctx, db)
	assert.ErrorIs(t, err, ErrMigrationDirty)
	assert.Equal(t, 15, status.Version)
	assert.True(t, status.Dirty)

	// Test 2: Recovery should clear dirty flag
	err = wrapper.recoverDirtyMigration(ctx, db, status.Version)
	assert.NoError(t, err)

	// Test 3: Preflight should now succeed
	status, err = wrapper.PreflightCheck(ctx, db)
	assert.NoError(t, err)
	assert.Equal(t, 15, status.Version)
	assert.False(t, status.Dirty)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DROP TABLE schema_migrations")
}

// TestIntegration_AdvisoryLock tests advisory lock behavior against a real database.
func TestIntegration_AdvisoryLock(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	// Connect to test database
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Use newTestWrapper for consistent initialization
	wrapper := newTestWrapper(t, MigrationConfig{
		Component:   "integration-test-lock",
		LockTimeout: 5 * time.Second,
	})

	// Test 1: Acquire lock
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	assert.NoError(t, err)

	// Test 2: Same session can re-acquire (PostgreSQL behavior)
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	assert.NoError(t, err)

	// Test 3: Release lock
	err = wrapper.ReleaseAdvisoryLock(ctx, db)
	assert.NoError(t, err)

	// Note: We acquired twice so we need to release twice
	err = wrapper.ReleaseAdvisoryLock(ctx, db)
	assert.NoError(t, err)
}
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build -tags=integration ./pkg/mmigration/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration_integration_test.go
git commit -m "$(cat <<'EOF'
test(mmigration): add integration tests for dirty recovery

Add integration tests that run against a real PostgreSQL database:
- TestIntegration_DirtyRecovery: Full dirty -> recovery -> clean workflow
- TestIntegration_AdvisoryLock: Lock acquisition and release behavior

Run with: go test -tags=integration -v ./pkg/mmigration/...
Requires: TEST_DATABASE_URL environment variable
EOF
)"
```

**If Task Fails:**

1. **Build tag issues:**
   - Check: Ensure `//go:build integration` is first line
   - Fix: File must start with build tag
   - Rollback: `git checkout -- pkg/mmigration/migration_integration_test.go`

---

## Task 23: Add documentation comments and examples

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go`

**Prerequisites:**
- Task 22 completed

**Step 1: Add package-level documentation with examples**

Update the package documentation at the top of `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go` to include comprehensive examples:

```go
// Package mmigration provides migration management utilities with auto-recovery support.
// It wraps lib-commons PostgresConnection to add preflight checks, dirty state recovery,
// advisory locks for concurrent protection, and comprehensive observability.
//
// # CRITICAL REQUIREMENT: IDEMPOTENT MIGRATIONS
//
// ⚠️  ALL MIGRATIONS MUST BE IDEMPOTENT ⚠️
//
// Auto-recovery works by clearing the dirty flag and retrying migrations from scratch.
// If a migration partially completed before failure, it will be re-executed. Therefore:
//
//   - CREATE TABLE must use: CREATE TABLE IF NOT EXISTS
//   - DROP TABLE must use: DROP TABLE IF EXISTS
//   - ALTER TABLE ADD COLUMN must use: DO $$ ... IF NOT EXISTS ... $$
//   - CREATE INDEX must use: CREATE INDEX IF NOT EXISTS
//   - INSERT must be idempotent (use ON CONFLICT DO NOTHING/UPDATE)
//
// Example idempotent migration:
//
//   -- 000015_add_users_table.up.sql
//   CREATE TABLE IF NOT EXISTS users (
//       id UUID PRIMARY KEY,
//       email VARCHAR(255) NOT NULL UNIQUE
//   );
//   CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
//
// Failure to use idempotent patterns may result in errors like:
//   "relation already exists" or "column already exists"
//
// # Problem Statement
//
// When a migration fails mid-execution (e.g., due to crash or timeout), golang-migrate
// sets schema_migrations.dirty=true. On next startup, GetDB() returns ErrDirty and
// the service crashes in a boot loop.
//
// # Solution
//
// This package provides SafeGetDB which:
//  1. Acquires an advisory lock (prevents concurrent migrations)
//  2. Checks schema_migrations for dirty state
//  3. Validates migration file exists
//  4. If dirty and AutoRecoverDirty=true, clears the dirty flag (max 3 attempts per version)
//  5. Calls underlying GetDB() to run migrations
//  6. Releases the advisory lock and closes the raw connection
//
// # Security Constraints
//
// The recovery process ONLY clears the dirty flag. It NEVER modifies the migration
// version. This ensures:
//   - The migration that failed will be retried from scratch
//   - No migrations are skipped
//   - No data is lost
//
// # Recovery Limits
//
// To prevent infinite boot loops when a migration has a permanent bug:
//   - Maximum 3 recovery attempts per migration version (configurable)
//   - After limit exceeded, service refuses to start
//   - Requires manual intervention to fix the migration
//
// # Usage
//
// Basic usage in bootstrap:
//
//	// Create the wrapper (MigrationsPath is REQUIRED)
//	migrationCfg := mmigration.MigrationConfig{
//	    AutoRecoverDirty:      true,
//	    MaxRetries:            3,
//	    MaxRecoveryPerVersion: 3,
//	    RetryBackoff:          time.Second,
//	    MaxBackoff:            30 * time.Second,
//	    LockTimeout:           30 * time.Second,
//	    Component:             "transaction",
//	    MigrationsPath:        "/app/components/transaction/migrations",  // REQUIRED
//	}
//	wrapper, err := mmigration.NewMigrationWrapper(postgresConnection, migrationCfg, logger)
//	if err != nil {
//	    log.Fatal("failed to create migration wrapper:", err)
//	}
//
//	// Get database with migration protection
//	ctx := context.Background()
//	db, err := wrapper.SafeGetDBWithRetry(ctx)
//	if err != nil {
//	    // Handle failure - service cannot start
//	    log.Fatal(err)
//	}
//
// # Health Integration
//
// Add migration health to your service health endpoint:
//
//	// In routes.go
//	f.Get("/health/migrations", mmigration.FiberHealthHandler(migrationWrapper))
//
// # Metrics
//
// The package exports Prometheus metrics:
//   - midaz_migration_duration_seconds: Time spent in migration operations
//   - midaz_migration_recovery_total: Count of recovery attempts
//   - midaz_migration_lock_wait_seconds: Time waiting for advisory lock
//   - midaz_migration_status: Current health (1=healthy, 0=unhealthy)
//   - midaz_migration_version: Current migration version
//
// # Environment Variables
//
//   - MIGRATION_AUTO_RECOVER: Enable/disable auto-recovery (default: true)
//   - MIGRATION_MAX_RETRIES: Maximum retry attempts (default: 3)
package mmigration
```

**Step 2: Run go doc to verify documentation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go doc ./pkg/mmigration/`

**Expected output:**
```
package mmigration // import "github.com/LerianStudio/midaz/v3/pkg/mmigration"

Package mmigration provides migration management utilities with auto-recovery
support...
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmigration/migration.go
git commit -m "$(cat <<'EOF'
docs(mmigration): add comprehensive package documentation

Add detailed package documentation including:
- Problem statement explaining the dirty migration issue
- Solution overview with workflow description
- Security constraints (only clears dirty, never modifies version)
- Usage examples for bootstrap integration
- Health endpoint integration
- Prometheus metrics reference
- Environment variables
EOF
)"
```

**If Task Fails:**

1. **go doc errors:**
   - Check: Verify package comment format
   - Fix: Ensure comment block is immediately before `package` declaration
   - Rollback: `git checkout -- pkg/mmigration/migration.go`

---

## Task 24: Run full test suite and lint

**Prerequisites:**
- Task 23 completed

**Step 1: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && make lint`

**Expected output:**
```
... (no errors, warnings acceptable)
```

**If linter fails:**
- Fix lint issues reported
- Common issues: unused variables, missing error checks, formatting

**Step 2: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mmigration/... -v -race -cover`

**Expected output:**
```
=== RUN   TestPreflightCheck_CleanMigration
--- PASS: TestPreflightCheck_CleanMigration (0.00s)
... (all tests pass)
PASS
coverage: XX.X% of statements
ok      github.com/LerianStudio/midaz/v3/pkg/mmigration X.XXXs
```

**Step 3: Run affected component tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/bootstrap/... ./components/onboarding/internal/bootstrap/... -v -short`

**Expected output:**
```
... (tests pass or skip)
```

**Step 4: Commit any lint fixes**

```bash
git add .
git commit -m "$(cat <<'EOF'
fix: address lint issues in mmigration package
EOF
)"
```

**If Task Fails:**

1. **Test failures:**
   - Check: Review specific test output
   - Fix: Address failing assertions
   - Document: If legitimate issue, add to known issues

2. **Race condition detected:**
   - Check: Review concurrent access to shared state
   - Fix: Add mutex protection where needed

---

## Task 25: Final Code Review Checkpoint

**Prerequisites:**
- Task 24 completed

**Step 1: Dispatch all 3 reviewers**

> **REQUIRED SUB-SKILL:** Use requesting-code-review

Review all changes across:
- `pkg/mmigration/` (all files)
- `components/transaction/internal/bootstrap/config.go`
- `components/onboarding/internal/bootstrap/config.go`

**Step 2: Handle findings**

- **Critical/High/Medium:** Fix immediately
- **Low:** Add `TODO(review):` comment
- **Cosmetic:** Add `FIXME(nitpick):` comment

**Step 3: Verify zero Critical/High/Medium issues**

All must be resolved before marking plan complete.

**Step 4: Final commit**

```bash
git add .
git commit -m "$(cat <<'EOF'
feat(mmigration): complete Migration Auto-Recovery System

This commit completes the Migration Auto-Recovery System implementation:

## Summary
- New pkg/mmigration package with safe database access
- Automatic dirty migration recovery with advisory locks
- Prometheus metrics for migration observability
- Health endpoint integration for Kubernetes probes
- Integration with transaction and onboarding bootstrap

## Key Features
- PreflightCheck: Detects dirty migration state before GetDB()
- SafeGetDB: Wraps GetDB() with recovery and retry logic
- Advisory locks: Prevents concurrent migration runs
- Metrics: duration, recovery count, lock wait time, status

## Environment Variables
- MIGRATION_AUTO_RECOVER=true|false (default: true)
- MIGRATION_MAX_RETRIES=N (default: 3)

## Security
- Only clears dirty flag, NEVER modifies migration version
- Advisory locks scoped to component
- Rate-limited retries with exponential backoff
EOF
)"
```

---

## Plan Verification Checklist

Before marking this plan complete, verify:

- [ ] Historical precedent queried (empty - new project)
- [ ] Header with goal, architecture, tech stack, prerequisites
- [ ] Tasks broken into bite-sized steps (2-5 min each)
- [ ] Exact file paths for all files
- [ ] Complete code (no placeholders)
- [ ] Exact commands with expected output
- [ ] Failure recovery steps for each task
- [ ] Code review checkpoints after batches (Tasks 13, 21, 25)
- [ ] Passes Zero-Context Test

## Testing the Feature

To manually test the dirty recovery:

1. Start services normally (migrations run)
2. Stop services
3. Manually set dirty flag: `UPDATE schema_migrations SET dirty = true WHERE version = (SELECT MAX(version) FROM schema_migrations);`
4. Restart services
5. Verify logs show: "Attempting automatic recovery of dirty migration"
6. Verify service starts successfully
7. Verify schema_migrations.dirty = false

## Rollback Plan

If this feature causes issues in production:

1. Set `MIGRATION_AUTO_RECOVER=false` to disable auto-recovery
2. Manually investigate dirty migration state
3. If needed, revert code changes and redeploy
4. Manually fix schema_migrations table if required
