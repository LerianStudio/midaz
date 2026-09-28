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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// This file is the reaper-cadence proof. It exercises the REAL reservation
// reaper worker (workers.ReservationReaperWorker) over the REAL postgres reaper
// repository and the REAL operation expirer against a real tenant database.
// Nothing here is mocked: the only test-only seams are a controllable ticker
// clock (so the sub-minute cadence is fast and deterministic) and a spy
// connection that records whether the root pool is ever touched on the
// pool-resolution-failure path.

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

// failingPoolResolver always fails to resolve a tenant pool. The reaper must treat
// this as a cycle-level failure and skip, never falling back to the root pool.
type failingPoolResolver struct{}

func (failingPoolResolver) GetTenantDB(_ context.Context, _ string) (dbresolver.DB, error) {
	return nil, errors.New("tenant pool unavailable")
}

// fixedReaperNow is the deterministic "now" the reaper clock reports. Expired
// reservations are admitted at testutil.FixedTime(), so their 1s TTL lies
// strictly before it, and fresh ones at it, so their TTL lies after it. The find
// predicate stays unambiguous regardless of wall-clock time during the run.
func fixedReaperNow() time.Time {
	return testutil.FixedTime().Add(15 * time.Minute)
}

// reaperAdmission admits one ALLOW decision holding 10.125 against a fresh limit
// and returns its single reservation with the counter bucket it holds.
type reaperAdmission struct {
	reservationID uuid.UUID
	operation     model.ReserveOperationIdentity
	limitID       uuid.UUID
	scopeKey      string
	periodKey     string
}

func admitForReaper(t *testing.T, db *sql.DB, conn pgdb.Connection, admittedAt time.Time, asset string, seed int64) reaperAdmission {
	t.Helper()

	admission, _, base := admissionFixtureWithLifetime(t, db, conn, admittedAt, true, 10, time.Second, 720*time.Hour)
	r := withReserveAsset(base, asset, seed)
	r.ValidationMode = tracercontract.ValidationLimits
	limitID := admissionLimit(t, db, r, seed+2, "100")

	admitted, err := admission.Execute(completionContext(t.Context(), "producer"), r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, admitted.Decision)
	require.Len(t, admitted.ReservationIDs, 1)

	a := reaperAdmission{
		reservationID: admitted.ReservationIDs[0],
		operation:     model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: r.TransactionID},
		limitID:       limitID,
	}
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT scope_key, period_key FROM usage_reservations WHERE id=$1", a.reservationID).
		Scan(&a.scopeKey, &a.periodKey))

	return a
}

// newRealReaper wires the real worker + real postgres reaper repository + real
// operation expirer over the tenant database. clk drives the cadence; tenantID
// and poolResolver select single-tenant vs MT behaviour.
func newRealReaper(
	t *testing.T,
	db *sql.DB,
	conn pgdb.Connection,
	clk *tickerClock,
	tenantID string,
	resolver workers.WorkerPoolResolver,
) *workers.ReservationReaperWorker {
	t.Helper()

	config := workers.ReservationReaperWorkerConfig{ReapInterval: clk.interval, BatchSize: workers.DefaultReservationReaperBatchSize}

	worker, err := workers.NewReservationReaperWorkerWithPoolResolver(
		NewReservationReaperRepository(conn), expireCommand(t, db), config, testutil.NewMockLogger(), clk, tenantID, resolver,
	)
	require.NoError(t, err)

	return worker
}

// TestIntegration_ReservationReaperCadence_ReleasesExpiredWithinInterval admits
// one decision whose reservation TTL elapsed and one that is still fresh, runs
// the REAL reaper at a sub-minute (200ms) cadence, and asserts:
//   - the expired reservation flips to EXPIRED within the interval window,
//   - its held amount is returned to the counter (reserved_usage decremented),
//   - its operation is EXPIRED and audited exactly once,
//   - the fresh reservation is left strictly untouched (still RESERVED, still
//     holding).
func TestIntegration_ReservationReaperCadence_ReleasesExpiredWithinInterval(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := completionDatabase(t)
	conn := &testutil.IntegrationDBAdapter{DB: db}

	now := fixedReaperNow()

	expired := admitForReaper(t, db, conn, testutil.FixedTime(), "USD", 97021)
	fresh := admitForReaper(t, db, conn, now, "EUR", 97031)

	current, reserved := readCounterDecimal(t, db, expired.limitID, expired.scopeKey, expired.periodKey)
	assert.True(t, current.IsZero(), "a hold must not touch current_usage")
	assert.Equal(t, "10.125", reserved.String(), "the expired reservation holds its amount before the sweep")

	interval := 200 * time.Millisecond
	clk := &tickerClock{now: now, interval: interval}
	worker := newRealReaper(t, db, conn, clk, "", nil)

	runCtx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan error, 1)

	go func() { done <- worker.RunWithContext(runCtx) }()

	// The operation's expiry flips the row and audits the operation in one
	// transaction, so observing the audit event observes the whole sweep.
	require.Eventually(t, func() bool {
		return len(operationEvents(t, db, expired.operation.TransactionID, model.AuditEventOperationExpired)) == 1
	}, 5*time.Second, 20*time.Millisecond, "expired reservation must be released within the sub-minute cadence")

	cancel()
	require.NoError(t, <-done, "reaper loop must stop cleanly on context cancel")

	current, reserved = readCounterDecimal(t, db, expired.limitID, expired.scopeKey, expired.periodKey)
	assert.True(t, current.IsZero(), "expiry must NOT credit current_usage")
	assert.True(t, reserved.IsZero(), "expiry must decrement reserved_usage back to zero")
	assert.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, expired.reservationID))
	status, _ := readOperationState(t, db, expired.operation)
	assert.Equal(t, string(model.OperationExpired), status)

	freshCurrent, freshReserved := readCounterDecimal(t, db, fresh.limitID, fresh.scopeKey, fresh.periodKey)
	assert.True(t, freshCurrent.IsZero(), "fresh reservation's counter must be untouched")
	assert.Equal(t, "10.125", freshReserved.String(), "fresh reservation must keep holding its amount")
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, fresh.reservationID),
		"a non-expired reservation must survive the sweep")
	assert.Empty(t, operationEvents(t, db, fresh.operation.TransactionID, model.AuditEventOperationExpired))
}

// TestIntegration_ReservationReaperCadence_SkipsCycleOnPoolFailure proves the MT
// isolation invariant: when the tenant pool cannot be resolved, the reaper SKIPS
// the cycle and NEVER touches the root pool. An expired reservation sits on the
// database behind a spy connection; the reaper runs in MT mode with a failing
// pool resolver, and the test asserts:
//   - the spy connection is NEVER queried (root pool untouched),
//   - the expired reservation stays RESERVED and keeps holding,
//   - its operation is not expired.
func TestIntegration_ReservationReaperCadence_SkipsCycleOnPoolFailure(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := completionDatabase(t)
	now := fixedReaperNow()

	expired := admitForReaper(t, db, &testutil.IntegrationDBAdapter{DB: db}, testutil.FixedTime(), "USD", 98021)

	connQueried := false
	conn := spyConnection{db: db, queried: &connQueried}

	interval := 200 * time.Millisecond
	clk := &tickerClock{now: now, interval: interval}

	// MT mode (non-empty tenantID) + failing pool resolver: every cycle must skip.
	worker := newRealReaper(t, db, conn, clk, "tenant-x", failingPoolResolver{})

	runCtx, cancel := context.WithTimeout(t.Context(), 700*time.Millisecond)
	defer cancel()

	// Let the loop run through its immediate start-up cycle plus at least one tick.
	require.NoError(t, worker.RunWithContext(runCtx),
		"reaper loop must stop cleanly when the deadline elapses")

	assert.False(t, connQueried, "reaper must NOT query the root connection when the tenant pool fails to resolve")

	current, reserved := readCounterDecimal(t, db, expired.limitID, expired.scopeKey, expired.periodKey)
	assert.True(t, current.IsZero())
	assert.Equal(t, "10.125", reserved.String(), "a skipped cycle must not reap rows on the root DB")
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, expired.reservationID))
	assert.Empty(t, operationEvents(t, db, expired.operation.TransactionID, model.AuditEventOperationExpired),
		"a skipped cycle expires no operation")
}
