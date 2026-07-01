// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// fees_matrix_test.go covers Epic 1.1 of the cross-service test plan: the fee
// engine's money math across the fee-model matrix — deductible vs additive
// direction, multi-fee priority + maxBetweenTypes, and min/max eligibility
// boundaries + waivedAccounts — with double-entry reconciliation on every case.
//
// All helpers and types added here are prefixed "feematrix" so they cannot
// collide with the other parallel epic files in package e2e.
//
// Calibrated contracts (verified live before asserting; see findings):
//   - A deductible FLAT fee value must be <= the package minimumAmount, else
//     creation 400s with code 0208 (ErrCalculationValueFlatFee). So a valid
//     deductible flat-10 package needs minimumAmount >= "10".
//   - Deductible direction: sender debited the FULL transfer value, recipient
//     NETS (value - fee), fee account gets the fee; top-level amount stays the
//     original value (NOT fee-inflated).
//   - A deductible fee that MEETS OR EXCEEDS the amount it deducts from is
//     rejected with HTTP 422 / code 0233 (no funds move) — F-FEE-1 fixed. Per-fee
//     creation guards (0208 flat, 0207 percentage) bound a SINGLE fee, so the
//     reachable cases are fee == transfer (single flat) and sum > transfer
//     (accumulated deductible fees); both are pinned below.
//   - maxBetweenTypes charges the larger calculation; additive amount is
//     value + fee.
//   - A priority-2 fee with referenceAmount "afterFeesAmount" computes on
//     value + earlier-priority fees (e.g. 5% of 110 = 5.5, not 5% of 100).
//   - min/max eligibility is INCLUSIVE on both ends: [min, max].
//   - waivedAccounts for a non-deductible fee keys on the SOURCE (From)
//     account, not the destination.

// ---- builders -------------------------------------------------------------

// feematrixFeeSpec is one fee inside a package: its key, application rule,
// calculation type, value, reference amount, priority, deductible flag, and
// the alias it credits. A maxBetweenTypes fee carries two calculations, so
// rule/calcType/value are overridden by Calcs when Calcs is non-nil.
type feematrixFeeSpec struct {
	Key              string
	Rule             string // flatFee | percentual | maxBetweenTypes
	CalcType         string // flat | percentage (ignored when Calcs set)
	Value            string // calculation value (ignored when Calcs set)
	Calcs            []any  // explicit calculations (for maxBetweenTypes)
	ReferenceAmount  string // originalAmount | afterFeesAmount (defaults originalAmount)
	Priority         int
	IsDeductibleFrom bool
	CreditAccount    string
}

// feematrixPackageOpts carries package-level scoping/eligibility knobs.
type feematrixPackageOpts struct {
	MinAmount string // defaults "0"
	MaxAmount string // defaults "100000000"
	Waived    []string
}

// feematrixFeeMap renders one feematrixFeeSpec into the wire shape the package
// API expects.
func feematrixFeeMap(s feematrixFeeSpec) map[string]any {
	calcs := s.Calcs
	if calcs == nil {
		calcs = []any{map[string]any{"type": s.CalcType, "value": s.Value}}
	}

	ref := s.ReferenceAmount
	if ref == "" {
		ref = "originalAmount"
	}

	return map[string]any{
		"feeLabel": "Fee " + s.Key,
		"calculationModel": map[string]any{
			"applicationRule": s.Rule,
			"calculations":    calcs,
		},
		"referenceAmount":  ref,
		"priority":         s.Priority,
		"isDeductibleFrom": s.IsDeductibleFrom,
		"creditAccount":    s.CreditAccount,
	}
}

// feematrixCreatePackage registers an enabled package with the given fees and
// opts, requiring HTTP 201, and returns the decoded package object. Use
// feematrixTryCreatePackage when the creation status itself is under test.
func feematrixCreatePackage(t *testing.T, f fixture, opts feematrixPackageOpts, specs ...feematrixFeeSpec) map[string]any {
	t.Helper()

	r := feematrixTryCreatePackage(t, f, opts, specs...)
	if r.status != http.StatusCreated {
		t.Fatalf("create package: want 201, got %d\nbody: %s", r.status, r.body)
	}

	return r.json
}

// feematrixTryCreatePackage POSTs a package and returns the raw response so the
// caller can assert any status (used to pin the deductible-creation 400).
func feematrixTryCreatePackage(t *testing.T, f fixture, opts feematrixPackageOpts, specs ...feematrixFeeSpec) response {
	t.Helper()

	minAmount := opts.MinAmount
	if minAmount == "" {
		minAmount = "0"
	}

	maxAmount := opts.MaxAmount
	if maxAmount == "" {
		maxAmount = "100000000"
	}

	fees := make(map[string]any, len(specs))
	for _, s := range specs {
		fees[s.Key] = feematrixFeeMap(s)
	}

	body := map[string]any{
		"feeGroupLabel": "E2E Matrix " + uuid.NewString()[:8], "ledgerId": f.ledgerID,
		"minimumAmount": minAmount, "maximumAmount": maxAmount, "enable": true,
		"fees": fees,
	}
	if opts.Waived != nil {
		body["waivedAccounts"] = opts.Waived
	}

	return call(t, http.MethodPost, fmt.Sprintf("%s/v1/organizations/%s/packages", ledgerURL(), f.orgID), body)
}

// feematrixTransfer POSTs a plain JSON transfer of value from->to and requires
// 201, returning the decoded transaction. Reuses transferBody from the harness.
func feematrixTransfer(t *testing.T, f fixture, from, to, value string) map[string]any {
	t.Helper()
	return mustCreate(t, f.ledgers()+"/transactions/json", transferBody(from, to, value, nil))
}

// feematrixReconcile asserts source/destination/fee available balances match
// the expected cents exactly. wantSrc is the source's remaining balance,
// wantDst the destination's, wantFee the fee account's. A -1 skips that check.
func feematrixReconcile(t *testing.T, f fixture, src, dst, fee string, wantSrc, wantDst, wantFee int64) {
	t.Helper()

	if wantSrc >= 0 {
		if got := atoiDecimal(t, availableBalance(t, f, src)); got != wantSrc {
			t.Errorf("source %s available = %d, want %d", src, got, wantSrc)
		}
	}

	if wantDst >= 0 {
		if got := atoiDecimal(t, availableBalance(t, f, dst)); got != wantDst {
			t.Errorf("destination %s available = %d, want %d", dst, got, wantDst)
		}
	}

	if wantFee >= 0 {
		if got := atoiDecimal(t, availableBalance(t, f, fee)); got != wantFee {
			t.Errorf("fee account %s available = %d, want %d", fee, got, wantFee)
		}
	}
}

// feematrixFeeApplied reports whether a transaction's operations contain a
// credit leg for the fee account, distinguishing "fee applied" from "in-range
// but no fee" without depending on per-leg amounts (which carry sub-precision
// residuals on proportional fees).
func feematrixFeeApplied(t *testing.T, txn map[string]any, feeAlias string) bool {
	t.Helper()

	ops, ok := txn["operations"].([]any)
	if !ok {
		t.Fatalf("transaction missing operations: %v", txn)
	}

	for _, raw := range ops {
		op, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if alias, _ := op["accountAlias"].(string); alias == feeAlias {
			return true
		}
	}

	return false
}

// ---- 1.1.1 deductible vs additive direction -------------------------------

// TestFeeDirection proves the two fee directions and pins the deductible
// creation constraint and the over-large-deductible edge.
func TestFeeDirection(t *testing.T) {
	requireStack(t)

	// (a) Additive flat-10: sender debited 110, recipient 100, fee 10, amount 110.
	t.Run("additive flat fee", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, CreditAccount: fee})

		txn := feematrixTransfer(t, f, src, dst, "100")
		if amount := str(t, txn, "amount"); amount != "110" {
			t.Fatalf("additive amount = %s, want 110 (100 + flat 10)", amount)
		}
		// Sender debited 110, recipient gets full 100, fee account gets 10.
		feematrixReconcile(t, f, src, dst, fee, 890, 100, 10)
	})

	// Calibration: a deductible flat fee value must be <= minimumAmount, else
	// creation 400s with code 0208. Pin both the rejection and the valid path.
	t.Run("deductible package creation constraint", func(t *testing.T) {
		f := newFixture(t, false)
		createAccount(t, f, "@fee_income")

		// minimumAmount "0" + flat "10": value (10) > min (0) → 400 / 0208.
		// FINDING (F-FEE-2): a deductible flat fee cannot be created unless the
		// package minimumAmount is >= the flat value; this couples an
		// eligibility floor to a fee-direction flag in a non-obvious way.
		r := feematrixTryCreatePackage(t, f, feematrixPackageOpts{MinAmount: "0"},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, IsDeductibleFrom: true, CreditAccount: "@fee_income"})
		if r.status != http.StatusBadRequest {
			t.Fatalf("deductible flat-10 with min 0: want 400, got %d\nbody: %s", r.status, r.body)
		}
		if code, _ := r.json["code"].(string); code != "0208" {
			t.Errorf("deductible flat-10 with min 0: want code 0208, got %q\nbody: %s", code, r.body)
		}

		// minimumAmount "10" + flat "10": value (10) <= min (10) → 201.
		ok := feematrixTryCreatePackage(t, f, feematrixPackageOpts{MinAmount: "10"},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, IsDeductibleFrom: true, CreditAccount: "@fee_income"})
		if ok.status != http.StatusCreated {
			t.Fatalf("deductible flat-10 with min 10: want 201, got %d\nbody: %s", ok.status, ok.body)
		}
	})

	// A deductible fee with referenceAmount afterFeesAmount must be rejected.
	// At priority 1 the priority-one rule fires first (0188); at priority 2 the
	// deductible rule fires (0205). Pin both.
	t.Run("deductible requires originalAmount", func(t *testing.T) {
		f := newFixture(t, false)
		createAccount(t, f, "@fee_income")

		r1 := feematrixTryCreatePackage(t, f, feematrixPackageOpts{MinAmount: "10"},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", ReferenceAmount: "afterFeesAmount", Priority: 1, IsDeductibleFrom: true, CreditAccount: "@fee_income"})
		if r1.status != http.StatusBadRequest {
			t.Errorf("deductible afterFeesAmount prio1: want 400, got %d\nbody: %s", r1.status, r1.body)
		}
		if code, _ := r1.json["code"].(string); code != "0188" {
			t.Errorf("deductible afterFeesAmount prio1: want code 0188, got %q", code)
		}

		r2 := feematrixTryCreatePackage(t, f, feematrixPackageOpts{MinAmount: "10"},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", ReferenceAmount: "afterFeesAmount", Priority: 2, IsDeductibleFrom: true, CreditAccount: "@fee_income"})
		if r2.status != http.StatusUnprocessableEntity {
			t.Errorf("deductible afterFeesAmount prio2: want 422, got %d\nbody: %s", r2.status, r2.body)
		}
		if code, _ := r2.json["code"].(string); code != "0205" {
			t.Errorf("deductible afterFeesAmount prio2: want code 0205, got %q", code)
		}
	})

	// (b) Deductible flat-10 on a 100 transfer: sender debited the FULL 100,
	// recipient NETS 90, fee account gets 10; top-level amount stays 100.
	t.Run("deductible flat fee direction", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{MinAmount: "10"},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, IsDeductibleFrom: true, CreditAccount: fee})

		txn := feematrixTransfer(t, f, src, dst, "100")
		// Deductible: amount reflects the original transfer, NOT value + fee.
		if amount := str(t, txn, "amount"); amount != "100" {
			t.Fatalf("deductible amount = %s, want 100 (fee comes out of the transfer)", amount)
		}
		// Sender -100, recipient nets 90 (100 - 10 fee), fee account +10.
		feematrixReconcile(t, f, src, dst, fee, 900, 90, 10)
	})

	// Edge: a deductible fee that MEETS OR EXCEEDS the amount it deducts from must
	// not drive the recipient to zero/negative and must not be silently dropped.
	// F-FEE-1: the engine rejects with HTTP 422 (code 0233) and moves no funds.
	//
	// Reachability: a SINGLE flat deductible fee is creation-bounded to
	// value <= package minimumAmount (0208), while amount-eligibility requires
	// transfer >= minimumAmount — so flat <= min <= transfer, and the only
	// single-fee path to the guard is fee EXACTLY EQUAL to the transfer
	// (value == min == transfer). A strictly-larger deductible is reachable only
	// by ACCUMULATION (two valid deductible fees whose sum exceeds the amount).
	// Both are pinned. (transfer-below-min is NOT this case: it is filtered out
	// by min-eligibility and correctly applies no fee.)
	t.Run("deductible fee equal to the transfer is rejected", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		// flat-100 deductible, min 100: transfer 100 is eligible and the fee (100)
		// equals the transfer (100), so the recipient would net zero -> 0233.
		feematrixCreatePackage(t, f, feematrixPackageOpts{MinAmount: "100"},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "100", Priority: 1, IsDeductibleFrom: true, CreditAccount: fee})

		r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
		if r.status != http.StatusUnprocessableEntity {
			t.Fatalf("deductible == transfer: want 422, got %d\nbody: %s", r.status, r.body)
		}
		if code, _ := r.json["code"].(string); code != "0233" {
			t.Errorf("deductible == transfer: want code 0233, got %q\nbody: %s", code, r.body)
		}
		// Rejected outright: source keeps its funds, recipient and fee untouched.
		feematrixReconcile(t, f, src, dst, fee, 1000, 0, 0)
	})

	t.Run("accumulated deductible fees exceeding the transfer are rejected", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		// Two flat-6 deductible fees: each (6) <= minimumAmount (6) so both pass
		// creation, and transfer 10 is eligible, but 6 + 6 = 12 > 10 — the second
		// deductible leg would drive the recipient negative -> 0233.
		feematrixCreatePackage(t, f, feematrixPackageOpts{MinAmount: "6"},
			feematrixFeeSpec{Key: "a", Rule: "flatFee", CalcType: "flat", Value: "6", Priority: 1, IsDeductibleFrom: true, CreditAccount: fee},
			feematrixFeeSpec{Key: "b", Rule: "flatFee", CalcType: "flat", Value: "6", Priority: 2, IsDeductibleFrom: true, CreditAccount: fee})

		r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "10", nil))
		if r.status != http.StatusUnprocessableEntity {
			t.Fatalf("accumulated deductible: want 422, got %d\nbody: %s", r.status, r.body)
		}
		if code, _ := r.json["code"].(string); code != "0233" {
			t.Errorf("accumulated deductible: want code 0233, got %q\nbody: %s", code, r.body)
		}
		// Rejected outright: no partial application, no negative balance.
		feematrixReconcile(t, f, src, dst, fee, 1000, 0, 0)
	})
}

// ---- 1.1.2 multi-fee priority + maxBetweenTypes ---------------------------

// TestFeeMaxBetweenTypes proves maxBetweenTypes charges the larger of its
// calculation types in both directions (max is genuine, not always-percentage).
func TestFeeMaxBetweenTypes(t *testing.T) {
	requireStack(t)

	// [percentage 10, flat 5] on 100 → max is the 10% (=10), not the flat 5.
	t.Run("percentage wins", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{},
			feematrixFeeSpec{Key: "max", Rule: "maxBetweenTypes", Priority: 1, CreditAccount: fee, Calcs: []any{
				map[string]any{"type": "percentage", "value": "10"},
				map[string]any{"type": "flat", "value": "5"},
			}})

		txn := feematrixTransfer(t, f, src, dst, "100")
		if amount := str(t, txn, "amount"); amount != "110" {
			t.Fatalf("maxBetweenTypes [pct10,flat5] amount = %s, want 110 (fee 10)", amount)
		}
		feematrixReconcile(t, f, src, dst, fee, 890, 100, 10)
	})

	// [percentage 1, flat 50] on 100 → max is the flat 50, not the 1% (=1).
	t.Run("flat wins", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{},
			feematrixFeeSpec{Key: "max", Rule: "maxBetweenTypes", Priority: 1, CreditAccount: fee, Calcs: []any{
				map[string]any{"type": "percentage", "value": "1"},
				map[string]any{"type": "flat", "value": "50"},
			}})

		txn := feematrixTransfer(t, f, src, dst, "100")
		if amount := str(t, txn, "amount"); amount != "150" {
			t.Fatalf("maxBetweenTypes [pct1,flat50] amount = %s, want 150 (fee 50)", amount)
		}
		feematrixReconcile(t, f, src, dst, fee, 850, 100, 50)
	})
}

// TestFeeMultiPriority proves a two-fee package applies priority 1 on the
// original amount and priority 2 on the after-fees amount, and that crediting
// the same vs distinct accounts aggregates correctly without losing a leg.
func TestFeeMultiPriority(t *testing.T) {
	requireStack(t)

	// prio1 flat-10 (originalAmount) + prio2 pct-5 (afterFeesAmount), distinct
	// credit accounts. Calibrated: fee1=10 (on 100), fee2=5% of (100+10)=5.5,
	// total fee 15.5, amount 115.5. (Per-leg op amounts carry sub-precision
	// residuals; assert on aggregated balances + top-level amount only.)
	t.Run("two fees distinct credit accounts", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee1 := createAccount(t, f, "@fee1")
		fee2 := createAccount(t, f, "@fee2")
		fund(t, f, src, "1000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{},
			feematrixFeeSpec{Key: "a", Rule: "flatFee", CalcType: "flat", Value: "10", ReferenceAmount: "originalAmount", Priority: 1, CreditAccount: fee1},
			feematrixFeeSpec{Key: "b", Rule: "percentual", CalcType: "percentage", Value: "5", ReferenceAmount: "afterFeesAmount", Priority: 2, CreditAccount: fee2})

		txn := feematrixTransfer(t, f, src, dst, "100")
		// amount = 100 + 10 + 5.5 = 115.5 (priority-2 5% computes on 110).
		if amount := str(t, txn, "amount"); amount != "115.5" {
			t.Fatalf("multi-fee amount = %s, want 115.5 (10 flat + 5%% of 110)", amount)
		}

		// fee1 = 10 (flat on original 100). Asserted as decimal balance.
		if got := availableBalance(t, f, fee1); got != "10" {
			t.Errorf("fee1 balance = %s, want 10", got)
		}
		// fee2 = 5.5 (5% of 100+10 = afterFeesAmount). Decimal, not integer.
		if got := availableBalance(t, f, fee2); got != "5.5" {
			t.Errorf("fee2 balance = %s, want 5.5 (5%% of after-fees 110)", got)
		}
		// Recipient gets the full 100; sender bears principal + both fees.
		if got := availableBalance(t, f, dst); got != "100" {
			t.Errorf("dst balance = %s, want 100", got)
		}
		if got := availableBalance(t, f, src); got != "884.5" {
			t.Errorf("src balance = %s, want 884.5 (1000 - 115.5)", got)
		}
	})

	// Same two fees crediting the SAME account: both legs land, aggregating to
	// 15.5 — no lost leg.
	t.Run("two fees same credit account aggregate", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "1000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{},
			feematrixFeeSpec{Key: "a", Rule: "flatFee", CalcType: "flat", Value: "10", ReferenceAmount: "originalAmount", Priority: 1, CreditAccount: fee},
			feematrixFeeSpec{Key: "b", Rule: "percentual", CalcType: "percentage", Value: "5", ReferenceAmount: "afterFeesAmount", Priority: 2, CreditAccount: fee})

		txn := feematrixTransfer(t, f, src, dst, "100")
		if amount := str(t, txn, "amount"); amount != "115.5" {
			t.Fatalf("same-account multi-fee amount = %s, want 115.5", amount)
		}
		// Both fee legs aggregate onto the one account: 10 + 5.5 = 15.5.
		if got := availableBalance(t, f, fee); got != "15.5" {
			t.Errorf("aggregated fee balance = %s, want 15.5 (10 + 5.5, no lost leg)", got)
		}
		if got := availableBalance(t, f, src); got != "884.5" {
			t.Errorf("src balance = %s, want 884.5", got)
		}
		if got := availableBalance(t, f, dst); got != "100" {
			t.Errorf("dst balance = %s, want 100", got)
		}
	})
}

// ---- 1.1.3 eligibility boundaries + waivedAccounts ------------------------

// TestFeeBoundaries proves the min/max eligibility window is inclusive on both
// ends: a flat-10 fee with min=50, max=200 applies for [50,200] and not below
// 50 or above 200.
func TestFeeBoundaries(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fee := createAccount(t, f, "@fee_income")
	fund(t, f, src, "100000")

	feematrixCreatePackage(t, f, feematrixPackageOpts{MinAmount: "50", MaxAmount: "200"},
		feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, CreditAccount: fee})

	cases := []struct {
		value      string
		wantAmount string
		wantFee    bool
	}{
		{"49", "49", false},   // below min → no fee
		{"50", "60", true},    // at min (inclusive) → fee 10
		{"200", "210", true},  // at max (inclusive) → fee 10
		{"201", "201", false}, // above max → no fee
	}

	for _, tc := range cases {
		t.Run("transfer "+tc.value, func(t *testing.T) {
			txn := feematrixTransfer(t, f, src, dst, tc.value)
			if amount := str(t, txn, "amount"); amount != tc.wantAmount {
				t.Errorf("transfer %s amount = %s, want %s", tc.value, amount, tc.wantAmount)
			}
			if applied := feematrixFeeApplied(t, txn, fee); applied != tc.wantFee {
				t.Errorf("transfer %s fee applied = %v, want %v (boundary is inclusive [min,max])", tc.value, applied, tc.wantFee)
			}
		})
	}
}

// TestFeeWaived proves waivedAccounts exempts a transaction whose SOURCE
// account is waived (the waiver keys on the From side for a non-deductible
// fee), and does not exempt when only the destination is waived.
func TestFeeWaived(t *testing.T) {
	requireStack(t)

	// waived source: fee is exempted and the transaction carries the
	// all_source_accounts_exempt feeExemption metadata.
	t.Run("source account waived exempts the fee", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		other := createAccount(t, f, "@other")
		fund(t, f, src, "100000")
		fund(t, f, other, "100000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{Waived: []string{src}},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, CreditAccount: fee})

		// From the waived source: no fee, amount unchanged, exemption metadata set.
		waived := feematrixTransfer(t, f, src, dst, "100")
		if amount := str(t, waived, "amount"); amount != "100" {
			t.Errorf("waived-source amount = %s, want 100 (no fee)", amount)
		}
		if applied := feematrixFeeApplied(t, waived, fee); applied {
			t.Errorf("waived-source: fee leg should be absent")
		}

		meta, ok := waived["metadata"].(map[string]any)
		if !ok {
			t.Fatalf("waived-source: missing metadata: %v", waived)
		}

		exemption, ok := meta["feeExemption"].(map[string]any)
		if !ok {
			t.Fatalf("waived-source: missing feeExemption metadata: %v", meta)
		}
		if reason, _ := exemption["reason"].(string); reason != "all_source_accounts_exempt" {
			t.Errorf("waived-source feeExemption reason = %q, want all_source_accounts_exempt", reason)
		}

		// From a non-waived source: fee applies.
		normal := feematrixTransfer(t, f, other, dst, "100")
		if amount := str(t, normal, "amount"); amount != "110" {
			t.Errorf("non-waived-source amount = %s, want 110 (fee 10 applies)", amount)
		}
	})

	// waived destination only (source not waived): the fee still applies — the
	// waiver keys on the source/From account, not the destination.
	t.Run("destination waived does not exempt", func(t *testing.T) {
		f := newFixture(t, false)
		src := createAccount(t, f, "@src")
		dst := createAccount(t, f, "@dst")
		fee := createAccount(t, f, "@fee_income")
		fund(t, f, src, "100000")

		feematrixCreatePackage(t, f, feematrixPackageOpts{Waived: []string{dst}},
			feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, CreditAccount: fee})

		txn := feematrixTransfer(t, f, src, dst, "100")
		if amount := str(t, txn, "amount"); amount != "110" {
			t.Errorf("destination-waived amount = %s, want 110 (waiver keys on source, fee still applies)", amount)
		}
		if applied := feematrixFeeApplied(t, txn, fee); !applied {
			t.Errorf("destination-waived: fee leg should be present (source not waived)")
		}
	})
}
