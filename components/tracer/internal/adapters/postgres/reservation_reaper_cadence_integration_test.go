// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// This file is the Gate-7 reaper-cadence PROOF (F3-T21). It exercises the REAL
// reservation reaper worker (workers.ReservationReaperWorker) over the REAL
// postgres reaper repository (postgres.ReservationReaperRepository) and the REAL
// batch-summary auditor (command.RecordAuditEventCommand) against a real
// testcontainer database. Nothing here is mocked: the only test-only seams are a
// controllable ticker clock (so the sub-minute cadence is fast and deterministic)
// and a spy connection that records whether the root pool is ever touched on the
// pool-resolution-failure path.

// sqlTxBeginner adapts a *sql.DB to pgdb.TxBeginner for the reaper repository,
// which opens its own per-row release transaction. The production wiring uses
// db.TxBeginnerAdapter over a dbresolver.DB; the integration suite connects with a
// raw *sql.DB (testutil.SetupIntegrationDB), whose BeginTx returns *sql.Tx — which
// satisfies pgdb.Tx structurally but not the TxBeginner return type, so this thin
// adapter bridges the two without touching production code.
type sqlTxBeginner struct {
	db *sql.DB
}

func (b sqlTxBeginner) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
	return b.db.BeginTx(ctx, opts)
}

// tickerClock is a clock whose Now() is fixed (so expiry comparisons are
// deterministic) but whose ticker is a real time.Ticker (so RunWithContext's
// loop actually fires within a sub-minute interval). The reaper's runLoop sweeps
// once immediately on start and then on every tick; this clock makes both happen
// against the real DB without the test calling time.Now() for its assertions.
type tickerClock struct {
	now      time.Time
	interval time.Duration
}

func (c tickerClock) Now() time.Time {
	return c.now.UTC()
}

func (c tickerClock) NewTicker(_ time.Duration) (<-chan time.Time, func()) {
	ticker := time.NewTicker(c.interval)
	return ticker.C, ticker.Stop
}

// spyConnection records whether the reaper ever resolved a database connection,
// and serves a real DB when it does. On the pool-resolution-failure path the
// reaper must SKIP the cycle before touching this connection at all — that is the
// "root pool NEVER queried" invariant. queried flips true the moment GetDB is
// called, which is the first thing the find query does.
type spyConnection struct {
	db      pgdb.DB
	queried *bool
}

func (s spyConnection) GetDB(_ context.Context) (pgdb.DB, error) {
	*s.queried = true
	return s.db, nil
}

// spyTxBeginner mirrors spyConnection for the per-row release transaction path.
type spyTxBeginner struct {
	db      *sql.DB
	queried *bool
}

func (s spyTxBeginner) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
	*s.queried = true
	return s.db.BeginTx(ctx, opts)
}

// failingPoolResolver always fails to resolve a tenant pool. The reaper must treat
// this as a cycle-level failure and skip, never falling back to the root pool.
type failingPoolResolver struct{}

func (failingPoolResolver) GetTenantDB(_ context.Context, _ string) (dbresolver.DB, error) {
	return nil, errors.New("tenant pool unavailable")
}

// fixedReaperNow is the deterministic "now" the reaper clock reports. Seeded
// reservation_expires_at values are placed strictly before this for expired rows
// and far after it for fresh rows, so the find query's predicate is unambiguous
// regardless of wall-clock time during the run.
func fixedReaperNow() time.Time {
	return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
}

// newRealReaper wires the real worker + real postgres reaper repository + real
// auditor over the real testcontainer DB. clk drives the cadence; tenantID and
// poolResolver select single-tenant vs MT behaviour.
func newRealReaper(
	t *testing.T,
	db *sql.DB,
	conn pgdb.Connection,
	txb pgdb.TxBeginner,
	clk *tickerClock,
	tenantID string,
	resolver workers.WorkerPoolResolver,
) *workers.ReservationReaperWorker {
	t.Helper()

	counterRepo := NewUsageCounterRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	resRepo := NewUsageReservationRepositoryWithConnection(counterRepo)
	reaperRepo := NewReservationReaperRepository(conn, txb, resRepo)

	auditRepo := NewAuditEventRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	auditor := command.NewRecordAuditEventCommand(auditRepo)

	config := workers.ReservationReaperWorkerConfig{ReapInterval: clk.interval}

	worker, err := workers.NewReservationReaperWorkerWithPoolResolver(
		reaperRepo, auditor, config, testutil.NewMockLogger(), clk, tenantID, resolver,
	)
	require.NoError(t, err)

	return worker
}

// countExpiryAuditRows returns the number of batch-summary expiry audit rows
// written for the sweep window. The reaper writes exactly ONE row per non-empty
// sweep (RESERVATION_EXPIRED / resource_type='reservation'), regardless of how
// many reservations expired (Q11). We scope the count to a sweptAt context value
// unique to this test so a shared DB across tests cannot inflate it.
func countExpiryAuditRows(t *testing.T, db *sql.DB, sweptAt time.Time) int {
	t.Helper()

	var n int

	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM audit_events
		WHERE event_type = 'RESERVATION_EXPIRED'
		  AND resource_type = 'reservation'
		  AND context->>'sweptAt' = $1
	`, sweptAt.UTC().Format(time.RFC3339Nano)).Scan(&n)
	require.NoError(t, err, "failed to count expiry audit rows")

	return n
}

// TestIntegration_ReservationReaperCadence_ReleasesExpiredWithinInterval is the
// core of Gate 7. It seeds one EXPIRED RESERVED row and one FRESH (non-expired)
// RESERVED row, runs the REAL reaper at a sub-minute (200ms) cadence, and asserts:
//   - the expired row flips to EXPIRED within the interval window,
//   - its held amount is returned to the counter (reserved_usage decremented),
//   - the fresh row is left strictly untouched (still RESERVED, still holding),
//   - exactly ONE batch-summary audit row is written for the sweep.
func TestIntegration_ReservationReaperCadence_ReleasesExpiredWithinInterval(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	limitID := createTestLimit(t, db, 9701)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:9701-" + testutil.MustDeterministicUUID(9711).String()[:8]
	periodKey := "2026-06"
	freshScopeKey := "acct:9701-fresh-" + testutil.MustDeterministicUUID(9712).String()[:8]

	ctx := context.Background()

	now := fixedReaperNow()

	// Reserve via the real repository so reserved_usage is genuinely held on the
	// counter (the reaper must return it). Expired row: TTL strictly before the
	// reaper's now. Fresh row: TTL far after now (the reaper must never touch it).
	expired, err := model.NewReservation(
		limitID, testutil.MustDeterministicUUID(9721), scopeKey, periodKey, 400,
		now.Add(-10*time.Minute), now.Add(-15*time.Minute),
	)
	require.NoError(t, err)

	fresh, err := model.NewReservation(
		limitID, testutil.MustDeterministicUUID(9722), freshScopeKey, periodKey, 250,
		now.Add(1*time.Hour), now,
	)
	require.NoError(t, err)

	resRepo := newReservationRepoIntegration(db)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return resRepo.ReserveWithTx(ctx, tx, expired, 10000)
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return resRepo.ReserveWithTx(ctx, tx, fresh, 10000)
	}))

	// Sanity: both rows are RESERVED and holding their amounts before the sweep.
	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current, "expired row must not touch current_usage on reserve")
	assert.Equal(t, int64(400), reserved, "expired row must hold its amount before the sweep")
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, expired.ID))

	freshCurrent, freshReserved := readCounter(t, db, limitID, freshScopeKey, periodKey)
	assert.Equal(t, int64(0), freshCurrent)
	assert.Equal(t, int64(250), freshReserved, "fresh row holds its amount before the sweep")
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, fresh.ID))

	// Real worker, real repo, real auditor, real DB. Single-tenant (no pool
	// resolver): the reaper reads via the static connection. 200ms cadence proves
	// the sub-minute requirement without a 30s wait; runLoop sweeps once on start.
	interval := 200 * time.Millisecond
	clk := &tickerClock{now: now, interval: interval}
	conn := &testutil.IntegrationDBAdapter{DB: db}
	txb := sqlTxBeginner{db: db}
	worker := newRealReaper(t, db, conn, txb, clk, "", nil)

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)

	go func() { done <- worker.RunWithContext(runCtx) }()

	// The expired row must flip to EXPIRED within a small multiple of the interval
	// (the immediate start-up sweep alone should do it). Poll the row status with a
	// deadline well under a minute to assert "released within the interval".
	require.Eventually(t, func() bool {
		return readReservationStatus(t, db, expired.ID) == string(model.StatusExpired)
	}, 5*time.Second, 20*time.Millisecond, "expired reservation must be released within the sub-minute cadence")

	cancel()
	require.NoError(t, <-done, "reaper loop must stop cleanly on context cancel")

	// Expired row: released as EXPIRED, its hold returned to the counter.
	current, reserved = readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current, "release must NOT credit current_usage")
	assert.Equal(t, int64(0), reserved, "release must decrement reserved_usage back to zero")
	assert.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, expired.ID))

	// Fresh row: strictly untouched — still RESERVED, still holding its amount.
	freshCurrent, freshReserved = readCounter(t, db, limitID, freshScopeKey, periodKey)
	assert.Equal(t, int64(0), freshCurrent, "fresh row's counter must be untouched")
	assert.Equal(t, int64(250), freshReserved, "fresh row must keep holding its amount")
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, fresh.ID),
		"a non-expired reservation must survive the sweep")

	// Exactly ONE batch-summary audit row for the sweep, scoped to this run's now.
	assert.Equal(t, 1, countExpiryAuditRows(t, db, now),
		"a non-empty sweep writes exactly one batch-summary audit row (Q11)")
}

// TestIntegration_ReservationReaperCadence_SkipsCycleOnPoolFailure proves the
// MT isolation invariant (R22 / Gate 7): when the tenant pool cannot be resolved,
// the reaper SKIPS the cycle and NEVER touches the root pool. We seed an expired
// row on the (root) testcontainer DB, run the reaper in MT mode with a failing
// pool resolver, and assert:
//   - the spy connection / tx-beginner are NEVER queried (root pool untouched),
//   - the expired row stays RESERVED (the reaper did not reap it on the wrong DB),
//   - no batch-summary audit row is written.
func TestIntegration_ReservationReaperCadence_SkipsCycleOnPoolFailure(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	limitID := createTestLimit(t, db, 9801)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:9801-" + testutil.MustDeterministicUUID(9811).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	// A distinct "now" from the release test: the suite shares one testcontainer DB,
	// and countExpiryAuditRows scopes to context->>'sweptAt'. A different sweep
	// timestamp keeps this test's audit-row count independent of any other test's.
	now := fixedReaperNow().Add(1 * time.Hour)

	// An expired row sitting on the root DB. If the reaper wrongly fell back to the
	// root pool, it would reap this; the invariant is that it must not.
	expired, err := model.NewReservation(
		limitID, testutil.MustDeterministicUUID(9821), scopeKey, periodKey, 400,
		now.Add(-10*time.Minute), now.Add(-15*time.Minute),
	)
	require.NoError(t, err)

	resRepo := newReservationRepoIntegration(db)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return resRepo.ReserveWithTx(ctx, tx, expired, 10000)
	}))

	// Spy connection + tx-beginner wrap the root DB and record any access. The
	// reaper must short-circuit at pool resolution BEFORE reaching either.
	connQueried := false
	txQueried := false
	conn := spyConnection{db: db, queried: &connQueried}
	txb := spyTxBeginner{db: db, queried: &txQueried}

	interval := 200 * time.Millisecond
	clk := &tickerClock{now: now, interval: interval}

	// MT mode (non-empty tenantID) + failing pool resolver: every cycle must skip.
	worker := newRealReaper(t, db, conn, txb, clk, "tenant-x", failingPoolResolver{})

	runCtx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
	defer cancel()

	// Let the loop run through its immediate start-up cycle plus at least one tick.
	require.NoError(t, worker.RunWithContext(runCtx),
		"reaper loop must stop cleanly when the deadline elapses")

	// Root pool NEVER queried: neither the find connection nor the release tx was
	// ever resolved, because the cycle skipped at pool resolution.
	assert.False(t, connQueried, "reaper must NOT query the root connection when the tenant pool fails to resolve")
	assert.False(t, txQueried, "reaper must NOT open a root-pool transaction when the tenant pool fails to resolve")

	// The expired row on the root DB is untouched — still RESERVED, still holding.
	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current)
	assert.Equal(t, int64(400), reserved, "a skipped cycle must not reap rows on the root DB")
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, expired.ID))

	// No batch-summary audit row: a skipped cycle audits nothing.
	assert.Equal(t, 0, countExpiryAuditRows(t, db, now),
		"a skipped cycle writes no batch-summary audit row")
}
