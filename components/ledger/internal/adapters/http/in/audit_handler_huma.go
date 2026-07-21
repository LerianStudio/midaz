// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the CRM protection-audit listing.
// It mirrors the asset exemplar (asset_handler_huma.go); see that file's header
// for the full conventions. Audit-specific notes:
//
//  1. AUTH is appName "midaz" (crm_routes.go ApplicationName), resource
//     "protection". The Fiber guard chain is Bearer-only, so the per-op Security
//     metadata here is Bearer-only too — SPEC metadata only;
//     runtime auth stays the Fiber guard chain (auth.Authorize("midaz","protection",
//     "get") + tenant + ParseUUIDPathParameters("organization")) attached BEFORE the
//     Huma terminal.
//  2. ORG-SCOPED (no ledger in the path): the shell resolves only organization_id
//     via the shared parseOrg helper (defined in ledger_handler_huma.go).
//  3. The shell captures the raw query via Resolve and rebuilds the
//     map[string]string the getAuditEvents core consumes (via the shared
//     queriesFromValues helper), so the binding is byte-identical to the Fiber
//     c.Queries() path — the core, not Huma, owns ALL query validation (limit/
//     cursor/sort_order via ValidateParameters, dates via parseAuditTime, outcome
//     via the reduced enum). No native Huma 422/400 on the query.
//  4. Errors go through pkgHTTP.HumaProblem.

// secAuditBearer advertises that the audit operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber guard chain). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secAuditBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// GetAuditEventsInputHuma advertises the list query params in the spec (doc-only, no
// validation tags — the core is the sole validator) and captures the raw query via
// Resolve for the imperative binder.
type GetAuditEventsInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token; only required when the auth plugin is enabled"`
	Limit          string `query:"limit" doc:"Maximum number of events to return (default 20)"`
	Cursor         string `query:"cursor" doc:"Opaque pagination cursor"`
	SortOrder      string `query:"sort_order" doc:"Sort order: asc or desc (default desc)"`
	Action         string `query:"action" doc:"Filter by action"`
	Actor          string `query:"actor" doc:"Filter by actor"`
	Outcome        string `query:"outcome" doc:"Filter by outcome: success, failure, or already_exists"`
	StartDate      string `query:"start_date" doc:"Inclusive lower time bound (yyyy-mm-dd or RFC3339)"`
	EndDate        string `query:"end_date" doc:"Inclusive upper time bound (yyyy-mm-dd or RFC3339)"`

	// rawQuery is the request's parsed query, captured by Resolve. It is the binding
	// source (NOT the struct-tag fields above), so it matches c.Queries() exactly.
	rawQuery url.Values
}

// Resolve captures the raw query before the handler. It performs NO validation and
// NEVER returns an error — canonical rejection stays in the getAuditEvents core.
func (in *GetAuditEventsInputHuma) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	in.rawQuery = u.Query()

	return nil
}

// GetAuditEventsOutputHuma carries the audit envelope verbatim.
type GetAuditEventsOutputHuma struct {
	Status int
	Body   *auditEventsEnvelope
}

// GetAuditEventsHuma binds the query imperatively then delegates to the shared
// getAuditEvents core.
func (handler *AuditHandler) GetAuditEventsHuma(ctx context.Context, in *GetAuditEventsInputHuma) (*GetAuditEventsOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	envelope, err := handler.getAuditEvents(ctx, orgID, queriesFromValues(in.rawQuery))
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAuditEventsOutputHuma{Status: http.StatusOK, Body: envelope}, nil
}

// RegisterAuditRoutes registers the migrated protection-audit operation on the
// shared Huma API. It is the per-file seam the unified server calls (conditionally,
// only in envelope encryption mode — mirroring the Fiber `if auditHandler != nil`
// guard in crm_routes.go); the auth ("midaz","protection","get") + tenant +
// ParseUUIDPathParameters("organization") middleware chain is attached on the /v1
// group (Fiber-level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler_huma.go's RegisterAssetRoutes header for the /v1 rationale).
func RegisterAuditRoutes(api huma.API, h *AuditHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "getAuditEvents",
		Method:      http.MethodGet,
		Path:        "/organizations/{organization_id}/protection/audit",
		Summary:     "List Protection Audit Events",
		Tags:        []string{"Protection"},
		Security:    secAuditBearer,
	}, h.GetAuditEventsHuma)
}
