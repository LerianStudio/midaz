// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file mirrors the organization/ledger-scoped v1 transaction ops onto the /v2 version group
// of the shared Huma contract. It is a BESPOKE registrar, distinct from transaction_v2_register.go:
// that file owns the ops that have a dedicated v2 wire shape (the flat-body create direct/hold/
// block/unblock and the commit/cancel/revert lifecycle shells). This file carries the SEVEN v1 ops
// that have no v2 shape of their own — the legacy-create twins (json/inflow/outflow/annotation),
// the PATCH update, and the two reads (get-by-id + list) — as STRAIGHT MIRRORS: same handler
// method, same v1 request/response types, only the operationId carries the version suffix.
//
// block/unblock create and commit/cancel/revert lifecycle are DELIBERATELY absent here: they
// already publish v2 operationIds via RegisterTransactionV2Routes. Re-registering them with the
// same +V2 suffix would emit a duplicate operationId, and huma.OpenAPI.AddOperation scans the whole
// document and panics on a duplicate — a boot panic. RegisterTransactionRoutes is left untouched
// for the same reason: adding a suffix there would re-emit those already-v2 ops.
//
// Reusing the v1 handler methods and v1 input/output types means Huma dedups every schema to the
// v1 component already registered, so this registrar mints NO new schema components. The RawBody /
// publishV2CreateBodySchema machinery is not touched: the legacy-create twins decode imperatively
// exactly as their v1 originals do.

// RegisterTransactionMirrorV2Routes registers the seven mirrored transaction ops on the /v2
// version group of the shared Huma API, reusing the v1 handler methods and v1 request/response
// types. Each operationId is its v1 twin's id with routeOpSuffixV2 appended; the Summary strings
// are copied verbatim from RegisterTransactionRoutes. Auth is the Fiber guard chain attached in
// RegisterTransactionMirrorV2RoutesToApp BEFORE these terminals — the per-op Security metadata is
// SPEC-ONLY. Paths are GROUP-RELATIVE (the group's prefix writes the /v2 segment into each op's
// path).
func RegisterTransactionMirrorV2Routes(api huma.API, h *TransactionHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"
		idPath   = listPath + "/{transaction_id}"
		tag      = "Transactions"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionJSON" + routeOpSuffixV2,
		Method:           http.MethodPost,
		Path:             listPath + "/json",
		Summary:          "Create a Transaction using JSON",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate) — reuses the v1 handler.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionJSONHuma)
	attachTypedRequestBody[mtransaction.CreateTransactionInput](api, "createTransactionJSON"+routeOpSuffixV2)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionInflow" + routeOpSuffixV2,
		Method:           http.MethodPost,
		Path:             listPath + "/inflow",
		Summary:          "Create a Transaction without passing from source",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionInflowHuma)
	attachTypedRequestBody[mtransaction.CreateTransactionInflowInput](api, "createTransactionInflow"+routeOpSuffixV2)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionOutflow" + routeOpSuffixV2,
		Method:           http.MethodPost,
		Path:             listPath + "/outflow",
		Summary:          "Create a Transaction without passing to distribution",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionOutflowHuma)
	attachTypedRequestBody[mtransaction.CreateTransactionOutflowInput](api, "createTransactionOutflow"+routeOpSuffixV2)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionAnnotation" + routeOpSuffixV2,
		Method:           http.MethodPost,
		Path:             listPath + "/annotation",
		Summary:          "Create a Transaction Annotation using JSON",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionAnnotationHuma)
	attachTypedRequestBody[mtransaction.CreateTransactionInput](api, "createTransactionAnnotation"+routeOpSuffixV2)

	huma.Register(api, huma.Operation{
		OperationID:      "updateTransaction" + routeOpSuffixV2,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a Transaction",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively — plain decode, not merge-patch.
	}, h.UpdateTransactionHuma)
	attachTypedRequestBody[transaction.UpdateTransactionInput](api, "updateTransaction"+routeOpSuffixV2)

	huma.Register(api, huma.Operation{
		OperationID: "getTransaction" + routeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get a Transaction by ID",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getAllTransactions" + routeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Transactions",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetAllTransactionsHuma)
}

// RegisterTransactionMirrorV2RoutesToApp wires the seven mirrored transaction ops end-to-end on
// the /v2 contract: the Fiber guard chain on the /v2 group with group-relative paths, then the
// Huma terminals on that group's Huma API. It attaches the SUBSET of the v1 transaction guard
// chain that covers these seven paths — the four legacy-create POSTs and the PATCH+GET reads —
// dropping the block/unblock create POSTs and the commit/cancel/revert lifecycle POSTs, which are
// guarded and published by RegisterTransactionV2RoutesToApp instead.
//
// The (appName, resource, verb) tuples are the v1 transaction tuples verbatim: ("midaz",
// "transactions","post") for the creates, ("midaz","transactions","patch") for the update, and
// ("midaz","transactions","get") for the reads — no new policy surface. It is additive; /v1 keeps
// serving the same ops in parallel.
func RegisterTransactionMirrorV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions"
		idPath   = listPath + "/:transaction_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	// Four legacy-create ops — ("transactions","post").
	routePost(group, listPath+"/json", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/inflow", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/outflow", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/annotation", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	// PATCH update — ("transactions","patch").
	routePatch(group, idPath, protectedMidaz(auth, "transactions", "patch", routeOptions, parse))

	// Two reads — ("transactions","get").
	routeGet(group, idPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))

	RegisterTransactionMirrorV2Routes(api, th)
}
