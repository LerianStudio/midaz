// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the composition resource (the single
// holder-account orchestration route). It mirrors the asset exemplar
// (asset_handler_huma.go); see that file's header for the full conventions.
// Composition-specific notes:
//
//  1. AUTH is appName "midaz" (composition_routes.go midazName, resource "accounts",
//     verb "post"): a tenant that can already open accounts uses composition with no
//     new RBAC grant. The swaggo @Security on the Fiber wrapper is BearerAuth ONLY,
//     so the per-op Security metadata here is Bearer-only too — SPEC metadata only;
//     runtime auth stays the Fiber guard chain (auth.Authorize("midaz","accounts",
//     "post") + tenant + ParseUUIDPathParameters("holder")) attached BEFORE the Huma
//     terminal, NOT a Huma Security scheme.
//  2. Path is THREE-LEVEL with the holder as :id (org/ledger/holder). The shells
//     resolve org+ledger via the shared parseOrgLedger and the holder via parsePathUUID.
//     ParseUUIDPathParameters("holder") is the sole UUID validator (no format tag).
//  3. POST carries a body validated imperatively (RawBody + SkipValidateBody ->
//     http.DecodeAndValidate), never a native Huma 422. The Authorization header is
//     forwarded to the composed account-create use case verbatim (the account-create
//     is an inherited use case that authenticates against the ledger for its writes).
//  4. Success is 201 with the composite HolderAccountResponse. A partial failure
//     (account committed, instrument write failed) is NOT an error: the service
//     returns a 201 body carrying a typed instrumentError block and nil error, so it
//     rides the success path here unchanged. Errors go through pkgHTTP.HumaProblem.

// secCompositionBearer advertises that the composition operation accepts a JWT
// bearer token (Bearer-only, matching the Fiber swaggo @Security BearerAuth). SPEC
// metadata only; runtime auth is the Fiber guard chain.
var secCompositionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /holders/{id}/accounts ----------------------------------------------

// CreateHolderAccountInputHuma is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator; the org/ledger/holder path params are validated
// by the Fiber ParseUUIDPathParameters middleware, not by a format tag. The
// Authorization header is forwarded to the composed account-create use case.
type CreateHolderAccountInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Holder ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token (forwarded to the composed account-create use case)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateHolderAccountOutputHuma pins 201 (matching http.Created) and carries the
// composite response verbatim.
type CreateHolderAccountOutputHuma struct {
	Status int
	Body   *mmodel.HolderAccountResponse
}

// CreateHolderAccountHuma decodes+validates the raw body imperatively then delegates
// to the shared createHolderAccount core.
func (handler *CompositionHandler) CreateHolderAccountHuma(ctx context.Context, in *CreateHolderAccountInputHuma) (*CreateHolderAccountOutputHuma, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	holderID, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateHolderAccountInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	out, err := handler.createHolderAccount(ctx, orgID, ledgerID, holderID, payload, in.Authorization)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateHolderAccountOutputHuma{Status: http.StatusCreated, Body: out}, nil
}

// RegisterCompositionRoutes registers the single migrated composition operation on
// the shared Huma API. It is the per-file seam the unified server calls; the auth
// ("midaz","accounts","post") + tenant + ParseUUIDPathParameters("holder") middleware
// chain is attached on the /v1 group (Fiber-level) BEFORE the Huma terminal, not
// here. Path is GROUP-RELATIVE (see asset_handler_huma.go's RegisterAssetRoutes
// header for the /v1 rationale).
func RegisterCompositionRoutes(api huma.API, h *CompositionHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "createHolderAccount",
		Method:      http.MethodPost,
		Path:        "/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts",
		Summary:     "Open a holder-owned account (with optional instrument)",
		Tags:        []string{"Composition"},
		Security:    secCompositionBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.CreateHolderAccountHuma)
}
