// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const directV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/direct"

const holdV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/hold"

const blockV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/block"

const unblockV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/unblock"

const commitV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit"

const cancelV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel"

const revertV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert"

// v2Routes enumerates every registered v2 transaction op: the Fiber path the protected
// chain mounts, the group-relative path the Huma contract advertises, the OperationID
// clients key off, and whether the op carries a request body. The create actions take the
// action name straight off the collection path; the lifecycle actions hang off
// :transaction_id and are bodiless. Defaulting to 201 Created holds for all of them, so it
// is asserted as a shared invariant instead of a per-case field. opPath is spelled out
// rather than derived from fiberPath so a typo in either const cannot pass both the mount
// and the contract assertion.
var v2Routes = []struct {
	action      string
	fiberPath   string
	opPath      string
	operationID string
	hasBody     bool
}{
	{
		action:      "direct",
		fiberPath:   directV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/direct",
		operationID: "createTransactionDirectV2",
		hasBody:     true,
	},
	{
		action:      "hold",
		fiberPath:   holdV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/hold",
		operationID: "createTransactionHoldV2",
		hasBody:     true,
	},
	{
		action:      "block",
		fiberPath:   blockV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/block",
		operationID: "createTransactionBlockV2",
		hasBody:     true,
	},
	{
		action:      "unblock",
		fiberPath:   unblockV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/unblock",
		operationID: "createTransactionUnblockV2",
		hasBody:     true,
	},
	{
		action:      "commit",
		fiberPath:   commitV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit",
		operationID: "commitTransactionV2",
	},
	{
		action:      "cancel",
		fiberPath:   cancelV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel",
		operationID: "cancelTransactionV2",
	},
	{
		action:      "revert",
		fiberPath:   revertV2RoutePath,
		opPath:      "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert",
		operationID: "revertTransactionV2",
	},
}

// concreteV2Path substitutes the Fiber path params with fixed UUIDs so a request reaches
// the mounted route (ParseUUIDPathParameters passes) instead of 404ing. Deriving it from
// fiberPath is deliberate here: it aims the auth assertion at the SAME route the mount
// test proves is registered, and a wrong path would surface as a 404 rather than the
// expected 401.
func concreteV2Path(fiberPath string) string {
	return strings.NewReplacer(
		":organization_id", "00000000-0000-0000-0000-000000000001",
		":ledger_id", "00000000-0000-0000-0000-000000000002",
		":transaction_id", "00000000-0000-0000-0000-000000000003",
	).Replace(fiberPath)
}

// registerV2TransactionRoutesForTest wires the v2 transaction ops onto a fresh Fiber app +
// its own /v2 Huma contract, exactly as the production humaMountV2 seam does. A zero-value
// TransactionHandler is safe because registration never invokes the handler.
func registerV2TransactionRoutesForTest(auth *middleware.AuthClient) *fiber.App {
	app := fiber.New()

	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2RoutesToApp(apiV2, humaAPI, auth, &TransactionHandler{}, nil)

	return app
}

// TestRegisterTransactionV2RoutesToApp_MountsRoutes asserts every v2 transaction op is
// mounted as a POST on the /v2 group (Fiber chain), sharing the v1 transactions:post
// protected chain.
func TestRegisterTransactionV2RoutesToApp_MountsRoutes(t *testing.T) {
	t.Parallel()

	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: false})

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			assert.Truef(t, routeSet[http.MethodPost+":"+rt.fiberPath],
				"should register POST %s", rt.fiberPath)
		})
	}
}

// registerV2TransactionContractForTest builds a fresh /v2 Huma document with its own
// component registry and registers only the v2 transaction contract onto it, mirroring the
// production humaMountV2 seam (namer installed before any huma.Register). It returns the
// document so contract assertions read paths and components off the same instance.
func registerV2TransactionContractForTest() *huma.OpenAPI {
	app := fiber.New()

	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	return humaAPI.OpenAPI()
}

// TestRegisterTransactionV2Routes_RegistersHumaOperations asserts every v2 transaction op
// is present on the v2 Huma document at its group-relative path, advertising the canonical
// OperationID and defaulting to 201 Created.
func TestRegisterTransactionV2Routes_RegistersHumaOperations(t *testing.T) {
	t.Parallel()

	paths := registerV2TransactionContractForTest().Paths

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := paths[rt.opPath]
			require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, rt.opPath)
			require.NotNilf(t, pathItem.Post, "%s op path should carry a POST operation", rt.action)

			assert.Equalf(t, rt.operationID, pathItem.Post.OperationID,
				"%s op should advertise OperationID %s", rt.action, rt.operationID)
			assert.Equalf(t, http.StatusCreated, pathItem.Post.DefaultStatus,
				"%s op should default to 201 Created (create/lifecycle parity)", rt.action)
		})
	}
}

// TestV2Routes_RequireAuth proves every v2 transaction route shares the v1 protected
// chain: with auth enabled and no bearer token the request is rejected before reaching
// the handler.
func TestV2Routes_RequireAuth(t *testing.T) {
	t.Parallel()

	// Address must be non-empty so Authorize enforces the token check (it is never
	// dialed: a missing token short-circuits with 401 first).
	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, concreteV2Path(rt.fiberPath), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, fiber.StatusUnauthorized, resp.StatusCode,
				"tokenless v2 %s request must be rejected by the transactions:post auth chain", rt.action)
		})
	}
}

// v2CreateBodySchemaName is the component name the v2 create-body schema is published
// under, and v2CreateBodySchemaRef the ref that names it. Both are spelled literally so a
// rename of the registered Go type shows up here instead of silently changing the served
// contract and every generated client.
const (
	v2CreateBodySchemaName = "CreateTransactionV2Input"
	v2CreateBodySchemaRef  = "#/components/schemas/" + v2CreateBodySchemaName
)

// TestRegisterTransactionV2Routes_PublishesCreateBodySchema asserts the v2 create ops
// describe their body with a $ref to the published input component instead of the opaque
// `string`/`binary` schema Huma derives from a RawBody field. The bodiless lifecycle ops
// must stay free of a requestBody.
func TestRegisterTransactionV2Routes_PublishesCreateBodySchema(t *testing.T) {
	t.Parallel()

	oapi := registerV2TransactionContractForTest()

	assert.Containsf(t, oapi.Components.Schemas.Map(), v2CreateBodySchemaName,
		"v2 contract should publish the create-body component %s", v2CreateBodySchemaName)

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := oapi.Paths[rt.opPath]
			require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, rt.opPath)
			require.NotNilf(t, pathItem.Post, "%s op path should carry a POST operation", rt.action)

			if !rt.hasBody {
				assert.Nilf(t, pathItem.Post.RequestBody,
					"bodiless lifecycle op %s must not advertise a requestBody", rt.action)

				return
			}

			require.NotNilf(t, pathItem.Post.RequestBody, "%s op should advertise a requestBody", rt.action)

			media, ok := pathItem.Post.RequestBody.Content["application/json"]
			require.Truef(t, ok, "%s op requestBody should carry an application/json media type", rt.action)
			require.NotNilf(t, media, "%s op application/json media type should not be nil", rt.action)
			require.NotNilf(t, media.Schema, "%s op application/json media type should carry a schema", rt.action)

			assert.Equalf(t, v2CreateBodySchemaRef, media.Schema.Ref,
				"%s op body schema should $ref the published v2 input component", rt.action)
			assert.Emptyf(t, media.Schema.Format,
				"%s op body schema must not stay the opaque binary RawBody schema", rt.action)
		})
	}
}

// v2CreateBodyRequiredFields are the ONLY fields the published create-body component may
// mark required: the ones common to both request forms. The side fields are deliberately
// absent — `from`/`to` and `sources`/`destinations` are alternative spellings of the same
// two sides, so requiring any of them would declare one form mandatory.
var v2CreateBodyRequiredFields = []string{"asset", "amount"}

// v2CreateBodySideFields are the four side fields of the request body: the scalar pair and
// the leg-array pair.
var v2CreateBodySideFields = []string{"from", "to", "sources", "destinations"}

// TestRegisterTransactionV2Routes_CreateBodyDocumentsBothSideForms asserts the published
// create-body component keeps the or/or between the scalar and array forms out of
// `required`: each side field is documented as a property but none of them is mandatory,
// while the fields common to both forms stay required. This is what the explicit
// `required:"false"` tags on the leg arrays buy — Huma treats a field without `omitempty`
// as required unless the tag says otherwise, so without them the contract would advertise
// the array form as the only one. Since the flat component cannot express the exclusivity
// structurally, the component description must state it and name every side field.
func TestRegisterTransactionV2Routes_CreateBodyDocumentsBothSideForms(t *testing.T) {
	t.Parallel()

	schema, ok := registerV2TransactionContractForTest().Components.Schemas.Map()[v2CreateBodySchemaName]
	require.Truef(t, ok, "v2 contract should publish the create-body component %s", v2CreateBodySchemaName)
	require.NotNilf(t, schema, "published %s component should not be nil", v2CreateBodySchemaName)

	assert.ElementsMatchf(t, v2CreateBodyRequiredFields, schema.Required,
		"%s should require only the fields common to both request forms", v2CreateBodySchemaName)
	require.NotEmptyf(t, schema.Description,
		"%s is the only place the scalar-or-arrays exclusivity can be stated", v2CreateBodySchemaName)

	for _, field := range v2CreateBodySideFields {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			assert.Containsf(t, schema.Properties, field,
				"%s should document the %s side field as a property", v2CreateBodySchemaName, field)
			assert.NotContainsf(t, schema.Required, field,
				"%s must not mark the %s side field required", v2CreateBodySchemaName, field)
			assert.Containsf(t, schema.Description, field,
				"%s description should name the %s side field when spelling out the or/or", v2CreateBodySchemaName, field)
		})
	}
}

// newV2DocForTest returns a bare /v2 Huma document with the ledger namer installed and no
// operations registered, the starting point for the body-schema publisher's guard cases.
func newV2DocForTest() huma.API {
	app := fiber.New()

	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	return humaAPI
}

// TestPublishV2CreateBodySchema_DegradesToNoOp asserts the publisher leaves the contract
// alone instead of panicking when there is nothing to attach a body schema to: a spec-disabled
// API, a document missing its component registry, a base path carrying no create ops, and a
// create op whose body is not JSON. None of them may publish the component either — it is
// registered only when it is actually referenced.
func TestPublishV2CreateBodySchema_DegradesToNoOp(t *testing.T) {
	t.Parallel()

	const basePath = "/transactions"

	tests := []struct {
		name string
		api  func() huma.API
	}{
		{
			name: "nil api",
			api:  func() huma.API { return nil },
		},
		{
			name: "document without components",
			api: func() huma.API {
				api := newV2DocForTest()
				api.OpenAPI().Components = nil

				return api
			},
		},
		{
			name: "document without a schema registry",
			api: func() huma.API {
				api := newV2DocForTest()
				api.OpenAPI().Components.Schemas = nil

				return api
			},
		},
		{
			name: "no create op registered at the base path",
			api:  newV2DocForTest,
		},
		{
			name: "create op body is not json",
			api: func() huma.API {
				api := newV2DocForTest()
				api.OpenAPI().Paths = map[string]*huma.PathItem{
					basePath + "/direct": {Post: &huma.Operation{
						RequestBody: &huma.RequestBody{
							Content: map[string]*huma.MediaType{"text/plain": {}},
						},
					}},
				}

				return api
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := tc.api()

			require.NotPanics(t, func() { publishV2CreateBodySchema(api, basePath) })

			if api == nil {
				return
			}

			if components := api.OpenAPI().Components; components != nil && components.Schemas != nil {
				assert.NotContains(t, components.Schemas.Map(), v2CreateBodySchemaName,
					"body component should be published only when an op references it")
			}
		})
	}
}
