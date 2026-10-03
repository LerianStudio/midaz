// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// carrierAuthz stands in for the Access Manager: it records every question and
// denies one that names any identifier it was told is outside the scope.
type carrierAuthz struct {
	mu        sync.Mutex
	outside   map[string]bool
	questions []map[string]string
}

func newCarrierAuthz(t *testing.T, outside ...string) (*carrierAuthz, *httptest.Server) {
	t.Helper()

	authz := &carrierAuthz{outside: make(map[string]bool, len(outside))}
	for _, id := range outside {
		authz.outside[id] = true
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("carrier authz: decode request body: %v", err)
		}

		authz.mu.Lock()
		authz.questions = append(authz.questions, body.Attributes)

		allow := true

		for _, value := range body.Attributes {
			if authz.outside[value] {
				allow = false
			}
		}
		authz.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		answer := `{"authorized":false}`
		if allow {
			answer = `{"authorized":true}`
		}

		if _, err := w.Write([]byte(answer)); err != nil {
			t.Errorf("carrier authz: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return authz, server
}

func (a *carrierAuthz) asked() []map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]map[string]string(nil), a.questions...)
}

// mountCarrierRoutes mounts the production v1 and v2 registrars whose routes read
// scope dimensions from the body or the query, behind a lib-auth client wired from
// the embedded manifest exactly as the boot wires it. A stub ends the chain behind
// the guard with 204 and counts the requests it saw.
func mountCarrierRoutes(t *testing.T, authzURL string) (*fiber.App, *int) {
	t.Helper()

	auth := &middleware.AuthClient{Address: authzURL, Enabled: true}
	require.NoError(t, declaration.WireScope(auth, ledgerembed.MidazManifest))

	reached := 0
	options := &pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: []fiber.Handler{func(c fiber.Ctx) error {
		reached++

		return c.SendStatus(fiber.StatusNoContent)
	}}}

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	v1 := app.Group("/v1")
	v1API := openapi.New(app, v1, openapi.Config{Title: "carriers-v1", Version: "test", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(v1API)
	RegisterAccountRoutesToApp(v1, v1API, auth, &AccountHandler{}, options)

	v2 := app.Group("/v2")
	v2API := openapi.New(app, v2, openapi.Config{Title: "carriers-v2", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(v2API)
	RegisterAccountV2RoutesToApp(v2, v2API, auth, &AccountHandler{}, options)
	RegisterInstrumentV2RoutesToApp(v2, v2API, auth, &InstrumentHandler{}, options, options)
	RegisterHolderAccountsV2RoutesToApp(v2, v2API, auth, &HolderAccountsHandler{}, options)
	RegisterCompositionV2RoutesToApp(v2, v2API, auth, &CompositionHandler{}, options)
	RegisterPackageV2RoutesToApp(v2, v2API, auth, &PackageHandler{}, options)
	RegisterBillingPackageV2RoutesToApp(v2, v2API, auth, &BillingPackageHandler{}, options)

	return app, &reached
}

func sendCarrierRequest(t *testing.T, app *fiber.App, method, target, token, body string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}

	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(raw)
}

// TestScopeCarriers_AccountCreateBodyPointers drives the v2 account create with a
// partner credential. A body naming no portfolio, segment, holder or parent is
// asked about with the path alone; each pointer it does name joins the question,
// and one outside the scope refuses the request before the handler.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook.
func TestScopeCarriers_AccountCreateBodyPointers(t *testing.T) {
	org, ledgerID := uuid.NewString(), uuid.NewString()
	portfolio, segment, holder, parent := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	outside := uuid.NewString()
	path := "/v2/organizations/" + org + "/ledgers/" + ledgerID + "/accounts"

	t.Run("no pointer in the body", func(t *testing.T) {
		authz, server := newCarrierAuthz(t, outside)
		app, reached := mountCarrierRoutes(t, server.URL)

		status, raw := sendCarrierRequest(t, app, fiber.MethodPost, path, bodyScopeToken(t, "partner-a"),
			`{"name":"a","assetCode":"BRL","type":"deposit","portfolioId":null}`)

		require.Equalf(t, fiber.StatusNoContent, status, "%s", raw)
		assert.Equal(t, []map[string]string{{"organizationId": org, "ledgerId": ledgerID}}, authz.asked())
		assert.Equal(t, 1, *reached)
	})

	t.Run("every pointer inside the scope", func(t *testing.T) {
		authz, server := newCarrierAuthz(t, outside)
		app, reached := mountCarrierRoutes(t, server.URL)

		body := `{"name":"a","assetCode":"BRL","type":"deposit","portfolioId":"` + portfolio + `","segmentId":"` + segment +
			`","holderId":"` + holder + `","parentAccountId":"` + parent + `"}`

		status, raw := sendCarrierRequest(t, app, fiber.MethodPost, path, bodyScopeToken(t, "partner-a"), body)

		require.Equalf(t, fiber.StatusNoContent, status, "%s", raw)
		assert.Equal(t, []map[string]string{{
			"organizationId": org, "ledgerId": ledgerID,
			"portfolioId": portfolio, "segmentId": segment, "holderId": holder, "accountId": parent,
		}}, authz.asked())
		assert.Equal(t, 1, *reached)
	})

	for _, field := range []string{"portfolioId", "segmentId", "holderId", "parentAccountId"} {
		t.Run(field+" outside the scope", func(t *testing.T) {
			_, server := newCarrierAuthz(t, outside)
			app, reached := mountCarrierRoutes(t, server.URL)

			status, raw := sendCarrierRequest(t, app, fiber.MethodPost, path, bodyScopeToken(t, "partner-a"),
				`{"name":"a","assetCode":"BRL","type":"deposit","`+field+`":"`+outside+`"}`)

			assert.Equalf(t, fiber.StatusForbidden, status, "%s", raw)
			assert.Zero(t, *reached)
		})
	}
}

// TestScopeCarriers_BodyPointersOnTheOtherWrites pins one pointer outside the
// scope on every other write that reads one from its body: each is refused before
// the handler, and the same write naming an identifier inside the scope passes.
func TestScopeCarriers_BodyPointersOnTheOtherWrites(t *testing.T) {
	org, ledgerID, holder, account := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	inside, outside := uuid.NewString(), uuid.NewString()
	ledgerPath := "/organizations/" + org + "/ledgers/" + ledgerID

	tests := []struct {
		name   string
		method string
		path   string
		body   func(id string) string
	}{
		{"v1 account create parent", fiber.MethodPost, "/v1" + ledgerPath + "/accounts",
			func(id string) string { return `{"name":"a","assetCode":"BRL","type":"deposit","parentAccountId":"` + id + `"}` }},
		{"v1 account update portfolio", fiber.MethodPatch, "/v1" + ledgerPath + "/accounts/" + account,
			func(id string) string { return `{"portfolioId":"` + id + `"}` }},
		{"v2 account update segment", fiber.MethodPatch, "/v2" + ledgerPath + "/accounts/" + account,
			func(id string) string { return `{"segmentId":"` + id + `"}` }},
		{"holder account create portfolio", fiber.MethodPost, "/v2" + ledgerPath + "/holders/" + holder + "/accounts",
			func(id string) string { return `{"name":"a","assetCode":"BRL","type":"deposit","portfolioId":"` + id + `"}` }},
		{"instrument create account", fiber.MethodPost, "/v2/organizations/" + org + "/holders/" + holder + "/instruments",
			func(id string) string { return `{"ledgerId":"` + ledgerID + `","accountId":"` + id + `"}` }},
		{"billing package target segment", fiber.MethodPost, "/v2" + ledgerPath + "/billing-packages",
			func(id string) string { return `{"label":"b","type":"maintenance","accountTarget":{"segmentId":"` + id + `"}}` }},
		{"fee package segment", fiber.MethodPost, "/v2" + ledgerPath + "/packages",
			func(id string) string { return `{"feeGroupLabel":"p","segmentId":"` + id + `"}` }},
	}

	for _, tc := range tests {
		t.Run(tc.name+" inside the scope", func(t *testing.T) {
			authz, server := newCarrierAuthz(t, outside)
			app, reached := mountCarrierRoutes(t, server.URL)

			status, raw := sendCarrierRequest(t, app, tc.method, tc.path, bodyScopeToken(t, "partner-a"), tc.body(inside))

			require.Equalf(t, fiber.StatusNoContent, status, "%s", raw)
			require.Len(t, authz.asked(), 1)
			assert.Contains(t, authz.asked()[0], "organizationId")
			assert.Containsf(t, valuesOf(authz.asked()[0]), inside, "the body pointer must join the question")
			assert.Equal(t, 1, *reached)
		})

		t.Run(tc.name+" outside the scope", func(t *testing.T) {
			_, server := newCarrierAuthz(t, outside)
			app, reached := mountCarrierRoutes(t, server.URL)

			status, raw := sendCarrierRequest(t, app, tc.method, tc.path, bodyScopeToken(t, "partner-a"), tc.body(outside))

			assert.Equalf(t, fiber.StatusForbidden, status, "%s", raw)
			assert.Zero(t, *reached)
		})
	}
}

func valuesOf(attributes map[string]string) []string {
	out := make([]string, 0, len(attributes))
	for _, v := range attributes {
		out = append(out, v)
	}

	return out
}

// TestScopeCarriers_ScopeRoutesNameMountedRoutes keeps every route-level scope
// declaration pointed at a route the unified server really mounts: a path that
// matches no registration is a declaration nothing ever reads.
func TestScopeCarriers_ScopeRoutesNameMountedRoutes(t *testing.T) {
	app, _ := buildUnifiedHumaAPI()

	mounted := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		mounted[r.Method+" "+r.Path] = true
	}

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledgerembed.MidazManifest, &manifest))
	require.NotNil(t, manifest.Scope)
	require.NotEmpty(t, manifest.Scope.Routes)

	for _, route := range manifest.Scope.Routes {
		key := strings.ToUpper(route.Method) + " " + route.Path
		assert.Truef(t, mounted[key], "scope route %s matches no mounted route", key)
	}
}
