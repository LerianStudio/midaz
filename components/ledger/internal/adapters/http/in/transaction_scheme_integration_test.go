// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The payment scheme is a /v2 contract: a /v2 create accepts it, the ledger persists
// it on the transaction row, returns it on /v2 responses and forwards it to the tracer
// as the reservation's transaction type. These tests drive the real HTTP mounts against
// real containers under mode=enforce + failPosture=closed with a reserver that captures
// every request, so the scheme is asserted on all three surfaces at once — the wire
// response, the Postgres column and the reserve request — and a leaked /v1 scheme
// surfaces as a 400 or a forbiddenReserver failure.

const (
	schemePIX   = "PIX"
	schemeAsset = "USD"
	schemeValue = "1000"
	schemePayer = "@payer"
	schemePayee = "@receiver"
)

// capturingReserver is a stubReserver that also keeps every reserve request it was
// handed, so a test can assert on the fields the ledger forwarded rather than only on
// the number of calls.
type capturingReserver struct {
	stubReserver

	requests []tracer.ReserveRequest
}

// allowingCapturingReserver answers every reserve with one reservation id, the handle
// the ledger later confirms, and records the request.
func allowingCapturingReserver() *capturingReserver {
	return &capturingReserver{
		stubReserver: stubReserver{
			result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{uuid.New()}},
		},
	}
}

func (c *capturingReserver) Reserve(ctx context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	c.requests = append(c.requests, req)

	return c.stubReserver.Reserve(ctx, req)
}

// v2WithScheme attaches a scheme to an assembled v2 body.
func (h *feeHarness) v2WithScheme(body, scheme string) string {
	return strings.TrimSuffix(body, "}") + `,"scheme":"` + scheme + `"}`
}

// schemeTransferBody builds the one transfer every scheme case posts: payer to receiver,
// 1000 USD, against the harness scope.
func (h *feeHarness) schemeTransferBody(description string) string {
	return h.v2Body(description, schemeAsset, schemeValue,
		[]string{h.v2Leg(schemePayer, schemeValue)},
		[]string{h.v2Leg(schemePayee, schemeValue)})
}

// seedSchemeAccounts funds the payer and creates the receiver for schemeTransferBody.
func (h *feeHarness) seedSchemeAccounts(t *testing.T) {
	t.Helper()

	h.seedBalance(t, schemePayer, schemeAsset, decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, schemePayee, schemeAsset, decimal.Zero, "deposit")
}

// persistedScheme reads transaction.scheme straight from Postgres, so the column is
// asserted on its own and not only through the response projection.
func (h *feeHarness) persistedScheme(t *testing.T, txID uuid.UUID) sql.NullString {
	t.Helper()

	var scheme sql.NullString

	err := h.db.QueryRow(`SELECT scheme FROM transaction WHERE id = $1`, txID).Scan(&scheme)
	require.NoError(t, err, "read persisted scheme")

	return scheme
}

// TestIntegration_TransactionScheme_LedgerSeam covers the scheme on every surface the
// /v2 lifecycle reaches — the response, the row and the tracer reserve — and the /v1
// refusal, each case on a harness of its own.
func TestIntegration_TransactionScheme_LedgerSeam(t *testing.T) {
	t.Run("v2 direct with PIX reserves, persists and returns the scheme", func(t *testing.T) {
		h := setupFeeHarness(t)
		reserver := allowingCapturingReserver()
		h.handler.Command.TracerReserver = reserver
		h.seedEnforceClosedTracer(t)

		app := h.newV2App()
		h.seedSchemeAccounts(t)

		resp := h.createV2Direct(t, app, h.v2WithScheme(h.schemeTransferBody("v2 direct PIX"), schemePIX), nil)
		require.Equalf(t, 201, resp.status, "a /v2 direct create with a valid scheme must succeed: %s", string(resp.rawBody))

		txID := mustTxID(t, resp)

		require.Len(t, reserver.requests, 1, "the /v2 create must reserve exactly once")
		assert.Equal(t, txID, reserver.requests[0].TransactionID, "the reserve must name the created transaction")
		assert.Equal(t, schemePIX, reserver.requests[0].TransactionType, "the reserve must carry the declared scheme as the transaction type")
		assert.False(t, reserver.requests[0].Revert, "a create is not a revert")
		assert.False(t, reserver.requests[0].LongLived, "a direct create does not ask for a long-lived reservation")

		persisted := h.persistedScheme(t, txID)
		require.True(t, persisted.Valid, "transaction.scheme must be written for a declared scheme")
		assert.Equal(t, schemePIX, persisted.String, "transaction.scheme must hold the declared scheme")

		assert.Equal(t, schemePIX, resp.body["scheme"], "the /v2 response must return the declared scheme")
	})

	t.Run("v2 revert reserves with the original scheme", func(t *testing.T) {
		h := setupFeeHarness(t)
		reserver := allowingCapturingReserver()
		h.handler.Command.TracerReserver = reserver
		h.seedEnforceClosedTracer(t)

		app := h.newV2App()
		h.seedSchemeAccounts(t)

		created := h.createV2Direct(t, app, h.v2WithScheme(h.schemeTransferBody("v2 revert PIX"), schemePIX), nil)
		require.Equalf(t, 201, created.status, "the origin create must succeed: %s", string(created.rawBody))
		require.Len(t, reserver.requests, 1, "the origin create must reserve once")

		originID := mustTxID(t, created)

		reverted := h.post(t, app, h.v2StatePath(originID, "revert"), "", nil)
		require.Equalf(t, 201, reverted.status, "the /v2 revert must succeed: %s", string(reverted.rawBody))

		revertID := mustTxID(t, reverted)

		require.Len(t, reserver.requests, 2, "the /v2 revert must reserve capacity of its own")
		assert.Equal(t, revertID, reserver.requests[1].TransactionID, "the second reserve must name the revert transaction")
		assert.Equal(t, schemePIX, reserver.requests[1].TransactionType, "the revert must reserve under the origin's scheme")
		assert.True(t, reserver.requests[1].Revert, "the revert reserve must be flagged as a revert")

		persisted := h.persistedScheme(t, revertID)
		require.True(t, persisted.Valid, "the revert row must inherit the origin's scheme")
		assert.Equal(t, schemePIX, persisted.String, "the revert row must carry the origin's scheme verbatim")
		assert.Equal(t, schemePIX, reverted.body["scheme"], "the /v2 revert response must return the inherited scheme")
	})

	t.Run("v2 direct without scheme reserves empty and persists NULL", func(t *testing.T) {
		h := setupFeeHarness(t)
		reserver := allowingCapturingReserver()
		h.handler.Command.TracerReserver = reserver
		h.seedEnforceClosedTracer(t)

		app := h.newV2App()
		h.seedSchemeAccounts(t)

		resp := h.createV2Direct(t, app, h.schemeTransferBody("v2 direct no scheme"), nil)
		require.Equalf(t, 201, resp.status, "a /v2 direct create without a scheme must succeed: %s", string(resp.rawBody))

		txID := mustTxID(t, resp)

		require.Len(t, reserver.requests, 1, "the /v2 create must reserve exactly once")
		assert.Empty(t, reserver.requests[0].TransactionType, "an undeclared scheme must reach the tracer as an empty transaction type")

		assert.False(t, h.persistedScheme(t, txID).Valid, "transaction.scheme must be NULL when no scheme was declared")

		_, hasScheme := resp.body["scheme"]
		assert.False(t, hasScheme, "the /v2 response must not publish a scheme key when none was declared: %s", string(resp.rawBody))
	})

	t.Run("v2 direct with a malformed scheme is rejected before any reserve", func(t *testing.T) {
		h := setupFeeHarness(t)
		reserver := allowingCapturingReserver()
		h.handler.Command.TracerReserver = reserver
		h.seedEnforceClosedTracer(t)

		app := h.newV2App()
		h.seedSchemeAccounts(t)

		resp := h.createV2Direct(t, app, h.v2WithScheme(h.schemeTransferBody("v2 direct pix!"), "pix!"), nil)
		require.Equalf(t, 400, resp.status, "a scheme the shared rule refuses must be rejected by validation: %s", string(resp.rawBody))

		assert.Empty(t, reserver.requests, "a request refused by validation must never reserve")
	})

	t.Run("v2 direct with a padded lower-case scheme stores and reserves it normalized", func(t *testing.T) {
		h := setupFeeHarness(t)
		reserver := allowingCapturingReserver()
		h.handler.Command.TracerReserver = reserver
		h.seedEnforceClosedTracer(t)

		app := h.newV2App()
		h.seedSchemeAccounts(t)

		resp := h.createV2Direct(t, app, h.v2WithScheme(h.schemeTransferBody("v2 direct padded pix"), " pix "), nil)
		require.Equalf(t, 201, resp.status, "a scheme the shared rule normalizes must be accepted: %s", string(resp.rawBody))

		txID := mustTxID(t, resp)

		require.Len(t, reserver.requests, 1, "the /v2 create must reserve exactly once")
		assert.Equal(t, schemePIX, reserver.requests[0].TransactionType, "the reserve must carry the normalized scheme")

		persisted := h.persistedScheme(t, txID)
		require.True(t, persisted.Valid, "transaction.scheme must be written for a declared scheme")
		assert.Equal(t, schemePIX, persisted.String, "transaction.scheme must hold the normalized scheme")

		assert.Equal(t, schemePIX, resp.body["scheme"], "the /v2 response must return the normalized scheme")
	})

	t.Run("v1 json naming scheme is rejected and dials nothing", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.handler.Command.TracerReserver = &forbiddenReserver{t: t}
		h.seedEnforceClosedTracer(t)

		app := h.newApp()
		h.seedSchemeAccounts(t)

		body := `{"description":"v1 scheme","pending":false,"scheme":"PIX","send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"1000"}}]}}}`

		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 400, resp.status, "a /v1 body naming scheme must be refused as an unknown field: %s", string(resp.rawBody))
		assert.Equal(t, cn.ErrUnexpectedFieldsInTheRequest.Error(), resp.body["code"],
			"the refusal must carry the unexpected-fields sentinel")
	})

	t.Run("v2 hold with PIX reserves once and commit confirms by transaction", func(t *testing.T) {
		h := setupFeeHarness(t)
		reserver := allowingCapturingReserver()
		h.handler.Command.TracerReserver = reserver
		h.seedEnforceClosedTracer(t)

		app := h.newV2App()
		h.seedSchemeAccounts(t)

		held := h.createV2Hold(t, app, h.v2WithScheme(h.schemeTransferBody("v2 hold PIX"), schemePIX), nil)
		require.Equalf(t, 201, held.status, "a /v2 hold with a valid scheme must succeed: %s", string(held.rawBody))

		txID := mustTxID(t, held)

		require.Len(t, reserver.requests, 1, "the /v2 hold must reserve exactly once")
		assert.Equal(t, schemePIX, reserver.requests[0].TransactionType, "the hold reserve must carry the declared scheme")
		assert.True(t, reserver.requests[0].LongLived, "a pending create must ask for a long-lived reservation")
		assert.Equal(t, schemePIX, held.body["scheme"], "the /v2 hold response must return the declared scheme")

		committed := h.post(t, app, h.v2StatePath(txID, "commit"), "", nil)
		require.Equalf(t, 201, committed.status, "the /v2 commit must succeed: %s", string(committed.rawBody))

		assert.Len(t, reserver.requests, 1, "a commit confirms the hold's reservation and never reserves again")
		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTxns, "the commit must confirm by the transaction id")
		assert.Empty(t, reserver.releasedTxns, "a commit releases nothing")
		assert.Equal(t, schemePIX, committed.body["scheme"], "the committed transaction keeps its scheme")
	})
}
