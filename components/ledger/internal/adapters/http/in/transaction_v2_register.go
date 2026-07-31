// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"reflect"
	"time"

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
		BodyReadTimeout:  v2CreateBodyReadTimeout,
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
		BodyReadTimeout:  v2CreateBodyReadTimeout,
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
		BodyReadTimeout:  v2CreateBodyReadTimeout,
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
		BodyReadTimeout:  v2CreateBodyReadTimeout,
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
// 1 MiB leaves generous headroom over the largest body the per-side leg cap admits: 500 legs per
// side at ~200 bytes each is ~210 KB across both sides. Metadata is not part of that figure —
// `keymax=100` and `valuemax=2000` bound the LENGTH of a key and a value, and nothing bounds how
// many keys a metadata object carries, so metadata alone can fill whatever the byte ceiling
// leaves. That is precisely what this ceiling is for: it is the only bound on the parts of the
// body the field-level tags leave open, metadata key count and the unbounded individual fields
// such as an account alias among them.
//
// v2CreateBodyLimit enforces it on the Fiber chain; the same value is declared on the Huma ops
// so the contract states it and the read is bounded there too.
const v2CreateMaxBodyBytes int64 = 1 << 20

// v2CreateBodyReadTimeout is the body-read deadline the v2 create ops declare. It is stated here
// for the same reason as v2CreateMaxBodyBytes: Huma defaults it only for ops that declare a typed
// Body field, and the v2 ops carry RawBody.
//
// It does NOT bound a stalled client under this binary's Fiber configuration. humafiber applies
// the deadline to the request connection only when the Fiber app runs with StreamRequestBody
// enabled and a tiny BodyLimit; the app sets neither, so fasthttp has already buffered the whole
// body before any handler runs and the deadline is set and cleared over a bytes.Reader that cannot
// block. What the value does is publish the intended deadline on the op, and it would take effect
// if the app were ever switched to streaming request bodies.
//
// 5 seconds matches Huma's own default for the ops it does default, which is ample for a body
// bounded at v2CreateMaxBodyBytes.
const v2CreateBodyReadTimeout = 5 * time.Second

// v2CreateBodyDescription is the prose the published create-body component carries. The
// component stays ONE flat object listing both spellings of the transaction sides, so the
// mutual exclusivity between them has no structural expression in the schema and has to be
// stated here.
const v2CreateBodyDescription = "Transaction request body. Each side of the transaction is " +
	"spelled EITHER with its scalar field (`from`, `to`) OR with its leg array (`sources`, " +
	"`destinations`) — never both on the same side, though the two sides may choose " +
	"differently. Leave the spelling you are not using OUT of the body: on `from` and `to` an " +
	"explicit `null` is rejected. `asset`, `amount`, `description`, `code`, `routeId`, " +
	"`operationRouteId` and `metadata` are common to both forms, and `amount` is always the " +
	"transaction total that the legs' `share` expressions divide. Each leg array holds at most " +
	"500 legs."

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
// rejection.
//
// It has to sit ahead of the Huma terminal because Huma enforces the same ceiling on its own read
// and answers 413 itself, but as a bare {status,title,detail} carrying no `code` and spelling the
// configured byte figure out in the detail. Rejecting here keeps both out of the response. The
// guard is attached only to the four create routes; the global Fiber body limit is a different,
// much larger ceiling that every endpoint in the binary shares.
//
// The comparison is `>=` to match Huma's own boundary, where a read that fills the limit
// exactly is already rejected. That is what leaves the Huma ceiling unreachable and therefore
// pure defense in depth.
//
// The measured value is the DECODED body length, which is what Huma measures too: its reader is
// handed whatever Fiber's Body() returns, and Body() decompresses when the request declares a
// Content-Encoding. The declared Content-Length is the compressed wire size instead, so measuring
// that would let a compressed body over the ceiling through to the layer that renders without a
// `code`.
//
// Reading the body here is safe only while the app leaves StreamRequestBody false. With streaming
// enabled, c.Body() would drain the request stream and hand the Huma terminal behind this guard a
// reader with nothing left in it.
//
// The 413 is rendered from PayloadTooLargeError rather than the shared registry entry for the
// code, whose message names a byte figure belonging to a different endpoint.
func v2CreateBodyLimit(c fiber.Ctx) error {
	size := len(c.Body())
	if int64(size) < v2CreateMaxBodyBytes {
		return c.Next()
	}

	ctx := c.Context()

	libObservability.NewLoggerFromContext(ctx).Log(
		ctx, libLog.LevelWarn,
		"Rejected an oversized transaction body",
		libLog.Int("http.request.body.size", size),
	)

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

	// The body-limit guard rides only the create routes: the lifecycle ops carry no body.
	// It sits after auth so an unauthenticated caller is answered 401 rather than being told
	// how large a body this endpoint accepts.
	for _, action := range v2CreateActionPaths {
		routePost(group, transactionsChainPath+action,
			protectedMidaz(auth, "transactions", "post", routeOptions, parse, v2CreateBodyLimit))
	}

	routePost(group, transactionsIDChainPath+"/commit", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsIDChainPath+"/revert", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	RegisterTransactionV2Routes(api, th)
}
