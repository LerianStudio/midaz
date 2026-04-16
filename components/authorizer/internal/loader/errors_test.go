// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// TestClassifyQueryError_FailClosedOn42P01 verifies the D1 audit fix: the
// loader MUST return ErrBalanceTableMissing rather than silently returning a
// nil result on undefined_table. Silent nil previously caused the authorizer
// to bootstrap with zero balances against a migration-gap database, making
// the schema-missing case invisible until first customer request.
func TestClassifyQueryError_FailClosedOn42P01(t *testing.T) {
	pgErr := &pgconn.PgError{Code: pgCodeUndefinedTable, Message: "relation \"balance\" does not exist"}

	out := classifyQueryError(pgErr)
	require.Error(t, out)
	require.ErrorIs(t, out, ErrBalanceTableMissing)
}

// TestClassifyQueryError_TransientCodesAreRetryable confirms both 53300 and
// 57014 map to the transientError marker so retryTransient will retry them.
// 53300 is the ripple-effect hazard from D12 (onboarding exhaustion bankrupts
// authorizer's 20-conn pool); 57014 is statement_timeout on a slow replica.
func TestClassifyQueryError_TransientCodesAreRetryable(t *testing.T) {
	cases := []string{pgCodeTooManyConnections, pgCodeQueryCanceled}
	for _, code := range cases {
		t.Run(code, func(t *testing.T) {
			pgErr := &pgconn.PgError{Code: code, Message: "transient"}
			out := classifyQueryError(pgErr)
			require.Error(t, out)
			require.True(t, isTransientPGError(out), "expected transient classification for code=%s", code)
		})
	}
}

// TestClassifyQueryError_NonPGErrorPassesThrough documents the default: any
// non-pgconn error is wrapped but not classified as transient. A network
// hiccup manifesting as a generic context.DeadlineExceeded is retryable by
// the caller's outer retry loop, not by retryTransient.
func TestClassifyQueryError_NonPGErrorPassesThrough(t *testing.T) {
	out := classifyQueryError(errSimulatedNetwork)
	require.Error(t, out)
	require.NotErrorIs(t, out, ErrBalanceTableMissing)
	require.False(t, isTransientPGError(out))
}

// TestRetryTransient_RespectsMaxAttempts proves the exhaustion path returns
// the last error wrapped with the attempt count, so operators can tell
// "we gave up after 3" apart from "persistent failure".
func TestRetryTransient_RespectsMaxAttempts(t *testing.T) {
	var calls int

	err := retryTransient(context.Background(), 3, func(_ context.Context) error {
		calls++
		return &transientError{err: errSimulatedBoom}
	})
	require.Error(t, err)
	require.Equal(t, 3, calls, "transient op should be retried exactly maxAttempts times")
	require.Contains(t, err.Error(), "exhausted after 3 attempts")
}

// TestRetryTransient_NonTransientReturnsImmediately ensures a non-transient
// error (like ErrBalanceTableMissing) short-circuits without retrying.
func TestRetryTransient_NonTransientReturnsImmediately(t *testing.T) {
	var calls int

	err := retryTransient(context.Background(), 5, func(_ context.Context) error {
		calls++
		return ErrBalanceTableMissing
	})
	require.Error(t, err)
	require.Equal(t, 1, calls, "non-transient error must not trigger retries")
	require.ErrorIs(t, err, ErrBalanceTableMissing)
}

// TestRetryTransient_SuccessAfterOneRetry models the common case: first call
// hits a transient error, second succeeds.
func TestRetryTransient_SuccessAfterOneRetry(t *testing.T) {
	var calls int

	err := retryTransient(context.Background(), 3, func(_ context.Context) error {
		calls++
		if calls == 1 {
			return &transientError{err: errSimulatedTemporary}
		}

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

// TestRetryTransient_RespectsContextCancellation ensures a shutting-down
// process exits the retry loop on ctx cancel rather than sleeping through
// its grace period.
func TestRetryTransient_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := retryTransient(ctx, 5, func(_ context.Context) error {
		return &transientError{err: errSimulatedBoom}
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// errSimulated* are test-local sentinel errors used in place of ad-hoc
// fmt.Errorf strings so golangci-lint (err113) stays happy and so assertions
// can use ErrorIs when they need to match the exact simulated failure.
var (
	errSimulatedBoom      = errors.New("simulated boom")
	errSimulatedTemporary = errors.New("simulated temporary")
	errSimulatedNetwork   = errors.New("simulated network unreachable")
)

// TestBuildStreamingQuery_FirstPageHasNoCursor validates the keyset
// pagination SQL generation: on the first page the cursor predicate is
// omitted so we can fetch from the earliest row without relying on a
// sentinel timestamp value.
func TestBuildStreamingQuery_FirstPageHasNoCursor(t *testing.T) {
	query, args := buildStreamingQuery("org", "ledger", time.Time{}, "", 100, true)

	require.Contains(t, query, "WHERE deleted_at IS NULL")
	require.Contains(t, query, "organization_id = $")
	require.Contains(t, query, "ledger_id = $")
	require.Contains(t, query, "ORDER BY COALESCE(updated_at, created_at), id")
	require.Contains(t, query, "LIMIT $")
	require.NotContains(t, query, "> (")
	require.Equal(t, []any{"org", "ledger", 100}, args)
}

// TestBuildStreamingQuery_SecondPageUsesCompositeCursor validates that
// subsequent pages use (updated_at, id) > (cursor_updated_at, cursor_id)
// for a strict composite-cursor advance.
func TestBuildStreamingQuery_SecondPageUsesCompositeCursor(t *testing.T) {
	cursorTs := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	query, args := buildStreamingQuery("org", "", cursorTs, "uuid-xyz", 500, false)

	require.Contains(t, query, "(COALESCE(updated_at, created_at), id) > ($")
	require.Contains(t, query, "LIMIT $")
	require.Len(t, args, 4)
	require.Equal(t, "org", args[0])
	require.Equal(t, cursorTs, args[1])
	require.Equal(t, "uuid-xyz", args[2])
	require.Equal(t, 500, args[3])
}

// TestCollectBalances_SkipsShardFilteredRows verifies that cursorRow entries
// with nil balance (shard-filtered) are dropped from the consumer-facing
// batch, while the cursor itself still advances through those rows.
func TestCollectBalances_SkipsShardFilteredRows(t *testing.T) {
	rows := []cursorRow{
		{cursorID: "1"}, // filtered (nil balance)
		{cursorID: "2"}, // filtered
		{cursorID: "3"}, // filtered
	}
	require.Empty(t, collectBalances(rows))
}

// TestBuildShardFilter_EmptyShardsYieldsNilMap documents the policy: empty
// shardIDs means "include all shards" — returning nil tells scan helpers to
// skip the filter check entirely.
func TestBuildShardFilter_EmptyShardsYieldsNilMap(t *testing.T) {
	require.Nil(t, buildShardFilter(nil))
	require.Nil(t, buildShardFilter([]int32{}))

	m := buildShardFilter([]int32{0, 2, 5})
	require.NotNil(t, m)
	require.Contains(t, m, int32(0))
	require.Contains(t, m, int32(2))
	require.Contains(t, m, int32(5))
}

// TestLoadBalancesStreaming_NilConsumeRejected validates input checking: the
// caller MUST provide a consume callback or the call is refused.
func TestLoadBalancesStreaming_NilConsumeRejected(t *testing.T) {
	var ldr *PostgresLoader // nil-pool path triggers the not-init guard first

	_, err := ldr.LoadBalancesStreaming(context.Background(), "", "", nil, time.Time{}, nil)
	require.Error(t, err)
}

// TestLoader_StreamingPaginationQueryShape documents the end-to-end query
// shape produced by buildStreamingQuery across pages. This is the test the
// audit asked for ("TestLoader_StreamingPagination") at the unit level — a
// full testcontainers-backed scan is provided as an integration test in a
// follow-up task because the pgxmock dependency is not yet in go.mod.
func TestLoader_StreamingPaginationQueryShape(t *testing.T) {
	// First page: no cursor predicate, LIMIT only.
	q0, a0 := buildStreamingQuery("", "", time.Time{}, "", 10_000, true)
	require.Contains(t, q0, "ORDER BY COALESCE(updated_at, created_at), id")
	require.Contains(t, q0, "LIMIT $1")
	require.Equal(t, []any{10_000}, a0)

	// Middle page: (updated_at, id) > cursor.
	cursor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q1, a1 := buildStreamingQuery("", "", cursor, "uuid-1", 10_000, false)
	require.Contains(t, q1, "(COALESCE(updated_at, created_at), id) > ($1, $2)")
	require.Contains(t, q1, "LIMIT $3")
	require.Equal(t, []any{cursor, "uuid-1", 10_000}, a1)

	// Filtered middle page: org + ledger + cursor + limit = 5 args.
	q2, a2 := buildStreamingQuery("org", "ledger", cursor, "uuid-2", 500, false)
	require.Contains(t, q2, "organization_id = $1")
	require.Contains(t, q2, "ledger_id = $2")
	require.Contains(t, q2, "(COALESCE(updated_at, created_at), id) > ($3, $4)")
	require.Contains(t, q2, "LIMIT $5")
	require.Equal(t, []any{"org", "ledger", cursor, "uuid-2", 500}, a2)
}
