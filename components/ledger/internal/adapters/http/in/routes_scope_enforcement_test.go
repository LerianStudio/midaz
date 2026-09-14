// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three instances the battery below is written against. Organization X holds two
// ledgers; the partner credential is scoped to B and has no grant on H.
const (
	scopeTestOrganizationX = "11111111-1111-1111-1111-111111111111"
	scopeTestLedgerB       = "22222222-2222-2222-2222-222222222222"
	scopeTestLedgerH       = "33333333-3333-3333-3333-333333333333"
	scopeTestPartner       = "acme/partner-b"
)

// scopeTestAuthorizeRequest is the subset of the authorization service's request body the
// battery decides on. Only the members the scope work introduced are named; the rest of
// the payload is deliberately not asserted here, since TestAuthorizePayload in lib-auth
// owns the byte-level contract.
type scopeTestAuthorizeRequest struct {
	Sub        string            `json:"sub"`
	Resource   string            `json:"resource"`
	Action     string            `json:"action"`
	Attributes map[string]string `json:"attributes"`
}

// fakeAccessManager stands in for the access manager. It grants the partner exactly one
// ledger — organization X, ledger B — and denies everything else with reason "scope",
// which is the answer a real partner-scoped grant produces.
//
// It records every question it was asked, so a test can assert not only the answer but
// whether the question was asked at all: a denial lib-auth reaches on its own, before the
// round trip, leaves no record here.
type fakeAccessManager struct {
	*httptest.Server

	mu    sync.Mutex
	asked []scopeTestAuthorizeRequest
}

func newFakeAccessManager(t *testing.T) *fakeAccessManager {
	t.Helper()

	fake := &fakeAccessManager{}

	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// NewAuthClient probes the address on construction. That probe is not an
		// authorization question and must not be recorded as one, or the counts the
		// battery asserts would include it.
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/authorize") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("healthy"))

			return
		}

		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req scopeTestAuthorizeRequest
		require.NoError(t, json.Unmarshal(raw, &req))

		fake.mu.Lock()
		fake.asked = append(fake.asked, req)
		fake.mu.Unlock()

		authorized := req.Attributes[scopeFieldOrganizationID] == scopeTestOrganizationX &&
			req.Attributes[scopeFieldLedgerID] == scopeTestLedgerB

		body := map[string]any{"authorized": authorized}
		if !authorized {
			body["reason"] = "scope"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(body))
	}))

	t.Cleanup(fake.Server.Close)

	return fake
}

// questions returns every authorization question asked so far, in order.
func (f *fakeAccessManager) questions() []scopeTestAuthorizeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]scopeTestAuthorizeRequest(nil), f.asked...)
}

// scopeTestPartnerToken mints the credential the access manager issues for a
// partner-bound application: an application token carrying the "partner" claim.
func scopeTestPartnerToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":    "application",
		"sub":     "acme/partner-app",
		"partner": scopeTestPartner,
	}).SignedString([]byte("scope-battery-secret"))
	require.NoError(t, err)

	return signed
}

// scopeTestPlainToken mints an application credential that is NOT bound to a partner.
// Such a credential is never refused locally, so every guarded route it touches produces
// an authorization question — which is what makes it the right credential for a sweep
// that measures whether the check ran at all.
func scopeTestPlainToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application",
		"sub":  "acme/plain-app",
	}).SignedString([]byte("scope-battery-secret"))
	require.NoError(t, err)

	return signed
}

// scopeTestApp mounts the given Fiber paths through the SAME funnel every real midaz
// route goes through — protectedMidaz + the route verb helpers — so what the battery
// exercises is the production guard chain, not a re-statement of it. Each route ends in a
// 200 terminal, so any non-200 answer came from the guard.
func scopeTestApp(t *testing.T, address string, paths map[string]string) *fiber.App {
	t.Helper()

	app := fiber.New()
	auth := middleware.NewAuthClient(address, true, libLog.NewNop())
	terminal := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

	for path, verb := range paths {
		switch verb {
		case fiber.MethodPost:
			routePost(app, path, protectedMidaz(auth, path, "transactions", "post", nil, terminal))
		default:
			routeGet(app, path, protectedMidaz(auth, path, "accounts", "get", nil, terminal))
		}
	}

	return app
}

// scopeTestRequest issues one request through the app carrying the partner credential.
func scopeTestRequest(t *testing.T, app *fiber.App, method, target, body string) int {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Authorization", "Bearer "+scopeTestPartnerToken(t))

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp.StatusCode
}

// The route shapes the battery mounts, spelled exactly as the production registrars spell
// them.
const (
	scopeTestAccountsPath      = "/organizations/:organization_id/ledgers/:ledger_id/accounts"
	scopeTestOrganizationsPath = "/organizations"
	scopeTestV2CreatePath      = "/transactions/direct"
)

// TestPartnerScope_AcceptanceBattery is the whole acceptance battery in ONE run, against
// ONE fake access manager and ONE credential, so no case can pass for a reason another
// case does not share.
//
// The positive control is the first assertion and is not optional. A battery that only
// produces denials proves nothing: every route could be denying for a reason that has
// nothing to do with the scope under test — a malformed token, a misdeclared route, an
// unreachable authorization service. The ALLOW is what shows the machinery works and that
// the denials are the scope talking.
func TestPartnerScope_AcceptanceBattery(t *testing.T) {
	t.Parallel()

	fake := newFakeAccessManager(t)
	app := scopeTestApp(t, fake.URL, map[string]string{
		scopeTestAccountsPath:      fiber.MethodGet,
		scopeTestOrganizationsPath: fiber.MethodGet,
		scopeTestV2CreatePath:      fiber.MethodPost,
	})

	// (1) POSITIVE CONTROL — the ledger the partner is scoped to answers 200.
	allowed := scopeTestRequest(t, app, fiber.MethodGet,
		"/organizations/"+scopeTestOrganizationX+"/ledgers/"+scopeTestLedgerB+"/accounts", "")
	require.Equal(t, fiber.StatusOK, allowed,
		"positive control: the partner's own ledger must pass, or every denial below is unattributable")

	// The ALLOW was decided on the identifiers the route declared, not on the tuple alone.
	asked := fake.questions()
	require.Len(t, asked, 1)
	assert.Equal(t, map[string]string{
		scopeFieldOrganizationID: scopeTestOrganizationX,
		scopeFieldLedgerID:       scopeTestLedgerB,
	}, asked[0].Attributes, "the route must send WHERE the request points, not only what it does")

	// (2) A sibling ledger the partner holds no grant on is refused.
	denied := scopeTestRequest(t, app, fiber.MethodGet,
		"/organizations/"+scopeTestOrganizationX+"/ledgers/"+scopeTestLedgerH+"/accounts", "")
	assert.Equal(t, fiber.StatusForbidden, denied, "a ledger outside the partner's scope must be refused")

	asked = fake.questions()
	require.Len(t, asked, 2)
	assert.Equal(t, scopeTestLedgerH, asked[1].Attributes[scopeFieldLedgerID],
		"the refused request must have NAMED the ledger it was refused for")

	// (3) A route that names no ledger is refused without a round trip. The credential is
	// only ever allowed to reach some instances; a route that cannot say which instance
	// the request points at has nothing to decide the "where" on, so the answer cannot be
	// delegated — it has to be a denial taken here.
	orgList := scopeTestRequest(t, app, fiber.MethodGet, "/organizations", "")
	assert.Equal(t, fiber.StatusForbidden, orgList,
		"a partner credential on a route that names no instance must be refused")
	assert.Len(t, fake.questions(), 2,
		"the refusal must be taken before the round trip: asking would risk a widening answer")

	// (4) The v2 create surface names no organization and no ledger in its path — its
	// scope lives in the request body — so a partner credential is refused there for the
	// same reason as (3), whatever the body says.
	v2 := scopeTestRequest(t, app, fiber.MethodPost, "/transactions/direct",
		`{"debits":[{"organizationId":"`+scopeTestOrganizationX+`","ledgerId":"`+scopeTestLedgerH+`"}]}`)
	assert.Equal(t, fiber.StatusForbidden, v2,
		"a transaction leg outside the partner's scope must not be created")
	assert.Len(t, fake.questions(), 2,
		"the v2 create refusal is taken before the round trip too")
}
