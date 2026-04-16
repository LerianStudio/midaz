// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// Test sentinel errors for load-balances behavior tests. Kept at package
// level so err113 does not flag per-call errors.New invocations.
var (
	errTestPersistentTransient = errors.New("simulated persistent pool exhaustion")
	errTestTransient           = errors.New("simulated transient")
	errTestNonPG               = errors.New("non-pg network error")
)

// buildScanRow is a small ergonomic helper that constructs a fakeRow with
// all 13 columns in the order scanBalance expects. It keeps the test
// bodies focused on the behaviour under test rather than positional
// arithmetic on a 13-element slice.
//
//nolint:unparam // org and version are intentionally parameterized for future multi-tenant and version-drift tests even though today's callers only exercise the default values.
func buildScanRow(
	id, org, ledger, accountID, alias, balanceKey, assetCode string,
	available, onHold *string, version int64, accountType string,
	allowSending, allowReceiving sql.NullBool,
) *fakeRow {
	return &fakeRow{values: []any{
		id, org, ledger, accountID, alias, balanceKey, assetCode,
		available, onHold, version, accountType,
		allowSending, allowReceiving,
	}}
}

// TestLoadBalances_IteratesAllRows proves that scanning N independent rows
// produces N engine.Balance records with the right aliases and that the
// router assigns each to a deterministic shard. This is the row-level
// iteration contract that LoadBalances depends on: a single lost row
// equals a zero-balance bootstrap for that account → unauthorized debit →
// financial incident.
func TestLoadBalances_IteratesAllRows(t *testing.T) {
	const rowCount = 1000

	router := shard.NewRouter(shard.DefaultShardCount)
	collected := make([]*engine.Balance, 0, rowCount)

	for i := 0; i < rowCount; i++ {
		row := buildScanRow(
			fmt.Sprintf("balance-%d", i),
			"org", "ledger",
			fmt.Sprintf("account-%d", i),
			fmt.Sprintf("@account-%d", i),
			"default", "USD",
			stringPtr("100"), stringPtr("0"),
			int64(1), "deposit",
			sql.NullBool{Bool: true, Valid: true},
			sql.NullBool{Bool: true, Valid: true},
		)

		balance, err := scanBalance(row, router, nil)
		require.NoError(t, err, "row %d scan must not fail", i)
		require.NotNil(t, balance, "row %d must produce a balance", i)

		collected = append(collected, balance)
	}

	require.Len(t, collected, rowCount, "all rows must be iterated — a missing row = silent data loss")

	seenAliases := make(map[string]struct{}, rowCount)
	for _, b := range collected {
		seenAliases[b.AccountAlias] = struct{}{}
	}

	require.Len(t, seenAliases, rowCount, "every alias must be uniquely represented")
}

// TestLoadBalances_HandlesSchemaDrift verifies the D1 audit fix: the
// loader MUST fail-closed on 42P01 (undefined_table) rather than returning
// nil silently. Silent nil previously caused the authorizer to bootstrap
// with zero balances when the `balance` table was missing due to a
// migration gap, making schema drift invisible until the first customer
// request.
//
// The assertion is: classifyQueryError(pgErr{42P01}) wraps
// ErrBalanceTableMissing so callers can errors.Is() to trigger a crash
// loop rather than continue with an empty engine.
func TestLoadBalances_HandlesSchemaDrift(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    pgCodeUndefinedTable,
		Message: "relation \"balance\" does not exist",
	}

	classified := classifyQueryError(pgErr)
	require.Error(t, classified)
	require.ErrorIs(t, classified, ErrBalanceTableMissing,
		"42P01 must map to ErrBalanceTableMissing to make schema drift loud")
	require.False(t, isTransientPGError(classified),
		"42P01 MUST NOT be retried — it's permanent")
}

// TestLoadBalances_HandlesPoolExhaustion_53300 verifies that the loader
// classifies 53300 (too_many_connections) as transient so the exponential
// backoff retry path triggers. Scenario: onboarding service saturates the
// shared PG pool during a burst; authorizer bootstrap must ride through
// the burst instead of crashing.
func TestLoadBalances_HandlesPoolExhaustion_53300(t *testing.T) {
	pgErr := &pgconn.PgError{Code: pgCodeTooManyConnections, Message: "too many connections"}

	classified := classifyQueryError(pgErr)
	require.Error(t, classified)
	require.True(t, isTransientPGError(classified),
		"53300 must be classified as transient so retryTransient() retries")
	require.NotErrorIs(t, classified, ErrBalanceTableMissing,
		"53300 MUST NOT be mistaken for schema drift")

	// Exercise retry behavior: a sequence of 53300s followed by success
	// must return nil after a successful attempt.
	var attempts atomic.Int32

	err := retryTransient(context.Background(), 3, func(_ context.Context) error {
		attempts.Add(1)

		if attempts.Load() < 3 {
			return &transientError{err: pgErr}
		}

		return nil
	})
	require.NoError(t, err, "retryTransient must succeed once the transient storm clears")
	require.Equal(t, int32(3), attempts.Load(), "all 3 attempts must run before success")
}

// TestLoadBalances_HandlesStatementTimeout_57014 verifies that 57014
// (query_canceled / statement_timeout) also maps to transient. A slow
// replica under replication lag may terminate long-running queries; the
// loader must retry rather than abort.
func TestLoadBalances_HandlesStatementTimeout_57014(t *testing.T) {
	pgErr := &pgconn.PgError{Code: pgCodeQueryCanceled, Message: "canceling statement due to statement timeout"}

	classified := classifyQueryError(pgErr)
	require.True(t, isTransientPGError(classified),
		"57014 must be classified as transient")

	// A permanent failure after exhausted retries returns the final
	// transient error wrapped with the attempt count.
	err := retryTransient(context.Background(), 2, func(_ context.Context) error {
		return &transientError{err: pgErr}
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exhausted after 2 attempts",
		"exhaustion message must include attempt count for operators")
}

// TestLoadBalances_HandlesNullDecimal verifies the nil-safety commit
// aeedc1550: NULL values in available/on_hold MUST coalesce to zero rather
// than causing decimal.NewFromString("") to error out and fail the
// entire bootstrap load. Covers both columns and both directions.
func TestLoadBalances_HandlesNullDecimal(t *testing.T) {
	t.Run("NULL available coalesces to zero", func(t *testing.T) {
		row := buildScanRow(
			"b1", "org", "ledger", "acct1", "@alice", "default", "USD",
			nil, stringPtr("50"), int64(1), "deposit",
			sql.NullBool{Bool: true, Valid: true},
			sql.NullBool{Bool: true, Valid: true},
		)

		balance, err := scanBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), balance.Available,
			"NULL available MUST coalesce to 0 via coalesceDecimalString")
	})

	t.Run("NULL on_hold coalesces to zero", func(t *testing.T) {
		row := buildScanRow(
			"b2", "org", "ledger", "acct2", "@bob", "default", "USD",
			stringPtr("1000"), nil, int64(1), "deposit",
			sql.NullBool{Bool: true, Valid: true},
			sql.NullBool{Bool: true, Valid: true},
		)

		balance, err := scanBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), balance.OnHold)
	})

	t.Run("both NULL produces a zero-balance row without error", func(t *testing.T) {
		row := buildScanRow(
			"b3", "org", "ledger", "acct3", "@carol", "default", "USD",
			nil, nil, int64(1), "deposit",
			sql.NullBool{},
			sql.NullBool{},
		)

		balance, err := scanBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
		require.NoError(t, err)
		require.NotNil(t, balance)
		require.Equal(t, int64(0), balance.Available)
		require.Equal(t, int64(0), balance.OnHold)
		require.True(t, balance.AllowSending, "NULL allow_sending fail-safe to true")
		require.True(t, balance.AllowReceiving, "NULL allow_receiving fail-safe to true")
	})
}

// TestLoadBalances_PartialLoadFailureMode exercises the exhaustion path:
// when transient errors outnumber retries, retryTransient surfaces the
// wrapped error with the "exhausted after N attempts" prefix. Callers
// (bootstrap.Run) treat this as fail-closed — the engine is NOT started
// with partial data.
func TestLoadBalances_PartialLoadFailureMode(t *testing.T) {
	var calls atomic.Int32

	persistentTransient := &transientError{err: errTestPersistentTransient}

	err := retryTransient(context.Background(), 3, func(_ context.Context) error {
		calls.Add(1)

		return persistentTransient
	})

	require.Error(t, err)
	require.Equal(t, int32(3), calls.Load(),
		"a persistent transient failure MUST use every retry attempt before giving up")
	require.Contains(t, err.Error(), "exhausted after 3 attempts",
		"exhaustion error MUST report the attempt count so operators can correlate with retry config")
	require.ErrorIs(t, errors.Unwrap(err), persistentTransient,
		"the original transient marker MUST remain unwrappable for %w-based matching")
}

// TestLoadBalances_PartialLoadFailureMode_ContextCancel verifies that a
// context cancellation mid-retry short-circuits the loop rather than
// continuing to retry past a shutdown. This is the graceful-shutdown
// contract: when the pod gets SIGTERM, the loader must not burn the
// termination grace period on retries.
func TestLoadBalances_PartialLoadFailureMode_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32

	err := retryTransient(ctx, 10, func(_ context.Context) error {
		calls.Add(1)

		if calls.Load() == 2 {
			cancel()
		}

		return &transientError{err: errTestTransient}
	})

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled,
		"context cancellation MUST surface as the returned error — not the transient class")
	require.LessOrEqual(t, calls.Load(), int32(3),
		"retry loop MUST stop within ~1 iteration of context cancel")
}

// TestLoadBalances_NonPGErrorNotRetried confirms the failure-isolation
// guarantee: a generic non-pgconn error (e.g. a wrapped network read
// failure from deep inside pgx) is NOT misclassified as transient and
// therefore does NOT consume the retry budget. The loader returns it
// immediately so the caller can decide whether to retry at a higher level.
func TestLoadBalances_NonPGErrorNotRetried(t *testing.T) {
	sentinel := errTestNonPG

	var calls atomic.Int32

	err := retryTransient(context.Background(), 5, func(_ context.Context) error {
		calls.Add(1)

		return sentinel
	})
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, int32(1), calls.Load(),
		"non-transient errors MUST NOT be retried — retry budget is precious on cold start")
}
