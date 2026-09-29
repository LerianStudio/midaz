// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestAuthGuard_AuthorizesCallers(t *testing.T) {
	t.Parallel()

	var nilGuard *AuthGuard

	for name, tc := range map[string]struct {
		guard *AuthGuard
		want  bool
	}{
		"nil guard":                    {guard: nilGuard},
		"plugin auth disabled":         {guard: NewAuthGuard(AuthGuardConfig{APIKeyEnabled: true, APIKey: "k"}, authMiddleware.NewAuthClient("", false, libLog.NewNop()))},
		"plugin auth without a client": {guard: &AuthGuard{cfg: AuthGuardConfig{PluginAuthEnabled: true}}},
		"addressless client":           {guard: &AuthGuard{cfg: AuthGuardConfig{PluginAuthEnabled: true}, authClient: &authMiddleware.AuthClient{Enabled: true}}},
		"disabled client":              {guard: &AuthGuard{cfg: AuthGuardConfig{PluginAuthEnabled: true}, authClient: &authMiddleware.AuthClient{Address: "http://access-manager.test"}}},
		"authorizing client":           {guard: &AuthGuard{cfg: AuthGuardConfig{PluginAuthEnabled: true}, authClient: &authMiddleware.AuthClient{Enabled: true, Address: "http://access-manager.test"}}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.guard.AuthorizesCallers())
		})
	}
}

func applicationTestToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"type": "application", "sub": "ledger-app", "azp": "ledger-client"}).SignedString([]byte("unverified"))
	require.NoError(t, err)

	return signed
}

// callerProbe records what a CallerResolver was handed.
type callerProbe struct {
	caller TokenCaller
	ok     bool
	called bool
}

func (p *callerProbe) resolve(c fiber.Ctx, caller TokenCaller, ok bool) error {
	p.caller, p.ok, p.called = caller, ok, true

	return c.SendStatus(http.StatusNoContent)
}

func callProbe(t *testing.T, app *fiber.App, header string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp.StatusCode
}

func TestAuthorizedCaller_ReadsTokenClaims(t *testing.T) {
	t.Parallel()

	signed := applicationTestToken(t)
	caller := TokenCaller{Type: TokenTypeApplication, ClientID: "ledger-client"}

	for name, tc := range map[string]struct {
		header string
		want   TokenCaller
		ok     bool
	}{
		"bearer token":           {header: "Bearer " + signed, want: caller, ok: true},
		"lowercase bearer":       {header: "bearer " + signed, want: caller, ok: true},
		"raw token (no scheme)":  {header: signed, want: caller, ok: true},
		"no header":              {},
		"not a bearer":           {header: "Basic " + signed},
		"bearer without a token": {header: "Bearer"},
		"malformed token":        {header: "Bearer not-a-jwt"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				got TokenCaller
				ok  bool
			)

			app := fiber.New()
			app.Post("/", func(c fiber.Ctx) error {
				got, ok = authorizedCaller(c)

				return c.SendStatus(http.StatusNoContent)
			})

			require.Equal(t, http.StatusNoContent, callProbe(t, app, tc.header))
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestAuthGuard_WithAuthorizedCaller_HandsNoCallerWithoutAuthorization(t *testing.T) {
	t.Parallel()

	var nilGuard *AuthGuard

	for name, guard := range map[string]*AuthGuard{
		"nil guard":            nilGuard,
		"plugin auth disabled": NewAuthGuard(AuthGuardConfig{}, authMiddleware.NewAuthClient("", false, libLog.NewNop())),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			probe := &callerProbe{}
			handlers := guard.WithAuthorizedCaller("reservations", "post", probe.resolve)

			app := fiber.New()
			app.Post("/", handlers[0], toAny(handlers[1:])...)

			require.Equal(t, http.StatusNoContent, callProbe(t, app, "Bearer "+applicationTestToken(t)))
			require.True(t, probe.called)
			require.False(t, probe.ok, "a guard that does not authorize callers never vouches for a token")
			require.Equal(t, TokenCaller{}, probe.caller)
		})
	}
}

// TestAuthGuard_WithAuthorizedCaller_SourceOfTheCaller strips the
// Authorization header between the guard and the resolver, so only a caller
// taken from the Principal lib-auth published survives: under M2M inversion
// the resolver still sees the application, while under the legacy derivation
// lib-auth publishes none and the claim fallback finds no token.
func TestAuthGuard_WithAuthorizedCaller_SourceOfTheCaller(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		inversion bool
		want      TokenCaller
		ok        bool
	}{
		"M2M inversion reads the published principal": {inversion: true, want: TokenCaller{Type: TokenTypeApplication, ClientID: "ledger-client"}, ok: true},
		"legacy derivation reads the token claims":    {inversion: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := authMiddleware.NewAuthClient(newAuthorizedMockServer(t).URL, true, libLog.NewNop())
			client.M2MInversionEnabled = tc.inversion
			guard := NewAuthGuard(AuthGuardConfig{AppName: "tracer", PluginAuthEnabled: true}, client)

			probe := &callerProbe{}
			handlers := guard.WithAuthorizedCaller("reservations", "post", probe.resolve)
			require.Len(t, handlers, 2)

			stripToken := func(c fiber.Ctx) error {
				c.Request().Header.Del(fiber.HeaderAuthorization)

				return c.Next()
			}

			app := fiber.New()
			app.Post("/", handlers[0], stripToken, handlers[1])

			require.Equal(t, http.StatusNoContent, callProbe(t, app, "Bearer "+applicationTestToken(t)))
			require.True(t, probe.called)
			require.Equal(t, tc.ok, probe.ok)
			require.Equal(t, tc.want, probe.caller)
		})
	}
}

func toAny(handlers []fiber.Handler) []any {
	out := make([]any, 0, len(handlers))
	for _, handler := range handlers {
		out = append(out, handler)
	}

	return out
}

func TestAuthorizedCaller_ReadsPlatformAttributes(t *testing.T) {
	t.Parallel()

	base := jwt.MapClaims{
		"type": "application", "sub": "admin/ledger-m2m-tracer-t1", "azp": "random-client",
		"tenantId": "0195d3b45a0170008000000000000001", "sourceService": "ledger", "isInternal": "true",
	}

	for name, tc := range map[string]struct {
		mutate func(jwt.MapClaims)
		want   TokenCaller
	}{
		"tenant-manager application": {
			want: TokenCaller{Type: TokenTypeApplication, ClientID: "random-client", TenantID: "0195d3b45a0170008000000000000001", SourceService: "ledger", Internal: true},
		},
		"isInternal false": {
			mutate: func(c jwt.MapClaims) { c["isInternal"] = "false" },
			want:   TokenCaller{Type: TokenTypeApplication, ClientID: "random-client", TenantID: "0195d3b45a0170008000000000000001", SourceService: "ledger"},
		},
		"isInternal as a JSON bool": {
			mutate: func(c jwt.MapClaims) { c["isInternal"] = true },
			want:   TokenCaller{Type: TokenTypeApplication, ClientID: "random-client", TenantID: "0195d3b45a0170008000000000000001", SourceService: "ledger"},
		},
		"padded isInternal": {
			mutate: func(c jwt.MapClaims) { c["isInternal"] = " true" },
			want:   TokenCaller{Type: TokenTypeApplication, ClientID: "random-client", TenantID: "0195d3b45a0170008000000000000001", SourceService: "ledger"},
		},
		"no platform attributes": {
			mutate: func(c jwt.MapClaims) { delete(c, "tenantId"); delete(c, "sourceService"); delete(c, "isInternal") },
			want:   TokenCaller{Type: TokenTypeApplication, ClientID: "random-client"},
		},
		"non-string attributes": {
			mutate: func(c jwt.MapClaims) { c["tenantId"] = 7; c["sourceService"] = []string{"ledger"} },
			want:   TokenCaller{Type: TokenTypeApplication, ClientID: "random-client", Internal: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			claims := jwt.MapClaims{}
			for key, value := range base {
				claims[key] = value
			}

			if tc.mutate != nil {
				tc.mutate(claims)
			}

			signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("unverified"))
			require.NoError(t, err)

			var (
				got TokenCaller
				ok  bool
			)

			app := fiber.New()
			app.Post("/", func(c fiber.Ctx) error {
				got, ok = authorizedCaller(c)

				return c.SendStatus(http.StatusNoContent)
			})

			require.Equal(t, http.StatusNoContent, callProbe(t, app, "Bearer "+signed))
			require.True(t, ok)
			require.Equal(t, tc.want, got)
		})
	}
}
