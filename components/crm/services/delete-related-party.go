// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) DeleteRelatedPartyByID(ctx context.Context, organizationID string, holderID, aliasID, relatedPartyID uuid.UUID) (err error) {
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
		attribute.String("app.request.instrument_id", aliasID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	err = uc.InstrumentRepo.DeleteRelatedParty(ctx, organizationID, holderID, aliasID, relatedPartyID)
	if err != nil {
		recordSpanError(span, "Failed to delete related party", err)

		return err
	}

	return nil
}
