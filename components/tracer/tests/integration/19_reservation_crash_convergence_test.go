// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ---------------------------------------------------------------------------
// Test-only seams. The reservation SUT (ReservationService + reaper worker)
// depends on three INTERFACES: a LimitResolver, a ReservationAuditWriter, and a
// pgdb.TxBeginner. The proofs below are about counter CONVERGENCE and the
// over-commit WHERE-guard, not about limit resolution or the audit hash chain
// (those are proven by ResolveReservations' own unit tests and F3-T07). So we
// drive the real service + real repo + real reaper over a real PostgreSQL
// connection, substituting only those orthogonal collaborators with
// deterministic test doubles. No production code is modified.
// ---------------------------------------------------------------------------

// resSQLTxBeginner adapts a raw *sql.DB to pgdb.TxBeginner so ReservationService
// can own its own transactions over the integration database. *sql.Tx already
// satisfies pgdb.Tx (ExecContext/QueryContext/QueryRowContext/Commit/Rollback).
type resSQLTxBeginner struct {
	db *sql.DB
}

func (b resSQLTxBeginner) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
	tx, err := b.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// resStubResolver returns a fixed set of ReservationSpecs, isolating the proof
// from the DB-wide "list every active limit for this currency" behaviour of the
// real ResolveReservations (which would couple the assertion to other tests'
// lingering limits). It is the LimitResolver the ReservationService consumes.
type resStubResolver struct {
	specs  []query.ReservationSpec
	denied bool
	err    error
}

func (r resStubResolver) ResolveReservations(_ context.Context, _ *model.CheckLimitsInput) ([]query.ReservationSpec, bool, error) {
	return r.specs, r.denied, r.err
}

// resCountingAudit satisfies BOTH the ReservationService's ReservationAuditWriter
// and the reaper's ReservationExpiryAuditor. It records nothing to the hash chain
// (that path is proven by F3-T07) but counts calls so the proofs can assert the
// audit side fired once per transition / once per sweep without standing up the
// audit repository and its global advisory lock.
type resCountingAudit struct {
	perRow atomic.Int64
	batch  atomic.Int64
}

func (a *resCountingAudit) RecordReservationEventWithTx(
	_ context.Context,
	_ pgdb.DB,
	_ model.AuditEventType,
	_ model.AuditAction,
	_ uuid.UUID,
	_ command.ReservationAuditContext,
) error {
	a.perRow.Add(1)
	return nil
}

func (a *resCountingAudit) RecordReservationExpiryBatch(_ context.Context, _ command.ReservationExpiryBatchSummary) error {
	a.batch.Add(1)
	return nil
}

// resSeedLimit inserts an ACTIVE DAILY limit with the given ceiling and a unique
// name (mirroring createTestLimitNamed in the postgres package — the shared
// createTestLimit derives the name from the UUID prefix, which is identical for
// every deterministic seed because MustDeterministicUUID encodes the seed in the
// TRAILING bytes, so two limits in one test would collide on
// idx_limits_name_active).
func resSeedLimit(t *testing.T, db *sql.DB, seed int64, name string, maxAmount int64) uuid.UUID {
	t.Helper()

	limitID := testutil.MustDeterministicUUID(seed)

	_, err := db.Exec(`
		INSERT INTO limits (id, name, limit_type, max_amount, currency, scopes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, limitID, "Reservation Proof Limit "+name, "DAILY", decimal.NewFromInt(maxAmount), "USD", "[]", "ACTIVE")
	require.NoError(t, err, "failed to seed proof limit")

	return limitID
}

func resCleanupLimit(t *testing.T, db *sql.DB, limitID uuid.UUID) {
	t.Helper()

	if _, err := db.Exec("DELETE FROM limits WHERE id = $1", limitID); err != nil {
		t.Logf("cleanup: failed to delete limit %s: %v", limitID, err)
	}
}

// resReadCounter reads the (current_usage, reserved_usage) pair for a counter.
// A missing counter row yields (0, 0): a reserve that the guard denied before any
// INSERT, or a counter that was never created, is observationally a zero counter.
func resReadCounter(t *testing.T, db *sql.DB, limitID uuid.UUID, scopeKey, periodKey string) (current, reserved int64) {
	t.Helper()

	err := db.QueryRow(
		"SELECT current_usage, reserved_usage FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey,
	).Scan(&current, &reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0
	}

	require.NoError(t, err, "failed to read counter buckets")

	return current, reserved
}

func resReadStatus(t *testing.T, db *sql.DB, reservationID uuid.UUID) string {
	t.Helper()

	var status string

	err := db.QueryRow("SELECT status FROM usage_reservations WHERE id = $1", reservationID).Scan(&status)
	require.NoError(t, err, "failed to read reservation status")

	return status
}

// resWireService builds a ReservationService over the integration DB, with the
// given resolver specs and a counting audit writer. The returned audit writer is
// shared with the reaper in tests that need it.
func resWireService(t *testing.T, db *sql.DB, resolver services.LimitResolver, audit *resCountingAudit) *services.ReservationService {
	t.Helper()

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	counterRepo := postgres.NewUsageCounterRepositoryWithConnection(adapter)
	resRepo := postgres.NewUsageReservationRepositoryWithConnection(counterRepo)

	svc, err := services.NewReservationService(
		resSQLTxBeginner{db: db},
		resolver,
		resRepo,
		audit,
		nil, // RealClock for reserve/confirm/release timestamps
	)
	require.NoError(t, err, "failed to wire reservation service")

	return svc
}

// resWireReaper builds the real reaper worker over the integration DB with an
// injected MockClock, so a sweep can be driven past the reservation TTL
// deterministically via RunOnce. The reaper shares the counting audit writer so
// the batch-summary call can be asserted.
func resWireReaper(t *testing.T, db *sql.DB, audit *resCountingAudit, sweepAt time.Time) *workers.ReservationReaperWorker {
	t.Helper()

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	counterRepo := postgres.NewUsageCounterRepositoryWithConnection(adapter)
	resRepo := postgres.NewUsageReservationRepositoryWithConnection(counterRepo)
	reaperRepo := postgres.NewReservationReaperRepository(adapter, resSQLTxBeginner{db: db}, resRepo)

	reaper, err := workers.NewReservationReaperWorker(
		reaperRepo,
		audit,
		workers.DefaultReservationReaperWorkerConfig(),
		testutil.NewMockLogger(),
		testutil.NewMockClock(sweepAt),
		"", // single-tenant
	)
	require.NoError(t, err, "failed to wire reservation reaper worker")

	return reaper
}

// spec is a small helper for the stub resolver: one counter-backed limit.
func resSpec(limitID uuid.UUID, scopeKey, periodKey string, amount, maxAmount int64) query.ReservationSpec {
	return query.ReservationSpec{
		LimitID:   limitID,
		ScopeKey:  scopeKey,
		PeriodKey: periodKey,
		Amount:    amount,
		MaxAmount: maxAmount,
	}
}

// resCheckInput is a minimal valid CheckLimitsInput. The stub resolver ignores
// its contents, but ReservationService.Reserve forwards it and a nil input is
// rejected, so the proofs pass a well-formed one.
func resCheckInput(t *testing.T) *model.CheckLimitsInput {
	t.Helper()

	input, err := model.NewCheckLimitsInput(
		decimal.NewFromInt(100),
		"USD",
		testutil.MustDeterministicUUID(900001),
		nil, nil, nil, nil, nil,
		testutil.TestNow(),
	)
	require.NoError(t, err)

	return input
}

// ---------------------------------------------------------------------------
// PROOF 1 — Crash convergence (Gate 1)
//
// The two-phase reservation must converge to the correct counter state under a
// ledger crash at EVERY interleaving of reserve <-> confirm. The three sub-tests
// are the three crash windows the plan enumerates; each asserts EXACT counter
// values, which is the whole point — the TTL reaper is what makes (a) and (b)
// converge, and (c) proves a delivered confirm survives a pre-reaper crash.
// ---------------------------------------------------------------------------

func TestReservationCrashConvergence(t *testing.T) {
	// (a) Reserve succeeded; the ledger died BEFORE the balance commit, so no
	// confirm and no release is ever sent. The short-TTL reservation must be
	// swept by the reaper and reserved_usage returned to baseline (0). This is
	// the leaked-reservation case.
	t.Run("reserve_then_crash_before_commit_reaper_drains", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		db := testutil.SetupIntegrationDB(t)

		limitID := resSeedLimit(t, db, 8701, "crash-a", 10000)
		t.Cleanup(func() { resCleanupLimit(t, db, limitID) })

		scopeKey := "acct:crash-a-" + testutil.MustDeterministicUUID(8711).String()[:8]
		periodKey := "2026-06"
		txID := testutil.MustDeterministicUUID(8721)

		audit := &resCountingAudit{}
		svc := resWireService(t, db, resStubResolver{
			specs: []query.ReservationSpec{resSpec(limitID, scopeKey, periodKey, 400, 10000)},
		}, audit)

		ctx := context.Background()

		// Phase one: reserve. Capacity is held in reserved_usage.
		res, err := svc.Reserve(ctx, txID, resCheckInput(t), false)
		require.NoError(t, err)
		require.False(t, res.Denied)
		require.Len(t, res.ReservationIDs, 1)

		current, reserved := resReadCounter(t, db, limitID, scopeKey, periodKey)
		assert.Equal(t, int64(0), current, "reserve must not touch current_usage")
		assert.Equal(t, int64(400), reserved, "reserve must hold capacity in reserved_usage")

		// --- LEDGER CRASH: neither Confirm nor Release is ever called. ---

		// TTL elapses. The reaper sweeps at now + TTL + a margin and expires the
		// abandoned reservation.
		sweepAt := time.Now().UTC().Add(6 * time.Minute)
		reaper := resWireReaper(t, db, audit, sweepAt)

		released, err := reaper.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, released, "the abandoned reservation must be reaped")

		// Convergence: reserved_usage is back to baseline, current_usage never moved.
		current, reserved = resReadCounter(t, db, limitID, scopeKey, periodKey)
		assert.Equal(t, int64(0), current, "an abandoned reserve must NEVER reach current_usage")
		assert.Equal(t, int64(0), reserved, "the reaper must drain the leaked reserved_usage")
		assert.Equal(t, string(model.StatusExpired), resReadStatus(t, db, res.ReservationIDs[0]))
		assert.Equal(t, int64(1), audit.batch.Load(), "one batch-summary audit row per sweep")
	})

	// (b) The balance committed on the ledger, but the confirm transport was
	// LOST (network drop, ledger crash after commit but before the confirm
	// reaches the tracer). The tracer never hears the confirm, so to the tracer
	// the reservation is indistinguishable from (a): the SAME TTL + reaper path
	// converges it. THIS is why the TTL is mandatory — a best-effort confirm can
	// always be lost, and without the reaper the capacity would leak forever.
	t.Run("commit_then_confirm_lost_same_ttl_converges", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		db := testutil.SetupIntegrationDB(t)

		limitID := resSeedLimit(t, db, 8702, "crash-b", 10000)
		t.Cleanup(func() { resCleanupLimit(t, db, limitID) })

		scopeKey := "acct:crash-b-" + testutil.MustDeterministicUUID(8712).String()[:8]
		periodKey := "2026-06"
		txID := testutil.MustDeterministicUUID(8722)

		audit := &resCountingAudit{}
		svc := resWireService(t, db, resStubResolver{
			specs: []query.ReservationSpec{resSpec(limitID, scopeKey, periodKey, 700, 10000)},
		}, audit)

		ctx := context.Background()

		res, err := svc.Reserve(ctx, txID, resCheckInput(t), false)
		require.NoError(t, err)
		require.Len(t, res.ReservationIDs, 1)

		_, reserved := resReadCounter(t, db, limitID, scopeKey, periodKey)
		require.Equal(t, int64(700), reserved)

		// --- The ledger COMMITTED its balance, then the Confirm RPC was lost. ---
		// From the tracer's perspective nothing arrived; the reservation is still
		// RESERVED. The TTL reaper is the durability backstop.

		sweepAt := time.Now().UTC().Add(6 * time.Minute)
		reaper := resWireReaper(t, db, audit, sweepAt)

		released, err := reaper.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, released)

		// The reservation drains to baseline via EXPIRED, exactly as in (a). The
		// counter does NOT credit the lost-confirm amount — the tracer cannot
		// invent a confirm it never received; reconciliation of a genuinely
		// committed-but-unconfirmed transaction is the ledger's retry concern, not
		// the counter's. The invariant the tracer guarantees is convergence to a
		// drained reserved_usage, never a leaked hold.
		current, reserved := resReadCounter(t, db, limitID, scopeKey, periodKey)
		assert.Equal(t, int64(0), current)
		assert.Equal(t, int64(0), reserved, "lost confirm + TTL reaper converges reserved_usage to baseline")
		assert.Equal(t, string(model.StatusExpired), resReadStatus(t, db, res.ReservationIDs[0]))
	})

	// (c) The confirm WAS delivered and committed; the ledger then crashed before
	// the reaper ran. The CONFIRMED rows must stay CONFIRMED, current_usage must
	// equal exactly the sum of committed amounts, and reserved_usage must be
	// fully drained. A subsequent reaper sweep must be a no-op (no RESERVED rows).
	t.Run("confirm_delivered_then_crash_before_reaper_persists", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		db := testutil.SetupIntegrationDB(t)

		limitID := resSeedLimit(t, db, 8703, "crash-c", 10000)
		t.Cleanup(func() { resCleanupLimit(t, db, limitID) })

		scopeKey := "acct:crash-c-" + testutil.MustDeterministicUUID(8713).String()[:8]
		periodKey := "2026-06"
		ctx := context.Background()

		audit := &resCountingAudit{}

		// Two separate transactions, each reserving + confirming against the SAME
		// counter, so the committed sum is unambiguous: 300 + 250 = 550.
		var confirmedIDs []uuid.UUID

		for _, c := range []struct {
			txSeed int64
			amount int64
		}{
			{8731, 300},
			{8732, 250},
		} {
			svc := resWireService(t, db, resStubResolver{
				specs: []query.ReservationSpec{resSpec(limitID, scopeKey, periodKey, c.amount, 10000)},
			}, audit)

			res, err := svc.Reserve(ctx, testutil.MustDeterministicUUID(c.txSeed), resCheckInput(t), false)
			require.NoError(t, err)
			require.Len(t, res.ReservationIDs, 1)

			require.NoError(t, svc.Confirm(ctx, res.ReservationIDs[0]))
			confirmedIDs = append(confirmedIDs, res.ReservationIDs[0])
		}

		// After both confirms: current_usage == sum of committed, reserved drained.
		current, reserved := resReadCounter(t, db, limitID, scopeKey, periodKey)
		assert.Equal(t, int64(550), current, "current_usage must equal the sum of committed amounts")
		assert.Equal(t, int64(0), reserved, "confirm must fully drain reserved_usage")

		for _, id := range confirmedIDs {
			assert.Equal(t, string(model.StatusConfirmed), resReadStatus(t, db, id))
		}

		// --- LEDGER CRASH after the confirms, before any reaper sweep. ---
		// The reaper runs: there are NO RESERVED rows, so it is a no-op and the
		// CONFIRMED state is untouched.
		sweepAt := time.Now().UTC().Add(6 * time.Minute)
		reaper := resWireReaper(t, db, audit, sweepAt)

		released, err := reaper.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, released, "the reaper must NOT touch CONFIRMED rows")

		current, reserved = resReadCounter(t, db, limitID, scopeKey, periodKey)
		assert.Equal(t, int64(550), current, "committed usage must survive a pre-reaper crash")
		assert.Equal(t, int64(0), reserved)

		for _, id := range confirmedIDs {
			assert.Equal(t, string(model.StatusConfirmed), resReadStatus(t, db, id))
		}
	})

	// By-transaction interleaving: one ledger transaction reserves against TWO
	// limits (e.g. a daily account cap + a global cap). The confirm is lost. The
	// reaper must expire BOTH reservation rows of that transaction and drain BOTH
	// counters back to baseline — the per-transaction fan-out cannot leave one
	// limit's capacity stranded because the confirm addressed by transaction id
	// never arrived.
	t.Run("by_transaction_two_limits_lost_confirm_reaper_expires_both", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		db := testutil.SetupIntegrationDB(t)

		limitA := resSeedLimit(t, db, 8704, "crash-bytxn-A", 10000)
		limitB := resSeedLimit(t, db, 8705, "crash-bytxn-B", 10000)
		t.Cleanup(func() {
			resCleanupLimit(t, db, limitA)
			resCleanupLimit(t, db, limitB)
		})

		scopeA := "acct:bytxn-" + testutil.MustDeterministicUUID(8714).String()[:8]
		scopeB := "global-" + testutil.MustDeterministicUUID(8715).String()[:8]
		periodKey := "2026-06"
		txID := testutil.MustDeterministicUUID(8724)

		audit := &resCountingAudit{}
		svc := resWireService(t, db, resStubResolver{
			specs: []query.ReservationSpec{
				resSpec(limitA, scopeA, periodKey, 400, 10000),
				resSpec(limitB, scopeB, periodKey, 250, 10000),
			},
		}, audit)

		ctx := context.Background()

		// One transaction, two reservations, in ONE service call.
		res, err := svc.Reserve(ctx, txID, resCheckInput(t), false)
		require.NoError(t, err)
		require.Len(t, res.ReservationIDs, 2, "one reservation per counter-backed limit")

		_, rsvA := resReadCounter(t, db, limitA, scopeA, periodKey)
		_, rsvB := resReadCounter(t, db, limitB, scopeB, periodKey)
		require.Equal(t, int64(400), rsvA)
		require.Equal(t, int64(250), rsvB)

		// --- Confirm lost for the whole transaction. TTL elapses. ---
		sweepAt := time.Now().UTC().Add(6 * time.Minute)
		reaper := resWireReaper(t, db, audit, sweepAt)

		released, err := reaper.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, released, "the reaper must expire BOTH reservations of the transaction")

		// BOTH counters drained, BOTH rows EXPIRED — no limit left stranded.
		curA, rsvA := resReadCounter(t, db, limitA, scopeA, periodKey)
		curB, rsvB := resReadCounter(t, db, limitB, scopeB, periodKey)
		assert.Equal(t, int64(0), curA)
		assert.Equal(t, int64(0), rsvA, "limit A reserved_usage drained")
		assert.Equal(t, int64(0), curB)
		assert.Equal(t, int64(0), rsvB, "limit B reserved_usage drained")

		for _, id := range res.ReservationIDs {
			assert.Equal(t, string(model.StatusExpired), resReadStatus(t, db, id))
		}
	})
}

// ---------------------------------------------------------------------------
// PROOF 2 — Over-commit under concurrency (Gate 2)
//
// The reserve CTE's WHERE guard is current_usage + reserved_usage + amount <=
// maxAmount. Under N parallel reserves against capacity N-1, exactly N-1 must
// succeed and the loser must get the limit-exceeded decision — never N. The
// invariant current_usage + reserved_usage <= maxAmount must hold at every
// observation, including mid-flight.
// ---------------------------------------------------------------------------

func TestReservationOverCommit(t *testing.T) {
	// Fixed N-1 case: N goroutines each reserve 1 unit against a limit of N-1.
	t.Run("n_parallel_reserves_capacity_n_minus_one", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		db := testutil.SetupIntegrationDB(t)

		const n = 8
		const capacity = n - 1

		limitID := resSeedLimit(t, db, 8801, "overcommit-fixed", capacity)
		t.Cleanup(func() { resCleanupLimit(t, db, limitID) })

		scopeKey := "acct:overcommit-" + testutil.MustDeterministicUUID(8811).String()[:8]
		periodKey := "2026-06"

		// Each goroutine reserves 1 unit under a DISTINCT transaction id (the
		// 4-tuple idempotency grain). Distinct tx ids mean these are genuinely
		// competing reserves, not idempotent retries collapsing onto one row.
		var (
			wg        sync.WaitGroup
			allowed   atomic.Int64
			denied    atomic.Int64
			hardError atomic.Int64
			maxSeen   atomic.Int64
		)

		// invariant observer: sample current+reserved repeatedly while the
		// reserves race; it must never exceed the ceiling.
		stopObserving := make(chan struct{})

		var observerWG sync.WaitGroup

		observerWG.Add(1)

		go func() {
			defer observerWG.Done()

			for {
				select {
				case <-stopObserving:
					return
				default:
					cur, rsv := resReadCounter(t, db, limitID, scopeKey, periodKey)
					sum := cur + rsv

					for {
						prev := maxSeen.Load()
						if sum <= prev || maxSeen.CompareAndSwap(prev, sum) {
							break
						}
					}
				}
			}
		}()

		for i := range n {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				audit := &resCountingAudit{}
				svc := resWireService(t, db, resStubResolver{
					specs: []query.ReservationSpec{resSpec(limitID, scopeKey, periodKey, 1, capacity)},
				}, audit)

				txID := testutil.MustDeterministicUUID(8820 + int64(idx))

				res, err := svc.Reserve(context.Background(), txID, resCheckInput(t), false)
				switch {
				case err != nil:
					hardError.Add(1)
				case res.Denied:
					denied.Add(1)
				default:
					allowed.Add(1)
				}
			}(i)
		}

		wg.Wait()
		close(stopObserving)
		observerWG.Wait()

		require.Equal(t, int64(0), hardError.Load(), "no reserve should fail with a non-business error")
		assert.Equal(t, int64(capacity), allowed.Load(), "exactly N-1 reserves must succeed")
		assert.Equal(t, int64(1), denied.Load(), "exactly one reserve must lose with the limit-exceeded decision")

		// Final state: the counter is saturated at the ceiling, never over it.
		cur, rsv := resReadCounter(t, db, limitID, scopeKey, periodKey)
		assert.Equal(t, int64(capacity), cur+rsv, "the counter must saturate at exactly maxAmount")
		assert.LessOrEqual(t, maxSeen.Load(), int64(capacity),
			"current_usage + reserved_usage must NEVER exceed maxAmount at any observation")
	})

	// Randomized property-style variant: multiple rounds with random capacities
	// and random per-reserve amounts. Across every round the WHERE-guard invariant
	// (current + reserved <= maxAmount) must hold, and the sum of accepted amounts
	// must never exceed the ceiling.
	t.Run("randomized_property_invariant_never_breaks", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		db := testutil.SetupIntegrationDB(t)

		// Deterministic seed so a failure is reproducible (no time.Now()-based seed).
		rng := rand.New(rand.NewSource(20260605))

		const rounds = 6

		for round := range rounds {
			// Random ceiling in [20, 120) and random number of contenders.
			capacity := int64(20 + rng.Intn(100))
			contenders := 5 + rng.Intn(10)

			limitSeed := int64(8900 + round)
			limitID := resSeedLimit(t, db, limitSeed, "overcommit-rand-"+uuid.NewString()[:8], capacity)

			scopeKey := "acct:rand-" + testutil.MustDeterministicUUID(int64(8950+round)).String()[:8]
			periodKey := "2026-06"

			// Pre-compute each contender's random amount in [1, capacity] so a
			// single reserve can never be denied by the amount-alone pre-check
			// (that path is not what this proof exercises; we want genuine
			// WHERE-guard contention).
			amounts := make([]int64, contenders)
			for i := range amounts {
				amounts[i] = int64(1 + rng.Intn(int(capacity)))
			}

			var (
				wg          sync.WaitGroup
				acceptedSum atomic.Int64
				mu          sync.Mutex
				accepted    []int64
			)

			for i := range contenders {
				wg.Add(1)

				go func(idx int) {
					defer wg.Done()

					audit := &resCountingAudit{}
					svc := resWireService(t, db, resStubResolver{
						specs: []query.ReservationSpec{resSpec(limitID, scopeKey, periodKey, amounts[idx], capacity)},
					}, audit)

					txID := testutil.MustDeterministicUUID(int64(810000 + round*100 + idx))

					res, err := svc.Reserve(context.Background(), txID, resCheckInput(t), false)
					if err == nil && !res.Denied {
						acceptedSum.Add(amounts[idx])

						mu.Lock()
						accepted = append(accepted, amounts[idx])
						mu.Unlock()
					}
				}(i)
			}

			wg.Wait()

			cur, rsv := resReadCounter(t, db, limitID, scopeKey, periodKey)

			// Core invariant: the persisted counter never exceeds the ceiling.
			assert.LessOrEqualf(t, cur+rsv, capacity,
				"round %d: current+reserved (%d) must not exceed maxAmount (%d); accepted=%v",
				round, cur+rsv, capacity, accepted)

			// The held capacity equals exactly the sum of accepted reserves — no
			// lost update, no phantom hold.
			assert.Equalf(t, acceptedSum.Load(), cur+rsv,
				"round %d: held reserved_usage (%d) must equal the sum of accepted amounts (%d)",
				round, cur+rsv, acceptedSum.Load())

			resCleanupLimit(t, db, limitID)
		}
	})
}

// resErrIsExceeds keeps the over-limit sentinel referenced so a future rename of
// constant.ErrUsageCounterExceedsLimit surfaces here at compile time rather than
// silently weakening the over-commit proof's premise (the loser is denied via
// that exact guard inside ReservationService.Reserve).
var _ = constant.ErrUsageCounterExceedsLimit
