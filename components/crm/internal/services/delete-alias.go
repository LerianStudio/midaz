package services

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteAliasByID removes an alias by its id and holder id
func (uc *UseCase) DeleteAliasByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_alias_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	logger.Infof("Delete alias by id %v", id)

	holderLinks, err := uc.HolderLinkRepo.FindByAliasID(ctx, organizationID, id, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to find holder links by alias id: %v", err)

		logger.Errorf("Failed to find holder links by alias id: %v", err)

		return err
	}

	if len(holderLinks) == 0 {
		libOpenTelemetry.HandleSpanError(&span, "No holder links found for alias id", cn.ErrHolderLinkNotFound)

		logger.Errorf("No holder links found for alias id: %v", id)

		return cn.ErrHolderLinkNotFound
	}

	err = uc.HolderLinkRepo.Delete(ctx, organizationID, *holderLinks[0].ID, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete holder link by id: %v", err)

		logger.Errorf("Failed to delete holder link by id: %v", err)

		return err
	}

	err = uc.AliasRepo.Delete(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete alias by id: %v", err)

		logger.Errorf("Failed to delete alias by id: %v", err)

		return err
	}

	return nil
}
