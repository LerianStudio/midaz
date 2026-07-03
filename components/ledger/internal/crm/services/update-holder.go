// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UpdateHolderByID updates a holder by its ID.
func (uc *UseCase) UpdateHolderByID(ctx context.Context, organizationID string, id uuid.UUID, uhi *mmodel.UpdateHolderInput, fieldsToRemove []string) (_ *mmodel.Holder, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_holder")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "update_holder", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

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
		recordSpanError(span, "Failed to update holder", err)

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
