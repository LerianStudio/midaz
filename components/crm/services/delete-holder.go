// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteHolderByID deletes a holder by its ID.
func (uc *UseCase) DeleteHolderByID(ctx context.Context, organizationID string, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_holder_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	count, err := uc.InstrumentRepo.Count(ctx, organizationID, id)
	if err != nil {
		recordSpanError(span, "Failed to check linked aliases for holder", err)

		return err
	}

	if count > 0 {
		return pkg.ValidateBusinessError(cn.ErrHolderHasInstruments, cn.EntityHolder)
	}

	err = uc.HolderRepo.Delete(ctx, organizationID, id, hardDelete)
	if err != nil {
		recordSpanError(span, "Failed to delete holder by id", err)

		return err
	}

	return nil
}
