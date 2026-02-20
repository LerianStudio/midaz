package services

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
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

	logger.Infof("Trying to delete related party: %v from alias: %v", relatedPartyID.String(), aliasID.String())

	err := uc.AliasRepo.DeleteRelatedParty(ctx, organizationID, holderID, aliasID, relatedPartyID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete related party", err)
		logger.Errorf("Failed to delete related party: %v", err)

		return err
	}

	logger.Infof("Successfully deleted related party: %v from alias: %v", relatedPartyID.String(), aliasID.String())

	return nil
}
