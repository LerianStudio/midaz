// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package engine

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"testing"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	pgkit "github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/postgres"
)

// pgManagerFake routes each tenant ID to its own *sql.DB, mirroring how the
// lib-commons tenant-manager hands back a per-tenant connection. The connector
// code is identical whether the handle came from this fake or the real manager;
// the isolation guarantee under test is the resolver routing each tenant to the
// database it owns.
type pgManagerFake struct {
	dbs map[string]*sql.DB
}

func (m *pgManagerFake) GetDB(_ context.Context, tenantID string) (sqlQuerier, error) {
	db, ok := m.dbs[tenantID]
	if !ok {
		return nil, fmt.Errorf("no connection for tenant %q", tenantID)
	}

	return db, nil
}

// startPostgres boots a Postgres container and returns a DSN base and a teardown.
func startPostgres(t *testing.T) (*pgkit.PostgresInfra, func()) {
	t.Helper()

	ctx := context.Background()

	infra := pgkit.NewPostgresInfra(pgkit.PostgresConfig{
		Name:     "engine",
		Database: "tenant_default",
		Username: "app",
		Password: "app",
	})

	suite, err := itestkit.New(t).WithInfra(infra).Build(ctx)
	require.NoError(t, err)

	return infra, func() { _ = suite.Terminate(ctx) }
}

// hostUpstream returns the host-reachable host:port for the running container.
// HostPort() returns the internal network alias when the suite joins a shared
// network — reachable only container-to-container — so the test process (on the
// host) uses the mapped Upstream address instead.
func hostUpstream(t *testing.T, infra *pgkit.PostgresInfra) string {
	t.Helper()

	endpoint, err := infra.Endpoint()
	require.NoError(t, err)

	return endpoint.Upstream
}

// openTenantDB connects to a named database on the running Postgres container,
// creating it first when create is true.
func openTenantDB(t *testing.T, infra *pgkit.PostgresInfra, dbName string, create bool) *sql.DB {
	t.Helper()

	upstream := hostUpstream(t, infra)

	if create {
		admin := connectDB(t, upstream, "tenant_default")
		_, _ = admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, dbName))
		_ = admin.Close()
	}

	return connectDB(t, upstream, dbName)
}

func connectDB(t *testing.T, upstream, dbName string) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("postgres://app:app@%s/%s?sslmode=disable", upstream, dbName)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	// The testcontainers postgres module signals readiness once, but the server
	// briefly restarts during init-db, so the first connections can be reset.
	// Retry the ping for a few seconds to ride out that window. This is test
	// infrastructure only — the connector code never sees this race.
	deadline := time.Now().Add(30 * time.Second)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr := db.PingContext(ctx)
		cancel()

		if pingErr == nil {
			return db
		}

		if time.Now().After(deadline) {
			require.NoError(t, pingErr)
		}

		time.Sleep(250 * time.Millisecond)
	}
}

func TestIntegration_PostgresConnector_StreamsAndProjects(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()
	db := openTenantDB(t, infra, "tenant_default", false)

	_, err := db.ExecContext(ctx, `CREATE TABLE accounts (id text PRIMARY KEY, balance int, secret text)`)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, balance, secret) VALUES ($1, $2, $3)`,
			fmt.Sprintf("acc-%d", i), i*100, "do-not-extract")
		require.NoError(t, err)
	}

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: map[string]*sql.DB{"tenant-default": db}}, nil, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "ledger",
		Type:       DatasourceTypePostgres,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	require.NoError(t, connector.TestConnection(ctx))

	cursor, err := connector.QueryStream(ctx, fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{
			"ledger": {"public.accounts": {"id", "balance"}},
		},
	})
	require.NoError(t, err)

	count := 0

	for cursor.Next(ctx) {
		table, row := cursor.Row()
		assert.Equal(t, "public.accounts", table)
		// Projection must exclude the unselected "secret" column.
		_, hasSecret := row["secret"]
		assert.False(t, hasSecret, "secret column must not be projected")
		assert.Contains(t, row, "id")
		assert.Contains(t, row, "balance")

		count++
	}

	require.NoError(t, cursor.Err())
	require.NoError(t, cursor.Close(ctx))
	assert.Equal(t, 5, count)
}

// streamPostgresFiltered builds a connector against the seeded tenant DB and
// streams the accounts table with the supplied filters, returning the collected
// rows (filter translation runs against a real Postgres engine).
func streamPostgresFiltered(t *testing.T, db *sql.DB, fields []string, filters datasourceFilters) ([]map[string]any, error) {
	t.Helper()

	ctx := context.Background()

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: map[string]*sql.DB{"tenant-default": db}}, nil, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "ledger", Type: DatasourceTypePostgres,
	}, "tenant-default"))
	require.NoError(t, err)

	defer func() { _ = connector.Close(ctx) }()

	cursor, err := connector.QueryStream(ctx, fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{"ledger": {"public.accounts": fields}},
		Filters:      map[string]any{"ledger": filters},
	})
	if err != nil {
		return nil, err
	}

	defer func() { _ = cursor.Close(ctx) }()

	var rows []map[string]any

	for cursor.Next(ctx) {
		_, row := cursor.Row()
		rows = append(rows, row)
	}

	return rows, cursor.Err()
}

func seedFilterAccounts(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()

	_, err := db.ExecContext(ctx, `CREATE TABLE accounts (id text PRIMARY KEY, status text, balance int, created_at date)`)
	require.NoError(t, err)

	seed := []struct {
		id      string
		status  string
		balance int
		created string
	}{
		{"acc-1", "ACTIVE", 100, "2025-06-01"},
		{"acc-2", "PENDING", 500, "2025-06-15"},
		{"acc-3", "INACTIVE", 1000, "2025-06-30"},
		{"acc-4", "ACTIVE", 2000, "2025-07-10"},
	}
	for _, s := range seed {
		_, err = db.ExecContext(ctx,
			`INSERT INTO accounts (id, status, balance, created_at) VALUES ($1, $2, $3, $4)`,
			s.id, s.status, s.balance, s.created)
		require.NoError(t, err)
	}
}

func TestIntegration_PostgresConnector_FilterEqualitySingleAndMulti(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	db := openTenantDB(t, infra, "tenant_default", false)
	seedFilterAccounts(t, db)

	rows, err := streamPostgresFiltered(t, db, []string{"id", "status"},
		datasourceFilters{"public.accounts": {"status": {Equals: []any{"ACTIVE"}}}})
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	rows, err = streamPostgresFiltered(t, db, []string{"id", "status"},
		datasourceFilters{"public.accounts": {"status": {Equals: []any{"ACTIVE", "PENDING"}}}})
	require.NoError(t, err)
	assert.Len(t, rows, 3)
}

func TestIntegration_PostgresConnector_FilterRange(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	db := openTenantDB(t, infra, "tenant_default", false)
	seedFilterAccounts(t, db)

	rows, err := streamPostgresFiltered(t, db, []string{"id", "balance"},
		datasourceFilters{"public.accounts": {"balance": {GreaterThan: []any{100}, LessOrEqual: []any{1000}}}})
	require.NoError(t, err)
	assert.Len(t, rows, 2) // balance 500 and 1000
}

func TestIntegration_PostgresConnector_FilterBetweenDateExpansion(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	db := openTenantDB(t, infra, "tenant_default", false)
	seedFilterAccounts(t, db)

	// Between [2025-06-01, 2025-06-30] must include the 2025-06-30 row: the
	// date-only upper bound is expanded to end-of-day.
	rows, err := streamPostgresFiltered(t, db, []string{"id", "created_at"},
		datasourceFilters{"public.accounts": {"created_at": {Between: []any{"2025-06-01", "2025-06-30"}}}})
	require.NoError(t, err)
	assert.Len(t, rows, 3) // 06-01, 06-15, 06-30 (07-10 excluded)
}

func TestIntegration_PostgresConnector_FilterUnknownFieldErrors(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	db := openTenantDB(t, infra, "tenant_default", false)
	seedFilterAccounts(t, db)

	_, err := streamPostgresFiltered(t, db, []string{"id"},
		datasourceFilters{"public.accounts": {"ghost": {Equals: []any{"x"}}}})
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestIntegration_PostgresConnector_DiscoverSchema(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()
	db := openTenantDB(t, infra, "tenant_default", false)

	_, err := db.ExecContext(ctx, `CREATE TABLE accounts (id text PRIMARY KEY, balance int)`)
	require.NoError(t, err)

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: map[string]*sql.DB{"tenant-default": db}}, nil, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "ledger", Type: DatasourceTypePostgres,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	snapshot, err := connector.DiscoverSchema(ctx)
	require.NoError(t, err)

	assert.Equal(t, "ledger", snapshot.ConfigName)
	require.True(t, snapshot.HasTable("public.accounts"))

	for _, table := range snapshot.Tables {
		if table.Name == "public.accounts" {
			assert.ElementsMatch(t, []string{"id", "balance"}, table.Fields)
		}
	}
}

func TestIntegration_PostgresConnector_TenantIsolation(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()

	dbA := openTenantDB(t, infra, "tenant_default", false)
	dbB := openTenantDB(t, infra, "tenant_b", true)

	for _, setup := range []struct {
		db    *sql.DB
		owner string
	}{{dbA, "tenant-a"}, {dbB, "tenant-b"}} {
		_, err := setup.db.ExecContext(ctx, `CREATE TABLE accounts (id text PRIMARY KEY, owner text)`)
		require.NoError(t, err)
		_, err = setup.db.ExecContext(ctx, `INSERT INTO accounts (id, owner) VALUES ($1, $2)`, "row-1", setup.owner)
		require.NoError(t, err)
	}

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: map[string]*sql.DB{
		"tenant-a": dbA,
		"tenant-b": dbB,
	}}, nil, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	read := func(tenant string) string {
		connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
			ConfigName: "ledger", Type: DatasourceTypePostgres,
		}, tenant))
		require.NoError(t, err)
		defer func() { _ = connector.Close(ctx) }()

		cursor, err := connector.QueryStream(ctx, fetcher.ExtractionRequest{
			MappedFields: map[string]fetcher.FieldSelection{"ledger": {"public.accounts": {"id", "owner"}}},
		})
		require.NoError(t, err)
		defer func() { _ = cursor.Close(ctx) }()

		require.True(t, cursor.Next(ctx))
		_, row := cursor.Row()
		require.NoError(t, cursor.Err())

		owner, _ := row["owner"].(string)

		return owner
	}

	// Each tenant reads ONLY its own database — no cross-read.
	assert.Equal(t, "tenant-a", read("tenant-a"))
	assert.Equal(t, "tenant-b", read("tenant-b"))
}

func TestIntegration_PostgresConnector_ContextCancelMidStream(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()
	db := openTenantDB(t, infra, "tenant_default", false)

	_, err := db.ExecContext(ctx, `CREATE TABLE events (id serial PRIMARY KEY, payload text)`)
	require.NoError(t, err)

	for i := 0; i < 500; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO events (payload) VALUES ($1)`, "x")
		require.NoError(t, err)
	}

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: map[string]*sql.DB{"tenant-default": db}}, nil, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "ledger", Type: DatasourceTypePostgres,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	streamCtx, cancel := context.WithCancel(ctx)

	cursor, err := connector.QueryStream(streamCtx, fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{"ledger": {"public.events": {"id", "payload"}}},
	})
	require.NoError(t, err)
	defer func() { _ = cursor.Close(ctx) }()

	require.True(t, cursor.Next(streamCtx))

	cancel()

	// Stream stops promptly after cancellation and surfaces a canceled error.
	for cursor.Next(streamCtx) {
		// drain until it observes the cancellation
	}

	require.Error(t, cursor.Err())

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, cursor.Err(), &engineErr)
	assert.Equal(t, fetcher.CategoryCanceled, engineErr.Category)
}

func TestIntegration_PostgresConnector_LargeResultStreamsBounded(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()
	db := openTenantDB(t, infra, "tenant_default", false)

	_, err := db.ExecContext(ctx, `CREATE TABLE big (id serial PRIMARY KEY, payload text)`)
	require.NoError(t, err)

	const rowCount = 50000

	const payload = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // 64 bytes

	_, err = db.ExecContext(ctx,
		`INSERT INTO big (payload) SELECT $1 FROM generate_series(1, $2)`, payload, rowCount)
	require.NoError(t, err)

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: map[string]*sql.DB{"tenant-default": db}}, nil, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "ledger", Type: DatasourceTypePostgres,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	runtime.GC()

	var before runtime.MemStats

	runtime.ReadMemStats(&before)

	cursor, err := connector.QueryStream(ctx, fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{"ledger": {"public.big": {"id", "payload"}}},
	})
	require.NoError(t, err)

	var (
		count    int
		peakHeap uint64
	)

	for cursor.Next(ctx) {
		_, row := cursor.Row()
		require.Contains(t, row, "payload")

		count++

		if count%10000 == 0 {
			var m runtime.MemStats

			runtime.ReadMemStats(&m)

			if m.HeapInuse > peakHeap {
				peakHeap = m.HeapInuse
			}
		}
	}

	require.NoError(t, cursor.Err())
	require.NoError(t, cursor.Close(ctx))
	assert.Equal(t, rowCount, count)

	// The full materialized result would be ~rowCount*payload bytes (~3.2MB of
	// payload strings alone, plus per-row map overhead). Streaming holds only one
	// row at a time, so the heap growth during the drain stays far below the
	// materialized footprint. We assert peak in-use heap growth is a small
	// fraction of the full result, proving the cursor does not load-all.
	heapGrowth := int64(peakHeap) - int64(before.HeapInuse)
	fullResultBytes := int64(rowCount) * int64(len(payload))

	t.Logf("rows=%d heapGrowth=%d bytes fullResultLowerBound=%d bytes", count, heapGrowth, fullResultBytes)
	assert.Less(t, heapGrowth, fullResultBytes,
		"streaming heap growth must stay below the full materialized result size")
}
