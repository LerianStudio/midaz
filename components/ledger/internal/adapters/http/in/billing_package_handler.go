// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Huma transport of the billing-package resource: the response
// envelopes, the per-operation security metadata, and the five shells that decode a
// request, call a core in billing_package_core.go, and render the envelope.
//
// Every shell names the ledger in its path and resolves it via parseFeeV2Path, then
// hands it to the core, which passes it to the by-ID scope filters and pins the
// listings. A body that also carries a ledger must agree with the path — see
// requireBodyLedgerMatchesPath in fee_ledger_scope.go.
//
// Body ops carry RawBody + SkipValidateBody, and the fee body validator runs inside
// the replicated body-parsing span (decodeFeeBodyInSpan, NOT
// pkgHTTP.DecodeAndValidate). The listing binds its query imperatively. Errors go out
// through the pkgHTTP.HumaProblem envelope.
//
// AUTH is appName "midaz" (routes.go midazName), resource "billing-packages". The
// Fiber guard chain — auth.Authorize("midaz","billing-packages",verb) + the
// fees-scoped tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("billing-packages") — is attached on the /v2 group BEFORE
// these terminals (see billing_package_routes.go), so the per-op Security metadata is
// SPEC metadata only.

// secBillingBearer advertises that each billing-package operation accepts a JWT bearer
// token (Bearer-only, matching the Fiber guard chain). SPEC metadata
// only; runtime auth is the Fiber guard chain.
var secBillingBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// CreateBillingPackageResponse pins 201 Created.
type CreateBillingPackageResponse struct {
	Status int
	Body   *model.BillingPackage
}

// ListBillingPackagesResponse carries the pagination envelope verbatim.
type ListBillingPackagesResponse struct {
	Status int
	Body   model.Pagination
}

// GetBillingPackageResponse carries the billing package verbatim.
type GetBillingPackageResponse struct {
	Status int
	Body   *model.BillingPackage
}

// UpdateBillingPackageResponse carries the updated package with 200 OK.
type UpdateBillingPackageResponse struct {
	Status int
	Body   *model.BillingPackage
}

// DeleteBillingPackageResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204.
type DeleteBillingPackageResponse struct{}

// --- POST /ledgers/{ledger_id}/billing-packages ----------------------------------

// CreateBillingPackageV2Request is the ledger-scoped create envelope (RawBody, see
// CreatePackageV2Request).
type CreateBillingPackageV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// CreateBillingPackageV2 decodes+validates the raw body imperatively, refuses a
// body ledger that disagrees with the path, then delegates to the shared
// createBillingPackage core.
func (handler *BillingPackageHandler) CreateBillingPackageV2(ctx context.Context, in *CreateBillingPackageV2Request) (*CreateBillingPackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingPackage)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.createBillingPackage(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateBillingPackageResponse{Status: http.StatusCreated, Body: result}, nil
}

// --- GET /ledgers/{ledger_id}/billing-packages (list) ----------------------------

// ListBillingPackagesV2Request advertises the ledger-scoped list query params in the
// spec (doc-only, no validation tags — the core is the sole validator) and captures the
// raw query via Resolve for the imperative binder.
//
// There is no ledgerId param: the path names the ledger and the core refuses the key.
type ListBillingPackagesV2Request struct {
	FeeV2Path

	Type  string `query:"type" doc:"Filter by billing package type (volume, maintenance)"`
	Limit string `query:"limit" doc:"Number of items per page (default 10)"`
	Page  string `query:"page" doc:"Page number (default 1)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the core.
func (in *ListBillingPackagesV2Request) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// GetAllBillingPackagesV2 binds the query imperatively then delegates to the
// ledger-scoped listing.
func (handler *BillingPackageHandler) GetAllBillingPackagesV2(ctx context.Context, in *ListBillingPackagesV2Request) (*ListBillingPackagesResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllBillingPackagesInLedger(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListBillingPackagesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET/DELETE /ledgers/{ledger_id}/billing-packages/{id} -----------------------

// BillingPackageIDV2Request is the ledger-scoped by-id envelope, shared by the read
// and the delete.
type BillingPackageIDV2Request struct {
	FeeV2Path

	ID string `path:"id" doc:"BillingPackage ID (UUID)"`
}

// GetBillingPackageByIDV2 delegates to the shared getBillingPackageByID core with
// the path ledger, so a package another ledger of the organization owns reads as
// absent.
func (handler *BillingPackageHandler) GetBillingPackageByIDV2(ctx context.Context, in *BillingPackageIDV2Request) (*GetBillingPackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.getBillingPackageByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetBillingPackageResponse{Status: http.StatusOK, Body: result}, nil
}

// DeleteBillingPackageV2 delegates to deleteBillingPackage with the path ledger;
// returns a bodiless 204 on success.
func (handler *BillingPackageHandler) DeleteBillingPackageV2(ctx context.Context, in *BillingPackageIDV2Request) (*DeleteBillingPackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deleteBillingPackage(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeleteBillingPackageResponse{}, nil
}

// --- PATCH /ledgers/{ledger_id}/billing-packages/{id} ----------------------------

// UpdateBillingPackageV2Request is the ledger-scoped update envelope (RawBody, see
// CreatePackageV2Request).
type UpdateBillingPackageV2Request struct {
	FeeV2Path

	ID      string `path:"id" doc:"BillingPackage ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdateBillingPackageV2 decodes+validates the raw body imperatively then
// delegates to the shared updateBillingPackage core with the path ledger. The update
// body carries no ledger, so there is nothing to reconcile against the path.
func (handler *BillingPackageHandler) UpdateBillingPackageV2(ctx context.Context, in *UpdateBillingPackageV2Request) (*UpdateBillingPackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingPackageUpdate)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.updateBillingPackage(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateBillingPackageResponse{Status: http.StatusOK, Body: result}, nil
}
