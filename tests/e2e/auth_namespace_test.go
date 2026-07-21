// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

// EPIC 3.4 — AUTH-ON + CRM NAMESPACE FLIP.
//
// LIVE-VERIFICATION IS DEFERRED: no plugin-auth container ships in any compose
// today, so these tests COMPILE and SKIP cleanly on the default stack. To run
// them for real the stack needs auth wired on:
//   - PLUGIN_AUTH_ENABLED=true
//   - PLUGIN_AUTH_HOST pointed at a real plugin-auth (or an httptest mock that
//     mirrors its Authorize contract)
// and the suite gated in with E2E_AUTH=1. Until then the gate below skips.
//
// What these tests pin behaviorally:
//   - With auth enabled, an unauthenticated CRM call is rejected at the chain's
//     first link. The 401 source is MarkTrustedAuthAssertion, which returns
//     fiber.StatusUnauthorized when no bearer token is present
//     (pkg/net/http/protected_routes.go:48-49). LIVE-VERIFY whether plugin-auth's
//     own Authorize middleware short-circuits first with its own 401 — either
//     way the contract is "no token => 401", which is the hard assertion here.
//   - The CRM holders/instruments routes register under the "midaz" authz
//     namespace (the X1 flip from plugin-crm), via
//     auth.Authorize(ApplicationName, "holders"|"instruments", verb) where
//     ApplicationName="midaz" (crm_routes.go:15-20,36-53).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// authRequireAuth gates the auth suite. It is OFF by default because the default
// stack runs with auth disabled (and no plugin-auth container exists), so every
// CRM route would answer 2xx/4xx-business rather than 401, inverting these
// assertions. Opt in with E2E_AUTH=1 once plugin-auth is wired (see file header).
var (
	authOnce    sync.Once
	authEnabled bool
)

func authRequireAuth(t *testing.T) {
	t.Helper()

	authOnce.Do(func() {
		authEnabled = os.Getenv("E2E_AUTH") == "1"
	})

	if !authEnabled {
		t.Skipf("auth suite disabled — set E2E_AUTH=1 against a stack with PLUGIN_AUTH_ENABLED=true and PLUGIN_AUTH_HOST pointed at plugin-auth")
	}
}

// authToken mints an UNSIGNED JWT (alg=none) carrying claims. This is self-
// contained on purpose: it does NOT reuse any mt-prefixed helper even though the
// shape overlaps. The unsigned token is enough to exercise both the trusted-
// assertion ParseUnverified path (which never checks the signature) and a mock
// plugin-auth that authorizes on claims alone. jwt v5 requires the sentinel key
// jwt.UnsafeAllowNoneSignatureType to sign with SigningMethodNone.
func authToken(t *testing.T, claims map[string]any) string {
	t.Helper()

	mc := jwt.MapClaims{
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}
	for k, v := range claims {
		mc[k] = v
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, mc)

	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("mint unsigned jwt: %v", err)
	}

	return signed
}

// authCall mirrors harness call() (harness_test.go:95) but threads an optional
// bearer token, since call() exposes no header hook. Empty bearer means no
// Authorization header (the unauthenticated case). It returns the same response
// struct shape and likewise never fails the test itself — callers assert on
// status. The request path is re-implemented here (rather than mutating call())
// so this file stays additive and collides with no sibling helper.
func authCall(t *testing.T, method, url, bearer string, body any) response {
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
	req.Header.Set("X-Request-Id", uuid.NewString())

	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)

	out := response{status: resp.StatusCode, body: raw}
	_ = json.Unmarshal(raw, &out.json)

	return out
}

// TestAuthUnauthenticatedCRMRejected pins: with auth enabled, a CRM call carrying
// NO Authorization header is rejected with 401 before reaching the handler. This
// holds for both a holders route and an instruments route — the two namespaces
// the X1 flip moved under "midaz". The 401 originates in MarkTrustedAuthAssertion
// (empty token => fiber.StatusUnauthorized, protected_routes.go:48-49), unless
// plugin-auth's Authorize rejects first; either way the status is 401.
func TestAuthUnauthenticatedCRMRejected(t *testing.T) {
	requireStack(t)
	authRequireAuth(t)

	// Static org id is fine: auth must reject before any org lookup. A nil/dummy
	// UUID keeps the path well-formed for ParseUUIDPathParameters while the
	// request never gets that far.
	const org = "00000000-0000-0000-0000-000000000000"
	const holder = "00000000-0000-0000-0000-000000000001"

	cases := []struct {
		name string
		url  string
	}{
		{
			name: "holders",
			// crm_routes.go:36 — POST /v1/organizations/{org}/holders
			url: ledgerURL() + "/v1/organizations/" + org + "/holders",
		},
		{
			name: "instruments",
			// crm_routes.go:49 — POST .../holders/{holderId}/instruments
			url: ledgerURL() + "/v1/organizations/" + org + "/holders/" + holder + "/instruments",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Empty bearer => no Authorization header at all.
			r := authCall(t, http.MethodPost, tc.url, "", map[string]any{})
			if r.status != http.StatusUnauthorized {
				t.Fatalf("POST %s with no auth: want 401, got %d\nbody: %s", tc.url, r.status, r.body)
			}
		})
	}
}

// TestAuthAuthorizedUnderMidazNamespace pins the X1 namespace contract and the
// authorized happy path. The holders/instruments routes register
// auth.Authorize("midaz", "holders"|"instruments", verb) — ApplicationName is
// the literal "midaz" (crm_routes.go:15-20), the flip away from "plugin-crm".
// Tenant-manager RBAC grants migrate plugin-crm:* -> midaz:{holders,instruments}:*
// at the X1 release gate.
//
// With a mock-approved bearer (a tenant + subject the mock plugin-auth grants on
// the "midaz" namespace), the holders create call must NOT be rejected at the
// auth layer: it should pass auth and reach the business handler, yielding 201
// (or a business 4xx like 422 on payload, never 401/403).
func TestAuthAuthorizedUnderMidazNamespace(t *testing.T) {
	requireStack(t)
	authRequireAuth(t)

	// The mock plugin-auth must grant midaz:holders:post for this subject/tenant.
	// tenantId is read by MarkTrustedAuthAssertion (protected_routes.go:68) so the
	// downstream tenant middleware resolves a DB; sub becomes the user_id.
	token := authToken(t, map[string]any{
		"sub":      "e2e-auth-subject",
		"tenantId": "00000000-0000-0000-0000-0000000000aa",
	})

	org := createOrg(t) // requires the same auth to succeed; if it can't, the env is misconfigured.

	url := ledgerURL() + "/v1/organizations/" + org + "/holders"
	r := authCall(t, http.MethodPost, url, token, map[string]any{
		"type": "NATURAL_PERSON", "name": "Auth Probe", "document": "91315026015",
		"externalId": "E2E-AUTH-PROBE",
	})

	// Hard assertion: auth must not reject an approved token. The exact 2xx body
	// and tenant wiring is LIVE-VERIFY (deferred — needs a real/mock plugin-auth
	// granting the "midaz" namespace), but "authorized token is not 401/403" is
	// the load-bearing contract of the namespace flip.
	if r.status == http.StatusUnauthorized || r.status == http.StatusForbidden {
		t.Fatalf("POST %s with approved midaz-namespace token: rejected by auth (got %d)\nbody: %s", url, r.status, r.body)
	}
}
