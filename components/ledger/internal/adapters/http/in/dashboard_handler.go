// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the Huma transport for the three dashboard reads. Conventions
// carried from the sibling resources:
//
//  1. org+ledger are UUID-validated by ParseUUIDPathParameters("dashboard") on
//     the Fiber chain, so the In structs carry them as plain strings with only
//     `doc:` — no format tag, therefore no native Huma 422.
//  2. The window query params likewise carry ONLY `doc:`. resolveWindow
//     (dashboard_core.go) is their sole validator, so every bad window yields
//     the canonical 0498 rather than a framework refusal.
//  3. Errors go through the shared pkgHTTP.HumaProblem; auth is the Fiber guard
//     chain attached before the Huma terminal, and the per-op Security metadata
//     is SPEC-ONLY.

// dashboardCacheControl is the response caching directive every dashboard read
// carries. private because the figures are one tenant's, and max-age matches
// the server-side cache TTL and the window granularity, so an intermediary and
// the server expire together rather than a proxy holding a window the server
// has already recomputed.
const dashboardCacheControl = "private, max-age=60"

// secDashboardBearer is the spec-only Security metadata for the dashboard
// operations: a JWT bearer token. Runtime auth is the Fiber guard chain.
var secDashboardBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// GetDashboardMetricsRequest is the request envelope for the headline panel.
type GetDashboardMetricsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Period         string `query:"period" doc:"Relative window: 7d, 30d or 90d (default: 30d). Mutually exclusive with start_date/end_date."`
	StartDate      string `query:"start_date" doc:"Window start (RFC3339, inclusive). Requires end_date. Mutually exclusive with period."`
	EndDate        string `query:"end_date" doc:"Window end (RFC3339, exclusive). Requires start_date. The window may not exceed 90 days."`
}

// GetDashboardVolumeRequest is the request envelope for the per-day series.
type GetDashboardVolumeRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	Period         string `query:"period" doc:"Relative window: 7d, 30d or 90d (default: 30d). Mutually exclusive with start_date/end_date."`
	StartDate      string `query:"start_date" doc:"Window start (RFC3339, inclusive). Requires end_date. Mutually exclusive with period."`
	EndDate        string `query:"end_date" doc:"Window end (RFC3339, exclusive). Requires start_date. The window may not exceed 90 days."`
}

// GetDashboardAssetsRequest is the request envelope for the current position.
// It declares NO window parameters: the read aggregates no history, so there is
// no window for one to name. A caller that sends period anyway is answered with
// the position rather than refused — a console forwarding one selector to all
// three reads should not have to special-case this one.
type GetDashboardAssetsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
}

// Dashboard response envelopes. Each carries the shared Cache-Control header.
type (
	// GetDashboardMetricsResponse is the response for GET /dashboard/metrics.
	GetDashboardMetricsResponse struct {
		CacheControl string `header:"Cache-Control"`
		Body         *mmodel.LedgerDashboardMetrics
	}
	// GetDashboardVolumeResponse is the response for GET /dashboard/volume.
	GetDashboardVolumeResponse struct {
		CacheControl string `header:"Cache-Control"`
		Body         *mmodel.LedgerDashboardVolume
	}
	// GetDashboardAssetsResponse is the response for GET /dashboard/assets.
	GetDashboardAssetsResponse struct {
		CacheControl string `header:"Cache-Control"`
		Body         *mmodel.LedgerDashboardAssets
	}
)

// GetDashboardMetrics serves GET /dashboard/metrics.
func (handler *DashboardHandler) GetDashboardMetrics(ctx context.Context, in *GetDashboardMetricsRequest) (*GetDashboardMetricsResponse, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	window, err := handler.resolveWindow(in.Period, in.StartDate, in.EndDate)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	metrics, err := handler.getDashboardMetrics(ctx, organizationID, ledgerID, window)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetDashboardMetricsResponse{CacheControl: dashboardCacheControl, Body: metrics}, nil
}

// GetDashboardVolume serves GET /dashboard/volume.
func (handler *DashboardHandler) GetDashboardVolume(ctx context.Context, in *GetDashboardVolumeRequest) (*GetDashboardVolumeResponse, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	window, err := handler.resolveWindow(in.Period, in.StartDate, in.EndDate)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	volume, err := handler.getDashboardVolume(ctx, organizationID, ledgerID, window)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetDashboardVolumeResponse{CacheControl: dashboardCacheControl, Body: volume}, nil
}

// GetDashboardAssets serves GET /dashboard/assets.
func (handler *DashboardHandler) GetDashboardAssets(ctx context.Context, in *GetDashboardAssetsRequest) (*GetDashboardAssetsResponse, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	assets, err := handler.getDashboardAssets(ctx, organizationID, ledgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetDashboardAssetsResponse{CacheControl: dashboardCacheControl, Body: assets}, nil
}
