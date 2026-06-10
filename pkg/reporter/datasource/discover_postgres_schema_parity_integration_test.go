// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	pgkit "github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/postgres"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"
)

// TestDiscoverPostgresSchema_EnvPoolParity is the D8 drift-lock: the
// hand-rolled multi-tenant introspection in discoverPostgresSchema and the
// env-pool repository's postgres.GetDatabaseSchema are two independent
// information_schema walks that MUST produce the same TableSchema set against
// the same database. The fixture deliberately includes a VIEW alongside base
// tables so the test locks the `table_type = 'BASE TABLE'` filter: if either
// path stops excluding views (or starts), this test fails. A divergence here is
// the failure mode where a template passes under multi-tenancy but fails
// single-tenant (or vice versa) against the same schema.
//
// This is the lean interim guard. The recommended follow-up — collapsing both
// onto a single FromDB introspection constructor — is out of scope here.
func TestDiscoverPostgresSchema_EnvPoolParity(t *testing.T) {
	ctx := context.Background()

	infra := pgkit.NewPostgresInfra(pgkit.PostgresConfig{
		Name:     "schema-parity",
		Database: "parity_db",
		Username: "app",
		Password: "app",
	})

	suite, err := itestkit.New(t).WithInfra(infra).Build(ctx)
	require.NoError(t, err)

	defer func() { _ = suite.Terminate(ctx) }()

	endpoint, err := infra.Endpoint()
	require.NoError(t, err)

	db := openParityDB(t, endpoint.Upstream, "parity_db")
	defer func() { _ = db.Close() }()

	// Fixture: two base tables and one VIEW over them. The view must be excluded
	// by BOTH introspection paths.
	mustExec(t, db, `CREATE TABLE accounts (id uuid PRIMARY KEY, name text NOT NULL, balance numeric)`)
	mustExec(t, db, `CREATE TABLE ledgers (id uuid PRIMARY KEY, account_id uuid, created_at timestamptz)`)
	mustExec(t, db, `CREATE VIEW account_balances AS SELECT id, balance FROM accounts`)

	schemas := []string{"public"}

	// --- Path A: multi-tenant in-process introspection (discoverPostgresSchema).
	resolverDB := dbresolver.New(dbresolver.WithPrimaryDBs(db))

	mtSchema, err := discoverPostgresSchema(ctx, resolverDB, schemas)
	require.NoError(t, err, "discoverPostgresSchema must succeed")

	// --- Path B: env-pool repository (postgres.GetDatabaseSchema).
	repo, err := postgres.NewDataSourceRepository(&postgres.Connection{
		DBName:       "parity_db",
		ConnectionDB: db,
		Connected:    true,
		Logger:       &log.NopLogger{},
	})
	require.NoError(t, err)

	envSchema, err := repo.GetDatabaseSchema(ctx, schemas)
	require.NoError(t, err, "env-pool GetDatabaseSchema must succeed")

	// The view must NOT appear in either set.
	mtSet := normalizeSchemaSet(mtSchema)
	envSet := normalizeSchemaSet(envSchema)

	assert.NotContains(t, mtSet, "public.account_balances",
		"discoverPostgresSchema must exclude views (BASE TABLE filter)")
	assert.NotContains(t, envSet, "public.account_balances",
		"GetDatabaseSchema must exclude views (BASE TABLE filter)")

	// The load-bearing parity assertion: identical table+column sets.
	assert.Equal(t, envSet, mtSet,
		"multi-tenant and env-pool introspection must produce the same TableSchema set")
}

// normalizeSchemaSet reduces a []postgres.TableSchema to a comparable map of
// "schema.table" → sorted column-name slice, dropping ordering and the
// primary-key flag (which discoverPostgresSchema intentionally omits).
func normalizeSchemaSet(in []postgres.TableSchema) map[string][]string {
	out := make(map[string][]string, len(in))

	for _, ts := range in {
		cols := make([]string, 0, len(ts.Columns))
		for _, c := range ts.Columns {
			cols = append(cols, c.Name)
		}

		sort.Strings(cols)
		out[ts.QualifiedName()] = cols
	}

	return out
}

func openParityDB(t *testing.T, upstream, dbName string) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("postgres://app:app@%s/%s?sslmode=disable", upstream, dbName)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	// Ride out the brief init-db restart window the testcontainers postgres
	// module leaves open after first readiness.
	deadline := time.Now().Add(30 * time.Second)

	for {
		pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr := db.PingContext(pingCtx)

		cancel()

		if pingErr == nil {
			break
		}

		if time.Now().After(deadline) {
			require.NoError(t, pingErr, "postgres never became reachable")
		}

		time.Sleep(250 * time.Millisecond)
	}

	return db
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()

	_, err := db.ExecContext(context.Background(), query)
	require.NoError(t, err, "fixture exec failed: %s", query)
}
