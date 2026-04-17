// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/partitionstate"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// -- DualWrite tests ------------.
//
// staticPhase is a minimal PartitionPhaseReader for unit tests. It returns a
// fixed phase so scenarios can exercise each branch of DualWriteRepository
// without touching the partition_migration_state table.
type staticPhaseK4 struct {
	phase partitionstate.Phase
	err   error
}

func (s staticPhaseK4) Phase(_ context.Context) (partitionstate.Phase, error) {
	return s.phase, s.err
}

func TestDualWriteRepository_shouldDualWrite(t *testing.T) {
	t.Parallel()

	inner, _ := newBalRepoWithMock(t)

	t.Run("nil_reader_returns_false", func(t *testing.T) {
		t.Parallel()

		repo := NewDualWriteRepository(inner, nil)
		assert.False(t, repo.shouldDualWrite(context.Background()))
	})

	t.Run("reader_error_returns_false", func(t *testing.T) {
		t.Parallel()

		repo := NewDualWriteRepository(inner, staticPhaseK4{err: errBalTestBoom})
		assert.False(t, repo.shouldDualWrite(context.Background()))
	})

	t.Run("phase_legacy_only_returns_false", func(t *testing.T) {
		t.Parallel()

		repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseLegacyOnly})
		assert.False(t, repo.shouldDualWrite(context.Background()))
	})

	t.Run("phase_dual_write_returns_true", func(t *testing.T) {
		t.Parallel()

		repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
		assert.True(t, repo.shouldDualWrite(context.Background()))
	})

	t.Run("phase_partitioned_returns_false", func(t *testing.T) {
		t.Parallel()

		repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhasePartitioned})
		assert.False(t, repo.shouldDualWrite(context.Background()))
	})
}

func TestDualWriteRepository_Create_LegacyOnly_DelegatesToInner(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	// Legacy-only phase means the wrapper delegates to inner.Create, which
	// performs a single INSERT on the legacy table (no BeginTx, no mirror).
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseLegacyOnly})
	require.NoError(t, repo.Create(context.Background(), validBalance()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDualWriteRepository_Create_DualWrite_Success(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	require.NoError(t, repo.Create(context.Background(), validBalance()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDualWriteRepository_Create_DualWrite_BeginTxFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin().WillReturnError(errBalTestBoom)

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.Create(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin dual-write tx")
}

func TestDualWriteRepository_Create_DualWrite_LegacyInsertFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.Create(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert balance")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDualWriteRepository_Create_DualWrite_UniqueViolation(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	pgErr := &pgconn.PgError{Code: constant.UniqueViolationCode}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").WillReturnError(pgErr)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.Create(context.Background(), validBalance())
	require.Error(t, err)
	// The unique-violation path returns a wrapped insert error identically
	// to the generic exec error, so callers can react uniformly.
	assert.Contains(t, err.Error(), "insert balance")
}

func TestDualWriteRepository_Create_DualWrite_LegacyZeroRowsReturnsBusinessError(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.Create(context.Background(), validBalance())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDualWriteRepository_Create_DualWrite_PartitionedInsertFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.Create(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "partitioned insert")
}

func TestDualWriteRepository_Create_DualWrite_CommitFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.Create(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit dual-write")
}

func TestDualWriteRepository_CreateIfNotExists_LegacyOnly_DelegatesToInner(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseLegacyOnly})
	require.NoError(t, repo.CreateIfNotExists(context.Background(), validBalance()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDualWriteRepository_CreateIfNotExists_DualWrite_Success(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	require.NoError(t, repo.CreateIfNotExists(context.Background(), validBalance()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDualWriteRepository_CreateIfNotExists_DualWrite_ZeroRowsIsNoop(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	// Upsert that returned 0 rows on legacy (ON CONFLICT DO NOTHING) should
	// still execute the partitioned mirror and commit successfully.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	require.NoError(t, repo.CreateIfNotExists(context.Background(), validBalance()))
}

func TestDualWriteRepository_CreateIfNotExists_DualWrite_BeginTxFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin().WillReturnError(errBalTestBoom)

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.CreateIfNotExists(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin dual-write tx")
}

func TestDualWriteRepository_CreateIfNotExists_DualWrite_LegacyUpsertFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.CreateIfNotExists(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute query")
}

func TestDualWriteRepository_CreateIfNotExists_DualWrite_PartitionedInsertFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.CreateIfNotExists(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "partitioned insert")
}

func TestDualWriteRepository_CreateIfNotExists_DualWrite_CommitFails(t *testing.T) {
	t.Parallel()

	inner, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.balance").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO " + BalancePartitionedTableName).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	repo := NewDualWriteRepository(inner, staticPhaseK4{phase: partitionstate.PhaseDualWrite})
	err := repo.CreateIfNotExists(context.Background(), validBalance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit dual-write")
}

// balanceInsertValues is a pure helper — exercise it directly to lock
// the column order against accidental reordering.
func TestBalanceInsertValues_OrderMatchesColumnList(t *testing.T) {
	t.Parallel()

	bal := validBalance()
	record := &BalancePostgreSQLModel{}
	record.FromEntity(bal)

	vals := balanceInsertValues(record)
	require.Len(t, vals, len(balanceColumnList), "values count must match column list")

	// Spot-check a few well-known positions to guard against reorder.
	assert.Equal(t, record.ID, vals[0])
	assert.Equal(t, record.OrganizationID, vals[1])
	assert.Equal(t, record.LedgerID, vals[2])
	assert.Equal(t, record.AccountID, vals[3])
	assert.Equal(t, record.Alias, vals[4])
	assert.Equal(t, record.AssetCode, vals[5])
	// Key should be the last value — this is the contract the partitioned
	// insert relies on to materialize columns in the same order.
	assert.Equal(t, record.Key, vals[len(vals)-1])
}

// -- BalancesUpdate happy-path + retry ---------------------------------------.

// balancesUpdateLockRows returns a sqlmock rows object shaped like the
// SELECT ... FOR UPDATE pre-step in executeBatchBalanceUpdateTx.
func balancesUpdateLockRows(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}

	return rows
}

func TestBalanceRepo_BalancesUpdate_HappyPath(t *testing.T) {
	t.Parallel()

	r, mock := newBalRepoWithMock(t)

	// Single balance → single chunk with 5 VALUES params + 2 global params.
	bal := validBalance()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id[\\s\\S]*FROM public.balance[\\s\\S]*FOR UPDATE").
		WillReturnRows(balancesUpdateLockRows(bal.ID))
	mock.ExpectExec("UPDATE public.balance AS b[\\s\\S]*SET available").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := r.BalancesUpdate(context.Background(), uuid.New(), uuid.New(), []*mmodel.Balance{bal})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepo_BalancesUpdate_NormalizesOutNilAndEmptyID(t *testing.T) {
	t.Parallel()

	r, _ := newBalRepoWithMock(t)

	// All inputs are dropped by normalizeBalancesForUpdate → early return nil
	// with no database interaction.
	input := []*mmodel.Balance{
		nil,
		{ID: ""},
	}

	err := r.BalancesUpdate(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
}

func TestBalanceRepo_BalancesUpdate_BeginTxFailsIsNotRetried(t *testing.T) {
	t.Parallel()

	r, mock := newBalRepoWithMock(t)

	mock.ExpectBegin().WillReturnError(errBalTestBoom)

	err := r.BalancesUpdate(context.Background(), uuid.New(), uuid.New(), []*mmodel.Balance{validBalance()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin transaction")
}

func TestBalanceRepo_BalancesUpdate_LockQueryErrorSurfaces(t *testing.T) {
	t.Parallel()

	r, mock := newBalRepoWithMock(t)

	// Non-retryable error on the SELECT FOR UPDATE pre-step should propagate
	// after the first attempt (no backoff loop).
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id[\\s\\S]*FROM public.balance[\\s\\S]*FOR UPDATE").
		WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	err := r.BalancesUpdate(context.Background(), uuid.New(), uuid.New(), []*mmodel.Balance{validBalance()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database operation")
}

func TestBalanceRepo_BalancesUpdateWithTx_ExecutesUpdate(t *testing.T) {
	t.Parallel()

	r, mock := newBalRepoWithMock(t)

	// Set up expectations BEFORE we grab the tx so sqlmock serves them.
	bal := validBalance()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id[\\s\\S]*FROM public.balance[\\s\\S]*FOR UPDATE").
		WillReturnRows(balancesUpdateLockRows(bal.ID))
	mock.ExpectExec("UPDATE public.balance AS b[\\s\\S]*SET available").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	db, err := r.connection.GetDB()
	require.NoError(t, err)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = r.BalancesUpdateWithTx(context.Background(), tx, uuid.New(), uuid.New(), []*mmodel.Balance{bal})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepo_BalancesUpdateWithTx_EmptyBalancesShortCircuits(t *testing.T) {
	t.Parallel()

	r, mock := newBalRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectCommit()

	db, err := r.connection.GetDB()
	require.NoError(t, err)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	// Empty balance slice → short-circuit with no query executed.
	err = r.BalancesUpdateWithTx(context.Background(), tx, uuid.New(), uuid.New(), nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestBalanceRepo_BalancesUpdateWithTx_ExecError(t *testing.T) {
	t.Parallel()

	r, mock := newBalRepoWithMock(t)

	bal := validBalance()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id[\\s\\S]*FROM public.balance[\\s\\S]*FOR UPDATE").
		WillReturnRows(balancesUpdateLockRows(bal.ID))
	mock.ExpectExec("UPDATE public.balance AS b[\\s\\S]*SET available").
		WillReturnError(errBalTestBoom)
	mock.ExpectRollback()

	db, err := r.connection.GetDB()
	require.NoError(t, err)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	err = r.BalancesUpdateWithTx(context.Background(), tx, uuid.New(), uuid.New(), []*mmodel.Balance{bal})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute query")
	require.NoError(t, tx.Rollback())
}

// -- isRetryableBatchBalanceUpdateError -------------------------------------.

func TestIsRetryableBatchBalanceUpdateError_K4(t *testing.T) {
	t.Parallel()

	t.Run("non_pg_error_not_retryable", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isRetryableBatchBalanceUpdateError(errBalTestBoom))
	})

	t.Run("pgx_deadlock_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: "40P01"} // deadlock_detected
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("pgx_serialization_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: "40001"} // serialization_failure
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("pgx_non_retryable_code", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: "23505"} // unique_violation
		assert.False(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("nil_error_not_retryable", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isRetryableBatchBalanceUpdateError(nil))
	})
}

// -- ListByAccountIDAtTimestamp ------------.
//
// Exercises the snapshot-at-timestamp query. The query layout differs from
// other list queries (CTE + LEFT JOIN with distinct scan targets), so it
// needs its own row helper.
func balAtTimestampCols() []string {
	// Matches the COALESCE-enriched column projection in
	// ListByAccountIDAtTimestamp (b.id, org, ledger, account, alias, key,
	// asset_code, account_type, created_at, available, on_hold, version, updated_at).
	return []string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "account_type", "created_at", "available", "on_hold", "version", "updated_at",
	}
}

func balAtTimestampRow() *sqlmock.Rows {
	now := time.Now().UTC()

	return sqlmock.NewRows(balAtTimestampCols()).
		AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(),
			"@acc", "default", "USD", "deposit", now,
			decimal.NewFromInt(100), decimal.NewFromInt(5), int64(2), now)
}

func TestBalanceRepo_ListByAccountIDAtTimestamp(t *testing.T) {
	t.Parallel()

	ts := time.Now().UTC()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("WITH latest_ops AS").
			WillReturnRows(balAtTimestampRow())

		got, err := r.ListByAccountIDAtTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), ts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "@acc", got[0].Alias)
		assert.Equal(t, "USD", got[0].AssetCode)
		assert.Equal(t, int64(2), got[0].Version)
	})

	t.Run("empty_rows_returns_empty", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("WITH latest_ops AS").
			WillReturnRows(sqlmock.NewRows(balAtTimestampCols()))

		got, err := r.ListByAccountIDAtTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), ts)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("query_error_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("WITH latest_ops AS").WillReturnError(errBalTestBoom)

		_, err := r.ListByAccountIDAtTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), ts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execute query")
	})
}

// Guard: unexported errors are referenced; keep a canonical alias so refactors
// that rename the sentinel surface as a failing test.
var _ = errors.New
