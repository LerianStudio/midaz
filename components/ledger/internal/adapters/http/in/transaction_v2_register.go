// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction contract seam (ADR-006: filename-suffix
// versioning — v1 files are left untouched). It registers the v2 `direct`
// transaction op onto the SECOND, independent Huma contract instance and attaches
// the SAME Fiber auth chain the v1 transaction ops carry (protectedMidaz,
// authz namespace "midaz", (resource, verb) = ("transactions","post")). No new
// policy is introduced: authorization is per-tenant, identical to v1.
//
// The handler is a STUB for Task 1.1.2: it returns a clean RFC 9457 501
// Not Implemented (never a panic). Task 1.3.1 replaces the body with the real
// translate + funnel logic that reuses the v1 createTransaction core. Path params
// follow the asset/CRM Huma convention — plain strings with only `doc:` (no
// format:uuid tag) so ParseUUIDPathParameters stays the sole UUID validator on the
// Fiber chain, not a native Huma 422.

// CreateTransactionDirectV2InputHuma is the v2 direct-create request envelope. The
// org/ledger path params are plain strings (validated by the ParseUUIDPathParameters
// Fiber middleware attached before this terminal). RawBody keeps the body out of
// Huma's validator so Task 1.3.1 can decode the v2 direct model imperatively.
type CreateTransactionDirectV2InputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionDirectV2OutputHuma pins the 201 create status (parity with the
// v1 create ops). The response body lands in Task 1.3.1 once the translator exists.
type CreateTransactionDirectV2OutputHuma struct {
	Status int
}

// CreateTransactionDirectV2Huma is the STUB terminal for the v2 `direct` op. It
// returns a clean RFC 9457 501 Not Implemented so the route is real and protected
// while the translate + funnel logic is pending (Task 1.3.1). It never panics.
func (handler *TransactionHandler) CreateTransactionDirectV2Huma(_ context.Context, _ *CreateTransactionDirectV2InputHuma) (*CreateTransactionDirectV2OutputHuma, error) {
	return nil, huma.Error501NotImplemented("v2 direct transaction create is not implemented yet")
}

// RegisterTransactionV2Routes registers the v2 transaction ops on the INDEPENDENT
// v2 Huma API. Task 1.1.2 registers ONLY `direct`; hold/block/commit/cancel/revert
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
		SkipValidateBody: true, // body decoded imperatively (Task 1.3.1), mirroring the v1 create ops.
		DefaultStatus:    http.StatusCreated,
	}, h.CreateTransactionDirectV2Huma)
}

// RegisterTransactionV2RoutesToApp wires the v2 `direct` op end-to-end: it attaches
// the Fiber auth chain — auth.Authorize("midaz","transactions","post") + the tenant
// PostAuthMiddlewares + ParseUUIDPathParameters("transaction") — as MIDDLEWARE ONLY
// (group-relative path, no terminal) on the /v2 GROUP, then registers the Huma
// terminal via RegisterTransactionV2Routes on the SAME group's Huma API. This is the
// SAME (namespace, resource, verb) tuple and the SAME tenant chain the v1 transaction
// CREATE ops carry — no new policy, authorization is per-tenant.
func RegisterTransactionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, th *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const directPath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/direct"

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")

	routePost(group, directPath, protectedMidaz(auth, "transactions", "post", routeOptions, parse))

	RegisterTransactionV2Routes(api, th)
}
