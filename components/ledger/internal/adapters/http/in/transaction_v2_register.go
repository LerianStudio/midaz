// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction contract seam (filename-suffix
// versioning — v1 files are left untouched). It registers the v2 `direct` and `hold`
// transaction ops onto the SECOND, independent Huma contract instance and attaches
// the SAME Fiber auth chain the v1 transaction ops carry (protectedMidaz,
// authz namespace "midaz", (resource, verb) = ("transactions","post")). No new
// policy is introduced: authorization is per-tenant, identical to v1.
//
// The terminals (CreateTransactionDirectV2Huma, CreateTransactionHoldV2Huma) live in
// transaction_v2_handler.go: they decode the flat v2 body, translate it, and enter
// the v1 createTransaction funnel (hold with pending=true). Path params follow the
// asset/CRM Huma convention — plain strings with only `doc:` (no format:uuid tag) so
// ParseUUIDPathParameters stays the sole path-UUID validator on the Fiber chain, not a
// native Huma 422.

// RegisterTransactionV2Routes registers the v2 transaction ops on the INDEPENDENT
// v2 Huma API. It registers `direct` and `hold`; block/commit/cancel/revert
// arrive in later phases. Auth is the Fiber guard chain attached in
// RegisterTransactionV2RoutesToApp BEFORE this terminal, not here — the per-op
// Security metadata is SPEC-ONLY. Paths are GROUP-RELATIVE (the /v2 prefix rides
// the OpenAPI servers entry).
func RegisterTransactionV2Routes(api huma.API, h *TransactionHandler) {
	const transactionsBasePath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionDirectV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/direct",
		Summary:          "Create a Transaction using the v2 direct model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionDirectV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionHoldV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/hold",
		Summary:          "Create a Transaction using the v2 hold model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionHoldV2Huma)
}

// RegisterTransactionV2RoutesToApp wires the v2 `direct` and `hold` ops end-to-end: it
// attaches the Fiber auth chain — auth.Authorize("midaz","transactions","post") + the
// tenant PostAuthMiddlewares + ParseUUIDPathParameters("transaction") — as MIDDLEWARE
// ONLY (group-relative path, no terminal) on the /v2 GROUP, then registers the Huma
// terminals via RegisterTransactionV2Routes on the SAME group's Huma API. Both ops share
// the SAME (namespace, resource, verb) tuple and the SAME tenant chain the v1 transaction
// CREATE ops carry — no new policy, authorization is per-tenant.
func RegisterTransactionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const transactionsChainPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions"

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	routePost(group, transactionsChainPath+"/direct", protectedMidaz(auth, "transactions", "post", routeOptions, parse))
	routePost(group, transactionsChainPath+"/hold", protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	RegisterTransactionV2Routes(api, th)
}
