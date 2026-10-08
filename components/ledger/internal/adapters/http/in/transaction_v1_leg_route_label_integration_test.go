// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The /v1 leg carries a free-text, deprecated `route` label next to its canonical
// routeId. The label is accepted from the client and persisted on the operations
// of that leg exactly as sent; it never feeds route validation or routeId.
func TestIntegration_V1_LegRouteLabel(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	t.Run("each primary operation stores and returns its own leg's label verbatim", func(t *testing.T) {
		h.seedBalance(t, "@label-payer", "USD", decimal.NewFromInt(1000), "deposit")
		h.seedBalance(t, "@label-receiver", "USD", decimal.Zero, "deposit")

		resp := h.createJSON(t, app, v1LabeledTransfer("@label-payer", "@label-receiver", "100", "payer-label", "  receiver label  ", false), nil)
		require.Equalf(t, 201, resp.status, "labeled transfer must post: %s", string(resp.rawBody))
		txID := mustTxID(t, resp)

		want := map[string]*string{"@label-payer": strPtr("payer-label"), "@label-receiver": strPtr("  receiver label  ")}
		assert.Equal(t, want, operationRouteLabels(t, h, txID))
		assert.Equal(t, map[string]string{"@label-payer": "payer-label", "@label-receiver": "  receiver label  "}, responseOperationRoutes(t, resp.body))
		assert.Equal(t, map[string]string{"@label-payer": "payer-label", "@label-receiver": "  receiver label  "},
			responseOperationRoutes(t, getV1TransactionBody(t, h, app, txID)))
	})

	t.Run("legs without a label leave the route empty on v1 and v2", func(t *testing.T) {
		h.seedBalance(t, "@plain-payer", "USD", decimal.NewFromInt(1000), "deposit")
		h.seedBalance(t, "@plain-receiver", "USD", decimal.Zero, "deposit")

		v1 := h.createJSON(t, app, v1LabeledTransfer("@plain-payer", "@plain-receiver", "100", "", "", false), nil)
		require.Equalf(t, 201, v1.status, "unlabeled v1 transfer must post: %s", string(v1.rawBody))
		assert.Equal(t, map[string]*string{"@plain-payer": nil, "@plain-receiver": nil}, operationRouteLabels(t, h, mustTxID(t, v1)))

		v2 := h.createV2Direct(t, h.newV2App(), h.v2Body("unlabeled v2 transfer", "USD", "100",
			[]string{h.v2Leg("@plain-payer", "100")}, []string{h.v2Leg("@plain-receiver", "100")}), nil)
		require.Equalf(t, 201, v2.status, "v2 transfer must post: %s", string(v2.rawBody))
		assert.Equal(t, map[string]*string{"@plain-payer": nil, "@plain-receiver": nil}, operationRouteLabels(t, h, mustTxID(t, v2)))
	})

	t.Run("the overdraft companion of a labeled leg carries no label", func(t *testing.T) {
		seedOverdraftAccount(t, h.db, h.orgID, h.ledgerID, "@label-overdrawn", 50, 0)
		h.seedBalance(t, "@label-overdraft-receiver", "USD", decimal.Zero, "deposit")

		resp := h.createJSON(t, app, v1LabeledTransfer("@label-overdrawn", "@label-overdraft-receiver", "80", "overdrawn-label", "credited-label", false), nil)
		require.Equalf(t, 201, resp.status, "a transfer drawing overdraft must post: %s", string(resp.rawBody))
		txID := mustTxID(t, resp)

		var primary, companion *string
		require.NoError(t, h.db.QueryRow(`SELECT route FROM operation WHERE transaction_id = $1 AND account_alias = $2 AND balance_key = $3`,
			txID, "@label-overdrawn", cn.DefaultBalanceKey).Scan(&primary))
		require.NoError(t, h.db.QueryRow(`SELECT route FROM operation WHERE transaction_id = $1 AND account_alias = $2 AND balance_key = $3 AND type = $4`,
			txID, "@label-overdrawn", cn.OverdraftBalanceKey, cn.OVERDRAFT).Scan(&companion))
		assert.Equal(t, strPtr("overdrawn-label"), primary)
		assert.Nil(t, companion, "the companion is a system movement, not a sent leg")
	})

	for _, action := range []string{cn.ActionCommit, cn.ActionCancel} {
		t.Run("a "+action+" of a v1 hold stores the label of each leg", func(t *testing.T) {
			payer, receiver := "@hold-"+action+"-payer", "@hold-"+action+"-receiver"
			h.seedBalance(t, payer, "USD", decimal.NewFromInt(1000), "deposit")
			h.seedBalance(t, receiver, "USD", decimal.Zero, "deposit")

			hold := h.createJSON(t, app, v1LabeledTransfer(payer, receiver, "100", action+"-payer-label", action+"-receiver-label", true), nil)
			require.Equalf(t, 201, hold.status, "hold must post: %s", string(hold.rawBody))
			txID := mustTxID(t, hold)
			require.Equal(t, cn.PENDING, dbTxStatus(t, h.db, txID))

			holdOperations := operationIDs(t, h, txID)
			transition := h.post(t, app, h.statePath(txID, action), "", nil)
			require.Equalf(t, 201, transition.status, "%s must succeed: %s", action, string(transition.rawBody))

			written := 0
			for id, row := range operationRouteLabelsByID(t, h, txID) {
				if holdOperations[id] {
					continue
				}

				written++
				switch row.alias {
				case payer:
					assert.Equal(t, strPtr(action+"-payer-label"), row.label, "%s operation %s of the payer", action, row.opType)
				case receiver:
					assert.Equal(t, strPtr(action+"-receiver-label"), row.label, "%s operation %s of the receiver", action, row.opType)
				default:
					t.Fatalf("unexpected %s operation on %s", action, row.alias)
				}
			}
			require.NotZero(t, written, "the %s must write its own operations", action)
		})
	}

	t.Run("a v1 revert stores the label of the original leg on each reversed operation", func(t *testing.T) {
		h.seedBalance(t, "@revert-label-payer", "USD", decimal.NewFromInt(1000), "deposit")
		h.seedBalance(t, "@revert-label-receiver", "USD", decimal.Zero, "deposit")

		created := h.createJSON(t, app, v1LabeledTransfer("@revert-label-payer", "@revert-label-receiver", "100", "revert-payer-label", "revert-receiver-label", false), nil)
		require.Equalf(t, 201, created.status, "transfer must post: %s", string(created.rawBody))
		parentID := mustTxID(t, created)

		reverted := h.post(t, app, h.statePath(parentID, "revert"), "", nil)
		require.Equalf(t, 201, reverted.status, "revert must succeed: %s", string(reverted.rawBody))
		reverseID := postgresGetChildTx(t, h, parentID)
		require.NotNil(t, reverseID, "the revert must create a child reverse transaction")

		assert.Equal(t, map[string]*string{
			"@revert-label-payer":    strPtr("revert-payer-label"),
			"@revert-label-receiver": strPtr("revert-receiver-label"),
		}, operationRouteLabels(t, h, *reverseID))
	})

	t.Run("the label is independent of the routeId under route validation", func(t *testing.T) {
		routed := h.withSecondLedger(t)
		routes := routed.seedDirectRoutes(t, 1, 1)
		routed.seedBalance(t, "@routed-label-payer", "USD", decimal.NewFromInt(1000), "deposit")
		routed.seedBalance(t, "@routed-label-receiver", "USD", decimal.Zero, "deposit")

		body := fmt.Sprintf(`{"description":"routed and labeled","routeId":%q,"send":{"asset":"USD","value":"100",
			"source":{"from":[{"accountAlias":"@routed-label-payer","amount":{"asset":"USD","value":"100"},"routeId":%q,"route":"routed-payer-label"}]},
			"distribute":{"to":[{"accountAlias":"@routed-label-receiver","amount":{"asset":"USD","value":"100"},"routeId":%q,"route":"routed-receiver-label"}]}}}`,
			routes.transaction, routes.sources[0], routes.destinations[0])

		resp := routed.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "routed and labeled transfer must post: %s", string(resp.rawBody))
		txID := mustTxID(t, resp)

		requireRoutedLegs(t, routed, txID, routes)
		assert.Equal(t, map[string]*string{
			"@routed-label-payer":    strPtr("routed-payer-label"),
			"@routed-label-receiver": strPtr("routed-receiver-label"),
		}, operationRouteLabels(t, routed, txID))
	})
}

// v1LabeledTransfer builds a /v1 JSON transfer of value USD with the given leg
// labels; an empty label omits the field.
func v1LabeledTransfer(payer, receiver, value, payerLabel, receiverLabel string, pending bool) string {
	leg := func(alias, label string) string {
		if label == "" {
			return fmt.Sprintf(`{"accountAlias":%q,"amount":{"asset":"USD","value":%q}}`, alias, value)
		}

		return fmt.Sprintf(`{"accountAlias":%q,"amount":{"asset":"USD","value":%q},"route":%q}`, alias, value, label)
	}

	return fmt.Sprintf(`{"description":"leg route label","pending":%t,"send":{"asset":"USD","value":%q,"source":{"from":[%s]},"distribute":{"to":[%s]}}}`,
		pending, value, leg(payer, payerLabel), leg(receiver, receiverLabel))
}

// operationRouteLabels reads the persisted route label of each default-key
// operation of a transaction, keyed by account alias.
func operationRouteLabels(t *testing.T, h *feeHarness, txID uuid.UUID) map[string]*string {
	t.Helper()

	rows, err := h.db.Query(`SELECT account_alias, route FROM operation WHERE transaction_id = $1 AND balance_key = $2`, txID, cn.DefaultBalanceKey)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	labels := make(map[string]*string)
	for rows.Next() {
		var (
			alias string
			label *string
		)
		require.NoError(t, rows.Scan(&alias, &label))
		_, duplicate := labels[alias]
		require.False(t, duplicate, "one default-key operation per alias expected for %s", alias)
		labels[alias] = label
	}
	require.NoError(t, rows.Err())

	return labels
}

type labeledOperation struct {
	opType, alias string
	label         *string
}

// operationRouteLabelsByID reads every default-key operation of a transaction
// with its persisted route label, keyed by operation ID.
func operationRouteLabelsByID(t *testing.T, h *feeHarness, txID uuid.UUID) map[string]labeledOperation {
	t.Helper()

	rows, err := h.db.Query(`SELECT id, type, account_alias, route FROM operation WHERE transaction_id = $1 AND balance_key = $2`, txID, cn.DefaultBalanceKey)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	operations := make(map[string]labeledOperation)
	for rows.Next() {
		var (
			id  string
			row labeledOperation
		)
		require.NoError(t, rows.Scan(&id, &row.opType, &row.alias, &row.label))
		operations[id] = row
	}
	require.NoError(t, rows.Err())

	return operations
}

func operationIDs(t *testing.T, h *feeHarness, txID uuid.UUID) map[string]bool {
	t.Helper()

	ids := make(map[string]bool)
	for id := range operationRouteLabelsByID(t, h, txID) {
		ids[id] = true
	}

	return ids
}

// responseOperationRoutes maps each operation of a /v1 transaction body to its
// route, keyed by account alias.
func responseOperationRoutes(t *testing.T, body map[string]any) map[string]string {
	t.Helper()

	operations, ok := body["operations"].([]any)
	require.Truef(t, ok, "transaction body must list its operations: %v", body)

	routes := make(map[string]string, len(operations))
	for _, raw := range operations {
		op, ok := raw.(map[string]any)
		require.True(t, ok)
		alias, _ := op["accountAlias"].(string)
		route, _ := op["route"].(string)
		routes[alias] = route
	}

	return routes
}

func getV1TransactionBody(t *testing.T, h *feeHarness, app *fiber.App, txID uuid.UUID) map[string]any {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodGet, "/v1/organizations/"+h.orgID.String()+"/ledgers/"+h.ledgerID.String()+"/transactions/"+txID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, 200, resp.StatusCode, "GET transaction: %s", string(raw))

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))

	return body
}

func strPtr(value string) *string { return &value }
