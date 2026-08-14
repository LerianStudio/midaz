// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

// Epic 2.3: RESERVE LIFECYCLE driven by the ledger.
//
// These tests assert that the LEDGER — not a direct tracer caller — drives the
// tracer's two-phase reservation lifecycle across the seam:
//
//   - a PENDING create reserves capacity (long-lived hint) and DEFERS the
//     confirm to /commit and the release to /cancel;
//   - /commit drives ConfirmByTransaction(txid); /cancel drives
//     ReleaseByTransaction(txid) (transaction_state_handlers.go:531-536);
//   - both are addressed by transaction id alone and the tracer treats them as
//     idempotent (a repeat is a 200 no-op with flipped==0).
//
// The ONLY observable signals over HTTP (there is NO GET /v1/reservations/:id):
//   - reserve {denied, reservationIds};
//   - confirm/release-by-transaction {transactionId, status, flipped:int}, where
//     `flipped` counts RESERVED->terminal flips THIS call. After the ledger has
//     already confirmed/released, a re-confirm/re-release returns flipped==0.
//
// All package-private names here carry the "trlc" prefix so this file does not
// collide with the "trx" harness names or sibling Phase-2 test files.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// trlcReserveURL is the tracer reservation collection endpoint.
func trlcReserveURL() string { return tracerURL() + "/v1/reservations" }

// trlcByTxnURL builds the by-transaction confirm/release endpoint the ledger
// /commit and /cancel drive: POST /v1/reservations/transaction/{id}/{action}.
func trlcByTxnURL(txID, action string) string {
	return fmt.Sprintf("%s/transaction/%s/%s", trlcReserveURL(), txID, action)
}

// trlcFlipped extracts the integer `flipped` field (JSON number decodes as
// float64) from a confirm/release-by-transaction response, failing if absent.
// flipped = the count of RESERVED->terminal transitions THIS call performed.
func trlcFlipped(t *testing.T, r response) int {
	t.Helper()

	v, ok := r.json["flipped"]
	if !ok {
		t.Fatalf("missing field %q in %s", "flipped", r.body)
	}

	f, ok := v.(float64)
	if !ok {
		t.Fatalf("field %q is %T, want number: %v", "flipped", v, v)
	}

	return int(f)
}

// trlcSeedPendingTransfer creates a fresh enforce(fail-open) fixture, funds the
// source above maxLimit, seeds an ACTIVE PER_TRANSACTION limit of maxLimit scoped
// to the source account, and posts an in-limit PENDING transfer of `value`. It
// returns the fixture, the funded source alias, and the created transaction id.
//
// The created transaction MUST be PENDING (the create path reserves but defers
// the confirm/release to /commit and /cancel — the lifecycle this epic covers).
func trlcSeedPendingTransfer(t *testing.T, maxLimit, value string) (fixture, string, string) {
	t.Helper()

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "trlc-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "trlc-dst-"+uuid.NewString()[:8])

	// Cap the source account; an in-limit PENDING transfer reserves under it.
	seedLimitRule(t, f, maxLimit, map[string]any{"accountId": accountIDByAlias(t, f, src)})
	fund(t, f, src, "1000")

	body := transferBody(src, dst, value, nil)
	body["pending"] = true

	txn := mustCreate(t, f.ledgers()+"/transactions/json", body)

	if got := str(t, txn["status"].(map[string]any), "code"); got != "PENDING" {
		t.Fatalf("seeded transaction status = %s, want PENDING\nbody: %v", got, txn)
	}

	return f, src, str(t, txn, "id")
}

// TestReserveLifecycleCommitConfirms (Epic 2.3 A) proves the ledger CONFIRMS the
// reservation on /commit. On an enforce ledger with a source-scoped PER_TRANSACTION
// limit, an in-limit PENDING transfer reserves capacity; /commit must drive the
// tracer's ConfirmByTransaction. We prove the ledger already confirmed by
// re-confirming the SAME transaction directly on the tracer: a second confirm is
// an idempotent no-op (flipped==0) precisely because the ledger's /commit already
// flipped the RESERVED reservation to CONFIRMED. flipped>0 here would mean the
// ledger did NOT confirm across the seam (the reservation was still RESERVED when
// our direct call flipped it) — a stuck-RESERVED integrity defect.
func TestReserveLifecycleCommitConfirms(t *testing.T) {
	requireTracerWired(t)

	f, _, txID := trlcSeedPendingTransfer(t, "1000", "100")

	// Ledger drives confirm-by-transaction on /commit.
	r := txnOp(t, f, txID, "commit")
	if r.status != http.StatusCreated {
		t.Fatalf("commit: want 201, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json["status"].(map[string]any), "code"); got != "APPROVED" {
		t.Fatalf("committed status = %s, want APPROVED", got)
	}

	// Re-confirm the same transaction directly on the tracer. If the ledger
	// confirmed across the seam at /commit, the reservation is already CONFIRMED
	// and this is a no-op: flipped==0. flipped>0 means the reservation was still
	// RESERVED — the ledger did NOT confirm — a stuck-RESERVED integrity bug.
	reconfirm := call(t, http.MethodPost, trlcByTxnURL(txID, "confirm"), nil)
	if reconfirm.status != http.StatusOK {
		t.Fatalf("re-confirm-by-transaction: want 200, got %d\nbody: %s", reconfirm.status, reconfirm.body)
	}
	if flipped := trlcFlipped(t, reconfirm); flipped != 0 {
		t.Fatalf("re-confirm flipped = %d, want 0 — the ledger did NOT confirm the reservation on /commit (stuck RESERVED)", flipped)
	}
}

// TestReserveLifecycleCancelReleases (Epic 2.3 B) proves the ledger RELEASES the
// reservation on /cancel. An in-limit PENDING transfer reserves capacity; /cancel
// must drive the tracer's ReleaseByTransaction. We prove the release happened by
// re-releasing the SAME transaction directly: a second release is a no-op
// (flipped==0) because the ledger's /cancel already flipped RESERVED->RELEASED.
// flipped>0 would mean the reservation was still RESERVED after /cancel — a
// stuck-RESERVED integrity defect (capacity never returned).
func TestReserveLifecycleCancelReleases(t *testing.T) {
	requireTracerWired(t)

	f, src, txID := trlcSeedPendingTransfer(t, "1000", "100")

	// Ledger drives release-by-transaction on /cancel.
	r := txnOp(t, f, txID, "cancel")
	if r.status != http.StatusCreated {
		t.Fatalf("cancel: want 201, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json["status"].(map[string]any), "code"); got != "CANCELED" {
		t.Fatalf("canceled status = %s, want CANCELED", got)
	}

	// The cancelled pending never moved funds: the source balance is intact.
	if got := availableBalance(t, f, src); atoiDecimal(t, got) != 1000 {
		t.Fatalf("src available after cancel = %s, want 1000 (funds untouched)", got)
	}

	// Re-release the same transaction directly. If the ledger released across the
	// seam at /cancel, the reservation is already RELEASED and this is a no-op:
	// flipped==0. flipped>0 means the reservation was still RESERVED — the ledger
	// did NOT release — a stuck-RESERVED integrity bug (capacity never returned).
	rerelease := call(t, http.MethodPost, trlcByTxnURL(txID, "release"), nil)
	if rerelease.status != http.StatusOK {
		t.Fatalf("re-release-by-transaction: want 200, got %d\nbody: %s", rerelease.status, rerelease.body)
	}
	if flipped := trlcFlipped(t, rerelease); flipped != 0 {
		t.Fatalf("re-release flipped = %d, want 0 — the ledger did NOT release the reservation on /cancel (stuck RESERVED, capacity leaked)", flipped)
	}
}

// TestReserveLifecycleRevertConfirmsParent (Epic 2.3 B, revert variant) proves the
// revert path does NOT leak or double-handle the parent's reservation. A direct
// (non-PENDING) transfer CONFIRMS its reservation inline at create
// (transaction_create.go:1321-1322). Reverting it creates a NEW reverse
// transaction — it does NOT release the parent's already-CONFIRMED reservation.
// We assert the parent's reservation stays CONFIRMED after revert: a confirm of
// the parent transaction is a no-op (flipped==0), and a release of the parent is
// likewise a no-op (flipped==0) — a CONFIRMED reservation does not flip to
// RELEASED. flipped>0 on either would mean the parent reservation was left in an
// unexpected state across the revert (integrity defect).
func TestReserveLifecycleRevertConfirmsParent(t *testing.T) {
	requireTracerWired(t)

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "trlc-rev-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "trlc-rev-dst-"+uuid.NewString()[:8])

	seedLimitRule(t, f, "1000", map[string]any{"accountId": accountIDByAlias(t, f, src)})
	fund(t, f, src, "1000")

	// Direct (non-pending) transfer: reserve is CONFIRMED inline at create.
	txn := mustCreate(t, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	parentID := str(t, txn, "id")

	r := txnOp(t, f, parentID, "revert")
	if r.status != http.StatusCreated {
		t.Fatalf("revert: want 201, got %d\nbody: %s", r.status, r.body)
	}
	if got := str(t, r.json, "parentTransactionId"); got != parentID {
		t.Fatalf("revert parentTransactionId = %s, want %s", got, parentID)
	}

	// Parent reservation was CONFIRMED at create and revert does not touch it:
	// confirming again is a no-op.
	reconfirm := call(t, http.MethodPost, trlcByTxnURL(parentID, "confirm"), nil)
	if reconfirm.status != http.StatusOK {
		t.Fatalf("parent re-confirm: want 200, got %d\nbody: %s", reconfirm.status, reconfirm.body)
	}
	if flipped := trlcFlipped(t, reconfirm); flipped != 0 {
		t.Fatalf("parent re-confirm flipped = %d, want 0 — parent reservation was not CONFIRMED at create or was disturbed by revert", flipped)
	}

	// A CONFIRMED reservation must not flip to RELEASED on a release call: no-op.
	release := call(t, http.MethodPost, trlcByTxnURL(parentID, "release"), nil)
	if release.status != http.StatusOK {
		t.Fatalf("parent release: want 200, got %d\nbody: %s", release.status, release.body)
	}
	if flipped := trlcFlipped(t, release); flipped != 0 {
		t.Fatalf("parent release flipped = %d, want 0 — a CONFIRMED parent reservation was flipped to RELEASED by revert/release (integrity defect)", flipped)
	}
}

// TestReserveLifecycleConfirmIdempotent (Epic 2.3 C) proves re-confirm by
// transaction is a 200 no-op. After the ledger commits a PENDING transaction
// (confirming the reservation), repeated direct confirms on the tracer must each
// return flipped==0 — there is nothing left to flip. A non-zero flipped on a
// repeat means the tracer is double-counting a terminal reservation, or the
// ledger never confirmed (covered by the A test) — either is an integrity defect.
func TestReserveLifecycleConfirmIdempotent(t *testing.T) {
	requireTracerWired(t)

	f, _, txID := trlcSeedPendingTransfer(t, "1000", "100")

	if r := txnOp(t, f, txID, "commit"); r.status != http.StatusCreated {
		t.Fatalf("commit: want 201, got %d\nbody: %s", r.status, r.body)
	}

	// Two further direct confirms — each a no-op (the ledger already confirmed).
	for i := 0; i < 2; i++ {
		r := call(t, http.MethodPost, trlcByTxnURL(txID, "confirm"), nil)
		if r.status != http.StatusOK {
			t.Fatalf("confirm idempotency call %d: want 200, got %d\nbody: %s", i, r.status, r.body)
		}
		if flipped := trlcFlipped(t, r); flipped != 0 {
			t.Fatalf("confirm idempotency call %d flipped = %d, want 0 (double-count or non-idempotent confirm)", i, flipped)
		}
	}
}

// TestReserveLifecycleReleaseIdempotent (Epic 2.3 C) proves re-release by
// transaction is a 200 no-op. After the ledger cancels a PENDING transaction
// (releasing the reservation), repeated direct releases each return flipped==0.
func TestReserveLifecycleReleaseIdempotent(t *testing.T) {
	requireTracerWired(t)

	f, _, txID := trlcSeedPendingTransfer(t, "1000", "100")

	if r := txnOp(t, f, txID, "cancel"); r.status != http.StatusCreated {
		t.Fatalf("cancel: want 201, got %d\nbody: %s", r.status, r.body)
	}

	for i := 0; i < 2; i++ {
		r := call(t, http.MethodPost, trlcByTxnURL(txID, "release"), nil)
		if r.status != http.StatusOK {
			t.Fatalf("release idempotency call %d: want 200, got %d\nbody: %s", i, r.status, r.body)
		}
		if flipped := trlcFlipped(t, r); flipped != 0 {
			t.Fatalf("release idempotency call %d flipped = %d, want 0 (double-count or non-idempotent release)", i, flipped)
		}
	}
}

// TestReserveLifecycleReserveRequestIdNoDoubleHold (Epic 2.3 C) proves a reserve
// retry carrying the SAME requestId does not double-hold. The ledger derives a
// deterministic requestId per transaction (UUIDv5 of the transactionId,
// transaction_reservation_anchor.go:206), so a retried reserve presents the same
// requestId and the tracer dedups against the prior reservation rather than
// minting a second hold.
//
// We exercise the tracer reserve API directly (the ledger has no retry surface
// over HTTP): two reserves with identical transactionId+requestId+account+amount.
// The dedup contract is that the second reserve returns the SAME reservationIds as
// the first — one logical hold, not two. Distinct ids on the second call would be
// a double-hold (the same logical request reserving capacity twice).
func TestReserveLifecycleReserveRequestIdNoDoubleHold(t *testing.T) {
	requireTracer(t)

	// A source-scoped ACTIVE limit so the reserve actually creates a hold;
	// without a matching limit the tracer returns no reservationIds and there is
	// nothing to dedup. The fixture is enforce(fail-open) only to reuse the
	// limit-seeding helpers; this test drives the tracer directly.
	f := newEnforceFixture(t, "open")
	src := createAccount(t, f, "trlc-rid-src-"+uuid.NewString()[:8])
	accID := accountIDByAlias(t, f, src)
	seedLimitRule(t, f, "1000", map[string]any{"accountId": accID})

	payload := reservePayload("")
	payload["account"] = map[string]any{"accountId": accID}
	payload["amount"] = "100"
	// transactionId + requestId are FIXED so the retry presents the same request.

	first := call(t, http.MethodPost, trlcReserveURL(), payload)
	if first.status != http.StatusCreated {
		t.Fatalf("first reserve: want 201, got %d\nbody: %s", first.status, first.body)
	}
	if denied, _ := first.json["denied"].(bool); denied {
		t.Fatalf("first reserve denied unexpectedly (cap 1000, amount 100)\nbody: %s", first.body)
	}

	firstIDs := trlcReservationIDs(t, first)
	if len(firstIDs) == 0 {
		t.Skipf("first reserve produced no reservationIds (no capacity-backed limit matched) — cannot exercise dedup; body: %s", first.body)
	}

	// Retry with the identical request: same transactionId + requestId.
	second := call(t, http.MethodPost, trlcReserveURL(), payload)
	if second.status != http.StatusCreated {
		t.Fatalf("retry reserve: want 201, got %d\nbody: %s", second.status, second.body)
	}

	secondIDs := trlcReservationIDs(t, second)
	if !trlcSameIDSet(firstIDs, secondIDs) {
		t.Fatalf("reserve retry with the same requestId minted DIFFERENT reservationIds (double-hold)\nfirst:  %v\nsecond: %v", firstIDs, secondIDs)
	}
}

// trlcReservationIDs extracts the reservationIds array (each a string uuid) from a
// reserve response.
func trlcReservationIDs(t *testing.T, r response) []string {
	t.Helper()

	raw, ok := r.json["reservationIds"].([]any)
	if !ok {
		// Absent or null reservationIds => empty set (no hold created).
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("reservationIds element is %T, want string: %v", v, v)
		}
		out = append(out, s)
	}

	return out
}

// trlcSameIDSet reports whether two reservation-id slices contain the same set of
// ids (order-insensitive). Used to assert dedup: a same-requestId retry returns
// the same logical hold, not a fresh one.
func trlcSameIDSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	seen := make(map[string]int, len(a))
	for _, id := range a {
		seen[id]++
	}
	for _, id := range b {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}

	return true
}
