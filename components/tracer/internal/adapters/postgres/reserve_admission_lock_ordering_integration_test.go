// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// Lock-ordering proof for contextual reserve admission.
//
// A reserve locks its operation, its accounts' advisory locks, several
// usage_counters rows (one per limit and scope) and, through the audit
// BEFORE-INSERT trigger, the global audit advisory lock. Two concurrent reserves
// that enter overlapping counter rows in different relative order form a
// wait-for cycle, which PostgreSQL breaks by aborting one with SQLSTATE 40P01.
// The admission path takes the sorted account locks before touching any counter
// row and reserves in counter-coordinate order, so same-account reserves
// serialize instead of interleaving.
//
// The discriminating signal is pg_stat_database.deadlocks: PostgreSQL counts a
// deadlock the moment it detects one, whatever the caller then observes.

// readDeadlockCount reads PostgreSQL's cumulative deadlock counter for the
// current database.
func readDeadlockCount(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var deadlocks int64

	require.NoError(t, db.QueryRowContext(t.Context(),
		"SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()").Scan(&deadlocks))

	return deadlocks
}

// settledDeadlockCount polls the deadlock counter until it moves past before or
// a bounded window closes. The statistics can lag the racing window, so a
// single immediate read could turn a real cycle into a false pass.
func settledDeadlockCount(t *testing.T, db *sql.DB, before int64) int64 {
	t.Helper()

	settleCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	after := readDeadlockCount(t, db)
	for after == before && settleCtx.Err() == nil {
		time.Sleep(25 * time.Millisecond)
		after = readDeadlockCount(t, db)
	}

	return after
}

// TestIntegrationReserveAdmissionConcurrentSameAccountsNoDeadlock races
// reserves that debit the SAME two accounts under three limits, each scoped to
// both accounts. Even workers list the debits [X, Y], odd workers [Y, X], so any
// ordering that followed the request would enter the shared counter rows in
// opposite orders. After the race no deadlock is counted, every reserve is
// allowed, and every counter row holds exactly the sum of its debits.
func TestIntegrationReserveAdmissionConcurrentSameAccountsNoDeadlock(t *testing.T) {
	const (
		workers = 16
		limits  = 3
	)

	db := completionDatabase(t)
	db.SetMaxOpenConns(workers)
	db.SetMaxIdleConns(workers)

	admission, _, base := admissionFixture(t, db)
	base.ValidationMode = tracercontract.ValidationLimits

	x, y := testutil.MustDeterministicUUID(89951), testutil.MustDeterministicUUID(89952)
	debitX := tracercontract.Entry{AccountID: x, Amount: "1", Asset: base.Asset}
	debitY := tracercontract.Entry{AccountID: y, Amount: "1", Asset: base.Asset}

	limitIDs := make([]uuid.UUID, limits)
	for i := range limitIDs {
		limitIDs[i] = admissionLimit(t, db, withReserveDebits(base, 89953, debitX), 89960+int64(i), "1000000",
			model.Scope{AccountID: &x}, model.Scope{AccountID: &y})
	}

	// Build every request on the test goroutine: require's FailNow is only valid there.
	requests := make([]tracercontract.ReserveRequest, workers)
	for i := range requests {
		debits := []tracercontract.Entry{debitX, debitY}
		if i%2 == 1 {
			debits = []tracercontract.Entry{debitY, debitX}
		}

		requests[i] = withReserveDebits(base, 89970+int64(2*i), debits...)
	}

	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), time.Minute)
	defer cancel()

	type outcome struct {
		result *tracercontract.ReserveResult
		err    error
	}

	before := readDeadlockCount(t, db)

	start := make(chan struct{})
	results := make(chan outcome, workers)

	for i := range workers {
		go func() {
			<-start

			result, err := admission.Execute(ctx, requests[i])
			results <- outcome{result, err}
		}()
	}

	close(start)

	for range workers {
		got := <-results
		require.NoError(t, got.err, "no reserve may surface a technical error")
		require.Equal(t, tracercontract.DecisionAllow, got.result.Decision, "capacity is generous")
		require.Len(t, got.result.ReservationIDs, 2*limits, "one reservation per limit and debited account")
	}

	after := settledDeadlockCount(t, db, before)
	require.Equal(t, before, after, "no PostgreSQL deadlock (40P01) may form between same-account reserves")

	period := base.TransactionTimestamp.Format("2006-01-02")
	for _, limitID := range limitIDs {
		for _, account := range []uuid.UUID{x, y} {
			used, held := readCounterDecimal(t, db, limitID, "acct:"+account.String(), period)
			require.True(t, used.IsZero())
			require.Equal(t, int64(workers), held.IntPart(), "every held debit is counted exactly once")
			require.True(t, held.IsInteger())
		}
	}
}
