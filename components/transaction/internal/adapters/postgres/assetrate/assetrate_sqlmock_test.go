// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package assetrate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

var errARTestBoom = errors.New("asset-rate boom")

func newARRepoWithMock(t *testing.T) (*AssetRatePostgreSQLRepository, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	resolver := dbresolver.New(
		dbresolver.WithPrimaryDBs(db),
		dbresolver.WithReplicaDBs(db),
		dbresolver.WithLoadBalancer(dbresolver.RoundRobinLB),
	)

	conn := &libPostgres.PostgresConnection{
		ConnectionDB: &resolver,
		Connected:    true,
	}

	return &AssetRatePostgreSQLRepository{
		connection: conn,
		tableName:  "asset_rate",
	}, mock
}

func arCols() []string {
	return []string{
		"id", "organization_id", "ledger_id", "external_id",
		"from", "to", "rate", "rate_scale", "source", "ttl",
		"created_at", "updated_at",
	}
}

func arRow() *sqlmock.Rows {
	now := time.Now().UTC()
	src := "test"

	return sqlmock.NewRows(arCols()).
		AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(),
			"USD", "BRL", 5.25, 2.0, &src, int64(3600), now, now)
}

func validAssetRate() *AssetRate {
	src := "Central Bank"
	scale := 2.0

	return &AssetRate{
		ID:             uuid.NewString(),
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		ExternalID:     uuid.NewString(),
		From:           "USD",
		To:             "BRL",
		Rate:           5.25,
		Scale:          &scale,
		Source:         &src,
		TTL:            3600,
	}
}

func TestAssetRateRepo_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectExec("INSERT INTO asset_rate").WillReturnResult(sqlmock.NewResult(1, 1))

		got, err := r.Create(context.Background(), validAssetRate())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_business_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectExec("INSERT INTO asset_rate").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), validAssetRate())
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectExec("INSERT INTO asset_rate").WillReturnError(errARTestBoom)

		_, err := r.Create(context.Background(), validAssetRate())
		require.Error(t, err)
	})
}

func TestAssetRateRepo_FindByExternalID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(arRow())

		got, err := r.FindByExternalID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_business_not_found", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(sqlmock.NewRows(arCols()))

		_, err := r.FindByExternalID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnError(errARTestBoom)

		_, err := r.FindByExternalID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestAssetRateRepo_FindByCurrencyPair(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(arRow())

		got, err := r.FindByCurrencyPair(context.Background(), uuid.New(), uuid.New(), "USD", "BRL")
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_nil_nil", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(sqlmock.NewRows(arCols()))

		got, err := r.FindByCurrencyPair(context.Background(), uuid.New(), uuid.New(), "USD", "BRL")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnError(errARTestBoom)

		_, err := r.FindByCurrencyPair(context.Background(), uuid.New(), uuid.New(), "USD", "BRL")
		require.Error(t, err)
	})
}

func TestAssetRateRepo_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectExec("UPDATE asset_rate SET").WillReturnResult(sqlmock.NewResult(0, 1))

		got, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), validAssetRate())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_business_not_found", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectExec("UPDATE asset_rate SET").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), validAssetRate())
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectExec("UPDATE asset_rate SET").WillReturnError(errARTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), validAssetRate())
		require.Error(t, err)
	})
}

func TestAssetRateRepo_FindAllByAssetCodes(t *testing.T) {
	t.Parallel()

	filter := http.Pagination{Limit: 10, SortOrder: "DESC"}

	t.Run("success_with_to_codes", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(arRow())

		got, _, err := r.FindAllByAssetCodes(context.Background(), uuid.New(), uuid.New(), "USD", []string{"BRL"}, filter)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("success_nil_to_codes", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(arRow())

		got, _, err := r.FindAllByAssetCodes(context.Background(), uuid.New(), uuid.New(), "USD", nil, filter)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnError(errARTestBoom)

		_, _, err := r.FindAllByAssetCodes(context.Background(), uuid.New(), uuid.New(), "USD", []string{"BRL"}, filter)
		require.Error(t, err)
	})

	t.Run("empty_rows", func(t *testing.T) {
		t.Parallel()
		r, mock := newARRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM asset_rate").WillReturnRows(sqlmock.NewRows(arCols()))

		got, _, err := r.FindAllByAssetCodes(context.Background(), uuid.New(), uuid.New(), "USD", []string{"BRL"}, filter)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("invalid_cursor_returns_error", func(t *testing.T) {
		t.Parallel()
		r, _ := newARRepoWithMock(t)
		badFilter := http.Pagination{Limit: 10, SortOrder: "DESC", Cursor: "!!!not-valid!!!"}

		_, _, err := r.FindAllByAssetCodes(context.Background(), uuid.New(), uuid.New(), "USD", []string{"BRL"}, badFilter)
		require.Error(t, err)
	})
}
