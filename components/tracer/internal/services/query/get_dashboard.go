// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// GetDashboardQuery serves the three dashboard reads. The three share one type
// because they share everything that matters: the same window, the same
// repository, the same tracing shape and the same tenant. Splitting them into
// three near-identical services would multiply the wiring without separating
// anything.
type GetDashboardQuery struct {
	repo DashboardRepository
}

// NewGetDashboardQuery creates a GetDashboardQuery over the given repository.
// In production the repository handed in is the Valkey cache decorator, which
// falls through to postgres on a miss.
func NewGetDashboardQuery(repo DashboardRepository) *GetDashboardQuery {
	return &GetDashboardQuery{repo: repo}
}

// Metrics returns the headline dashboard panel for the window.
func (q *GetDashboardQuery) Metrics(ctx context.Context, window model.DashboardWindow) (*model.DashboardMetrics, error) {
	ctx, span, done := q.begin(ctx, "service.dashboard.metrics", window)
	defer done()

	if err := ctx.Err(); err != nil {
		libOtel.HandleSpanError(span, "Context cancelled before repository call", err)

		return nil, fmt.Errorf("dashboard metrics: %w", err)
	}

	metrics, err := q.repo.Metrics(ctx, window)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to read dashboard metrics", err)

		return nil, fmt.Errorf("dashboard metrics: %w", err)
	}

	span.SetAttributes(attribute.Int64("app.response.transactions_processed", metrics.TransactionsProcessed))

	return metrics, nil
}

// Volume returns the per-day volume series for the window.
func (q *GetDashboardQuery) Volume(ctx context.Context, window model.DashboardWindow) (*model.DashboardVolume, error) {
	ctx, span, done := q.begin(ctx, "service.dashboard.volume", window)
	defer done()

	if err := ctx.Err(); err != nil {
		libOtel.HandleSpanError(span, "Context cancelled before repository call", err)

		return nil, fmt.Errorf("dashboard volume: %w", err)
	}

	volume, err := q.repo.Volume(ctx, window)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to read dashboard volume", err)

		return nil, fmt.Errorf("dashboard volume: %w", err)
	}

	span.SetAttributes(attribute.Int("app.response.points", len(volume.Points)))

	return volume, nil
}

// FraudTypes returns the flagged-traffic breakdown for the window.
func (q *GetDashboardQuery) FraudTypes(ctx context.Context, window model.DashboardWindow) (*model.DashboardFraudTypes, error) {
	ctx, span, done := q.begin(ctx, "service.dashboard.fraud_types", window)
	defer done()

	if err := ctx.Err(); err != nil {
		libOtel.HandleSpanError(span, "Context cancelled before repository call", err)

		return nil, fmt.Errorf("dashboard fraud types: %w", err)
	}

	types, err := q.repo.FraudTypes(ctx, window)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to read dashboard fraud types", err)

		return nil, fmt.Errorf("dashboard fraud types: %w", err)
	}

	span.SetAttributes(attribute.Int64("app.response.total_flagged", types.TotalFlagged))

	return types, nil
}

// begin opens the span and enriches the logger, returning the span plus the
// func that closes it. Shared by the three reads so the tracing shape cannot
// drift between them.
func (q *GetDashboardQuery) begin(ctx context.Context, operation string, window model.DashboardWindow) (context.Context, trace.Span, func()) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, operation)

	_ = logging.WithTrace(ctx, logger)

	span.SetAttributes(
		attribute.String("app.request.window_start", window.From.Format(time.RFC3339)),
		attribute.String("app.request.window_end", window.To.Format(time.RFC3339)),
		attribute.String("app.request.period", window.Period),
	)

	return ctx, span, func() { span.End() }
}
