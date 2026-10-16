//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePsqlTimings(t *testing.T) {
	// Captured from psql 17.11 under LC_ALL=C through pg-repl, plus one sample over 1 s.
	samples, err := parsePsqlTimings("1\nTime: 0.109 ms\n1\nTime: 0.081 ms\n1\nTime: 1234.567 ms (00:01.235)\n", 3)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{109 * time.Microsecond, 81 * time.Microsecond, 1234567 * time.Microsecond}, samples)

	for name, out := range map[string]string{
		"incomplete":    "1\nTime: 0.109 ms\n1\nTime: 0.081 ms\n1\n",
		"decimal comma": "1\nTime: 0.109 ms\n1\nTime: 0,081 ms\n1\nTime: 0.067 ms\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parsePsqlTimings(out, 3)
			assert.Error(t, err)
		})
	}
}

// The probe crosses the configured delay on its own connection; adding or removing the toxic does not reconnect standby1.
func TestReplLatency(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()

	stack, err := openVerifiedStack(ctx, env)
	require.NoError(t, err)

	defer stack.close(ctx)

	sender := replSender(ctx, t, stack.primary)

	t.Cleanup(func() {
		assert.NoError(t, setReplLatency(env, 0), "pg-repl cleanup; after an error its toxics are unknown")
	})

	require.NoError(t, setReplLatency(env, 20))

	samples, err := probeReplRoundtrip(ctx)
	require.NoError(t, err)

	sorted := slices.Sorted(slices.Values(samples))
	t.Logf("sql_roundtrip_pg_repl, 20 ms downstream: n=%d min=%s median(lower)=%s max=%s samples=%v",
		len(sorted), sorted[0], sorted[(len(sorted)-1)/2], sorted[len(sorted)-1], samples)
	assert.GreaterOrEqual(t, sorted[0], 20*time.Millisecond, "every reply crosses the downstream delay")
	assert.Equal(t, sender, replSender(ctx, t, stack.primary), "adding latency reconnected replication")

	require.NoError(t, setReplLatency(env, 0))
	require.NoError(t, stack.awaitStandby(ctx))
	assert.Equal(t, sender, replSender(ctx, t, stack.primary), "removing latency reconnected replication")
}

// replSender identifies standby1's walsender; a reconnect starts a new one.
func replSender(ctx context.Context, t *testing.T, primary *pgx.Conn) string {
	t.Helper()

	var sender string
	require.NoError(t, primary.QueryRow(ctx, `SELECT pid || ' since ' || backend_start FROM pg_stat_replication
		WHERE application_name = $1`, standbyName).Scan(&sender))

	return sender
}
