//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"fmt"
	"slices"
	"testing"
	"time"

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
