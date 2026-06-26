// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	blockRoutePath   = "/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/block"
	unblockRoutePath = "/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/unblock"
	jsonRoutePath    = "/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json"
)

// registerTransactionRoutesForTest registers the transaction routes onto a fresh
// Fiber app using zero-value handlers. Route registration only wires the handler
// chain; the business handlers are not invoked, so nil internal dependencies are
// safe here. This mirrors the existing routes_test.go pattern.
func registerTransactionRoutesForTest(auth *middleware.AuthClient, opts *pkgHTTP.ProtectedRouteOptions) *fiber.App {
	app := fiber.New()

	RegisterTransactionRoutesToApp(
		app,
		auth,
		&TransactionHandler{},
		&OperationHandler{},
		&AssetRateHandler{},
		&BalanceHandler{},
		&OperationRouteHandler{},
		&TransactionRouteHandler{},
		opts,
	)

	return app
}

// TestRegisterTransactionRoutesToApp_RegistersBlockAndUnblock asserts that the
// block and unblock POST routes are wired into RegisterTransactionRoutesToApp
// alongside the existing creation routes.
func TestRegisterTransactionRoutesToApp_RegistersBlockAndUnblock(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerTransactionRoutesForTest(auth, nil)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[fiber.MethodPost+":"+blockRoutePath],
		"should register POST transactions/block")
	assert.True(t, routeSet[fiber.MethodPost+":"+unblockRoutePath],
		"should register POST transactions/unblock")
	// Sanity: the reference creation route they are grouped with stays registered.
	assert.True(t, routeSet[fiber.MethodPost+":"+jsonRoutePath],
		"reference POST transactions/json must remain registered")
}

// TestBlockUnblockRoutes_RequireAuthLikeJSON proves the block and unblock routes
// share the exact same protected auth chain as transactions/json: with auth
// enabled and no bearer token, every one of these routes is rejected with 401
// before reaching the body-parsing/business handler.
func TestBlockUnblockRoutes_RequireAuthLikeJSON(t *testing.T) {
	t.Parallel()

	// Address must be non-empty so Authorize enforces the token check
	// (it is never dialed: a missing token short-circuits with 401 first).
	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerTransactionRoutesForTest(auth, nil)

	concretePath := func(template string) string {
		// Replace the path params with concrete values so the route matches.
		path := template
		path = replaceFirst(path, ":organization_id", "00000000-0000-0000-0000-000000000001")
		path = replaceFirst(path, ":ledger_id", "00000000-0000-0000-0000-000000000002")

		return path
	}

	for _, tc := range []struct {
		name     string
		template string
	}{
		{name: "json reference", template: jsonRoutePath},
		{name: "block", template: blockRoutePath},
		{name: "unblock", template: unblockRoutePath},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(fiber.MethodPost, concretePath(tc.template), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
				"%s must be guarded by the transactions:post auth chain and reject a tokenless request with 401", tc.name)
		})
	}
}

func replaceFirst(s, old, new string) string {
	idx := indexOf(s, old)
	if idx < 0 {
		return s
	}

	return s[:idx] + new + s[idx+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}
