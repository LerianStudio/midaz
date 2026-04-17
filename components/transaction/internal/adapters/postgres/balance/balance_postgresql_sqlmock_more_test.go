// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// balListRow builds a row that matches the balanceColumnList order used by
// ListByAccountIDs, ListByIDs, ListByAliases, ListExternalByOrganizationLedger,
// ListByAccountID, ListAllByAccountID, ListAll, and ListByAliasesWithKeys.
func balListRow() *sqlmock.Rows {
	now := time.Now().UTC()

	return sqlmock.NewRows(balRowColsFind()).
		AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), "@acc",
			"USD", decimal.NewFromInt(1000), decimal.NewFromInt(0), int64(1), "deposit",
			true, true, now, now, nil, "default")
}

func TestBalanceRepo_ListByAccountIDs(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, err := r.ListByAccountIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.ListByAccountIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("empty_rows", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").
			WillReturnRows(sqlmock.NewRows(balRowColsFind()))

		got, err := r.ListByAccountIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestBalanceRepo_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("empty_ids_shortcircuit", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestBalanceRepo_ListByAliases(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, err := r.ListByAliases(context.Background(), uuid.New(), uuid.New(), []string{"@a"})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.ListByAliases(context.Background(), uuid.New(), uuid.New(), []string{"@a"})
		require.Error(t, err)
	})
}

func TestBalanceRepo_ListExternalByOrganizationLedger(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, err := r.ListExternalByOrganizationLedger(context.Background(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.ListExternalByOrganizationLedger(context.Background(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestBalanceRepo_ListByAccountID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, err := r.ListByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.ListByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestBalanceRepo_ListByAliasesWithKeys(t *testing.T) {
	t.Parallel()

	t.Run("empty_input_returns_empty", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		got, err := r.ListByAliasesWithKeys(context.Background(), uuid.New(), uuid.New(), nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, err := r.ListByAliasesWithKeys(context.Background(), uuid.New(), uuid.New(), []string{"@a#default"})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("invalid_format_returns_error", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		_, err := r.ListByAliasesWithKeys(context.Background(), uuid.New(), uuid.New(), []string{"@a|default"})
		require.Error(t, err)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.ListByAliasesWithKeys(context.Background(), uuid.New(), uuid.New(), []string{"@a#default"})
		require.Error(t, err)
	})
}

func TestBalanceRepo_Sync(t *testing.T) {
	t.Parallel()

	t.Run("success_updates_row", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("UPDATE public.balance[\\s\\S]*SET available").
			WillReturnResult(sqlmock.NewResult(0, 1))

		ok, err := r.Sync(context.Background(), uuid.New(), uuid.New(), mmodel.BalanceRedis{
			ID:        uuid.NewString(),
			Available: decimal.NewFromInt(10),
			OnHold:    decimal.NewFromInt(0),
			Version:   2,
		})
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("stale_version_returns_false", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("UPDATE public.balance[\\s\\S]*SET available").
			WillReturnResult(sqlmock.NewResult(0, 0))

		ok, err := r.Sync(context.Background(), uuid.New(), uuid.New(), mmodel.BalanceRedis{
			ID:        uuid.NewString(),
			Available: decimal.NewFromInt(10),
			OnHold:    decimal.NewFromInt(0),
			Version:   2,
		})
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("invalid_uuid_returns_parse_error", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		_, err := r.Sync(context.Background(), uuid.New(), uuid.New(), mmodel.BalanceRedis{ID: "not-a-uuid"})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("UPDATE public.balance[\\s\\S]*SET available").WillReturnError(errBalTestBoom)

		_, err := r.Sync(context.Background(), uuid.New(), uuid.New(), mmodel.BalanceRedis{
			ID:        uuid.NewString(),
			Available: decimal.NewFromInt(10),
			OnHold:    decimal.NewFromInt(0),
			Version:   2,
		})
		require.Error(t, err)
	})
}

func TestBalanceRepo_UpdateAllByAccountID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		allow := true

		mock.ExpectExec("UPDATE public.balance[\\s\\S]*SET allow_sending").
			WillReturnResult(sqlmock.NewResult(0, 3))

		err := r.UpdateAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(),
			mmodel.UpdateBalance{AllowSending: &allow, AllowReceiving: &allow})
		require.NoError(t, err)
	})

	t.Run("nil_allow_sending_rejected", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		allow := true
		err := r.UpdateAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(),
			mmodel.UpdateBalance{AllowReceiving: &allow})
		require.Error(t, err)
	})

	t.Run("nil_allow_receiving_rejected", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		allow := true
		err := r.UpdateAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(),
			mmodel.UpdateBalance{AllowSending: &allow})
		require.Error(t, err)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		allow := true

		mock.ExpectExec("UPDATE public.balance[\\s\\S]*SET allow_sending").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := r.UpdateAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(),
			mmodel.UpdateBalance{AllowSending: &allow, AllowReceiving: &allow})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		allow := true

		mock.ExpectExec("UPDATE public.balance[\\s\\S]*SET allow_sending").WillReturnError(errBalTestBoom)

		err := r.UpdateAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(),
			mmodel.UpdateBalance{AllowSending: &allow, AllowReceiving: &allow})
		require.Error(t, err)
	})
}

func TestBalanceRepo_BalancesUpdate(t *testing.T) {
	t.Parallel()

	t.Run("empty_noop", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		err := r.BalancesUpdate(context.Background(), uuid.New(), uuid.New(), nil)
		require.NoError(t, err)
	})

	// We don't exercise the happy path here because the batch update path uses
	// a row-lock pre-step (SELECT ... FOR UPDATE) followed by UPDATE with
	// argument shapes that are brittle to pin without reimplementing the
	// chunking math. The empty-slice short-circuit + normalizer coverage is
	// the useful regression surface.
}

func TestBalanceRepo_ListAll(t *testing.T) {
	t.Parallel()

	filter := http.Pagination{Limit: 10, SortOrder: "DESC"}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, _, err := r.ListAll(context.Background(), uuid.New(), uuid.New(), filter)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("empty_rows", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").
			WillReturnRows(sqlmock.NewRows(balRowColsFind()))

		got, _, err := r.ListAll(context.Background(), uuid.New(), uuid.New(), filter)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, _, err := r.ListAll(context.Background(), uuid.New(), uuid.New(), filter)
		require.Error(t, err)
	})

	t.Run("invalid_cursor", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		bad := http.Pagination{Limit: 10, SortOrder: "DESC", Cursor: "!!!bad!!!"}

		_, _, err := r.ListAll(context.Background(), uuid.New(), uuid.New(), bad)
		require.Error(t, err)
	})
}

func TestBalanceRepo_ListAllByAccountID(t *testing.T) {
	t.Parallel()

	filter := http.Pagination{Limit: 10, SortOrder: "DESC"}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnRows(balListRow())

		got, _, err := r.ListAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(), filter)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, _, err := r.ListAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(), filter)
		require.Error(t, err)
	})

	t.Run("invalid_cursor", func(t *testing.T) {
		t.Parallel()
		r, _ := newBalRepoWithMock(t)
		bad := http.Pagination{Limit: 10, SortOrder: "DESC", Cursor: "!!!bad!!!"}

		_, _, err := r.ListAllByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(), bad)
		require.Error(t, err)
	})
}

func TestBalanceRepo_BalancesUpdateWithTx_NilOrEmpty(t *testing.T) {
	t.Parallel()

	r, _ := newBalRepoWithMock(t)
	// nil tx is accepted as a no-op per implementation contract.
	err := r.BalancesUpdateWithTx(context.Background(), nil, uuid.New(), uuid.New(), []*mmodel.Balance{validBalance()})
	require.NoError(t, err)
}
