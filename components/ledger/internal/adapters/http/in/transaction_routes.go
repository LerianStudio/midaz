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

// RegisterTransactionRoutes registers the twelve transaction operations on the
// shared Huma API. It is the per-file seam the unified server calls; the auth
// (auth.Authorize("midaz","transactions",verb)) + tenant + ParseUUIDPathParameters
// ("transaction") chain for these routes is attached in the unified server (Fiber level)
// BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE (the /v1 prefix rides the
// OpenAPI servers entry).
func RegisterTransactionRoutes(api huma.API, h *TransactionHandler) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"
		idPath   = listPath + "/{transaction_id}"
		tag      = "Transactions"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionJSON",
		Method:           http.MethodPost,
		Path:             listPath + "/json",
		Summary:          "Create a Transaction using JSON",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively (http.DecodeAndValidate) — see file header.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionJSON)
	attachTypedRequestBody[mtransaction.CreateTransactionInput](api, "createTransactionJSON")

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionInflow",
		Method:           http.MethodPost,
		Path:             listPath + "/inflow",
		Summary:          "Create a Transaction without passing from source",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionInflow)
	attachTypedRequestBody[mtransaction.CreateTransactionInflowInput](api, "createTransactionInflow")

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionOutflow",
		Method:           http.MethodPost,
		Path:             listPath + "/outflow",
		Summary:          "Create a Transaction without passing to distribution",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionOutflow)
	attachTypedRequestBody[mtransaction.CreateTransactionOutflowInput](api, "createTransactionOutflow")

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionAnnotation",
		Method:           http.MethodPost,
		Path:             listPath + "/annotation",
		Summary:          "Create a Transaction Annotation using JSON",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionAnnotation)
	attachTypedRequestBody[mtransaction.CreateTransactionInput](api, "createTransactionAnnotation")

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionBlock",
		Method:           http.MethodPost,
		Path:             listPath + "/block",
		Summary:          "Create a Block Transaction",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionBlock)
	attachTypedRequestBody[mtransaction.CreateTransactionInput](api, "createTransactionBlock")

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionUnblock",
		Method:           http.MethodPost,
		Path:             listPath + "/unblock",
		Summary:          "Create an Unblock Transaction",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionUnblock)
	attachTypedRequestBody[mtransaction.CreateTransactionInput](api, "createTransactionUnblock")

	huma.Register(api, huma.Operation{
		OperationID: "commitTransaction",
		Method:      http.MethodPost,
		Path:        idPath + "/commit",
		Summary:     "Commit a Transaction",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
		// commit returns 201 (matching http.Created); no request body.
		DefaultStatus: http.StatusCreated,
	}, h.CommitTransaction)

	huma.Register(api, huma.Operation{
		OperationID:   "cancelTransaction",
		Method:        http.MethodPost,
		Path:          idPath + "/cancel",
		Summary:       "Cancel a pre transaction",
		Tags:          []string{tag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated,
	}, h.CancelTransaction)

	huma.Register(api, huma.Operation{
		OperationID:   "revertTransaction",
		Method:        http.MethodPost,
		Path:          idPath + "/revert",
		Summary:       "Revert a Transaction",
		Tags:          []string{tag},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated,
	}, h.RevertTransaction)

	huma.Register(api, huma.Operation{
		OperationID:      "updateTransaction",
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a Transaction",
		Tags:             []string{tag},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body validated imperatively — plain decode, not merge-patch.
	}, h.UpdateTransaction)
	attachTypedRequestBody[transaction.UpdateTransactionInput](api, "updateTransaction")

	huma.Register(api, huma.Operation{
		OperationID: "getTransaction",
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Get a Transaction by ID",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetTransaction)

	huma.Register(api, huma.Operation{
		OperationID: "getAllTransactions",
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "Get all Transactions",
		Tags:        []string{tag},
		Security:    secTransactionBearer,
	}, h.GetAllTransactions)
}

// RegisterTransactionHumaRoutesToApp wires the twelve transaction ops (six CREATE —
// json/inflow/outflow/annotation/block/unblock, three id-only STATE, one PATCH, two
// READ). Auth is auth.Authorize("midaz","transactions",verb) + tenant +
// ParseUUIDPathParameters("transaction"), attached as middleware-only on the /v1 group
// BEFORE the Huma terminals, so each op keeps its (appName, resource, verb) tuple. Paths
// are relative to the /v1 group; the Huma terminals are attached by
// RegisterTransactionRoutes.
func RegisterTransactionHumaRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const (
		listPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions"
		idPath   = listPath + "/:transaction_id"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	// Six CREATE ops — ("transactions","post").
	routePost(group, listPath+"/json", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/inflow", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/outflow", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/annotation", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/block", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, listPath+"/unblock", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	// Three STATE ops (id-only, bodiless) — ("transactions","post").
	routePost(group, idPath+"/commit", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, idPath+"/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, idPath+"/revert", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	// PATCH — ("transactions","patch").
	routePatch(group, idPath, protectedMidaz(auth, "transactions", "patch", routeOptions, parse))

	// Two READ ops — ("transactions","get").
	routeGet(group, idPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "transactions", "get", routeOptions, parse))

	RegisterTransactionRoutes(api, th)
}
