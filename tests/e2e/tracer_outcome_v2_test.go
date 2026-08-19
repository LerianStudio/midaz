// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func requireTracerOutcomeV2(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_TRACER_OUTCOME_V2") != "1" {
		t.Skip("requires a Ledger booted with TRACER_OUTCOME_MODE=ledger_outcome_v2 and its outcome worker enabled")
	}
	requireTracerWired(t)
}

// TestTracerOutcomeV2LedgerToTracerAndLostACK is the mandatory black-box gate
// for the durable protocol. The first half crosses the real Ledger -> Tracer
// seam and waits for Tracer's atomic outcome audit before replaying the exact
// receipt. The second half deliberately drops the first ApplyOutcome response,
// waits for its durable audit, and proves the retry returns replayed=true.
func TestTracerOutcomeV2LedgerToTracerAndLostACK(t *testing.T) {
	requireTracerOutcomeV2(t)

	f := newEnforceFixture(t, "closed")
	src := createAccount(t, f, "v2-src-"+uuid.NewString())
	dst := createAccount(t, f, "v2-dst-"+uuid.NewString())
	sourceID := accountIDByAlias(t, f, src)
	fund(t, f, src, "1000")
	limitID := seedCapacityLimitRule(t, f, "100", map[string]any{"accountId": sourceID})

	periodStartedAt := time.Now().UTC()
	transaction := mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	transactionID := str(t, transaction, "id")
	if got := atoiDecimal(t, availableBalance(t, f, src)); got != 900 {
		t.Fatalf("source balance after V2 transfer = %d, want 900", got)
	}
	if got := atoiDecimal(t, availableBalance(t, f, dst)); got != 100 {
		t.Fatalf("destination balance after V2 transfer = %d, want 100", got)
	}

	// The CONFIRMED audit is committed in the same PostgreSQL transaction as the
	// receipt and counter move. Waiting for it proves the Ledger worker, not this
	// test, delivered the terminal outcome.
	ledgerReservationID, ledgerAuditContext := waitForReservationAudit(t, "RESERVATION_CONFIRMED", "", transactionID, 20*time.Second)
	assertReservationAuditContext(t, ledgerAuditContext, ledgerReservationID, transactionID, limitID,
		"acct:"+sourceID, 100, periodStartedAt, time.Now().UTC())
	parsedTransactionID, err := uuid.Parse(transactionID)
	if err != nil {
		t.Fatalf("parse ledger transaction id: %v", err)
	}
	outcomeID := utils.TransactionTracerOutcomeID(parsedTransactionID)
	replay := call(t, http.MethodPost,
		tracerURL()+"/v1/reservations/transaction/"+transactionID+"/outcome",
		map[string]any{"outcomeId": outcomeID.String(), "outcome": "COMMITTED"},
	)
	assertOutcomeReplay(t, replay, transactionID, outcomeID.String(), 1)

	// Once a transaction chose V2, both legacy terminal operations are rejected
	// even after it is terminal; they cannot masquerade as a V1 no-op.
	for _, action := range []string{"confirm", "release"} {
		legacy := call(t, http.MethodPost, trlcByTxnURL(transactionID, action), nil)
		if legacy.status != http.StatusConflict || !strings.Contains(string(legacy.body), "0504") {
			t.Fatalf("legacy %s for V2 transaction: want 409/0504, got %d\nbody: %s", action, legacy.status, legacy.body)
		}
	}

	// A separate direct V2 hold makes the lost-ACK proof deterministic and
	// observable without touching Ledger's already delivered receipt.
	directTransactionID := uuid.New()
	directOutcomeID := uuid.New()
	directAccount := createAccount(t, f, "v2-ack-src-"+uuid.NewString())
	directAccountID := accountIDByAlias(t, f, directAccount)
	directLimitID := seedCapacityLimitRule(t, f, "100", map[string]any{"accountId": directAccountID})
	directPayload := reservePayload("")
	directPayload["transactionId"] = directTransactionID.String()
	directPayload["amount"] = "25"
	directPayload["account"] = map[string]any{"accountId": directAccountID}
	directPayload["deliveryMode"] = "LEDGER_OUTCOME_V2"
	directPeriodStartedAt := time.Now().UTC()
	directReserve := call(t, http.MethodPost, tracerURL()+"/v1/reservations/ledger-outcome-v2", directPayload)
	if directReserve.status != http.StatusCreated {
		t.Fatalf("direct V2 reserve: want 201, got %d\nbody: %s", directReserve.status, directReserve.body)
	}
	if got := str(t, directReserve.json, "deliveryMode"); got != "LEDGER_OUTCOME_V2" {
		t.Fatalf("direct V2 reserve echoed deliveryMode=%q", got)
	}
	directReservationID := firstReservationID(t, directReserve)

	outcomeBody := map[string]any{"outcomeId": directOutcomeID.String(), "outcome": "COMMITTED"}
	postAndDropResponse(t, tracerURL()+"/v1/reservations/transaction/"+directTransactionID.String()+"/outcome", outcomeBody)
	_, directAuditContext := waitForReservationAudit(t, "RESERVATION_CONFIRMED", directReservationID, directTransactionID.String(), 20*time.Second)
	assertReservationAuditContext(t, directAuditContext, directReservationID, directTransactionID.String(), directLimitID,
		"acct:"+directAccountID, 25, directPeriodStartedAt, time.Now().UTC())
	lostACKReplay := call(t, http.MethodPost,
		tracerURL()+"/v1/reservations/transaction/"+directTransactionID.String()+"/outcome", outcomeBody)
	assertOutcomeReplay(t, lostACKReplay, directTransactionID.String(), directOutcomeID.String(), 1)

}

func firstReservationID(t *testing.T, r response) string {
	t.Helper()
	ids, ok := r.json["reservationIds"].([]any)
	if !ok || len(ids) == 0 {
		t.Fatalf("reserve returned no hold: %s", r.body)
	}
	id, ok := ids[0].(string)
	if !ok || id == "" {
		t.Fatalf("reservation id has invalid shape: %v", ids[0])
	}
	return id
}

func assertOutcomeReplay(t *testing.T, r response, transactionID, outcomeID string, minimumReservations int) {
	t.Helper()
	if r.status != http.StatusOK {
		t.Fatalf("ApplyOutcome replay: want 200, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json, "transactionId"); got != transactionID {
		t.Fatalf("receipt transactionId=%s, want %s", got, transactionID)
	}
	if got := str(t, r.json, "outcomeId"); got != outcomeID {
		t.Fatalf("receipt outcomeId=%s, want %s", got, outcomeID)
	}
	if got := str(t, r.json, "outcome"); got != "COMMITTED" {
		t.Fatalf("receipt outcome=%s, want COMMITTED", got)
	}
	if replayed, ok := r.json["replayed"].(bool); !ok || !replayed {
		t.Fatalf("receipt replayed=%v, want true: %s", r.json["replayed"], r.body)
	}
	count, ok := r.json["reservationCount"].(float64)
	if !ok || int(count) < minimumReservations {
		t.Fatalf("receipt reservationCount=%v, want >=%d", r.json["reservationCount"], minimumReservations)
	}
}

func waitForReservationAudit(t *testing.T, eventType, reservationID, transactionID string, timeout time.Duration) (string, map[string]any) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Reservation enums are not yet accepted by the legacy audit-list query
		// validator, so filter the bounded newest page in the assertion instead.
		query := url.Values{"limit": {"1000"}}
		r := call(t, http.MethodGet, tracerURL()+"/v1/audit-events?"+query.Encode(), nil)
		if r.status != http.StatusOK {
			t.Fatalf("list reservation audits: want 200, got %d\nbody: %s", r.status, r.body)
		}
		events, _ := r.json["auditEvents"].([]any)
		for _, raw := range events {
			event, _ := raw.(map[string]any)
			if gotType, _ := event["eventType"].(string); gotType != eventType {
				continue
			}
			if reservationID != "" {
				if gotID, _ := event["resourceId"].(string); gotID != reservationID {
					continue
				}
			}
			ctx, _ := event["context"].(map[string]any)
			if tx, _ := ctx["transactionId"].(string); tx == transactionID {
				id, _ := event["resourceId"].(string)
				return id, ctx
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no %s audit for transaction %s within %s", eventType, transactionID, timeout)
	return "", nil
}

func assertReservationAuditContext(
	t *testing.T,
	ctx map[string]any,
	reservationID, transactionID, limitID, scopeKey string,
	amount int,
	periodStartedAt, periodObservedAt time.Time,
) {
	t.Helper()

	if got, _ := ctx["reservationId"].(string); got != reservationID {
		t.Fatalf("audit reservationId=%q, want %q", got, reservationID)
	}
	if got, _ := ctx["transactionId"].(string); got != transactionID {
		t.Fatalf("audit transactionId=%q, want %q", got, transactionID)
	}
	if got, _ := ctx["limitId"].(string); got != limitID {
		t.Fatalf("audit limitId=%q, want %q", got, limitID)
	}
	if got, _ := ctx["scopeKey"].(string); got != scopeKey {
		t.Fatalf("audit scopeKey=%q, want %q", got, scopeKey)
	}
	if got, _ := ctx["status"].(string); got != "CONFIRMED" {
		t.Fatalf("audit status=%q, want CONFIRMED", got)
	}
	// ReservationAuditContext.Amount is a decimal.Decimal, which serializes
	// to a JSON string in the audit context; compare numerically so scale
	// variants ("100" vs "100.00") stay equal.
	rawAmount, ok := ctx["amount"].(string)
	if !ok {
		t.Fatalf("audit amount=%v (%T), want decimal string", ctx["amount"], ctx["amount"])
	}
	gotAmount, err := decimal.NewFromString(rawAmount)
	if err != nil || !gotAmount.Equal(decimal.NewFromInt(int64(amount))) {
		t.Fatalf("audit amount=%q, want %d", rawAmount, amount)
	}

	gotPeriod, _ := ctx["periodKey"].(string)
	startPeriod := periodStartedAt.UTC().Format(time.DateOnly)
	endPeriod := periodObservedAt.UTC().Format(time.DateOnly)
	if gotPeriod != startPeriod && gotPeriod != endPeriod {
		t.Fatalf("audit periodKey=%q, want %q or %q", gotPeriod, startPeriod, endPeriod)
	}
}

// postAndDropResponse writes a complete HTTP request and closes the socket
// without reading a status line or body. The subsequent audit wait proves the
// server committed despite the caller observing no acknowledgement.
func postAndDropResponse(t *testing.T, rawURL string, body any) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" {
		t.Fatalf("lost-ACK probe requires an http URL, got %q: %v", rawURL, err)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal lost-ACK outcome: %v", err)
	}
	apiKey := os.Getenv("E2E_TRACER_API_KEY")
	if apiKey == "" {
		t.Fatal("E2E_TRACER_API_KEY is required for the lost-ACK outcome probe")
	}
	address := u.Host
	if !strings.Contains(address, ":") {
		address += ":80"
	}
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		t.Fatalf("dial lost-ACK outcome: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	request := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nX-Request-Id: %s\r\nX-API-Key: %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		u.RequestURI(), u.Host, uuid.NewString(), apiKey, len(payload), payload)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write lost-ACK outcome: %v", err)
	}
}
