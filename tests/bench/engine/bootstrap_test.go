//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireDB skips only when the whole bench environment is absent; partial or misdirected config fails.
func requireDB(t *testing.T) *benchEnv {
	t.Helper()

	env, err := loadBenchEnv()
	if errors.Is(err, errBenchEnvUnset) {
		t.Skip("set BENCH_PG_PRIMARY_DSN, BENCH_PG_CLIENT_PROXY_DSN, BENCH_PG_STANDBY_DSN and BENCH_TOXIPROXY_URL to run against the bench stack")
	}

	require.NoError(t, err)
	// The lib-commons Migrator refuses this sslmode=disable stack without it; t.Setenv scopes it to this test.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	return env
}

// Two guarded runs, the second over dirtied data, rebuild the same empty schema from the real migrations.
func TestBootstrapSchema(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()

	require.NoError(t, bootstrapSchema(ctx, env), "first bootstrap")
	assertBootstrappedSchema(ctx, t, env)

	_, err := connectPrimary(ctx, t, env).Exec(ctx,
		`INSERT INTO account_control VALUES ('00000000-0000-0000-0000-00000000000a',
		'00000000-0000-0000-0000-00000000000b', '00000000-0000-0000-0000-00000000000c', false, 10, 1)`)
	require.NoError(t, err)

	require.NoError(t, bootstrapSchema(ctx, env), "second bootstrap")
	assertBootstrappedSchema(ctx, t, env)
}

// A target the guard cannot prove is this stack's synchronous primary is refused before any DDL: the probe survives.
func TestBootstrapRejectsWrongTarget(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	// Ownership is proven first, so the probe and marker writes below can only touch this compose stack.
	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	t.Cleanup(func() {
		// A fresh session: cancellation may have closed the test's own. A run killed before this keeps the
		// wrong marker, so every later bootstrap refuses the stack.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		conn, err := pgx.ConnectConfig(cleanupCtx, env.primary)
		if !assert.NoError(t, err, "restore the disposable marker") {
			return
		}
		defer conn.Close(cleanupCtx)

		_, err = conn.Exec(cleanupCtx, setMarker(disposableMarker)+"; DROP TABLE IF EXISTS bench_guard_probe")
		assert.NoError(t, err, "restore the disposable marker")
	})

	_, err = stack.primary.Exec(ctx, `CREATE TABLE bench_guard_probe (id int); INSERT INTO bench_guard_probe VALUES (1)`)
	require.NoError(t, err)

	swapped := *env
	swapped.primary, swapped.standby = env.standby, env.primary

	localCommit := *env
	localCommit.primary = env.primary.Copy()
	localCommit.primary.RuntimeParams["synchronous_commit"] = "local"

	for name, tc := range map[string]struct {
		env          *benchEnv
		marker, want string
	}{
		"unmarked database":           {env, "midaz-bench-engine:kept", "database marker"},
		"primary and standby swapped": {&swapped, disposableMarker, "cluster_name"},
		"local synchronous commit":    {&localCommit, disposableMarker, "synchronous_commit"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := stack.primary.Exec(ctx, setMarker(tc.marker))
			require.NoError(t, err)

			require.ErrorContains(t, bootstrapSchema(ctx, tc.env), tc.want)

			var probes int
			require.NoError(t, stack.primary.QueryRow(ctx, `SELECT count(*) FROM bench_guard_probe`).Scan(&probes))
			assert.Equal(t, 1, probes, "a refused bootstrap must not reset the schema")
		})
	}
}

func setMarker(marker string) string {
	// The next guard reads the marker on the standby, so this test waits for replay, not only flush.
	return "SET LOCAL synchronous_commit = 'remote_apply'; COMMENT ON DATABASE " + benchDatabase + " IS '" + marker + "'"
}

// connectPrimary opens a fresh session: a schema rebuild invalidates statements cached on older ones.
func connectPrimary(ctx context.Context, t *testing.T, env *benchEnv) *pgx.Conn {
	t.Helper()

	conn, err := pgx.ConnectConfig(ctx, env.primary)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close(context.Background()) })

	return conn
}

// assertBootstrappedSchema checks clean migration 46 with its valid partial index, the lib-commons outbox
// enum and four indexes, and empty bench and outbox tables.
func assertBootstrappedSchema(ctx context.Context, t *testing.T, env *benchEnv) {
	t.Helper()

	conn := connectPrimary(ctx, t, env)

	var (
		version                       int64
		dirty, valid, partial, isEnum bool
		rows                          int
		indexes                       []string
	)

	require.NoError(t, conn.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty))
	assert.Equal(t, int64(46), version)
	assert.False(t, dirty)

	require.NoError(t, conn.QueryRow(ctx, `SELECT indisvalid, indpred IS NOT NULL FROM pg_index
		WHERE indexrelid = 'public.idx_operation_balance_version'::regclass`).Scan(&valid, &partial))
	assert.True(t, valid, "migration 46 index must be valid")
	assert.True(t, partial, "migration 46 index must be partial")

	require.NoError(t, conn.QueryRow(ctx,
		`SELECT typtype = 'e' FROM pg_type WHERE oid = 'public.outbox_event_status'::regtype`).Scan(&isEnum))
	assert.True(t, isEnum, "outbox_event_status must be an enum")

	require.NoError(t, conn.QueryRow(ctx, `SELECT array_agg(indexname::text ORDER BY indexname)
		FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'outbox_events'`).Scan(&indexes))
	assert.Equal(t, []string{
		"idx_outbox_events_event_type_status_created_at",
		"idx_outbox_events_status_created_at",
		"idx_outbox_events_status_updated_at",
		"outbox_events_pkey",
	}, indexes)

	// Each count fails on a missing table; zero proves the reset dropped earlier rows.
	require.NoError(t, conn.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM engine_writer_fence) + (SELECT count(*) FROM engine_request) +
		(SELECT count(*) FROM account_control) + (SELECT count(*) FROM outbox_events)`).Scan(&rows))
	assert.Zero(t, rows, "reset must leave no rows behind")
}
