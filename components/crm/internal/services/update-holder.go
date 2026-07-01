// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	libObs "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (uc *UseCase) UpdateHolderByID(ctx context.Context, organizationID string, id uuid.UUID, uhi *mmodel.UpdateHolderInput, fieldsToRemove []string) (*mmodel.Holder, error) {
	logger, tracer, reqId, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update holder: %v", id.String()))

	holder := &mmodel.Holder{
		ExternalID:    uhi.ExternalID,
		Name:          uhi.Name,
		Contact:       uhi.Contact,
		Addresses:     uhi.Addresses,
		NaturalPerson: uhi.NaturalPerson,
		LegalPerson:   uhi.LegalPerson,
		Metadata:      uhi.Metadata,
		UpdatedAt:     time.Now(),
	}

	updatedHolder, err := uc.HolderRepo.Update(ctx, organizationID, id, holder, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update holder", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update holder: %v", err))

		return nil, err
	}

	uc.emitHolderUpdatedEvent(ctx, span, logger, updatedHolder, organizationID)

	return updatedHolder, nil
}

func (uc *UseCase) emitHolderUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, h *mmodel.Holder, organizationID string) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.HolderUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewHolderUpdated(h, organizationID).ToEmitRequest(tenantID, h.UpdatedAt)
		})
}
