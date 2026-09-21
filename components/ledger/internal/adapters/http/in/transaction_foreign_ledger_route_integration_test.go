// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestDirectV2ForeignLedgerTransactionRoute is the end-to-end proof for
// midaz#2506: a transaction posted against ledger L2 that names a transaction
// route belonging to ledger L1 of the SAME organization is refused with HTTP
// 404 / code 0105 — on the first attempt, which misses in Redis and misses in
// the database, and on an immediate retry, which hits the NOT_FOUND sentinel
// the first attempt cached for 60s.
//
// Before the fix both arms of GetOrCreateTransactionRouteCache returned
// services.ErrDatabaseItemNotFound (a bare errors.New), which no arm of
// classifyForProblem recognises, so the envelope fell through to HTTP 500: a
// permanent client mistake was reported as a server fault and tripped the
// caller's circuit breaker. The retry assertion is the cache-independence
// claim — the sentinel branch must answer exactly like the database branch.
func TestDirectV2ForeignLedgerTransactionRoute(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	routes := h.seedDirectRoutes(t, 1, 1)
	app := h.newV2App()

	h.seedBalance(t, "@foreign-payer", "BRL", decimal.NewFromInt(1000), "deposit")
	h.seedBalance(t, "@foreign-receiver", "BRL", decimal.Zero, "deposit")

	// A second ledger under the SAME organization, route-validating, holding
	// accounts under the same aliases. The only difference between the control
	// request and the two refused ones is the ledger the legs are scoped to.
	other := h.withSecondLedger(t)
	foreignPayerID := other.seedBalance(t, "@foreign-payer", "BRL", decimal.NewFromInt(1000), "deposit")
	foreignReceiverID := other.seedBalance(t, "@foreign-receiver", "BRL", decimal.Zero, "deposit")

	ownBody := h.v2RoutedBody("own ledger route", "BRL", "100", routes.transaction,
		[]string{h.v2RoutedLeg("@foreign-payer", "100", routes.sources[0])},
		[]string{h.v2RoutedLeg("@foreign-receiver", "100", routes.destinations[0])})

	// Same route ids, same aliases, same amounts — scoped to L2 instead of L1.
	foreignBody := other.v2RoutedBody("foreign ledger route", "BRL", "100", routes.transaction,
		[]string{other.v2RoutedLeg("@foreign-payer", "100", routes.sources[0])},
		[]string{other.v2RoutedLeg("@foreign-receiver", "100", routes.destinations[0])})

	sentinelKey := utils.AccountingRoutesInternalKey(h.orgID, other.ledgerID, routes.transaction)

	t.Run("control own ledger accepts its own route", func(t *testing.T) {
		resp := h.createV2Direct(t, app, ownBody, nil)
		require.Equalf(t, 201, resp.status, "a route used inside its own ledger must still post: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		require.Len(t, legs, 2)
		requireBalanced(t, legs, "control transfer on the owning ledger")
	})

	t.Run("cold cache foreign ledger route answers 404", func(t *testing.T) {
		resp := other.createV2Direct(t, app, foreignBody, nil)
		require.Equalf(t, 404, resp.status, "a foreign-ledger route must be not-found, not a server fault: %s", string(resp.rawBody))
		assert.Equal(t, "0105", resp.body["code"], "body: %s", string(resp.rawBody))
		other.assertNoMoneyMoved(t, foreignPayerID, foreignReceiverID)
	})

	t.Run("cold miss caches the not found sentinel", func(t *testing.T) {
		cached, err := h.redisRepo.GetBytes(h.ctx(), sentinelKey)
		require.NoError(t, err, "the cold miss must have cached a sentinel at %s", sentinelKey)
		assert.True(t, bytes.Equal(cached, []byte("NOT_FOUND")), "cached value must be the NOT_FOUND sentinel, got %q", cached)
	})

	t.Run("warm cache repeats the same 404 from the sentinel", func(t *testing.T) {
		resp := other.createV2Direct(t, app, foreignBody, nil)
		require.NotEqualf(t, 500, resp.status, "the sentinel branch must not fall back to a server fault: %s", string(resp.rawBody))
		require.Equalf(t, 404, resp.status, "the retry inside the sentinel TTL must answer like the first attempt: %s", string(resp.rawBody))
		assert.Equal(t, "0105", resp.body["code"], "body: %s", string(resp.rawBody))
		other.assertNoMoneyMoved(t, foreignPayerID, foreignReceiverID)
	})
}

// withSecondLedger returns a shallow copy of the harness scoped to a second,
// route-validating ledger under the same organization. Every seeding and v2
// body helper reads h.ledgerID, so the copy seeds and posts against L2 while
// sharing the one wired stack — handler, use cases, containers — with the
// original.
func (h *feeHarness) withSecondLedger(t *testing.T) *feeHarness {
	t.Helper()

	other := *h
	other.ledgerID = postgrestestutil.CreateTestLedger(t, h.db, h.orgID)

	postgrestestutil.SetLedgerSettings(t, h.db, other.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	return &other
}

// assertNoMoneyMoved proves a refused request moved no money in this ledger: the seeded
// balances still read what they were seeded with, and no operation row was written. The
// balance half is the load-bearing one — a refusal that left a hold or a debit on the payer
// while writing no operation row would satisfy the row count alone.
func (h *feeHarness) assertNoMoneyMoved(t *testing.T, payerID, receiverID uuid.UUID) {
	t.Helper()

	assertBalance(t, h, payerID, "1000")
	assertBalance(t, h, receiverID, "0")

	var operations int

	err := h.db.QueryRow(`SELECT COUNT(*) FROM operation WHERE organization_id=$1 AND ledger_id=$2`, h.orgID, h.ledgerID).Scan(&operations)
	require.NoError(t, err, "count operations after rejected request")
	assert.Zero(t, operations)
}
