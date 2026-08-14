// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

// Epic 3.3: tracer gRPC + mTLS TRANSPORT coverage.
//
// The transport between the ledger and the tracer (REST vs gRPC) is INTERNAL to
// the ledger: it is a wire detail behind handler.TracerReserver, swapped at
// bootstrap by TRACER_TRANSPORT. The HTTP-layer enforce contract (over-limit ->
// 422 0177, balance unchanged; in-limit -> 201, tracerSkipped=false) is defined
// in terms of the reserve DECISION, not the transport that carried it. These
// tests re-run the Phase 2 denial contract against a gRPC+mTLS-wired ledger to
// prove NO cross-transport drift: the same observable HTTP behavior must hold
// whether the reserve crossed REST or gRPC.
//
// LIVE-VERIFY: no cert-provisioning script exists in the repo, so these tests
// COMPILE and SKIP cleanly by default. To run them live the supervisor must
// stand up the full mTLS cert chain and flip the ledger transport:
//   - a CA, a ledger-CLIENT cert/key, and a tracer-SERVER cert/key, all chaining
//     to that CA (see bootstrap/tls_seam.go on BOTH sides for the loader);
//   - on the LEDGER: TRACER_TLS_CERT_FILE / TRACER_TLS_KEY_FILE / TRACER_TLS_CA_FILE
//     (client identity + the CA that signed the tracer server cert), plus
//     TRACER_TRANSPORT=grpc, TRACER_TLS_MODE=mtls, TRACER_GRPC_PORT;
//   - on the TRACER: its server cert/key + TRACER_TLS_CLIENT_CA_FILE (the CA that
//     signed the ledger client cert) so it can verify the peer.
//   boot fails fast when cert material is missing or unreadable, so a
//   MISconfigured run cannot false-pass — it never starts.
//
// All package-private names here carry the "tgrpc" prefix so this file does not
// collide with sibling Phase-2 / Phase-3 test files in the same e2e package.
// The Phase 2 tracer helpers (seedLimitRule, newEnforceFixture,
// accountIDByAlias) live in tracer_harness_test.go and are REUSED, not redefined.

import (
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// tgrpcOverLimitMax is the per-transaction cap for the gRPC denial mirror. A
// transfer strictly above this is denied; one at or below succeeds. Same value
// as the Phase 2 REST denial test so the two transports are compared on an
// identical cap.
const tgrpcOverLimitMax = "50"

// tgrpcGRPCOnce caches the one-time manual-gate read so the skip reason is
// computed once for the whole suite run.
var (
	tgrpcGRPCOnce    sync.Once
	tgrpcGRPCEnabled bool
)

// tgrpcRequireGRPC skips the calling test unless the operator explicitly opted
// into the gRPC+mTLS transport run via E2E_TRACER_GRPC=1. This is a MANUAL gate,
// not a behavioral probe: standing up the cert chain and flipping
// TRACER_TRANSPORT=grpc is the supervisor's lane (no provisioning script exists),
// so on the default REST/no-cert dev stack these tests SKIP cleanly with zero
// failures.
func tgrpcRequireGRPC(t *testing.T) {
	t.Helper()

	tgrpcGRPCOnce.Do(func() {
		tgrpcGRPCEnabled = os.Getenv("E2E_TRACER_GRPC") == "1"
	})

	if !tgrpcGRPCEnabled {
		t.Skip("manual: requires the ledger talking to the tracer over gRPC+mTLS (TRACER_TRANSPORT=grpc, TRACER_TLS_MODE=mtls + a CA/client/server cert chain — see file-top LIVE-VERIFY); run with E2E_TRACER_GRPC=1")
	}
}

// TestTracerGRPCOverLimitDenied mirrors the Phase 2 TestTracerEnforceDenial
// EXACTLY, but against a ledger whose reserve crosses gRPC+mTLS instead of REST.
// The point is cross-transport invariance: the HTTP-layer enforce contract must
// be identical regardless of the internal transport.
//
//   - over-limit (100 > 50): the ledger REJECTS before commit. Contract pinned
//     to 422 (0177 ErrTransactionReservationDenied -> UnprocessableOperationError)
//     AND the source balance is UNCHANGED — no funds moved on a denied reserve.
//   - in-limit (40 <= 50): the ledger COMMITS (positive control) — the gate does
//     not blanket-reject, and tracerSkipped is false (reserve honored over gRPC).
//
// fail-open is deliberate (matching Phase 2): a transport-LEVEL error on the
// gRPC+mTLS hop would PROCEED rather than reject, so a TLS handshake/transport
// flake surfaces as a (loud) positive-control / unchanged-balance mismatch, not
// a false denial. The denial itself is a clean Denied=true decision carried over
// the wire — independent of posture — so this still gates under fail-open.
func TestTracerGRPCOverLimitDenied(t *testing.T) {
	requireStack(t)
	tgrpcRequireGRPC(t)

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "tgrpc-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "tgrpc-dst-"+uuid.NewString()[:8])

	// Cap the SOURCE account at 50. A source-scoped limit matches a plain JSON
	// transfer because the reserve carries account.accountId = first source
	// account — the SAME reserve payload regardless of transport.
	seedLimitRule(t, f, tgrpcOverLimitMax, map[string]any{"accountId": accountIDByAlias(t, f, src)})

	// Fund well above the over-limit amount so the ONLY gate that can reject the
	// "100" transfer is the tracer reserve (not insufficient funds).
	fund(t, f, src, "1000")

	balanceBefore := availableBalance(t, f, src)

	// --- over-limit transfer: MUST be denied before any balance move ---
	over := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if over.status >= 200 && over.status < 300 {
		// MONEY/INTEGRITY: an over-limit transfer that commits is a failed enforce
		// gate — funds moved past an ACTIVE cap. Over gRPC this would also point at
		// a silently-dropped reserve (transport returned no decision).
		t.Fatalf("over-limit transfer over gRPC (cap %s, amount 100) returned 2xx %d — enforce denial gate did not reject\nbody: %s",
			tgrpcOverLimitMax, over.status, over.body)
	}

	// Cross-transport contract: the gRPC+mTLS reserve must yield the SAME
	// ErrTransactionReservationDenied (0177) -> UnprocessableOperationError -> 422
	// as the REST path. LIVE-VERIFY: confirm the 422 (not a 503/transport error)
	// against the live mTLS-wired stack.
	if over.status != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit denial over gRPC: want 422 (0177 ErrTransactionReservationDenied), got %d\nbody: %s",
			over.status, over.body)
	}

	// Balance MUST be unchanged: a denied reserve rejects BEFORE
	// ProcessBalanceOperations, so no debit occurred — transport irrelevant.
	if balanceAfter := availableBalance(t, f, src); balanceAfter != balanceBefore {
		t.Fatalf("source balance moved on a DENIED gRPC transfer: before=%s after=%s — funds escaped the enforce gate",
			balanceBefore, balanceAfter)
	}

	// --- positive control: in-limit transfer (40 <= 50) MUST succeed ---
	under := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "40", nil))
	if under.status != http.StatusCreated {
		t.Fatalf("in-limit transfer over gRPC (cap %s, amount 40): want 201 (reserve permits), got %d\nbody: %s",
			tgrpcOverLimitMax, under.status, under.body)
	}

	// The successful in-limit transfer was gated by an honored reserve carried
	// over gRPC, not a per-call skip, so tracerSkipped must be false.
	if v, ok := under.json["tracerSkipped"].(bool); !ok {
		t.Fatalf("in-limit transfer response missing boolean tracerSkipped field\nbody: %s", under.body)
	} else if v {
		t.Fatalf("tracerSkipped=true on an enforce transfer with no skip requested; reserve should have been honored over gRPC\nbody: %s", under.body)
	}

	// And the in-limit debit actually moved funds: before(1000) - 40 = 960.
	if got, want := atoiDecimal(t, availableBalance(t, f, src)), atoiDecimal(t, balanceBefore)-40; got != want {
		t.Fatalf("source balance after in-limit gRPC transfer = %d, want %d (before %s - 40)",
			got, want, balanceBefore)
	}
}

// TestTracerGRPCInLimitCommits is the gRPC-transport canary: on a gRPC+mTLS-wired
// enforce ledger, an IN-LIMIT transfer must COMMIT (201) and the resulting
// transaction's tracerSkipped audit field must be FALSE — the reserve crossed
// the mTLS hop, returned permit, and was honored (not skipped). It isolates the
// happy path from the denial mirror so a green-handshake / permit decision is
// asserted independently of the reject branch.
func TestTracerGRPCInLimitCommits(t *testing.T) {
	requireStack(t)
	tgrpcRequireGRPC(t)

	f := newEnforceFixture(t, "open")

	alias := "tgrpc-commit-" + uuid.NewString()[:8]
	src := createAccount(t, f, alias)
	dst := createAccount(t, f, "tgrpc-commit-dst-"+uuid.NewString()[:8])

	// Cap of 1000 on the source account; an in-limit transfer of 100 must pass
	// the gRPC reserve.
	seedLimitRule(t, f, "1000", map[string]any{"accountId": accountIDByAlias(t, f, src)})

	fund(t, f, src, "500")
	balanceBefore := availableBalance(t, f, src)

	r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if r.status != http.StatusCreated {
		t.Fatalf("in-limit transfer over gRPC: want 201 (reserve permits), got %d\nbody: %s", r.status, r.body)
	}

	// tracerSkipped=false proves the reserve was consulted over gRPC and honored,
	// not bypassed — the audit flag is the per-call-skip signal and no skip was
	// requested.
	if v, ok := r.json["tracerSkipped"].(bool); !ok {
		t.Fatalf("transaction response missing boolean tracerSkipped field\nbody: %s", r.body)
	} else if v {
		t.Fatalf("tracerSkipped=true on an enforce gRPC transfer with no skip requested; reserve should have been honored\nbody: %s", r.body)
	}

	// Funds moved: before(500) - 100 = 400.
	if got, want := atoiDecimal(t, availableBalance(t, f, src)), atoiDecimal(t, balanceBefore)-100; got != want {
		t.Fatalf("source balance after in-limit gRPC transfer = %d, want %d (before %s - 100)",
			got, want, balanceBefore)
	}
}
