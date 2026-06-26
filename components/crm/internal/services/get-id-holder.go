// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	libObs "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetHolderByID fetches a holder from the repository.
func (uc *UseCase) GetHolderByID(ctx context.Context, organizationID string, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error) {
	logger, tracer, reqId, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_holder_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Get holder by id %v", id))

	holder, err := uc.HolderRepo.Find(ctx, organizationID, id, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get holder by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get holder by id %v", id))

		return nil, err
	}

	return holder, nil
}
