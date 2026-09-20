// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// DashboardService is the read surface the dashboard handler depends on. It is
// declared here, where it is used, per the repository's port convention.
type DashboardService interface {
	Metrics(ctx context.Context, window model.DashboardWindow) (*model.DashboardMetrics, error)
	Volume(ctx context.Context, window model.DashboardWindow) (*model.DashboardVolume, error)
	FraudTypes(ctx context.Context, window model.DashboardWindow) (*model.DashboardFraudTypes, error)
	TopRules(ctx context.Context, window model.DashboardWindow) (*model.DashboardTopRules, error)
}

// DashboardHandler serves the operator dashboard reads.
type DashboardHandler struct {
	service DashboardService
	clock   clock.Clock
}

// NewDashboardHandler creates a dashboard handler. clk resolves the relative
// period ("last 30 days") against the service clock, never time.Now(), so the
// window a test asks for is the window it gets.
func NewDashboardHandler(service DashboardService, clk clock.Clock) *DashboardHandler {
	if clk == nil {
		clk = clock.New()
	}

	return &DashboardHandler{service: service, clock: clk}
}

// DashboardWindowInput carries the window query parameters shared by every
// dashboard read. Params are snake_case, matching every other Tracer read.
type DashboardWindowInput struct {
	Period    string
	StartDate string
	EndDate   string
}

// resolveWindow normalizes the query parameters into a window, converting a
// rejected window into the canonical Midaz 400 rather than letting the raw
// parse error reach the wire.
func (h *DashboardHandler) resolveWindow(ctx context.Context, in DashboardWindowInput) (model.DashboardWindow, error) {
	window, err := model.NewDashboardWindow(in.Period, in.StartDate, in.EndDate, h.clock.Now())
	if err != nil {
		return model.DashboardWindow{}, h.badWindow(ctx, err)
	}

	return window, nil
}

// badWindow renders an invalid window as the canonical validation error. The
// specific reason ("unsupported period 45d", "window may not exceed 90 days")
// goes to the span and the log, not to the caller: the canonical message
// already names every way a window can be wrong, and an operator debugging a
// console stuck on a 400 needs which one it was.
func (h *DashboardHandler) badWindow(ctx context.Context, err error) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	spanCtx, span := tracer.Start(ctx, "handler.dashboard.invalid_window")
	defer span.End()

	libOtel.HandleSpanBusinessErrorEvent(span, "Invalid dashboard window", err)

	// A window error is the caller's mistake, not the service's, so it is
	// logged at Debug: a client looping on a bad period must not be able to
	// fill an operator's logs.
	logging.WithTrace(spanCtx, logger).Log(spanCtx, libLog.LevelDebug,
		"rejected dashboard window", libLog.Err(err))

	if !errors.Is(err, constant.ErrInvalidDashboardWindow) {
		// Unreachable: NewDashboardWindow only ever wraps this sentinel.
		return pkg.ValidateBusinessError(constant.ErrInvalidDashboardWindow, "Dashboard")
	}

	return pkg.ValidateBusinessError(constant.ErrInvalidDashboardWindow, "Dashboard")
}

// getMetrics is the transport-agnostic core of GET /v1/dashboard/metrics.
func (h *DashboardHandler) getMetrics(ctx context.Context, in DashboardWindowInput) (*model.DashboardMetrics, error) {
	window, err := h.resolveWindow(ctx, in)
	if err != nil {
		return nil, err
	}

	return h.service.Metrics(ctx, window)
}

// getVolume is the transport-agnostic core of GET /v1/dashboard/volume.
func (h *DashboardHandler) getVolume(ctx context.Context, in DashboardWindowInput) (*model.DashboardVolume, error) {
	window, err := h.resolveWindow(ctx, in)
	if err != nil {
		return nil, err
	}

	return h.service.Volume(ctx, window)
}

// getFraudTypes is the transport-agnostic core of GET /v1/dashboard/fraud-types.
func (h *DashboardHandler) getFraudTypes(ctx context.Context, in DashboardWindowInput) (*model.DashboardFraudTypes, error) {
	window, err := h.resolveWindow(ctx, in)
	if err != nil {
		return nil, err
	}

	return h.service.FraudTypes(ctx, window)
}

// getTopRules is the transport-agnostic core of GET /v1/dashboard/top-rules.
func (h *DashboardHandler) getTopRules(ctx context.Context, in DashboardWindowInput) (*model.DashboardTopRules, error) {
	window, err := h.resolveWindow(ctx, in)
	if err != nil {
		return nil, err
	}

	return h.service.TopRules(ctx, window)
}
