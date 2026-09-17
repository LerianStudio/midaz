// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type directRouteFixture struct {
	transaction  uuid.UUID
	sources      []uuid.UUID
	destinations []uuid.UUID
}

// TestDirectV2FeeAccountingRoutes exercises the production HTTP v2 translator,
// fee calculator, route cache, accounting validator and synchronous persistence
// together. The cases intentionally use non-deductible fees because those add a
// second debit and expose both routeFrom propagation and payer-route inheritance.
func TestDirectV2FeeAccountingRoutes(t *testing.T) {
	t.Run("explicit two source and two destination routes", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.enableAccountingEngine(t)
		routes := h.seedDirectRoutes(t, 2, 2)
		app := h.newV2App()

		h.seedBalance(t, "@route-payer", "BRL", decimal.NewFromInt(1000), "deposit")
		h.seedBalance(t, "@route-receiver", "BRL", decimal.Zero, "deposit")
		h.seedBalance(t, "@route-fee", "BRL", decimal.Zero, "deposit")
		h.seedAdditionalBalance(t, "@route-payer", "settlement", "BRL", decimal.NewFromInt(777), "deposit")

		fee := flatFee("route_fee", "@route-fee", "5", false)
		fee.routeFrom = routeString(routes.sources[1])
		fee.routeTo = routeString(routes.destinations[1])
		h.seedPackage(t, packageSpec{label: "explicit_routes", fees: []feeSpec{fee}})

		body := h.v2RoutedBody("explicit fee routes", "BRL", "100", routes.transaction,
			[]string{h.v2RoutedLeg("@route-payer", "100", routes.sources[0])},
			[]string{h.v2RoutedLeg("@route-receiver", "100", routes.destinations[0])})

		key := "fee-routes-" + uuid.New().String()
		headers := map[string]string{"X-Idempotency": key, "X-TTL": "60"}
		first := h.createV2Direct(t, app, body, headers)
		require.Equalf(t, 201, first.status, "direct create must succeed: %s", string(first.rawBody))

		txID := mustTxID(t, first)
		legs := loadLegs(t, h.db, txID)
		require.Len(t, legs, 4)
		requireBalanced(t, legs, "explicit 2/2 fee transaction")
		requireLeg(t, legs, "@route-payer", "DEBIT", "100", routes.sources[0], "default")
		requireLeg(t, legs, "@route-payer", "DEBIT", "5", routes.sources[1], "default")
		requireLeg(t, legs, "@route-receiver", "CREDIT", "100", routes.destinations[0], "default")
		requireLeg(t, legs, "@route-fee", "CREDIT", "5", routes.destinations[1], "default")

		assertLiveBalance(t, h, "@route-payer", "default", "895")
		assertLiveBalance(t, h, "@route-receiver", "default", "100")
		assertLiveBalance(t, h, "@route-fee", "default", "5")
		assertLiveBalance(t, h, "@route-payer", "settlement", "777")

		// Replay must return the first fee-inclusive result without posting a
		// second transaction or moving any balance twice.
		time.Sleep(300 * time.Millisecond)
		second := h.createV2Direct(t, app, body, headers)
		require.Equalf(t, 201, second.status, "idempotent replay must succeed: %s", string(second.rawBody))
		assert.Equal(t, "true", second.replayed)
		assert.Equal(t, first.body["id"], second.body["id"])
		require.Len(t, loadLegs(t, h.db, txID), 4)
		assertLiveBalance(t, h, "@route-payer", "default", "895")
		assertLiveBalance(t, h, "@route-receiver", "default", "100")
		assertLiveBalance(t, h, "@route-fee", "default", "5")
		assertLiveBalance(t, h, "@route-payer", "settlement", "777")
	})

	t.Run("fee debit inherits the single source route", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.enableAccountingEngine(t)
		routes := h.seedDirectRoutes(t, 1, 2)
		app := h.newV2App()

		h.seedBalance(t, "@inherit-payer", "BRL", decimal.NewFromInt(1000), "deposit")
		h.seedBalance(t, "@inherit-receiver", "BRL", decimal.Zero, "deposit")
		h.seedBalance(t, "@inherit-fee", "BRL", decimal.Zero, "deposit")

		fee := flatFee("inherited_route_fee", "@inherit-fee", "5", false)
		fee.routeTo = routeString(routes.destinations[1])
		h.seedPackage(t, packageSpec{label: "inherited_routes", fees: []feeSpec{fee}})

		body := h.v2RoutedBody("inherited fee source route", "BRL", "100", routes.transaction,
			[]string{h.v2RoutedLeg("@inherit-payer", "100", routes.sources[0])},
			[]string{h.v2RoutedLeg("@inherit-receiver", "100", routes.destinations[0])})

		resp := h.createV2Direct(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "direct create with inherited route must succeed: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		require.Len(t, legs, 4)
		requireBalanced(t, legs, "inherited 1/2 fee transaction")
		requireLeg(t, legs, "@inherit-payer", "DEBIT", "100", routes.sources[0], "default")
		requireLeg(t, legs, "@inherit-payer", "DEBIT", "5", routes.sources[0], "default")
		requireLeg(t, legs, "@inherit-receiver", "CREDIT", "100", routes.destinations[0], "default")
		requireLeg(t, legs, "@inherit-fee", "CREDIT", "5", routes.destinations[1], "default")
		assertLiveBalance(t, h, "@inherit-payer", "default", "895")
		assertLiveBalance(t, h, "@inherit-receiver", "default", "100")
		assertLiveBalance(t, h, "@inherit-fee", "default", "5")
	})

	t.Run("explicit incompatible route is rejected without movement", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.enableAccountingEngine(t)
		routes := h.seedDirectRoutes(t, 2, 2)
		app := h.newV2App()

		payerID := h.seedBalance(t, "@invalid-payer", "BRL", decimal.NewFromInt(1000), "deposit")
		receiverID := h.seedBalance(t, "@invalid-receiver", "BRL", decimal.Zero, "deposit")
		feeRevenueID := h.seedBalance(t, "@invalid-fee", "BRL", decimal.Zero, "deposit")

		fee := flatFee("invalid_route_fee", "@invalid-fee", "5", false)
		fee.routeFrom = routeString(uuid.New())
		fee.routeTo = routeString(routes.destinations[1])
		h.seedPackage(t, packageSpec{label: "invalid_explicit_route", fees: []feeSpec{fee}})

		body := h.v2RoutedBody("invalid explicit fee route", "BRL", "100", routes.transaction,
			[]string{h.v2RoutedLeg("@invalid-payer", "100", routes.sources[0])},
			[]string{h.v2RoutedLeg("@invalid-receiver", "100", routes.destinations[0])})

		resp := h.createV2Direct(t, app, body, nil)
		require.Equalf(t, 422, resp.status, "invalid explicit route must be rejected: %s", string(resp.rawBody))
		assert.Equal(t, "0117", resp.body["code"])
		assertNoMovement(t, h, payerID, receiverID, feeRevenueID)
	})

	t.Run("missing payer route is rejected without movement", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.enableAccountingEngine(t)
		routes := h.seedDirectRoutes(t, 1, 2)
		app := h.newV2App()

		payerID := h.seedBalance(t, "@missing-payer", "BRL", decimal.NewFromInt(1000), "deposit")
		receiverID := h.seedBalance(t, "@missing-receiver", "BRL", decimal.Zero, "deposit")
		feeRevenueID := h.seedBalance(t, "@missing-fee", "BRL", decimal.Zero, "deposit")

		fee := flatFee("missing_route_fee", "@missing-fee", "5", false)
		fee.routeTo = routeString(routes.destinations[1])
		h.seedPackage(t, packageSpec{label: "missing_route", fees: []feeSpec{fee}})

		body := h.v2RoutedBody("missing payer fee route", "BRL", "100", routes.transaction,
			[]string{h.v2Leg("@missing-payer", "100")},
			[]string{h.v2RoutedLeg("@missing-receiver", "100", routes.destinations[0])})

		resp := h.createV2Direct(t, app, body, nil)
		require.Equalf(t, 422, resp.status, "missing source route must be rejected: %s", string(resp.rawBody))
		assert.Equal(t, "0117", resp.body["code"])
		assertNoMovement(t, h, payerID, receiverID, feeRevenueID)
	})
}

func (h *feeHarness) seedDirectRoutes(t *testing.T, sourceCount, destinationCount int) directRouteFixture {
	t.Helper()

	postgrestestutil.SetLedgerSettings(t, h.db, h.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	fixture := directRouteFixture{
		transaction:  postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, "fee direct route"),
		sources:      make([]uuid.UUID, 0, sourceCount),
		destinations: make([]uuid.UUID, 0, destinationCount),
	}

	for i := range sourceCount {
		id := h.seedDirectOperationRoute(t, fmt.Sprintf("fee source %d", i+1), "source", "debit")
		fixture.sources = append(fixture.sources, id)
		postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, id, fixture.transaction)
	}

	for i := range destinationCount {
		id := h.seedDirectOperationRoute(t, fmt.Sprintf("fee destination %d", i+1), "destination", "credit")
		fixture.destinations = append(fixture.destinations, id)
		postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, id, fixture.transaction)
	}

	return fixture
}

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

func routeString(id uuid.UUID) *string {
	value := id.String()
	return &value
}

func requireLeg(t *testing.T, legs []persistedLeg, alias, legType, amount string, routeID uuid.UUID, balanceKey string) {
	t.Helper()

	wantAmount := decimal.RequireFromString(amount)
	for _, leg := range legs {
		if leg.Alias == alias && leg.Type == legType && leg.Amount.Equal(wantAmount) {
			require.NotNil(t, leg.Route, "matching leg must carry a route")
			assert.Equal(t, routeID.String(), *leg.Route)
			assert.Equal(t, balanceKey, leg.Key)
			return
		}
	}

	require.Failf(t, "operation leg not found", "%s %s %s", alias, legType, amount)
}

func assertBalance(t *testing.T, h *feeHarness, balanceID uuid.UUID, want string) {
	t.Helper()
	got := postgrestestutil.GetBalanceAvailable(t, h.db, balanceID)
	assert.Truef(t, got.Equal(decimal.RequireFromString(want)), "balance %s: got %s, want %s", balanceID, got, want)
}

func assertLiveBalance(t *testing.T, h *feeHarness, alias, key, want string) {
	t.Helper()

	balances, err := h.queryUC.GetBalances(h.ctx(), h.orgID, h.ledgerID, []string{mtransaction.AliasKey(alias, key)})
	require.NoError(t, err, "read live balance")
	require.Len(t, balances, 1, "live balance lookup")
	got := balances[0].Available
	assert.Truef(t, got.Equal(decimal.RequireFromString(want)), "%s#%s: got %s, want %s", alias, key, got, want)
}

func assertNoMovement(t *testing.T, h *feeHarness, payerID, receiverID, feeRevenueID uuid.UUID) {
	t.Helper()
	assertBalance(t, h, payerID, "1000")
	assertBalance(t, h, receiverID, "0")
	assertBalance(t, h, feeRevenueID, "0")

	var operations int
	err := h.db.QueryRow(`SELECT COUNT(*) FROM operation WHERE organization_id=$1 AND ledger_id=$2`, h.orgID, h.ledgerID).Scan(&operations)
	require.NoError(t, err, "count operations after rejected request")
	assert.Zero(t, operations)
}
