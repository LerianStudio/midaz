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
// (commit/cancel/revert) carry no body or headers, so instead of new
// v2 handlers they REUSE the transport-neutral v1 shells in transaction_handler_huma.go
// (CommitTransactionHuma / CancelTransactionHuma / RevertTransactionHuma) verbatim — the v2 surface adds only the
// route, not a duplicate handler. Path params follow the asset/CRM Huma convention — plain
// strings with only `doc:` (no format:uuid tag) so ParseUUIDPathParameters stays the sole
// path-UUID validator on the Fiber chain, not a native Huma 422.

// RegisterTransactionV2Routes registers the v2 transaction ops on the INDEPENDENT
// v2 Huma API. It registers the create ops `direct`, `hold`, `block`, and `unblock`,
// plus the bodiless lifecycle ops `commit`, `cancel`, and `revert` (by transaction_id).
// The lifecycle ops reuse the transport-neutral v1 shells
// (CommitTransactionHuma/CancelTransactionHuma/RevertTransactionHuma) verbatim — no v2-specific handler and
// no idempotency, since they carry no body or headers. Auth is the Fiber guard chain
// attached in RegisterTransactionV2RoutesToApp BEFORE this terminal, not here — the
// per-op Security metadata is SPEC-ONLY. Paths are GROUP-RELATIVE (the /v2 prefix
// rides the OpenAPI servers entry).
func RegisterTransactionV2Routes(api huma.API, h *TransactionHandler) {
	const transactionsBasePath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"

	const transactionsIDBasePath = transactionsBasePath + "/{transaction_id}"

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

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionBlockV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/block",
		Summary:          "Create a Transaction using the v2 block model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionBlockV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:      "createTransactionUnblockV2",
		Method:           http.MethodPost,
		Path:             transactionsBasePath + "/unblock",
		Summary:          "Create a Transaction using the v2 unblock model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true, // body decoded imperatively (http.DecodeAndValidate), mirroring the v1 create ops.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionUnblockV2Huma)

	huma.Register(api, huma.Operation{
		OperationID:   "commitTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/commit",
		Summary:       "Commit a Transaction (v2)",
		Tags:          []string{"Transactions"},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.CommitTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "cancelTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/cancel",
		Summary:       "Cancel a pending Transaction (v2)",
		Tags:          []string{"Transactions"},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.CancelTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID:   "revertTransactionV2",
		Method:        http.MethodPost,
		Path:          transactionsIDBasePath + "/revert",
		Summary:       "Revert a Transaction (v2)",
		Tags:          []string{"Transactions"},
		Security:      secTransactionBearer,
		DefaultStatus: http.StatusCreated, // bodiless lifecycle op — no SkipValidateBody, mirroring v1.
	}, h.RevertTransactionHuma)
}

// RegisterTransactionV2RoutesToApp wires the v2 `direct`, `hold`, `block`, `unblock`,
// `commit`, `cancel`, and `revert` ops end-to-end: it attaches the Fiber auth chain — auth.Authorize("midaz","transactions","post")
// + the tenant PostAuthMiddlewares + ParseUUIDPathParameters("transaction") — as MIDDLEWARE
// ONLY (group-relative path, no terminal) on the /v2 GROUP, then registers the Huma
// terminals via RegisterTransactionV2Routes on the SAME group's Huma API. All ops share
// the SAME (namespace, resource, verb) tuple and the SAME tenant chain the v1 transaction
// CREATE ops carry — no new policy, authorization is per-tenant.
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
