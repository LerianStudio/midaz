// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ---------------------------------------------------------------------------
// Lock-ordering deadlock proof.
//
// The pre-fix reserve loop takes a fine-grained usage_counters ROW lock (via the
// reserve CTE's INSERT ... ON CONFLICT DO UPDATE) BEFORE the audit BEFORE-INSERT
// trigger takes the GLOBAL advisory lock pg_advisory_xact_lock(314159265). Two
// concurrent same-account reserves whose overlapping counter sets are entered in
// DIFFERENT relative order form a wait-for cycle — one holds the advisory lock and
// waits on a counter row the other holds, while the other holds that row and waits
// on the advisory lock — which PostgreSQL breaks by aborting one with SQLSTATE
// 40P01 (deadlock_detected).
//
// These proofs drive the REAL ReservationService over a REAL PostgreSQL connection
// with the REAL audit writer, so the audit trigger's advisory lock actually fires
// (the crash-convergence proofs deliberately stub the audit writer to skip it).
// The discriminating, retry-independent signal is PostgreSQL's own
// pg_stat_database.deadlocks counter: a deadlock is counted the moment it is
// detected, even when the transient-retry then re-runs the victim and
// hides the 40P01 from the caller. The root-cause fix (a per-account advisory lock
// acquired at transaction start, before any counter row is touched) must drive that
// delta to exactly zero: same-account reserves serialize on the scope lock and can
// no longer interleave into a cycle.
// ---------------------------------------------------------------------------

// resOpenConcurrentDB opens a dedicated *sql.DB against the integration container
// with a pool large enough for genuine concurrency (the shared SetupIntegrationDB
// caps MaxOpenConns at 2, which would serialize the racing reserves and mask the
// cycle). Closed via t.Cleanup.
func resOpenConcurrentDB(t *testing.T, maxConns int) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", testutil.GetTestDSN())
	require.NoError(t, err, "failed to open concurrent integration DB")

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	db.SetConnMaxLifetime(2 * time.Minute)

	require.NoError(t, db.Ping(), "failed to ping concurrent integration DB")

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("cleanup: failed to close concurrent DB: %v", err)
		}
	})

	return db
}

// resWireServiceRealAudit builds a ReservationService with the REAL audit writer
// (command.RecordAuditEventCommand over the postgres audit repository), so the
// audit BEFORE-INSERT trigger — and its global advisory lock — is exercised on the
// reserve path. This is what the crash-convergence proofs skip via resCountingAudit.
func resWireServiceRealAudit(t *testing.T, db *sql.DB, resolver services.LimitResolver) *services.ReservationService {
	t.Helper()

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	counterRepo := postgres.NewUsageCounterRepositoryWithConnection(adapter)
	resRepo := postgres.NewUsageReservationRepositoryWithConnection(counterRepo)
	auditRepo := postgres.NewAuditEventRepositoryWithConnection(adapter)
	auditWriter := command.NewRecordAuditEventCommand(auditRepo)

	svc, err := services.NewReservationService(
		resSQLTxBeginner{db: db},
		resolver,
		resRepo,
		auditWriter,
		nil, // RealClock
	)
	require.NoError(t, err, "failed to wire reservation service with real audit writer")

	return svc
}

// resReadDeadlockCount reads PostgreSQL's cumulative deadlock counter for the test
// database. A delta of zero across a concurrent window proves no wait-for cycle
// ever formed — the retry-independent invariant this task guarantees.
func resReadDeadlockCount(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var deadlocks int64

	err := db.QueryRow(
		"SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()",
	).Scan(&deadlocks)
	require.NoError(t, err, "failed to read pg_stat_database.deadlocks")

	return deadlocks
}

// resSettledDeadlockCount reads the deadlock counter with a bounded settle-poll.
// pg_stat_database.deadlocks is maintained by the stats collector and can lag the
// racing window by the reporting interval, so a single immediate read can let a
// late-surfacing deadlock turn a real cycle into a false pass. It polls with fresh
// autocommit reads until the counter advances past before or the deadline expires;
// a genuinely deadlock-free run simply exhausts the bounded deadline at before. The
// deadline is derived from the test context, not a wall-clock read.
func resSettledDeadlockCount(t *testing.T, db *sql.DB, before int64) int64 {
	t.Helper()

	settleCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	after := resReadDeadlockCount(t, db)
	for after == before && settleCtx.Err() == nil {
		time.Sleep(25 * time.Millisecond)
		after = resReadDeadlockCount(t, db)
	}

	return after
}

// resCheckInputForAccount is resCheckInput with a caller-chosen account id, so the
// distinct-account proof gives each reserve its own per-account advisory-lock key.
func resCheckInputForAccount(t *testing.T, accountID uuid.UUID) *model.CheckLimitsInput {
	t.Helper()

	input, err := model.NewCheckLimitsInput(
		decimal.NewFromInt(100),
		"USD",
		accountID,
		nil, nil, nil, nil, nil,
		testutil.TestNow(),
	)
	require.NoError(t, err)

	return input
}

// TestIntegration_ReservationConcurrentSameAccount_NoDeadlock is the primary
// lock-ordering proof: N reserves on the SAME account, each holding TWO limits,
// entered in ALTERNATING relative order (even goroutines [A,B], odd [B,A]) so the
// pre-fix counter-row-before-advisory-lock ordering can cycle. After the race:
//   - PostgreSQL's deadlock counter must not have advanced (no cycle formed),
//   - no reserve may surface a hard (non-business) error,
//   - every reserve is allowed (capacity is generous) and both counters hold
//     exactly the sum of the held amounts.
func TestIntegration_ReservationConcurrentSameAccount_NoDeadlock(t *testing.T) {
	testutil.SetupTestTracing(t)

	const (
		goroutines = 16
		amount     = 10
		capacity   = 1_000_000
	)

	db := resOpenConcurrentDB(t, goroutines)

	limitA := resSeedLimit(t, db, 9301, "lockorder-A", capacity)
	limitB := resSeedLimit(t, db, 9302, "lockorder-B", capacity)
	t.Cleanup(func() {
		resCleanupLimit(t, db, limitA)
		resCleanupLimit(t, db, limitB)
	})

	// One shared account: every reserve contends on the SAME two counter rows.
	accountID := testutil.MustDeterministicUUID(9300)
	scopeA := "acct:lockorder-" + testutil.MustDeterministicUUID(9311).String()[:8]
	scopeB := "global-lockorder-" + testutil.MustDeterministicUUID(9312).String()[:8]
	periodKey := "2026-06"

	// Alternating spec order across goroutines is what makes the pre-fix ordering
	// cyclic: [A,B] vs [B,A] over the same two counters.
	orderAB := []query.ReservationSpec{
		resSpec(limitA, scopeA, periodKey, amount, capacity),
		resSpec(limitB, scopeB, periodKey, amount, capacity),
	}
	orderBA := []query.ReservationSpec{
		resSpec(limitB, scopeB, periodKey, amount, capacity),
		resSpec(limitA, scopeA, periodKey, amount, capacity),
	}

	// Build the services and the shared input on the TEST goroutine: both helpers
	// call require.NoError internally, and require's FailNow (runtime.Goexit) is
	// only valid on the goroutine running the test. Only two spec orders exist, so
	// two services cover every worker.
	svcAB := resWireServiceRealAudit(t, db, resStubResolver{specs: orderAB})
	svcBA := resWireServiceRealAudit(t, db, resStubResolver{specs: orderBA})
	input := resCheckInputForAccount(t, accountID)

	before := resReadDeadlockCount(t, db)

	var (
		wg        sync.WaitGroup
		allowed   atomic.Int64
		denied    atomic.Int64
		hardError atomic.Int64
	)

	for i := range goroutines {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			svc := svcAB
			if idx%2 == 1 {
				svc = svcBA
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			txID := testutil.MustDeterministicUUID(int64(93100 + idx))

			res, err := svc.Reserve(ctx, txID, input, false)
			switch {
			case err != nil:
				hardError.Add(1)
				t.Logf("goroutine %d surfaced a hard error: %v", idx, err)
			case res.Denied:
				denied.Add(1)
			default:
				allowed.Add(1)
			}
		}(i)
	}

	wg.Wait()

	after := resSettledDeadlockCount(t, db, before)

	// The pg_stat_database.deadlocks delta is a probabilistic signal: it only
	// advances if the racing goroutines happen to interleave into a cycle within the
	// window. TestIntegration_AuditHashChain_PreFixForkReproduction (test 24) is the
	// authoritative, deterministic RED for the same lock-ordering class — it drives
	// the fork by hand rather than by timing, so it fails reliably pre-fix.
	assert.Equal(t, before, after,
		"no PostgreSQL deadlock (40P01) may form: the per-account advisory lock must serialize same-account reserves before any counter row is locked (delta before=%d after=%d)",
		before, after)
	assert.Equal(t, int64(0), hardError.Load(), "no reserve may surface a hard (non-business) error")
	assert.Equal(t, int64(0), denied.Load(), "capacity is generous, no reserve may be denied")
	assert.Equal(t, int64(goroutines), allowed.Load(), "every reserve must be allowed")

	_, rsvA := resReadCounter(t, db, limitA, scopeA, periodKey)
	_, rsvB := resReadCounter(t, db, limitB, scopeB, periodKey)
	assert.Equal(t, int64(goroutines*amount), rsvA, "limit A reserved_usage must equal the sum of held amounts")
	assert.Equal(t, int64(goroutines*amount), rsvB, "limit B reserved_usage must equal the sum of held amounts")
}

// TestIntegration_ReservationConcurrentDistinctAccounts_Parallelizes proves the
// per-account lock does NOT collapse cross-account throughput into a single global
// queue: N reserves, each on its OWN account (its own per-account advisory-lock key
// and its own counter), all succeed with no deadlock and each account's counter
// holds exactly its own amount.
func TestIntegration_ReservationConcurrentDistinctAccounts_Parallelizes(t *testing.T) {
	testutil.SetupTestTracing(t)

	const (
		goroutines = 16
		amount     = 10
		capacity   = 1_000_000
	)

	db := resOpenConcurrentDB(t, goroutines)

	limitID := resSeedLimit(t, db, 9320, "distinct-accounts", capacity)
	t.Cleanup(func() { resCleanupLimit(t, db, limitID) })

	periodKey := "2026-06"

	before := resReadDeadlockCount(t, db)

	var (
		wg        sync.WaitGroup
		allowed   atomic.Int64
		hardError atomic.Int64
	)

	scopes := make([]string, goroutines)
	// Build each worker's service and input on the TEST goroutine: both helpers
	// call require.NoError, whose FailNow (runtime.Goexit) must run on the test
	// goroutine. Each worker has its own account/scope, so one per index.
	svcs := make([]*services.ReservationService, goroutines)
	inputs := make([]*model.CheckLimitsInput, goroutines)

	for i := range goroutines {
		// Full UUID, not a prefix: MustDeterministicUUID encodes the seed in the
		// TRAILING bytes, so a [:8] slice collapses every scope onto one counter.
		scopes[i] = "acct:distinct-" + testutil.MustDeterministicUUID(int64(93400+i)).String()

		specs := []query.ReservationSpec{resSpec(limitID, scopes[i], periodKey, amount, capacity)}
		svcs[i] = resWireServiceRealAudit(t, db, resStubResolver{specs: specs})
		inputs[i] = resCheckInputForAccount(t, testutil.MustDeterministicUUID(int64(93500+i)))

		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			svc := svcs[idx]

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			txID := testutil.MustDeterministicUUID(int64(93600 + idx))

			res, err := svc.Reserve(ctx, txID, inputs[idx], false)
			switch {
			case err != nil:
				hardError.Add(1)
				t.Logf("goroutine %d surfaced a hard error: %v", idx, err)
			case res.Denied:
				// unreachable with generous capacity
			default:
				allowed.Add(1)
			}
		}(i)
	}

	wg.Wait()

	after := resSettledDeadlockCount(t, db, before)

	assert.Equal(t, before, after, "distinct-account reserves share no counter and must never deadlock")
	assert.Equal(t, int64(0), hardError.Load(), "no reserve may surface a hard error")
	assert.Equal(t, int64(goroutines), allowed.Load(), "every distinct-account reserve must be allowed")

	for i := range goroutines {
		_, reserved := resReadCounter(t, db, limitID, scopes[i], periodKey)
		assert.Equal(t, int64(amount), reserved, "each account's counter must hold exactly its own amount")
	}
}

// TestIntegration_ReservationConcurrentSameAccount_OverLimitStillDenies proves the
// fix does not weaken the over-commit guard: N reserves of 1 unit each against a
// capacity of N-1 on the SAME account (alternating two-limit order) must still
// admit exactly N-1 and deny exactly one, with no deadlock and no hard error.
func TestIntegration_ReservationConcurrentSameAccount_OverLimitStillDenies(t *testing.T) {
	testutil.SetupTestTracing(t)

	const (
		goroutines = 12
		capacity   = goroutines - 1
	)

	db := resOpenConcurrentDB(t, goroutines)

	// limitA carries the binding capacity; limitB is generous so only limitA denies.
	limitA := resSeedLimit(t, db, 9330, "overlimit-A", capacity)
	limitB := resSeedLimit(t, db, 9331, "overlimit-B", 1_000_000)
	t.Cleanup(func() {
		resCleanupLimit(t, db, limitA)
		resCleanupLimit(t, db, limitB)
	})

	accountID := testutil.MustDeterministicUUID(9333)
	scopeA := "acct:overlimit-" + testutil.MustDeterministicUUID(9341).String()[:8]
	scopeB := "global-overlimit-" + testutil.MustDeterministicUUID(9342).String()[:8]
	periodKey := "2026-06"

	// Build the two spec-order services and the shared input on the TEST goroutine:
	// resWireServiceRealAudit / resCheckInputForAccount call require.NoError, whose
	// FailNow (runtime.Goexit) is only valid on the test goroutine.
	specsAB := []query.ReservationSpec{
		resSpec(limitA, scopeA, periodKey, 1, capacity),
		resSpec(limitB, scopeB, periodKey, 1, 1_000_000),
	}
	specsBA := []query.ReservationSpec{specsAB[1], specsAB[0]}
	svcAB := resWireServiceRealAudit(t, db, resStubResolver{specs: specsAB})
	svcBA := resWireServiceRealAudit(t, db, resStubResolver{specs: specsBA})
	input := resCheckInputForAccount(t, accountID)

	before := resReadDeadlockCount(t, db)

	var (
		wg        sync.WaitGroup
		allowed   atomic.Int64
		denied    atomic.Int64
		hardError atomic.Int64
	)

	for i := range goroutines {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			svc := svcAB
			if idx%2 == 1 {
				svc = svcBA
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			txID := testutil.MustDeterministicUUID(int64(93700 + idx))

			res, err := svc.Reserve(ctx, txID, input, false)
			switch {
			case err != nil:
				hardError.Add(1)
				t.Logf("goroutine %d surfaced a hard error: %v", idx, err)
			case res.Denied:
				denied.Add(1)
			default:
				allowed.Add(1)
			}
		}(i)
	}

	wg.Wait()

	after := resSettledDeadlockCount(t, db, before)

	assert.Equal(t, before, after, "the over-limit path must also be deadlock-free")
	assert.Equal(t, int64(0), hardError.Load(), "no reserve may surface a hard error")
	assert.Equal(t, int64(capacity), allowed.Load(), "exactly N-1 reserves must be admitted under the binding cap")
	assert.Equal(t, int64(1), denied.Load(), "exactly one reserve must lose with the limit-exceeded decision")

	cur, rsv := resReadCounter(t, db, limitA, scopeA, periodKey)
	assert.Equal(t, int64(capacity), cur+rsv, "the binding counter must saturate at exactly the cap, never over it")
}
