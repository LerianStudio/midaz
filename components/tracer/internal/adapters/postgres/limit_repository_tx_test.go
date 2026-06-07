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

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// setupLimitRepoWithSQLMock returns a bare *LimitRepository (the conn field is
// left nil because *WithTx methods bypass Connection.GetDB and use the caller-
// provided pgdb.DB directly), a *sql.DB that satisfies pgdb.DB, the sqlmock
// for query expectations, and a cleanup function.
func setupLimitRepoWithSQLMock(t *testing.T) (*LimitRepository, *sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	repo := &LimitRepository{
		tableName: "limits",
	}

	cleanup := func() {
		sqlMock.ExpectClose()
		require.NoError(t, db.Close())
	}

	return repo, db, sqlMock, cleanup
}

// ============================================================================
// CreateWithTx
// ============================================================================

func TestLimitRepository_CreateWithTx(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		mockSetup func(mock sqlmock.Sqlmock, lmt *model.Limit)
		wantErr   bool
		errType   error
		errMsg    string
	}{
		{
			name: "Success - inserts limit using supplied db handle",
			mockSetup: func(mock sqlmock.Sqlmock, lmt *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO limits`)).
					WithArgs(
						lmt.ID,
						lmt.Name,
						lmt.Description,
						lmt.LimitType,
						lmt.MaxAmount,
						lmt.Currency,
						sqlmock.AnyArg(), // scopesJSON
						lmt.Status,
						sqlmock.AnyArg(), // resetAt
						sqlmock.AnyArg(), // activeTimeStart
						sqlmock.AnyArg(), // activeTimeEnd
						sqlmock.AnyArg(), // customStartDate
						sqlmock.AnyArg(), // customEndDate
						lmt.CreatedAt,
						lmt.UpdatedAt,
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "Error - database insert fails",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO limits`)).
					WillReturnError(errors.New("connection lost"))
			},
			wantErr: true,
			errMsg:  "failed to insert limit",
		},
		{
			name: "Error - unique name violation maps to ErrLimitNameAlreadyExists",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO limits`)).
					WillReturnError(&pgconn.PgError{
						Code:           "23505",
						ConstraintName: "idx_limits_name_active",
						Message:        "duplicate key value violates unique constraint",
					})
			},
			wantErr: true,
			errType: constant.ErrLimitNameAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupLimitRepoWithSQLMock(t)
			defer cleanup()

			lmt := testLimit()
			tt.mockSetup(sqlMock, lmt)

			err := repo.CreateWithTx(context.Background(), db, lmt)

			if tt.wantErr {
				require.Error(t, err)

				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}

				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

// TestLimitRepository_CreateWithTx_NilDB verifies that CreateWithTx rejects a
// nil db handle at the public boundary with pgdb.ErrNilConnection instead of
// silently resolving a non-transactional connection via r.conn.GetDB. This
// preserves the atomicity contract advertised by *WithTx methods: callers
// MUST provide a transaction.
func TestLimitRepository_CreateWithTx_NilDB(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo := &LimitRepository{tableName: "limits"}
	lmt := testLimit()

	err := repo.CreateWithTx(context.Background(), nil, lmt)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil db must surface as pgdb.ErrNilConnection")
}

// ============================================================================
// UpdateWithTx
// ============================================================================

func TestLimitRepository_UpdateWithTx(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		mockSetup func(mock sqlmock.Sqlmock, lmt *model.Limit)
		wantErr   bool
		errType   error
		errMsg    string
	}{
		{
			name: "Success - updates limit using supplied db handle",
			mockSetup: func(mock sqlmock.Sqlmock, lmt *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WithArgs(
						lmt.Name,
						lmt.Description,
						lmt.MaxAmount,
						sqlmock.AnyArg(), // scopesJSON
						lmt.Status,
						sqlmock.AnyArg(), // resetAt
						sqlmock.AnyArg(), // activeTimeStart
						sqlmock.AnyArg(), // activeTimeEnd
						sqlmock.AnyArg(), // customStartDate
						sqlmock.AnyArg(), // customEndDate
						lmt.UpdatedAt,
						lmt.ID,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name: "Error - limit not found (zero rows)",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errType: constant.ErrLimitNotFound,
		},
		{
			name: "Error - database update fails",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to update limit",
		},
		{
			name: "Error - unique name violation maps to ErrLimitNameAlreadyExists",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Limit) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnError(&pgconn.PgError{
						Code:           "23505",
						ConstraintName: "idx_limits_name_active",
						Message:        "duplicate key value violates unique constraint",
					})
			},
			wantErr: true,
			errType: constant.ErrLimitNameAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupLimitRepoWithSQLMock(t)
			defer cleanup()

			lmt := testLimit()
			tt.mockSetup(sqlMock, lmt)

			err := repo.UpdateWithTx(context.Background(), db, lmt)

			if tt.wantErr {
				require.Error(t, err)

				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}

				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

// TestLimitRepository_UpdateWithTx_NilDB verifies that UpdateWithTx rejects a
// nil db handle at the public boundary with pgdb.ErrNilConnection instead of
// silently resolving a non-transactional connection via r.conn.GetDB. This
// preserves the atomicity contract advertised by *WithTx methods: callers
// MUST provide a transaction.
func TestLimitRepository_UpdateWithTx_NilDB(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo := &LimitRepository{tableName: "limits"}
	lmt := testLimit()

	err := repo.UpdateWithTx(context.Background(), nil, lmt)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil db must surface as pgdb.ErrNilConnection")
}

// ============================================================================
// UpdateStatusWithTx
// ============================================================================

func TestLimitRepository_UpdateStatusWithTx(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		limitID   uuid.UUID
		status    model.LimitStatus
		mockSetup func(mock sqlmock.Sqlmock)
		wantErr   bool
		errType   error
		errMsg    string
	}{
		{
			name:    "Success - updates status to inactive on provided db handle",
			limitID: testutil.MustDeterministicUUID(300),
			status:  model.LimitStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WithArgs(
						model.LimitStatusInactive,
						sqlmock.AnyArg(), // updated_at
						testutil.MustDeterministicUUID(300),
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:    "Success - updates to deleted and sets deleted_at",
			limitID: testutil.MustDeterministicUUID(301),
			status:  model.LimitStatusDeleted,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WithArgs(
						model.LimitStatusDeleted,
						sqlmock.AnyArg(), // updated_at
						sqlmock.AnyArg(), // deleted_at (same as updated_at)
						testutil.MustDeterministicUUID(301),
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:    "Error - limit not found (zero rows)",
			limitID: testutil.MustDeterministicUUID(302),
			status:  model.LimitStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errType: constant.ErrLimitNotFound,
		},
		{
			name:    "Error - database update fails",
			limitID: testutil.MustDeterministicUUID(303),
			status:  model.LimitStatusInactive,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE limits`)).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
			errMsg:  "failed to update limit status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupLimitRepoWithSQLMock(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			err := repo.UpdateStatusWithTx(context.Background(), db, tt.limitID, tt.status, testutil.FixedTime())

			if tt.wantErr {
				require.Error(t, err)

				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}

				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

// TestLimitRepository_UpdateStatusWithTx_NilDB verifies that UpdateStatusWithTx
// rejects a nil db handle at the public boundary with pgdb.ErrNilConnection
// instead of silently resolving a non-transactional connection via
// r.conn.GetDB. This preserves the atomicity contract advertised by *WithTx
// methods: callers MUST provide a transaction.
func TestLimitRepository_UpdateStatusWithTx_NilDB(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo := &LimitRepository{tableName: "limits"}

	err := repo.UpdateStatusWithTx(
		context.Background(),
		nil,
		testutil.MustDeterministicUUID(999),
		model.LimitStatusInactive,
		testutil.FixedTime(),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil db must surface as pgdb.ErrNilConnection")
}
