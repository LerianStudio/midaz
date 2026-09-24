// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

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
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestAuthzRefusal_TracerEdge drives the real lib-auth client against an Access
// Manager in httptest through the tracer's production ErrorHandler
// (routes.go mounts CanonicalFiberErrorHandler on a bare fiber.App, so the tracer
// serves RFC 9457 everywhere). Since lib-auth v5 returns the refusal instead of
// writing it, that handler is what decides the wire, and every row asserts the
// protected handler never ran.
func TestAuthzRefusal_TracerEdge(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// withToken=false never reaches the Access Manager: lib-auth refuses
		// before the round trip.
		withToken  bool
		amStatus   int
		amBody     string
		wantStatus int
		wantCode   string
	}{
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

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			accessManager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.amStatus)

				if _, err := w.Write([]byte(testCase.amBody)); err != nil {
					t.Errorf("access manager: write response: %v", err)
				}
			}))
			t.Cleanup(accessManager.Close)

			guard := NewAuthGuard(
				AuthGuardConfig{PluginAuthEnabled: true, AppName: "tracer"},
				authMiddleware.NewAuthClient(accessManager.URL, true, libLog.NewNop()),
			)
			require.NotNil(t, guard)

			var handlerRan atomic.Bool

			app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
			app.Get("/validations", guard.Protect("validations", "get"), func(c fiber.Ctx) error {
				handlerRan.Store(true)

				return c.SendString("served")
			})

			req := httptest.NewRequest(fiber.MethodGet, "/validations", nil)
			if testCase.withToken {
				req.Header.Set(fiber.HeaderAuthorization, "Bearer "+makeJWT(t, jwt.MapClaims{
					"type":  "normal-user",
					"owner": "guard-org",
					"sub":   "guard-user",
				}))
			}

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, testCase.wantStatus, resp.StatusCode)
			assert.False(t, handlerRan.Load(), "the protected handler ran despite the refusal")
			assert.Contains(t, resp.Header.Get(fiber.HeaderContentType), "application/problem+json")

			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var body struct {
				Code   string `json:"code"`
				Status int    `json:"status"`
			}

			require.NoError(t, json.Unmarshal(raw, &body))
			assert.Equal(t, testCase.wantCode, body.Code)
			assert.Equal(t, testCase.wantStatus, body.Status)
		})
	}
}
