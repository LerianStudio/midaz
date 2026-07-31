// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
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
	v2LegSchemaName        = "V2LegInput"
	v2ShareSchemaName      = "V2ShareInput"
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

	// A side field submitted as an explicit null is rejected: dropping json `omitempty` makes
	// the decoder's re-marshal always emit the key, so a submitted null never matches the
	// emitted empty value. The published description is the only place a client can learn
	// that omitting the field is the way to leave a side unspelled.
	assert.Containsf(t, schema.Description, "null",
		"%s description should state that an explicit null side field is rejected", v2CreateBodySchemaName)

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

// TestRegisterTransactionV2Routes_CreateOpsBoundBodyReads asserts the four body-carrying create
// ops state BOTH bounds on their request-body read: the byte ceiling and the read deadline.
// Huma applies its defaults for either one only to ops that declare a typed `Body` field; the
// v2 create ops carry `RawBody`, so without explicit values the read is unbounded in size and
// has no deadline, and a client that opens a request and stalls holds a worker indefinitely on
// the money path. The bodiless lifecycle ops advertise no body, so they carry neither bound.
func TestRegisterTransactionV2Routes_CreateOpsBoundBodyReads(t *testing.T) {
	t.Parallel()

	paths := registerV2TransactionContractForTest().Paths

	for _, rt := range v2Routes {
		t.Run(rt.action, func(t *testing.T) {
			t.Parallel()

			pathItem, ok := paths[rt.opPath]
			require.Truef(t, ok, "v2 contract should carry the %s op path %q", rt.action, rt.opPath)
			require.NotNilf(t, pathItem.Post, "%s op path should carry a POST operation", rt.action)

			if !rt.hasBody {
				assert.Zerof(t, pathItem.Post.MaxBodyBytes,
					"bodiless lifecycle op %s needs no body ceiling", rt.action)
				assert.Zerof(t, pathItem.Post.BodyReadTimeout,
					"bodiless lifecycle op %s needs no body read deadline", rt.action)

				return
			}

			assert.EqualValuesf(t, v2CreateMaxBodyBytes, pathItem.Post.MaxBodyBytes,
				"%s op must state its request-body ceiling instead of reading an unbounded body", rt.action)
			assert.Equalf(t, v2CreateBodyReadTimeout, pathItem.Post.BodyReadTimeout,
				"%s op must state its body read deadline instead of reading with none", rt.action)
			assert.Positivef(t, pathItem.Post.BodyReadTimeout,
				"%s op body read deadline must be a real deadline, not disabled", rt.action)
		})
	}
}

// TestV2CreateOps_OversizedBodyCarriesCanonicalCode drives a real oversized POST through the
// mounted Fiber chain and asserts the answer is a 413 carrying the canonical payload-too-large
// code, like every other v2 rejection.
//
// Asserting the registered MaxBodyBytes value proves only that the ceiling is declared. Huma
// enforces the same ceiling on its own read, but raises it as an internal error that renders
// without a `code` and spells the byte figure out in the detail — so enforcement has to be
// exercised end-to-end, and the response body inspected, to know which layer answered.
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

// oversizedV2CreateBody spells a syntactically valid v2 create body padded to exactly size
// bytes with a single description field, so the only thing a rejection can be about is length.
func oversizedV2CreateBody(size int64) string {
	const prefix = `{"asset":"BRL","amount":"100","from":"@a","to":"@b","description":"`
	const suffix = `"}`

	padding := size - int64(len(prefix)) - int64(len(suffix))

	return prefix + strings.Repeat("x", int(padding)) + suffix
}

// TestRegisterTransactionV2Routes_LegComponentDescribesValueExpressions asserts the published
// leg component carries prose for the rule the schema cannot express structurally: a leg fills
// exactly one of `amount` or `share`. The rule is enforced at runtime, so a contract that omits
// it leaves clients to discover it by getting rejected.
func TestRegisterTransactionV2Routes_LegComponentDescribesValueExpressions(t *testing.T) {
	t.Parallel()

	schema, ok := registerV2TransactionContractForTest().Components.Schemas.Map()[v2LegSchemaName]
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
// published v2 request component. The scalar side fields must stay out of the parent's list
// even though they carry no json `omitempty` — that is what their explicit `required:"false"`
// tags buy, and without them dropping `omitempty` would declare one spelling mandatory.
func TestRegisterTransactionV2Routes_ComponentRequiredFields(t *testing.T) {
	t.Parallel()

	schemas := registerV2TransactionContractForTest().Components.Schemas.Map()

	tests := []struct {
		component string
		want      []string
	}{
		{component: v2CreateBodySchemaName, want: []string{"asset", "amount"}},
		{component: v2LegSchemaName, want: []string{"account"}},
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
