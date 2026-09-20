// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// This file registers the four dashboard reads on Huma, following the pattern
// established in rule_handler_huma.go. Conventions carried verbatim:
//
//   - Query params carry NO validation struct tag (only doc:). The imperative
//     model.NewDashboardWindow is the sole validator, so a bad period yields
//     the canonical Midaz 400 (0498), never a native Huma 422.
//   - Handler funcs delegate to the transport-agnostic cores on
//     *DashboardHandler.
//   - Errors flow through the package-level humaProblem (rule_handler_huma.go).
//
// The reads are GET-only: the dashboard aggregates an append-only trail and
// owns no state of its own, so there is nothing here to write.

// dashboardCacheControl is the response caching directive every dashboard read
// carries. private because the figures are one tenant's; max-age matches the
// server-side TTL, so an intermediary and the server expire together instead
// of a proxy holding a window the server has already recomputed.
const dashboardCacheControl = "private, max-age=60"

// DashboardWindowInputHuma is the Huma request envelope shared by the four
// reads. Every param is a plain string with only doc: — see the file header.
type DashboardWindowInputHuma struct {
	Period    string `query:"period" doc:"Relative window: 7d, 30d or 90d (default: 30d). Mutually exclusive with start_date/end_date."`
	StartDate string `query:"start_date" doc:"Window start (RFC3339). Requires end_date. Mutually exclusive with period."`
	EndDate   string `query:"end_date" doc:"Window end (RFC3339), exclusive. Requires start_date. Window may not exceed 90 days."`
}

// input converts the Huma envelope into the transport-agnostic input.
func (in *DashboardWindowInputHuma) input() DashboardWindowInput {
	return DashboardWindowInput{Period: in.Period, StartDate: in.StartDate, EndDate: in.EndDate}
}

// Dashboard response envelopes. Each carries the shared Cache-Control header.
type (
	// DashboardMetricsOutputHuma is the response for GET /v1/dashboard/metrics.
	DashboardMetricsOutputHuma struct {
		CacheControl string `header:"Cache-Control"`
		Body         *model.DashboardMetrics
	}
	// DashboardVolumeOutputHuma is the response for GET /v1/dashboard/volume.
	DashboardVolumeOutputHuma struct {
		CacheControl string `header:"Cache-Control"`
		Body         *model.DashboardVolume
	}
	// DashboardFraudTypesOutputHuma is the response for GET /v1/dashboard/fraud-types.
	DashboardFraudTypesOutputHuma struct {
		CacheControl string `header:"Cache-Control"`
		Body         *model.DashboardFraudTypes
	}
	// DashboardTopRulesOutputHuma is the response for GET /v1/dashboard/top-rules.
	DashboardTopRulesOutputHuma struct {
		CacheControl string `header:"Cache-Control"`
		Body         *model.DashboardTopRules
	}
)

// dashboardOp builds the shared per-operation definition for a dashboard read.
func dashboardOp(opID, path, summary, description string) huma.Operation {
	return huma.Operation{
		OperationID: opID,
		Method:      http.MethodGet,
		Path:        path,
		Summary:     summary,
		Description: description,
		Tags:        []string{"Dashboard"},
		Security:    secBearerOrAPIKey,
	}
}

// RegisterDashboardRoutes mounts the four dashboard read operations.
func RegisterDashboardRoutes(api huma.API, h *DashboardHandler) {
	const base = "/dashboard"

	huma.Register(api, dashboardOp("getDashboardMetrics", base+"/metrics",
		"Get dashboard headline metrics",
		"Returns validation counts, decision rates, blocked volume per asset, mean decision latency and the live active rule and limit counts for the window."),
		h.GetMetricsHuma)

	huma.Register(api, dashboardOp("getDashboardVolume", base+"/volume",
		"Get dashboard volume series",
		"Returns the per-day count of validations across the window."),
		h.GetVolumeHuma)

	huma.Register(api, dashboardOp("getDashboardFraudTypes", base+"/fraud-types",
		"Get dashboard flagged-traffic breakdown",
		"Returns DENY + REVIEW decisions broken down by transaction type, with each type's share of the flagged traffic."),
		h.GetFraudTypesHuma)

	huma.Register(api, dashboardOp("getDashboardTopRules", base+"/top-rules",
		"Get the busiest rules in the window",
		"Returns up to ten rules ordered by how often they matched, with the number of validations that evaluated each, its detection rate, and the mean end-to-end latency of the validations it matched."),
		h.GetTopRulesHuma)
}

// GetMetricsHuma serves GET /v1/dashboard/metrics.
func (h *DashboardHandler) GetMetricsHuma(ctx context.Context, in *DashboardWindowInputHuma) (*DashboardMetricsOutputHuma, error) {
	metrics, err := h.getMetrics(ctx, in.input())
	if err != nil {
		return nil, humaProblem(err)
	}

	return &DashboardMetricsOutputHuma{CacheControl: dashboardCacheControl, Body: metrics}, nil
}

// GetVolumeHuma serves GET /v1/dashboard/volume.
func (h *DashboardHandler) GetVolumeHuma(ctx context.Context, in *DashboardWindowInputHuma) (*DashboardVolumeOutputHuma, error) {
	volume, err := h.getVolume(ctx, in.input())
	if err != nil {
		return nil, humaProblem(err)
	}

	return &DashboardVolumeOutputHuma{CacheControl: dashboardCacheControl, Body: volume}, nil
}

// GetFraudTypesHuma serves GET /v1/dashboard/fraud-types.
func (h *DashboardHandler) GetFraudTypesHuma(ctx context.Context, in *DashboardWindowInputHuma) (*DashboardFraudTypesOutputHuma, error) {
	types, err := h.getFraudTypes(ctx, in.input())
	if err != nil {
		return nil, humaProblem(err)
	}

	return &DashboardFraudTypesOutputHuma{CacheControl: dashboardCacheControl, Body: types}, nil
}

// GetTopRulesHuma serves GET /v1/dashboard/top-rules.
func (h *DashboardHandler) GetTopRulesHuma(ctx context.Context, in *DashboardWindowInputHuma) (*DashboardTopRulesOutputHuma, error) {
	topRules, err := h.getTopRules(ctx, in.input())
	if err != nil {
		return nil, humaProblem(err)
	}

	return &DashboardTopRulesOutputHuma{CacheControl: dashboardCacheControl, Body: topRules}, nil
}
