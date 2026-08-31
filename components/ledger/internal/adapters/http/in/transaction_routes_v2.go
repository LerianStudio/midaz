// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"reflect"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction contract seam (filename-suffix
// versioning — v1 files are left untouched). It registers the v2 `direct`, `hold`,
// `block`, `unblock`, `commit`, `cancel`, and `revert` transaction ops onto the /v2
// version group of the shared Huma contract and attaches
// the SAME Fiber auth chain the v1 transaction ops carry (protectedMidaz,
// authz namespace "midaz", (resource, verb) = ("transactions","post")). No new
// policy is introduced: authorization is per-tenant, identical to v1 — the tuple names
// no organization, so it is unaffected by which paths carry one.
//
// The CREATE terminals (CreateTransactionDirectV2, CreateTransactionHoldV2,
// CreateTransactionBlockV2, CreateTransactionUnblockV2) live in
// transaction_handler_v2.go: they decode the flat v2 body, translate it, and enter
// the v1 createTransaction funnel (hold with pending=true) under the scope the body
// resolved. They therefore hang off a path that names no organization and no ledger.
// The LIFECYCLE terminals (commit/cancel/revert) address an EXISTING transaction and
// carry no body, so their scope can only come from the URL: they stay under the
// organization/ledger prefix. They are thin v2-specific shells
// (CommitTransactionV2 / CancelTransactionV2 / RevertTransactionV2, also in
// transaction_handler_v2.go) over the SAME transport-neutral core the v1 shells in
// transaction_handler_huma.go call (commitTransaction / revertTransaction) — the only
// difference is the response envelope, which answers the /v2 wire shape (TransactionV2,
// `debit`/`credit`) instead of the canonical transaction.Transaction. Their path params
// follow the asset/CRM Huma convention — plain strings with only `doc:` (no format:uuid
// tag) so ParseUUIDPathParameters stays the sole path-UUID validator on the Fiber chain,
// not a native Huma 422.

// RegisterTransactionV2Routes registers the v2 transaction ops on the /v2 version
// group of the shared Huma API. It registers the create ops `direct`, `hold`, `block`, and `unblock` on the
// scope-free create path, plus the bodiless lifecycle ops `commit`, `cancel`, and `revert`
// (by organization, ledger and transaction_id).
// The lifecycle ops are thin v2 shells over the SAME transport-neutral core the v1 shells
// call — no idempotency HEADERS, since they carry no body or headers. Auth is the Fiber
// guard chain attached in RegisterTransactionV2RoutesToApp BEFORE this terminal, not here —
// the per-op Security metadata is SPEC-ONLY. Every path is declared GROUP-RELATIVE: it names
// no /v2 segment. Once every op is registered, publishV2CreateBodySchema gives the create ops
// a typed request-body schema.
func RegisterTransactionV2Routes(api huma.API, h *TransactionHandler) {
	const transactionsIDBasePath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}"

	// Shared OpenAPI tag for every v2 transaction op, mirroring the v1 sibling's
	// `const tag` in transaction_handler_huma.go.
	const transactionsTag = "Transactions"

	for _, action := range v2CreateActions {
		huma.Register(api, huma.Operation{
			OperationID:      action.operationID,
			Method:           http.MethodPost,
			Path:             v2CreateBasePath + action.suffix,
			Summary:          action.summary,
			Tags:             []string{transactionsTag},
			Security:         secTransactionBearer,
			SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
			MaxBodyBytes:     v2CreateMaxBodyBytes,
			DefaultStatus:    http.StatusCreated,
		}, action.terminal(h))
	}

	huma.Register(api, huma.Operation{
		OperationID:   "commitTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/commit",
		Summary:       "Commit a Transaction (v2)",
		Tags:          []string{transactionsTag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.CommitTransactionV2)

	huma.Register(api, huma.Operation{
		OperationID:   "cancelTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/cancel",
		Summary:       "Cancel a pending Transaction (v2)",
		Tags:          []string{transactionsTag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.CancelTransactionV2)

	huma.Register(api, huma.Operation{
		OperationID:   "revertTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/revert",
		Summary:       "Revert a Transaction (v2)",
		Tags:          []string{transactionsTag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.RevertTransactionV2)

	publishV2CreateBodySchema(api)
}

// v2CreateBasePath is the collection the v2 create actions hang off, group-relative to /v2.
// It names no organization and no ledger, so the path each op registers with Huma and the path
// the Fiber guard chain mounts are spelled from HERE — the two sides of the create surface have
// one spelling between them, not two that have to be kept equal.
const v2CreateBasePath = "/transactions"

// v2CreateBodyContentType is the media type the v2 create ops accept, matching the
// `contentType` tag on CreateTransactionInputV2.RawBody — the key Huma files the
// request body under.
const v2CreateBodyContentType = "application/json"

// v2CreateTerminal is the shape every v2 create terminal shares. All four actions accept the
// same request envelope and answer with the same success envelope; only the identity they pass
// to createTransactionV2 differs.
type v2CreateTerminal func(context.Context, *CreateTransactionInputV2) (*CreateTransactionOutputV2, error)

// v2CreateAction is one v2 create action: the suffix it hangs off v2CreateBasePath plus the
// identity the published contract gives it. It exists so BOTH sides of the create surface walk
// one list — the Huma contract to publish the op, the Fiber registrar to mount the guard chain —
// which is what makes it impossible to publish an action at a path the chain does not guard.
//
// terminal is a function of the handler rather than a bound method so the list can stay a
// package-level value.
type v2CreateAction struct {
	suffix      string
	operationID string
	summary     string
	terminal    func(*TransactionHandler) v2CreateTerminal
}

// v2CreateActions are the v2 create ops, the ones that carry a body. The lifecycle ops
// (commit/cancel/revert) are absent because they are bodiless: they have no request body to
// describe and no body scope to read.
var v2CreateActions = []v2CreateAction{
	{
		suffix:      "/direct",
		operationID: "createTransactionDirectV2",
		summary:     "Create a Transaction using the v2 direct model",
		terminal:    func(h *TransactionHandler) v2CreateTerminal { return h.CreateTransactionDirectV2 },
	},
	{
		suffix:      "/hold",
		operationID: "createTransactionHoldV2",
		summary:     "Create a Transaction using the v2 hold model",
		terminal:    func(h *TransactionHandler) v2CreateTerminal { return h.CreateTransactionHoldV2 },
	},
	{
		suffix:      "/block",
		operationID: "createTransactionBlockV2",
		summary:     "Create a Transaction using the v2 block model",
		terminal:    func(h *TransactionHandler) v2CreateTerminal { return h.CreateTransactionBlockV2 },
	},
	{
		suffix:      "/unblock",
		operationID: "createTransactionUnblockV2",
		summary:     "Create a Transaction using the v2 unblock model",
		terminal:    func(h *TransactionHandler) v2CreateTerminal { return h.CreateTransactionUnblockV2 },
	},
}

// v2CreateMaxBodyBytes is the request-body ceiling of the v2 create ops. It is stated here
// because Huma defaults MaxBodyBytes only for ops that declare a typed Body field; the v2 ops
// carry RawBody, which leaves the read unbounded.
//
// 1 MiB leaves generous headroom over the largest body the per-side leg cap admits: 500 legs per
// side at ~200 bytes each is ~210 KB across both sides. Metadata is not part of that figure —
// `keymax=100` and `valuemax=2000` bound the LENGTH of a key and a value, and nothing bounds how
// many keys a metadata object carries, so metadata alone can fill whatever the byte ceiling
// leaves. That is precisely what this ceiling is for: it is the only bound on the parts of the
// body the field-level tags leave open, metadata key count and the unbounded individual fields
// such as an account alias among them.
//
// v2CreateBodyLimit enforces it on the Fiber chain; the ops declare the same value so Huma bounds
// its own read too.
const v2CreateMaxBodyBytes int64 = 1 << 20

// v2CreateBodyDescription is the prose the published create-body component carries. The scope
// rule is stated here because the create endpoint names no organization and no ledger, so this
// component is the only place a client can learn where to state it.
const v2CreateBodyDescription = "Transaction request body. `debits` and `credits` are the two " +
	"required, non-empty leg arrays of the transaction; one debit paired with many credits, or " +
	"the reverse, is a valid request. Every leg names the `organizationId` and `ledgerId` its " +
	"account belongs to; all of them must name the SAME pair, and that pair is the organization " +
	"and ledger the transaction is created in. A request whose accounts name different pairs is " +
	"rejected. `asset`, `amount`, `description`, `code`, `routeId`, `operationRouteId` and " +
	"`metadata` sit alongside the two leg arrays, and `amount` is the transaction total that " +
	"the legs' `share` expressions divide. Each leg array holds at most 500 legs."

// v2LegDescription is the prose the published leg component carries. Like the parent
// component, the leg stays one flat object, so the "exactly one value expression" rule has no
// structural expression in the schema and has to be stated here.
const v2LegDescription = "One leg of a transaction side. Fill EXACTLY ONE value expression " +
	"per leg: `amount` for an explicit value, or `share` for a percentage of the transaction " +
	"total. A leg carrying both, or neither, is rejected."

// publishV2CreateBodySchema replaces the opaque request-body schema of the v2 create ops
// with a $ref to the typed v2 input component, so the contract documents the accepted
// fields instead of an unstructured byte stream. It then stamps the request and leg
// components with the prose rules the flat schemas cannot express structurally.
//
// It must run AFTER every huma.Register in this function, because registration is what
// creates op.RequestBody.
//
// The create ops are identified by OPERATION ID, over a scan of the whole document. A path key
// carries whatever prefix the API was assembled with and is not readable back off the API value, so
// a path key is not something this function can spell; an operation ID does not depend on the
// prefix. Scanning every path is safe because of two invariants: an operation ID names at most one
// operation within a document, and the v1 and v2 ID sets are held disjoint across the two published
// contracts by the contract tests — so no other op can answer to a v2 create ID.
//
// Rewriting media.Schema changes DOCUMENTATION only: the create ops declare SkipValidateBody
// with a RawBody field, so Huma validates nothing against the schema this publishes and the
// request body is decoded imperatively either way.
//
// Nil-guards the document so a spec-disabled build degrades to a no-op instead of panicking, and
// the rewrite is ALL-OR-NOTHING: it runs only once the scan has found the JSON body of EVERY v2
// create action. Typing three of four ops would publish a contract that reads as correct while one
// op still advertises an opaque byte stream — a partial match that is easy to miss, which is the
// silent half-failure identifying ops by ID exists to rule out. Publishing nothing instead surfaces
// the same defect as a uniform regression across every create op and both prose components. The ops
// are registered by walking the same list this scan matches against, so a partial match means
// registration and this scan have fallen out of step.
func publishV2CreateBodySchema(api huma.API) {
	if api == nil {
		return
	}

	oapi := api.OpenAPI()
	if oapi == nil || oapi.Components == nil || oapi.Components.Schemas == nil {
		return
	}

	createOperationIDs := make(map[string]struct{}, len(v2CreateActions))
	for _, action := range v2CreateActions {
		createOperationIDs[action.operationID] = struct{}{}
	}

	// Keyed by operation ID so the count is one entry per create ACTION: a document that filed
	// the same op under two path keys cannot inflate it into a full match.
	createBodies := make(map[string]*huma.MediaType, len(v2CreateActions))

	for _, pathItem := range oapi.Paths {
		if pathItem.Post == nil || pathItem.Post.RequestBody == nil {
			continue
		}

		if _, ok := createOperationIDs[pathItem.Post.OperationID]; !ok {
			continue
		}

		if media, ok := pathItem.Post.RequestBody.Content[v2CreateBodyContentType]; ok {
			createBodies[pathItem.Post.OperationID] = media
		}
	}

	if len(createBodies) != len(v2CreateActions) {
		return
	}

	inputType := reflect.TypeFor[mtransaction.CreateTransactionV2Input]()

	// Registering is idempotent for a given type; each call hands back a fresh $ref
	// so the ops never share one schema value. Every one of them names the same component, so
	// the last is the ref the description is stamped through.
	var bodyRef string

	for _, media := range createBodies {
		media.Schema = oapi.Components.Schemas.Schema(inputType, true, "")
		bodyRef = media.Schema.Ref
	}

	describeV2Component(oapi, bodyRef, v2CreateBodyDescription)

	legRef := oapi.Components.Schemas.Schema(reflect.TypeFor[mtransaction.V2LegInput](), true, "").Ref
	describeV2Component(oapi, legRef, v2LegDescription)
}

// describeV2Component stamps description onto the component that ref names, if the document
// carries one. Refs are resolved rather than assumed so a namer change surfaces as a missing
// description in the contract tests instead of a nil dereference here.
func describeV2Component(oapi *huma.OpenAPI, ref, description string) {
	if component := oapi.Components.Schemas.SchemaFromRef(ref); component != nil {
		component.Description = description
	}
}

// v2CreateBodyLimit rejects a v2 create request whose body reaches v2CreateMaxBodyBytes, so the
// oversized-body answer carries the canonical payload-too-large code like every other v2
// rejection. It sits ahead of the Huma terminal because Huma enforces the same ceiling itself but
// answers without a `code` and with the configured byte figure spelled out in the detail.
//
// The comparison is `>=` to match Huma's own boundary, where a read that fills the limit exactly
// is already rejected.
//
// The value measured is the DECODED body length, which is what Huma measures too. A declared
// Content-Length is the compressed wire size instead, so measuring that would let a compressed
// body over the ceiling reach the layer that renders without a `code`.
//
// A decoded length is only available while Fiber can decode. Fiber bounds decompression by the
// app-wide fiber.Config BodyLimit — 4 MiB here, since the app leaves it unset — and reports a body
// past it by setting the response status itself and returning the failure text in place of the
// body. That text is far shorter than this ceiling, so the status has to be consulted as well as
// the length; measuring the length alone reads an undecodable body as a small one and forwards it.
// TestV2CreateBodyLimit_MeasuresDecodedBody pins both regions.
//
// The 413 is rendered from PayloadTooLargeError rather than the shared registry entry for the
// code, whose message names a byte figure belonging to a different endpoint.
func v2CreateBodyLimit(c fiber.Ctx) error {
	size := len(c.Body())

	switch status := c.Response().StatusCode(); {
	case status == http.StatusRequestEntityTooLarge:
		return rejectOversizedV2Body(c)
	case status >= http.StatusBadRequest:
		// An encoding Fiber refuses outright. It has already chosen the answer, and what
		// c.Body() returned is failure text, so the terminal must not be handed it as a body.
		return nil
	case int64(size) < v2CreateMaxBodyBytes:
		return c.Next()
	}

	ctx := c.Context()

	libObservability.NewLoggerFromContext(ctx).Log(
		ctx, libLog.LevelWarn,
		"Rejected an oversized transaction body",
		libLog.Int("http.request.body.size", size),
	)

	return rejectOversizedV2Body(c)
}

// rejectOversizedV2Body renders the canonical payload-too-large answer for a v2 create request.
func rejectOversizedV2Body(c fiber.Ctx) error {
	ctx := c.Context()

	libOpentelemetry.HandleSpanBusinessErrorEvent(
		trace.SpanFromContext(ctx), "request body exceeds the accepted size", constant.ErrPayloadTooLarge,
	)

	return pkgHTTP.WithError(c, pkg.PayloadTooLargeError{
		EntityType: constant.EntityTransaction,
		Code:       constant.ErrPayloadTooLarge.Error(),
		Title:      "Payload Too Large",
		Message:    "The request payload exceeds the maximum accepted size for this operation. Please reduce the payload and try again.",
	})
}

// RegisterTransactionV2RoutesToApp wires the v2 `direct`, `hold`, `block`, `unblock`,
// `commit`, `cancel`, and `revert` ops end-to-end: it attaches the Fiber auth chain —
// auth.Authorize("midaz","transactions","post") + the tenant PostAuthMiddlewares (plus
// ParseUUIDPathParameters("transaction") on the routes that carry path UUIDs) — as
// MIDDLEWARE ONLY (group-relative path, no terminal) on the /v2 GROUP, then registers the
// Huma terminals via RegisterTransactionV2Routes on the SAME group's Huma API. All ops share
// the SAME (namespace, resource, verb) tuple and the SAME tenant chain the v1 transaction
// CREATE ops carry — no new policy, authorization is per-tenant. The tuple names no
// organization and the tenant is read from the validated JWT, so neither depends on a path
// parameter.
//
// The create routes are mounted by walking v2CreateActions, the same list RegisterTransactionV2Routes
// publishes from, joined to the same v2CreateBasePath — so an action published on the contract and
// the chain guarding it are built from one spelling. TestV2CreateOps_ContractPathsSitBehindTheGuardChain
// checks that from the outside, against the live contract and the live router.
func RegisterTransactionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const transactionsIDChainPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id"

	// ParseUUIDPathParameters rides only the lifecycle routes. A create route declares no
	// path parameter, and the middleware walks the route's declared parameters — so on a
	// create it would have nothing to validate. The organization and ledger a create names
	// are body fields, validated by the input's uuid tags at the decode boundary.
	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	// The body-limit guard rides only the create routes: the lifecycle ops carry no body.
	// It sits after auth so an unauthenticated caller is answered 401 rather than being told
	// how large a body this endpoint accepts.
	for _, action := range v2CreateActions {
		routePost(group, v2CreateBasePath+action.suffix,
			protectedMidaz(auth, "transactions", "post", routeOptions, v2CreateBodyLimit))
	}

	routePost(group, transactionsIDChainPath+"/commit", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/revert", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	RegisterTransactionV2Routes(api, th)
}
