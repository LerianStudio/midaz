// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
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

	for _, want := range []struct {
		id     uuid.UUID
		amount string
	}{{payerID, "1000"}, {receiverID, "0"}} {
		got := postgrestestutil.GetBalanceAvailable(t, h.db, want.id)
		assert.Truef(t, got.Equal(decimal.RequireFromString(want.amount)),
			"balance %s: got %s, want %s", want.id, got, want.amount)
	}

	var operations int

	err := h.db.QueryRow(`SELECT COUNT(*) FROM operation WHERE organization_id=$1 AND ledger_id=$2`, h.orgID, h.ledgerID).Scan(&operations)
	require.NoError(t, err, "count operations after rejected request")
	assert.Zero(t, operations)
}

// directRouteFixture is the route set a route-validating direct create needs: one
// transaction route plus the operation routes linked to it, split by side.
type directRouteFixture struct {
	transaction  uuid.UUID
	sources      []uuid.UUID
	destinations []uuid.UUID
}

// seedDirectRoutes turns route validation on for the harness ledger and seeds the
// route set a direct create must satisfy. It also wires the two route repositories
// onto the shared query use case: setupFeeHarness leaves them nil because every
// other proof in this package runs with validateRoutes off, and the route cache
// reads TransactionRouteRepo the moment it is on.
func (h *feeHarness) seedDirectRoutes(t *testing.T, sourceCount, destinationCount int) directRouteFixture {
	t.Helper()

	h.queryUC.OperationRouteRepo = operationroute.NewOperationRoutePostgreSQLRepository(h.pgConn)
	h.queryUC.TransactionRouteRepo = transactionroute.NewTransactionRoutePostgreSQLRepository(h.pgConn)

	postgrestestutil.SetLedgerSettings(t, h.db, h.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	fixture := directRouteFixture{
		transaction:  postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, "foreign ledger direct route"),
		sources:      make([]uuid.UUID, 0, sourceCount),
		destinations: make([]uuid.UUID, 0, destinationCount),
	}

	for i := range sourceCount {
		id := h.seedDirectOperationRoute(t, fmt.Sprintf("direct source %d", i+1), "source", "debit")
		fixture.sources = append(fixture.sources, id)
		postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, id, fixture.transaction)
	}

	for i := range destinationCount {
		id := h.seedDirectOperationRoute(t, fmt.Sprintf("direct destination %d", i+1), "destination", "credit")
		fixture.destinations = append(fixture.destinations, id)
		postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, id, fixture.transaction)
	}

	return fixture
}

// seedDirectOperationRoute creates one operation route and gives it a "direct"
// accounting entry in the named direction, which is what places it under the
// direct action of the route cache.
func (h *feeHarness) seedDirectOperationRoute(t *testing.T, title, operationType, direction string) uuid.UUID {
	t.Helper()

	id := postgrestestutil.CreateTestOperationRouteSimple(t, h.db, h.orgID, h.ledgerID, title, operationType)
	entries := fmt.Sprintf(`{"direct":{"%s":{"code":"%s","description":"%s"}}}`,
		direction, id.String(), title)
	res, err := h.db.Exec(`UPDATE operation_route SET accounting_entries=$1::jsonb WHERE id=$2`, entries, id)
	require.NoError(t, err, "seed operation route accounting entries")
	affected, err := res.RowsAffected()
	require.NoError(t, err, "read seeded operation route count")
	require.EqualValues(t, 1, affected)

	return id
}

// v2RoutedLeg builds a v2 leg with its canonical operation route.
func (h *feeHarness) v2RoutedLeg(alias, amount string, operationRouteID uuid.UUID) string {
	return `{"alias":"` + alias + `",` + h.v2Scope() + `,"amount":"` + amount +
		`","operationRouteId":"` + operationRouteID.String() + `"}`
}

// v2RoutedBody is v2Body with the canonical transaction route attached.
func (h *feeHarness) v2RoutedBody(description, asset, amount string, transactionRouteID uuid.UUID, debits, credits []string) string {
	return strings.TrimSuffix(h.v2Body(description, asset, amount, debits, credits), "}") +
		`,"routeId":"` + transactionRouteID.String() + `"}`
}
