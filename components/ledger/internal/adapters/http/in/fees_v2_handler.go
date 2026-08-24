// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the shell layer over the fee and billing surface. Every operation
// here names the ledger in its path and delegates to the transport-agnostic cores in
// fees_package.go, fees_handler.go, billing_package.go and
// billing_calculate_handler.go. Two things are worth knowing, and both are about
// which ledger the request acts within:
//
//  1. The path names the ledger, so the shells resolve it via parseFeeV2Path and hand
//     it to the core, which passes it to the by-ID scope filters and pins the listings.
//  2. The bodies that carry a ledger still carry it — the field is required on models
//     shared with the in-process fee seam, so it cannot be dropped for one caller — and
//     the shells refuse a value that names a different ledger than the path. See
//     requireBodyLedgerMatchesPath.
//
// The rest of the shell mechanics: the fee body validator runs inside the replicated
// body-parsing span (decodeFeeBodyInSpan, NOT pkgHTTP.DecodeAndValidate), the two
// listings bind their query imperatively, and errors go out through the
// pkgHTTP.HumaProblem envelope.
//
// AUTH is unchanged: appName "plugin-fees" with the same (resource, verb) tuples the
// organization-scoped routes carry, attached as the Fiber guard chain in
// RegisterFeesV2RoutesToApp BEFORE these terminals. The per-op Security metadata on
// the registrations is spec metadata only.

// FeeV2Path is the ledger-scoped path prefix every v2 fee and billing operation
// carries. The parameters have no format tag: ParseUUIDPathParameters on the Fiber
// guard chain stays the sole path-UUID validator, as it is on every other migrated
// resource.
type FeeV2Path struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// parseFeeV2Path resolves the organization and the ledger a ledger-scoped fee request
// acts within.
//
// The nil identifier is refused rather than carried inward. It is a syntactically
// valid UUID, so ParseUUIDPathParameters admits it, and both fee repositories read it
// as "no ledger requested" — a by-ID read would then match a package on any ledger of
// the organization and a listing would return every ledger's. No ledger is created
// with it, so nothing legitimate is turned away.
func parseFeeV2Path(p FeeV2Path) (organizationID, ledgerID uuid.UUID, err error) {
	organizationID, ledgerID, err = parseOrgLedger(p.OrganizationID, p.LedgerID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if ledgerID == uuid.Nil {
		return uuid.Nil, uuid.Nil, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", "ledger_id")
	}

	return organizationID, ledgerID, nil
}

// --- POST /ledgers/{ledger_id}/packages -----------------------------------------

// CreatePackageV2Request is the ledger-scoped create envelope. RawBody keeps the
// body out of Huma's validator (see file header).
type CreatePackageV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// CreatePackageV2 decodes+validates the raw body imperatively, refuses a body
// ledger that disagrees with the path, then delegates to the shared createPackage
// core.
func (handler *PackageHandler) CreatePackageV2(ctx context.Context, in *CreatePackageV2Request) (*CreatePackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.CreatePackageInput)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packOut, err := handler.createPackage(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreatePackageResponse{Status: http.StatusCreated, Body: packOut}, nil
}

// --- GET /ledgers/{ledger_id}/packages (list) -----------------------------------

// ListPackagesV2Request advertises the ledger-scoped list query params in the spec
// (doc-only, no validation tags — the fee core is the sole validator) and captures the
// raw query via Resolve for the imperative binder.
//
// There is no ledgerId param: the path names the ledger and the core refuses the key.
type ListPackagesV2Request struct {
	FeeV2Path

	SegmentID        string `query:"segmentId" doc:"Filter by segment ID (UUID)"`
	TransactionRoute string `query:"transactionRoute" doc:"Filter by transaction route"`
	Enable           string `query:"enable" doc:"Filter by enabled flag (true, false)"`
	Limit            string `query:"limit" doc:"Number of items per page (default 10)"`
	Page             string `query:"page" doc:"Page number (default 1)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the core.
func (in *ListPackagesV2Request) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// GetAllPackagesV2 binds the query imperatively then delegates to the
// ledger-scoped listing.
func (handler *PackageHandler) GetAllPackagesV2(ctx context.Context, in *ListPackagesV2Request) (*ListPackagesResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllPackagesInLedger(ctx, orgID, ledgerID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListPackagesResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET/DELETE /ledgers/{ledger_id}/packages/{id} -------------------------------

// PackageIDV2Request is the ledger-scoped by-id envelope, shared by the read and the
// delete. The id path param carries no format tag (ParseUUIDPathParameters is the sole
// validator).
type PackageIDV2Request struct {
	FeeV2Path

	ID string `path:"id" doc:"Package ID (UUID)"`
}

// GetPackageByIDV2 delegates to the shared getPackageByID core with the path
// ledger, so a package another ledger of the organization owns reads as absent.
func (handler *PackageHandler) GetPackageByIDV2(ctx context.Context, in *PackageIDV2Request) (*GetPackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packModel, err := handler.getPackageByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetPackageResponse{Status: http.StatusOK, Body: packModel}, nil
}

// DeletePackageByIDV2 delegates to deletePackageByID with the path ledger; returns
// a bodiless 204 on success.
func (handler *PackageHandler) DeletePackageByIDV2(ctx context.Context, in *PackageIDV2Request) (*DeletePackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deletePackageByID(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeletePackageResponse{}, nil
}

// --- PATCH /ledgers/{ledger_id}/packages/{id} ------------------------------------

// UpdatePackageV2Request is the ledger-scoped update envelope (RawBody, see Create).
type UpdatePackageV2Request struct {
	FeeV2Path

	ID      string `path:"id" doc:"Package ID (UUID)"`
	RawBody []byte `contentType:"application/json"`
}

// UpdatePackageByIDV2 decodes+validates the raw body imperatively then delegates
// to the shared updatePackageByID core with the path ledger. The update body carries
// no ledger, so there is nothing to reconcile against the path.
func (handler *PackageHandler) UpdatePackageByIDV2(ctx context.Context, in *UpdatePackageV2Request) (*UpdatePackageResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.UpdatePackageInput)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	packUpdated, err := handler.updatePackageByID(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdatePackageResponse{Status: http.StatusOK, Body: packUpdated}, nil
}

// --- POST /ledgers/{ledger_id}/estimates -----------------------------------------

// EstimateFeeV2Request is the ledger-scoped estimate envelope (RawBody, see Create).
type EstimateFeeV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// EstimateFeeCalculationV2 decodes+validates the raw body imperatively, refuses a
// body ledger that disagrees with the path, then delegates to the shared
// estimateFeeCalculation core and serializes the envelope verbatim.
//
// The response Body stays a pre-serialized []byte for the reason
// EstimateFeeResponse documents: the estimate embeds the projected transaction tree,
// whose time alias makes Huma's schema generator panic. The escape hatch is a property
// of the response type, so it holds identically on both contracts.
func (handler *FeeHandler) EstimateFeeCalculationV2(ctx context.Context, in *EstimateFeeV2Request) (*EstimateFeeResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.FeeEstimate)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	response, err := handler.estimateFeeCalculation(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	body, err := json.Marshal(response)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "Fee"))
	}

	return &EstimateFeeResponse{Status: http.StatusOK, Body: body}, nil
}

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

// --- POST /ledgers/{ledger_id}/billing/calculate ---------------------------------

// CalculateBillingV2Request is the ledger-scoped calculate envelope (RawBody, see
// CreatePackageV2Request).
type CalculateBillingV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// CalculateBillingV2 decodes+validates the raw body imperatively, refuses a body
// ledger that disagrees with the path, then delegates to the shared calculateBilling
// core.
func (handler *BillingCalculateHandler) CalculateBillingV2(ctx context.Context, in *CalculateBillingV2Request) (*CalculateBillingResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingCalculateRequest)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.calculateBilling(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CalculateBillingResponse{Status: http.StatusOK, Body: result}, nil
}
