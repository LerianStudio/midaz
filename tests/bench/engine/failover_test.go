//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	toxiproxyclient "github.com/Shopify/toxiproxy/v2/client"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// An owner whose renewals stop idles past its lease while a successor takes epoch 2, reloads and approves 1,000
// transfers. Its next batch moves only balances the successor never touched: the renewal refuses it before the claim
// the test holds, so the batch answers the fence cause, not a deadline, and persists nothing.
func TestStaleOwnerFenced(t *testing.T) {
	g := newGate(t, true)
	hot, fee := mv(classHot, 0, constant.DefaultBalanceKey), mv(classFee, 0, constant.DefaultBalanceKey)

	snapshotBaseline(g.ctx, t, g.conn)

	const appA, appB = "bench-stale-owner", "bench-stale-successor"

	bA := namedBatcher(t, g.b, g.env, appA, nil)
	innerA, err := startOwner(g.ctx, t, bA, "owner-a", gateMaxBatch).wait(t)
	require.NoError(t, err)

	wA := captureCalls(innerA, 2)
	row := holdLock(g.ctx, t, g.env, lockBalance, hot(0).BalanceID)
	warm := batchReq(t, "stale/warm", "stale/warm", g.clock.now(), hot(-1), ordinaryOf(0)(1))
	warming := submitEach(g.ctx, t, wA, []request{warm})

	awaitBackend(g.ctx, t, g.conn, appA, "Lock") // its UPDATE waits on the held balance, after the pass renewed
	bA.owner.ticker.Stop()                       // the worker is inside the pass, so no tick can renew after Stop
	require.NoError(t, row.Rollback(g.ctx))
	assert.Equal(t, []string{"approved+tx"}, summarize(t, []request{warm}, warming.wait(t)))

	awaitPrimary(g.ctx, t, g.conn, "owner-a's lease expires with its renewals stopped", fenceExpired, benchTenantID)

	innerB, err := startOwner(g.ctx, t, namedBatcher(t, g.b, g.env, appB, nil), "owner-b", gateMaxBatch).wait(t)
	require.NoError(t, err)

	tok, _ := readFence(g.ctx, t, g.conn)
	assert.Equal(t, ownerToken{owner: "owner-b", epoch: 2}, tok)

	wB, ring := captureCalls(innerB, 1000), make([]request, 1000)
	for i := range ring {
		name := fmt.Sprintf("stale/ring/%d", i)
		ring[i] = batchReq(t, name, name, g.clock.now(), ordinaryOf(i)(-1), ordinaryOf(i+1)(1))
	}

	assert.Equal(t, slices.Repeat([]string{"approved+tx"}, len(ring)), summarize(t, ring, submitEach(g.ctx, t, wB, ring).wait(t)))

	// A's cached versions of hot and fee still match the database: only the fence guards this batch.
	stale := batchReq(t, "stale/late", "stale/late", g.clock.now(), hot(-5), fee(5))
	claim := holdLock(g.ctx, t, g.env, claimSQL, benchTenantID, []string{stale.id.String()}, [][]byte{stale.fingerprint[:]},
		[]time.Time{stale.admissionCutoff})

	assertUnknown(t, []result{wA.Submit(g.ctx, stale)}, renewRefused) // a claim first would wait out A's batch budget
	require.NoError(t, claim.Rollback(g.ctx))

	require.ErrorContains(t, wA.Close(g.ctx), renewRefused, "Close reports the first cause")
	require.NoError(t, wB.Close(g.ctx))
	assert.Equal(t, map[string]int{"approved": 1001, "unknown": 1},
		verifyLedger(g.ctx, t, g.conn, slices.Concat(wA.calls, wB.calls)))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
}

const (
	lostAckApp      = "bench-lost-ack" // the gate names the writer's session lostAckApp-0
	clientDropToxic = "pg-client-drop"
	commitUnknown   = "commit outcome unknown"
	backendIdle     = `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1 AND state = 'idle')`
	noBackend       = `SELECT NOT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1)`
)

// The reply to an ownerless writer's COMMIT, applied on the standby, is lost: it answers unknown at its batch budget and
// retries nothing. Healed, the same writer redials through the proxy and replays a retry past the cutoff, moving nothing.
func TestOwnerlessLostCommitAck(t *testing.T) {
	g, w, inner, retry := loseCommitReply(t, false)

	assert.Equal(t, []string{"replay/approved+tx"}, summarize(t, []request{retry}, []result{w.Submit(g.ctx, retry)}))
	require.ErrorIs(t, w.Close(g.ctx), context.DeadlineExceeded, "Close reports the lost reply")
	assert.Equal(t, 2, inner.workers[0].passes, "one pass per batch: nothing retried the lost COMMIT")
	assert.Equal(t, map[string]int{"unknown": 1, "replay/approved": 1, unansweredApproval: 1},
		verifyLedger(g.ctx, t, g.conn, w.calls))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
}

// An owner whose financial COMMIT reply is lost answers unknown at its budget, ends and runs no SQL once healed. Its
// successor, through the healed proxy, replays a retry past its cutoff and approves on the versions the COMMIT left.
func TestOwnerLostCommitAck(t *testing.T) {
	g, first, inner, retry := loseCommitReply(t, true)

	untouched := batchReq(t, "lostack/untouched", "lostack/untouched", g.clock.now(), ordinaryOf(2)(-1), ordinaryOf(3)(1))
	assertUnknown(t, []result{first.Submit(g.ctx, untouched)}, commitUnknown)
	require.ErrorIs(t, first.Close(g.ctx), context.DeadlineExceeded, "Close reports the lost reply")
	awaitPrimary(g.ctx, t, g.conn, "the lost owner opened no connection once healed", noBackend, lostAckApp+"-0")
	assert.Equal(t, 1, inner.workers[0].passes, "nothing retried the lost COMMIT")
	awaitPrimary(g.ctx, t, g.conn, "the lost owner's lease expires", fenceExpired, benchTenantID)

	innerB, err := startOwner(g.ctx, t, namedBatcher(t, g.b, g.env, lostAckApp+"-successor", nil), "owner-b", gateMaxBatch).wait(t)
	require.NoError(t, err)

	second := captureCalls(innerB, 2)
	again := batchReq(t, "lostack/again", "lostack/again", g.clock.now(), retry.postings...)
	assert.Equal(t, []string{"replay/approved+tx", "approved+tx"}, summarize(t, []request{retry, again},
		[]result{second.Submit(g.ctx, retry), second.Submit(g.ctx, again)}), "the successor reloaded what the lost COMMIT left")
	require.NoError(t, second.Close(g.ctx))
	assert.Equal(t, map[string]int{"unknown": 2, "replay/approved": 1, "approved": 1, unansweredApproval: 1},
		verifyLedger(g.ctx, t, g.conn, slices.Concat(first.calls, second.calls)))
	assert.Zero(t, g.broker.publishes.Load(), "obligations stay in the outbox")
}

// loseCommitReply starts one writer reaching the primary only through pg-client under remote_apply and loses the reply
// to its COMMIT of a transfer the standby applied while the pass's context was live. Once pgx closed that connection it
// heals pg-client and returns the round, whose env crosses pg-client, the writer and the transfer past its cutoff.
func loseCommitReply(t *testing.T, owner bool) (*gate, *captureWriter, *ownerlessWriter, request) {
	t.Helper()

	g := newGate(t, owner)

	stack, err := openVerifiedStack(g.ctx, g.env)
	require.NoError(t, err)

	defer stack.close(g.ctx)

	passes := make(passContexts, 1)
	proxied, lossy := *g.env, *g // only writers cross pg-client; setup, locks, polls and checks stay direct
	proxied.primary = g.env.clientProxy.Copy()
	proxied.primary.RuntimeParams["synchronous_commit"] = "remote_apply" // a COMMIT waits for replay: the rendezvous
	proxied.sessionTracer = func() pgx.QueryTracer { return passes }
	lossy.env = &proxied

	w, inner := lossy.start(t, lostAckApp, 1, gateMaxBatch, 2)
	app := lostAckApp + "-0"
	original := batchReq(t, "lostack/original", "lostack/original", g.clock.now(),
		mv(classHot, 0, constant.DefaultBalanceKey)(-5), ordinaryOf(0)(5))

	outbox := holdLock(g.ctx, t, g.env, `LOCK TABLE outbox_events IN SHARE MODE`)
	paying := submitEach(g.ctx, t, w, []request{original})

	awaitBackend(g.ctx, t, g.conn, app, "relation") // inside its pass, so an owner's idle renewals stay out of SyncRep
	pauseReplay(g.ctx, t, g.env, stack.standby)
	require.NoError(t, outbox.Rollback(g.ctx))
	awaitBackend(g.ctx, t, g.conn, app, "SyncRep")
	require.Len(t, passes, 1, "the waiting pass finalized its claim")

	pass := <-passes
	heal := dropClientReplies(t, g.env)
	_, err = stack.standby.Exec(g.ctx, `SELECT pg_wal_replay_resume()`)
	require.NoError(t, err)

	awaitPrimary(g.ctx, t, g.conn, "the COMMIT applied on the standby and replied", backendIdle, app)
	require.Equal(t, 1, countTransactions(g.ctx, t, stack.standby, []request{original}), "the standby applied the COMMIT")
	require.NoError(t, pass.Err(), "the pass was live when its COMMIT finished: no deadline of its own ended the wait")
	assertUnknown(t, paying.wait(t), commitUnknown)

	awaitPrimary(g.ctx, t, g.conn, "pgx closed its connection through the dropping proxy", noBackend, app)
	heal()

	g.clock.advance(time.Minute) // past the original's cutoff and response deadline
	original.deadline = g.clock.now().Add(usualBudget.caller)

	return &lossy, w, inner, original
}

// passContexts hands over the context of a finalize statement, a financial pass's own: BEGIN, every statement of the
// pass and pgx's COMMIT run under it. A statement never waits on it; contexts that do not fit are dropped.
type passContexts chan context.Context

func (p passContexts) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if data.SQL == finalizeSQL {
		select {
		case p <- ctx:
		default:
		}
	}

	return ctx
}

func (passContexts) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// dropClientReplies drops every server reply on pg-client, open connections included, keeping them open. Its heal,
// also run by cleanup, removes only that toxic, closing what it held, and fails on any other toxic, which it keeps.
func dropClientReplies(t *testing.T, env *benchEnv) func() {
	t.Helper()
	t.Cleanup(http.DefaultClient.CloseIdleConnections) // after the heal: the SDK's keep-alives would outlive noLeaks

	proxy, err := toxiproxyclient.NewClient(env.toxiproxy.String()).Proxy("pg-client")
	require.NoError(t, err)
	// toxiproxy.json listens on 0.0.0.0:5434; a toxic already there is not this test's to remove.
	require.True(t, proxy.Enabled && proxy.Upstream == "bench-pg-primary:5432" && strings.HasSuffix(proxy.Listen, ":5434") &&
		len(proxy.ActiveToxics) == 0, "pg-client %+v", proxy)

	heal := sync.OnceFunc(func() {
		assert.NoError(t, proxy.RemoveToxic(clientDropToxic), "remove %s from pg-client through the toxiproxy API", clientDropToxic)

		left, readErr := proxy.Toxics()
		if assert.NoError(t, readErr) {
			assert.Empty(t, left, "pg-client toxics this test did not add, left in place")
		}
	})
	t.Cleanup(heal) // registered before the toxic exists

	_, err = proxy.AddToxic(clientDropToxic, "timeout", "downstream", 1, toxiproxyclient.Attributes{"timeout": 0})
	require.NoError(t, err)

	return heal
}
