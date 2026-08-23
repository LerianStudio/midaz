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
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file publishes the organization/ledger-scoped transaction reads and the PATCH update on the
// /v2 version group of the shared Huma contract. It is a BESPOKE registrar, distinct from
// transaction_v2_register.go: that file owns the ops that have a dedicated v2 wire shape (the
// flat-body create direct/hold/block/unblock and the commit/cancel/revert lifecycle shells). This
// file carries the THREE remaining transaction ops — the PATCH update and the two reads (get-by-id
// + list) — pointing each at its dedicated /v2 handler method (UpdateTransactionV2,
// GetTransactionV2, GetAllTransactionsV2). Those methods call the SAME query/command core
// their v1 twins call but answer with the /v2 wire shape (TransactionV2, and the TransactionV2 list
// envelope), so this registrar publishes the TransactionV2 / OperationV2 response components for
// these ops. The PATCH request body still reuses the v1 transaction.UpdateTransactionInput type.
// Each operationId is its v1 twin's id with the version suffix.
//
// The legacy-create ops (json/inflow/outflow/annotation) are served on /v1 only; the /v2
// transaction create surface is the flat-body direct/hold/block/unblock model in
// transaction_v2_register.go. block/unblock create and commit/cancel/revert lifecycle are also
// absent here: they already publish v2 operationIds via RegisterTransactionV2Routes, and
// re-registering them with the same +V2 suffix would emit a duplicate operationId — and
// huma.OpenAPI.AddOperation scans the whole document and panics on a duplicate, a boot panic.
// RegisterTransactionRoutes is left untouched for the same reason: adding a suffix there would
// re-emit those already-v2 ops.

// RegisterTransactionMirrorV2Routes registers the three transaction reads/update ops on the /v2
// version group of the shared Huma API, pointing each at its /v2 handler method so the responses
// carry the /v2 wire shape (TransactionV2). Each operationId is its v1 twin's id with
// routeOpSuffixV2 appended; the Summary strings are copied verbatim from RegisterTransactionRoutes.
// Auth is the Fiber guard chain attached in RegisterTransactionMirrorV2RoutesToApp BEFORE these
// terminals — the per-op Security metadata is SPEC-ONLY. Paths are GROUP-RELATIVE (the group's
// prefix writes the /v2 segment into each op's path).
func RegisterTransactionMirrorV2Routes(api huma.API, h *TransactionHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"
		idPath   = listPath + "/{transaction_id}"
		tag      = "Transactions"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "updateTransaction" + routeOpSuffixV2,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a Transaction",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively — plain decode, not merge-patch.
	}, h.UpdateTransactionV2)
	attachTypedRequestBody[transaction.UpdateTransactionInput](api, "updateTransaction"+routeOpSuffixV2)

	huma.Register(api, huma.Operation{
		OperationID: "getTransaction" + routeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get a Transaction by ID",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetTransactionV2)

	huma.Register(api, huma.Operation{
		OperationID: "getAllTransactions" + routeOpSuffixV2,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Transactions",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetAllTransactionsV2)
}

// RegisterTransactionMirrorV2RoutesToApp wires the three transaction reads/update ops end-to-end
// on the /v2 contract: the Fiber guard chain on the /v2 group with group-relative paths, then the
// Huma terminals on that group's Huma API. It attaches the SUBSET of the v1 transaction guard
// chain that covers these three paths — the PATCH update and the two GET reads — dropping the
// legacy-create POSTs (served on /v1 only), the block/unblock create POSTs, and the
// commit/cancel/revert lifecycle POSTs, which are guarded and published by
// RegisterTransactionV2RoutesToApp instead.
//
// The (appName, resource, verb) tuples are the v1 transaction tuples verbatim:
// ("midaz","transactions","patch") for the update and ("midaz","transactions","get") for the
// reads — no new policy surface. It is additive; /v1 keeps serving the same ops in parallel.
func RegisterTransactionMirrorV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions"
		idPath   = listPath + "/:transaction_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	// PATCH update — ("transactions","patch").
	routePatch(group, idPath, protectedMidaz(auth, "transactions", "patch", routeOptions, parse))

	// Two reads — ("transactions","get").
	routeGet(group, idPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))

	RegisterTransactionMirrorV2Routes(api, th)
}
