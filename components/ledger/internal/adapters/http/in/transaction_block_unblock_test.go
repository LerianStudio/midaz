// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildOverriddenTransaction verifies the block/unblock handler builder seam:
//   - the parsed input is built into the same Transaction as the JSON path;
//   - the requested OperationTypeOverride (BLOCK/UNBLOCK) is set on the result;
//   - the Pending flag is forced off so the transaction stays on the direct,
//     single-entry ACTIVE path where the override is honored (invariant from
//     Epic 1.1: the marker is silently dropped on the PENDING/CANCELED
//     double-entry path, so block/unblock must never route through it).
func TestBuildOverriddenTransaction(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}

	newInput := func(pending bool) *mtransaction.CreateTransactionInput {
		return &mtransaction.CreateTransactionInput{
			Description: "block test",
			Pending:     pending,
			Metadata:    map[string]any{"reason": "fraud-hold"},
			Send: mtransaction.Send{
				Asset: "BRL",
			},
		}
	}

	tests := []struct {
		name         string
		override     string
		inputPending bool
	}{
		{name: "block forces non-pending", override: constant.BLOCK, inputPending: false},
		{name: "block strips pending", override: constant.BLOCK, inputPending: true},
		{name: "unblock forces non-pending", override: constant.UNBLOCK, inputPending: false},
		{name: "unblock strips pending", override: constant.UNBLOCK, inputPending: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := newInput(tt.inputPending)

			got := handler.buildOverriddenTransaction(input, tt.override)

			assert.Equal(t, tt.override, got.OperationTypeOverride,
				"override must be set on the built transaction")
			assert.False(t, got.Pending,
				"block/unblock must be a direct ACTIVE transfer, never pending")
			assert.Equal(t, constant.CREATED, got.InitialStatus(),
				"non-pending transaction must resolve to CREATED (single-entry path)")

			// Body fields flow through untouched (same build as the JSON path).
			assert.Equal(t, "block test", got.Description)
			assert.Equal(t, "BRL", got.Send.Asset)
			assert.Equal(t, map[string]any{"reason": "fraud-hold"}, got.Metadata,
				"metadata must flow through untouched")
		})
	}
}

// TestCreateTransactionBlockUnblock_HTTPWiring drives each HTTP handler with a
// non-positive send value, which short-circuits with HTTP 422 before any
// repository call. This proves the handler is wired into the Fiber chain,
// parses CreateTransactionInput, builds the transaction, and delegates to the
// shared createTransaction path (mirrors TestCreateTransactionJSON_NonPositiveValue_Returns422).
func TestCreateTransactionBlockUnblock_HTTPWiring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		route   string
		handler func(handler *TransactionHandler) func(p any, c *fiber.Ctx) error
	}{
		{
			name:  "block",
			route: "block",
			handler: func(handler *TransactionHandler) func(p any, c *fiber.Ctx) error {
				return handler.CreateTransactionBlock
			},
		},
		{
			name:  "unblock",
			route: "unblock",
			handler: func(handler *TransactionHandler) func(p any, c *fiber.Ctx) error {
				return handler.CreateTransactionUnblock
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			orgID := uuid.New()
			ledgerID := uuid.New()

			// No mocks needed: the non-positive value guard short-circuits
			// before any repository call.
			handler := &TransactionHandler{}

			app := fiber.New()
			app.Post(
				"/test/:organization_id/:ledger_id/transactions/"+tt.route,
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				http.WithBody(new(mtransaction.CreateTransactionInput), tt.handler(handler)),
			)

			requestBody := `{
				"pending": true,
				"send": {
					"asset": "USD",
					"value": "0",
					"source": {
						"from": [{"accountAlias": "@source", "amount": {"asset": "USD", "value": "0"}}]
					},
					"distribute": {
						"to": [{"accountAlias": "@dest", "amount": {"asset": "USD", "value": "0"}}]
					}
				}
			}`

			req := httptest.NewRequest("POST",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/"+tt.route,
				strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, 422, resp.StatusCode,
				"non-positive value must short-circuit with 422, proving the handler reached createTransaction")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			err = resp.Body.Close()
			require.NoError(t, err)

			assert.Contains(t, string(body), constant.ErrInvalidTransactionNonPositiveValue.Error(),
				"expected error code for non-positive transaction value")
		})
	}
}
