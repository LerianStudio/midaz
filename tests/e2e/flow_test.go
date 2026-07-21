// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// TestFullLedgerFlow walks the whole money path end to end and asserts the
// double-entry invariants at each step: provision the hierarchy, fund an
// account, transfer between accounts, then bring CRM (holder-owned account)
// and fees (a flat-fee package) into the picture and prove the fee leg lands
// in the transaction.
func TestFullLedgerFlow(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)

	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fee := createAccount(t, f, "@fee_income")

	// Fund the source with 1000 from the external account.
	fund(t, f, src, "1000")
	if got := availableBalance(t, f, src); got != "1000" {
		t.Fatalf("after funding: src available = %s, want 1000", got)
	}

	// Plain transfer of 100, no fees configured yet.
	mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if got := availableBalance(t, f, src); atoiDecimal(t, got) != 900 {
		t.Fatalf("after transfer: src available = %s, want 900", got)
	}
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 100 {
		t.Fatalf("after transfer: dst available = %s, want 100", got)
	}

	// CRM path: a holder, then a holder-owned account (CRM-composed endpoint).
	holderID := createHolder(t, f.orgID)
	owned := createHolderAccount(t, f, holderID)
	if str(t, owned, "holderId") != holderID {
		t.Fatalf("holder-owned account not bound to holder: %v", owned)
	}

	// Fees path: register a flat 10-USD fee, then transfer 100 and prove the
	// fee leg applied — source pays 110, dest receives 100, fee account gets 10,
	// and the transaction records the package it applied.
	createFlatFeePackage(t, f, fee, "10")

	txn := mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if amount := str(t, txn, "amount"); amount != "110" {
		t.Fatalf("fee-bearing transaction amount = %s, want 110 (100 + 10 fee)", amount)
	}

	meta, _ := txn["metadata"].(map[string]any)
	if meta == nil || meta["packageAppliedID"] == nil {
		t.Fatalf("fee-bearing transaction missing metadata.packageAppliedID: %v", txn["metadata"])
	}

	// src: 900 - 110 = 790 ; dst: 100 + 100 = 200 ; fee: 0 + 10 = 10
	if got := availableBalance(t, f, src); atoiDecimal(t, got) != 790 {
		t.Fatalf("after fee transfer: src available = %s, want 790", got)
	}
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 200 {
		t.Fatalf("after fee transfer: dst available = %s, want 200", got)
	}
	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 10 {
		t.Fatalf("after fee transfer: fee account available = %s, want 10", got)
	}
}

// TestSkipFlagsExplicitFalseAccepted is the regression guard for the
// unknown-field validator bug: an omitempty bool skip flag sent as the explicit
// (and default-safe) `false` must be accepted, not rejected as an unexpected
// field. Pre-fix the API returns 400 (code 0053); post-fix it returns 201.
//
// `false` means "do not skip", so it is a no-op and must succeed regardless of
// whether the ledger opted into the override.
func TestSkipFlagsExplicitFalseAccepted(t *testing.T) {
	requireStack(t)

	f := newFixture(t, true)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	cases := []map[string]any{
		{"fees": false},
		{"tracer": false},
		{"fees": false, "tracer": false},
	}

	for _, skip := range cases {
		r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "10", skip))
		if r.status != http.StatusCreated {
			t.Errorf("transfer with skip=%v: want 201, got %d\nbody: %s", skip, r.status, r.body)
		}
	}

	// The same class on the account path: skip.holder=false must be accepted.
	r := call(t, http.MethodPost, f.ledgers()+"/accounts", map[string]any{
		"name": "skip-false", "assetCode": "USD", "type": "deposit", "alias": "@skipfalse",
		"skip": map[string]any{"holder": false},
	})
	if r.status != http.StatusCreated {
		t.Errorf("account with skip.holder=false: want 201, got %d\nbody: %s", r.status, r.body)
	}
}

// TestSkipFeesTrueBypassesFee proves the happy path of the skip feature: on a
// ledger that opts in (allowFeeSkip), skip.fees=true bypasses an otherwise
// matching fee package — the transaction settles with no fee leg.
func TestSkipFeesTrueBypassesFee(t *testing.T) {
	requireStack(t)

	f := newFixture(t, true)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fee := createAccount(t, f, "@fee_income")
	fund(t, f, src, "1000")
	createFlatFeePackage(t, f, fee, "10")

	txn := mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", map[string]any{"fees": true}))
	if skipped, _ := txn["feesSkipped"].(bool); !skipped {
		t.Fatalf("skip.fees=true: feesSkipped = %v, want true", txn["feesSkipped"])
	}
	if amount := str(t, txn, "amount"); amount != "100" {
		t.Fatalf("skip.fees=true: amount = %s, want 100 (no fee)", amount)
	}
	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 0 {
		t.Fatalf("skip.fees=true: fee account = %s, want 0 (no fee charged)", got)
	}
}

// TestSkipWithoutOverrideRejected proves the fail-closed half of the two-key
// model: requesting a skip on a ledger that did NOT opt in is rejected with
// 422, not silently honored.
func TestSkipWithoutOverrideRejected(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false) // no overrides
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "10", map[string]any{"fees": true}))
	if r.status != http.StatusUnprocessableEntity {
		t.Fatalf("skip.fees=true without override: want 422, got %d\nbody: %s", r.status, r.body)
	}
}
