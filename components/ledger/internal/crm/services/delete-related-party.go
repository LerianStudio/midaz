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

func (uc *UseCase) DeleteRelatedPartyByID(ctx context.Context, organizationID string, holderID, instrumentID, relatedPartyID uuid.UUID) (err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_related_party")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "delete_related_party", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", instrumentID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	err = uc.InstrumentRepo.DeleteRelatedParty(ctx, organizationID, holderID, instrumentID, relatedPartyID)
	if err != nil {
		recordSpanError(span, "Failed to delete related party", err)

		return err
	}

	deletedAt := time.Now().UTC()

	uc.emitInstrumentRelatedPartyDeletedEvent(ctx, span, logger, instrumentID.String(), holderID.String(), organizationID, relatedPartyID.String(), deletedAt)

	return nil
}

func (uc *UseCase) emitInstrumentRelatedPartyDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, instrumentID, holderID, organizationID, relatedPartyID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.InstrumentRelatedPartyDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewInstrumentRelatedPartyDeleted(instrumentID, holderID, organizationID, relatedPartyID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
