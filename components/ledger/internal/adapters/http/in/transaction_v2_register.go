// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"reflect"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction contract seam (filename-suffix
// versioning — v1 files are left untouched). It registers the v2 `direct`, `hold`,
// `block`, `unblock`, `commit`, `cancel`, and `revert` transaction ops onto the SECOND,
// independent Huma contract instance and attaches
// the SAME Fiber auth chain the v1 transaction ops carry (protectedMidaz,
// authz namespace "midaz", (resource, verb) = ("transactions","post")). No new
// policy is introduced: authorization is per-tenant, identical to v1.
//
// The CREATE terminals (CreateTransactionDirectV2Huma, CreateTransactionHoldV2Huma,
// CreateTransactionBlockV2Huma, CreateTransactionUnblockV2Huma) live in
// transaction_v2_handler.go: they decode the flat v2 body, translate it, and enter
// the v1 createTransaction funnel (hold with pending=true). The LIFECYCLE terminals
// (commit/cancel/revert) carry no body or headers, so instead of new v2 handlers they
// REUSE the transport-neutral v1 shells in transaction_handler_huma.go
// (CommitTransactionHuma / CancelTransactionHuma / RevertTransactionHuma) verbatim —
// the v2 surface adds only the route, not a duplicate handler. Path params follow the
// asset/CRM Huma convention — plain strings with only `doc:` (no format:uuid tag) so
// ParseUUIDPathParameters stays the sole path-UUID validator on the Fiber chain, not a
// native Huma 422.

// RegisterTransactionV2Routes registers the v2 transaction ops on the INDEPENDENT
// v2 Huma API. It registers the create ops `direct`, `hold`, `block`, and `unblock`,
// plus the bodiless lifecycle ops `commit`, `cancel`, and `revert` (by transaction_id).
// The lifecycle ops reuse the transport-neutral v1 shells
// (CommitTransactionHuma/CancelTransactionHuma/RevertTransactionHuma) verbatim — no
// v2-specific handler, and no idempotency HEADERS, since they carry no body or headers.
// Auth is the Fiber guard chain attached in RegisterTransactionV2RoutesToApp BEFORE
// this terminal, not here — the per-op Security metadata is SPEC-ONLY. Paths are
// GROUP-RELATIVE (the /v2 prefix rides the OpenAPI servers entry). Once every op is
// registered, publishV2CreateBodySchema gives the create ops a typed request-body schema.
func RegisterTransactionV2Routes(api huma.API, h *TransactionHandler) {
	const transactionsBasePath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"

	const transactionsIDBasePath = transactionsBasePath + "/{transaction_id}"

	// Shared OpenAPI tag for every v2 transaction op, mirroring the v1 sibling's
	// `const tag` in transaction_handler_huma.go.
	const transactionsTag = "Transactions"

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionDirectV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/direct",
		Summary:          "Create a Transaction using the v2 direct model",
		Tags:             []string{transactionsTag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		MaxBodyBytes:     v2CreateMaxBodyBytes,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionDirectV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionHoldV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/hold",
		Summary:          "Create a Transaction using the v2 hold model",
		Tags:             []string{transactionsTag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		MaxBodyBytes:     v2CreateMaxBodyBytes,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionHoldV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionBlockV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/block",
		Summary:          "Create a Transaction using the v2 block model",
		Tags:             []string{transactionsTag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		MaxBodyBytes:     v2CreateMaxBodyBytes,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionBlockV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionUnblockV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/unblock",
		Summary:          "Create a Transaction using the v2 unblock model",
		Tags:             []string{transactionsTag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		MaxBodyBytes:     v2CreateMaxBodyBytes,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionUnblockV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:   "commitTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/commit",
		Summary:       "Commit a Transaction (v2)",
		Tags:          []string{transactionsTag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.CommitTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "cancelTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/cancel",
		Summary:       "Cancel a pending Transaction (v2)",
		Tags:          []string{transactionsTag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.CancelTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "revertTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/revert",
		Summary:       "Revert a Transaction (v2)",
		Tags:          []string{transactionsTag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.RevertTransactionHuma)

	publishV2CreateBodySchema(api, transactionsBasePath)
}

// v2CreateBodyContentType is the media type the v2 create ops accept, matching the
// `contentType` tag on CreateTransactionV2InputHuma.RawBody — the key Huma files the
// request body under.
const v2CreateBodyContentType = "application/json"

// v2CreateActionPaths are the action suffixes of the v2 create ops, the ones that carry a
// body. The lifecycle ops (commit/cancel/revert) are absent because they are bodiless and
// have no request body to describe.
var v2CreateActionPaths = []string{"/direct", "/hold", "/block", "/unblock"}

// v2CreateMaxBodyBytes is the request-body ceiling of the v2 create ops. It is stated here
// because Huma defaults MaxBodyBytes only for ops that declare a typed Body field; the v2 ops
// carry RawBody, which leaves the read unbounded.
//
// 1 MiB is roughly 2.5x the largest body the published limits admit: 500 legs per side at
// ~200 bytes each (~210 KB across both sides) plus the metadata ceiling of 100 keys at a
// 100-char key and a 2000-char value (~210 KB). The remainder absorbs whitespace, so a
// pretty-printed body at the leg cap still fits. The leg cap, not the byte cap, is what bounds
// the funnel's per-leg work; this ceiling bounds the fields the leg cap cannot, such as an
// account alias the leg schema leaves unbounded.
const v2CreateMaxBodyBytes int64 = 1 << 20

// v2CreateBodyDescription is the prose the published create-body component carries. The
// component stays ONE flat object listing both spellings of the transaction sides, so the
// mutual exclusivity between them has no structural expression in the schema and has to be
// stated here.
const v2CreateBodyDescription = "Transaction request body. Each side of the transaction is " +
	"spelled EITHER with its scalar field (`from`, `to`) OR with its leg array (`sources`, " +
	"`destinations`) — never both on the same side, though the two sides may choose " +
	"differently. `asset`, `amount`, `description`, `code`, `routeId`, `operationRouteId` and " +
	"`metadata` are common to both forms, and `amount` is always the transaction total that " +
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
// Nil-guards the document and every op it touches so a spec-disabled build, or a create
// action that stops registering, degrades to a no-op instead of panicking.
func publishV2CreateBodySchema(api huma.API, basePath string) {
	if api == nil {
		return
	}

	oapi := api.OpenAPI()
	if oapi == nil || oapi.Components == nil || oapi.Components.Schemas == nil {
		return
	}

	inputType := reflect.TypeFor[mtransaction.CreateTransactionV2Input]()

	var bodyRef string

	for _, action := range v2CreateActionPaths {
		pathItem, ok := oapi.Paths[basePath+action]
		if !ok || pathItem.Post == nil || pathItem.Post.RequestBody == nil {
			continue
		}

		media, ok := pathItem.Post.RequestBody.Content[v2CreateBodyContentType]
		if !ok || media == nil {
			continue
		}

		// Registering is idempotent for a given type; each call hands back a fresh $ref
		// so the ops never share one schema value.
		media.Schema = oapi.Components.Schemas.Schema(inputType, true, "")
		bodyRef = media.Schema.Ref
	}

	// Empty when no create op referenced the type, in which case nothing was registered
	// and there is no component to describe.
	if bodyRef == "" {
		return
	}

	describeV2Component(oapi, bodyRef, v2CreateBodyDescription)

	legRef := oapi.Components.Schemas.Schema(reflect.TypeFor[mtransaction.V2LegInput](), true, "").Ref
	describeV2Component(oapi, legRef, v2LegDescription)
}

// describeV2Component stamps description onto the component ref names, if the document
// carries one. Refs are resolved rather than assumed so a namer change surfaces as a missing
// description in the contract tests instead of a nil dereference here.
func describeV2Component(oapi *huma.OpenAPI, ref, description string) {
	if component := oapi.Components.Schemas.SchemaFromRef(ref); component != nil {
		component.Description = description
	}
}

// RegisterTransactionV2RoutesToApp wires the v2 `direct`, `hold`, `block`, `unblock`,
// `commit`, `cancel`, and `revert` ops end-to-end: it attaches the Fiber auth chain —
// auth.Authorize("midaz","transactions","post") + the tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("transaction") — as MIDDLEWARE ONLY (group-relative path, no
// terminal) on the /v2 GROUP, then registers the Huma terminals via
// RegisterTransactionV2Routes on the SAME group's Huma API. All ops share the SAME
// (namespace, resource, verb) tuple and the SAME tenant chain the v1 transaction CREATE
// ops carry — no new policy, authorization is per-tenant.
func RegisterTransactionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const transactionsChainPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions"

	const transactionsIDChainPath = transactionsChainPath + "/:transaction_id"

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	routePost(group, transactionsChainPath+"/direct", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsChainPath+"/hold", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsChainPath+"/block", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsChainPath+"/unblock", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/commit", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/revert", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	RegisterTransactionV2Routes(api, th)
}
