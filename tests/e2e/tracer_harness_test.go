// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// This file (Epic 2.1) adds the tracer-WIRED harness capability that the
// enforce/advisory reservation tests depend on. It seeds tracer limits, builds
// enforce-mode ledgers, and behaviorally detects whether THIS ledger forwards
// reserves to the tracer (TRACER_BASE_URL set globally on the ledger).
//
// All package-private names here carry the "trx" prefix so this file does not
// collide with sibling Phase-2 test files in the same `e2e` package.

// ---- tracer limit seeding -------------------------------------------------

// trxLimitID is the wire field name the tracer returns for a created limit. The
// limit model serializes its identifier as "limitId" (NOT "id"); the activate
// route still takes the bare id value as its :id path segment.
const trxLimitID = "limitId"

// seedLimitRule creates an ACTIVE PER_TRANSACTION limit on the tracer and
// returns its id. PER_TRANSACTION is the simplest cap to reason about for
// denial tests: the limit is breached whenever a single transaction's amount
// exceeds maxAmount, with no period accumulation.
//
// scope selects which transactions the limit matches. The tracer rejects an
// empty scopes array (validator: scopes required,min=1 + scopenotempty per
// element), so an account-AGNOSTIC limit is not expressible. When scope is nil
// this helper falls back to a transactionType:"CARD" scope — valid, but it only
// matches reserves that carry that transaction type, NOT a plain JSON transfer.
// Callers that need a deterministic denial against a plain transfer MUST pass an
// account-scoped scope, e.g. map[string]any{"accountId": "<source-account-uuid>"}.
func seedLimitRule(t *testing.T, f fixture, maxAmount string, scope map[string]any) string {
	t.Helper()

	if scope == nil {
		scope = map[string]any{"transactionType": "CARD"}
	}

	created := mustCreate(t, tracerURL()+"/v1/limits", map[string]any{
		"name":      "E2E Limit " + uuid.NewString()[:8],
		"limitType": "PER_TRANSACTION",
		"maxAmount": maxAmount,
		"currency":  "USD",
		"scopes":    []any{scope},
	})

	id := str(t, created, trxLimitID)

	act := call(t, http.MethodPost, tracerURL()+"/v1/limits/"+id+"/activate", nil)
	if act.status != http.StatusOK {
		t.Fatalf("POST /v1/limits/%s/activate: want 200, got %d\nbody: %s", id, act.status, act.body)
	}

	got := call(t, http.MethodGet, tracerURL()+"/v1/limits/"+id, nil)
	if got.status != http.StatusOK {
		t.Fatalf("GET /v1/limits/%s: want 200, got %d\nbody: %s", id, got.status, got.body)
	}

	if s := str(t, got.json, "status"); s != "ACTIVE" {
		t.Fatalf("seeded limit %s status=%q, want ACTIVE\nbody: %s", id, s, got.body)
	}

	return id
}

// ---- enforce-mode ledger fixture ------------------------------------------

// trxCreateEnforceLedger mirrors createLedger's create shape but sets the
// per-ledger tracer settings to enforce mode with the given failPosture, and
// opts into allowTracerSkip so per-call skip tests can exercise the override.
// A complete settings object is mandatory: a partial one leaves tracer.mode=""
// which the API rejects (0176).
func trxCreateEnforceLedger(t *testing.T, orgID, failPosture string) string {
	t.Helper()

	body := map[string]any{
		"name": "E2E Enforce Ledger " + uuid.NewString()[:8],
		"settings": map[string]any{
			"accounting": map[string]any{"requireHolder": false},
			"tracer":     map[string]any{"mode": "enforce", "failPosture": failPosture, "timeoutMs": 250},
			"overrides":  map[string]any{"allowFeeSkip": true, "allowTracerSkip": true, "allowHolderSkip": true},
		},
	}

	led := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/ledgers", ledgerURL(), orgID), body)

	return str(t, led, "id")
}

// newEnforceFixture builds a fixture whose ledger runs the tracer in enforce
// mode with the given failPosture ("open" or "closed") and a USD asset, ready
// for reservation tests. It mirrors newFixture's org+ledger+asset sequence.
func newEnforceFixture(t *testing.T, failPosture string) fixture {
	t.Helper()

	f := fixture{orgID: createOrg(t)}
	f.ledgerID = trxCreateEnforceLedger(t, f.orgID, failPosture)
	createAsset(t, f, "USD")

	return f
}

// ---- wired detection ------------------------------------------------------

// trxWiredOnce caches the one-time behavioral probe that decides whether the
// ledger forwards reserves to the tracer.
var (
	trxWiredOnce   sync.Once
	trxWired       bool
	trxWiredReason string
)

// requireTracerWired skips the calling test unless the ledger is BOTH reachable
// AND actually forwarding reserves to the tracer (global TRACER_BASE_URL set).
//
// The dev stack ships with TRACER_BASE_URL empty, so the reserve anchor injects
// no reserver and never calls the tracer (transaction_reservation_anchor.go:103
// short-circuits to proceed when handler.TracerReserver == nil). In that state a
// transaction that should be DENIED commits anyway, so an enforce ledger alone
// cannot prove wiring — only behavior can.
//
// Probe: on a throwaway enforce(fail-open) ledger, seed a PER_TRANSACTION limit
// of maxAmount "1" scoped to the funded source account, then attempt a transfer
// of "100" (100x over the cap). Wired+enforcing => the reserve is consulted and
// the over-limit transfer is DENIED (422, non-2xx). Not wired => no reserve
// happens and the transfer SUCCEEDS (201). We cache wired := (transfer denied).
//
// fail-open is chosen for the probe so that if the tracer call itself errored
// (vs. cleanly denying), the ledger would PROCEED — i.e. a transport flake
// reads as not-wired (skip), never as a false positive of wiring.
func requireTracerWired(t *testing.T) {
	t.Helper()

	requireStack(t)
	requireTracer(t)

	trxWiredOnce.Do(func() {
		f := newEnforceFixture(t, "open")

		src := createAccount(t, f, "trx-probe-src-"+uuid.NewString()[:8])
		dst := createAccount(t, f, "trx-probe-dst-"+uuid.NewString()[:8])

		// Cap the source account at 1; fund it well above the over-limit amount so
		// the ONLY gate that can reject the "100" transfer is the tracer reserve.
		seedLimitRule(t, f, "1", map[string]any{"accountId": accountIDByAlias(t, f, src)})
		fund(t, f, src, "1000")

		r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
		switch {
		case r.status == http.StatusCreated:
			trxWired = false
			trxWiredReason = fmt.Sprintf("over-limit transfer (cap 1, amount 100) returned 201 — no reserve happened (got body: %s)", r.body)
		case r.status >= 200 && r.status < 300:
			trxWired = false
			trxWiredReason = fmt.Sprintf("over-limit transfer returned 2xx %d — reserve did not gate", r.status)
		default:
			// Denied (expected 422) — the reserve was consulted and blocked the
			// over-limit transfer: the ledger is wired and enforcing.
			trxWired = true
			trxWiredReason = fmt.Sprintf("over-limit transfer denied with %d (wired)", r.status)
		}
	})

	if !trxWired {
		t.Skipf("ledger is reachable but NOT forwarding reserves to the tracer: %s — wire it by appending TRACER_BASE_URL=http://midaz-tracer:4020 and TRACER_TRANSPORT=rest to components/ledger/.env, then force-recreate the ledger container", trxWiredReason)
	}
}

// TestTracerWiredSmoke is the canary: on a wired enforce ledger, an IN-LIMIT
// transfer must SUCCEED (the reserve participated and did not deny) and the
// resulting transaction's tracerSkipped audit field must be false (the reserve
// was honored, not skipped). The supervisor runs this live against a wired
// ledger; in the unwired dev stack it SKIPS via requireTracerWired.
func TestTracerWiredSmoke(t *testing.T) {
	requireTracerWired(t)

	f := newEnforceFixture(t, "open")

	alias := "trx-smoke-" + uuid.NewString()[:8]
	src := createAccount(t, f, alias)
	dst := createAccount(t, f, "trx-smoke-dst-"+uuid.NewString()[:8])

	// Cap of 1000 on the source account; an in-limit transfer of 100 must pass.
	seedLimitRule(t, f, "1000", map[string]any{"accountId": accountIDByAlias(t, f, src)})

	fund(t, f, src, "500")

	r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if r.status != http.StatusCreated {
		t.Fatalf("in-limit transfer: want 201 (reserve permits), got %d\nbody: %s", r.status, r.body)
	}

	if v, ok := r.json["tracerSkipped"].(bool); !ok {
		t.Fatalf("transaction response missing boolean tracerSkipped field\nbody: %s", r.body)
	} else if v {
		t.Fatalf("tracerSkipped=true on an enforce transfer with no skip requested; reserve should have been honored\nbody: %s", r.body)
	}
}

// accountIDByAlias fetches an account's id given its alias (the tracer scopes
// limits by account UUID, but transfers address accounts by alias).
func accountIDByAlias(t *testing.T, f fixture, alias string) string {
	t.Helper()

	r := call(t, http.MethodGet, f.ledgers()+"/accounts/alias/"+alias, nil)
	if r.status != http.StatusOK {
		t.Fatalf("GET account by alias %s: want 200, got %d\nbody: %s", alias, r.status, r.body)
	}

	return str(t, r.json, "id")
}
