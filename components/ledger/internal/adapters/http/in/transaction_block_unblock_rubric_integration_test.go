// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// rubricRouteFixture is a transaction route linking exactly one source and one
// destination operation route, each carrying its own accounting entries.
type rubricRouteFixture struct {
	transaction uuid.UUID
	source      uuid.UUID
	destination uuid.UUID
}

// rubricPost posts a block or unblock through one API version, with every leg
// naming its operation route and the body naming the transaction route.
type rubricPost func(t *testing.T, h *feeHarness, action string, routes rubricRouteFixture, sourceAlias, destinationAlias, amount string) txResponse

// TestIntegration_BlockUnblock_ConfiguredRubric proves, on both API versions and
// against the real engine, that a block or unblock books the rubric of the route's
// dedicated block/unblock entry, falls back to the direct rubric when the route
// configures none, and keeps the overdraft rubric on a repayment companion.
func TestIntegration_BlockUnblock_ConfiguredRubric(t *testing.T) {
	h := setupFeeHarness(t)
	postgrestestutil.SetLedgerSettings(t, h.db, h.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	v1App := h.newApp()
	v2App := h.newV2App()

	surfaces := []struct {
		name string
		post rubricPost
	}{
		{
			name: "v1",
			post: func(t *testing.T, h *feeHarness, action string, routes rubricRouteFixture, sourceAlias, destinationAlias, amount string) txResponse {
				t.Helper()

				body := fmt.Sprintf(`{"description":"%s rubric","routeId":%q,"send":{"asset":"BRL","value":%q,`+
					`"source":{"from":[{"accountAlias":%q,"amount":{"asset":"BRL","value":%q},"routeId":%q}]},`+
					`"distribute":{"to":[{"accountAlias":%q,"amount":{"asset":"BRL","value":%q},"routeId":%q}]}}}`,
					action, routes.transaction, amount, sourceAlias, amount, routes.source, destinationAlias, amount, routes.destination)

				return h.post(t, v1App, h.txPath(action), body, nil)
			},
		},
		{
			name: "v2",
			post: func(t *testing.T, h *feeHarness, action string, routes rubricRouteFixture, sourceAlias, destinationAlias, amount string) txResponse {
				t.Helper()

				body := h.v2RoutedBody(action+" rubric", "BRL", amount, routes.transaction,
					[]string{h.v2RoutedLeg(sourceAlias, amount, routes.source)},
					[]string{h.v2RoutedLeg(destinationAlias, amount, routes.destination)})

				return h.post(t, v2App, h.v2CreatePath(action), body, nil)
			},
		},
	}

	dedicated := func(direction, suffix string) map[string]any {
		return map[string]any{
			cn.ActionDirect:  rubricEntry(direction, "D-"+suffix),
			cn.ActionBlock:   rubricEntry(direction, "B-"+suffix),
			cn.ActionUnblock: rubricEntry(direction, "U-"+suffix),
		}
	}
	directOnly := func(direction, suffix string) map[string]any {
		return map[string]any{cn.ActionDirect: rubricEntry(direction, "D-"+suffix)}
	}

	for _, surface := range surfaces {
		t.Run(surface.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see transaction_handler_v2_integration_test.go).
			alias := func(name string) string { return "@rubric-" + surface.name + "-" + name }

			t.Run("a block books the block rubric of the route", func(t *testing.T) {
				routes := h.seedRubricRoutes(t, surface.name+" block dedicated", dedicated(cn.DirectionDebit, "SRC"), dedicated(cn.DirectionCredit, "DST"))
				h.seedBalance(t, alias("block-src"), "BRL", decimal.NewFromInt(100), "deposit")
				h.seedBalance(t, alias("block-dst"), "BRL", decimal.Zero, "deposit")

				resp := surface.post(t, h, "block", routes, alias("block-src"), alias("block-dst"), "40")
				require.Equalf(t, 201, resp.status, "block must post: %s", string(resp.rawBody))

				assert.Equal(t, map[string]string{
					"BLOCK/debit/default":  "B-SRC",
					"BLOCK/credit/default": "B-DST",
				}, operationRubrics(t, h, mustTxID(t, resp)))
			})

			t.Run("an unblock books the unblock rubric of the route", func(t *testing.T) {
				routes := h.seedRubricRoutes(t, surface.name+" unblock dedicated", dedicated(cn.DirectionDebit, "SRC"), dedicated(cn.DirectionCredit, "DST"))
				h.seedBalance(t, alias("unblock-src"), "BRL", decimal.NewFromInt(100), "deposit")
				h.seedBalance(t, alias("unblock-dst"), "BRL", decimal.Zero, "deposit")

				resp := surface.post(t, h, "unblock", routes, alias("unblock-src"), alias("unblock-dst"), "40")
				require.Equalf(t, 201, resp.status, "unblock must post: %s", string(resp.rawBody))

				assert.Equal(t, map[string]string{
					"UNBLOCK/debit/default":  "U-SRC",
					"UNBLOCK/credit/default": "U-DST",
				}, operationRubrics(t, h, mustTxID(t, resp)))
			})

			t.Run("a route without block or unblock entries keeps the direct rubric", func(t *testing.T) {
				routes := h.seedRubricRoutes(t, surface.name+" direct only", directOnly(cn.DirectionDebit, "SRC"), directOnly(cn.DirectionCredit, "DST"))
				h.seedBalance(t, alias("fallback-src"), "BRL", decimal.NewFromInt(100), "deposit")
				h.seedBalance(t, alias("fallback-dst"), "BRL", decimal.Zero, "deposit")

				blocked := surface.post(t, h, "block", routes, alias("fallback-src"), alias("fallback-dst"), "40")
				require.Equalf(t, 201, blocked.status, "block must post: %s", string(blocked.rawBody))
				assert.Equal(t, map[string]string{
					"BLOCK/debit/default":  "D-SRC",
					"BLOCK/credit/default": "D-DST",
				}, operationRubrics(t, h, mustTxID(t, blocked)))

				unblocked := surface.post(t, h, "unblock", routes, alias("fallback-src"), alias("fallback-dst"), "10")
				require.Equalf(t, 201, unblocked.status, "unblock must post: %s", string(unblocked.rawBody))
				assert.Equal(t, map[string]string{
					"UNBLOCK/debit/default":  "D-SRC",
					"UNBLOCK/credit/default": "D-DST",
				}, operationRubrics(t, h, mustTxID(t, unblocked)))
			})

			t.Run("an unblock repaying overdraft keeps the overdraft rubric on the companion", func(t *testing.T) {
				destination := dedicated(cn.DirectionCredit, "DST")
				destination[cn.ActionOverdraft] = map[string]any{
					cn.DirectionDebit:  rubricCode("O-DST-D"),
					cn.DirectionCredit: rubricCode("O-DST-C"),
				}
				routes := h.seedRubricRoutes(t, surface.name+" unblock overdraft", dedicated(cn.DirectionDebit, "SRC"), destination)
				h.seedBalance(t, alias("repay-src"), "BRL", decimal.NewFromInt(100), "deposit")
				h.seedOverdrawnBalance(t, alias("repay-dst"), "BRL", decimal.NewFromInt(10))

				resp := surface.post(t, h, "unblock", routes, alias("repay-src"), alias("repay-dst"), "100")
				require.Equalf(t, 201, resp.status, "unblock must post: %s", string(resp.rawBody))

				assert.Equal(t, map[string]string{
					"UNBLOCK/debit/default":                      "U-SRC",
					"UNBLOCK/credit/default":                     "U-DST",
					"OVERDRAFT/credit/" + cn.OverdraftBalanceKey: "O-DST-C",
				}, operationRubrics(t, h, mustTxID(t, resp)))
			})
		})
	}
}

// seedRubricRoutes creates a source and a destination operation route with the
// given accounting entries and links both, and nothing else, to a new transaction
// route, so the route count check accepts a two-leg transfer.
func (h *feeHarness) seedRubricRoutes(t *testing.T, title string, sourceEntries, destinationEntries map[string]any) rubricRouteFixture {
	t.Helper()

	fixture := rubricRouteFixture{
		transaction: postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, title),
		source:      h.seedOperationRouteEntries(t, title+" source", "source", sourceEntries),
		destination: h.seedOperationRouteEntries(t, title+" destination", "destination", destinationEntries),
	}

	postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, fixture.source, fixture.transaction)
	postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, fixture.destination, fixture.transaction)

	return fixture
}

func (h *feeHarness) seedOperationRouteEntries(t *testing.T, title, operationType string, entries map[string]any) uuid.UUID {
	t.Helper()

	id := postgrestestutil.CreateTestOperationRouteSimple(t, h.db, h.orgID, h.ledgerID, title, operationType)

	raw, err := json.Marshal(entries)
	require.NoError(t, err, "encode operation route accounting entries")

	res, err := h.db.Exec(`UPDATE operation_route SET accounting_entries=$1::jsonb WHERE id=$2`, string(raw), id)
	require.NoError(t, err, "seed operation route accounting entries")
	affected, err := res.RowsAffected()
	require.NoError(t, err, "read seeded operation route count")
	require.EqualValues(t, 1, affected)

	return id
}

// seedOverdrawnBalance seeds an account whose default balance is in overdraft by
// debt, together with the overdraft companion that mirrors that debt.
func (h *feeHarness) seedOverdrawnBalance(t *testing.T, alias, asset string, debt decimal.Decimal) {
	t.Helper()

	balanceID := h.seedBalance(t, alias, asset, decimal.Zero, "deposit")
	_, err := h.db.Exec(
		`UPDATE balance SET settings = '{"allowOverdraft":true,"overdraftLimitEnabled":true,"overdraftLimit":"1000"}'::jsonb, overdraft_used = $2 WHERE id = $1`,
		balanceID, debt,
	)
	require.NoError(t, err, "enable overdraft on %s", alias)

	companionID := h.seedAdditionalBalance(t, alias, cn.OverdraftBalanceKey, asset, debt, "deposit")
	_, err = h.db.Exec(`UPDATE balance SET direction = 'debit', settings = '{"balanceScope":"internal"}'::jsonb WHERE id = $1`, companionID)
	require.NoError(t, err, "shape the overdraft companion of %s", alias)
}

// operationRubrics maps each persisted operation of the transaction, keyed by
// type/direction/balance key, to its rubric code.
func operationRubrics(t *testing.T, h *feeHarness, txID uuid.UUID) map[string]string {
	t.Helper()

	rows, err := h.db.Query(`SELECT type, direction, balance_key, route_code FROM operation WHERE transaction_id = $1`, txID)
	require.NoError(t, err, "query operation rubrics")

	defer func() { _ = rows.Close() }()

	rubrics := make(map[string]string)

	for rows.Next() {
		var (
			opType, direction, key string
			code                   *string
		)

		require.NoError(t, rows.Scan(&opType, &direction, &key, &code), "scan operation rubric")
		require.NotNil(t, code, "%s %s operation on %s must carry a rubric code", opType, direction, key)

		rowKey := opType + "/" + direction + "/" + key
		require.NotContains(t, rubrics, rowKey, "one operation per type, direction and balance key")
		rubrics[rowKey] = *code
	}

	require.NoError(t, rows.Err(), "operation rubric rows iteration")

	return rubrics
}

func rubricEntry(direction, code string) map[string]any {
	return map[string]any{direction: rubricCode(code)}
}

func rubricCode(code string) map[string]string {
	return map[string]string{"code": code, "description": code + " rubric"}
}
