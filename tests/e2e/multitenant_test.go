// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

// Epic 3.2 — Multi-tenant scoping.
//
// LIVE-VERIFICATION IS DEFERRED. These tests pin the tenant-isolation contract
// but cannot be exercised on the current default `make up` stack: multi-tenancy
// requires MULTI_TENANT_ENABLED=true AND auth enabled AND a live tenant-manager
// (per-tenant Postgres/Mongo pools resolved by the tenant middleware). The
// supervisor confirms the exact cross-tenant status code (mt prefix LIVE-VERIFY
// notes) once that infra exists. Until then the suite SKIPS cleanly.
//
// Why MT genuinely needs the auth path: there is NO dev-bypass header on the
// user-facing ledger routes. The trusted x-tenant-id seam exists only on the
// tracer reservation adapter (components/tracer/internal/adapters/seamtenant/
// resolver.go:33) — never on /v1/organizations. The only way to set the tenant
// on a user-facing request is the JWT tenantId claim, which the auth middleware
// lifts via jwt.ParseUnverified (pkg/net/http/protected_routes.go:68-70) and,
// when valid, binds onto the request context.

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

// mtMultiTenantOnce gates the MT suite on the opt-in env flag. The default
// stack runs single-tenant, so without an explicit signal these tests skip
// rather than fail: there is no way to discover MT readiness over the wire
// (no public endpoint reports tenant-manager health), so it must be declared.
var mtMultiTenantOnce sync.Once

var mtMultiTenantEnabled bool

// mtRequireMultiTenant skips unless E2E_MULTI_TENANT=1. Pair it with
// requireStack(t): requireStack proves the ledger is up; this proves the
// caller has stood up the MT + auth + tenant-manager infra these tests need.
func mtRequireMultiTenant(t *testing.T) {
	t.Helper()

	mtMultiTenantOnce.Do(func() {
		mtMultiTenantEnabled = os.Getenv("E2E_MULTI_TENANT") == "1"
	})

	if !mtMultiTenantEnabled {
		t.Skip("multi-tenant e2e disabled — set E2E_MULTI_TENANT=1 with MULTI_TENANT_ENABLED + auth + a live tenant-manager")
	}
}

// mtTenantToken mints an UNSIGNED JWT (alg=none) carrying the tenantId claim.
// The ledger auth middleware parses bearer tokens with jwt.ParseUnverified
// (pkg/net/http/protected_routes.go:68-70) — it trusts the upstream auth layer
// for signature validation — so an unsigned token with a valid tenantId claim
// is honored and binds the tenant onto the request context. sub is set to a
// per-tenant synthetic principal so the two tenants are distinguishable in any
// audit trail. Pattern proven in tracer integration 14a (mintJWTWithTenantID).
func mtTenantToken(t *testing.T, tenantID string) string {
	t.Helper()

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":      "e2e-" + tenantID,
		"tenantId": tenantID,
		"iat":      now.Unix(),
		"exp":      now.Add(1 * time.Hour).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)

	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("mint MT token for %s: %v", tenantID, err)
	}

	return signed
}

// mtCall mirrors the harness call() helper but attaches an Authorization
// bearer header so the request carries a tenant identity. It returns the same
// response shape (status/body/json) and, like call(), never fails the test
// itself — callers assert on status, including for the negative cross-tenant
// path. Kept local (mt prefix) so it does not collide with the harness call().
func mtCall(t *testing.T, method, url, bearer string, body any) response {
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
	req.Header.Set("Authorization", "Bearer "+bearer)
	// Fresh request id keeps the idempotency cache from short-circuiting repeats.
	req.Header.Set("X-Request-Id", uuid.NewString())

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)

	out := response{status: resp.StatusCode, body: raw}
	// Best-effort decode; list/array or empty bodies leave json nil.
	_ = json.Unmarshal(raw, &out.json)

	return out
}

// mtNewTenantID returns a fresh, RFC4122 tenant id. tmcore.IsValidTenantID
// (the gate the auth middleware applies before binding the claim) accepts a
// UUID, so a random one keeps each test run on its own tenant pair.
func mtNewTenantID() string { return uuid.NewString() }

// mtOrgBody builds a minimal valid organization create payload (mirrors
// createOrg) with a unique legal name so retries never clash.
func mtOrgBody() map[string]any {
	return map[string]any{
		"legalName":       "MT Org " + uuid.NewString()[:8],
		"legalDocument":   "123456789012345",
		"doingBusinessAs": "MT",
	}
}

// TestMultiTenantIsolationCrossTenant proves a resource created under tenant A
// is not reachable under tenant B's identity. The tenant comes solely from the
// JWT tenantId claim, so two tokens with different tenant ids land on two
// distinct tenant-scoped datastores.
func TestMultiTenantIsolationCrossTenant(t *testing.T) {
	requireStack(t)
	mtRequireMultiTenant(t)

	tenantA := mtNewTenantID()
	tenantB := mtNewTenantID()

	tokenA := mtTenantToken(t, tenantA)
	tokenB := mtTenantToken(t, tenantB)

	// Create an org under tenant A.
	created := mtCall(t, http.MethodPost, ledgerURL()+"/v1/organizations", tokenA, mtOrgBody())
	if created.status != http.StatusCreated {
		t.Fatalf("create org under tenant A: want 201, got %d\nbody: %s", created.status, created.body)
	}

	orgID := str(t, created.json, "id")

	// Sanity: tenant A can read its own org back. This separates a genuine
	// isolation failure from a resource that never persisted.
	ownerRead := mtCall(t, http.MethodGet, ledgerURL()+"/v1/organizations/"+orgID, tokenA, nil)
	if ownerRead.status != http.StatusOK {
		t.Fatalf("tenant A read own org %s: want 200, got %d\nbody: %s", orgID, ownerRead.status, ownerRead.body)
	}

	// Cross-tenant read: tenant B must NOT see tenant A's org. The org id does
	// not exist in tenant B's scope, so the resource resolves to not-found.
	// LIVE-VERIFY: pin 404 (per-tenant datastore -> the row is simply absent,
	// so the not-found path, not an authz 403, is the expected contract). The
	// supervisor confirms the exact status (404 vs 403) once MT infra is live.
	crossRead := mtCall(t, http.MethodGet, ledgerURL()+"/v1/organizations/"+orgID, tokenB, nil)
	if crossRead.status != http.StatusNotFound {
		t.Fatalf("cross-tenant read of org %s under tenant B: want 404 (isolation), got %d\nbody: %s",
			orgID, crossRead.status, crossRead.body)
	}

	// The load-bearing invariant regardless of the exact code: tenant B must
	// never receive the resource. A 200 here is a hard isolation breach.
	if crossRead.status == http.StatusOK {
		t.Fatalf("ISOLATION BREACH: tenant B read tenant A's org %s (status 200)\nbody: %s", orgID, crossRead.body)
	}
}

// TestMultiTenantSameAliasCoexists proves the same logical resource identity
// (an organization with the same legalDocument) exists independently under two
// tenants. Per-tenant datastores mean A's row and B's row do not collide: both
// creates succeed and yield distinct ids.
func TestMultiTenantSameAliasCoexists(t *testing.T) {
	requireStack(t)
	mtRequireMultiTenant(t)

	tokenA := mtTenantToken(t, mtNewTenantID())
	tokenB := mtTenantToken(t, mtNewTenantID())

	// Identical payload (same legalDocument) submitted under each tenant. The
	// shared body makes the coexistence claim sharp: it is the SAME logical org.
	shared := map[string]any{
		"legalName":       "MT Shared " + uuid.NewString()[:8],
		"legalDocument":   "123456789012345",
		"doingBusinessAs": "MT",
	}

	createdA := mtCall(t, http.MethodPost, ledgerURL()+"/v1/organizations", tokenA, shared)
	if createdA.status != http.StatusCreated {
		t.Fatalf("create shared org under tenant A: want 201, got %d\nbody: %s", createdA.status, createdA.body)
	}

	createdB := mtCall(t, http.MethodPost, ledgerURL()+"/v1/organizations", tokenB, shared)
	if createdB.status != http.StatusCreated {
		t.Fatalf("create shared org under tenant B: want 201, got %d\nbody: %s", createdB.status, createdB.body)
	}

	idA := str(t, createdA.json, "id")
	idB := str(t, createdB.json, "id")

	// Distinct ids confirm two independent rows in two tenant scopes — not one
	// shared row returned twice.
	if idA == idB {
		t.Fatalf("expected distinct org ids across tenants, got same id %s for both", idA)
	}
}
