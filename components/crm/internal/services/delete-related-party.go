// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) DeleteRelatedPartyByID(ctx context.Context, organizationID string, holderID, aliasID, relatedPartyID uuid.UUID) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_related_party")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to delete related party: %v from alias: %v", relatedPartyID.String(), aliasID.String()))

	err := uc.AliasRepo.DeleteRelatedParty(ctx, organizationID, holderID, aliasID, relatedPartyID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete related party", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete related party: %v", err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully deleted related party: %v from alias: %v", relatedPartyID.String(), aliasID.String()))

	return nil
}
