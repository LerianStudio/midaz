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

	pgdb "tracer/internal/adapters/postgres/db"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// setupRuleRepoWithSQLMock returns a bare *Repository (the conn field is left
// nil because *WithTx methods bypass Connection.GetDB and use the caller-
// provided pgdb.DB directly), a *sql.DB that satisfies pgdb.DB, the sqlmock
// for query expectations, and a cleanup function.
func setupRuleRepoWithSQLMock(t *testing.T) (*Repository, *sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	repo := &Repository{}

	cleanup := func() {
		sqlMock.ExpectClose()
		require.NoError(t, db.Close())
	}

	return repo, db, sqlMock, cleanup
}

// ============================================================================
// CreateWithTx
// ============================================================================

func TestRepository_CreateWithTx(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		mockSetup func(mock sqlmock.Sqlmock, rule *model.Rule)
		wantErr   bool
		errIs     error
		errMsg    string
	}{
		{
			name: "Success - inserts rule using supplied db handle",
			mockSetup: func(mock sqlmock.Sqlmock, rule *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rules`)).
					WithArgs(
						rule.ID,
						rule.Name,
						rule.Description,
						rule.Expression,
						rule.Action,
						sqlmock.AnyArg(), // scopesJSON
						rule.Status,
						sqlmock.AnyArg(), // context_id
						rule.CreatedAt,
						rule.UpdatedAt,
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "Error - database insert fails",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rules`)).
					WillReturnError(errors.New("connection lost"))
			},
			wantErr: true,
			errMsg:  "failed to insert rule",
		},
		{
			name: "Error - unique name violation maps to ErrRuleNameAlreadyExistsInCtx",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rules`)).
					WillReturnError(&pgconn.PgError{
						Code:           "23505",
						ConstraintName: "idx_rules_name_per_context_active",
						Message:        "duplicate key value violates unique constraint",
					})
			},
			wantErr: true,
			errIs:   constant.ErrRuleNameAlreadyExistsInCtx,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupRuleRepoWithSQLMock(t)
			defer cleanup()

			rule := testRule()
			tt.mockSetup(sqlMock, rule)

			result, err := repo.CreateWithTx(context.Background(), db, rule)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)

				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}

				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, rule.ID, result.ID)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

// TestRepository_CreateWithTx_NilDB verifies that CreateWithTx rejects a nil
// db handle at the public boundary with pgdb.ErrNilConnection instead of
// silently resolving a non-transactional connection via r.conn.GetDB. This
// preserves the atomicity contract advertised by *WithTx methods: callers
// MUST provide a transaction.
func TestRepository_CreateWithTx_NilDB(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo := &Repository{}
	rule := testRule()

	result, err := repo.CreateWithTx(context.Background(), nil, rule)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil db must surface as pgdb.ErrNilConnection")
}

// ============================================================================
// UpdateWithTx
// ============================================================================

func TestRepository_UpdateWithTx(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		mockSetup func(mock sqlmock.Sqlmock, rule *model.Rule)
		wantErr   bool
		errIs     error
		errMsg    string
	}{
		{
			name: "Success - updates rule using supplied db handle",
			mockSetup: func(mock sqlmock.Sqlmock, rule *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						rule.Name,
						rule.Description,
						rule.Expression,
						rule.Action,
						sqlmock.AnyArg(), // scopesJSON
						rule.Status,
						sqlmock.AnyArg(), // context_id
						rule.UpdatedAt,
						rule.ID,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name: "Error - rule not found (zero rows)",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errIs:   constant.ErrRuleNotFound,
		},
		{
			name: "Error - database update fails",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnError(errors.New("constraint violation"))
			},
			wantErr: true,
			errMsg:  "failed to update rule",
		},
		{
			name: "Error - unique name violation maps to ErrRuleNameAlreadyExistsInCtx",
			mockSetup: func(mock sqlmock.Sqlmock, _ *model.Rule) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnError(&pgconn.PgError{
						Code:           "23505",
						ConstraintName: "idx_rules_name_per_context_active",
						Message:        "duplicate key value violates unique constraint",
					})
			},
			wantErr: true,
			errIs:   constant.ErrRuleNameAlreadyExistsInCtx,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupRuleRepoWithSQLMock(t)
			defer cleanup()

			rule := testRule()
			tt.mockSetup(sqlMock, rule)

			err := repo.UpdateWithTx(context.Background(), db, rule)

			if tt.wantErr {
				require.Error(t, err)

				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
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

// TestRepository_UpdateWithTx_NilDB verifies that UpdateWithTx rejects a nil
// db handle at the public boundary with pgdb.ErrNilConnection instead of
// silently resolving a non-transactional connection via r.conn.GetDB. This
// preserves the atomicity contract advertised by *WithTx methods: callers
// MUST provide a transaction.
func TestRepository_UpdateWithTx_NilDB(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo := &Repository{}
	rule := testRule()

	err := repo.UpdateWithTx(context.Background(), nil, rule)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil db must surface as pgdb.ErrNilConnection")
}

// ============================================================================
// DeleteWithTx
// ============================================================================

func TestRepository_DeleteWithTx(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		ruleID    uuid.UUID
		mockSetup func(mock sqlmock.Sqlmock, id uuid.UUID)
		wantErr   bool
		errIs     error
		errMsg    string
	}{
		{
			name:   "Success - soft deletes rule using supplied db handle",
			ruleID: testutil.MustDeterministicUUID(400),
			mockSetup: func(mock sqlmock.Sqlmock, id uuid.UUID) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WithArgs(
						model.RuleStatusDeleted,
						sqlmock.AnyArg(), // deleted_at
						sqlmock.AnyArg(), // updated_at
						id,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:   "Error - rule not found (zero rows)",
			ruleID: testutil.MustDeterministicUUID(401),
			mockSetup: func(mock sqlmock.Sqlmock, _ uuid.UUID) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
			errIs:   constant.ErrRuleNotFound,
		},
		{
			name:   "Error - database delete fails",
			ruleID: testutil.MustDeterministicUUID(402),
			mockSetup: func(mock sqlmock.Sqlmock, _ uuid.UUID) {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE rules`)).
					WillReturnError(errors.New("foreign key violation"))
			},
			wantErr: true,
			errMsg:  "failed to delete rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupRuleRepoWithSQLMock(t)
			defer cleanup()

			tt.mockSetup(sqlMock, tt.ruleID)

			err := repo.DeleteWithTx(context.Background(), db, tt.ruleID)

			if tt.wantErr {
				require.Error(t, err)

				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
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

// TestRepository_DeleteWithTx_NilDB verifies that DeleteWithTx rejects a nil
// db handle at the public boundary with pgdb.ErrNilConnection instead of
// silently resolving a non-transactional connection via r.conn.GetDB. This
// preserves the atomicity contract advertised by *WithTx methods: callers
// MUST provide a transaction.
func TestRepository_DeleteWithTx_NilDB(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	repo := &Repository{}

	err := repo.DeleteWithTx(context.Background(), nil, testutil.MustDeterministicUUID(999))

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil db must surface as pgdb.ErrNilConnection")
}
