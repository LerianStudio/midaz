// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package organization

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
	return orgRow().
		AddRow(uuid.NewString(), nil, "x", nil, "1", emptyAddressJSON(), "ACTIVE", nil,
			anyTime(), anyTime(), sql.NullTime{}).
		RowError(0, sql.ErrConnDone)
}

func TestOrgRepository_ScanLoopErrors(t *testing.T) {
	t.Parallel()

	t.Run("FindAll_rows_err", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM organization").WillReturnRows(rowsErrRows())

		_, err := r.FindAll(context.Background(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})

	t.Run("ListByIDs_rows_err", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM organization").WillReturnRows(rowsErrRows())

		_, err := r.ListByIDs(context.Background(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("FindAll_scan_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		bad := sqlmock.NewRows([]string{"id"}).AddRow("x")
		mock.ExpectQuery("SELECT .* FROM organization").WillReturnRows(bad)

		_, err := r.FindAll(context.Background(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})
}

func TestOrgRepository_RowsAffectedErrors(t *testing.T) {
	t.Parallel()

	t.Run("Create_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectExec("INSERT INTO organization").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := r.Create(context.Background(), validOrg())
		require.Error(t, err)
	})

	t.Run("Update_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectExec("UPDATE organization SET").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := r.Update(context.Background(), uuid.New(), &mmodel.Organization{LegalName: "x"})
		require.Error(t, err)
	})

	t.Run("Delete_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newRepoWithMock(t)
		mock.ExpectExec("UPDATE organization SET deleted_at").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
		require.Error(t, r.Delete(context.Background(), uuid.New()))
	})
}
