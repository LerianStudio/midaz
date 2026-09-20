// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// DashboardHandler serves the three ledger dashboard reads. They are GET-only:
// the dashboard aggregates tables the ledger already writes and owns no state
// of its own, so there is nothing here to write and no Command use case to
// hold.
type DashboardHandler struct {
	Query *query.UseCase

	// Clock supplies "now" when a caller names a relative period. It is a field
	// rather than a call to time.Now() so a test pins the window it asserts on;
	// a nil Clock means the wall clock.
	Clock func() time.Time
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The cores below own the span and the service call and take primitive args, so
// nothing transport-shaped reaches them. Every canonical Midaz error they
// return is rendered by the caller via pkgHTTP.HumaProblem, which fixes the code
// and the HTTP status.

// now reads the handler's clock, falling back to the wall clock.
func (handler *DashboardHandler) now() time.Time {
	if handler.Clock == nil {
		return time.Now().UTC()
	}

	return handler.Clock()
}

// resolveWindow derives the aggregation window from the raw query parameters.
//
// It is the SOLE validator of those parameters, which is why they carry no Huma
// validation tags: a `period` with an enum tag would be refused by the framework
// with a native 422 that the ledger's own error catalogue does not explain, and
// a caller could not tell that refusal apart from a body-validation failure.
// Everything wrong with a window — an unsupported period, both forms at once, a
// range over 90 days, an inverted range, a date that is not RFC3339 — lands on
// the one canonical 0498.
func (handler *DashboardHandler) resolveWindow(period, startDate, endDate string) (dashboard.Window, error) {
	window, err := dashboard.NewWindow(period, startDate, endDate, handler.now())
	if err != nil {
		if !errors.Is(err, constant.ErrInvalidDashboardWindow) {
			// Unreachable: NewWindow only ever wraps this sentinel. Mapping it
			// anyway keeps a future sentinel from escaping as a bare 500.
			return dashboard.Window{}, pkg.ValidateBusinessError(constant.ErrInvalidDashboardWindow, constant.EntityDashboard)
		}

		return dashboard.Window{}, pkg.ValidateBusinessError(constant.ErrInvalidDashboardWindow, constant.EntityDashboard)
	}

	return window, nil
}

// getDashboardMetrics owns the span + service call for the headline panel.
func (handler *DashboardHandler) getDashboardMetrics(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_dashboard_metrics")
	defer span.End()

	metrics, err := handler.Query.GetDashboardMetrics(ctx, organizationID, ledgerID, window)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to read dashboard metrics on query", err)

		return nil, err
	}

	return metrics, nil
}

// getDashboardVolume owns the span + service call for the per-day series.
func (handler *DashboardHandler) getDashboardVolume(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_dashboard_volume")
	defer span.End()

	volume, err := handler.Query.GetDashboardVolume(ctx, organizationID, ledgerID, window)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to read dashboard volume on query", err)

		return nil, err
	}

	return volume, nil
}

// getDashboardAssets owns the span + service call for the current position.
func (handler *DashboardHandler) getDashboardAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (*mmodel.DashboardAssets, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_dashboard_assets")
	defer span.End()

	assets, err := handler.Query.GetDashboardAssets(ctx, organizationID, ledgerID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to read dashboard assets on query", err)

		return nil, err
	}

	return assets, nil
}
