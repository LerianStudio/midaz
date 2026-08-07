// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const directV2RoutePath = "/v2/transactions/direct"

const holdV2RoutePath = "/v2/transactions/hold"

const blockV2RoutePath = "/v2/transactions/block"

const unblockV2RoutePath = "/v2/transactions/unblock"

const commitV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit"

const cancelV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel"

const revertV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert"

// v2Routes enumerates every registered v2 transaction op: the Fiber path the protected
// chain mounts, the group-relative path the Huma contract advertises, the OperationID
// clients key off, and whether the op carries a request body. The create actions name no
// organization or ledger — they are scoped by the request body; the lifecycle actions
// address an existing transaction, so they stay under the organization/ledger prefix and
// hang off :transaction_id, and they are bodiless. Defaulting to 201 Created holds for all
// of them, so it is asserted as a shared invariant instead of a per-case field. opPath is
// spelled out rather than derived from fiberPath so a typo in either const cannot pass both
// the mount and the contract assertion.
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
		opPath:      "/transactions/direct",
		operationID: "createTransactionDirectV2",
		hasBody:     true,
	},
	{
		action:      "hold",
		fiberPath:   holdV2RoutePath,
		opPath:      "/transactions/hold",
		operationID: "createTransactionHoldV2",
		hasBody:     true,
	},
	{
		action:      "block",
		fiberPath:   blockV2RoutePath,
		opPath:      "/transactions/block",
		operationID: "createTransactionBlockV2",
		hasBody:     true,
	},
	{
		action:      "unblock",
		fiberPath:   unblockV2RoutePath,
		opPath:      "/transactions/unblock",
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

// concreteV2Path substitutes whichever Fiber path params a route declares with fixed UUIDs so
// a request reaches the mounted route instead of 404ing; a create path declares none and comes
// back unchanged. Deriving it from fiberPath is deliberate here: it aims the auth assertion at
// the SAME route the mount test proves is registered, and a wrong path would surface as a 404
// rather than the expected 401.
func concreteV2Path(fiberPath string) string {
	return strings.NewReplacer(
		":organization_id", "00000000-0000-0000-0000-000000000001",
		":ledger_id", "00000000-0000-0000-0000-000000000002",
		":transaction_id", "00000000-0000-0000-0000-000000000003",
	).Replace(fiberPath)
}

// registerV2TransactionSurfaceForTest wires the v2 transaction ops onto a fresh Fiber app +
// its own /v2 Huma contract, exactly as the production humaMountV2 seam does, and returns
// BOTH halves of that one registration: the app the Fiber chain is mounted on and the
// document the Huma contract was written to. Returning them together is what lets a test
// compare the two sides against each other instead of against a literal it also owns. A
// zero-value TransactionHandler is safe because registration never invokes the handler.
func registerV2TransactionSurfaceForTest(auth *middleware.AuthClient) (*fiber.App, *huma.OpenAPI) {
	app := fiber.New()

	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2RoutesToApp(apiV2, humaAPI, auth, &TransactionHandler{}, nil)

	return app, humaAPI.OpenAPI()
}

// registerV2TransactionRoutesForTest keeps the app-only view for the tests that assert on
// routing alone.
func registerV2TransactionRoutesForTest(auth *middleware.AuthClient) *fiber.App {
	app, _ := registerV2TransactionSurfaceForTest(auth)

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

// registerIsolatedV2TransactionContractForTest builds a fresh Huma document with its own
// component registry and registers only the v2 transaction contract onto it, group-relative with
// no version prefix (namer installed before any huma.Register). It returns the document so the
// component / required-fields / bounds assertions read schemas and group-relative paths off one
// instance.
//
// This is an ISOLATED document, NOT how production assembles the served contract. Production mounts
// v2 through a /v2 Huma group hung off the single shared contract — the seam buildUnifiedHumaAPI
// (contract_spec_routes_test.go) exercises and the one that generates the committed dump. The
// assertions reading this document care only about component shape and the group-relative
// registration, neither of which depends on the version prefix a client sees; keeping it bare
// spares them from reaching across the whole ledger surface to name a v2 component. Path-key
// assertions that DO depend on the /v2 prefix read the real document instead.
func registerIsolatedV2TransactionContractForTest() *huma.OpenAPI {
	app := fiber.New()

	humaAPI := openapi.New(app, app, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	return humaAPI.OpenAPI()
}

// TestRegisterTransactionV2Routes_RegistersHumaOperations asserts every v2 transaction op
// is present on the v2 Huma document at its group-relative path, advertising the canonical
// OperationID and defaulting to 201 Created.
func TestRegisterTransactionV2Routes_RegistersHumaOperations(t *testing.T) {
	t.Parallel()

	paths := registerIsolatedV2TransactionContractForTest().Paths

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

// humaPathToConcreteURL turns a group-relative Huma path into a request URL on the /v2 group,
// substituting the fixed UUIDs concreteV2Path uses for the Fiber spelling of the same params.
// The two spellings must resolve to the same URL for a path both sides declare, which is what
// makes them comparable.
func humaPathToConcreteURL(opPath string) string {
	return "/v2" + strings.NewReplacer(
		"{organization_id}", "00000000-0000-0000-0000-000000000001",
		"{ledger_id}", "00000000-0000-0000-0000-000000000002",
		"{transaction_id}", "00000000-0000-0000-0000-000000000003",
	).Replace(opPath)
}

// TestV2CreateOps_ContractPathsSitBehindTheGuardChain proves the published contract and the
// mounted Fiber chain name the SAME path for every v2 create op, by reading the paths off the
// live contract and requesting them against the live app from ONE registration — never off a
// literal this test also owns.
//
// A tokenless request answers 401 only when the guard chain is mounted at the requested path.
// A create path the contract advertises but the chain does not guard reaches the Huma terminal
// instead and is answered by the body decoder, so editing either side alone turns this red:
// move the chain and the contract's path is unguarded; move the contract and its new path is
// unguarded. The Fiber route the Huma adapter installs is what makes the unguarded case
// observable as a non-401 rather than as a 404.
func TestV2CreateOps_ContractPathsSitBehindTheGuardChain(t *testing.T) {
	t.Parallel()

	// Address must be non-empty so Authorize enforces the token check (it is never dialed:
	// a missing token short-circuits with 401 first).
	app, oapi := registerV2TransactionSurfaceForTest(&middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	// The create ops are exactly the ops carrying a request body; the lifecycle ops are
	// bodiless, which is asserted independently by the create-body-schema test.
	contractCreatePaths := make([]string, 0, len(v2CreateActions))

	for opPath, item := range oapi.Paths {
		if item.Post != nil && item.Post.RequestBody != nil {
			contractCreatePaths = append(contractCreatePaths, opPath)
		}
	}

	require.Len(t, contractCreatePaths, len(v2CreateActions),
		"the contract must advertise one body-carrying op per v2 create action")

	for _, opPath := range contractCreatePaths {
		t.Run(opPath, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, humaPathToConcreteURL(opPath), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, fiber.StatusUnauthorized, resp.StatusCode,
				"the contract path %q must be the path the transactions:post guard chain is mounted on", opPath)
		})
	}
}

// v2LegacyScopedCreateURLs are the organization/ledger-scoped create URLs the v2 surface no
// longer serves. A create is scoped by its body, so the same request posted under a URL prefix
// naming an organization and a ledger must not be routed: routing it would leave two disagreeing
// sources of scope on one request.
var v2LegacyScopedCreateURLs = []string{
	"/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/direct",
	"/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/hold",
	"/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/block",
	"/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/unblock",
}

// TestV2CreateOps_ScopedPathIsNotRouted proves the scope-carrying create paths answer 404 —
// neither the guard chain nor a Huma terminal is mounted on them. Auth is DISABLED so a 401
// cannot stand in for the absence of a route.
func TestV2CreateOps_ScopedPathIsNotRouted(t *testing.T) {
	t.Parallel()

	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: false})

	for _, url := range v2LegacyScopedCreateURLs {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, fiber.StatusNotFound, resp.StatusCode,
				"the scope-carrying create URL %q must not be routed", url)
		})
	}
}

// v2LifecycleActions are the three v2 ops that act on an existing transaction.
var v2LifecycleActions = []string{"commit", "cancel", "revert"}

// TestV2LifecycleOps_StayOrganizationAndLedgerScoped locks the lifecycle ops to the
// organization/ledger-scoped path on BOTH sides of the surface. They create no transaction and
// carry no body, so they have no body scope to read: their scope can only come from the URL, and
// dropping it from their path would leave them unable to name the transaction they act on.
func TestV2LifecycleOps_StayOrganizationAndLedgerScoped(t *testing.T) {
	t.Parallel()

	app, oapi := registerV2TransactionSurfaceForTest(&middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	for _, action := range v2LifecycleActions {
		t.Run(action, func(t *testing.T) {
			t.Parallel()

			const (
				fiberPrefix = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/"
				opPrefix    = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/"
			)

			assert.Truef(t, routeSet[http.MethodPost+":"+fiberPrefix+action],
				"the %s op must stay mounted on the organization/ledger-scoped Fiber path", action)

			pathItem, ok := oapi.Paths[opPrefix+action]
			require.Truef(t, ok, "the %s op must stay published on the organization/ledger-scoped contract path", action)
			require.NotNilf(t, pathItem.Post, "the %s op path must carry a POST operation", action)
			assert.Nilf(t, pathItem.Post.RequestBody,
				"the %s op stays bodiless, so it has no body scope to read", action)
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
	v2LegSchemaName        = "V2LegInput"
	v2ShareSchemaName      = "V2ShareInput"

	// v1ResponseSchemaName is the component the v1 ops answer with. The v2 one is
	// v2TransactionSchemaName, declared once in transaction_v2_output_test.go: a second
	// spelling of the same literal would let a rename be applied to one and missed in the
	// other, which is the drift both constants exist to catch.
	v1ResponseSchemaName = "Transaction"
)

// TestRegisterTransactionV2Routes_ResponseSchemaDoesNotShadowV1 pins that the v2 response
// component does not reuse the name the v1 one is published under.
//
// The two API versions are served as separate Huma instances, so each keeps its own component
// registry and both COULD publish a differently shaped schema under one name without either
// complaining. They are then joined into a single hub document for the collection, and a name
// carrying two shapes has to be disambiguated at that point — behaviour this repo has never
// exercised. Keeping the names apart means the join never has to.
func TestRegisterTransactionV2Routes_ResponseSchemaDoesNotShadowV1(t *testing.T) {
	t.Parallel()

	// Registered ONCE: the helper installs the schema namer, which is process-global Huma
	// state, so registering per subtest would have parallel subtests racing on it.
	oapi := registerIsolatedV2TransactionContractForTest()
	schemas := oapi.Components.Schemas.Map()

	require.Containsf(t, schemas, v2TransactionSchemaName,
		"v2 contract should publish its response component as %s", v2TransactionSchemaName)
	assert.NotContainsf(t, schemas, v1ResponseSchemaName,
		"the v2 contract must not publish a %s component: the v1 contract already publishes that "+
			"name with a different shape, and the hub join has no tested answer for one name carrying two",
		v1ResponseSchemaName)

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := oapi.Paths[rt.opPath]
			require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, rt.opPath)

			for status, resp := range pathItem.Post.Responses {
				if !strings.HasPrefix(status, "2") {
					continue
				}

				media, ok := resp.Content["application/json"]
				require.Truef(t, ok, "%s %s should answer application/json", rt.action, status)
				assert.Equalf(t, "#/components/schemas/"+v2TransactionSchemaName, media.Schema.Ref,
					"%s should answer with the v2 response component", rt.action)
			}
		})
	}
}

// v2ContractAssemblies are the two documents the v2 create surface reaches a client through: the
// ISOLATED bare document a test builds for component assertions (group-relative, no prefix), and
// the REAL /v2-prefixed document the unified server assembles — buildUnifiedHumaAPI
// (contract_spec_routes_test.go), the same huma.API that generates the committed dump. The property
// under test is that the body-schema publisher keys on operation ID, so it stamps the create ops
// the SAME way whether or not the document carries a /v2 prefix; the assertions therefore run over
// both, each with the prefix it keys its paths under. Reading the real document is what proves the
// property against the mount a client actually hits, not only against a fixture — a publisher keyed
// on paths would find nothing under /v2 and leave those create ops with the opaque RawBody schema.
var v2ContractAssemblies = []struct {
	name   string
	prefix string
	doc    func() *huma.OpenAPI
}{
	{
		name:   "isolated document, no prefix",
		prefix: "",
		doc:    registerIsolatedV2TransactionContractForTest,
	},
	{
		name:   "real /v2 document assembled by the unified server",
		prefix: "/v2",
		doc: func() *huma.OpenAPI {
			_, api := buildUnifiedHumaAPI()

			return api.OpenAPI()
		},
	},
}

// TestRegisterTransactionV2Routes_PublishesCreateBodySchema asserts the v2 create ops describe
// their body with a $ref to the published input component instead of the opaque `string`/`binary`
// schema Huma derives from a RawBody field, that both prose components carry their prose, and that
// the bodiless lifecycle ops stay free of a requestBody — under EITHER assembly.
//
// A group's prefix decides the document's path keys and nothing a client sees, and it is not
// readable from inside the contract, so a path key is not a handle the publisher can spell.
// Identifying the ops by operation ID is what makes the two assemblies agree; a publisher keyed on
// paths finds nothing in the prefixed one and fails silently — the ops keep the opaque RawBody
// schema, both prose descriptions go unstamped, and registration itself still succeeds.
func TestRegisterTransactionV2Routes_PublishesCreateBodySchema(t *testing.T) {
	t.Parallel()

	for _, tt := range v2ContractAssemblies {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oapi := tt.doc()
			schemas := oapi.Components.Schemas.Map()

			bodySchema, ok := schemas[v2CreateBodySchemaName]
			require.Truef(t, ok, "v2 contract should publish the create-body component %s", v2CreateBodySchemaName)
			require.NotNilf(t, bodySchema, "published %s component should not be nil", v2CreateBodySchemaName)
			assert.Equalf(t, v2CreateBodyDescription, bodySchema.Description,
				"%s should carry the create-body prose", v2CreateBodySchemaName)

			legSchema, ok := schemas[v2LegSchemaName]
			require.Truef(t, ok, "v2 contract should publish the %s component", v2LegSchemaName)
			require.NotNilf(t, legSchema, "published %s component should not be nil", v2LegSchemaName)
			assert.Equalf(t, v2LegDescription, legSchema.Description,
				"%s should carry the leg prose", v2LegSchemaName)

			for _, rt := range v2Routes {
				t.Run(rt.action, func(t *testing.T) {
					t.Parallel()

					opPath := tt.prefix + rt.opPath

					pathItem, ok := oapi.Paths[opPath]
					require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, opPath)
					require.NotNilf(t, pathItem.Post, "%s op path should carry a POST operation", rt.action)
					require.Equalf(t, rt.operationID, pathItem.Post.OperationID,
						"assembly must leave the %s operation ID alone", rt.action)

					if !rt.hasBody {
						assert.Nilf(t, pathItem.Post.RequestBody,
							"bodiless lifecycle op %s must not advertise a requestBody", rt.action)

						return
					}

					require.NotNilf(t, pathItem.Post.RequestBody, "%s op should advertise a requestBody", rt.action)

					media, ok := pathItem.Post.RequestBody.Content[v2CreateBodyContentType]
					require.Truef(t, ok, "%s op requestBody should carry an application/json media type", rt.action)
					require.NotNilf(t, media, "%s op application/json media type should not be nil", rt.action)
					require.NotNilf(t, media.Schema, "%s op application/json media type should carry a schema", rt.action)

					assert.Equalf(t, v2CreateBodySchemaRef, media.Schema.Ref,
						"%s op body schema should $ref the published v2 input component", rt.action)
					assert.Emptyf(t, media.Schema.Format,
						"%s op body schema must not stay the opaque binary RawBody schema", rt.action)
				})
			}
		})
	}
}

// v2CreateBodyRequiredFields are the fields the published create-body component must mark
// required: the two fields common to every request plus the two leg arrays, both of which are
// mandatory and non-empty now that the surface publishes only the array form.
var v2CreateBodyRequiredFields = []string{"asset", "amount", "debits", "credits"}

// v2CreateBodySideFields are the two side fields of the request body.
var v2CreateBodySideFields = []string{"debits", "credits"}

// TestRegisterTransactionV2Routes_CreateBodyDocumentsBothSides asserts the published
// create-body component marks BOTH leg arrays required, alongside the fields common to every
// request. Dropping json `omitempty` from Debits/Credits is what buys this: Huma treats a field
// without `omitempty` as required by default, and neither field carries a `required:"false"`
// override to undo that.
func TestRegisterTransactionV2Routes_CreateBodyDocumentsBothSides(t *testing.T) {
	t.Parallel()

	schema, ok := registerIsolatedV2TransactionContractForTest().Components.Schemas.Map()[v2CreateBodySchemaName]
	require.Truef(t, ok, "v2 contract should publish the create-body component %s", v2CreateBodySchemaName)
	require.NotNilf(t, schema, "published %s component should not be nil", v2CreateBodySchemaName)

	assert.ElementsMatchf(t, v2CreateBodyRequiredFields, schema.Required,
		"%s should require the common fields and both leg arrays", v2CreateBodySchemaName)
	require.NotEmptyf(t, schema.Description,
		"%s description documents the two leg arrays", v2CreateBodySchemaName)

	for _, field := range v2CreateBodySideFields {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			assert.Containsf(t, schema.Properties, field,
				"%s should document the %s side field as a property", v2CreateBodySchemaName, field)
			assert.Containsf(t, schema.Required, field,
				"%s must mark the %s side field required", v2CreateBodySchemaName, field)
			assert.Containsf(t, schema.Description, field,
				"%s description should name the %s side field", v2CreateBodySchemaName, field)
		})
	}
}

// v2CreateBodyScopeFields are the two fields an account names its scope with. The create path
// carries neither, so the published body component is the only place a client can read them off.
var v2CreateBodyScopeFields = []string{"organizationId", "ledgerId"}

// TestRegisterTransactionV2Routes_CreateBodyDocumentsTheScope asserts the published create-body
// component states where the organization and ledger are named and that all accounts must agree
// on one pair. The create endpoint names no scope, so a client that cannot read this off the
// contract has nowhere else to look; and the agreement rule has no structural expression in a
// flat schema, so prose is the only place it can be stated.
func TestRegisterTransactionV2Routes_CreateBodyDocumentsTheScope(t *testing.T) {
	t.Parallel()

	oapi := registerIsolatedV2TransactionContractForTest()

	schema, ok := oapi.Components.Schemas.Map()[v2CreateBodySchemaName]
	require.Truef(t, ok, "v2 contract should publish the create-body component %s", v2CreateBodySchemaName)
	require.NotNilf(t, schema, "published %s component should not be nil", v2CreateBodySchemaName)

	for _, field := range v2CreateBodyScopeFields {
		assert.Containsf(t, schema.Description, field,
			"%s description must name the %s field an account carries", v2CreateBodySchemaName, field)
	}

	assert.Containsf(t, schema.Description, "SAME pair",
		"%s description must state that every account names the same organization and ledger", v2CreateBodySchemaName)

	// The two fields live on the leg component, which every leg of either side shares.
	legSchema, ok := oapi.Components.Schemas.Map()[v2LegSchemaName]
	require.Truef(t, ok, "v2 contract should publish the %s component", v2LegSchemaName)
	require.NotNilf(t, legSchema, "published %s component should not be nil", v2LegSchemaName)

	for _, field := range v2CreateBodyScopeFields {
		assert.Containsf(t, legSchema.Properties, field,
			"%s must document the %s field", v2LegSchemaName, field)
	}
}

// v2CreateBodyCeilingFloor and v2CreateBodyCeilingCap bound the VALUE of v2CreateMaxBodyBytes.
// Both the op declaration and the Fiber guard read that one constant, so comparing them against it
// holds for any value it takes — a ceiling loosened to a gigabyte would travel green through the
// whole suite.
const (
	// Clear of the largest body the per-side leg cap admits — 500 legs per side at ~200 bytes
	// each is ~210 KB across both sides — so no request the field tags accept is refused on
	// length alone.
	v2CreateBodyCeilingFloor int64 = 512 << 10

	// The app-wide Fiber body limit, which the app leaves at its default. fasthttp refuses a
	// request body past it before this guard runs, so a ceiling above it would be unreachable
	// for an uncompressed body and the answer would come from a layer that carries no `code`.
	v2CreateBodyCeilingCap int64 = int64(fiber.DefaultBodyLimit)
)

// TestRegisterTransactionV2Routes_CreateOpsBoundBodyReads asserts the four body-carrying create
// ops declare a request-body byte ceiling. Huma defaults MaxBodyBytes only for ops that declare a
// typed `Body` field, and the v2 create ops carry `RawBody`, so an unstated ceiling means an
// unbounded read. The bodiless lifecycle ops advertise no body, so they declare none.
//
// The declared figure is pinned twice over. Comparing it against v2CreateMaxBodyBytes ties the
// DECLARED ceiling to the ENFORCED one, since the Fiber guard v2CreateBodyLimit reads that same
// constant — the assertion fails the moment an op declares a figure the guard does not police.
// Bounding that constant between a floor and a cap stated independently of it is what stops the
// pair from being loosened together.
func TestRegisterTransactionV2Routes_CreateOpsBoundBodyReads(t *testing.T) {
	t.Parallel()

	assert.GreaterOrEqual(t, v2CreateMaxBodyBytes, v2CreateBodyCeilingFloor,
		"the request-body ceiling must stay clear of the largest body the per-side leg cap admits")
	assert.LessOrEqual(t, v2CreateMaxBodyBytes, v2CreateBodyCeilingCap,
		"the request-body ceiling must not pass the app-wide Fiber body limit, which refuses a body first")

	paths := registerIsolatedV2TransactionContractForTest().Paths

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := paths[rt.opPath]
			require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, rt.opPath)
			require.NotNilf(t, pathItem.Post, "%s op path should carry a POST operation", rt.action)

			if !rt.hasBody {
				assert.Zerof(t, pathItem.Post.MaxBodyBytes,
					"bodiless lifecycle op %s needs no body ceiling", rt.action)

				return
			}

			assert.EqualValuesf(t, v2CreateMaxBodyBytes, pathItem.Post.MaxBodyBytes,
				"%s op must declare the same request-body ceiling the Fiber guard enforces", rt.action)
		})
	}
}

// TestV2CreateOps_OversizedBodyCarriesCanonicalCode drives a real oversized POST through the
// mounted Fiber chain and asserts the answer is a 413 carrying the canonical payload-too-large
// code, like every other v2 rejection.
//
// Asserting the registered MaxBodyBytes value proves only that the ceiling is declared. Huma
// enforces the same ceiling on its own read and answers 413 too, but without a `code` and with the
// byte figure spelled out in the detail — so both layers produce the same STATUS, and only
// inspecting the response body tells which one answered.
func TestV2CreateOps_OversizedBodyCarriesCanonicalCode(t *testing.T) {
	t.Parallel()

	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: false})

	// One byte past the declared ceiling: large enough that no layer can accept it, and the
	// payload itself stays valid JSON so nothing else can reject it first.
	oversized := oversizedV2CreateBody(v2CreateMaxBodyBytes + 1)

	for _, rt := range v2Routes {
		if !rt.hasBody {
			continue
		}

		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, concreteV2Path(rt.fiberPath), strings.NewReader(oversized))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equalf(t, fiber.StatusRequestEntityTooLarge, resp.StatusCode,
				"an oversized v2 %s body must be rejected as request-entity-too-large, got: %s", rt.action, raw)

			var body map[string]any
			require.NoErrorf(t, json.Unmarshal(raw, &body), "response must be JSON, got: %s", raw)

			assert.Equalf(t, constant.ErrPayloadTooLarge.Error(), body["code"],
				"the v2 %s oversized-body rejection must carry the canonical payload-too-large code", rt.action)

			detail, _ := body["detail"].(string)
			assert.NotContainsf(t, detail, strconv.FormatInt(v2CreateMaxBodyBytes, 10),
				"the v2 %s rejection must not publish the configured byte ceiling", rt.action)
			assert.NotContainsf(t, detail, "limit=",
				"the v2 %s rejection must not leak the internal limit phrasing", rt.action)
		})
	}
}

// TestV2CreateBodyLimit_Boundary drives the guard in isolation, with a sentinel terminal behind
// it, so both sides of the boundary are observable. The end-to-end sweep above proves the guard
// is wired onto every create route but cannot exercise the accepting side: a body that passes
// reaches the real terminal.
//
// The rejecting threshold is `>=` deliberately. Huma enforces the same ceiling on its own read
// and rejects a read that fills the limit EXACTLY, so a guard using `>` would hand the
// exactly-at-the-ceiling body to the layer that renders without a code — which is the whole
// defect being fixed.
func TestV2CreateBodyLimit_Boundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		size       int64
		wantPassed bool
	}{
		{name: "well under the ceiling", size: 1024, wantPassed: true},
		{name: "one byte under the ceiling", size: v2CreateMaxBodyBytes - 1, wantPassed: true},
		{name: "exactly at the ceiling", size: v2CreateMaxBodyBytes},
		{name: "one byte over the ceiling", size: v2CreateMaxBodyBytes + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const sentinelStatus = http.StatusTeapot

			// The terminal echoes the body length it can still read, so a guard that
			// consumed the body while measuring it would show up as a short read here
			// rather than as a mystery decode failure further in.
			var seenBySuccessor int

			app := fiber.New()
			app.Post("/probe", v2CreateBodyLimit, func(c fiber.Ctx) error {
				seenBySuccessor = len(c.Body())

				return c.SendStatus(sentinelStatus)
			})

			body := oversizedV2CreateBody(tt.size)
			require.EqualValues(t, tt.size, len(body), "the padded body must be exactly the requested size")

			req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			if tt.wantPassed {
				assert.Equal(t, sentinelStatus, resp.StatusCode,
					"a body under the ceiling must reach the terminal")
				assert.Equal(t, len(body), seenBySuccessor,
					"the guard must leave the whole body readable by the terminal behind it")

				return
			}

			assert.Equal(t, fiber.StatusRequestEntityTooLarge, resp.StatusCode,
				"a body at or past the ceiling must be rejected by the guard")

			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var problem map[string]any
			require.NoErrorf(t, json.Unmarshal(raw, &problem), "response must be JSON, got: %s", raw)

			assert.Equal(t, constant.ErrPayloadTooLarge.Error(), problem["code"],
				"the rejection must carry the canonical payload-too-large code")

			detail, _ := problem["detail"].(string)
			assert.NotEmpty(t, detail, "the rejection must carry a human-readable detail")
			assert.NotContains(t, detail, strconv.FormatInt(v2CreateMaxBodyBytes, 10),
				"the rejection must not publish the configured byte ceiling")
		})
	}
}

// TestV2CreateBodyLimit_MeasuresDecodedBody pins WHICH length the guard compares against the
// ceiling, over both regions a compressed body can decode into.
//
// A gzipped body declares a Content-Length of its compressed wire size, while Fiber's Body() —
// the same call humafiber's reader is handed — returns the DECODED bytes. A guard measuring the
// declared length would pass an over-ceiling compressed body straight to the layer that answers
// without a `code`.
//
// The second row sits above fiber.DefaultBodyLimit, where Fiber does not produce decoded bytes at
// all and Body() reports the failure in the return value instead. That value is far shorter than
// the ceiling, so a guard that measures it and nothing else reads the failure as a small body.
// Both rows must land on the canonical 413.
func TestV2CreateBodyLimit_MeasuresDecodedBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		decodedSize int64
	}{
		{
			name:        "decoded length over the ceiling",
			decodedSize: v2CreateMaxBodyBytes + 1,
		},
		{
			name:        "decoded length over Fiber's decode limit",
			decodedSize: int64(fiber.DefaultBodyLimit) + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var compressed bytes.Buffer

			zw := gzip.NewWriter(&compressed)
			_, err := zw.Write([]byte(oversizedV2CreateBody(tt.decodedSize)))
			require.NoError(t, err)
			require.NoError(t, zw.Close())

			require.Less(t, int64(compressed.Len()), v2CreateMaxBodyBytes,
				"the compressed wire size must sit UNDER the ceiling for this case to mean anything")

			app := fiber.New()
			app.Post("/probe", v2CreateBodyLimit, func(c fiber.Ctx) error {
				return c.SendStatus(http.StatusTeapot)
			})

			req := httptest.NewRequest(http.MethodPost, "/probe", bytes.NewReader(compressed.Bytes()))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			req.Header.Set(fiber.HeaderContentEncoding, "gzip")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equalf(t, fiber.StatusRequestEntityTooLarge, resp.StatusCode,
				"a compressed body whose DECODED length passes the ceiling must be rejected by the guard, got: %s", raw)

			var problem map[string]any
			require.NoErrorf(t, json.Unmarshal(raw, &problem), "response must be JSON, got: %s", raw)

			assert.Equal(t, constant.ErrPayloadTooLarge.Error(), problem["code"],
				"the rejection must carry the canonical payload-too-large code, not Huma's uncoded 413")
		})
	}
}

// oversizedV2CreateBody spells a syntactically valid v2 create body padded to exactly size
// bytes with a single description field, so the only thing a rejection can be about is length.
func oversizedV2CreateBody(size int64) string {
	const prefix = `{"asset":"BRL","amount":"100",` +
		`"debits":[{"alias":"@a",` + v2ScopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@b",` + v2ScopeJSON + `,"amount":"100"}],"description":"`
	const suffix = `"}`

	padding := size - int64(len(prefix)) - int64(len(suffix))

	return prefix + strings.Repeat("x", int(padding)) + suffix
}

// v2ShareBounds is the bound pair each share factor must publish. The figures are stated as
// literals rather than read back from the struct tags, because the tags are what generate the
// schema — comparing the two would hold for any value, including none. A bound dropped from a tag
// has to surface here as a contract change.
var v2ShareBounds = map[string]struct{ min, max float64 }{
	"percentage":             {min: 1, max: 100},
	"percentageOfPercentage": {min: 0, max: 100},
}

// TestRegisterTransactionV2Routes_ShareComponentPublishesBounds locks the numeric bounds of the two
// share factors in the published contract. The create ops decode the body imperatively, so the
// `minimum`/`maximum` tags enforce nothing — the schema is the only place a client can read a bound
// instead of discovering it by rejection, which makes its disappearance invisible to every other
// test in this suite.
//
// The upper bounds are asserted EQUAL to each other as well as to their figures: the two factors
// multiply into one product, so a cap on only one of them decides acceptance by which factor the
// client happened to put the larger number in.
func TestRegisterTransactionV2Routes_ShareComponentPublishesBounds(t *testing.T) {
	t.Parallel()

	schema, ok := registerIsolatedV2TransactionContractForTest().Components.Schemas.Map()[v2ShareSchemaName]
	require.Truef(t, ok, "v2 contract should publish the share component %s", v2ShareSchemaName)
	require.NotNilf(t, schema, "published %s component should not be nil", v2ShareSchemaName)

	assert.Equal(t, v2ShareBounds["percentage"].max, v2ShareBounds["percentageOfPercentage"].max,
		"both share factors must carry the SAME upper bound")

	for field, want := range v2ShareBounds {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			property, ok := schema.Properties[field]
			require.Truef(t, ok, "%s should document the %s factor", v2ShareSchemaName, field)
			require.NotNilf(t, property, "the %s property should not be nil", field)

			require.NotNilf(t, property.Minimum,
				"%s must publish its lower bound, or a client meets it only as a rejection", field)
			assert.Equalf(t, want.min, *property.Minimum, "%s lower bound", field)

			require.NotNilf(t, property.Maximum,
				"%s must publish its upper bound, or a client meets it only as a rejection", field)
			assert.Equalf(t, want.max, *property.Maximum, "%s upper bound", field)
		})
	}
}

// TestRegisterTransactionV2Routes_LegComponentDescribesValueExpressions asserts the published
// leg component carries prose for the rule the schema cannot express structurally: a leg fills
// exactly one of `amount` or `share`. The rule is enforced at runtime, so a contract that omits
// it leaves clients to discover it by getting rejected.
func TestRegisterTransactionV2Routes_LegComponentDescribesValueExpressions(t *testing.T) {
	t.Parallel()

	schema, ok := registerIsolatedV2TransactionContractForTest().Components.Schemas.Map()[v2LegSchemaName]
	require.Truef(t, ok, "v2 contract should publish the leg component %s", v2LegSchemaName)
	require.NotNilf(t, schema, "published %s component should not be nil", v2LegSchemaName)

	require.NotEmptyf(t, schema.Description,
		"%s is the only place the one-value-expression-per-leg rule can be stated", v2LegSchemaName)

	for _, expression := range []string{"amount", "share"} {
		assert.Containsf(t, schema.Description, expression,
			"%s description should name the %s value expression", v2LegSchemaName, expression)
	}

	assert.NotContainsf(t, schema.Properties, "remaining",
		"%s must not publish a remaining expression: a remaining leg commits an unbalanced transaction", v2LegSchemaName)
}

// TestRegisterTransactionV2Routes_ComponentRequiredFields locks the `required` list of every
// published v2 request component.
func TestRegisterTransactionV2Routes_ComponentRequiredFields(t *testing.T) {
	t.Parallel()

	schemas := registerIsolatedV2TransactionContractForTest().Components.Schemas.Map()

	tests := []struct {
		component string
		want      []string
	}{
		{component: v2CreateBodySchemaName, want: v2CreateBodyRequiredFields},
		{component: v2LegSchemaName, want: []string{"alias", "organizationId", "ledgerId"}},
		{component: v2ShareSchemaName, want: []string{"percentage"}},
	}

	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			t.Parallel()

			schema, ok := schemas[tt.component]
			require.Truef(t, ok, "v2 contract should publish the %s component", tt.component)
			require.NotNilf(t, schema, "published %s component should not be nil", tt.component)

			assert.ElementsMatchf(t, tt.want, schema.Required, "%s required list", tt.component)
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
// API, a document missing its component registry, a document carrying no create ops, a path item
// whose POST is nil, a create op whose body is not JSON, and a document carrying only SOME of the
// create ops. None of them may publish the component either — it is registered only when every
// create action has been found, so a partial match answers like the empty one instead of leaving
// three of four ops typed and the shortfall invisible.
//
// The documents here are SYNTHETIC on purpose: each hand-builds a degenerate shape the real
// mounted surface never produces (nil api, dropped registry, a non-JSON body, a partial op set),
// which is the only way to reach the publisher's guard branches. Do NOT repoint these at the real
// buildUnifiedHumaAPI document by analogy with the path-key tests — a well-formed document cannot
// exercise a single case below.
func TestPublishV2CreateBodySchema_DegradesToNoOp(t *testing.T) {
	t.Parallel()

	const createPath = "/transactions/direct"

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
			name: "no create op registered",
			api:  newV2DocForTest,
		},
		{
			name: "path item without a post operation",
			api: func() huma.API {
				api := newV2DocForTest()
				api.OpenAPI().Paths = map[string]*huma.PathItem{createPath: {}}

				return api
			},
		},
		{
			name: "create op body is not json",
			api: func() huma.API {
				api := newV2DocForTest()
				api.OpenAPI().Paths = map[string]*huma.PathItem{
					createPath: {Post: &huma.Operation{
						OperationID: v2CreateActions[0].operationID,
						RequestBody: &huma.RequestBody{
							Content: map[string]*huma.MediaType{"text/plain": {}},
						},
					}},
				}

				return api
			},
		},
		{
			// All the create ops but one. A publisher that typed whatever it happened to find
			// would report success here while one op still advertised an opaque byte stream.
			name: "not every create op registered",
			api: func() huma.API {
				api := newV2DocForTest()

				paths := make(map[string]*huma.PathItem, len(v2CreateActions)-1)
				for _, action := range v2CreateActions[1:] {
					paths[v2CreateBasePath+action.suffix] = postWithJSONBodyForTest(action.operationID, v2UnrewrittenBodyRef)
				}

				api.OpenAPI().Paths = paths

				return api
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := tc.api()

			require.NotPanics(t, func() { publishV2CreateBodySchema(api) })

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

// v2ForeignOperationID is a v1 create operation ID, standing in for an op the publisher must leave
// alone. It is checked against the committed v1 dump rather than only spelled here: renaming the v1
// op would otherwise leave this test asserting over an ID no contract carries, still green while it
// no longer represents the collision it is named for.
const v2ForeignOperationID = "createTransactionJSON"

// v2UnrewrittenBodyRef is the body schema every op in the fixture below starts out with. It names a
// component neither contract publishes, so "left alone" and "rewritten" cannot be confused: a
// skipped op still carries this ref, a rewritten one carries v2CreateBodySchemaRef.
const v2UnrewrittenBodyRef = "#/components/schemas/UnrewrittenRequestBody"

// postWithJSONBodyForTest spells one path item: a POST advertising a JSON request body under ref.
// The media type is the one the publisher scans for, so a fixture cannot file its body under a key
// the publisher never reads.
func postWithJSONBodyForTest(operationID, ref string) *huma.PathItem {
	return &huma.PathItem{Post: &huma.Operation{
		OperationID: operationID,
		RequestBody: &huma.RequestBody{
			Content: map[string]*huma.MediaType{
				v2CreateBodyContentType: {Schema: &huma.Schema{Ref: ref}},
			},
		},
	}}
}

// TestPublishV2CreateBodySchema_LeavesForeignOpsAlone asserts the operation ID is the ONLY thing
// separating a v2 create op from any other op carrying a JSON body: the publisher walks every path
// in the document, so a v1 create op sitting alongside the v2 ones comes through untouched while
// the v2 ops take the published component. Both outcomes are read off ONE run of the publisher, so
// the discriminator is stated as a single property rather than as two separate tests.
//
// The document here is SYNTHETIC on purpose: it seats a foreign op carrying a sentinel body ref
// beside the v2 ops so "left alone" and "rewritten" are two distinguishable refs. The real mounted
// surface never puts a v1 op on the v2 document, so it cannot stage this collision — do NOT repoint
// this at buildUnifiedHumaAPI by analogy with the path-key tests.
func TestPublishV2CreateBodySchema_LeavesForeignOpsAlone(t *testing.T) {
	t.Parallel()

	const foreignPath = "/transactions/json"

	require.Containsf(t, collectSpecOperationIDs(t, specPath), v2ForeignOperationID,
		"%s must be a real v1 operation ID, or the foreign op is fictional and covers no collision",
		v2ForeignOperationID)

	// The two refs are the whole discriminator: were they equal, both rows below would expect
	// the ref every op ends up with and neither outcome would say anything.
	require.NotEqualf(t, v2CreateBodySchemaRef, v2UnrewrittenBodyRef,
		"the unrewritten sentinel ref must differ from the published ref %s, or left-alone and "+
			"rewritten are the same outcome", v2CreateBodySchemaRef)

	api := newV2DocForTest()

	paths := map[string]*huma.PathItem{
		foreignPath: postWithJSONBodyForTest(v2ForeignOperationID, v2UnrewrittenBodyRef),
	}

	// EVERY create action is present: the publisher rewrites only once it has found them all, so a
	// fixture carrying fewer would exercise the no-op path instead of the discriminator.
	for _, action := range v2CreateActions {
		paths[v2CreateBasePath+action.suffix] = postWithJSONBodyForTest(action.operationID, v2UnrewrittenBodyRef)
	}

	api.OpenAPI().Paths = paths

	publishV2CreateBodySchema(api)

	require.Containsf(t, api.OpenAPI().Components.Schemas.Map(), v2CreateBodySchemaName,
		"a document carrying every create action should publish the %s component", v2CreateBodySchemaName)

	tests := []struct {
		name    string
		opPath  string
		wantRef string
	}{
		{
			name:    "op outside v2CreateActions keeps its own body schema",
			opPath:  foreignPath,
			wantRef: v2UnrewrittenBodyRef,
		},
		{
			name:    "create op takes the published v2 input component",
			opPath:  v2CreateBasePath + v2CreateActions[0].suffix,
			wantRef: v2CreateBodySchemaRef,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := api.OpenAPI().Paths[tt.opPath]
			require.Truef(t, ok, "the document should still carry the op path %q", tt.opPath)

			media, ok := pathItem.Post.RequestBody.Content[v2CreateBodyContentType]
			require.Truef(t, ok, "%s should keep its application/json media type", tt.opPath)
			require.NotNilf(t, media.Schema, "%s should keep a body schema", tt.opPath)

			assert.Equalf(t, tt.wantRef, media.Schema.Ref, "the body schema of %s", tt.opPath)
		})
	}
}
