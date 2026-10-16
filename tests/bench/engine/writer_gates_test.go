//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Each concurrent gate repeats on a reset stack; its expectations hold whatever the schedule.
const gateRounds, gateWorkers, gateMaxBatch = 5, 32, 256

func TestOwnerlessWriterIdentity(t *testing.T) { identityGate(t, false) }

func TestOwnerWriterIdentity(t *testing.T) { identityGate(t, true) }

// Copies and divergent payloads of each identity race over the writer: whichever payload claims the identity decides it
// once and fixes its money, its copies replay, other payloads conflict. Retried past its cutoff under a new response
// deadline, a winning payload replays without moving money.
func identityGate(t *testing.T, owner bool) {
	t.Helper()

	const identities, copies = 128, 4

	for round := range gateRounds {
		t.Run(fmt.Sprintf("round%d", round), func(t *testing.T) {
			g := newGate(t, owner)
			hot := mv(classHot, 0, constant.DefaultBalanceKey)
			reqs := make([]request, 0, identities*(copies+1))

			for i := range identities {
				name := fmt.Sprintf("identity/%d", i)
				reqs = append(reqs, slices.Repeat([]request{batchReq(t, name, name, g.clock.now(), hot(-1), ordinaryOf(i)(1))}, copies)...)
				reqs = append(reqs, batchReq(t, name, name+"/divergent", g.clock.now(), ordinaryOf(i+1)(-2), hot(2)))
			}

			w, inner := g.concurrent(t, "bench-gate-identity", reqs)

			i := slices.IndexFunc(w.calls, func(c call) bool { return c.req.id == reqs[0].id && c.res.outcome == outcomeApproved })
			require.GreaterOrEqual(t, i, 0, "identity has no approved answer")

			retry := w.calls[i].req

			g.clock.advance(usualBudget.caller + time.Second) // past its cutoff and its original response deadline
			retry.deadline = g.clock.now().Add(usualBudget.caller)

			assert.Equal(t, []string{"replay/approved+tx"}, summarize(t, []request{retry}, []result{w.Submit(g.ctx, retry)}))
			assert.EqualValues(t, 1, inner.entryLate.Load())

			got := g.finish(t, w)
			won := (got["replay/approved"] - 1) / (copies - 1) // identities their first payload won; the retry adds one replay
			assert.Equal(t, map[string]int{
				"approved": identities, "replay/approved": won*(copies-1) + 1, "conflict": won + copies*(identities-won),
			}, got)
			t.Logf("first payloads won %d of %d identities", won, identities)
		})
	}

	// Sequential, so once: a spent credit's replay, queued ahead of a new debit of the same amount in one batch, pays
	// nothing again, so that debit finds the recipient empty. An owner finds it so in the cache the spend published.
	t.Run("spent-credit", func(t *testing.T) {
		g := newGate(t, owner)
		recipient := mv(classOrdinary, 2, constant.DefaultBalanceKey)
		g.exec(t, `UPDATE balance SET available = 0 WHERE id = $1`, recipient(0).BalanceID)

		credit := batchReq(t, "identity/credit", "identity/credit", g.clock.now(), ordinaryOf(0)(-50), recipient(50))
		spend := batchReq(t, "identity/spend", "identity/spend", g.clock.now(), recipient(-50), ordinaryOf(1)(50))
		respend := batchReq(t, "identity/respend", "identity/respend", g.clock.now(), recipient(-50), ordinaryOf(1)(50))

		const app = "bench-gate-credit"

		w, inner := g.start(t, app, 1, 8, 4)
		assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{credit}, []result{w.Submit(g.ctx, credit)}))

		row := holdLock(g.ctx, t, g.env, lockBalance, ordinaryOf(1)(0).BalanceID)
		spending := submitEach(g.ctx, t, w, []request{spend})
		awaitBackend(g.ctx, t, g.conn, app+"-0", "Lock")

		replaying := submitEach(g.ctx, t, w, []request{credit})
		awaitQueued(t, inner, 1)
		respending := submitEach(g.ctx, t, w, []request{respend})
		awaitQueued(t, inner, 2)
		require.NoError(t, row.Rollback(g.ctx))

		assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{spend}, spending.wait(t)))
		assert.Equal(t, []string{"replay/approved+tx"}, summarize(t, []request{credit}, replaying.wait(t)))
		assert.Equal(t, []string{"refused/insufficient_funds"}, summarize(t, []request{respend}, respending.wait(t)))
		g.finish(t, w)
		assert.Equal(t, 3, inner.workers[0].batches, "the replay and the new debit shared the third batch")
	})
}

func TestOwnerlessWriterMultiKey(t *testing.T) { multiKeyGate(t, false) }

func TestOwnerWriterMultiKey(t *testing.T) { multiKeyGate(t, true) }

// Both seeded keys of one account debit its one control: 300 debits of 10, from either key or 5 from each, meet a shared
// limit of 1000, so exactly 100 approve whatever their order. The control's version counts debit postings.
func multiKeyGate(t *testing.T, owner bool) {
	t.Helper()

	for round := range gateRounds {
		t.Run(fmt.Sprintf("round%d", round), func(t *testing.T) {
			g := newGate(t, owner)
			g.exec(t, `UPDATE account_control SET limit_remaining = 1000 WHERE account_id = $1`, accountID(classMultikey, 0))

			mk, mk2 := mv(classMultikey, 0, constant.DefaultBalanceKey), mv(classMultikey, 0, keySecondary)
			debits := [][]posting{{mk(-10)}, {mk2(-10)}, {mk(-5), mk2(-5)}}
			reqs := make([]request, 300)

			for i := range reqs {
				name := fmt.Sprintf("multikey/%d", i)
				reqs[i] = batchReq(t, name, name, g.clock.now(), slices.Concat(debits[i%3], []posting{ordinaryOf(i)(10)})...)
			}

			w, _ := g.concurrent(t, "bench-gate-multikey", reqs)
			assert.Equal(t, map[string]int{"approved": 100, "refused/limit_exceeded": 200}, g.finish(t, w))
		})
	}
}

func TestOwnerlessWriterProtection(t *testing.T) { protectionGate(t, false) }

func TestOwnerWriterProtection(t *testing.T) { protectionGate(t, true) }

// Funds and flags refuse whatever the schedule: 200 debits of 10 against 1000 on the hot balance, and 20 each from a
// sending-disabled balance, to a receiving-disabled one after a valid debit, and from a blocked account. The other legs
// stay rich, so each refusal has one reason, and a refused credit leaves its earlier debit unwritten.
func protectionGate(t *testing.T, owner bool) {
	t.Helper()

	for round := range gateRounds {
		t.Run(fmt.Sprintf("round%d", round), func(t *testing.T) {
			g := newGate(t, owner)
			hot, fee := mv(classHot, 0, constant.DefaultBalanceKey), mv(classFee, 0, constant.DefaultBalanceKey)
			mk := mv(classMultikey, 0, constant.DefaultBalanceKey)

			g.exec(t, `UPDATE balance SET available = 1000 WHERE id = $1`, hot(0).BalanceID)
			g.exec(t, `UPDATE balance SET allow_sending = false WHERE id = $1`, ordinaryOf(3)(0).BalanceID)
			g.exec(t, `UPDATE balance SET allow_receiving = false WHERE id = $1`, fee(0).BalanceID)
			g.exec(t, `UPDATE account_control SET blocked = true WHERE account_id = $1`, accountID(classMultikey, 0))

			legs := [][]posting{{ordinaryOf(3)(-1), ordinaryOf(0)(1)}, {ordinaryOf(1)(-1), fee(1)}, {mk(-1), ordinaryOf(2)(1)}}
			reqs := make([]request, 0, 260)

			for i := range 260 {
				name := fmt.Sprintf("protection/%d", i)
				postings := []posting{hot(-10), ordinaryOf(i)(10)}

				if i >= 200 {
					postings = legs[i%3]
				}

				reqs = append(reqs, batchReq(t, name, name, g.clock.now(), postings...))
			}

			w, _ := g.concurrent(t, "bench-gate-protection", reqs)
			assert.Equal(t, map[string]int{
				"approved": 100, "refused/insufficient_funds": 100,
				"refused/sending_disabled": 20, "refused/receiving_disabled": 20, "refused/blocked": 20,
			}, g.finish(t, w))
		})
	}
}

func TestOwnerlessWriterAdmission(t *testing.T) { admissionGate(t, false) }

func TestOwnerWriterAdmission(t *testing.T) { admissionGate(t, true) }

// A new refusal crossing its cutoff before COMMIT voids the pass as an approval does: the rerun approves afresh and
// expires the refusal, and the void pass leaves nothing in the ledger or an owner's cache. The busy refusal parks the
// one worker on its own claim held in flight, since an owner refuses from its cache without touching a balance row.
func admissionGate(t *testing.T, owner bool) {
	t.Helper()

	for round := range gateRounds {
		t.Run(fmt.Sprintf("round%d", round), func(t *testing.T) {
			g := newGate(t, owner)
			from := mv(classOrdinary, 3, constant.DefaultBalanceKey)
			g.exec(t, `UPDATE account_control SET blocked = true WHERE account_id = $1`, accountID(classOrdinary, 3))

			busy := batchReq(t, "admission/busy", "admission/busy", g.clock.now(), from(-1), ordinaryOf(0)(1))
			short, err := newRequest(uuid.NewSHA1(benchNS, []byte("gate/admission/short")), uuid.NewSHA1(benchNS, []byte("gate/tx/admission/short")),
				[]posting{from(-1), ordinaryOf(2)(1)}, g.clock.now(), budget{caller: usualBudget.caller, admission: time.Second})
			require.NoError(t, err)

			queued := []request{
				batchReq(t, "admission/usual", "admission/usual", g.clock.now(), mv(classHot, 0, constant.DefaultBalanceKey)(-1), ordinaryOf(1)(1)),
				short,
			}

			const app = "bench-gate-admission"

			claim := holdLock(g.ctx, t, g.env, claimSQL, benchTenantID, []string{busy.id.String()}, [][]byte{busy.fingerprint[:]},
				[]time.Time{busy.admissionCutoff})
			w, inner := g.start(t, app, 1, 8, 1+len(queued))
			first := submitEach(g.ctx, t, w, []request{busy})

			awaitBackend(g.ctx, t, g.conn, app+"-0", "Lock")

			rest := submitEach(g.ctx, t, w, queued)
			awaitQueued(t, inner, len(queued))

			outbox := holdLock(g.ctx, t, g.env, `LOCK TABLE outbox_events IN SHARE MODE`)
			require.NoError(t, claim.Rollback(g.ctx))
			awaitBackend(g.ctx, t, g.conn, app+"-0", "relation") // the queued pass decided both and writes the approval's events
			g.clock.advance(time.Second)
			require.NoError(t, outbox.Rollback(g.ctx))

			assert.Equal(t, []string{"refused/blocked"}, summarize(t, []request{busy}, first.wait(t)))
			assert.Equal(t, []string{"approved+tx", "expired"}, summarize(t, queued, rest.wait(t)))
			assert.Equal(t, map[string]int{"refused/blocked": 1, "approved": 1, "expired": 1}, g.finish(t, w))
			assert.Equal(t, workerStats{batches: 2, batchStats: batchStats{passes: 3}}, inner.workers[0], "the void pass reran once")
		})
	}
}

func TestOwnerlessWriterConservation(t *testing.T) { conservationGate(t, false) }

func TestOwnerWriterConservation(t *testing.T) { conservationGate(t, true) }

// Each shape is one reset round of 200k closed-loop requests on the process clock: every one approves, and the ledger,
// baseline, outbox and each answer reconcile after a drained Close. The writer lives on the test context; the run has
// 5 min and Close a fresh 30 s. Each request keeps usualBudget. Volume runs without race.
func conservationGate(t *testing.T, owner bool) {
	t.Helper()

	const n, producers = 200_000, 256

	for _, round := range []struct {
		name string
		mix  mix
		fee  bool
	}{
		{"outflow", mixOutflow, false},
		{"inflow", mixInflow, false},
		{"mixed", mixMixed, false},
		{"outflow_fee", mixOutflow, true},
		{"mixed_fee", mixMixed, true},
	} {
		t.Run(round.name, func(t *testing.T) {
			noLeaks(t)

			env, ctx := requireDB(t), t.Context()
			wl := newTestWorkload(t, "conservation/"+round.name, round.mix, round.fee, 4)

			setupAt := wl.now()
			b, conn, broker := newTestBatcher(ctx, t, env, wl.now)
			snapshotBaseline(ctx, t, conn)

			var inner *ownerlessWriter

			// The owner's constructor is bounded only by the command timeouts.
			if owner {
				var err error

				inner, err = startOwner(ctx, t, namedBatcher(t, b, env, "bench-conserve-0", nil), "bench-conserve", gateMaxBatch).wait(t)
				require.NoError(t, err)
			} else {
				inner = newTestOwnerless(ctx, t, env, b, "bench-conserve", gateWorkers, gateMaxBatch)
			}

			w, rec := captureCalls(inner, n), newRecorder()

			work, stopWork := context.WithTimeout(ctx, 5*time.Minute)
			runAt := wl.now()
			runErr := runClosed(work, w, wl, producers, n, usualBudget, rec)

			stopWork()

			closing, stopClose := context.WithTimeout(ctx, 30*time.Second)
			closeAt := wl.now()
			closeErr := w.Close(closing)
			closedAt := wl.now()

			stopClose()

			var batches, passes, deadlocks int

			for _, s := range inner.workers {
				batches, passes, deadlocks = batches+s.batches, passes+s.passes, deadlocks+s.deadlocks
			}

			t.Logf("setup=%s run=%s close=%s runErr=%v closeErr=%v entryLate=%d publishes=%d batches=%d passes=%d deadlocks=%d",
				runAt.Sub(setupAt), closeAt.Sub(runAt), closedAt.Sub(closeAt), runErr, closeErr, inner.entryLate.Load(),
				broker.publishes.Load(), batches, passes, deadlocks)
			t.Logf("per worker: %+v", inner.workers)
			t.Logf("local closed-loop correctness run, not a capacity benchmark: %s", rec.summary())
			require.NoError(t, closeErr, "only a drained writer is verified: an aborted backend may still commit")

			checkAt := wl.now()
			got := verifyLedger(ctx, t, conn, w.calls)
			t.Logf("checker=%s answers=%v", wl.now().Sub(checkAt), got)

			assert.NoError(t, runErr)
			assert.Zero(t, inner.entryLate.Load(), "attempts late at entry")
			assert.Zero(t, broker.publishes.Load(), "obligations stay in the outbox")
			assert.Equal(t, map[string]int{string(outcomeApproved): n}, got)
		})
	}
}

// gate is one reset round: the stack reseeded with 4 ordinary balances, a fixed clock, the connection that owns
// fixtures, the baseline and the checks, and whether an owner writes.
type gate struct {
	ctx    context.Context
	env    *benchEnv
	clock  *testClock
	b      *batcher
	conn   *pgx.Conn
	broker *refusingBroker
	owner  bool
}

func newGate(t *testing.T, owner bool) *gate {
	t.Helper()
	noLeaks(t)

	env := requireDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	t.Cleanup(cancel)

	clock := newTestClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	b, conn, broker := newTestBatcher(ctx, t, env, clock.now)

	return &gate{ctx: ctx, env: env, clock: clock, b: b, conn: conn, broker: broker, owner: owner}
}

// exec applies one fixture row change before the baseline.
func (g *gate) exec(t *testing.T, query string, id uuid.UUID) {
	t.Helper()

	_, err := g.conn.Exec(g.ctx, query, id)
	require.NoError(t, err)
}

// start snapshots the baseline, then starts under capture of n calls workers ownerless sessions named app-<i>, or one
// owner on a session named app-0, which acquires and reloads the fixtures first.
func (g *gate) start(t *testing.T, app string, workers, maxBatch, n int) (*captureWriter, *ownerlessWriter) {
	t.Helper()
	snapshotBaseline(g.ctx, t, g.conn)

	if !g.owner {
		inner := newTestOwnerless(g.ctx, t, g.env, g.b, app, workers, maxBatch)

		return captureCalls(inner, n), inner
	}

	inner, err := startOwner(g.ctx, t, namedBatcher(t, g.b, g.env, app+"-0", nil), app, maxBatch).wait(t)
	require.NoError(t, err)

	return captureCalls(inner, n), inner
}

// concurrent submits every request at once, one caller each, to 32 workers, or the owner, of cap 256, and waits for
// every answer. The capture keeps room for one retry.
func (g *gate) concurrent(t *testing.T, app string, reqs []request) (*captureWriter, *ownerlessWriter) {
	t.Helper()

	w, inner := g.start(t, app, gateWorkers, gateMaxBatch, len(reqs)+1)
	submitEach(g.ctx, t, w, reqs).wait(t)

	return w, inner
}

// finish closes the writer after every Submit returned and verifies the ledger, the baseline and the answers.
func (g *gate) finish(t *testing.T, w *captureWriter) map[string]int {
	t.Helper()
	require.NoError(t, w.Close(g.ctx))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")

	return verifyLedger(g.ctx, t, g.conn, w.calls)
}

// ordinaryOf returns the posting builder of ordinary balance i modulo the 4 seeded.
func ordinaryOf(i int) func(int64) posting { return mv(classOrdinary, i%4, constant.DefaultBalanceKey) }

// awaitQueued polls, within 30 s, until n attempts wait in w's queue.
func awaitQueued(t *testing.T, w *ownerlessWriter, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return len(w.queue) == n }, 30*time.Second, 10*time.Millisecond, "%d attempts queued", n)
}
