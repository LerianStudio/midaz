// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteInstrumentByID removes an instrument by its ID and holder ID.
func (uc *UseCase) DeleteInstrumentByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_instrument_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
	)

	err := uc.InstrumentRepo.Delete(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		recordSpanError(span, "Failed to delete alias by id", err)

		return err
	}

	return nil
}
