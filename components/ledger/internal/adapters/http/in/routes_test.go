// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// routeSetOf collects every route registered on app as "METHOD:path", stripping prefix
// from the path so surfaces mounted on different version groups compare directly. Pass an
// empty prefix to keep the paths as Fiber reports them.
func routeSetOf(app *fiber.App, prefix string) map[string]bool {
	set := make(map[string]bool, len(app.GetRoutes()))
	for _, r := range app.GetRoutes() {
		set[r.Method+":"+strings.TrimPrefix(r.Path, prefix)] = true
	}

	return set
}

// routeShapeOf maps every registered route to "METHOD:path" -> the handler-chain length of
// each entry Fiber holds for it, sorted. A registrar mounts TWO entries per op — the auth
// middleware chain (no terminal, falls through) and the Huma terminal — so the chain
// lengths, not the path set, are what separate a guarded route from a public one: dropping
// the auth attach leaves the path mounted by the terminal alone.
func routeShapeOf(app *fiber.App, prefix string) map[string][]int {
	shape := make(map[string][]int)
	for _, r := range app.GetRoutes() {
		key := r.Method + ":" + strings.TrimPrefix(r.Path, prefix)
		shape[key] = append(shape[key], len(r.Handlers))
	}

	for _, lengths := range shape {
		sort.Ints(lengths)
	}

	return shape
}

// sortedRouteKeys returns the keys of set in lexical order, for stable failure messages and
// set comparison.
func sortedRouteKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// TestRegisterRoute_PreservesAuthFirstChainOrder locks the money-path invariant that
// registerRoute invokes chain[0] (auth) first and then the remaining handlers in
// chain[1]→chain[n] order. Each marker handler appends its name to a shared slice and
// calls c.Next(); the terminal returns 200. The observed order proves auth always runs
// before tenant/parse/terminal after the Fiber v3 (handler any, handlers ...any) split.
func TestRegisterRoute_PreservesAuthFirstChainOrder(t *testing.T) {
	app := fiber.New()

	var order []string
	marker := func(name string) fiber.Handler {
		return func(c fiber.Ctx) error {
			order = append(order, name)
			return c.Next()
		}
	}

	chain := []fiber.Handler{
		marker("auth"),
		marker("tenant"),
		marker("parse"),
		func(c fiber.Ctx) error {
			order = append(order, "terminal")
			return c.SendStatus(fiber.StatusOK)
		},
	}

	registerRoute(app, fiber.MethodGet, "/register-route-order", chain)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/register-route-order", nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"auth", "tenant", "parse", "terminal"}, order)
}

// TestRegisterRoute_SingleHandlerChain covers the append([]any{handler}, tail...)
// boundary where tail is empty — the ProtectedRouteChain len==1 case (auth handler only
// acting as terminal). The route must register and respond without panicking.
func TestRegisterRoute_SingleHandlerChain(t *testing.T) {
	app := fiber.New()

	terminalCalled := false
	chain := []fiber.Handler{
		func(c fiber.Ctx) error {
			terminalCalled = true
			return c.SendStatus(fiber.StatusOK)
		},
	}

	require.NotPanics(t, func() {
		registerRoute(app, fiber.MethodGet, "/register-route-single", chain)
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/register-route-single", nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, terminalCalled, "single-handler chain terminal should fire")
}

// TestRouteVerbHelpers_MapToHTTPMethods locks each verb helper to the HTTP method it
// names. The helpers are one-line wrappers around registerRoute and every registrar in
// routes.go picks its method by calling one of them, so a copy-paste swap (routePut
// registering PATCH) would silently repoint a whole resource's verb — and the authz tuple
// rides the verb. Each helper registers on its own path so the method set at that path is
// attributable to exactly one helper.
func TestRouteVerbHelpers_MapToHTTPMethods(t *testing.T) {
	terminal := []fiber.Handler{func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }}

	cases := []struct {
		name   string
		path   string
		method string
		route  func(r fiber.Router, path string, chain []fiber.Handler)
	}{
		{name: "routePost", path: "/verb-post", method: fiber.MethodPost, route: routePost},
		{name: "routeGet", path: "/verb-get", method: fiber.MethodGet, route: routeGet},
		{name: "routePatch", path: "/verb-patch", method: fiber.MethodPatch, route: routePatch},
		{name: "routePut", path: "/verb-put", method: fiber.MethodPut, route: routePut},
		{name: "routeDelete", path: "/verb-delete", method: fiber.MethodDelete, route: routeDelete},
		{name: "routeHead", path: "/verb-head", method: fiber.MethodHead, route: routeHead},
	}

	app := fiber.New()
	for _, tc := range cases {
		tc.route(app, tc.path, terminal)
	}

	mounted := routeSetOf(app, "")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Truef(t, mounted[tc.method+":"+tc.path],
				"%s must register %s %s; mounted: %v", tc.name, tc.method, tc.path, sortedRouteKeys(mounted))

			for _, other := range cases {
				if other.method == tc.method {
					continue
				}

				assert.Falsef(t, mounted[other.method+":"+tc.path],
					"%s must not also register %s on %s", tc.name, other.method, tc.path)
			}
		})
	}
}

// TestV2RegistrarsMountSameSurfaceAsV1 locks the contract the RegisterXxxV2RoutesToApp
// wrappers exist to keep: same paths, same methods, same handlers as their /v1 twin,
// differing only in the operation IDs the document publishes. Both wrappers delegate to
// one unexported core, so the surfaces can only diverge if a wrapper stops delegating —
// which this catches by comparing the group-relative route shape (paths, methods, and the
// handler-chain length of every entry mounted on each) side by side.
//
// Each version is mounted on its OWN app and Huma API: the operation-ID suffix is the only
// thing keeping v1 and v2 registrations distinct inside a shared document, and that
// disjunction is already guarded by TestContractOperationIDsAreUniqueAcrossVersions.
//
// MUST-NOT-PARALLELIZE: newLedgerHumaTestAPI calls libProblem.Install().
func TestV2RegistrarsMountSameSurfaceAsV1(t *testing.T) {
	type registrar func(group fiber.Router, api huma.API, auth *middleware.AuthClient)

	cases := []struct {
		name string
		v1   registrar
		v2   registrar
	}{
		{
			name: "assets",
			v1: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterAssetRoutesToApp(g, a, auth, &AssetHandler{}, nil)
			},
			v2: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterAssetV2RoutesToApp(g, a, auth, &AssetHandler{}, nil)
			},
		},
		{
			name: "balances",
			v1: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterBalanceRoutesToApp(g, a, auth, &BalanceHandler{}, nil)
			},
			v2: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterBalanceV2RoutesToApp(g, a, auth, &BalanceHandler{}, nil)
			},
		},
		{
			name: "operations",
			v1: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterOperationRoutesToApp(g, a, auth, &OperationHandler{}, nil)
			},
			v2: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterOperationV2RoutesToApp(g, a, auth, &OperationHandler{}, nil)
			},
		},
		{
			name: "transaction-count",
			v1: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterCountTransactionRoutesToApp(g, a, auth, &TransactionHandler{}, nil)
			},
			v2: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterCountTransactionV2RoutesToApp(g, a, auth, &TransactionHandler{}, nil)
			},
		},
		{
			name: "operation-routes",
			v1: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterOperationRouteRoutesToApp(g, a, auth, &OperationRouteHandler{}, nil)
			},
			v2: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterOperationRouteV2RoutesToApp(g, a, auth, &OperationRouteHandler{}, nil)
			},
		},
		{
			name: "transaction-routes",
			v1: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterTransactionRouteRoutesToApp(g, a, auth, &TransactionRouteHandler{}, nil)
			},
			v2: func(g fiber.Router, a huma.API, auth *middleware.AuthClient) {
				RegisterTransactionRouteV2RoutesToApp(g, a, auth, &TransactionRouteHandler{}, nil)
			},
		},
	}

	auth := &middleware.AuthClient{Enabled: false}

	mount := func(register registrar) map[string][]int {
		app := fiber.New()
		group, hAPI := newLedgerHumaTestAPI(app, "/v1")
		register(group, hAPI, auth)

		return routeShapeOf(app, "/v1")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v1Shape := mount(tc.v1)
			require.NotEmptyf(t, v1Shape, "%s /v1 registrar must mount at least one route", tc.name)

			assert.Equalf(t, v1Shape, mount(tc.v2),
				"%s /v2 surface must mirror /v1 path-for-path, method-for-method and chain-for-chain", tc.name)
		})
	}
}

// TestAuthz_RoutingResources_AuthorizeUnderMidazAppName locks the authorization
// appName for the three route-management resources (operation-routes,
// transaction-routes, account-types): every op must be guarded by
// auth.Authorize(midazName, resource, verb). It drives the PRODUCTION registration
// functions through a capturing authz server that records the forwarded product;
// a future migration that repoints any of these ops to a different appName makes
// the recorded product diverge and fails this guard.
//
// The capturing server always denies (authorized=false). The auth middleware
// forwards the product to the authz service BEFORE reading the decision, so the
// product is captured on every request while the 403 short-circuits the chain —
// the business terminals never run, so zero-value handlers are sufficient.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook and
// Huma validation uses process-global sync.Pools; concurrent builds cross-
// contaminate. These cases are sub-second; keep them sequential.
func TestAuthz_RoutingResources_AuthorizeUnderMidazAppName(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()
	resourceID := uuid.New()
	base := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String()

	cases := []struct {
		name     string
		register func(group fiber.Router, api huma.API, auth *middleware.AuthClient)
		list     string
		byID     string
	}{
		{
			name: "operation-routes",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				registerOperationRouteRoutesToApp(group, api, auth, &OperationRouteHandler{}, nil, routeOpSuffixV1)
			},
			list: base + "/operation-routes",
			byID: base + "/operation-routes/" + resourceID.String(),
		},
		{
			name: "transaction-routes",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				registerTransactionRouteRoutesToApp(group, api, auth, &TransactionRouteHandler{}, nil, routeOpSuffixV1)
			},
			list: base + "/transaction-routes",
			byID: base + "/transaction-routes/" + resourceID.String(),
		},
		{
			name: "account-types",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				registerAccountTypeRoutesToApp(group, api, auth, &AccountTypeHandler{}, nil, routeOpSuffixV1)
			},
			list: base + "/account-types",
			byID: base + "/account-types/" + resourceID.String(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var product string

			srv := newAuthzProductCapture(t, &product)
			defer srv.Close()

			auth := &middleware.AuthClient{Address: srv.URL, Enabled: true}

			f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
			libProblem.Install()

			// Mirror production: the ledger registers ErrorEnvelope on the app root, so
			// /v1 serves the v3 envelope.
			f.Use(ledgerMiddleware.ErrorEnvelope())

			group := f.Group("/v1")
			api := openapi.New(f, group, openapi.Config{Title: "authz-guard", Version: "test", Servers: []string{"/v1"}})
			tc.register(group, api, auth)

			token := "Bearer " + guardBearerToken(t)

			verbs := []struct {
				method string
				path   string
			}{
				{fiber.MethodPost, tc.list},
				{fiber.MethodGet, tc.list},
				{fiber.MethodGet, tc.byID},
				{fiber.MethodPatch, tc.byID},
				{fiber.MethodDelete, tc.byID},
			}

			for _, v := range verbs {
				product = ""

				req := httptest.NewRequest(v.method, v.path, nil)
				req.Header.Set("Authorization", token)

				resp, err := f.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)
				require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
					"%s %s must reach auth and be denied", v.method, v.path)
				assert.Equalf(t, midazName, product,
					"%s %s must authorize under the midaz appName, got %q", v.method, v.path, product)
			}
		})
	}
}

// newAuthzProductCapture returns an httptest server standing in for the authz
// service. It records the forwarded product into *product and always denies, so
// the caller's route chain stops at the 403 without running business terminals.
func newAuthzProductCapture(t *testing.T, product *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authz capture: decode request body: %v", err)
		}

		*product = body["product"]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte(`{"authorized":false}`)); err != nil {
			t.Errorf("authz capture: write response: %v", err)
		}
	}))
}

// guardBearerToken mints a normal-user token whose owner+sub claims let the auth
// middleware derive a subject and forward the route product. The token is parsed
// unverified by the middleware (no verification cert configured), so the signing
// key is irrelevant.
func guardBearerToken(t *testing.T) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":  "normal-user",
		"owner": "guard-org",
		"sub":   "guard-user",
	})

	signed, err := tok.SignedString([]byte("guard-secret"))
	require.NoError(t, err)

	return signed
}
