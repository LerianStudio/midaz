// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

// This is the F3 cross-component contract lock between the ledger's outbound
// reserve client and the tracer's reserve validation. It exists because the two
// shapes drifted silently: the ledger sent `account` as a STRING and omitted
// requestId / a valid transactionTimestamp, and the tracer's reserve endpoint —
// which embeds the ValidationRequest shape — rejected it with HTTP 400, so
// `tracer.mode=enforce` never enforced (it fail-open SKIPPED on every
// transaction). See docs/v4/plan/F3-T20-latency-report.md §5 finding #1.
//
// What is REAL on each side here:
//   - LEDGER: the real *TracerClient.Reserve — the actual production marshaling
//     of the outbound ReserveRequest wire body and the actual HTTP POST. This is
//     the side that carried the bug.
//   - TRACER: the real github.com/.../tracer/pkg/model.ValidationRequest JSON
//     parse plus the real ValidateForReserve validation rules — the side that
//     rejected the payload. The httptest endpoint below runs the SAME parse +
//     validate the production tracer reserve handler runs
//     (reservation_handler.go → NormalizeAndReserveValidate →
//     model.ValidateForReserve); on success it returns 201 with the reserve
//     decision, exactly as the handler does.
//
// Why the endpoint is reconstructed rather than the literal tracer handler:
// Go's `internal` rule walls components/tracer/internal/... off from
// components/ledger/..., so the ledger test package physically cannot import the
// tracer's internal ReservationHandler. The load-bearing half of the contract —
// the JSON wire shape and the reserve validation RULES — lives in the tracer's
// non-internal pkg/model and IS imported here, so both real sides meet over the
// real wire. Drift in the tracer's reserve validation OR the ledger's outbound
// shape fails this test (proven by TestReserveContract_DetectsLedgerShapeDrift).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracermodel "github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// tracerReserveEndpoint is the real tracer reserve validation mounted over
// httptest. It parses the body into the tracer's REAL ValidationRequest (+
// transactionId) and runs the tracer's REAL reserve validation rules
// (ValidateForReserve) — the same parse + validation the production handler
// runs. denied/reservationIDs let a test script the post-validation decision so
// the success-decision-flows-back assertion is meaningful.
type tracerReserveEndpoint struct {
	now            time.Time
	denied         bool
	reservationIDs []uuid.UUID

	parsed bool // set true once a body successfully parsed + validated
}

// tracerReserveBody mirrors the tracer's internal ReserveRequest wrapper: the
// ledger transactionId plus the embedded ValidationRequest. The embedded type
// is the tracer's REAL model.ValidationRequest, so its JSON tags (account as the
// AccountContext object, requestId, transactionType, transactionTimestamp) are
// the real tracer contract — the ledger wire body must deserialize into it.
type tracerReserveBody struct {
	TransactionID              uuid.UUID `json:"transactionId"`
	tracermodel.ValidationRequest
}

func (e *tracerReserveEndpoint) handler(w http.ResponseWriter, r *http.Request) {
	var body tracerReserveBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// This is the exact failure the original bug produced: `cannot unmarshal
		// string into ...account of type AccountContext`. A 400 here means the
		// ledger sent a shape the tracer cannot parse.
		writeTracerError(w, http.StatusBadRequest, "TRC-0003", "invalid request body: "+err.Error())
		return
	}

	if body.TransactionID == uuid.Nil {
		writeTracerError(w, http.StatusBadRequest, "TRC-0371", "transactionId is required")
		return
	}

	// The REAL tracer reserve validation rules.
	if err := body.ValidationRequest.ValidateForReserve(e.now); err != nil {
		writeTracerError(w, http.StatusBadRequest, "TRC-0001", "reserve validation failed: "+err.Error())
		return
	}

	e.parsed = true

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ReserveResult{
		TransactionID:  body.TransactionID,
		Denied:         e.denied,
		ReservationIDs: e.reservationIDs,
	})
}

func writeTracerError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

// ledgerStyleReserveRequest builds the reserve request the ledger anchor sends:
// requestId derived from the transactionID, the structured account scope, a
// fee-inclusive amount/currency, and an in-window timestamp. ts must be inside
// the tracer's accept window (not future, within 24h) relative to the
// endpoint's now.
func ledgerStyleReserveRequest(transactionID, requestID uuid.UUID, ts time.Time) ReserveRequest {
	return ReserveRequest{
		TransactionID:        transactionID,
		RequestID:            requestID.String(),
		Amount:               "1000",
		Currency:             "BRL",
		Account:              ReserveAccount{AccountID: uuid.NewString()},
		TransactionTimestamp: ts.UTC().Format(time.RFC3339Nano),
	}
}

// TestReserveContract_LedgerPayloadAcceptedByTracer is the contract lock: the
// real ledger client's outbound reserve body must be ACCEPTED (no 4xx) by the
// real tracer reserve validation, and the reserve decision must flow back. This
// fails if either the ledger outbound shape or the tracer reserve validation
// drifts apart.
func TestReserveContract_LedgerPayloadAcceptedByTracer(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	reservationID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	endpoint := &tracerReserveEndpoint{now: now, reservationIDs: []uuid.UUID{reservationID}}
	srv := httptest.NewServer(http.HandlerFunc(endpoint.handler))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	txID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	reqID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	result, err := client.Reserve(context.Background(), ledgerStyleReserveRequest(txID, reqID, now))

	// The tracer ACCEPTED the ledger payload (no 4xx => no client error, since
	// the client maps any non-201 to an error).
	require.NoError(t, err, "the tracer must ACCEPT the ledger reserve payload; a 4xx here is the contract gap reappearing")
	require.True(t, endpoint.parsed, "the tracer must have parsed + validated the ledger body")

	// The reserve decision flows back.
	require.NotNil(t, result)
	assert.False(t, result.Denied)
	require.Len(t, result.ReservationIDs, 1)
	assert.Equal(t, reservationID, result.ReservationIDs[0])
}

// TestReserveContract_DeniedDecisionFlowsBack proves a DENIED decision (a
// successful 201) round-trips as a business outcome, not a transport error.
func TestReserveContract_DeniedDecisionFlowsBack(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now, denied: true}
	srv := httptest.NewServer(http.HandlerFunc(endpoint.handler))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	txID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	reqID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	result, err := client.Reserve(context.Background(), ledgerStyleReserveRequest(txID, reqID, now))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Denied, "a DENIED limit decision must round-trip as a successful reserve result")
	assert.Empty(t, result.ReservationIDs)
}

// TestReserveContract_AccountlessLedgerPayloadAccepted proves the relaxation:
// the ledger may reserve for an external-only source with no internal account
// UUID. An empty account must still be ACCEPTED (matches non-account-scoped
// limits) rather than 400.
func TestReserveContract_AccountlessLedgerPayloadAccepted(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now}
	srv := httptest.NewServer(http.HandlerFunc(endpoint.handler))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	req := ledgerStyleReserveRequest(
		uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		now,
	)
	req.Account = ReserveAccount{} // external-only source: no internal account UUID

	result, err := client.Reserve(context.Background(), req)

	require.NoError(t, err, "an accountless reserve must be accepted on the relaxed reserve path")
	require.True(t, endpoint.parsed)
	require.NotNil(t, result)
}

// TestReserveContract_DetectsLedgerShapeDrift is the NEGATIVE proof: it
// reconstructs the ORIGINAL buggy ledger wire shape (account as a STRING,
// missing requestId / valid transactionTimestamp) and asserts the real tracer
// validation REJECTS it with a 4xx. This proves the contract lock catches the
// exact drift that caused the F3 gap — if someone reverts the ledger client to
// the old shape, the positive test above breaks and this test documents why.
func TestReserveContract_DetectsLedgerShapeDrift(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now}
	srv := httptest.NewServer(http.HandlerFunc(endpoint.handler))
	defer srv.Close()

	// The ORIGINAL buggy outbound shape: account is a STRING, no requestId, no
	// transactionTimestamp — exactly what the ledger sent at HEAD before this fix.
	originalBuggyBody := map[string]any{
		"transactionId": uuid.MustParse("77777777-7777-7777-7777-777777777777").String(),
		"amount":        "1000",
		"currency":      "BRL",
		"account":       "@source-account", // STRING, not the AccountContext object
		"transactionType": "pending-long-lived", // the invalid enum the old hint smuggled in
	}

	body, err := json.Marshal(originalBuggyBody)
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/v1/reservations", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.GreaterOrEqual(t, resp.StatusCode, 400, "the original buggy ledger shape MUST be rejected by the tracer")
	assert.Less(t, resp.StatusCode, 500, "rejection is a 4xx client error, not a 5xx")
	assert.False(t, endpoint.parsed, "the tracer must NOT have accepted the buggy body")
}
