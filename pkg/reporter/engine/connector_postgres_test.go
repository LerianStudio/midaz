// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPostgresConnector(t *testing.T) (*postgresConnector, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return &postgresConnector{
		configName: "ledger",
		db:         db,
		schemas:    []string{"public"},
		breaker:    noopBreaker{},
	}, mock
}

func drain(t *testing.T, cursor fetcher.RowCursor) []map[string]any {
	t.Helper()

	var out []map[string]any

	for cursor.Next(context.Background()) {
		_, row := cursor.Row()
		out = append(out, row)
	}

	require.NoError(t, cursor.Err())
	require.NoError(t, cursor.Close(context.Background()))

	return out
}

func TestPostgresCursor_StreamsRowsWithProjection(t *testing.T) {
	t.Parallel()

	c, mock := newPostgresConnector(t)

	// Only the selected (root) columns are projected, quoted and sorted.
	mock.ExpectQuery(`SELECT "amount", "id" FROM "public"\."accounts"`).
		WillReturnRows(sqlmock.NewRows([]string{"amount", "id"}).
			AddRow("100", "a-1").
			AddRow("200", "a-2"))

	req := fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{
			"ledger": {"public.accounts": {"id", "amount"}},
		},
	}

	cursor, err := c.QueryStream(context.Background(), req)
	require.NoError(t, err)

	rows := drain(t, cursor)
	require.Len(t, rows, 2)
	assert.Equal(t, "a-1", rows[0]["id"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresCursor_RollsOverMultipleTables(t *testing.T) {
	t.Parallel()

	c, mock := newPostgresConnector(t)

	// Tables are opened in sorted order: accounts before transactions.
	mock.ExpectQuery(`FROM "public"\."accounts"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a-1"))
	mock.ExpectQuery(`FROM "public"\."transactions"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("t-1").AddRow("t-2"))

	req := fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{
			"ledger": {
				"public.accounts":     {"id"},
				"public.transactions": {"id"},
			},
		},
	}

	cursor, err := c.QueryStream(context.Background(), req)
	require.NoError(t, err)

	var tables []string

	for cursor.Next(context.Background()) {
		table, _ := cursor.Row()
		tables = append(tables, table)
	}

	require.NoError(t, cursor.Err())
	assert.Equal(t, []string{"public.accounts", "public.transactions", "public.transactions"}, tables)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresCursor_EmptySelectionYieldsNoRows(t *testing.T) {
	t.Parallel()

	c, _ := newPostgresConnector(t)

	cursor, err := c.QueryStream(context.Background(), fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{"ledger": {}},
	})
	require.NoError(t, err)

	assert.False(t, cursor.Next(context.Background()))
	assert.NoError(t, cursor.Err())
}

func TestPostgresCursor_ContextCancelStopsStream(t *testing.T) {
	t.Parallel()

	c, mock := newPostgresConnector(t)

	mock.ExpectQuery(`FROM "public"\."accounts"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a-1").AddRow("a-2"))

	cursor, err := c.QueryStream(context.Background(), fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{
			"ledger": {"public.accounts": {"id"}},
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, cursor.Next(ctx))
	require.Error(t, cursor.Err())

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, cursor.Err(), &engineErr)
	assert.Equal(t, fetcher.CategoryCanceled, engineErr.Category)
}

func TestPostgresCursor_RejectsUnsupportedFilters(t *testing.T) {
	t.Parallel()

	c, _ := newPostgresConnector(t)

	// Filters are accepted by the contract but not yet translated; QueryStream
	// must fail closed rather than emit an unfiltered full-table SELECT.
	_, err := c.QueryStream(context.Background(), fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{
			"ledger": {"public.accounts": {"id"}},
		},
		Filters: map[string]any{
			"ledger": map[string]any{"public.accounts": map[string]any{"status": "ACTIVE"}},
		},
	})
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestPostgresConnector_TestConnectionPings(t *testing.T) {
	t.Parallel()

	c, _ := newPostgresConnector(t)

	// Without MonitorPingsOption sqlmock treats PingContext as a no-op success,
	// which is enough to assert the connector routes the ping through its
	// breaker and reports connectivity.
	assert.NoError(t, c.TestConnection(context.Background()))
}

func TestProjectColumns_StarSelectsAll(t *testing.T) {
	t.Parallel()

	assert.Nil(t, projectColumns([]string{"*"}))
}

func TestProjectColumns_NestedCollapsesToRoot(t *testing.T) {
	t.Parallel()

	cols := projectColumns([]string{"fee_charge.totalAmount", "fee_charge.scale", "id"})
	assert.Equal(t, []string{`"fee_charge"`, `"id"`}, cols)
}
