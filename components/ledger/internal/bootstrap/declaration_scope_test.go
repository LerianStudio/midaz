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
		var body struct {
			Attributes map[string]string `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authorize recorder: undecodable body: %v", err)
		}

		rec.mu.Lock()
		rec.asked = append(rec.asked, body.Attributes)
		rec.mu.Unlock()

		w.Header().Set("Connection", "close")
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

// TestBuildDeclarationPublishers_DisabledStillWiresScope proves that a ledger
// whose RI declaration is off — a multi-tenant deployment, where publication is
// the tenant manager's job — still teaches its routes' authorization client the
// manifest's scope, so a partner's question names the ledger it targets.
// Without it the Access Manager is asked with no attributes and a partner
// restricted to one ledger is refused on that very ledger.
func TestBuildDeclarationPublishers_DisabledStillWiresScope(t *testing.T) {
	// Nothing registered process-wide for "midaz" by an earlier test may stand
	// in for the client's own catalog.
	require.NoError(t, middleware.SetProductManifestScope("midaz"))

	rec := newAuthorizeRecorder(t)
	auth := &middleware.AuthClient{Address: rec.URL, Enabled: true, Logger: obs.Nop(), M2MInversionEnabled: true}

	app := fiber.New()
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id",
		auth.Authorize("midaz", "ledgers", "get"),
		func(c fiber.Ctx) error { return c.SendString("ok") })

	stops, err := buildDeclarationPublishers(&Config{DeclarationEnabled: false}, auth, libLog.NewNop())
	require.NoError(t, err)
	assert.Empty(t, stops, "with the declaration off nothing is published")

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-1/ledgers/led-1", nil)
	req.Header.Set("Authorization", "Bearer "+partnerToken(t))

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []map[string]string{{"organizationId": "org-1", "ledgerId": "led-1"}}, rec.questions(),
		"the partner's question must carry the route's scope attributes")
}
