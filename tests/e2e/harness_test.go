// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

// Package e2e holds end-to-end tests that exercise the live midaz stack over
// HTTP: the unified ledger binary (onboarding + transaction + CRM + fees) on
// :3002 and the tracer service on :4020. The suite assumes the stack is already
// running (bring it up with `make up`); it does not spin up containers itself,
// mirroring the existing tracer BDD `make test-e2e` contract.
//
// Run: make test-ledger-e2e   (or: go test -tags e2e ./tests/e2e/...)
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Base URLs are overridable so the suite can target a remote stack in CI.
// Defaults match the local `make up` ports.
func ledgerURL() string { return envOr("LEDGER_URL", "http://localhost:3002") }
func tracerURL() string { return envOr("TRACER_URL", "http://localhost:4020") }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

// httpClient is shared; the per-request timeout is generous because a cold
// stack (first transaction after boot) can be slow.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// stackOnce gates the whole suite on the ledger being reachable. A down stack
// skips rather than fails: e2e is opt-in and needs `make up` first.
var (
	stackOnce sync.Once
	stackUp   bool
)

// requireStack skips the calling test when the ledger /readyz probe is not 200.
func requireStack(t *testing.T) {
	t.Helper()

	stackOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ledgerURL()+"/readyz", nil)
		if err != nil {
			return
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()

		stackUp = resp.StatusCode == http.StatusOK
	})

	if !stackUp {
		t.Skipf("ledger not reachable at %s/readyz — start the stack with `make up` (set LEDGER_URL to override)", ledgerURL())
	}
}

// response carries the decoded body and status of an HTTP call. body is the
// raw bytes (for error reporting); json is the decoded object when the body
// was a JSON object.
type response struct {
	status int
	body   []byte
	json   map[string]any
}

// call performs an HTTP request with an optional JSON body and decodes the
// response. It never fails the test itself — callers assert on status — so it
// can be used both for happy-path helpers and for negative assertions.
func call(t *testing.T, method, url string, body any) response {
	t.Helper()

	var reader io.Reader

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}

		reader = bytes.NewReader(raw)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		t.Fatalf("build request %s %s: %v", method, url, err)
	}

	req.Header.Set("Content-Type", "application/json")
	// Fresh request id keeps the idempotency cache from short-circuiting repeats.
	req.Header.Set("X-Request-Id", uuid.NewString())

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)

	out := response{status: resp.StatusCode, body: raw}
	// Best-effort decode; list/array bodies or empty bodies leave json nil.
	_ = json.Unmarshal(raw, &out.json)

	return out
}

// mustCreate posts body and requires HTTP 201, returning the decoded object.
// On any other status it fails with the full response body so contract drift
// is obvious from the test log.
func mustCreate(t *testing.T, url string, body any) map[string]any {
	t.Helper()

	r := call(t, http.MethodPost, url, body)
	if r.status != http.StatusCreated {
		t.Fatalf("POST %s: want 201, got %d\nbody: %s", url, r.status, r.body)
	}

	return r.json
}

// ---- flow helpers ---------------------------------------------------------

// fixture is the set of IDs created by a flow, threaded through assertions.
type fixture struct {
	orgID    string
	ledgerID string
}

func (f fixture) ledgers() string {
	return fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s", ledgerURL(), f.orgID, f.ledgerID)
}

func createOrg(t *testing.T) string {
	t.Helper()

	org := mustCreate(t, ledgerURL()+"/v1/organizations", map[string]any{
		"legalName":       "E2E Org " + uuid.NewString()[:8],
		"legalDocument":   "123456789012345",
		"doingBusinessAs": "E2E",
	})

	return str(t, org, "id")
}

// createLedger creates a ledger. When overrides is true the ledger opts into
// all per-call skips and keeps tracer off; settings must be complete because
// a partial settings object leaves tracer.mode="" which the API rejects (0176).
func createLedger(t *testing.T, orgID string, allowSkips bool) string {
	t.Helper()

	body := map[string]any{"name": "E2E Ledger " + uuid.NewString()[:8]}
	if allowSkips {
		body["settings"] = map[string]any{
			"accounting": map[string]any{"requireHolder": false},
			"tracer":     map[string]any{"mode": "off", "failPosture": "open", "timeoutMs": 250},
			"overrides":  map[string]any{"allowFeeSkip": true, "allowTracerSkip": true, "allowHolderSkip": true},
		}
	}

	led := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/ledgers", ledgerURL(), orgID), body)

	return str(t, led, "id")
}

func newFixture(t *testing.T, allowSkips bool) fixture {
	t.Helper()

	f := fixture{orgID: createOrg(t)}
	f.ledgerID = createLedger(t, f.orgID, allowSkips)
	createAsset(t, f, "USD")

	return f
}

func createAsset(t *testing.T, f fixture, code string) {
	t.Helper()

	mustCreate(t, f.ledgers()+"/assets", map[string]any{
		"name": code + " currency", "type": "currency", "code": code,
	})
}

// createAccount opens a plain (non-CRM) account and returns its alias.
func createAccount(t *testing.T, f fixture, alias string) string {
	t.Helper()

	acc := mustCreate(t, f.ledgers()+"/accounts", map[string]any{
		"name": "Acct " + alias, "assetCode": "USD", "type": "deposit", "alias": alias,
	})

	return str(t, acc, "alias")
}

func createHolder(t *testing.T, orgID string) string {
	t.Helper()

	h := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/holders", ledgerURL(), orgID), map[string]any{
		"type": "NATURAL_PERSON", "name": "Jane Doe", "document": "91315026015",
		"externalId": "E2E-" + uuid.NewString()[:8],
	})

	return str(t, h, "id")
}

// createHolderAccount opens a holder-owned account (the CRM-composed path) and
// returns the inner account object. The endpoint wraps its result in a
// composition envelope {account, instrument}; type is required despite the
// postman example showing only assetCode.
func createHolderAccount(t *testing.T, f fixture, holderID string) map[string]any {
	t.Helper()

	env := mustCreate(t, fmt.Sprintf("%s/holders/%s/accounts", f.ledgers(), holderID), map[string]any{
		"assetCode": "USD", "type": "deposit",
	})

	acc, ok := env["account"].(map[string]any)
	if !ok {
		t.Fatalf("holder-owned account response missing 'account' envelope: %v", env)
	}

	return acc
}

// createFeePackage registers an enabled fee package crediting creditAlias on
// every matching transaction. applicationRule/calcType select the model
// ("flatFee"/"flat" for a fixed fee, "percentual"/"percentage" for a percent).
func createFeePackage(t *testing.T, f fixture, creditAlias, applicationRule, calcType, value string) map[string]any {
	t.Helper()

	return mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/packages", ledgerURL(), f.orgID), map[string]any{
		"feeGroupLabel": "E2E Std", "ledgerId": f.ledgerID,
		"minimumAmount": "0", "maximumAmount": "100000000", "enable": true,
		"fees": map[string]any{
			"adminFee": map[string]any{
				"feeLabel": "Admin",
				"calculationModel": map[string]any{
					"applicationRule": applicationRule,
					"calculations":    []any{map[string]any{"type": calcType, "value": value}},
				},
				"referenceAmount":  "originalAmount",
				"priority":         1,
				"isDeductibleFrom": false,
				"creditAccount":    creditAlias,
			},
		},
	})
}

// createFlatFeePackage registers a fixed-amount fee package.
func createFlatFeePackage(t *testing.T, f fixture, creditAlias, flatValue string) map[string]any {
	t.Helper()
	return createFeePackage(t, f, creditAlias, "flatFee", "flat", flatValue)
}

// txnOp POSTs a no-body transaction lifecycle operation (commit/cancel/revert)
// and returns the response for the caller to assert on.
func txnOp(t *testing.T, f fixture, txnID, op string) response {
	t.Helper()
	return call(t, http.MethodPost, fmt.Sprintf("%s/transactions/%s/%s", f.ledgers(), txnID, op), nil)
}

// createInstrument links a holder to a ledger account (CRM instrument).
func createInstrument(t *testing.T, orgID, ledgerID, holderID, accountID string) map[string]any {
	t.Helper()
	return mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/holders/%s/instruments", ledgerURL(), orgID, holderID), map[string]any{
		"accountId": accountID, "ledgerId": ledgerID,
	})
}

// accountID opens a plain account and returns its id (alias variant is createAccount).
func accountID(t *testing.T, f fixture, alias string) string {
	t.Helper()
	acc := mustCreate(t, f.ledgers()+"/accounts", map[string]any{
		"name": "Acct " + alias, "assetCode": "USD", "type": "deposit", "alias": alias,
	})
	return str(t, acc, "id")
}

// requireTracer skips the calling test when the tracer /readyz is not 200.
var (
	tracerOnce sync.Once
	tracerUp   bool
)

func requireTracer(t *testing.T) {
	t.Helper()

	tracerOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, tracerURL()+"/readyz", nil)
		if err != nil {
			return
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()

		tracerUp = resp.StatusCode == http.StatusOK
	})

	if !tracerUp {
		t.Skipf("tracer not reachable at %s/readyz — start it with the tracer compose (set TRACER_URL to override)", tracerURL())
	}
}

// fund credits alias with value USD from the external account via an inflow.
func fund(t *testing.T, f fixture, alias, value string) {
	t.Helper()

	mustCreate(t, f.ledgers()+"/transactions/inflow", map[string]any{
		"description": "fund",
		"send": map[string]any{
			"asset": "USD", "value": value,
			"distribute": map[string]any{
				"to": []any{map[string]any{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": value}}},
			},
		},
	})
}

// transferBody builds a JSON-transaction body moving value USD from->to. skip,
// when non-nil, is attached verbatim (e.g. {"fees": false}).
func transferBody(from, to, value string, skip map[string]any) map[string]any {
	body := map[string]any{
		"description": "xfer " + uuid.NewString()[:8],
		"send": map[string]any{
			"asset": "USD", "value": value,
			"source":     map[string]any{"from": []any{map[string]any{"accountAlias": from, "amount": map[string]any{"asset": "USD", "value": value}}}},
			"distribute": map[string]any{"to": []any{map[string]any{"accountAlias": to, "amount": map[string]any{"asset": "USD", "value": value}}}},
		},
	}
	if skip != nil {
		body["skip"] = skip
	}

	return body
}

// availableBalance returns the available balance string for an account alias.
func availableBalance(t *testing.T, f fixture, alias string) string {
	t.Helper()

	r := call(t, http.MethodGet, f.ledgers()+"/accounts/alias/"+alias+"/balances", nil)
	if r.status != http.StatusOK {
		t.Fatalf("GET balances %s: want 200, got %d\nbody: %s", alias, r.status, r.body)
	}

	items, ok := r.json["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("GET balances %s: no items in %s", alias, r.body)
	}

	first, _ := items[0].(map[string]any)

	return str(t, first, "available")
}

// ---- tiny JSON navigation -------------------------------------------------

// str extracts a string field, failing the test if absent or not a string.
func str(t *testing.T, m map[string]any, key string) string {
	t.Helper()

	v, ok := m[key]
	if !ok {
		t.Fatalf("missing field %q in %v", key, m)
	}

	s, ok := v.(string)
	if !ok {
		t.Fatalf("field %q is %T, want string: %v", key, v, v)
	}

	return s
}

// atoiDecimal parses an integer-valued decimal string (balances/amounts are
// whole numbers in these tests) for arithmetic assertions.
func atoiDecimal(t *testing.T, s string) int64 {
	t.Helper()

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("parse decimal %q: %v", s, err)
	}

	return n
}
