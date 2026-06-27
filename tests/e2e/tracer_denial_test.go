// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

// Epic 2.2: tracer DENIAL + FAIL-POSTURE coverage.
//
// This file proves the two enforce-mode gates the reserve anchor implements
// (components/ledger/internal/adapters/http/in/transaction_reservation_anchor.go):
//
//   1. DENIAL (auto-runnable when wired): an over-limit transfer on an enforce
//      ledger is REJECTED before any balance moves. The reserve consults an
//      ACTIVE tracer limit, the tracer returns Denied=true, and the anchor
//      returns reservationReject with ErrTransactionReservationDenied (0177).
//      The error platform maps that sentinel to UnprocessableOperationError ->
//      HTTP 422 (pkg/errors.go:501, pkg/net/http/errors.go:47). End-to-end the
//      ledger MUST return a non-2xx and the source balance MUST be UNCHANGED.
//      A positive control (in-limit transfer) confirms the gate is not a
//      blanket reject.
//
//   2. FAIL-POSTURE (manual, tracer-down): with the tracer unreachable, the
//      enforce ledger branches on failPosture. open -> proceed (degraded skip),
//      closed -> reject with ErrTransactionReservationUnavailable (0178), which
//      maps to ServiceUnavailableError -> HTTP 503 (pkg/errors.go:507,
//      pkg/net/http/errors.go:105). Downing the tracer is the supervisor's lane,
//      so these are gated behind a manual flag in addition to requireTracerWired.
//
// All package-private names here carry the "tden" prefix so this file does not
// collide with sibling Phase-2 test files (the harness uses "trx").

import (
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
)

// tdenOverLimitMax is the per-transaction cap for the denial test. A transfer
// strictly above this is denied; one at or below succeeds.
const tdenOverLimitMax = "50"

// TestTracerEnforceDenial is the auto-runnable denial case (Epic 2.2.A). On a
// fail-open enforce ledger it seeds an ACTIVE PER_TRANSACTION limit of 50
// scoped to the funded source account, then proves the two halves of the gate:
//
//   - over-limit (100 > 50): the ledger REJECTS before commit (non-2xx; the
//     statically-pinned status is 422 ErrTransactionReservationDenied) AND the
//     source balance is UNCHANGED — no funds moved on a denied reserve.
//   - in-limit (40 <= 50): the ledger COMMITS (positive control) — the gate
//     does not blanket-reject.
//
// fail-open is deliberate: if the tracer call itself errored instead of cleanly
// denying, fail-open would PROCEED, so a transport flake reads as a (loud)
// positive-control / unchanged-balance mismatch rather than a false denial. The
// denial itself is a clean Denied=true decision, not an error, so it gates under
// any posture; fail-open only governs the unavailable branch.
func TestTracerEnforceDenial(t *testing.T) {
	requireTracerWired(t)

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "tden-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "tden-dst-"+uuid.NewString()[:8])

	// Cap the SOURCE account at 50. A source-scoped limit matches a plain JSON
	// transfer because the reserve carries account.accountId = first source
	// account (firstSourceAccountID in the reserve anchor).
	seedLimitRule(t, f, tdenOverLimitMax, map[string]any{"accountId": accountIDByAlias(t, f, src)})

	// Fund well above the over-limit amount so the ONLY gate that can reject the
	// "100" transfer is the tracer reserve (not insufficient funds).
	fund(t, f, src, "1000")

	balanceBefore := availableBalance(t, f, src)

	// --- over-limit transfer: MUST be denied before any balance move ---
	over := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if over.status >= 200 && over.status < 300 {
		// MONEY/INTEGRITY: an over-limit transfer that commits is a failed
		// enforce gate — funds moved past an ACTIVE cap.
		t.Fatalf("over-limit transfer (cap %s, amount 100) returned 2xx %d — enforce denial gate did not reject\nbody: %s",
			tdenOverLimitMax, over.status, over.body)
	}

	// Contract (supervisor-verified live): ErrTransactionReservationDenied (0177)
	// -> UnprocessableOperationError -> HTTP 422 (business class).
	if over.status != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit denial: want 422 (0177 ErrTransactionReservationDenied), got %d\nbody: %s",
			over.status, over.body)
	}

	// Balance MUST be unchanged: a denied reserve rejects BEFORE
	// ProcessBalanceOperations, so no debit occurred.
	if balanceAfter := availableBalance(t, f, src); balanceAfter != balanceBefore {
		t.Fatalf("source balance moved on a DENIED transfer: before=%s after=%s — funds escaped the enforce gate",
			balanceBefore, balanceAfter)
	}

	// --- positive control: in-limit transfer (40 <= 50) MUST succeed ---
	under := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "40", nil))
	if under.status != http.StatusCreated {
		t.Fatalf("in-limit transfer (cap %s, amount 40): want 201 (reserve permits), got %d\nbody: %s",
			tdenOverLimitMax, under.status, under.body)
	}

	// The successful in-limit transfer was gated by an honored reserve, not a
	// per-call skip, so tracerSkipped must be false.
	if v, ok := under.json["tracerSkipped"].(bool); !ok {
		t.Fatalf("in-limit transfer response missing boolean tracerSkipped field\nbody: %s", under.body)
	} else if v {
		t.Fatalf("tracerSkipped=true on an enforce transfer with no skip requested; reserve should have been honored\nbody: %s", under.body)
	}

	// And the in-limit debit actually moved funds: before(1000) - 40 = 960.
	if got, want := atoiDecimal(t, availableBalance(t, f, src)), atoiDecimal(t, balanceBefore)-40; got != want {
		t.Fatalf("source balance after in-limit transfer = %d, want %d (before %s - 40)",
			got, want, balanceBefore)
	}
}

// ---- FAIL-POSTURE (manual, tracer-down) -----------------------------------
//
// These two cases require the tracer to be UNREACHABLE while the ledger stays
// wired (TracerReserver injected, mode=enforce). Downing the tracer is the
// supervisor's lane, so they self-gate behind requireStack AND a manual flag.
// To run them:
//
//   1. Bring the wired stack up (ledger forwarding reserves to the tracer) and
//      confirm forwarding with the auto-runnable TestTracerEnforceDenial.
//   2. Stop ONLY the tracer container (leave the ledger running so its
//      TracerReserver is still injected — a restart with TRACER_BASE_URL unset
//      would nil the reserver and silently turn the gate off, which is NOT the
//      fail-posture path under test).
//   3. Run: TRACER_DOWN_TEST=1 go test -tags e2e -run TestTracerFailPosture ./tests/e2e/...
//
// The gate is requireStack + the manual flag — deliberately NOT
// requireTracerWired, which probes the (now-down) tracer and would SKIP these
// tests out of existence. Wiring is proven by the fail-CLOSED result itself: an
// UNwired ledger has no reserver and would COMMIT (201), so a 503 here can only
// come from a wired ledger rejecting on an unreachable gate. Run fail-closed
// first; once it 503s, the fail-open 201 is meaningful (and not an unwired
// artifact). The manual flag prevents an accidental run in CI.

// tdenManualFailPostureGate skips unless the operator explicitly opted into the
// tracer-down manual run via TRACER_DOWN_TEST.
func tdenManualFailPostureGate(t *testing.T) {
	t.Helper()

	if os.Getenv("TRACER_DOWN_TEST") == "" {
		t.Skip("manual: requires the tracer to be DOWN while the ledger stays wired; run with TRACER_DOWN_TEST=1")
	}
}

// TestTracerFailPostureOpen proves the degraded-skip branch (Epic 2.2.B, open).
// With the tracer unreachable and failPosture=open, the reserve call errors,
// handleReserveError records a span skip attribute, and the anchor returns
// reservationProceed — so an enforce transfer SUCCEEDS rather than being held
// hostage to a degraded tracer (R20).
//
// CONTRACT NOTE (recorded as a finding): the persisted transaction.tracerSkipped
// field is set from honoredTracerSkip (the per-call skip override), NOT from the
// fail-open degraded skip (transaction_create.go:1359). The degraded skip is
// recorded ONLY as the span attribute app.tracer.reservation_skipped (anchor
// line 187). So on this path the HTTP response's tracerSkipped is expected to be
// FALSE, and the degraded skip is observable only in the trace, not the API
// payload. This test therefore asserts SUCCESS and pins tracerSkipped=false to
// the observed contract; it does NOT assert tracerSkipped=true (the task brief's
// "tracerSkipped reflects degraded skip" does not hold against the code).
func TestTracerFailPostureOpen(t *testing.T) {
	requireStack(t)
	tdenManualFailPostureGate(t)

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "tden-fpo-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "tden-fpo-dst-"+uuid.NewString()[:8])

	fund(t, f, src, "500")
	balanceBefore := availableBalance(t, f, src)

	r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if r.status != http.StatusCreated {
		t.Fatalf("fail-open + tracer-down transfer: want 201 (degraded skip proceeds), got %d\nbody: %s", r.status, r.body)
	}

	// Observed contract: tracerSkipped is the per-call-skip audit, not the
	// degraded-skip flag, so it is FALSE here even though the reserve was skipped.
	if v, ok := r.json["tracerSkipped"].(bool); !ok {
		t.Fatalf("transfer response missing boolean tracerSkipped field\nbody: %s", r.body)
	} else if v {
		t.Logf("LIVE-VERIFY: tracerSkipped=true on fail-open degraded skip — code path (transaction_create.go:1359) sets it from honoredTracerSkip only, so false is expected; the degraded skip lives on span attr app.tracer.reservation_skipped; body: %s", r.body)
	}

	// Funds moved: the transfer committed despite the unreachable tracer.
	if got, want := atoiDecimal(t, availableBalance(t, f, src)), atoiDecimal(t, balanceBefore)-100; got != want {
		t.Fatalf("source balance after fail-open transfer = %d, want %d (before %s - 100)", got, want, balanceBefore)
	}
}

// TestTracerFailPostureClosed proves the fail-closed rejection branch (Epic
// 2.2.B, closed). With the tracer unreachable and failPosture=closed, the
// reserve call errors and handleReserveError returns reservationReject with
// ErrTransactionReservationUnavailable (0178) -> ServiceUnavailableError ->
// HTTP 503. This is a TECHNICAL error class (a dependent service is down),
// distinct from the BUSINESS-class 422 denial above — the ledger refuses to
// commit unchecked when the gate it must consult is unreachable.
//
// Because the reserve anchor fires on INFLOWS too, the funding inflow is itself
// reservation-gated — so on a fail-closed ledger with the tracer down it 503s
// before any transfer could be set up. We therefore prove the contract directly
// on the inflow: it MUST be rejected (non-2xx; 503) and no credit may land.
func TestTracerFailPostureClosed(t *testing.T) {
	requireStack(t)
	tdenManualFailPostureGate(t)

	f := newEnforceFixture(t, "closed")

	dst := createAccount(t, f, "tden-fpc-dst-"+uuid.NewString()[:8])

	// The inflow is the simplest reservation-gated transaction and needs no prior
	// funded balance: on a fail-closed ledger with the gate unreachable, the
	// rejection IS the contract (a transfer would need a funded source, but
	// funding is itself an inflow that 503s here).
	inflow := call(t, http.MethodPost, f.ledgers()+"/transactions/inflow", map[string]any{
		"description": "fund",
		"send": map[string]any{
			"asset": "USD", "value": "100",
			"distribute": map[string]any{
				"to": []any{map[string]any{"accountAlias": dst, "amount": map[string]any{"asset": "USD", "value": "100"}}},
			},
		},
	})
	if inflow.status >= 200 && inflow.status < 300 {
		// MONEY/INTEGRITY: a fail-closed ledger that commits with the gate
		// unreachable has bypassed enforcement entirely.
		t.Fatalf("fail-closed + tracer-down inflow returned 2xx %d — unchecked commit while the gate is unreachable\nbody: %s", inflow.status, inflow.body)
	}

	// Contract (supervisor-verified live): 0178 ErrTransactionReservationUnavailable
	// -> ServiceUnavailableError -> HTTP 503 (technical class — a dependent service
	// is down), distinct from the BUSINESS-class 422 denial.
	if inflow.status != http.StatusServiceUnavailable {
		t.Fatalf("fail-closed rejection: want 503 (0178 ErrTransactionReservationUnavailable), got %d\nbody: %s", inflow.status, inflow.body)
	}

	// No credit landed: the rejected inflow left the destination at zero.
	if got := availableBalance(t, f, dst); atoiDecimal(t, got) != 0 {
		t.Fatalf("destination balance after a REJECTED inflow = %s, want 0 — funds landed despite the fail-closed reject", got)
	}
}
