// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package segment

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
	return segmentRow().
		AddRow(uuid.NewString(), "x", uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
			anyTime(), anyTime(), sql.NullTime{}).
		RowError(0, sql.ErrConnDone)
}

func TestSegmentRepository_ScanLoopErrors(t *testing.T) {
	t.Parallel()

	t.Run("FindAll_rows_err", func(t *testing.T) {
		t.Parallel()
		p, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(rowsErrRows())

		_, err := p.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})

	t.Run("FindByIDs_rows_err", func(t *testing.T) {
		t.Parallel()
		p, mock := newRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(rowsErrRows())

		_, err := p.FindByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("FindAll_scan_error", func(t *testing.T) {
		t.Parallel()
		p, mock := newRepoWithMock(t)
		bad := sqlmock.NewRows([]string{"id"}).AddRow("x")
		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(bad)

		_, err := p.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})
}

func TestSegmentRepository_RowsAffectedErrors(t *testing.T) {
	t.Parallel()

	t.Run("Create_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		p, mock := newRepoWithMock(t)
		mock.ExpectExec("INSERT INTO segment").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := p.Create(context.Background(), &mmodel.Segment{})
		require.Error(t, err)
	})

	t.Run("Update_rows_affected_error", func(t *testing.T) {
		t.Parallel()
		p, mock := newRepoWithMock(t)
		mock.ExpectExec("UPDATE segment SET").WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

		_, err := p.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Segment{Name: "x"})
		require.Error(t, err)
	})
}
