// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// TestPendingCommit holds a pending transaction (funds reserved, not settled),
// then commits it and proves the balance settles only on commit.
func TestPendingCommit(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	body := transferBody(src, dst, "50", nil)
	body["pending"] = true

	txn := mustCreate(t, f.ledgers()+"/transactions/json", body)
	if got := str(t, txn["status"].(map[string]any), "code"); got != "PENDING" {
		t.Fatalf("pending transaction status = %s, want PENDING", got)
	}

	// While pending, the destination has not received the funds yet.
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 0 {
		t.Fatalf("dst available before commit = %s, want 0", got)
	}

	r := txnOp(t, f, str(t, txn, "id"), "commit")
	if r.status != http.StatusCreated {
		t.Fatalf("commit: want 201, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json["status"].(map[string]any), "code"); got != "APPROVED" {
		t.Fatalf("committed status = %s, want APPROVED", got)
	}

	// Now the destination has the funds.
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 50 {
		t.Fatalf("dst available after commit = %s, want 50", got)
	}
}

// TestPendingCancel holds then cancels a pending transaction; the funds never move.
func TestPendingCancel(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	body := transferBody(src, dst, "30", nil)
	body["pending"] = true
	txn := mustCreate(t, f.ledgers()+"/transactions/json", body)

	r := txnOp(t, f, str(t, txn, "id"), "cancel")
	if r.status != http.StatusCreated {
		t.Fatalf("cancel: want 201, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json["status"].(map[string]any), "code"); got != "CANCELED" {
		t.Fatalf("canceled status = %s, want CANCELED", got)
	}
	if got := availableBalance(t, f, src); atoiDecimal(t, got) != 1000 {
		t.Fatalf("src available after cancel = %s, want 1000 (funds untouched)", got)
	}
}

// TestRevert reverses a settled transaction and proves balances return to their
// pre-transfer state, with the reverse transaction linked to its parent.
func TestRevert(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	txn := mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "200", nil))
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 200 {
		t.Fatalf("dst after transfer = %s, want 200", got)
	}

	r := txnOp(t, f, str(t, txn, "id"), "revert")
	if r.status != http.StatusCreated {
		t.Fatalf("revert: want 201, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json, "parentTransactionId"); got != str(t, txn, "id") {
		t.Fatalf("revert parentTransactionId = %s, want %s", got, str(t, txn, "id"))
	}

	// Balances restored: the reverse moved the 200 back from dst to src.
	if got := availableBalance(t, f, src); atoiDecimal(t, got) != 1000 {
		t.Fatalf("src after revert = %s, want 1000", got)
	}
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 0 {
		t.Fatalf("dst after revert = %s, want 0", got)
	}
}

// TestIdempotencyReplay sends the same transaction body twice. The ledger keys
// idempotency on a hash of the body, so the second call must replay the first
// (same transaction id) and must NOT double-debit.
func TestIdempotencyReplay(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	// A fixed (non-unique) description so both posts hash identically.
	body := map[string]any{
		"description": "idempotent-fixed",
		"send": map[string]any{
			"asset": "USD", "value": "70",
			"source":     map[string]any{"from": []any{map[string]any{"accountAlias": src, "amount": map[string]any{"asset": "USD", "value": "70"}}}},
			"distribute": map[string]any{"to": []any{map[string]any{"accountAlias": dst, "amount": map[string]any{"asset": "USD", "value": "70"}}}},
		},
	}

	first := mustCreate(t, f.ledgers()+"/transactions/json", body)
	second := mustCreate(t, f.ledgers()+"/transactions/json", body)

	if str(t, first, "id") != str(t, second, "id") {
		t.Fatalf("idempotency: second post created a new transaction (%s != %s)", str(t, second, "id"), str(t, first, "id"))
	}

	// Only one debit happened despite two posts.
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 70 {
		t.Fatalf("dst after replay = %s, want 70 (single debit)", got)
	}
}

// TestPercentageFee proves the percentage fee model: a 10% fee on a 100 transfer
// charges 10, so the source pays 110 and the fee account receives 10.
func TestPercentageFee(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fee := createAccount(t, f, "@fee_income")
	fund(t, f, src, "1000")

	createFeePackage(t, f, fee, "percentual", "percentage", "10")

	txn := mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if amount := str(t, txn, "amount"); amount != "110" {
		t.Fatalf("percentage-fee transaction amount = %s, want 110 (100 + 10%% fee)", amount)
	}
	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 10 {
		t.Fatalf("fee account after 10%% fee = %s, want 10", got)
	}
}

// TestInstrumentLinking creates a holder, a ledger account, then a CRM instrument
// binding the account to the holder.
func TestInstrumentLinking(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	holderID := createHolder(t, f.orgID)
	accID := accountID(t, f, "@instrument_acct")

	inst := createInstrument(t, f.orgID, f.ledgerID, holderID, accID)
	if str(t, inst, "accountId") != accID {
		t.Fatalf("instrument not bound to account: got %v", inst["accountId"])
	}
}
