// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

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

// rowsErrRows builds a sqlmock rows set that succeeds on Next/Scan for the
// first row but then surfaces an error via rows.Err() in the outer loop.
// We reuse this across every "*ByIDs / List* / FindAll" method so the rows.Err()
// branch of each of those methods gets exercised.
func rowsErrRows() *sqlmock.Rows {
	return accountRow().
		AddRow(
			uuid.NewString(), "a", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		).
		RowError(0, sql.ErrConnDone)
}

func TestAccountRepository_ScanLoopErrors(t *testing.T) {
	t.Parallel()

	t.Run("FindAll_rows_err_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rowsErrRows())

		_, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), nil, http.Pagination{
			Limit: 10, Page: 1, SortOrder: "asc",
		})
		require.Error(t, err)
	})

	t.Run("ListByIDs_rows_err_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rowsErrRows())

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), nil, []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("ListByAlias_rows_err_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rowsErrRows())

		_, err := r.ListByAlias(context.Background(), uuid.New(), uuid.New(), uuid.New(), []string{"x"})
		require.Error(t, err)
	})

	t.Run("ListAccountsByIDs_rows_err_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rowsErrRows())

		_, err := r.ListAccountsByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("ListAccountsByAlias_rows_err_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rowsErrRows())

		_, err := r.ListAccountsByAlias(context.Background(), uuid.New(), uuid.New(), []string{"x"})
		require.Error(t, err)
	})

	t.Run("FindAll_scan_error_inside_loop", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		// Wrong column count triggers the per-row Scan error branch.
		bad := sqlmock.NewRows([]string{"id", "name"}).AddRow("x", "y")
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(bad)

		_, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), nil, http.Pagination{
			Limit: 10, Page: 1, SortOrder: "asc",
		})
		require.Error(t, err)
	})

	t.Run("ListByIDs_scan_error_inside_loop", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		bad := sqlmock.NewRows([]string{"id"}).AddRow("only-col")
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(bad)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), nil, []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("ListByAlias_scan_error_inside_loop", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		bad := sqlmock.NewRows([]string{"id"}).AddRow("only-col")
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(bad)

		_, err := r.ListByAlias(context.Background(), uuid.New(), uuid.New(), uuid.New(), []string{"x"})
		require.Error(t, err)
	})
}

func TestAccountRepository_RowsAffectedErrors(t *testing.T) {
	t.Parallel()

	t.Run("Create_rows_affected_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account").
			WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := r.Create(context.Background(), &mmodel.Account{})
		require.Error(t, err)
	})

	t.Run("Update_rows_affected_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").
			WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(),
			&mmodel.Account{Name: "x"})
		require.Error(t, err)
	})
}

func TestAccountRepository_FindWithDeleted_AdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("not_found_sql_err_no_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(sql.ErrNoRows)

		_, err := r.FindWithDeleted(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})

	t.Run("generic_scan_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(sql.ErrConnDone)

		_, err := r.FindWithDeleted(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})
}
