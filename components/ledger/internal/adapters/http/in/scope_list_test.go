// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// partnerListToken is a parseable application token bound to a partner: only a
// partner-bound request carries a filter the authorization service answers.
func partnerListToken(t *testing.T) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":    "application",
		"owner":   "list-org",
		"sub":     "list-org/list-app",
		"partner": "list-partner",
	}).SignedString([]byte("list-secret"))
	require.NoError(t, err)

	return signed
}

// listScopeThroughGuard runs a filtering route behind the real guard, against an
// authorization service answering allowed, and returns what the handler reads.
func listScopeThroughGuard(t *testing.T, allowed string, token string, dimensions ...string) pkgHTTP.ScopeConfinement {
	t.Helper()

	return listScope(guardedContext(t, allowed, token, dimensions...), dimensions...)
}

// partnerScopedContext is the request context a filtering route hands its handler
// for a partner credential the authorization service answers with allowed.
func partnerScopedContext(t *testing.T, allowed string, dimensions ...string) context.Context {
	t.Helper()

	return guardedContext(t, allowed, partnerListToken(t), dimensions...)
}

// guardedContext runs a filtering route behind the real guard and returns the
// context its handler receives.
func guardedContext(t *testing.T, allowed string, token string, dimensions ...string) context.Context {
	t.Helper()

	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorized":true` + allowed + `}`))
	}))
	t.Cleanup(authz.Close)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	declaration := middleware.RequireScope(midazName,
		middleware.Dim("organizationId", middleware.FromPath).At("organization_id"),
	).Filter(dimensions...)

	var got context.Context

	app := fiber.New()
	app.Get("/organizations/:organization_id/things",
		auth.Authorize(midazName, "accounts", "get", declaration),
		func(c fiber.Ctx) error {
			got = c.Context()

			return c.SendStatus(fiber.StatusOK)
		})

	req := httptest.NewRequest(fiber.MethodGet, "/organizations/"+uuid.NewString()+"/things", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	return got
}

func TestListScope(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	t.Run("the allowed values confine the list", func(t *testing.T) {
		got := listScopeThroughGuard(t, `,"allowed":{"accountId":["`+a.String()+`","`+b.String()+`"]}`, partnerListToken(t), "accountId", "portfolioId")
		assert.Equal(t, pkgHTTP.ScopeConfinement{"accountId": {a, b}}, got, "a dimension with no allowed values confines nothing")
	})

	t.Run("an empty allowed list lists nothing", func(t *testing.T) {
		got := listScopeThroughGuard(t, `,"allowed":{"accountId":[]}`, partnerListToken(t), "accountId")
		assert.Equal(t, pkgHTTP.ScopeConfinement{"accountId": {}}, got)
		assert.True(t, got.ListsNothing())
	})

	t.Run("a value that is not an id confines to nothing more", func(t *testing.T) {
		got := listScopeThroughGuard(t, `,"allowed":{"accountId":["not-an-id","`+a.String()+`"]}`, partnerListToken(t), "accountId")
		assert.Equal(t, pkgHTTP.ScopeConfinement{"accountId": {a}}, got)
	})

	t.Run("a caller bound to no partner is not confined", func(t *testing.T) {
		got := listScopeThroughGuard(t, `,"allowed":{"accountId":[]}`, guardBearerToken(t), "accountId")
		assert.Nil(t, got)
	})

	t.Run("a context that never passed the guard is not confined", func(t *testing.T) {
		assert.Nil(t, listScope(context.Background(), "accountId"))
	})
}

// TestListScope_AbsentDimensionIsUnrestricted pins, through the real guard, that a
// filtered dimension the authorization service leaves out of its answer confines
// nothing, while one answered with an empty list confines to nothing.
func TestListScope_AbsentDimensionIsUnrestricted(t *testing.T) {
	ledgerID := uuid.New()

	got := listScopeThroughGuard(t, `,"allowed":{"ledgerId":["`+ledgerID.String()+`"]}`, partnerListToken(t), scopeDimensionLedger, scopeDimensionAccount)
	assert.Equal(t, pkgHTTP.ScopeConfinement{"ledgerId": {ledgerID}}, got, "accountId, left out, confines nothing")

	got = listScopeThroughGuard(t, `,"allowed":{"accountId":[]}`, partnerListToken(t), scopeDimensionLedger, scopeDimensionAccount)
	assert.Equal(t, pkgHTTP.ScopeConfinement{"accountId": {}}, got)
	assert.True(t, got.ListsNothing(), "an empty answer confines to nothing")
}
