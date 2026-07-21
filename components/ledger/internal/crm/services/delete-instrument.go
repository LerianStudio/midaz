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
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DeleteInstrumentByID removes an instrument by its ID and holder ID.
func (uc *UseCase) DeleteInstrumentByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) (err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_instrument_by_id")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "delete_instrument", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
	)

	err = uc.InstrumentRepo.Delete(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		recordSpanError(span, "Failed to delete instrument by id", err)

		return err
	}

	deletedAt := time.Now().UTC()

	uc.emitInstrumentDeletedEvent(ctx, span, logger, id.String(), holderID.String(), organizationID, hardDelete, deletedAt)

	return nil
}

func (uc *UseCase) emitInstrumentDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, holderID, organizationID string, hardDelete bool, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.InstrumentDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewInstrumentDeleted(id, holderID, organizationID, hardDelete, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
