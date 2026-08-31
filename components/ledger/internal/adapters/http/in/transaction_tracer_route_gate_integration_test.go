// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// The tracer reservation lifecycle is a /v2 contract. These tests drive the real HTTP
// mounts against real containers with the most aggressive tracer configuration a ledger
// can carry — mode=enforce with failPosture=closed, the only combination that can reject
// a create — so a leaked gate surfaces as a 503 or a forbiddenReserver failure rather
// than as an indistinguishable success.
//
// The unit gates prove the seam returns early; these prove the wiring that reaches it is
// correct on every mounted /v1 route.

// seedEnforceClosedTracer writes mode=enforce + failPosture=closed onto the harness
// ledger and drops the settings cache entry, so the next GetParsedLedgerSettings on the
// create funnel reads the new value from Postgres rather than a warm Redis key.
func (h *feeHarness) seedEnforceClosedTracer(t *testing.T) {
	t.Helper()

	_, err := h.db.Exec(
		`UPDATE ledger SET settings = $1::jsonb WHERE organization_id = $2 AND id = $3`,
		`{"tracer":{"mode":"enforce","failPosture":"closed"}}`, h.orgID, h.ledgerID,
	)
	require.NoError(t, err, "seed tracer settings")

	require.NoError(t,
		h.redisRepo.Del(h.ctx(), utils.LedgerSettingsInternalKey(h.orgID, h.ledgerID)),
		"drop the settings cache entry so the funnel re-reads Postgres")
}

// unavailableReserver is a tracer that is down: every Reserve fails with the availability
// sentinel, which under enforce + failPosture=closed rejects the create.
func unavailableReserver() *stubReserver {
	return &stubReserver{reserveErr: fmt.Errorf("dial tcp: %w", tracer.ErrTracerUnavailable)}
}

// TestTracerRouteGate_V1NeverReachesTracer drives every mounted /v1 route that touches
// the reservation lifecycle against a reserver that fails the test if it is called.
func TestTracerRouteGate_V1NeverReachesTracer(t *testing.T) {
	t.Run("create modes and revert", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.handler.TracerReserver = &forbiddenReserver{t: t}
		h.seedEnforceClosedTracer(t)

		app := h.newApp()
		h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

		body := `{"description":"v1 gated","pending":false,"send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"1000"}}]}}}`

		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status,
			"a /v1 create must succeed under enforce+closed: the tracer is not its contract: %s", string(resp.rawBody))

		txID := mustTxID(t, resp)

		reverted := h.post(t, app, h.statePath(txID, "revert"), "", nil)
		require.Equalf(t, 201, reverted.status, "a /v1 revert must succeed under enforce+closed: %s", string(reverted.rawBody))
	})

	t.Run("pending commit", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.handler.TracerReserver = &forbiddenReserver{t: t}
		h.seedEnforceClosedTracer(t)

		app := h.newApp()
		h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

		body := `{"description":"v1 pending","pending":true,"send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"1000"}}]}}}`

		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "a /v1 pending create must succeed: %s", string(resp.rawBody))

		committed := h.post(t, app, h.statePath(mustTxID(t, resp), "commit"), "", nil)
		require.Equalf(t, 201, committed.status, "a /v1 commit must succeed and dial nothing: %s", string(committed.rawBody))
	})

	t.Run("pending cancel", func(t *testing.T) {
		h := setupFeeHarness(t)
		h.handler.TracerReserver = &forbiddenReserver{t: t}
		h.seedEnforceClosedTracer(t)

		app := h.newApp()
		h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

		body := `{"description":"v1 pending cancel","pending":true,"send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"1000"}}]}}}`

		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "a /v1 pending create must succeed: %s", string(resp.rawBody))

		canceled := h.post(t, app, h.statePath(mustTxID(t, resp), "cancel"), "", nil)
		require.Equalf(t, 201, canceled.status, "a /v1 cancel must succeed and dial nothing: %s", string(canceled.rawBody))
	})
}

// TestTracerRouteGate_V2StillEnforces is the other half of the gate: the same ledger, the
// same down tracer, on /v2. Without this the /v1 assertions above could pass for the wrong
// reason — a globally disabled tracer rather than a route-scoped gate.
func TestTracerRouteGate_V2StillEnforces(t *testing.T) {
	h := setupFeeHarness(t)
	reserver := unavailableReserver()
	h.handler.TracerReserver = reserver
	h.seedEnforceClosedTracer(t)

	app := h.newV2App()
	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

	resp := h.createV2Direct(t, app,
		h.v2Body("v2 enforced", "USD", "1000",
			[]string{h.v2Leg("@payer", "1000")},
			[]string{h.v2Leg("@receiver", "1000")}), nil)

	require.Equalf(t, 503, resp.status,
		"a /v2 create must be rejected when the tracer is down under failPosture=closed: %s", string(resp.rawBody))
	assert.Equal(t, "0178", resp.body["code"],
		"the rejection must carry the reservation-unavailable sentinel")
	assert.Equal(t, 1, reserver.reserveCalls, "the /v2 create must actually attempt the reserve")
}

// TestTracerRouteGate_V2CreateCommittedOnV1_SkipsConfirm pins the ACCEPTED trade-off of
// gating the by-transaction confirm on the route version.
//
// A by-transaction confirm/release cannot tell whether the transaction holds
// reservations, so the /v1 gate drops it unconditionally. A PENDING created on /v2 —
// which DID reserve — and committed through /v1 therefore never receives its confirm:
// the reservation stays RESERVED until the TTL reaper releases it, and the committed
// amount is never counted against the usage limit.
//
// This is a decided contract, not a defect: mixing mounts across one transaction
// lifecycle is unsupported (docs/api/SCOPING.md). Closing it needs create-time
// reservation state persisted on the transaction row for the gate to read instead of the
// route version — at which point this test should be inverted, deliberately.
func TestTracerRouteGate_V2CreateCommittedOnV1_SkipsConfirm(t *testing.T) {
	h := setupFeeHarness(t)

	reservationID := uuid.New()
	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
	h.handler.TracerReserver = reserver
	h.seedEnforceClosedTracer(t)

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

	v2App := h.newV2App()

	held := h.createV2Hold(t, v2App,
		h.v2Body("v2 pending", "USD", "1000",
			[]string{h.v2Leg("@payer", "1000")},
			[]string{h.v2Leg("@receiver", "1000")}), nil)
	require.Equalf(t, 201, held.status, "the /v2 hold must succeed: %s", string(held.rawBody))
	require.Equal(t, 1, reserver.reserveCalls, "the /v2 hold must reserve")

	txID := mustTxID(t, held)

	committed := h.post(t, h.newApp(), h.statePath(txID, "commit"), "", nil)
	require.Equalf(t, 201, committed.status, "the /v1 commit must still succeed: %s", string(committed.rawBody))

	assert.Empty(t, reserver.confirmedTxns,
		"ACCEPTED GAP: a /v1 commit sends no confirm even for a /v2-created reservation — the reaper releases it at TTL")
	assert.Empty(t, reserver.releasedTxns, "and nothing is released either; the reservation simply expires")
}
