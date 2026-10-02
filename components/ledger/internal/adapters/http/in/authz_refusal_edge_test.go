// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// refusalRow is one way the Access Manager can fail to grant, and the envelope
// the ledger must answer with. It is shared with the tracer's twin test; both
// exist because a refusal now travels back as an error instead of a written
// response, so the service's ErrorHandler — not lib-auth — decides the wire.
type refusalRow struct {
	name string
	// amStatus/amBody are what the Access Manager answers on /v1/authorize.
	// withToken=false never reaches it: lib-auth refuses before the round trip.
	withToken  bool
	amStatus   int
	amBody     string
	wantStatus int
	wantCode   string
}

func refusalRows() []refusalRow {
	return []refusalRow{
		{
			name:       "no token",
			wantStatus: fiber.StatusUnauthorized,
			wantCode:   "0042",
		},
		{
			name:       "access manager denies",
			withToken:  true,
			amStatus:   http.StatusOK,
			amBody:     `{"authorized":false}`,
			wantStatus: fiber.StatusForbidden,
			wantCode:   "0043",
		},
		{
			name:       "access manager never decided",
			withToken:  true,
			amStatus:   http.StatusInternalServerError,
			amBody:     `{"error":"boom"}`,
			wantStatus: fiber.StatusServiceUnavailable,
			wantCode:   "0525",
		},
		{
			name:       "access manager refused the caller",
			withToken:  true,
			amStatus:   http.StatusForbidden,
			amBody:     `{"code":"AUT-0021","title":"Forbidden","detail":"IP not allowed"}`,
			wantStatus: fiber.StatusForbidden,
			wantCode:   "AUT-0021",
		},
		{
			// An ingress or proxy in front of the Access Manager answers the
			// refusal itself, with no domain code to carry: the status is still
			// the caller's, never a Midaz 500.
			name:       "access manager refused with no decodable body",
			withToken:  true,
			amStatus:   http.StatusConflict,
			amBody:     "<html>no</html>",
			wantStatus: fiber.StatusBadRequest,
			wantCode:   "0047",
		},
		{
			// A 405 from the Access Manager is a refusal, not the caller's wrong
			// method: only Fiber's router singleton means that.
			name:       "access manager answered 405 with no decodable body",
			withToken:  true,
			amStatus:   http.StatusMethodNotAllowed,
			amBody:     "<html>no</html>",
			wantStatus: fiber.StatusBadRequest,
			wantCode:   "0047",
		},
		{
			name:       "access manager has no subject for the token",
			withToken:  true,
			amStatus:   http.StatusNotFound,
			amBody:     `{"code":"AUT-1015"}`,
			wantStatus: fiber.StatusNotFound,
			wantCode:   "AUT-1015",
		},
		{
			name:       "access manager refused with a numeric code",
			withToken:  true,
			amStatus:   http.StatusUnauthorized,
			amBody:     `{"code":401}`,
			wantStatus: fiber.StatusUnauthorized,
			wantCode:   "0042",
		},
		{
			name:       "access manager refused without a code",
			withToken:  true,
			amStatus:   http.StatusForbidden,
			amBody:     `{"message":"x"}`,
			wantStatus: fiber.StatusForbidden,
			wantCode:   "0043",
		},
	}
}

// newAccessManager stands in for the Access Manager at the address the real
// lib-auth client dials, answering every path with the row's fixed answer.
func newAccessManager(t *testing.T, row refusalRow) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(row.amStatus)

		if _, err := w.Write([]byte(row.amBody)); err != nil {
			t.Errorf("access manager: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return server
}

// refusalBody covers both envelopes the ledger serves. status is the RFC 9457
// member, which the legacy /v1 rewrite drops — its absence is how a /v1 response
// proves it was rewritten.
type refusalBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// TestAuthzRefusal_LedgerEdge drives the real lib-auth client against an Access
// Manager in httptest through the ledger's production ErrorHandler, and pins
// both envelope families: /v2 serves RFC 9457, /v1 the legacy {code,title,message}.
// Every row asserts the protected handler never ran — a refusal that renders
// correctly but serves the request is the failure this guards against.
//
// NOT parallel: Huma validation and the problem hook are process-global.
func TestAuthzRefusal_LedgerEdge(t *testing.T) {
	families := []struct {
		name        string
		prefix      string
		contentType string
		// legacy reports whether this family is rewritten out of problem+json.
		legacy bool
	}{
		{name: "v1", prefix: "/v1", contentType: fiber.MIMEApplicationJSON, legacy: true},
		{name: "v2", prefix: "/v2", contentType: "application/problem+json"},
	}

	for _, row := range refusalRows() {
		for _, family := range families {
			t.Run(row.name+" on "+family.name, func(t *testing.T) {
				accessManager := newAccessManager(t, row)

				var handlerRan atomic.Bool

				auth := authMiddleware.NewAuthClient(accessManager.URL, true, libLog.NewNop())

				app := fiber.New(fiber.Config{
					ErrorHandler: ledgerMiddleware.WrapErrorHandler(pkgHTTP.CanonicalFiberErrorHandler),
				})
				app.Use(ledgerMiddleware.ErrorEnvelope())

				group := app.Group(family.prefix)
				registerRoute(group, fiber.MethodGet, "/organizations",
					protectedMidaz(auth, "organization", "get", nil, func(c fiber.Ctx) error {
						handlerRan.Store(true)

						return c.SendString("served")
					}))

				req := httptest.NewRequest(fiber.MethodGet, family.prefix+"/organizations", nil)
				if row.withToken {
					req.Header.Set(fiber.HeaderAuthorization, "Bearer "+guardBearerToken(t))
				}

				resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)

				defer func() { _ = resp.Body.Close() }()

				require.Equal(t, row.wantStatus, resp.StatusCode)
				assert.False(t, handlerRan.Load(), "the protected handler ran despite the refusal")
				assert.Contains(t, resp.Header.Get(fiber.HeaderContentType), family.contentType)

				raw, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var body refusalBody
				require.NoError(t, json.Unmarshal(raw, &body))
				assert.Equal(t, row.wantCode, body.Code)

				if family.legacy {
					assert.Zero(t, body.Status, "the legacy envelope carries no status member")
					assert.NotEmpty(t, body.Message)
				} else {
					assert.Equal(t, row.wantStatus, body.Status)
				}
			})
		}
	}
}
