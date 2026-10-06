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

// askAsPartner sends one request on the rule route as an application credential
// bound to a partner, and returns what the Access Manager was asked.
func askAsPartner(t *testing.T, auth *middleware.AuthClient, rec *authorizeRecorder) []map[string]string {
	t.Helper()

	app := fiber.New()
	app.Get("/v1/rules/:rule_id", auth.Authorize("tracer", "rules", "get"),
		func(c fiber.Ctx) error { return c.SendString("ok") })

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application", "sub": "acme/app", "partner": "acme/p1",
	}).SignedString([]byte("test-only"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/rules/rule-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	return rec.questions()
}

func newRoutesAuthClient(t *testing.T, rec *authorizeRecorder) *middleware.AuthClient {
	t.Helper()

	// Nothing registered process-wide for "tracer" by an earlier test may stand
	// in for the client's own catalog.
	require.NoError(t, middleware.SetProductManifestScope("tracer"))

	return &middleware.AuthClient{Address: rec.URL, Enabled: true, Logger: obs.Nop(), M2MInversionEnabled: true}
}

// TestBuildDeclarationPublisher_DisabledStillWiresScope proves that a tracer
// whose RI declaration is off — a multi-tenant deployment, where publication is
// the tenant manager's job — still teaches its routes' authorization client the
// manifest's scope, so a partner's question names the rule it targets.
func TestBuildDeclarationPublisher_DisabledStillWiresScope(t *testing.T) {
	rec := newAuthorizeRecorder(t)
	auth := newRoutesAuthClient(t, rec)

	stops, err := buildDeclarationPublisher(&Config{DeclarationEnabled: false}, auth, libLog.NewNop())
	require.NoError(t, err)
	assert.Empty(t, stops, "with the declaration off nothing is published")

	assert.Equal(t, []map[string]string{{"ruleId": "rule-1"}}, askAsPartner(t, auth, rec),
		"the partner's question must carry the route's scope attributes")
}

// TestWireDeclarationPublisher_DisabledWiresTheRoutesClient proves the boot
// hands the routes' own client to the flag-off path: the publisher's client is
// built only with the flag on, so it cannot be the one that learns the scope.
func TestWireDeclarationPublisher_DisabledWiresTheRoutesClient(t *testing.T) {
	rec := newAuthorizeRecorder(t)
	auth := newRoutesAuthClient(t, rec)

	stops, err := wireDeclarationPublisher(&Config{DeclarationEnabled: false}, "", auth, libLog.NewNop())
	require.NoError(t, err)
	assert.Empty(t, stops)

	assert.Equal(t, []map[string]string{{"ruleId": "rule-1"}}, askAsPartner(t, auth, rec))
}
