// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

func rowsErrRows() *sqlmock.Rows {
	return ledgerRow().
		AddRow(uuid.NewString(), "l", uuid.NewString(), "ACTIVE", nil, anyTime(), anyTime(), sql.NullTime{}).
		RowError(0, sql.ErrConnDone)
}

func TestLedgerRepository_ScanLoopErrors(t *testing.T) {
	t.Parallel()

	t.Run("FindAll_rows_err", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM ledger").WillReturnRows(rowsErrRows())

		_, err := r.FindAll(context.Background(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})

	t.Run("ListByIDs_rows_err", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM ledger").WillReturnRows(rowsErrRows())

		_, err := r.ListByIDs(context.Background(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("FindAll_scan_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		bad := sqlmock.NewRows([]string{"id"}).AddRow("x")
		mock.ExpectQuery("SELECT .* FROM ledger").WillReturnRows(bad)

		_, err := r.FindAll(context.Background(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})

	t.Run("ListByIDs_scan_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		bad := sqlmock.NewRows([]string{"id"}).AddRow("x")
		mock.ExpectQuery("SELECT .* FROM ledger").WillReturnRows(bad)

		_, err := r.ListByIDs(context.Background(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestLedgerRepository_RowsAffectedErrors(t *testing.T) {
	t.Parallel()

	t.Run("Create_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectExec("INSERT INTO ledger").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := r.Create(context.Background(), &mmodel.Ledger{})
		require.Error(t, err)
	})

	t.Run("Update_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectExec("UPDATE ledger SET").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), &mmodel.Ledger{Name: "x"})
		require.Error(t, err)
	})

	t.Run("Delete_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectExec("UPDATE ledger SET deleted_at").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New()))
	})
}
