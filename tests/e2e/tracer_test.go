// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// reservePayload builds a tracer reserve body. transactionType is omitted when
// empty (the ledger's typeless reserve). The timestamp must be a real, in-window
// value — the tracer rejects future / too-far-past timestamps — so this uses the
// wall clock rather than a fixed time.
func reservePayload(transactionType string) map[string]any {
	p := map[string]any{
		"transactionId":        uuid.NewString(),
		"requestId":            uuid.NewString(),
		"amount":               "10",
		"currency":             "USD",
		"account":              map[string]any{"accountId": uuid.NewString()},
		"transactionTimestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if transactionType != "" {
		p["transactionType"] = transactionType
	}
	return p
}

// TestTracerReserveContract exercises the tracer reserve API directly and guards
// the F2 fix: a typeless reserve (what the ledger sends) must succeed, a valid
// type must succeed, and a genuinely invalid type must be a clean 4xx — never a
// 500.
func TestTracerReserveContract(t *testing.T) {
	requireTracer(t)

	reserve := tracerURL() + "/v1/reservations"

	t.Run("empty transactionType reserves (F2 regression — was 500)", func(t *testing.T) {
		r := call(t, http.MethodPost, reserve, reservePayload(""))
		if r.status != http.StatusCreated {
			t.Fatalf("typeless reserve: want 201, got %d\nbody: %s", r.status, r.body)
		}
	})

	t.Run("no account reserves (F3 regression — was 500)", func(t *testing.T) {
		p := reservePayload("")
		delete(p, "account") // external-only source: no internal account UUID
		r := call(t, http.MethodPost, reserve, p)
		if r.status != http.StatusCreated {
			t.Fatalf("accountless reserve: want 201, got %d\nbody: %s", r.status, r.body)
		}
	})

	t.Run("valid transactionType reserves", func(t *testing.T) {
		r := call(t, http.MethodPost, reserve, reservePayload("PIX"))
		if r.status != http.StatusCreated {
			t.Fatalf("PIX reserve: want 201, got %d\nbody: %s", r.status, r.body)
		}
	})

	t.Run("invalid transactionType is 4xx not 5xx", func(t *testing.T) {
		r := call(t, http.MethodPost, reserve, reservePayload("GARBAGE"))
		if r.status != http.StatusBadRequest {
			t.Fatalf("invalid type: want 400, got %d\nbody: %s", r.status, r.body)
		}
	})
}

// TestTracerConfirmByTransactionNoReservations exercises the phase-two
// confirm-by-transaction endpoint the ledger /commit drives: addressing a
// transaction that holds no reservations is an idempotent no-op (flipped=0, 200).
func TestTracerConfirmByTransactionNoReservations(t *testing.T) {
	requireTracer(t)

	txID := uuid.NewString()
	r := call(t, http.MethodPost, tracerURL()+"/v1/reservations/transaction/"+txID+"/confirm", nil)
	if r.status != http.StatusOK {
		t.Fatalf("confirm-by-transaction (no reservations): want 200, got %d\nbody: %s", r.status, r.body)
	}
	if flipped, ok := r.json["flipped"].(float64); !ok || flipped != 0 {
		t.Fatalf("flipped = %v, want 0", r.json["flipped"])
	}
}
