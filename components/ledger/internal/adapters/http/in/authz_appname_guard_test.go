// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

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
	orgID := uuid.NewString()
	ledgerID := uuid.NewString()
	resourceID := uuid.NewString()
	base := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID

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
			byID: base + "/operation-routes/" + resourceID,
		},
		{
			name: "transaction-routes",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				registerTransactionRouteRoutesToApp(group, api, auth, &TransactionRouteHandler{}, nil, routeOpSuffixV1)
			},
			list: base + "/transaction-routes",
			byID: base + "/transaction-routes/" + resourceID,
		},
		{
			name: "account-types",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				registerAccountTypeRoutesToApp(group, api, auth, &AccountTypeHandler{}, nil, routeOpSuffixV1)
			},
			list: base + "/account-types",
			byID: base + "/account-types/" + resourceID,
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
