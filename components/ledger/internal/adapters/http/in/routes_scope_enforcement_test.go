// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// newAuthzAttributeCapture stands in for the Access Manager and records the
// instance identifiers the route actually sent, request by request. It always
// denies, so the chain stops at the 403 and the business terminals never run —
// zero-value handlers are therefore sufficient.
//
// The capture asserts on what LEFT the process. A route that compiles, or a
// declaration that merely exists, proves nothing: only the recorded map proves the
// authorization service was told WHICH organization and WHICH ledger the request
// pointed at.
func newAuthzAttributeCapture(t *testing.T, captured *map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authz capture: decode request body: %v", err)
		}

		*captured = body.Attributes

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte(`{"authorized":false}`)); err != nil {
			t.Errorf("authz capture: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return server
}

// scopeEnforcementRow is one production route driven end to end: the registrar that
// mounts it, the request to send, and the exact attribute map the Access Manager
// must receive for it.
type scopeEnforcementRow struct {
	name     string
	register func(group fiber.Router, api huma.API, auth *middleware.AuthClient)
	method   string
	path     string
	// want is the attribute map the authorization service must receive. nil means
	// the route names no instance and must send none.
	want map[string]string
}

// TestScopeEnforcement_ProductionRoutesSendTheirInstanceIdentifiers drives the real
// registrars through the real lib-auth client, wired with the embedded manifest's
// scope as the boot wires it, and asserts, per route, the identifiers
// that reached the authorization service — name and value.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools.
func TestScopeEnforcement_ProductionRoutesSendTheirInstanceIdentifiers(t *testing.T) {
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	entityID := uuid.New().String()

	orgLedger := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID

	bothDims := map[string]string{"organizationId": orgID, "ledgerId": ledgerID}
	orgOnly := map[string]string{"organizationId": orgID}

	// with returns base plus one entity dimension, leaving base untouched.
	with := func(base map[string]string, name, value string) map[string]string {
		out := make(map[string]string, len(base)+1)
		for k, v := range base {
			out[k] = v
		}

		out[name] = value

		return out
	}

	ledgerRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		registerLedgerRoutesToApp(group, api, auth, &LedgerHandler{}, nil, v1OpSuffix)
	}
	accountRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		RegisterAccountRoutesToApp(group, api, auth, &AccountHandler{}, nil)
	}
	holderRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		registerHolderRoutesToApp(group, api, auth, &HolderHandler{}, nil, nil, v1OpSuffix)
	}
	organizationRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		registerOrganizationRoutesToApp(group, api, auth, &OrganizationHandler{}, nil, v1OpSuffix)
	}
	metadataRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		registerMetadataIndexRoutesToApp(group, api, auth, &MetadataIndexHandler{}, nil, v1OpSuffix)
	}
	transactionV2Registrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		RegisterTransactionV2RoutesToApp(group, api, auth, &TransactionHandler{}, nil)
	}

	feeDebtRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		RegisterFeeDebtV2RoutesToApp(group, api, auth, &FeeDebtHandler{}, nil)
	}
	feeDebtCollectRegistrar := func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
		RegisterFeeDebtCollectV2RoutesToApp(group, api, auth, &TransactionHandler{}, nil)
	}

	rows := []scopeEnforcementRow{
		{
			name:     "list ledgers carries the organization only",
			register: ledgerRegistrar,
			method:   fiber.MethodGet,
			path:     "/v1/organizations/" + orgID + "/ledgers",
			want:     orgOnly,
		},
		{
			name:     "get one ledger carries organization and ledger",
			register: ledgerRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger,
			want:     bothDims,
		},
		{
			name:     "delete one ledger carries organization and ledger",
			register: ledgerRegistrar,
			method:   fiber.MethodDelete,
			path:     orgLedger,
			want:     bothDims,
		},
		{
			name:     "ledger settings carries organization and ledger",
			register: ledgerRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger + "/settings",
			want:     bothDims,
		},
		{
			name:     "list accounts carries organization and ledger",
			register: accountRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger + "/accounts",
			want:     bothDims,
		},
		{
			name:     "get one account carries organization, ledger and account",
			register: accountRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger + "/accounts/" + entityID,
			want:     with(bothDims, "accountId", entityID),
		},
		{
			name:     "account by alias carries organization and ledger",
			register: accountRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger + "/accounts/alias/some-alias",
			want:     bothDims,
		},
		{
			name:     "list holders carries the organization only",
			register: holderRegistrar,
			method:   fiber.MethodGet,
			path:     "/v1/organizations/" + orgID + "/holders",
			want:     orgOnly,
		},
		{
			name:     "get one holder carries organization and holder",
			register: holderRegistrar,
			method:   fiber.MethodGet,
			path:     "/v1/organizations/" + orgID + "/holders/" + entityID,
			want:     with(orgOnly, "holderId", entityID),
		},
		{
			name:     "get one organization carries the organization",
			register: organizationRegistrar,
			method:   fiber.MethodGet,
			path:     "/v1/organizations/" + orgID,
			want:     orgOnly,
		},
		{
			name:     "update one organization carries the organization",
			register: organizationRegistrar,
			method:   fiber.MethodPatch,
			path:     "/v1/organizations/" + orgID,
			want:     orgOnly,
		},
		{
			name:     "delete one organization carries the organization",
			register: organizationRegistrar,
			method:   fiber.MethodDelete,
			path:     "/v1/organizations/" + orgID,
			want:     orgOnly,
		},
		{
			name:     "list organizations sends no identifier",
			register: organizationRegistrar,
			method:   fiber.MethodGet,
			path:     "/v1/organizations",
			want:     nil,
		},
		{
			name:     "the metadata-index surface names no instance",
			register: metadataRegistrar,
			method:   fiber.MethodGet,
			path:     "/v1/settings/metadata-indexes",
			want:     nil,
		},
		{
			// The v2 create routes take the organization and the ledger in the BODY.
			// Nothing in the path names them, so nothing is sent.
			name:     "the v2 transaction create route sends no identifier",
			register: transactionV2Registrar,
			method:   fiber.MethodPost,
			path:     "/v1/transactions/direct",
			want:     nil,
		},
		{
			name:     "a v2 transaction lifecycle route carries organization, ledger and transaction",
			register: transactionV2Registrar,
			method:   fiber.MethodPost,
			path:     orgLedger + "/transactions/" + entityID + "/commit",
			want:     with(bothDims, "transactionId", entityID),
		},
		{
			name:     "list fee debts carries organization and ledger",
			register: feeDebtRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger + "/fee-debts",
			want:     bothDims,
		},
		{
			name:     "get one fee debt carries organization, ledger and fee debt",
			register: feeDebtRegistrar,
			method:   fiber.MethodGet,
			path:     orgLedger + "/fee-debts/some-debt",
			want:     with(bothDims, "feeDebtId", "some-debt"),
		},
		{
			name:     "collect fee debts carries organization and ledger",
			register: feeDebtCollectRegistrar,
			method:   fiber.MethodPost,
			path:     orgLedger + "/fee-debts/collect",
			want:     bothDims,
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var captured map[string]string

			server := newAuthzAttributeCapture(t, &captured)
			auth := &middleware.AuthClient{Address: server.URL, Enabled: true}
			require.NoError(t, declaration.WireScope(auth, ledgerembed.MidazManifest))

			app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
			libProblem.Install()
			app.Use(ledgerMiddleware.ErrorEnvelope())

			group := app.Group("/v1")
			api := openapi.New(app, group, openapi.Config{Title: "scope-enforcement", Version: "test", Servers: []string{"/v1"}})
			pkgHTTP.InstallLedgerSchemaNamer(api)
			row.register(group, api, auth)

			req := httptest.NewRequest(row.method, row.path, nil)
			req.Header.Set(fiber.HeaderAuthorization, "Bearer "+guardBearerToken(t))

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
				"%s %s must reach the authorization service and be denied", row.method, row.path)

			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, row.want, captured,
				"%s %s sent the wrong instance identifiers to the authorization service", row.method, row.path)
		})
	}
}
