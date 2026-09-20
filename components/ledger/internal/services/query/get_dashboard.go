// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// GetDashboardMetrics returns the window's transaction count, its breakdown by
// status, and the settled volume per asset.
func (uc *UseCase) GetDashboardMetrics(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_dashboard_metrics")
	defer span.End()

	metrics, err := uc.DashboardRepo.Metrics(ctx, organizationID, ledgerID, window)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read dashboard metrics", err)

		logger.Log(ctx, libLog.LevelError, "Failed to read dashboard metrics", libLog.Err(err))

		return nil, err
	}

	return metrics, nil
}

// GetDashboardVolume returns one point per UTC calendar day the window touches.
func (uc *UseCase) GetDashboardVolume(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_dashboard_volume")
	defer span.End()

	volume, err := uc.DashboardRepo.Volume(ctx, organizationID, ledgerID, window)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read dashboard volume", err)

		logger.Log(ctx, libLog.LevelError, "Failed to read dashboard volume", libLog.Err(err))

		return nil, err
	}

	return volume, nil
}

// GetDashboardAssets returns the ledger's current position per asset. It takes
// no window: a balance is a running total, not an aggregate over history.
func (uc *UseCase) GetDashboardAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (*mmodel.DashboardAssets, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_dashboard_assets")
	defer span.End()

	assets, err := uc.DashboardRepo.Assets(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read dashboard assets", err)

		logger.Log(ctx, libLog.LevelError, "Failed to read dashboard assets", libLog.Err(err))

		return nil, err
	}

	return assets, nil
}
