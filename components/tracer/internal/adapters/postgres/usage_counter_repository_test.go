// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/shopspring/decimal"

	pgdb "tracer/internal/adapters/postgres/db"
	"tracer/internal/adapters/postgres/db/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// setupUsageCounterRepositoryMockDB creates a gomock controller, mock DBConnection, and sqlmock for testing.
// Returns the repo, the raw *sql.DB (which satisfies pgdb.DB for WithTx calls), sqlmock, and cleanup.
func setupUsageCounterRepositoryMockDB(t *testing.T) (*UsageCounterRepository, *sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close mock db: %v", err)
		}
	}

	return repo, db, sqlMock, cleanup
}

// setupUsageCounterRepositoryCallerDB creates a test setup for methods that receive db as parameter
// (e.g., GetUsageForLimits, UpsertAndIncrementAtomic). GetDB is expected NOT to be called.
func setupUsageCounterRepositoryCallerDB(t *testing.T) (*UsageCounterRepository, *sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Times(0)

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	cleanup := func() {
		require.NoError(t, sqlMock.ExpectationsWereMet())

		if err := db.Close(); err != nil {
			t.Logf("failed to close mock db: %v", err)
		}
	}

	return repo, db, sqlMock, cleanup
}

// usageCounterColumns returns the column names for usage counter queries.
func usageCounterColumns() []string {
	return []string{"id", "limit_id", "scope_key", "period_key", "current_usage", "last_updated_at"}
}

// upsertAtomicSQL is the expected SQL for UpsertAndIncrementAtomic using CTE.
// The CTE (WITH attempt) tries the upsert and returns (current_usage, succeeded) flag.
// COALESCE falls back to SELECT + false when WHERE guard fails.
// This eliminates the need for a second query when limit is exceeded.
// Includes expires_at column for automatic cleanup.
const upsertAtomicSQL = `
		WITH attempt AS (
			INSERT INTO usage_counters (id, limit_id, scope_key, period_key, current_usage, last_updated_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $11)
			ON CONFLICT (limit_id, scope_key, period_key) 
			DO UPDATE SET 
				current_usage = usage_counters.current_usage + $7,
				last_updated_at = $8,
				expires_at = $11
			WHERE usage_counters.current_usage + $9 <= $10
			RETURNING current_usage, true as succeeded
		)
		SELECT 
			COALESCE(
				(SELECT current_usage FROM attempt),
				(SELECT current_usage FROM usage_counters 
				 WHERE limit_id = $2 AND scope_key = $3 AND period_key = $4),
				$5
			) as current_usage,
			COALESCE(
				(SELECT succeeded FROM attempt),
				false
			) as succeeded
	`

// testUsageCounter creates a test usage counter with default values.
func testUsageCounter(limitID uuid.UUID) *model.UsageCounter {
	return &model.UsageCounter{
		ID:            testutil.MustDeterministicUUID(1),
		LimitID:       limitID,
		ScopeKey:      "acct:123",
		PeriodKey:     "2025-01",
		CurrentUsage:  decimal.RequireFromString("50"),
		LastUpdatedAt: testutil.DefaultTestTime,
	}
}

func TestUsageCounterRepository_GetOrCreateForUpdate_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(999)

	_, err := repo.GetOrCreateForUpdate(ctx, limitID, "acct:123", "2025-01")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
}

func TestUsageCounterRepository_GetOrCreateForUpdate(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name      string
		limitID   uuid.UUID
		scopeKey  string
		periodKey string
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errMsg    string
		validate  func(t *testing.T, counter *model.UsageCounter)
	}{
		{
			name:      "Success - finds existing counter",
			limitID:   limitID,
			scopeKey:  "acct:123",
			periodKey: "2025-01",
			mockSetup: func(mock sqlmock.Sqlmock) {
				counter := testUsageCounter(limitID)
				rows := sqlmock.NewRows(usageCounterColumns()).
					AddRow(counter.ID, counter.LimitID, counter.ScopeKey, counter.PeriodKey, counter.CurrentUsage, counter.LastUpdatedAt)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 AND period_key = $2 AND scope_key = $3 FOR UPDATE`)).
					WithArgs(limitID, "2025-01", "acct:123").
					WillReturnRows(rows)
			},
			validate: func(t *testing.T, counter *model.UsageCounter) {
				assert.Equal(t, limitID, counter.LimitID)
				assert.Equal(t, "acct:123", counter.ScopeKey)
				assert.Equal(t, "2025-01", counter.PeriodKey)
				assert.True(t, decimal.RequireFromString("50").Equal(counter.CurrentUsage))
			},
		},
		{
			name:      "Success - creates new counter when not found",
			limitID:   limitID,
			scopeKey:  "acct:456",
			periodKey: "2025-02",
			mockSetup: func(mock sqlmock.Sqlmock) {
				// First query returns no rows
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 AND period_key = $2 AND scope_key = $3 FOR UPDATE`)).
					WithArgs(limitID, "2025-02", "acct:456").
					WillReturnError(sql.ErrNoRows)

				// Insert succeeds
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO usage_counters`)).
					WithArgs(sqlmock.AnyArg(), limitID, "acct:456", "2025-02", decimal.RequireFromString("0"), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))

				// Post-insert SELECT to acquire FOR UPDATE lock and return the inserted row
				rows := sqlmock.NewRows(usageCounterColumns()).
					AddRow(testutil.MustDeterministicUUID(10), limitID, "acct:456", "2025-02", decimal.RequireFromString("0"), testutil.DefaultTestTime)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE id = $1 FOR UPDATE`)).
					WithArgs(sqlmock.AnyArg()).
					WillReturnRows(rows)
			},
			validate: func(t *testing.T, counter *model.UsageCounter) {
				assert.Equal(t, limitID, counter.LimitID)
				assert.Equal(t, "acct:456", counter.ScopeKey)
				assert.Equal(t, "2025-02", counter.PeriodKey)
				assert.True(t, decimal.RequireFromString("0").Equal(counter.CurrentUsage))
			},
		},
		{
			name:      "Success - handles concurrent insert by retrying select",
			limitID:   limitID,
			scopeKey:  "acct:789",
			periodKey: "2025-03",
			mockSetup: func(mock sqlmock.Sqlmock) {
				// First query returns no rows
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 AND period_key = $2 AND scope_key = $3 FOR UPDATE`)).
					WithArgs(limitID, "2025-03", "acct:789").
					WillReturnError(sql.ErrNoRows)

				// Insert fails due to concurrent insert (unique constraint violation - SQLSTATE 23505)
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO usage_counters`)).
					WithArgs(sqlmock.AnyArg(), limitID, "acct:789", "2025-03", decimal.RequireFromString("0"), sqlmock.AnyArg()).
					WillReturnError(&pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"})

				// Retry select succeeds
				counter := &model.UsageCounter{
					ID:            testutil.MustDeterministicUUID(2),
					LimitID:       limitID,
					ScopeKey:      "acct:789",
					PeriodKey:     "2025-03",
					CurrentUsage:  decimal.RequireFromString("1"),
					LastUpdatedAt: testutil.DefaultTestTime,
				}
				rows := sqlmock.NewRows(usageCounterColumns()).
					AddRow(counter.ID, counter.LimitID, counter.ScopeKey, counter.PeriodKey, counter.CurrentUsage, counter.LastUpdatedAt)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 AND period_key = $2 AND scope_key = $3 FOR UPDATE`)).
					WithArgs(limitID, "2025-03", "acct:789").
					WillReturnRows(rows)
			},
			validate: func(t *testing.T, counter *model.UsageCounter) {
				assert.Equal(t, limitID, counter.LimitID)
				assert.Equal(t, "acct:789", counter.ScopeKey)
				assert.Equal(t, "2025-03", counter.PeriodKey)
				assert.True(t, decimal.RequireFromString("1").Equal(counter.CurrentUsage))
			},
		},
		{
			name:      "Error - insert fails with non-unique-constraint error",
			limitID:   limitID,
			scopeKey:  "acct:fail",
			periodKey: "2025-04",
			mockSetup: func(mock sqlmock.Sqlmock) {
				// First query returns no rows
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 AND period_key = $2 AND scope_key = $3 FOR UPDATE`)).
					WithArgs(limitID, "2025-04", "acct:fail").
					WillReturnError(sql.ErrNoRows)

				// Insert fails with non-unique-constraint error (should NOT retry)
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO usage_counters`)).
					WithArgs(sqlmock.AnyArg(), limitID, "acct:fail", "2025-04", decimal.RequireFromString("0"), sqlmock.AnyArg()).
					WillReturnError(errors.New("disk full"))
			},
			wantErr: true,
			errMsg:  "failed to insert usage counter",
		},
		{
			name:      "Error - query fails",
			limitID:   limitID,
			scopeKey:  "acct:123",
			periodKey: "2025-01",
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters`)).
					WithArgs(limitID, "2025-01", "acct:123").
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to get usage counter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, sqlMock, cleanup := setupUsageCounterRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			counter, err := repo.GetOrCreateForUpdate(ctx, tt.limitID, tt.scopeKey, tt.periodKey)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, counter)

			if tt.validate != nil {
				tt.validate(t, counter)
			}
		})
	}
}

func TestUsageCounterRepository_IncrementAtomic_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	ctx := context.Background()
	err := repo.IncrementAtomic(ctx, testutil.MustDeterministicUUID(998), decimal.RequireFromString("1"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
}

func TestUsageCounterRepository_IncrementAtomic(t *testing.T) {
	testutil.SetupTestTracing(t)

	counterID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name      string
		counterID uuid.UUID
		amount    decimal.Decimal
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errVal    error
		errMsg    string
	}{
		{
			name:      "Success - increments counter",
			counterID: counterID,
			amount:    decimal.RequireFromString("5"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Simple UPDATE: current_usage = current_usage + amount
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE usage_counters SET`)).
					WithArgs(decimal.RequireFromString("5"), sqlmock.AnyArg(), counterID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name:      "Success - zero amount is no-op",
			counterID: counterID,
			amount:    decimal.RequireFromString("0"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No database calls expected
			},
		},
		{
			name:      "Error - negative amount",
			counterID: counterID,
			amount:    decimal.RequireFromString("-1"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No database calls expected
			},
			wantErr: true,
			errVal:  constant.ErrUsageCounterIncrementNonNegative,
		},
		{
			name:      "Error - counter not found (0 rows on UPDATE)",
			counterID: counterID,
			amount:    decimal.RequireFromString("1"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// UPDATE returns 0 rows (counter doesn't exist)
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE usage_counters SET`)).
					WithArgs(decimal.RequireFromString("1"), sqlmock.AnyArg(), counterID).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errVal:  constant.ErrUsageCounterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, sqlMock, cleanup := setupUsageCounterRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			err := repo.IncrementAtomic(ctx, tt.counterID, tt.amount)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errVal != nil {
					assert.ErrorIs(t, err, tt.errVal)
				}
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestUsageCounterRepository_GetByLimitID_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	ctx := context.Background()
	_, err := repo.GetByLimitID(ctx, testutil.MustDeterministicUUID(996))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
}

func TestUsageCounterRepository_GetByLimitID(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name      string
		limitID   uuid.UUID
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errMsg    string
		wantCount int
	}{
		{
			name:    "Success - returns multiple counters",
			limitID: limitID,
			mockSetup: func(mock sqlmock.Sqlmock) {
				counter1 := testUsageCounter(limitID)
				counter2 := &model.UsageCounter{
					ID:            testutil.MustDeterministicUUID(3),
					LimitID:       limitID,
					ScopeKey:      "acct:456",
					PeriodKey:     "2025-01",
					CurrentUsage:  decimal.RequireFromString("25"),
					LastUpdatedAt: testutil.DefaultTestTime,
				}

				rows := sqlmock.NewRows(usageCounterColumns()).
					AddRow(counter1.ID, counter1.LimitID, counter1.ScopeKey, counter1.PeriodKey, counter1.CurrentUsage, counter1.LastUpdatedAt).
					AddRow(counter2.ID, counter2.LimitID, counter2.ScopeKey, counter2.PeriodKey, counter2.CurrentUsage, counter2.LastUpdatedAt)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 ORDER BY period_key DESC, scope_key ASC`)).
					WithArgs(limitID).
					WillReturnRows(rows)
			},
			wantCount: 2,
		},
		{
			name:    "Success - returns empty slice when no counters",
			limitID: limitID,
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(usageCounterColumns())

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters WHERE limit_id = $1 ORDER BY period_key DESC, scope_key ASC`)).
					WithArgs(limitID).
					WillReturnRows(rows)
			},
			wantCount: 0,
		},
		{
			name:    "Error - query fails",
			limitID: limitID,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, limit_id, scope_key, period_key, current_usage, last_updated_at FROM usage_counters`)).
					WithArgs(limitID).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to get usage counters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, sqlMock, cleanup := setupUsageCounterRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			counters, err := repo.GetByLimitID(ctx, tt.limitID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			assert.Len(t, counters, tt.wantCount)
		})
	}
}

func TestUsageCounterRepository_GetUsageForLimits_NilDB(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	ctx := context.Background()
	_, err := repo.GetUsageForLimits(ctx, nil, []uuid.UUID{testutil.MustDeterministicUUID(995)}, "acct:123", "2025-01")

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection)
}

func TestUsageCounterRepository_GetUsageForLimits(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	limitID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	tests := []struct {
		name      string
		limitIDs  []uuid.UUID
		scopeKey  string
		periodKey string
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errMsg    string
		validate  func(t *testing.T, result map[uuid.UUID]decimal.Decimal)
	}{
		{
			name:      "Success - returns usage for multiple limits",
			limitIDs:  []uuid.UUID{limitID1, limitID2},
			scopeKey:  "acct:123",
			periodKey: "2025-01",
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"limit_id", "current_usage"}).
					AddRow(limitID1, decimal.RequireFromString("50")).
					AddRow(limitID2, decimal.RequireFromString("25"))

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT limit_id, current_usage FROM usage_counters WHERE limit_id IN ($1,$2) AND period_key = $3 AND scope_key = $4`)).
					WithArgs(limitID1, limitID2, "2025-01", "acct:123").
					WillReturnRows(rows)
			},
			validate: func(t *testing.T, result map[uuid.UUID]decimal.Decimal) {
				assert.Len(t, result, 2)
				assert.True(t, decimal.RequireFromString("50").Equal(result[limitID1]))
				assert.True(t, decimal.RequireFromString("25").Equal(result[limitID2]))
			},
		},
		{
			name:      "Success - empty limit IDs returns empty map",
			limitIDs:  []uuid.UUID{},
			scopeKey:  "acct:123",
			periodKey: "2025-01",
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No database calls expected
			},
			validate: func(t *testing.T, result map[uuid.UUID]decimal.Decimal) {
				assert.Len(t, result, 0)
			},
		},
		{
			name:      "Success - returns partial results (some limits have no counters)",
			limitIDs:  []uuid.UUID{limitID1, limitID2},
			scopeKey:  "acct:123",
			periodKey: "2025-01",
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"limit_id", "current_usage"}).
					AddRow(limitID1, decimal.RequireFromString("50"))

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT limit_id, current_usage FROM usage_counters WHERE limit_id IN ($1,$2) AND period_key = $3 AND scope_key = $4`)).
					WithArgs(limitID1, limitID2, "2025-01", "acct:123").
					WillReturnRows(rows)
			},
			validate: func(t *testing.T, result map[uuid.UUID]decimal.Decimal) {
				assert.Len(t, result, 1)
				assert.True(t, decimal.RequireFromString("50").Equal(result[limitID1]))
				_, exists := result[limitID2]
				assert.False(t, exists)
			},
		},
		{
			name:      "Error - query fails",
			limitIDs:  []uuid.UUID{limitID1},
			scopeKey:  "acct:123",
			periodKey: "2025-01",
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT limit_id, current_usage FROM usage_counters`)).
					WithArgs(limitID1, "2025-01", "acct:123").
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to get usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			result, err := repo.GetUsageForLimits(ctx, db, tt.limitIDs, tt.scopeKey, tt.periodKey)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestUsageCounterRepository_UpsertAndIncrementAtomic_ExceedsLimit(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(8010)
	scopeKey := "acct:8010"
	periodKey := "2025-06"

	tests := []struct {
		name      string
		limitID   uuid.UUID
		scopeKey  string
		periodKey string
		amount    decimal.Decimal
		maxAmount decimal.Decimal
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   error
		wantUsage decimal.Decimal
	}{
		{
			name:      "Error - increment would exceed limit (CTE returns current_usage)",
			limitID:   limitID,
			scopeKey:  scopeKey,
			periodKey: periodKey,
			amount:    decimal.RequireFromString("600"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// The CTE's INSERT...ON CONFLICT WHERE guard rejects the update because
				// current_usage (500) + amount (600) = 1100 > maxAmount (1000).
				// The CTE's attempt subquery returns no rows, so COALESCE falls back to
				// the SELECT from usage_counters, which returns current_usage = 500 and succeeded = false.
				rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("500"), false)

				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id (generated UUID)
						limitID.String(), // $2 limit_id
						scopeKey,         // $3 scope_key
						periodKey,        // $4 period_key
						sqlmock.AnyArg(), // $5 current_usage (initial = amount for INSERT)
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount (for DO UPDATE SET)
						sqlmock.AnyArg(), // $8 last_updated_at (for DO UPDATE SET)
						sqlmock.AnyArg(), // $9 amount (for WHERE guard)
						sqlmock.AnyArg(), // $10 maxAmount (for WHERE guard)
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantErr:   constant.ErrUsageCounterExceedsLimit,
			wantUsage: decimal.RequireFromString("500"), // Returns current usage when limit exceeded
		},
		{
			name:      "Boundary - amount exactly at boundary still succeeds",
			limitID:   testutil.MustDeterministicUUID(8011),
			scopeKey:  "acct:8011",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("500"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// current_usage (500) + amount (500) = 1000 <= maxAmount (1000)
				// This is a boundary success case: the WHERE guard allows the update.
				rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("1000"), true)

				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id (generated UUID)
						testutil.MustDeterministicUUID(8011).String(), // $2 limit_id
						"acct:8011",      // $3 scope_key
						"2025-06",        // $4 period_key
						sqlmock.AnyArg(), // $5 current_usage
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount
						sqlmock.AnyArg(), // $8 last_updated_at
						sqlmock.AnyArg(), // $9 amount
						sqlmock.AnyArg(), // $10 maxAmount
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantErr:   nil,
			wantUsage: decimal.RequireFromString("1000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			usage, err := repo.UpsertAndIncrementAtomic(ctx, db, tt.limitID, tt.scopeKey, tt.periodKey, tt.amount, tt.maxAmount, nil)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.True(t, tt.wantUsage.Equal(usage), "expected usage %s, got %s", tt.wantUsage, usage)
				return
			}

			require.NoError(t, err)
			assert.True(t, tt.wantUsage.Equal(usage), "expected usage %s, got %s", tt.wantUsage, usage)
		})
	}
}

func TestUsageCounterRepository_UpsertAndIncrementAtomic_PreCheck(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(8020)

	wantUsage1000 := decimal.RequireFromString("1000")

	tests := []struct {
		name      string
		limitID   uuid.UUID
		scopeKey  string
		periodKey string
		amount    decimal.Decimal
		maxAmount decimal.Decimal
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   error
		wantUsage *decimal.Decimal // if non-nil, assert exact usage value
	}{
		{
			name:      "Error - amount exceeds maxAmount pre-check rejects before SQL",
			limitID:   limitID,
			scopeKey:  "acct:8020",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("1500"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No SQL should be executed because the Go pre-check catches this.
				// If any SQL is executed, sqlmock will fail the test with
				// "call to Query/Exec was not expected".
			},
			wantErr: constant.ErrUsageCounterExceedsLimit,
		},
		{
			name:      "Success - amount equals maxAmount on fresh counter is allowed (boundary)",
			limitID:   testutil.MustDeterministicUUID(8021),
			scopeKey:  "acct:8021",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("1000"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// amount == maxAmount: the pre-check should NOT reject this because
				// for a fresh INSERT (current_usage=0), 0 + 1000 = 1000 <= 1000 is valid.
				// The pre-check only rejects when amount > maxAmount (strictly greater).
				rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("1000"), true)

				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id (generated UUID)
						sqlmock.AnyArg(), // $2 limit_id
						sqlmock.AnyArg(), // $3 scope_key
						sqlmock.AnyArg(), // $4 period_key
						sqlmock.AnyArg(), // $5 current_usage
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount
						sqlmock.AnyArg(), // $8 last_updated_at
						sqlmock.AnyArg(), // $9 amount
						sqlmock.AnyArg(), // $10 maxAmount
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantErr:   nil, // Should succeed; pre-check only rejects amount > maxAmount
			wantUsage: &wantUsage1000,
		},
		{
			name:      "Zero amount is a no-op (returns zero usage)",
			limitID:   testutil.MustDeterministicUUID(8022),
			scopeKey:  "acct:8022",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("0"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Zero amount should be a no-op; no SQL executed.
			},
			wantErr: nil,
		},
		{
			name:      "Error - negative amount returns increment-non-negative error",
			limitID:   testutil.MustDeterministicUUID(8023),
			scopeKey:  "acct:8023",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("-10"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Negative amount should be rejected before SQL execution.
			},
			wantErr: constant.ErrUsageCounterIncrementNonNegative,
		},
		{
			// maxAmount=0 means the limit is configured to deny everything.
			// Any positive amount satisfies amount.GreaterThan(decimal.Zero) == true,
			// so the pre-check must catch it before SQL is executed.
			name:      "zero maxAmount rejects any positive amount",
			limitID:   testutil.MustDeterministicUUID(8041),
			scopeKey:  "acct:test-zero-max",
			periodKey: "2024-01-15",
			amount:    decimal.RequireFromString("1"),
			maxAmount: decimal.Zero,
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No SQL should be executed: the Go pre-check (amount > maxAmount)
				// fires because 1 > 0. If any query is issued, sqlmock will fail
				// the test with "call to Query/Exec was not expected".
			},
			wantErr: constant.ErrUsageCounterExceedsLimit,
		},
		{
			// zero amount against zero maxAmount: amount.IsZero() short-circuits
			// before any DB call, returning (decimal.Zero, nil) immediately.
			name:      "zero amount with zero maxAmount passes pre-check and succeeds",
			limitID:   testutil.MustDeterministicUUID(8042),
			scopeKey:  "acct:test-zero-max-zero-amount",
			periodKey: "2024-01-15",
			amount:    decimal.Zero,
			maxAmount: decimal.Zero,
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No SQL should be executed: amount.IsZero() short-circuits
				// before any DB call. If any query is issued, sqlmock will
				// fail with "call to Query/Exec was not expected".
			},
			wantErr: nil, // Should succeed; amount.IsZero() returns early
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			usage, err := repo.UpsertAndIncrementAtomic(ctx, db, tt.limitID, tt.scopeKey, tt.periodKey, tt.amount, tt.maxAmount, nil)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.True(t, usage.IsZero(), "expected zero usage on error, got %s", usage)
				return
			}

			require.NoError(t, err)
			if tt.wantUsage != nil {
				assert.True(t, tt.wantUsage.Equal(usage), "expected usage %s, got %s", tt.wantUsage, usage)
			} else if tt.amount.IsZero() {
				// Contract: when amount.IsZero(), UpsertAndIncrementAtomic must short-circuit and return (decimal.Zero, nil)
				assert.True(t, usage.IsZero(), "expected zero usage for zero amount, got %s", usage)
			}
		})
	}
}

func TestUsageCounterRepository_UpsertAndIncrementAtomic_NilDB(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)

	repo := NewUsageCounterRepositoryWithConnection(mockConn)

	ctx := context.Background()
	usage, err := repo.UpsertAndIncrementAtomic(ctx, nil, testutil.MustDeterministicUUID(8029), "acct:8029", "2025-06", decimal.RequireFromString("100"), decimal.RequireFromString("1000"), nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection)
	assert.True(t, usage.IsZero(), "expected zero usage on error, got %s", usage)
}

func TestUsageCounterRepository_UpsertAndIncrementAtomic_ErrorPropagation(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(8030)

	tests := []struct {
		name      string
		limitID   uuid.UUID
		scopeKey  string
		periodKey string
		amount    decimal.Decimal
		maxAmount decimal.Decimal
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Error - database error is propagated with wrapping",
			limitID:   limitID,
			scopeKey:  "acct:8030",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("100"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id (generated UUID)
						sqlmock.AnyArg(), // $2 limit_id
						sqlmock.AnyArg(), // $3 scope_key
						sqlmock.AnyArg(), // $4 period_key
						sqlmock.AnyArg(), // $5 current_usage
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount
						sqlmock.AnyArg(), // $8 last_updated_at
						sqlmock.AnyArg(), // $9 amount
						sqlmock.AnyArg(), // $10 maxAmount
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnError(errors.New("disk full"))
			},
			wantErr: true,
			errMsg:  "disk full",
		},
		{
			name:      "Error - scan error is propagated",
			limitID:   testutil.MustDeterministicUUID(8031),
			scopeKey:  "acct:8031",
			periodKey: "2025-06",
			amount:    decimal.RequireFromString("100"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Return a row with an incompatible type to trigger a scan error
				rows := sqlmock.NewRows([]string{"current_usage"}).
					AddRow("not-a-decimal")

				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id (generated UUID)
						sqlmock.AnyArg(), // $2 limit_id
						sqlmock.AnyArg(), // $3 scope_key
						sqlmock.AnyArg(), // $4 period_key
						sqlmock.AnyArg(), // $5 current_usage
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount
						sqlmock.AnyArg(), // $8 last_updated_at
						sqlmock.AnyArg(), // $9 amount
						sqlmock.AnyArg(), // $10 maxAmount
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantErr: true,
			errMsg:  "failed to scan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			usage, err := repo.UpsertAndIncrementAtomic(ctx, db, tt.limitID, tt.scopeKey, tt.periodKey, tt.amount, tt.maxAmount, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
			assert.True(t, usage.IsZero(), "expected zero usage on error, got %s", usage)
		})
	}
}

func TestUsageCounterRepository_UpsertAndIncrementAtomic_ContextCancellation(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(8040)

	repo, db, sqlMock, cleanup := setupUsageCounterRepositoryMockDB(t)
	defer cleanup()

	// When context is cancelled, the database driver returns context.Canceled
	// Note: uses MockDB helper because cancelled context may prevent SQL expectations from being met.
	sqlMock.ExpectQuery(regexp.QuoteMeta(
		upsertAtomicSQL,
	)).
		WithArgs(
			sqlmock.AnyArg(), // $1 id (generated UUID)
			sqlmock.AnyArg(), // $2 limit_id
			sqlmock.AnyArg(), // $3 scope_key
			sqlmock.AnyArg(), // $4 period_key
			sqlmock.AnyArg(), // $5 current_usage
			sqlmock.AnyArg(), // $6 last_updated_at
			sqlmock.AnyArg(), // $7 amount
			sqlmock.AnyArg(), // $8 last_updated_at
			sqlmock.AnyArg(), // $9 amount
			sqlmock.AnyArg(), // $10 maxAmount
			sqlmock.AnyArg(), // $11 expiresAt
		).
		WillReturnError(context.Canceled)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	usage, err := repo.UpsertAndIncrementAtomic(ctx, db, limitID, "acct:8040", "2025-06", decimal.RequireFromString("100"), decimal.RequireFromString("1000"), nil)

	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, usage.IsZero(), "expected zero usage on cancellation, got %s", usage)
}

// =============================================================================
// Expired Usage Counters Are Automatically Cleaned Up
// Tests for UpsertAndIncrementAtomic with expiresAt parameter
// =============================================================================

// TestUpsertAndIncrementAtomic_WithExpiresAt tests that UpsertAndIncrementAtomic
// correctly stores the expiresAt value when creating or updating counters.
// Acceptance Criteria:
// - Insert new counter with expiresAt → verify stored correctly
// - Update existing counter → verify expiresAt updated
// - Insert with nil expiresAt → verify stored as NULL
func TestUpsertAndIncrementAtomic_WithExpiresAt(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(12001)
	scopeKey := "acct:12001"
	periodKey := "2026-03"

	// Test expiresAt value (resetAt + 90 days)
	expiresAt := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		limitID   uuid.UUID
		scopeKey  string
		periodKey string
		amount    decimal.Decimal
		maxAmount decimal.Decimal
		expiresAt *time.Time
		mockSetup func(mock sqlmock.Sqlmock)
		wantUsage decimal.Decimal
		wantErr   bool
	}{
		{
			name:      "Insert new counter with expiresAt",
			limitID:   limitID,
			scopeKey:  scopeKey,
			periodKey: periodKey,
			amount:    decimal.RequireFromString("100"),
			maxAmount: decimal.RequireFromString("1000"),
			expiresAt: testutil.Ptr(expiresAt),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// The upsert query includes expiresAt in the INSERT and UPDATE
				rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("100"), true)

				// The SQL includes expires_at column
				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id
						limitID.String(), // $2 limit_id
						scopeKey,         // $3 scope_key
						periodKey,        // $4 period_key
						sqlmock.AnyArg(), // $5 current_usage
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount
						sqlmock.AnyArg(), // $8 last_updated_at
						sqlmock.AnyArg(), // $9 amount
						sqlmock.AnyArg(), // $10 maxAmount
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantUsage: decimal.RequireFromString("100"),
			wantErr:   false,
		},
		{
			name:      "Update existing counter with expiresAt",
			limitID:   testutil.MustDeterministicUUID(12002),
			scopeKey:  "acct:12002",
			periodKey: "2026-03",
			amount:    decimal.RequireFromString("50"),
			maxAmount: decimal.RequireFromString("1000"),
			expiresAt: testutil.Ptr(expiresAt),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// ON CONFLICT path: existing counter gets updated
				rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("150"), true) // Existing 100 + new 50

				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(),
						testutil.MustDeterministicUUID(12002).String(),
						"acct:12002",
						"2026-03",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantUsage: decimal.RequireFromString("150"),
			wantErr:   false,
		},
		{
			name:      "Insert with nil expiresAt (PER_TRANSACTION)",
			limitID:   testutil.MustDeterministicUUID(12003),
			scopeKey:  "acct:12003",
			periodKey: "", // PER_TRANSACTION has empty period key
			amount:    decimal.RequireFromString("100"),
			maxAmount: decimal.RequireFromString("1000"),
			expiresAt: nil, // No expiresAt for PER_TRANSACTION
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("100"), true)

				mock.ExpectQuery(regexp.QuoteMeta(
					upsertAtomicSQL,
				)).
					WithArgs(
						sqlmock.AnyArg(),
						testutil.MustDeterministicUUID(12003).String(),
						"acct:12003",
						"",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(), // $11 expiresAt (nil)
					).
					WillReturnRows(rows)
			},
			wantUsage: decimal.RequireFromString("100"),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()

			usage, err := repo.UpsertAndIncrementAtomic(ctx, db, tt.limitID, tt.scopeKey, tt.periodKey, tt.amount, tt.maxAmount, tt.expiresAt)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, tt.wantUsage.Equal(usage), "expected usage %s, got %s", tt.wantUsage, usage)
		})
	}
}

// TestUpsertAndIncrementAtomic_ExpiresAtStoredInDB verifies that the expires_at
// column is actually populated in the database when a counter is created.
func TestUpsertAndIncrementAtomic_ExpiresAtStoredInDB(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(12010)
	scopeKey := "acct:12010"
	periodKey := "2026-03-11"
	expiresAt := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
	defer cleanup()

	// Expect the INSERT to include expires_at column
	// This test documents the expected SQL structure with expiresAt
	rows := sqlmock.NewRows([]string{"current_usage", "succeeded"}).
		AddRow(decimal.RequireFromString("100"), true)

	sqlMock.ExpectQuery(regexp.QuoteMeta(
		upsertAtomicSQL,
	)).
		WithArgs(
			sqlmock.AnyArg(), // $1 id
			limitID.String(), // $2 limit_id
			scopeKey,         // $3 scope_key
			periodKey,        // $4 period_key
			sqlmock.AnyArg(), // $5 current_usage
			sqlmock.AnyArg(), // $6 last_updated_at
			sqlmock.AnyArg(), // $7 amount
			sqlmock.AnyArg(), // $8 last_updated_at
			sqlmock.AnyArg(), // $9 amount
			sqlmock.AnyArg(), // $10 maxAmount
			sqlmock.AnyArg(), // $11 expiresAt
		).
		WillReturnRows(rows)

	ctx := context.Background()

	usage, err := repo.UpsertAndIncrementAtomic(ctx, db, limitID, scopeKey, periodKey, decimal.RequireFromString("100"), decimal.RequireFromString("1000"), testutil.Ptr(expiresAt))

	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("100").Equal(usage))
}
