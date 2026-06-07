// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// GetLimitQuery handles retrieving a single limit.
type GetLimitQuery struct {
	repo LimitRepository
}

// NewGetLimitQuery creates a new GetLimitQuery with dependencies.
func NewGetLimitQuery(repo LimitRepository) *GetLimitQuery {
	return &GetLimitQuery{repo: repo}
}

// Execute retrieves a limit by ID.
// Returns constant.ErrLimitNotFound if the limit doesn't exist.
func (q *GetLimitQuery) Execute(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Validate input
	if id == uuid.Nil {
		err := constant.ErrLimitInvalidID
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit ID provided", err)

		return nil, err
	}

	logger.With(
		libLog.String("operation", "service.limit.get"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting limit")

	span.SetAttributes(attribute.String("app.request.limit_id", id.String()))

	// Retrieve from repository
	limit, err := q.repo.GetByID(ctx, id)
	if err != nil {
		// Distinguish business errors (not found) from infrastructure errors
		if errors.Is(err, constant.ErrLimitNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)
			logger.With(
				libLog.String("operation", "service.limit.get"),
				libLog.String("limit.id", id.String()),
			).Log(ctx, libLog.LevelWarn, "Limit not found")
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve limit", err)
			logger.With(
				libLog.String("operation", "service.limit.get"),
				libLog.String("limit.id", id.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to get limit")
		}

		return nil, err
	}

	logger.With(
		libLog.String("operation", "service.limit.get"),
		libLog.String("limit.id", limit.ID.String()),
		libLog.String("limit.name", limit.Name),
		libLog.String("limit.status", string(limit.Status)),
	).Log(ctx, libLog.LevelInfo, "Limit retrieved successfully")

	return limit, nil
}
