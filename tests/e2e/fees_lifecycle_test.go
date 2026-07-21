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

// This file covers Epic 1.2 (fee package lifecycle & scoping):
//   - Task 1.2.1: fee package cache invalidation on update (value/disable/re-enable).
//   - Task 1.2.2: fee scoping by transaction route and by segment.
//
// All symbols added here are prefixed "feelifecycle" so they cannot collide
// with the other parallel Epic test files sharing this package.
//
// Calibration notes pinned from live curl against the running stack:
//   - PATCH /v1/organizations/{org}/packages/{id} returns 200 and merges the
//     `fees` map field-by-field (sending only a changed calculation is enough);
//     the next transaction reflects the new value — the per-(org,ledger) cache
//     invalidates on update.
//   - Route scoping keys on the transaction's free-form `route` string compared
//     by exact equality against the package's `transactionRoute`. It only takes
//     effect when 2+ packages exist for the (org,ledger): with a single package
//     the engine skips route/segment filtering entirely.
//   - Segment scoping is FUNCTIONAL on the JSON-transaction seam: the fee use
//     case resolves the source account's segment onto cf.SegmentID before
//     calculation, so a segment-scoped package applies only when the source's
//     segment matches — both as the sole package (single-package path now runs
//     the same scope filter) and alongside a coexisting unscoped package.
//   - Combined route+segment scope is a true AND: both legs must match. With the
//     route filter being exact-match (a nil-route package does NOT survive when
//     the transaction carries a route), an unsegmented source on a routed
//     transaction matches neither the combo package (segment leg fails) nor the
//     unscoped package (route leg fails), so no fee is charged.

// ---- helpers (feelifecycle-prefixed) --------------------------------------

// feelifecycleFeeSpec describes one fee package's scoping + flat value for the
// multi-package builder. An empty route or nil segment leaves that scope unset.
type feelifecycleFeeSpec struct {
	groupLabel  string
	creditAlias string
	flatValue   string
	route       string // package transactionRoute; "" leaves it unscoped
	segmentID   string // package segmentId; "" leaves it unscoped
}

// feelifecyclePackagesURL is the org-scoped fee packages collection.
func feelifecyclePackagesURL(f fixture) string {
	return fmt.Sprintf("%s/v1/organizations/%s/packages", ledgerURL(), f.orgID)
}

// feelifecycleCreateFlatPackage registers an enabled flat-fee package with the
// given scoping and returns its id. It mirrors harness createFeePackage but
// adds optional segment/route scope so the multi-package filter path runs.
func feelifecycleCreateFlatPackage(t *testing.T, f fixture, spec feelifecycleFeeSpec) string {
	t.Helper()

	body := map[string]any{
		"feeGroupLabel": spec.groupLabel, "ledgerId": f.ledgerID,
		"minimumAmount": "0", "maximumAmount": "100000000", "enable": true,
		"fees": map[string]any{
			"adminFee": map[string]any{
				"feeLabel": "Admin",
				"calculationModel": map[string]any{
					"applicationRule": "flatFee",
					"calculations":    []any{map[string]any{"type": "flat", "value": spec.flatValue}},
				},
				"referenceAmount":  "originalAmount",
				"priority":         1,
				"isDeductibleFrom": false,
				"creditAccount":    spec.creditAlias,
			},
		},
	}
	if spec.route != "" {
		body["transactionRoute"] = spec.route
	}

	if spec.segmentID != "" {
		body["segmentId"] = spec.segmentID
	}

	pkg := mustCreate(t, feelifecyclePackagesURL(f), body)

	return str(t, pkg, "id")
}

// feelifecyclePatchPackage PATCHes a package and requires the calibrated 200.
// Returns the decoded updated package. The fees map is merged field-by-field,
// so callers send only the fields they want to change.
func feelifecyclePatchPackage(t *testing.T, f fixture, packageID string, body map[string]any) map[string]any {
	t.Helper()

	r := call(t, http.MethodPatch, fmt.Sprintf("%s/%s", feelifecyclePackagesURL(f), packageID), body)
	if r.status != http.StatusOK {
		t.Fatalf("PATCH package %s: want 200, got %d\nbody: %s", packageID, r.status, r.body)
	}

	return r.json
}

// feelifecycleSetFlatValue PATCHes the adminFee flat calculation to newValue.
func feelifecycleSetFlatValue(t *testing.T, f fixture, packageID, newValue string) {
	t.Helper()

	feelifecyclePatchPackage(t, f, packageID, map[string]any{
		"fees": map[string]any{
			"adminFee": map[string]any{
				"calculationModel": map[string]any{
					"applicationRule": "flatFee",
					"calculations":    []any{map[string]any{"type": "flat", "value": newValue}},
				},
			},
		},
	})
}

// feelifecycleSetEnabled PATCHes the package enable flag.
func feelifecycleSetEnabled(t *testing.T, f fixture, packageID string, enabled bool) {
	t.Helper()
	feelifecyclePatchPackage(t, f, packageID, map[string]any{"enable": enabled})
}

// feelifecycleCreateSegment creates a ledger segment and returns its id.
func feelifecycleCreateSegment(t *testing.T, f fixture, name string) string {
	t.Helper()

	seg := mustCreate(t, f.ledgers()+"/segments", map[string]any{"name": name})

	return str(t, seg, "id")
}

// feelifecycleCreateAccountInSegment opens a plain account bound to segmentID
// and returns its alias. segmentID "" creates an unsegmented account.
func feelifecycleCreateAccountInSegment(t *testing.T, f fixture, alias, segmentID string) string {
	t.Helper()

	body := map[string]any{
		"name": "Acct " + alias, "assetCode": "USD", "type": "deposit", "alias": alias,
	}
	if segmentID != "" {
		body["segmentId"] = segmentID
	}

	acc := mustCreate(t, f.ledgers()+"/accounts", body)

	return str(t, acc, "alias")
}

// feelifecycleTransfer posts a JSON transfer of value from->to with an optional
// free-form route string, and returns the decoded transaction. It requires 201.
func feelifecycleTransfer(t *testing.T, f fixture, from, to, value, route string) map[string]any {
	t.Helper()

	send := map[string]any{
		"asset": "USD", "value": value,
		"source":     map[string]any{"from": []any{map[string]any{"accountAlias": from, "amount": map[string]any{"asset": "USD", "value": value}}}},
		"distribute": map[string]any{"to": []any{map[string]any{"accountAlias": to, "amount": map[string]any{"asset": "USD", "value": value}}}},
	}

	body := map[string]any{
		"description": "xfer " + uuid.NewString()[:8],
		"send":        send,
	}
	if route != "" {
		body["route"] = route
	}

	return mustCreate(t, f.ledgers()+"/transactions/json", body)
}

// feelifecycleAmount returns the top-level transaction amount string.
func feelifecycleAmount(t *testing.T, txn map[string]any) string {
	t.Helper()
	return str(t, txn, "amount")
}

// feelifecyclePackageApplied returns the metadata.packageAppliedID, or "" when
// no fee was applied (the metadata key is absent).
func feelifecyclePackageApplied(txn map[string]any) string {
	meta, _ := txn["metadata"].(map[string]any)
	if meta == nil {
		return ""
	}

	id, _ := meta["packageAppliedID"].(string)

	return id
}

// ---- Task 1.2.1: cache invalidation on update -----------------------------

// TestFeeCacheInvalidation proves the per-(org,ledger) fee cache invalidates on
// package update. A flat-10 package warms the cache, then a value bump, a
// disable, and a re-enable each take effect on the very next transaction —
// never a stale value. Every step reconciles the fee account to the cent.
func TestFeeCacheInvalidation(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fee := createAccount(t, f, "@fee_income")
	fund(t, f, src, "100000")

	pkgID := feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "Lifecycle", creditAlias: fee, flatValue: "10",
	})

	// 1) Warm the cache: flat 10 on a 100 transfer -> amount 110, fee 10.
	txn := feelifecycleTransfer(t, f, src, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "110" {
		t.Fatalf("warm: amount = %s, want 110 (100 + flat 10)", got)
	}

	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 10 {
		t.Fatalf("warm: fee account = %s, want 10", got)
	}

	// 2) Bump to flat 20. The cache must invalidate: next transfer charges 20.
	feelifecycleSetFlatValue(t, f, pkgID, "20")

	txn = feelifecycleTransfer(t, f, src, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "120" {
		t.Fatalf("after value bump: amount = %s, want 120 (flat 20, not stale 10)", got)
	}

	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 30 {
		t.Fatalf("after value bump: fee account = %s, want 30 (10 + 20)", got)
	}

	// 3) Disable the package. Next transfer must charge no fee.
	feelifecycleSetEnabled(t, f, pkgID, false)

	txn = feelifecycleTransfer(t, f, src, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "100" {
		t.Fatalf("after disable: amount = %s, want 100 (no fee)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != "" {
		t.Fatalf("after disable: packageAppliedID = %q, want empty (no fee leg)", applied)
	}

	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 30 {
		t.Fatalf("after disable: fee account = %s, want 30 (unchanged)", got)
	}

	// 4) Re-enable. The flat-20 value must come back on the next transfer.
	feelifecycleSetEnabled(t, f, pkgID, true)

	txn = feelifecycleTransfer(t, f, src, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "120" {
		t.Fatalf("after re-enable: amount = %s, want 120 (flat 20 restored)", got)
	}

	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 50 {
		t.Fatalf("after re-enable: fee account = %s, want 50 (30 + 20)", got)
	}
}

// ---- Task 1.2.2: scoping by route and segment -----------------------------

// TestFeeScopingByRoute proves transaction-route scoping. With two packages —
// one scoped to route "ROUTE-A" (flat 10) and one unscoped (flat 5) — a
// transaction carrying route "ROUTE-A" gets the route-scoped package, a
// transaction with no route gets the unscoped one, and a transaction with a
// non-matching route gets NO package (the route filter is exact-match and does
// NOT fall back to the unscoped package).
//
// Scoping only engages on the multi-package filter path; a single package
// bypasses route/segment filtering, so both packages must exist.
func TestFeeScopingByRoute(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	feeA := createAccount(t, f, "@fee_a")
	feeB := createAccount(t, f, "@fee_b")
	fund(t, f, src, "100000")

	pkgA := feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "RouteA", creditAlias: feeA, flatValue: "10", route: "ROUTE-A",
	})
	pkgB := feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "Unscoped", creditAlias: feeB, flatValue: "5",
	})

	// route ROUTE-A -> the route-scoped package (flat 10) wins.
	txn := feelifecycleTransfer(t, f, src, dst, "100", "ROUTE-A")
	if got := feelifecycleAmount(t, txn); got != "110" {
		t.Fatalf("route ROUTE-A: amount = %s, want 110 (route-scoped flat 10)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != pkgA {
		t.Fatalf("route ROUTE-A: packageAppliedID = %s, want %s (route pkg)", applied, pkgA)
	}

	// no route -> the unscoped package (flat 5) wins.
	txn = feelifecycleTransfer(t, f, src, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "105" {
		t.Fatalf("no route: amount = %s, want 105 (unscoped flat 5)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != pkgB {
		t.Fatalf("no route: packageAppliedID = %s, want %s (unscoped pkg)", applied, pkgB)
	}

	// non-matching route ROUTE-X -> NO package applies (no fee). The exact-match
	// route filter drops the unscoped package too, so nothing is charged.
	txn = feelifecycleTransfer(t, f, src, dst, "100", "ROUTE-X")
	if got := feelifecycleAmount(t, txn); got != "100" {
		t.Fatalf("non-matching route: amount = %s, want 100 (no package matches, no fallback to unscoped)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != "" {
		t.Fatalf("non-matching route: packageAppliedID = %q, want empty", applied)
	}

	// Reconcile: fee_a got exactly one 10 (the ROUTE-A txn), fee_b got exactly
	// one 5 (the no-route txn); the ROUTE-X txn charged neither.
	if got := availableBalance(t, f, feeA); atoiDecimal(t, got) != 10 {
		t.Fatalf("fee_a reconcile = %s, want 10", got)
	}

	if got := availableBalance(t, f, feeB); atoiDecimal(t, got) != 5 {
		t.Fatalf("fee_b reconcile = %s, want 5", got)
	}
}

// TestFeeScopingBySegmentSinglePackageHonorsScope proves the single-package path
// honors segment scope: a sole segment-scoped package is applied ONLY when the
// source account's segment matches. An UNSEGMENTED source gets NO fee (the fast
// path now runs the same scope filter as the multi-package path), while a source
// IN the segment gets the fee.
func TestFeeScopingBySegmentSinglePackageHonorsScope(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	segA := feelifecycleCreateSegment(t, f, "Segment A")

	// An unsegmented source: its segment does NOT match the package's segment A.
	plainSrc := feelifecycleCreateAccountInSegment(t, f, "@plain_src", "")
	// A source IN segment A: its segment DOES match.
	segSrc := feelifecycleCreateAccountInSegment(t, f, "@seg_src", segA)
	dst := createAccount(t, f, "@dst")
	fee := createAccount(t, f, "@fee_seg")
	fund(t, f, plainSrc, "100000")
	fund(t, f, segSrc, "100000")

	segPkg := feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "SegA", creditAlias: fee, flatValue: "10", segmentID: segA,
	})

	// Unsegmented source -> scope does NOT match -> no fee, amount stays 100.
	txn := feelifecycleTransfer(t, f, plainSrc, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "100" {
		t.Fatalf("unsegmented source, sole segment-scoped package: amount = %s, want 100 (scope not matched, no fee)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != "" {
		t.Fatalf("unsegmented source: packageAppliedID = %q, want empty (no fee leg)", applied)
	}

	// Segment-A source -> scope matches -> fee 10, amount 110.
	txn = feelifecycleTransfer(t, f, segSrc, dst, "100", "")
	if got := feelifecycleAmount(t, txn); got != "110" {
		t.Fatalf("segment-A source, sole segment-scoped package: amount = %s, want 110 (scope matched, flat 10)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != segPkg {
		t.Fatalf("segment-A source: packageAppliedID = %s, want %s (segment pkg)", applied, segPkg)
	}

	// Reconcile: only the segment-A transfer charged the fee.
	if got := availableBalance(t, f, fee); atoiDecimal(t, got) != 10 {
		t.Fatalf("segment-scoped fee account = %s, want 10 (charged once, for the in-segment source only)", got)
	}
}

// TestFeeScopingBySegmentMultiPackageSelectsScoped proves segment scoping selects
// the matching scoped package over a coexisting unscoped one: a transfer FROM an
// account in segment A selects the segment-A package (fee 10), because the fee
// use case resolves the source segment onto cf.SegmentID and filterBySegmentID
// keeps the segment-A package while dropping the unscoped one for a segmented
// source.
func TestFeeScopingBySegmentMultiPackageSelectsScoped(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	segA := feelifecycleCreateSegment(t, f, "Segment A")

	// Source IS in segment A; scoping picks the segment package.
	src := feelifecycleCreateAccountInSegment(t, f, "@seg_src", segA)
	dst := createAccount(t, f, "@dst")
	feeSeg := createAccount(t, f, "@fee_seg")
	feeStd := createAccount(t, f, "@fee_std")
	fund(t, f, src, "100000")

	segPkg := feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "SegA", creditAlias: feeSeg, flatValue: "10", segmentID: segA,
	})
	feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "Unscoped", creditAlias: feeStd, flatValue: "5",
	})

	txn := feelifecycleTransfer(t, f, src, dst, "100", "")

	// Segment-A source selects the segment-A package (fee 10), not the unscoped one.
	if got := feelifecycleAmount(t, txn); got != "110" {
		t.Fatalf("segment-A source, multi-package: amount = %s, want 110 (segment-A package selected)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != segPkg {
		t.Fatalf("segment-A source, multi-package: packageAppliedID = %s, want %s (segment pkg)", applied, segPkg)
	}

	if got := availableBalance(t, f, feeSeg); atoiDecimal(t, got) != 10 {
		t.Fatalf("segment-scoped fee account = %s, want 10 (charged for the in-segment source)", got)
	}

	if got := availableBalance(t, f, feeStd); atoiDecimal(t, got) != 0 {
		t.Fatalf("unscoped fee account = %s, want 0 (segment source dropped the unscoped pkg)", got)
	}
}

// TestFeeScopingCombinedRouteAndSegment proves combined route+segment scope is a
// true AND: both legs must match. With a package scoped to BOTH route "ROUTE-A"
// and segment A, plus a coexisting unscoped package:
//
//   - An UNSEGMENTED source on route "ROUTE-A" matches NEITHER package and gets
//     no fee. The combo package fails the segment leg (source segment is nil);
//     the unscoped package fails the route leg (its nil route does not survive a
//     transaction that carries a route — the route filter is exact-match, the
//     same behavior TestFeeScopingByRoute pins for a non-matching route). There
//     is no nil-route fallback under a present route, so amount stays 100.
//   - A segment-A source on route "ROUTE-A" matches BOTH legs of the combo
//     package and gets the fee (amount 110).
func TestFeeScopingCombinedRouteAndSegment(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	segA := feelifecycleCreateSegment(t, f, "Segment A")

	// One unsegmented source (fails the combo's segment leg) and one segment-A
	// source (matches both legs of the combo).
	plainSrc := feelifecycleCreateAccountInSegment(t, f, "@plain_src", "")
	segSrc := feelifecycleCreateAccountInSegment(t, f, "@seg_src", segA)
	dst := createAccount(t, f, "@dst")
	feeCombo := createAccount(t, f, "@fee_combo")
	feeStd := createAccount(t, f, "@fee_std")
	fund(t, f, plainSrc, "100000")
	fund(t, f, segSrc, "100000")

	comboPkg := feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "Combo", creditAlias: feeCombo, flatValue: "10", route: "ROUTE-A", segmentID: segA,
	})
	feelifecycleCreateFlatPackage(t, f, feelifecycleFeeSpec{
		groupLabel: "Unscoped", creditAlias: feeStd, flatValue: "5",
	})

	// Unsegmented source on ROUTE-A: combo fails the segment leg, unscoped fails
	// the route leg -> no package matches -> no fee.
	txn := feelifecycleTransfer(t, f, plainSrc, dst, "100", "ROUTE-A")
	if got := feelifecycleAmount(t, txn); got != "100" {
		t.Fatalf("unsegmented source on ROUTE-A: amount = %s, want 100 (combo segment leg fails, unscoped route leg fails, no fee)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != "" {
		t.Fatalf("unsegmented source on ROUTE-A: packageAppliedID = %q, want empty (no package matches)", applied)
	}

	// Segment-A source on ROUTE-A: both legs of the combo match -> fee 10.
	txn = feelifecycleTransfer(t, f, segSrc, dst, "100", "ROUTE-A")
	if got := feelifecycleAmount(t, txn); got != "110" {
		t.Fatalf("segment-A source on ROUTE-A: amount = %s, want 110 (combo matches both route and segment)", got)
	}

	if applied := feelifecyclePackageApplied(txn); applied != comboPkg {
		t.Fatalf("segment-A source on ROUTE-A: packageAppliedID = %s, want %s (combo pkg)", applied, comboPkg)
	}

	// Reconcile: only the segment-A transfer charged the combo fee; the unscoped
	// package was never selected (it loses the route leg on both transfers).
	if got := availableBalance(t, f, feeCombo); atoiDecimal(t, got) != 10 {
		t.Fatalf("combined-scope fee account = %s, want 10 (charged once, for the segment-A source on ROUTE-A)", got)
	}

	if got := availableBalance(t, f, feeStd); atoiDecimal(t, got) != 0 {
		t.Fatalf("unscoped fee account = %s, want 0 (route filter dropped it on both transfers)", got)
	}
}
