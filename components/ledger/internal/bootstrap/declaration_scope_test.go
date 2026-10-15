// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/LerianStudio/lib-auth/v5/auth/obs"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authorizeRecorder is an Access Manager stub that records the scope attributes
// of every authorization question and allows them all.
type authorizeRecorder struct {
	*httptest.Server

	mu    sync.Mutex
	asked []map[string]string
}

func newAuthorizeRecorder(t *testing.T) *authorizeRecorder {
	t.Helper()

	rec := &authorizeRecorder{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		// Only authorization questions are recorded; anything else — the
		// publisher's token mint, with the flag on — is refused.
		if r.URL.Path != "/v1/authorize" {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authorize recorder: undecodable body: %v", err)
		}

		rec.mu.Lock()
		rec.asked = append(rec.asked, body.Attributes)
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": true})
	}))
	t.Cleanup(rec.Close)

	return rec
}

func (r *authorizeRecorder) questions() []map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]map[string]string(nil), r.asked...)
}

// partnerToken is an application credential bound to a partner — the caller
// whose authorization question must carry the route's scope attributes.
func partnerToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application", "sub": "acme/app", "partner": "acme/p1",
	}).SignedString([]byte("test-only"))
	require.NoError(t, err)

	return signed
}

// askAsPartner sends one request on the ledger route as an application
// credential bound to a partner, and returns what the Access Manager was asked.
func askAsPartner(t *testing.T, app *fiber.App, rec *authorizeRecorder) []map[string]string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-1/ledgers/led-1", nil)
	req.Header.Set("Authorization", "Bearer "+partnerToken(t))

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	return rec.questions()
}

// newScopedLedgerApp builds the ledger route over a fresh routes client, with
// nothing registered process-wide for "midaz" by an earlier test standing in
// for the client's own catalog.
func newScopedLedgerApp(t *testing.T, rec *authorizeRecorder) (*fiber.App, *middleware.AuthClient) {
	t.Helper()

	require.NoError(t, middleware.SetProductManifestScope("midaz"))

	auth := &middleware.AuthClient{Address: rec.URL, Enabled: true, Logger: obs.Nop(), M2MInversionEnabled: true}

	app := fiber.New()
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id",
		auth.Authorize("midaz", "ledgers", "get"),
		func(c fiber.Ctx) error { return c.SendString("ok") })

	return app, auth
}

// TestBuildDeclarationPublishers_FlagOffAndNoIdPStillWiresScope proves partner
// scope does not depend on RI declaration: with the flag off and every IdP
// setting unset — a multi-tenant deployment, where publication is the tenant
// manager's job — the boot succeeds and the routes' client still learns the
// manifest's scope, so a partner's question names the ledger it targets.
// Without it the Access Manager is asked with no attributes and a partner
// restricted to one ledger is refused on that very ledger.
func TestBuildDeclarationPublishers_FlagOffAndNoIdPStillWiresScope(t *testing.T) {
	for _, name := range []string{"IDP_DECLARATION_ENABLED", "IDP_HOST", "IDP_M2M_CLIENT_ID", "IDP_M2M_CLIENT_SECRET"} {
		t.Setenv(name, "")
	}

	rec := newAuthorizeRecorder(t)
	app, auth := newScopedLedgerApp(t, rec)

	stops, err := buildDeclarationPublishers(&Config{DeclarationEnabled: false}, auth, libLog.NewNop())
	require.NoError(t, err, "partner scope needs no IdP setting, so the boot must succeed")
	assert.Empty(t, stops, "with the declaration off nothing is published")

	assert.Equal(t, []map[string]string{{"organizationId": "org-1", "ledgerId": "led-1"}}, askAsPartner(t, app, rec),
		"the partner's question must carry the route's scope attributes")
}

// TestBuildDeclarationPublishers_FlagOnWiresScopeOnce proves the flag-on path
// keeps the routes' scope intact: wired at boot and again by the publisher from
// the same manifest, the client asks exactly one question with the same
// attributes.
func TestBuildDeclarationPublishers_FlagOnWiresScopeOnce(t *testing.T) {
	rec := newAuthorizeRecorder(t)
	app, auth := newScopedLedgerApp(t, rec)

	cfg := &Config{
		DeclarationEnabled: true,
		IDPHost:            "http://identity.invalid",
		IDPM2MClientID:     "dummy-client-id",
		IDPM2MClientSecret: "dummy-client-secret",
	}

	stops, err := buildDeclarationPublishers(cfg, auth, libLog.NewNop())
	require.NoError(t, err)
	require.Len(t, stops, 1)

	t.Cleanup(func() {
		for _, stop := range stops {
			stop()
		}
	})

	assert.Equal(t, []map[string]string{{"organizationId": "org-1", "ledgerId": "led-1"}}, askAsPartner(t, app, rec))
}
