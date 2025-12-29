package mmigration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestMockLogger creates a gomock-generated MockLogger with default AnyTimes expectations.
// This allows the logger to be called any number of times without explicit expectations.
func newTestMockLogger(t *testing.T) (*MockLogger, *gomock.Controller) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockLog := NewMockLogger(ctrl)

	// Set up default expectations for all logger methods - allow any calls
	mockLog.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	mockLog.EXPECT().Infoln(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Warnf(gomock.Any(), gomock.Any()).AnyTimes()
	mockLog.EXPECT().Warnln(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	mockLog.EXPECT().Errorln(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	mockLog.EXPECT().Debugln(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Fatal(gomock.Any()).AnyTimes()
	mockLog.EXPECT().Fatalf(gomock.Any(), gomock.Any()).AnyTimes()
	mockLog.EXPECT().Fatalln(gomock.Any()).AnyTimes()
	mockLog.EXPECT().WithFields(gomock.Any()).Return(mockLog).AnyTimes()
	mockLog.EXPECT().WithDefaultMessageTemplate(gomock.Any()).Return(mockLog).AnyTimes()
	mockLog.EXPECT().Sync().Return(nil).AnyTimes()

	return mockLog, ctrl
}

// createMigrationFile creates a migration file for testing purposes.
func createMigrationFile(t *testing.T, dir string, version int) {
	t.Helper()
	filename := fmt.Sprintf("%06d_test_migration.up.sql", version)
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte("-- test migration"), 0o644)
	require.NoError(t, err, "Failed to create migration file: %s", path)
}

// newTestWrapper creates a MigrationWrapper with all required fields initialized for testing.
// It creates its own gomock controller and mock logger, returning the controller for cleanup.
func newTestWrapper(t *testing.T, config MigrationConfig) (*MigrationWrapper, *gomock.Controller) {
	t.Helper()

	mockLog, ctrl := newTestMockLogger(t)

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
		tmpDir := t.TempDir()
		config.MigrationsPath = tmpDir
	}

	wrapper := &MigrationWrapper{
		config:                     config,
		logger:                     mockLog,
		recoveryAttemptsPerVersion: make(map[int]int),
		status: MigrationStatus{
			LastChecked: time.Now(),
		},
	}

	return wrapper, ctrl
}

func TestPreflightCheck_CleanMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"version", "dirty"}).
		AddRow(18, false)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnRows(rows)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
	}

	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	assert.NoError(t, err)
	assert.Equal(t, 18, status.Version)
	assert.False(t, status.Dirty)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCheck_DirtyMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"version", "dirty"}).
		AddRow(15, true)
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnRows(rows)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
	}

	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	assert.ErrorIs(t, err, ErrMigrationDirty)
	assert.Equal(t, 15, status.Version)
	assert.True(t, status.Dirty)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCheck_NoMigrationsTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnError(sql.ErrNoRows)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
	}

	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	assert.NoError(t, err)
	assert.Equal(t, 0, status.Version)
	assert.False(t, status.Dirty)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCheck_ContextCanceled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnError(context.Canceled)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
	}

	_, err = wrapper.PreflightCheck(ctx, db)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestAcquireAdvisoryLock_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"pg_try_advisory_lock"}).
		AddRow(true)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component:   "transaction",
			LockTimeout: 5 * time.Second,
		},
		logger: mockLog,
	}

	ctx := context.Background()
	err = wrapper.AcquireAdvisoryLock(ctx, db)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcquireAdvisoryLock_Timeout(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Lock is held by another process, returns false on all attempts
	rows := sqlmock.NewRows([]string{"pg_try_advisory_lock"}).
		AddRow(false)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	// Expect stale lock holder query (will fail with no rows, but that's OK)
	mock.ExpectQuery("SELECT pid, usename, application_name, backend_start").
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component:   "transaction",
			LockTimeout: 100 * time.Millisecond, // Short timeout, less than retry interval
		},
		logger: mockLog,
	}

	ctx := context.Background()
	start := time.Now()
	err = wrapper.AcquireAdvisoryLock(ctx, db)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, ErrMigrationLockFailed)
	assert.Contains(t, err.Error(), "timeout")
	// Should respect the lock timeout
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 500*time.Millisecond) // Should timeout before first retry interval
}

func TestReleaseAdvisoryLock_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"pg_advisory_unlock"}).
		AddRow(true)
	mock.ExpectQuery("SELECT pg_advisory_unlock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component: "transaction",
		},
		logger: mockLog,
	}

	ctx := context.Background()
	err = wrapper.ReleaseAdvisoryLock(ctx, db)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAdvisoryLockKey_DifferentComponents(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapperTx := &MigrationWrapper{
		config: MigrationConfig{Component: "transaction"},
		logger: mockLog,
	}
	wrapperOnb := &MigrationWrapper{
		config: MigrationConfig{Component: "onboarding"},
		logger: mockLog,
	}

	keyTx := wrapperTx.advisoryLockKey()
	keyOnb := wrapperOnb.advisoryLockKey()

	assert.NotEqual(t, keyTx, keyOnb)
	assert.Equal(t, keyTx, wrapperTx.advisoryLockKey())
}

func TestRecoverDirtyMigration_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect UPDATE to clear dirty flag (only clears dirty, NEVER changes version)
	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnResult(sqlmock.NewResult(0, 1))

	tmpDir := t.TempDir()
	createMigrationFile(t, tmpDir, 15)

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		MigrationsPath:   tmpDir,
	})
	defer ctrl.Finish()

	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverDirtyMigration_AutoRecoverDisabled(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: false,
	})
	defer ctrl.Finish()

	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	assert.ErrorIs(t, err, ErrMigrationRecoveryFailed)
}

func TestRecoverDirtyMigration_UpdateFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnError(errors.New("database connection lost"))

	tmpDir := t.TempDir()
	createMigrationFile(t, tmpDir, 15)

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		MigrationsPath:   tmpDir,
	})
	defer ctrl.Finish()

	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverDirtyMigration_NoRowsAffected(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE schema_migrations SET dirty = false").
		WillReturnResult(sqlmock.NewResult(0, 0))

	tmpDir := t.TempDir()
	createMigrationFile(t, tmpDir, 15)

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		MigrationsPath:   tmpDir,
	})
	defer ctrl.Finish()

	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	// Should warn but not fail (idempotent operation)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverDirtyMigration_MaxRecoveryExceeded(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:             "transaction",
		AutoRecoverDirty:      true,
		MaxRecoveryPerVersion: 2,
	})
	defer ctrl.Finish()

	// Simulate previous attempts
	wrapper.recoveryAttemptsPerVersion[15] = 2

	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 15)

	assert.ErrorIs(t, err, ErrMaxRecoveryPerVersionExceeded)
}

func TestSafeGetDB_MockWorkflow(t *testing.T) {
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

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		LockTimeout:      5 * time.Second,
	})
	defer ctrl.Finish()

	ctx := context.Background()

	// Execute workflow manually
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

	tmpDir := t.TempDir()
	createMigrationFile(t, tmpDir, 15)

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		LockTimeout:      5 * time.Second,
		MigrationsPath:   tmpDir,
	})
	defer ctrl.Finish()

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

func TestCalculateBackoff(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			RetryBackoff: 1 * time.Second,
			MaxBackoff:   30 * time.Second,
			MaxRetries:   3,
		},
		logger: mockLog,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			backoff := wrapper.calculateBackoff(tt.attempt)
			assert.Equal(t, tt.expected, backoff)
		})
	}
}

func TestCalculateBackoff_MaxCap(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			RetryBackoff: 1 * time.Second,
			MaxBackoff:   5 * time.Second,
			MaxRetries:   10,
		},
		logger: mockLog,
	}

	// Attempt 10 would be 1024s without cap, but should cap at 5s
	backoff := wrapper.calculateBackoff(10)
	assert.Equal(t, 5*time.Second, backoff)
}

func TestShouldRetry(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			MaxRetries: 3,
		},
		logger: mockLog,
	}

	assert.True(t, wrapper.shouldRetry(0))
	assert.True(t, wrapper.shouldRetry(1))
	assert.True(t, wrapper.shouldRetry(2))
	assert.False(t, wrapper.shouldRetry(3))
	assert.False(t, wrapper.shouldRetry(4))
}

func TestIsRetryableError(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
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

func TestNewMigrationWrapper_RequiresMigrationsPath(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	config := DefaultConfig()
	config.Component = "test" // Set Component to pass that validation
	// MigrationsPath is intentionally left empty (zero value from DefaultConfig)

	wrapper, err := NewMigrationWrapper(nil, config, mockLog)

	assert.Nil(t, wrapper)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MigrationsPath is required")
	assert.Contains(t, err.Error(), "DefaultConfig()") // Verify helpful message
}

func TestNewMigrationWrapper_RequiresComponent(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	config := DefaultConfig()
	config.MigrationsPath = "/tmp/migrations" // Set MigrationsPath to pass that validation
	// Component is intentionally left empty (zero value from DefaultConfig)

	wrapper, err := NewMigrationWrapper(nil, config, mockLog)

	assert.Nil(t, wrapper)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Component is required")
	assert.Contains(t, err.Error(), "DefaultConfig()") // Verify helpful message
}

func TestPreflightCheck_ConnectionError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Simulate connection error (NOT sql.ErrNoRows, NOT "does not exist")
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnError(errors.New("connection refused"))

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
	}

	ctx := context.Background()
	_, err = wrapper.PreflightCheck(ctx, db)

	// Connection errors should NOT be swallowed
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query schema_migrations")
}

func TestPreflightCheck_TableDoesNotExist(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Simulate "table does not exist" error
	mock.ExpectQuery("SELECT version, dirty FROM schema_migrations").
		WillReturnError(errors.New("pq: relation \"schema_migrations\" does not exist"))

	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: DefaultConfig(),
		logger: mockLog,
	}

	ctx := context.Background()
	status, err := wrapper.PreflightCheck(ctx, db)

	// "Table does not exist" should be treated as fresh database
	assert.NoError(t, err)
	assert.Equal(t, 0, status.Version)
	assert.False(t, status.Dirty)
}

func TestRecoverDirtyMigration_MigrationFileNotFound(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tmpDir := t.TempDir()
	// Don't create migration file - intentionally missing

	wrapper, ctrl := newTestWrapper(t, MigrationConfig{
		Component:        "transaction",
		AutoRecoverDirty: true,
		MigrationsPath:   tmpDir,
	})
	defer ctrl.Finish()

	ctx := context.Background()
	err = wrapper.recoverDirtyMigration(ctx, db, 999) // Version 999 doesn't exist

	assert.ErrorIs(t, err, ErrMigrationFileNotFound)
}

func TestGetHealthStatus_Healthy(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component: "transaction",
		},
		logger: mockLog,
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
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	wrapper := &MigrationWrapper{
		config: MigrationConfig{
			Component: "onboarding",
		},
		logger: mockLog,
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
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	// Test healthy state
	healthyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: mockLog,
		status: MigrationStatus{Dirty: false, LastError: nil},
	}
	assert.True(t, healthyWrapper.IsHealthy())

	// Test unhealthy (dirty)
	dirtyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: mockLog,
		status: MigrationStatus{Dirty: true, LastError: nil},
	}
	assert.False(t, dirtyWrapper.IsHealthy())

	// Test unhealthy (error)
	errorWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: mockLog,
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

func TestFiberReadinessCheck(t *testing.T) {
	mockLog, ctrl := newTestMockLogger(t)
	defer ctrl.Finish()

	// Test healthy state
	healthyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: mockLog,
		status: MigrationStatus{
			Version: 18,
			Dirty:   false,
		},
	}

	assert.True(t, FiberReadinessCheck(healthyWrapper))

	// Test unhealthy state
	unhealthyWrapper := &MigrationWrapper{
		config: MigrationConfig{Component: "test"},
		logger: mockLog,
		status: MigrationStatus{
			Version:   15,
			Dirty:     true,
			LastError: ErrMigrationDirty,
		},
	}

	assert.False(t, FiberReadinessCheck(unhealthyWrapper))
}
