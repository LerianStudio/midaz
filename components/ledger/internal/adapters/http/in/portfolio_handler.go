// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the portfolio resource, following the
// asset exemplar (asset_handler.go) verbatim: shared parseOrgLedger /
// parsePathUUID / HumaProblem / DecodeAndValidate helpers, path params as plain
// strings (ParseUUIDPathParameters is the sole UUID validator — no format tag), raw
// body bytes decoded imperatively (no native Huma 422), and the query bound via the
// same ValidateParameters path. Auth stays the Fiber middleware chain attached in
// RegisterPortfolioRoutesToApp; the per-op Security metadata is SPEC-ONLY.

// secPortfolioBearer advertises that each portfolio operation accepts a JWT bearer
// token (SPEC metadata only; runtime auth is the Fiber guard chain). The scheme name
// is declared once on the shared Huma API.
var secPortfolioBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /portfolios ---------------------------------------------------------

// CreatePortfolioRequest is the Huma request envelope for POST.
type CreatePortfolioRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreatePortfolioResponse pins 201 (matching http.Created).
type CreatePortfolioResponse struct {
	Status int
	Body   *mmodel.Portfolio
}

// CreatePortfolio decodes+validates the raw body imperatively then delegates to
// the shared createPortfolio core.
func (handler *PortfolioHandler) CreatePortfolio(ctx context.Context, in *CreatePortfolioRequest) (*CreatePortfolioResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreatePortfolioInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	portfolio, err := handler.createPortfolio(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreatePortfolioResponse{Status: http.StatusCreated, Body: portfolio}, nil
}

// --- GET /portfolios (list) ---------------------------------------------------

// ListPortfoliosRequest advertises the list query params (doc-only) and captures
// the raw query via Resolve for the imperative ValidateParameters binder.
type ListPortfoliosRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Metadata       string `query:"metadata" doc:"JSON string to filter portfolios by metadata fields"`
	EntityID       string `query:"entity_id" doc:"Filter portfolios by entity ID"`
	Status         string `query:"status" doc:"Filter portfolios by status"`
	Limit          string `query:"limit" doc:"Max items per page (1-100, default 10)"`
	Page           string `query:"page" doc:"Page number (default 1)"`
	StartDate      string `query:"start_date" doc:"Filter portfolios created on/after this date (YYYY-MM-DD)"`
	EndDate        string `query:"end_date" doc:"Filter portfolios created on/before this date (YYYY-MM-DD)"`
	SortOrder      string `query:"sort_order" doc:"Sort direction (asc, desc)"`

	rawQuery url.Values
}

// Resolve captures the raw query before the handler (NO validation, never errors).
func (in *ListPortfoliosRequest) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// queries rebuilds the map[string]string that ValidateParameters consumes, matching
// Fiber's c.Queries() (last value wins for a repeated key, empty keys included).
func (in *ListPortfoliosRequest) queries() map[string]string {
	return queriesFromValues(in.rawQuery)
}

// ListPortfoliosResponse carries the pagination envelope verbatim.
type ListPortfoliosResponse struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListPortfolios binds the query imperatively then delegates to getAllPortfolios.
func (handler *PortfolioHandler) ListPortfolios(ctx context.Context, in *ListPortfoliosRequest) (*ListPortfoliosResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllPortfolios(ctx, orgID, ledgerID, in.queries())
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListPortfoliosResponse{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /portfolios/{id} -----------------------------------------------------

// GetPortfolioRequest is the by-id request envelope.
type GetPortfolioRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Portfolio ID (UUID)"`
}

// GetPortfolioResponse carries the portfolio verbatim.
type GetPortfolioResponse struct {
	Status int
	Body   *mmodel.Portfolio
}

// GetPortfolioByID delegates to getPortfolioByID.
func (handler *PortfolioHandler) GetPortfolioByID(ctx context.Context, in *GetPortfolioRequest) (*GetPortfolioResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	portfolio, err := handler.getPortfolioByID(ctx, orgID, ledgerID, id)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetPortfolioResponse{Status: http.StatusOK, Body: portfolio}, nil
}

// --- PATCH /portfolios/{id} ---------------------------------------------------

// UpdatePortfolioRequest is the update request envelope (RawBody, see Create).
type UpdatePortfolioRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Portfolio ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// UpdatePortfolioResponse carries the updated portfolio (200, matching http.OK).
type UpdatePortfolioResponse struct {
	Status int
	Body   *mmodel.Portfolio
}

// UpdatePortfolio decodes+validates the raw body imperatively then delegates to
// the shared updatePortfolio core.
func (handler *PortfolioHandler) UpdatePortfolio(ctx context.Context, in *UpdatePortfolioRequest) (*UpdatePortfolioResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdatePortfolioInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	portfolio, err := handler.updatePortfolio(ctx, orgID, ledgerID, id, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdatePortfolioResponse{Status: http.StatusOK, Body: portfolio}, nil
}

// --- DELETE /portfolios/{id} --------------------------------------------------

// DeletePortfolioResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber http.NoContent path.
type DeletePortfolioResponse struct{}

// DeletePortfolioByID delegates to deletePortfolio; returns a bodiless 204.
func (handler *PortfolioHandler) DeletePortfolioByID(ctx context.Context, in *GetPortfolioRequest) (*DeletePortfolioResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := handler.deletePortfolio(ctx, orgID, ledgerID, id); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &DeletePortfolioResponse{}, nil
}

// --- HEAD /portfolios/metrics/count -------------------------------------------

// CountPortfoliosRequest is the HEAD-count request envelope (org+ledger only).
type CountPortfoliosRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// CountPortfoliosResponse replicates the Fiber HEAD-count response: X-Total-Count
// carries the count, Content-Length is pinned to 0, body empty at 204.
type CountPortfoliosResponse struct {
	TotalCount    string `header:"X-Total-Count"`
	ContentLength string `header:"Content-Length"`
}

// CountPortfolios delegates to countPortfolios and sets the count headers.
func (handler *PortfolioHandler) CountPortfolios(ctx context.Context, in *CountPortfoliosRequest) (*CountPortfoliosResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	count, err := handler.countPortfolios(ctx, orgID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CountPortfoliosResponse{
		TotalCount:    strconv.FormatInt(count, 10),
		ContentLength: "0",
	}, nil
}

// RegisterPortfolioRoutes registers the six portfolio operations on the
// shared Huma API. Paths are GROUP-RELATIVE (the Huma API is bound to a versioned Fiber
// group). The auth + tenant + ParseUUIDPathParameters chain is attached by
// registerPortfolioRoutesToApp (Fiber-level), NOT here.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. A straight v1/v2 mirror reuses the same handler methods and the
// same input/output types, so only the operation IDs differ between the twins.
func RegisterPortfolioRoutes(api huma.API, h *PortfolioHandler, opSuffix string) {
	const (
		listPath  = "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios"
		idPath    = listPath + "/{id}"
		countPath = listPath + "/metrics/count"
		tag       = "Portfolios"
	)

	huma.Register(api, huma.Operation{
		OperationID:      "createPortfolio" + opSuffix,
		Method:           http.MethodPost,
		Path:             listPath,
		Summary:          "Create a new portfolio",
		Tags:             []string{tag},
		Security:         secPortfolioBearer,
		SkipValidateBody: true, // body validated imperatively (DecodeAndValidate).
		DefaultStatus:    http.StatusCreated,
	}, h.CreatePortfolio)
	attachTypedRequestBody[mmodel.CreatePortfolioInput](api, "createPortfolio"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "listPortfolios" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List all portfolios",
		Tags:        []string{tag},
		Security:    secPortfolioBearer,
	}, h.ListPortfolios)

	huma.Register(api, huma.Operation{
		OperationID: "getPortfolioByID" + opSuffix,
		Method:      http.MethodGet,
		Path:        idPath,
		Summary:     "Retrieve a specific portfolio",
		Tags:        []string{tag},
		Security:    secPortfolioBearer,
	}, h.GetPortfolioByID)

	huma.Register(api, huma.Operation{
		OperationID:      "updatePortfolio" + opSuffix,
		Method:           http.MethodPatch,
		Path:             idPath,
		Summary:          "Update a portfolio",
		Tags:             []string{tag},
		Security:         secPortfolioBearer,
		SkipValidateBody: true, // body validated imperatively.
	}, h.UpdatePortfolio)
	attachTypedRequestBody[mmodel.UpdatePortfolioInput](api, "updatePortfolio"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID:   "deletePortfolio" + opSuffix,
		Method:        http.MethodDelete,
		Path:          idPath,
		Summary:       "Delete a portfolio",
		Tags:          []string{tag},
		Security:      secPortfolioBearer,
		DefaultStatus: http.StatusNoContent, // bodiless 204.
	}, h.DeletePortfolioByID)

	huma.Register(api, huma.Operation{
		OperationID:   "countPortfolios" + opSuffix,
		Method:        http.MethodHead,
		Path:          countPath,
		Summary:       "Count total portfolios",
		Tags:          []string{tag},
		Security:      secPortfolioBearer,
		DefaultStatus: http.StatusNoContent, // X-Total-Count header + empty 204 body.
	}, h.CountPortfolios)
}

// RegisterPortfolioRoutesToApp wires the portfolio surface onto the /v1
// contract. See registerPortfolioRoutesToApp for what it attaches.
func RegisterPortfolioRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerPortfolioRoutesToApp(group, api, auth, ph, routeOptions, routeOpSuffixV1)
}

// RegisterPortfolioV2RoutesToApp wires the same portfolio surface onto the /v2 contract:
// same paths, same handlers, same authz tuples and tenant chain, differing only in the
// operation IDs the contract publishes. It is additive — /v1 keeps serving portfolios in
// parallel — and introduces no new policy surface.
func RegisterPortfolioV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	registerPortfolioRoutesToApp(group, api, auth, ph, routeOptions, routeOpSuffixV2)
}

// registerPortfolioRoutesToApp is the single description of the portfolio route surface,
// shared by every versioned contract that serves it, mirroring RegisterAssetRoutesToApp.
// For each of the six ops it attaches the Fiber auth chain —
// auth.Authorize("midaz","portfolios",verb) + tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("portfolio") — as MIDDLEWARE ONLY (no terminal handler) on the
// VERSIONED GROUP with GROUP-RELATIVE paths, then registers the Huma terminals via
// RegisterPortfolioRoutes on the SAME group's Huma API. The (resource, verb) authz tuples
// and tenant resolution therefore apply on whichever version group it is mounted on — no
// portfolio route becomes public.
//
// opSuffix distinguishes the operation IDs one version group publishes from another's —
// see routeOpSuffixV1. Nothing else varies between contracts, so a change to the surface
// reaches every version it is mounted on.
func registerPortfolioRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, ph *PortfolioHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		listPath  = "/organizations/:organization_id/ledgers/:ledger_id/portfolios"
		idPath    = listPath + "/:id"
		countPath = listPath + "/metrics/count"
	)

	parse := pkgHTTP.ParseUUIDPathParameters("portfolio")

	routePost(group, listPath, protectedMidaz(auth, "portfolios", "post", routeOptions, parse))
	routePatch(group, idPath, protectedMidaz(auth, "portfolios", "patch", routeOptions, parse))
	routeGet(group, listPath, protectedMidaz(auth, "portfolios", "get", routeOptions, parse))
	routeGet(group, idPath, protectedMidaz(auth, "portfolios", "get", routeOptions, parse))
	routeDelete(group, idPath, protectedMidaz(auth, "portfolios", "delete", routeOptions, parse))
	routeHead(group, countPath, protectedMidaz(auth, "portfolios", "head", routeOptions, parse))

	RegisterPortfolioRoutes(api, ph, opSuffix)
}
