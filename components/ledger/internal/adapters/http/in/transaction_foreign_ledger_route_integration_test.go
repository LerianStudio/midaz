// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// Accounting routes belong to the organization: a route-validating ledger accepts
// any transaction route of its organization, whatever ledger the route was
// created under, on /v1 and /v2 alike. A route of another organization stays
// refused with HTTP 404 / code 0105 — on the first attempt, which misses in Redis
// and in the database, and on an immediate retry, which hits the NOT_FOUND
// sentinel the first attempt cached for 60s. The retry assertion is the
// cache-independence claim: the sentinel branch must answer exactly like the
// database branch, never with a server fault.

func TestTransactionRoute_OtherLedgerOfTheOrganizationPostsWithIt(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	routes := h.seedDirectRoutes(t, 1, 1)
	v1App := h.newApp()
	v2App := h.newV2App()

	h.seedBalance(t, "@route-payer", "BRL", decimal.NewFromInt(1000), "deposit")
	h.seedBalance(t, "@route-receiver", "BRL", decimal.Zero, "deposit")

	// A second route-validating ledger of the SAME organization, holding accounts
	// under the same aliases. The routes were created under the first ledger.
	other := h.withSecondLedger(t)
	other.seedBalance(t, "@route-payer", "BRL", decimal.NewFromInt(1000), "deposit")
	other.seedBalance(t, "@route-receiver", "BRL", decimal.Zero, "deposit")

	t.Run("control the ledger the routes were created under posts with them", func(t *testing.T) {
		body := h.v2RoutedBody("own ledger route", "BRL", "100", routes.transaction,
			[]string{h.v2RoutedLeg("@route-payer", "100", routes.sources[0])},
			[]string{h.v2RoutedLeg("@route-receiver", "100", routes.destinations[0])})

		resp := h.createV2Direct(t, v2App, body, nil)
		require.Equalf(t, 201, resp.status, "a route used inside the ledger it was created under must post: %s", string(resp.rawBody))
		requireRoutedLegs(t, h, mustTxID(t, resp), routes)
	})

	t.Run("v2 direct in another ledger of the organization posts with its routes", func(t *testing.T) {
		body := other.v2RoutedBody("organization route on v2", "BRL", "100", routes.transaction,
			[]string{other.v2RoutedLeg("@route-payer", "100", routes.sources[0])},
			[]string{other.v2RoutedLeg("@route-receiver", "100", routes.destinations[0])})

		resp := other.createV2Direct(t, v2App, body, nil)
		require.Equalf(t, 201, resp.status, "a route of the organization must validate in any of its ledgers: %s", string(resp.rawBody))
		requireRoutedLegs(t, other, mustTxID(t, resp), routes)
	})

	t.Run("v1 json in another ledger of the organization posts with its routes", func(t *testing.T) {
		body := fmt.Sprintf(`{"description":"organization route on v1","routeId":%q,"send":{"asset":"BRL","value":"100",
			"source":{"from":[{"accountAlias":"@route-payer","amount":{"asset":"BRL","value":"100"},"routeId":%q}]},
			"distribute":{"to":[{"accountAlias":"@route-receiver","amount":{"asset":"BRL","value":"100"},"routeId":%q}]}}}`,
			routes.transaction, routes.sources[0], routes.destinations[0])

		resp := other.createJSON(t, v1App, body, nil)
		require.Equalf(t, 201, resp.status, "/v1 also validates routes across the organization: %s", string(resp.rawBody))
		requireRoutedLegs(t, other, mustTxID(t, resp), routes)
	})
}

func TestTransactionRoute_OtherOrganizationIsNotFound(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	routes := h.seedDirectRoutes(t, 1, 1)
	app := h.newV2App()

	stranger := h.withOtherOrganization(t)
	payerID := stranger.seedBalance(t, "@stranger-payer", "BRL", decimal.NewFromInt(1000), "deposit")
	receiverID := stranger.seedBalance(t, "@stranger-receiver", "BRL", decimal.Zero, "deposit")

	body := stranger.v2RoutedBody("route of another organization", "BRL", "100", routes.transaction,
		[]string{stranger.v2RoutedLeg("@stranger-payer", "100", routes.sources[0])},
		[]string{stranger.v2RoutedLeg("@stranger-receiver", "100", routes.destinations[0])})

	sentinelKey := utils.AccountingRoutesInternalKey(stranger.orgID, routes.transaction)

	t.Run("cold cache answers 404", func(t *testing.T) {
		resp := stranger.createV2Direct(t, app, body, nil)
		require.Equalf(t, 404, resp.status, "a route of another organization must be not-found, not a server fault: %s", string(resp.rawBody))
		assert.Equal(t, "0105", resp.body["code"], "body: %s", string(resp.rawBody))
		stranger.assertNoMoneyMoved(t, payerID, receiverID)
	})

	t.Run("cold miss caches the not found sentinel under the requesting organization", func(t *testing.T) {
		cached, err := h.redisRepo.GetBytes(h.ctx(), sentinelKey)
		require.NoError(t, err, "the cold miss must have cached a sentinel at %s", sentinelKey)
		assert.True(t, bytes.Equal(cached, []byte("NOT_FOUND")), "cached value must be the NOT_FOUND sentinel, got %q", cached)
	})

	t.Run("warm cache repeats the same 404 from the sentinel", func(t *testing.T) {
		resp := stranger.createV2Direct(t, app, body, nil)
		require.NotEqualf(t, 500, resp.status, "the sentinel branch must not fall back to a server fault: %s", string(resp.rawBody))
		require.Equalf(t, 404, resp.status, "the retry inside the sentinel TTL must answer like the first attempt: %s", string(resp.rawBody))
		assert.Equal(t, "0105", resp.body["code"], "body: %s", string(resp.rawBody))
		stranger.assertNoMoneyMoved(t, payerID, receiverID)
	})
}

func TestTransactionRoute_RevertInAnotherLedgerOfTheOrganizationPassesTheBidirectionalCheck(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	// A revert resolves its origin through the engine evidence, as production wires it.
	h.queryUC.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}
	app := h.newV2App()

	// Routes created under the first ledger; the transaction and its revert
	// happen in a second route-validating ledger of the same organization.
	transactionRouteID, payerRouteID, receiverRouteID := h.seedRevertibleRoutes(t)

	other := h.withSecondLedger(t)
	other.seedBalance(t, "@revert-payer", "BRL", decimal.NewFromInt(1000), "deposit")
	other.seedBalance(t, "@revert-receiver", "BRL", decimal.Zero, "deposit")

	body := other.v2RoutedBody("revertible organization route", "BRL", "100", transactionRouteID,
		[]string{other.v2RoutedLeg("@revert-payer", "100", payerRouteID)},
		[]string{other.v2RoutedLeg("@revert-receiver", "100", receiverRouteID)})

	created := other.createV2Direct(t, app, body, nil)
	require.Equalf(t, 201, created.status, "the direct transaction must post: %s", string(created.rawBody))

	parentID := mustTxID(t, created)

	reverted := other.post(t, app, other.v2StatePath(parentID, "revert"), "", nil)
	require.Equalf(t, 201, reverted.status, "operation routes created under another ledger of the organization must pass the revert bidirectional check: %s", string(reverted.rawBody))

	reverseID := postgresGetChildTx(t, other, parentID)
	require.NotNil(t, reverseID, "the revert must create a child reverse transaction")

	reverseLegs := loadLegs(t, other.db, *reverseID)
	requireBalanced(t, reverseLegs, "reverse of a transaction routed across ledgers")
	requireLeg(t, reverseLegs, "@revert-receiver", "DEBIT", "100", receiverRouteID, "default")
	requireLeg(t, reverseLegs, "@revert-payer", "CREDIT", "100", payerRouteID, "default")
}

// requireRoutedLegs asserts the payer and receiver legs of a direct transaction
// carry the operation routes of the fixture and the rubric code of their direct
// entry, which seedDirectOperationRoute sets to the operation route ID.
func requireRoutedLegs(t *testing.T, h *feeHarness, txID uuid.UUID, routes directRouteFixture) {
	t.Helper()

	legs := loadLegs(t, h.db, txID)
	require.Len(t, legs, 2)
	requireBalanced(t, legs, "routed transfer")

	for _, want := range []struct {
		legType string
		routeID uuid.UUID
	}{
		{legType: "DEBIT", routeID: routes.sources[0]},
		{legType: "CREDIT", routeID: routes.destinations[0]},
	} {
		var routeID, routeCode *string

		err := h.db.QueryRow(`SELECT route_id, route_code FROM operation WHERE transaction_id=$1 AND type=$2`, txID, want.legType).
			Scan(&routeID, &routeCode)
		require.NoError(t, err, "load the %s operation", want.legType)
		require.NotNil(t, routeID, "%s operation must carry its operation route", want.legType)
		require.NotNil(t, routeCode, "%s operation must carry its rubric code", want.legType)
		assert.Equal(t, want.routeID.String(), *routeID)
		assert.Equal(t, want.routeID.String(), *routeCode)
	}
}

// withOtherOrganization returns a shallow copy of the harness scoped to a
// route-validating ledger of a different organization, sharing the one wired stack.
func (h *feeHarness) withOtherOrganization(t *testing.T) *feeHarness {
	t.Helper()

	other := *h
	other.orgID = postgrestestutil.CreateTestOrganization(t, h.db)
	other.ledgerID = postgrestestutil.CreateTestLedger(t, h.db, other.orgID)

	postgrestestutil.SetLedgerSettings(t, h.db, other.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	return &other
}

// seedRevertibleRoutes turns on route validation for the harness ledger and
// creates, under it, a transaction route linking two bidirectional operation
// routes, each with direct and revert entries for both directions. It returns
// the transaction route and the payer and receiver operation routes.
func (h *feeHarness) seedRevertibleRoutes(t *testing.T) (transactionRouteID, payerRouteID, receiverRouteID uuid.UUID) {
	t.Helper()

	postgrestestutil.SetLedgerSettings(t, h.db, h.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	transactionRouteID = postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, "revertible route")

	newBidirectional := func(title string) uuid.UUID {
		id := postgrestestutil.CreateTestOperationRouteSimple(t, h.db, h.orgID, h.ledgerID, title, "bidirectional")
		rubric := fmt.Sprintf(`{"debit":{"code":"%[1]s-D","description":"%[2]s"},"credit":{"code":"%[1]s-C","description":"%[2]s"}}`, id, title)

		_, err := h.db.Exec(`UPDATE operation_route SET accounting_entries=$1::jsonb WHERE id=$2`,
			`{"direct":`+rubric+`,"revert":`+rubric+`}`, id)
		require.NoError(t, err, "seed bidirectional accounting entries")

		postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, id, transactionRouteID)

		return id
	}

	return transactionRouteID, newBidirectional("revertible payer"), newBidirectional("revertible receiver")
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
