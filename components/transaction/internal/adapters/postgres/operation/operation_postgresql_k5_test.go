// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// K5 extends K2's sqlmock coverage to the point-in-time balance query
// family (FindLastOperationBeforeTimestamp / …ForAccountBeforeTimestamp) and
// the residual error / pagination branches. Reuses K2's harness verbatim.

// opPITCols returns the column list the OperationPointInTimeModel scan
// expects. Matches operationPointInTimeColumns in the production file.
func opPITCols() []string {
	return []string{
		"id", "balance_id", "account_id", "asset_code", "balance_key",
		"available_balance_after", "on_hold_balance_after", "balance_version_after",
		"created_at",
	}
}

func opPITRow() *sqlmock.Rows {
	avail := decimal.NewFromInt(1000)
	onHold := decimal.NewFromInt(50)
	now := time.Now().UTC()

	return sqlmock.NewRows(opPITCols()).
		AddRow(
			uuid.NewString(), uuid.NewString(), uuid.NewString(), "USD", "default",
			&avail, &onHold, int64(7),
			now,
		)
}

func TestOperationRepo_FindLastOperationBeforeTimestamp(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_entity", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(opPITRow())

		got, err := r.FindLastOperationBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_nil_nil", func(t *testing.T) {
		t.Parallel()

		// Contract: "no operation before timestamp" is not an error. Callers
		// at the use-case layer interpret nil as "snapshot starts at zero".
		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(sqlmock.NewRows(opPITCols()))

		got, err := r.FindLastOperationBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC())
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnError(errOpTestBoom)

		_, err := r.FindLastOperationBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC())
		require.Error(t, err)
	})
}

func TestOperationRepo_FindLastOperationsForAccountBeforeTimestamp(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM \\(SELECT DISTINCT").
			WillReturnRows(opPITRow())

		ops, _, err := r.FindLastOperationsForAccountBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC(),
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, ops, 1)
	})

	t.Run("empty_returns_empty_slice", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM \\(SELECT DISTINCT").
			WillReturnRows(sqlmock.NewRows(opPITCols()))

		ops, _, err := r.FindLastOperationsForAccountBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC(),
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Empty(t, ops)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM \\(SELECT DISTINCT").
			WillReturnError(errOpTestBoom)

		_, _, err := r.FindLastOperationsForAccountBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC(),
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})

	t.Run("invalid_cursor_returns_decode_error", func(t *testing.T) {
		t.Parallel()

		r, _ := newOpRepoWithMock(t)
		_, _, err := r.FindLastOperationsForAccountBeforeTimestamp(context.Background(),
			uuid.New(), uuid.New(), uuid.New(), time.Now().UTC(),
			http.Pagination{Limit: 10, SortOrder: "desc", Cursor: "!!!not-base64!!!"})
		require.Error(t, err)
	})
}

// Additional coverage for the pagination cursor-decode guard on the two
// FindAll-style queries that K2 didn't exercise for cursor errors.
func TestOperationRepo_FindAll_InvalidCursor(t *testing.T) {
	t.Parallel()

	r, _ := newOpRepoWithMock(t)
	_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), uuid.New(),
		http.Pagination{Limit: 10, SortOrder: "desc", Cursor: "###bad###"})
	require.Error(t, err)
}

func TestOperationRepo_FindAllByAccount_InvalidCursor(t *testing.T) {
	t.Parallel()

	r, _ := newOpRepoWithMock(t)
	_, _, err := r.FindAllByAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), nil,
		http.Pagination{Limit: 10, SortOrder: "desc", Cursor: "###bad###"})
	require.Error(t, err)
}

// FindAllByAccount with a non-nil operationType filter exercises the extra
// `type = ?` predicate path. Result scanning is the common path and is
// already covered by K2's FindAllByAccount test, but the type-filter branch
// was not.
func TestOperationRepo_FindAllByAccount_TypeFilter(t *testing.T) {
	t.Parallel()

	r, mock := newOpRepoWithMock(t)
	opType := "DEBIT"

	mock.ExpectQuery("SELECT .* FROM public.operation").
		WillReturnRows(opRow(uuid.New(), uuid.New(), uuid.New(), uuid.New()))

	ops, _, err := r.FindAllByAccount(context.Background(),
		uuid.New(), uuid.New(), uuid.New(), &opType,
		http.Pagination{Limit: 10, SortOrder: "desc"})
	require.NoError(t, err)
	require.Len(t, ops, 1)
}

// Create: exercise the pgconn.PgError unique-violation branch. This path
// returns an error (not nil,nil) so we assert error propagation — the adapter
// logs the idempotent-retry event but still surfaces the failure to the
// caller, who decides whether to treat it as success.
func TestOperationRepo_Create_UniqueViolationSurfacesError(t *testing.T) {
	t.Parallel()

	r, mock := newOpRepoWithMock(t)
	pgErr := &pgconn.PgError{Code: constant.UniqueViolationCode}
	mock.ExpectExec("INSERT INTO public.operation").WillReturnError(pgErr)

	_, err := r.Create(context.Background(), opValidInput())
	require.Error(t, err)
}

// CreateBatch: begin tx error path was not covered by K2.
func TestOperationRepo_CreateBatch_BeginTxError(t *testing.T) {
	t.Parallel()

	r, mock := newOpRepoWithMock(t)
	mock.ExpectBegin().WillReturnError(errOpTestBoom)

	err := r.CreateBatch(context.Background(), []*Operation{opValidInput()})
	require.Error(t, err)
}

// CreateBatch: commit error path.
func TestOperationRepo_CreateBatch_CommitError(t *testing.T) {
	t.Parallel()

	r, mock := newOpRepoWithMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public.operation").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errOpTestBoom)

	err := r.CreateBatch(context.Background(), []*Operation{opValidInput()})
	require.Error(t, err)
}

// CreateBatch: nil entries in the slice are skipped — the chunk produces no
// rows, no exec is issued, but BeginTx/Commit still run.
func TestOperationRepo_CreateBatch_NilEntriesSkipped(t *testing.T) {
	t.Parallel()

	r, mock := newOpRepoWithMock(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	err := r.CreateBatch(context.Background(), []*Operation{nil})
	require.NoError(t, err)
}

// CreateBatchWithTx with nil tx documents a pre-existing production bug:
// the method unconditionally wraps the inner error via fmt.Errorf, so even
// the nil-executor early-return path surfaces a non-nil (but nil-wrapped)
// error. This is a characterization test — it locks in the current behaviour
// so any intentional fix is visible in the diff. Flagged for K6 to address.
func TestOperationRepo_CreateBatchWithTx_NilTxReturnsWrappedNilError(t *testing.T) {
	t.Parallel()

	r, _ := newOpRepoWithMock(t)
	err := r.CreateBatchWithTx(context.Background(), nil, []*Operation{opValidInput()})
	// Current prod behaviour: returns a non-nil error wrapping nil. This is
	// a latent bug (CreateBatchWithTx should guard for nil inner err before
	// wrapping) — documenting, not fixing.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to perform database operation")
}

// Update: exercise the body.IsEmpty branch by passing an input with no
// description/status/body — this drives the "no updatable fields" early
// branches inside the Update method. Still executes one UPDATE for
// updated_at, so sqlmock sees a single Exec.
func TestOperationRepo_Update_NoFieldsSetStillIssuesUpdate(t *testing.T) {
	t.Parallel()

	r, mock := newOpRepoWithMock(t)
	mock.ExpectExec("UPDATE public.operation SET").
		WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := r.Update(context.Background(),
		uuid.New(), uuid.New(), uuid.New(), uuid.New(),
		&Operation{}) // empty Operation — only updated_at should change
	require.NoError(t, err)
	require.NotNil(t, got)
}

// assert import anchor — keeps the import graph stable if future refactors
// drop require-only usage.
var _ = assert.True
